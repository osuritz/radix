package server

import (
	"bytes"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	mathrand "math/rand/v2"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"text/template"
	"time"

	"gopkg.in/yaml.v3"
)

// regexRoutePrefix marks a route path as a regular expression to be matched
// against the request path (e.g. `regex:^/api/v[0-9]+/x$`).
const regexRoutePrefix = "regex:"

// routeAlphanumeric is the alphabet used by the {{randomString n}} template
// helper.
const routeAlphanumeric = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789"

// FallbackType selects what happens when no custom route and no built-in
// endpoint matches a request.
type FallbackType string

const (
	// FallbackNotFound responds with 404 for unmatched requests (the default).
	FallbackNotFound FallbackType = "404"
	// FallbackProxy forwards unmatched requests to a configured proxy target.
	FallbackProxy FallbackType = "proxy"
)

// RoutesFile is the on-disk YAML schema for a mock routes configuration. Only
// the "Core" feature set is modeled here: settings, exact/param/regex/glob
// routes, inline or file-backed templated response bodies, and per-route delay.
//
// Advanced keys from the design doc (conditions, sequence, random, websocket,
// sse) are intentionally NOT modeled and are ignored gracefully when present:
// they unmarshal into nothing and have no effect. See the package docs / the
// mock command help for the supported subset.
type RoutesFile struct {
	Settings settingsYAML   `yaml:"settings"`
	Routes   []RouteDefYAML `yaml:"routes"`
}

// settingsYAML is the on-disk schema for the global `settings:` block. Every
// scalar is a pointer so that an absent field (nil) is distinguishable from an
// explicit zero/false value: this lets a file `cors: false` or `fail_rate: 0`
// override an otherwise-on default, while an omitted field leaves the
// CLI/default value untouched. The pointers are resolved into the concrete
// RouteSettings by normalizeSettings.
type settingsYAML struct {
	Latency       *yamlDuration  `yaml:"latency"`
	LatencyJitter *yamlDuration  `yaml:"latency_jitter"`
	FailRate      *float64       `yaml:"fail_rate"`
	FailStatus    *int           `yaml:"fail_status"`
	CORS          *bool          `yaml:"cors"`
	Fallback      FallbackConfig `yaml:"fallback"`
}

// RouteSettings holds the effective global mock settings used at request time.
// It is the resolved form of settingsYAML after defaults are filled and CLI
// overrides are merged. CLI flags take precedence over file settings, which in
// turn take precedence over the built-in defaults; merging is performed by the
// CLI layer via the store's reload overrides.
type RouteSettings struct {
	Latency       time.Duration
	LatencyJitter time.Duration
	FailRate      float64
	FailStatus    int
	CORS          bool
	Fallback      FallbackConfig
}

// FallbackConfig configures the unmatched-request fallback behavior.
type FallbackConfig struct {
	Type        FallbackType `yaml:"type"`
	ProxyTarget string       `yaml:"proxy_target"`
}

// RouteDefYAML is a single route definition as it appears in YAML. It accepts
// either a single Method or a list of Methods (Method takes precedence when both
// are set). An absent method/methods matches any method.
type RouteDefYAML struct {
	Path        string       `yaml:"path"`
	Method      string       `yaml:"method"`
	Methods     []string     `yaml:"methods"`
	Delay       yamlDuration `yaml:"delay"`
	DelayJitter yamlDuration `yaml:"delay_jitter"`
	Response    ResponseYAML `yaml:"response"`
}

// yamlDuration is a time.Duration that unmarshals from either a Go duration
// string (e.g. "200ms", "2s") or a bare number interpreted as seconds (so
// "latency: 0" and "delay: 1.5" both work). It keeps the YAML schema friendly
// while the rest of the code uses native time.Duration.
type yamlDuration time.Duration

// UnmarshalYAML parses a yamlDuration from a string or numeric scalar.
func (d *yamlDuration) UnmarshalYAML(value *yaml.Node) error {
	var s string
	if err := value.Decode(&s); err == nil {
		parsed, err := time.ParseDuration(strings.TrimSpace(s))
		if err != nil {
			return fmt.Errorf("invalid duration %q: %w", s, err)
		}
		*d = yamlDuration(parsed)
		return nil
	}
	var secs float64
	if err := value.Decode(&secs); err != nil {
		return fmt.Errorf("invalid duration %q: expected a duration string or number of seconds", value.Value)
	}
	*d = yamlDuration(time.Duration(secs * float64(time.Second)))
	return nil
}

// Duration returns the value as a time.Duration.
func (d yamlDuration) Duration() time.Duration { return time.Duration(d) }

// ResponseYAML describes the response a route produces. Body is an inline,
// templated body; File names a file (relative to the routes-file directory)
// whose templated contents are used as the body. Body takes precedence over
// File when both are set.
type ResponseYAML struct {
	Status  int               `yaml:"status"`
	Headers map[string]string `yaml:"headers"`
	Body    string            `yaml:"body"`
	File    string            `yaml:"file"`
}

// routeKind classifies how a compiled route matches a request path.
type routeKind int

const (
	routeExact routeKind = iota
	routeParam
	routeRegex
	routeGlob
)

// compiledRoute is the precompiled, request-time form of a RouteDefYAML.
type compiledRoute struct {
	kind        routeKind
	rawPath     string
	methods     map[string]struct{} // nil => any method
	delay       time.Duration
	delayJitter time.Duration

	// Match data, depending on kind.
	segments []string       // routeParam: split path segments (":name" for params)
	re       *regexp.Regexp // routeRegex
	globBase string         // routeGlob: prefix before the trailing "/*"

	status   int
	headers  map[string]string
	bodyTmpl *template.Template // parsed inline body template (nil when file-backed or empty)
	filePath string             // file: response body path, relative to baseDir (empty when inline)
}

// CompiledRoutes is the immutable, request-time representation of a routes file.
// It is built once by LoadRoutes / CompileRoutes and then read concurrently by
// many request goroutines without locking. Swap a whole *CompiledRoutes value
// atomically to reload; never mutate one in place.
type CompiledRoutes struct {
	routes   []compiledRoute
	settings RouteSettings
	baseDir  string // directory of the routes file, for resolving file: bodies
}

// Settings returns the global settings parsed from the routes file. The
// returned value is a copy and safe to read concurrently.
func (c *CompiledRoutes) Settings() RouteSettings {
	return c.settings
}

// LoadRoutes reads and compiles a routes file from disk. The returned
// CompiledRoutes is safe for concurrent request handling. File-backed response
// bodies are resolved relative to the directory containing path.
func LoadRoutes(path string) (*CompiledRoutes, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return nil, fmt.Errorf("mock routes: resolve path %q: %w", path, err)
	}
	// #nosec G304 - routes file path is user-provided (a dev tool config).
	data, err := os.ReadFile(abs)
	if err != nil {
		return nil, fmt.Errorf("mock routes: read %q: %w", abs, err)
	}
	return CompileRoutes(data, filepath.Dir(abs))
}

// CompileRoutes parses YAML route configuration and compiles it into a
// request-time matcher. baseDir is used to resolve file: response bodies and to
// guard against path traversal. It returns an error for malformed YAML, an
// invalid fallback type, or an invalid proxy target.
func CompileRoutes(data []byte, baseDir string) (*CompiledRoutes, error) {
	var rf RoutesFile
	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(false) // ignore unsupported keys (conditions, sequence, etc.)
	if err := dec.Decode(&rf); err != nil && !errors.Is(err, io.EOF) {
		return nil, fmt.Errorf("mock routes: parse YAML: %w", err)
	}

	settings, err := normalizeSettings(&rf.Settings)
	if err != nil {
		return nil, err
	}

	compiled := make([]compiledRoute, 0, len(rf.Routes))
	for i := range rf.Routes {
		rd := &rf.Routes[i]
		cr, cErr := compileRoute(rd)
		if cErr != nil {
			return nil, fmt.Errorf("mock routes: route #%d (%q): %w", i+1, rd.Path, cErr)
		}
		compiled = append(compiled, cr)
	}

	// Order routes by matching priority so the first match in slice order wins:
	// exact+method, exact+any, param, regex, glob. Sort is stable to preserve
	// the file order within each tier.
	sortRoutesByPriority(compiled)

	abs := baseDir
	if a, aErr := filepath.Abs(baseDir); aErr == nil {
		abs = a
	}

	return &CompiledRoutes{routes: compiled, settings: settings, baseDir: abs}, nil
}

// normalizeSettings validates the parsed file settings and resolves them into
// the effective RouteSettings. Fields the file leaves absent (nil) keep their
// built-in defaults here; the CLI layer later overlays any explicitly-set flags
// (see RoutesStore overrides), so the precedence is CLI > file > default. The
// fail_status default of 500 is therefore only a fallback for when neither the
// file nor a CLI flag supplies one.
func normalizeSettings(s *settingsYAML) (RouteSettings, error) {
	out := RouteSettings{
		FailStatus: http.StatusInternalServerError,
		Fallback:   s.Fallback,
	}

	if s.Latency != nil {
		out.Latency = s.Latency.Duration()
	}
	if s.LatencyJitter != nil {
		out.LatencyJitter = s.LatencyJitter.Duration()
	}
	if out.Latency < 0 || out.LatencyJitter < 0 {
		return RouteSettings{}, errors.New("mock routes: settings latency and latency_jitter must not be negative")
	}
	if s.FailRate != nil {
		out.FailRate = *s.FailRate
	}
	if s.FailStatus != nil && *s.FailStatus != 0 {
		out.FailStatus = *s.FailStatus
	}
	if s.CORS != nil {
		out.CORS = *s.CORS
	}

	if out.Fallback.Type == "" {
		out.Fallback.Type = FallbackNotFound
	}
	switch out.Fallback.Type {
	case FallbackNotFound:
		// ok
	case FallbackProxy:
		if err := validateProxyTarget(out.Fallback.ProxyTarget); err != nil {
			return RouteSettings{}, err
		}
	default:
		return RouteSettings{}, fmt.Errorf("mock routes: invalid fallback.type %q (must be %q or %q)",
			out.Fallback.Type, FallbackNotFound, FallbackProxy)
	}
	return out, nil
}

// validateProxyTarget ensures a fallback proxy target is a usable http/https URL.
func validateProxyTarget(target string) error {
	if strings.TrimSpace(target) == "" {
		return errors.New("mock routes: fallback.proxy_target is required when fallback.type is \"proxy\"")
	}
	u, err := url.Parse(target)
	if err != nil {
		return fmt.Errorf("mock routes: invalid fallback.proxy_target %q: %w", target, err)
	}
	if (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return fmt.Errorf("mock routes: invalid fallback.proxy_target %q: must be an http(s) URL", target)
	}
	return nil
}

// compileRoute turns a single YAML route definition into its request-time form,
// classifying its path and parsing its body template.
func compileRoute(rd *RouteDefYAML) (compiledRoute, error) {
	path := strings.TrimSpace(rd.Path)
	if path == "" {
		return compiledRoute{}, errors.New("path is required")
	}

	cr := compiledRoute{
		rawPath:     path,
		methods:     methodSet(rd),
		delay:       rd.Delay.Duration(),
		delayJitter: rd.DelayJitter.Duration(),
		status:      rd.Response.Status,
		headers:     rd.Response.Headers,
		filePath:    strings.TrimSpace(rd.Response.File),
	}
	if cr.status == 0 {
		cr.status = http.StatusOK
	}
	if cr.delay < 0 || cr.delayJitter < 0 {
		return compiledRoute{}, errors.New("delay and delay_jitter must not be negative")
	}

	switch {
	case strings.HasPrefix(path, regexRoutePrefix):
		// regex: patterns use Go regexp (regexp.MatchString) semantics and are
		// NOT auto-anchored — they match if the pattern is found anywhere in the
		// path. Use ^...$ to match the whole path (e.g. "regex:^/api/v[0-9]+$").
		expr := strings.TrimPrefix(path, regexRoutePrefix)
		re, err := regexp.Compile(expr)
		if err != nil {
			return compiledRoute{}, fmt.Errorf("invalid regex %q: %w", expr, err)
		}
		cr.kind = routeRegex
		cr.re = re
	case strings.HasSuffix(path, "/*"):
		cr.kind = routeGlob
		cr.globBase = strings.TrimSuffix(path, "/*")
	case strings.Contains(path, "/:"):
		cr.kind = routeParam
		cr.segments = splitPath(path)
	default:
		cr.kind = routeExact
	}

	tmpl, err := compileBodyTemplate(rd.Response)
	if err != nil {
		return compiledRoute{}, err
	}
	cr.bodyTmpl = tmpl

	return cr, nil
}

// compileBodyTemplate parses the inline body (or, if absent, defers file bodies
// to request time). An inline body that fails to parse is a load-time error so
// broken templates are caught before serving. File bodies are read and parsed
// per request (they live outside the config and may change), so only the
// presence of a path is validated here.
func compileBodyTemplate(resp ResponseYAML) (*template.Template, error) {
	if resp.Body != "" {
		return parseRouteTemplate("body", resp.Body)
	}
	return nil, nil // file body (or empty body) handled at request time
}

// parseRouteTemplate parses a template string with the route FuncMap installed.
func parseRouteTemplate(name, text string) (*template.Template, error) {
	t, err := template.New(name).Funcs(routeFuncMap()).Parse(text)
	if err != nil {
		return nil, fmt.Errorf("parse template: %w", err)
	}
	return t, nil
}

// methodSet builds the set of accepted methods for a route. Method takes
// precedence over Methods; an empty result means "any method".
func methodSet(rd *RouteDefYAML) map[string]struct{} {
	add := func(set map[string]struct{}, m string) {
		m = strings.ToUpper(strings.TrimSpace(m))
		if m != "" {
			set[m] = struct{}{}
		}
	}
	set := make(map[string]struct{})
	if rd.Method != "" {
		add(set, rd.Method)
	} else {
		for _, m := range rd.Methods {
			add(set, m)
		}
	}
	if len(set) == 0 {
		return nil
	}
	return set
}

// splitPath splits a path into non-empty segments.
func splitPath(p string) []string {
	parts := strings.Split(strings.Trim(p, "/"), "/")
	out := parts[:0]
	for _, s := range parts {
		if s != "" {
			out = append(out, s)
		}
	}
	return out
}

// priorityTier returns the sort tier for a route, lower meaning higher priority.
// Exact routes are split so that method-specific exact routes outrank
// any-method exact routes.
func priorityTier(cr *compiledRoute) int {
	switch cr.kind {
	case routeExact:
		if cr.methods == nil {
			return 1 // exact + any-method
		}
		return 0 // exact + specific method
	case routeParam:
		return 2
	case routeRegex:
		return 3
	case routeGlob:
		return 4
	default:
		return 5
	}
}

// sortRoutesByPriority stably orders routes by matching priority tier.
func sortRoutesByPriority(routes []compiledRoute) {
	// Stable insertion-style sort to preserve in-file order within each tier
	// without pulling in sort.SliceStable's reflection for such small slices.
	for i := 1; i < len(routes); i++ {
		for j := i; j > 0 && priorityTier(&routes[j]) < priorityTier(&routes[j-1]); j-- {
			routes[j], routes[j-1] = routes[j-1], routes[j]
		}
	}
}

// match returns the first route matching the request along with any extracted
// path params, or ok=false when nothing matches. Routes are already ordered by
// priority, so the first match in slice order wins.
func (c *CompiledRoutes) match(method, path string) (*compiledRoute, map[string]string, bool) {
	method = strings.ToUpper(method)
	for i := range c.routes {
		cr := &c.routes[i]
		if !cr.methodMatches(method) {
			continue
		}
		if params, ok := cr.pathMatches(path); ok {
			return cr, params, true
		}
	}
	return nil, nil, false
}

// methodMatches reports whether the route accepts the given method.
func (cr *compiledRoute) methodMatches(method string) bool {
	if cr.methods == nil {
		return true
	}
	_, ok := cr.methods[method]
	return ok
}

// pathMatches reports whether the route's path pattern matches path, returning
// any captured :param values.
func (cr *compiledRoute) pathMatches(path string) (map[string]string, bool) {
	switch cr.kind {
	case routeExact:
		return nil, path == cr.rawPath
	case routeParam:
		return matchParamPath(cr.segments, path)
	case routeRegex:
		return nil, cr.re.MatchString(path)
	case routeGlob:
		return nil, path == cr.globBase || strings.HasPrefix(path, cr.globBase+"/")
	default:
		return nil, false
	}
}

// matchParamPath matches a request path against ":param"-style segments,
// returning the captured parameters on success.
func matchParamPath(segments []string, path string) (map[string]string, bool) {
	reqSegs := splitPath(path)
	if len(reqSegs) != len(segments) {
		return nil, false
	}
	var params map[string]string
	for i, seg := range segments {
		if strings.HasPrefix(seg, ":") {
			if params == nil {
				params = make(map[string]string)
			}
			params[seg[1:]] = reqSegs[i]
			continue
		}
		if seg != reqSegs[i] {
			return nil, false
		}
	}
	return params, true
}

// serve renders and writes the route's response for the given request and
// extracted params. baseDir bounds file: body resolution.
func (cr *compiledRoute) serve(w http.ResponseWriter, r *http.Request, params map[string]string, baseDir string) {
	// Per-route delay (fixed + jitter), honoring request cancellation.
	if d := cr.delay + jitter(cr.delayJitter); d > 0 {
		t := time.NewTimer(d)
		defer t.Stop()
		select {
		case <-r.Context().Done():
			return
		case <-t.C:
		}
	}

	data, err := buildTemplateData(w, r, params)
	if err != nil {
		if errors.Is(err, errMockBodyTooLarge) {
			writeBodyTooLarge(w)
			return
		}
		http.Error(w, fmt.Sprintf("mock: build template data: %v", err), http.StatusInternalServerError)
		return
	}

	body, err := cr.renderBody(data, baseDir)
	if err != nil {
		http.Error(w, fmt.Sprintf("mock: render response: %v", err), http.StatusInternalServerError)
		return
	}

	for k, v := range cr.headers {
		w.Header().Set(k, v)
	}
	w.WriteHeader(cr.status)
	_, _ = w.Write(body)
}

// renderBody produces the response body, rendering either the precompiled
// inline template or the (templated) contents of the route's file.
func (cr *compiledRoute) renderBody(data map[string]any, baseDir string) ([]byte, error) {
	if cr.bodyTmpl != nil {
		return execTemplate(cr.bodyTmpl, data)
	}
	if cr.filePath != "" {
		return cr.renderFileBody(data, baseDir)
	}
	return nil, nil // no body
}

// renderFileBody reads the route's file (guarded against path traversal),
// renders it as a template, and returns the result.
func (cr *compiledRoute) renderFileBody(data map[string]any, baseDir string) ([]byte, error) {
	resolved, err := resolveWithinBase(baseDir, cr.filePath)
	if err != nil {
		return nil, err
	}
	// #nosec G304 - path is validated to stay within the routes-file directory.
	raw, err := os.ReadFile(resolved)
	if err != nil {
		return nil, fmt.Errorf("read file body %q: %w", cr.filePath, err)
	}
	tmpl, err := parseRouteTemplate("file", string(raw))
	if err != nil {
		return nil, err
	}
	return execTemplate(tmpl, data)
}

// resolveWithinBase cleans rel against base and verifies the result does not
// escape base, defeating both "../" path-traversal and symlink-escape attempts.
//
// The lexical check (clean + prefix) blocks "../" and the /a/b vs /a/bc prefix
// pitfall, but a symlink inside base pointing outside it would still be followed
// by os.ReadFile, leaking external content. To close that, the real (symlink-
// resolved) paths of base and the target are compared: EvalSymlinks resolves
// every component. When the target does not yet exist (EvalSymlinks errors), its
// parent directory is resolved instead and the filename re-joined, so a
// not-yet-created file is handled gracefully while a symlinked parent is still
// caught.
func resolveWithinBase(base, rel string) (string, error) {
	absBase, err := filepath.Abs(base)
	if err != nil {
		return "", fmt.Errorf("resolve base dir: %w", err)
	}
	cleaned := filepath.Clean(filepath.Join(absBase, rel))

	// Resolve the base dir's real path once; all containment checks are made
	// against it so a symlinked base directory is itself handled correctly.
	realBase, err := filepath.EvalSymlinks(absBase)
	if err != nil {
		return "", fmt.Errorf("resolve routes directory: %w", err)
	}

	realPath, err := filepath.EvalSymlinks(cleaned)
	if err != nil {
		if !os.IsNotExist(err) {
			return "", fmt.Errorf("resolve file path %q: %w", rel, err)
		}
		// Target does not exist yet: resolve its parent and re-join the name so a
		// symlinked parent directory still cannot escape base.
		realParent, perr := filepath.EvalSymlinks(filepath.Dir(cleaned))
		if perr != nil {
			return "", fmt.Errorf("resolve file path %q: %w", rel, perr)
		}
		realPath = filepath.Join(realParent, filepath.Base(cleaned))
	}

	if realPath != realBase && !strings.HasPrefix(realPath, realBase+string(os.PathSeparator)) {
		return "", fmt.Errorf("file path %q escapes the routes directory", rel)
	}
	return realPath, nil
}

// execTemplate runs a parsed template against data and returns the rendered
// bytes, wrapping execution errors.
func execTemplate(t *template.Template, data map[string]any) ([]byte, error) {
	var buf bytes.Buffer
	if err := t.Execute(&buf, data); err != nil {
		return nil, fmt.Errorf("execute template: %w", err)
	}
	return buf.Bytes(), nil
}

// jitter returns a random duration in [0, upper) (0 when upper <= 0).
func jitter(upper time.Duration) time.Duration {
	if upper <= 0 {
		return 0
	}
	return time.Duration(mathrand.Int64N(int64(upper)))
}

// buildTemplateData assembles the dot-accessible data context exposed to
// response templates: method, path, params, query, headers, and the parsed JSON
// body (or nil). The request body read is bounded by maxMockBodyBytes.
func buildTemplateData(w http.ResponseWriter, r *http.Request, params map[string]string) (map[string]any, error) {
	if params == nil {
		params = map[string]string{}
	}

	query := make(map[string]string)
	for k, vs := range r.URL.Query() {
		if len(vs) > 0 {
			query[k] = vs[0]
		}
	}

	headers := make(map[string]string)
	for k, vs := range r.Header {
		if len(vs) > 0 {
			headers[k] = vs[0] // canonical key, first value
		}
	}

	var bodyVal any
	if r.Body != nil {
		raw, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxMockBodyBytes))
		if err != nil {
			var maxErr *http.MaxBytesError
			if errors.As(err, &maxErr) {
				return nil, errMockBodyTooLarge
			}
			// Other read errors: proceed with whatever was read.
		}
		bodyVal = parseJSONBody(raw, r.Header.Get("Content-Type"))
	}

	return map[string]any{
		"method":  r.Method,
		"path":    r.URL.Path,
		"params":  params,
		"query":   query,
		"headers": headers,
		"body":    bodyVal,
	}, nil
}

// routeFuncMap returns the template helper functions available in response
// bodies. Generators use math/rand/v2 (acceptable for mock data; not security
// sensitive). uuid wraps uuidV4 so a RNG error surfaces as a template error
// (which the handler turns into a 500) rather than crashing.
func routeFuncMap() template.FuncMap {
	return template.FuncMap{
		"uuid": func() (string, error) {
			u, err := uuidV4()
			if err != nil {
				return "", fmt.Errorf("uuid: %w", err)
			}
			return u, nil
		},
		"now":       func() string { return time.Now().UTC().Format(time.RFC3339) },
		"timestamp": func() int64 { return time.Now().Unix() },
		"random": func(low, high int) (int, error) {
			if high <= low {
				return 0, fmt.Errorf("random: high (%d) must be greater than low (%d)", high, low)
			}
			return low + mathrand.IntN(high-low), nil
		},
		"randomString": func(n int) string {
			if n <= 0 {
				return ""
			}
			b := make([]byte, n)
			for i := range b {
				b[i] = routeAlphanumeric[mathrand.IntN(len(routeAlphanumeric))]
			}
			return string(b)
		},
		"env":    os.Getenv,
		"base64": func(s string) string { return base64.StdEncoding.EncodeToString([]byte(s)) },
	}
}
