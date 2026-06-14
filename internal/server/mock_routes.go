package server

import (
	"bytes"
	"crypto/md5"  //nolint:gosec // md5 is offered only for non-security mock fixture generation, never for auth/integrity.
	"crypto/sha1" //nolint:gosec // sha1 is offered only for non-security mock fixture generation, never for auth/integrity.
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	mathrand "math/rand/v2"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync/atomic"
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

// loremWords is the static word pool the {{lorem n}} template helper draws from.
// It is a small, hand-rolled list (no external faker/lorem dependency) — enough
// variety for readable placeholder copy without pulling in a package.
var loremWords = []string{
	"lorem", "ipsum", "dolor", "sit", "amet", "consectetur", "adipiscing", "elit",
	"sed", "do", "eiusmod", "tempor", "incididunt", "ut", "labore", "et", "dolore",
	"magna", "aliqua", "enim", "ad", "minim", "veniam", "quis", "nostrud",
	"exercitation", "ullamco", "laboris", "nisi", "aliquip", "ex", "ea", "commodo",
	"consequat", "duis", "aute", "irure", "in", "reprehenderit", "voluptate",
	"velit", "esse", "cillum", "fugiat", "nulla", "pariatur", "excepteur", "sint",
	"occaecat", "cupidatat", "non", "proident", "sunt", "culpa", "qui", "officia",
	"deserunt", "mollit", "anim", "id", "est", "laborum",
}

// fakerFirstNames, fakerLastNames, fakerStreets, fakerCities, fakerEmailDomains,
// and fakerStreetSuffixes are small static pools backing the {{faker.*}} helpers.
// They are deliberately hand-rolled (no external faker dependency) and only need
// enough entries to produce plausible, varied placeholder data.
var (
	fakerFirstNames = []string{
		"Alice", "Bob", "Carol", "Dave", "Eve", "Frank", "Grace", "Heidi",
		"Ivan", "Judy", "Mallory", "Niaj", "Olivia", "Peggy", "Rupert", "Sybil",
		"Trent", "Victor", "Walter", "Wendy",
	}
	fakerLastNames = []string{
		"Anderson", "Brown", "Clark", "Davis", "Evans", "Garcia", "Harris",
		"Johnson", "Jones", "Lee", "Martinez", "Miller", "Moore", "Nguyen",
		"Smith", "Taylor", "Thomas", "Walker", "White", "Wilson",
	}
	fakerStreets = []string{
		"Maple", "Oak", "Pine", "Cedar", "Elm", "Washington", "Lake", "Hill",
		"Park", "Sunset", "Lincoln", "River", "Spring", "Highland", "Forest",
	}
	fakerStreetSuffixes = []string{"St", "Ave", "Rd", "Blvd", "Ln", "Way", "Dr"}
	fakerCities         = []string{
		"Springfield", "Riverton", "Fairview", "Madison", "Georgetown",
		"Franklin", "Clinton", "Greenville", "Bristol", "Salem", "Newport",
		"Ashland", "Burlington", "Manchester", "Oxford",
	}
	fakerEmailDomains = []string{"example.com", "example.org", "example.net", "test.dev"}
)

// FallbackType selects what happens when no custom route and no built-in
// endpoint matches a request.
type FallbackType string

const (
	// FallbackNotFound responds with 404 for unmatched requests (the default).
	FallbackNotFound FallbackType = "404"
	// FallbackProxy forwards unmatched requests to a configured proxy target.
	FallbackProxy FallbackType = "proxy"
)

// RoutesFile is the on-disk YAML schema for a mock routes configuration. The
// modeled feature set is: settings, exact/param/regex/glob routes, inline or
// file-backed templated response bodies, per-route delay, and conditional
// responses (the `conditions:` block).
//
// The remaining advanced keys from the design doc (sequence, random, websocket,
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
//
// A route may carry Conditions (request-content-driven response selection), a
// plain Response, or both (the plain Response then acts as the fallback when no
// condition arm matches). Response is a pointer so an absent `response:` (nil)
// is distinguishable from an explicit empty `response: {}`:
//   - A route with no conditions always has an effective response; an absent or
//     empty top-level response defaults to 200 with an empty body (preserving the
//     pre-conditions behavior where a path-only route served 200 empty).
//   - A route with conditions uses the top-level response as a no-match fallback
//     ONLY when one was explicitly provided (non-nil); otherwise a no-match is a
//     404.
type RouteDefYAML struct {
	Path        string          `yaml:"path"`
	Method      string          `yaml:"method"`
	Methods     []string        `yaml:"methods"`
	Delay       yamlDuration    `yaml:"delay"`
	DelayJitter yamlDuration    `yaml:"delay_jitter"`
	Response    *ResponseYAML   `yaml:"response"`
	Conditions  []ConditionYAML `yaml:"conditions"`
}

// ConditionYAML is one arm of a route's `conditions:` block. An arm either
// matches request content via Match or matches unconditionally via Default
// (intended as the last arm). The first arm whose every Match entry is
// satisfied wins and its Response is served.
//
// Match keys are dotted and must be prefixed with one of: "body.<field>" (a
// top-level field of the parsed JSON object or a form-urlencoded value),
// "query.<key>" (first query value), or "headers.<Name>" (canonical-cased,
// first header value). Nested body paths (e.g. "body.a.b") are NOT supported.
// A value of "*" matches when the key is present with any non-empty value; any
// other value requires an exact string match.
type ConditionYAML struct {
	Match    map[string]string `yaml:"match"`
	Default  bool              `yaml:"default"`
	Response ResponseYAML      `yaml:"response"`
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

// compiledResponse is the precompiled, request-time form of a ResponseYAML: a
// status, headers, and a body that is either a parsed inline template or a
// file path resolved (and traversal-guarded) per request. It is immutable after
// compilation and therefore safe for concurrent reads.
type compiledResponse struct {
	status   int
	headers  map[string]string
	bodyTmpl *template.Template // parsed inline body template (nil when file-backed or empty)
	filePath string             // file: response body path, relative to baseDir (empty when inline)
	seq      *atomic.Uint64     // owning route's {{seq}} counter, threaded to per-request file templates
}

// compiledCondition is the precompiled form of a ConditionYAML: a set of match
// rules (empty when the arm is the unconditional default) and the response to
// serve when the arm wins.
type compiledCondition struct {
	rules     []matchRule
	isDefault bool
	resp      compiledResponse
}

// matchKind selects how a single condition match entry is resolved against the
// request data.
type matchKind int

const (
	matchBody   matchKind = iota // body.<field>
	matchQuery                   // query.<key>
	matchHeader                  // headers.<Name>
)

// matchRule is one compiled "body.x: value" / "query.x: value" /
// "headers.X: value" entry. wildcard is true when the YAML value was "*",
// meaning "present with any non-empty value"; otherwise want holds the exact
// value required.
type matchRule struct {
	kind     matchKind
	key      string
	wildcard bool
	want     string
}

// compiledRoute is the precompiled, request-time form of a RouteDefYAML. It is
// immutable after compilation; conditions are read-only at request time, so a
// *CompiledRoutes can be read concurrently by many request goroutines.
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

	// resp is the route's effective top-level response and hasResp reports
	// whether it should be served. For a conditions route, hasResp is true only
	// when an explicit top-level `response:` was provided, and resp is then the
	// no-match fallback; when hasResp is false a no-match request is a 404. For a
	// route with no conditions hasResp is always true (an absent/empty response
	// is normalized to an empty 200).
	resp    compiledResponse
	hasResp bool

	// conditions, when non-empty, select the response by request content;
	// evaluated in order, first satisfied arm wins. See compiledRoute.serve.
	conditions []compiledCondition

	// seq is this route's private monotonic counter, backing the {{seq}} template
	// helper. It is allocated fresh per compileRoute call and shared by every
	// template this route owns (top-level body, file body, and each condition
	// arm), so all of a route's templates draw from one sequence. Because a
	// hot-reload builds entirely new compiledRoute values, the counter resets to
	// 0 on every reload. It is a pointer so the closures captured in the route's
	// FuncMaps mutate this route's counter (not a copy). Future sequence/conditions
	// work (#39) can reuse this same per-route counter.
	seq *atomic.Uint64
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
// classifying its path and parsing its response(s).
//
// Response presence (whether an explicit top-level `response:` was provided) is
// tracked in compiledRoute.hasResp and governs no-match behavior for conditional
// routes. A route with no conditions always gets an effective response: an absent
// or empty top-level response defaults to 200 with an empty body (back-compat
// with the pre-conditions behavior where `- path: /x` served 200 empty).
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
		seq:         new(atomic.Uint64),
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

	// hasResp records whether an explicit top-level `response:` was provided
	// (pointer non-nil). It governs the no-match fallback for conditional routes:
	// only an explicit response is used as a fallback; absent → 404.
	cr.hasResp = rd.Response != nil
	if cr.hasResp {
		resp, err := compileResponse(*rd.Response, cr.seq)
		if err != nil {
			return compiledRoute{}, err
		}
		cr.resp = resp
	}

	conds, err := compileConditions(rd.Conditions, cr.seq)
	if err != nil {
		return compiledRoute{}, err
	}
	cr.conditions = conds

	// A route with no conditions always has an effective response. When none was
	// provided (nil pointer), default to an empty 200 — this restores the
	// pre-conditions behavior where a path-only route (`- path: /x`) served 200
	// with an empty body. `response: {}` lands here too via compileResponse's
	// status default.
	if len(cr.conditions) == 0 && !cr.hasResp {
		cr.resp = compiledResponse{status: http.StatusOK}
		cr.hasResp = true
	}

	return cr, nil
}

// compileResponse parses a ResponseYAML into its request-time form: status
// (defaulting to 200), headers, and an inline body template or file path. An
// inline body that fails to parse is a load-time error so broken templates are
// caught before serving; file bodies are read and parsed per request (they live
// outside the config and may change), so only the presence of a path is
// validated here. seq is the owning route's {{seq}} counter, recorded so a
// per-request file template draws from the same sequence as the route's inline
// templates.
func compileResponse(resp ResponseYAML, seq *atomic.Uint64) (compiledResponse, error) {
	out := compiledResponse{
		status:   resp.Status,
		headers:  resp.Headers,
		filePath: strings.TrimSpace(resp.File),
		seq:      seq,
	}
	if out.status == 0 {
		out.status = http.StatusOK
	}
	if resp.Body != "" {
		tmpl, err := parseRouteTemplate("body", resp.Body, seq)
		if err != nil {
			return compiledResponse{}, err
		}
		out.bodyTmpl = tmpl
	}
	return out, nil
}

// compileConditions parses a route's condition arms, compiling each arm's match
// rules and response body template at load time so malformed templates and bad
// match keys fail fast. Arm order is preserved (first-match-wins at request
// time). seq is the owning route's {{seq}} counter, shared by every arm's
// response template.
func compileConditions(conds []ConditionYAML, seq *atomic.Uint64) ([]compiledCondition, error) {
	if len(conds) == 0 {
		return nil, nil
	}
	out := make([]compiledCondition, 0, len(conds))
	for i := range conds {
		c := &conds[i]
		resp, err := compileResponse(c.Response, seq)
		if err != nil {
			return nil, fmt.Errorf("condition #%d: %w", i+1, err)
		}
		cc := compiledCondition{isDefault: c.Default, resp: resp}
		if !c.Default {
			rules, rErr := compileMatchRules(c.Match)
			if rErr != nil {
				return nil, fmt.Errorf("condition #%d: %w", i+1, rErr)
			}
			// A non-default arm with no match rules would have matchAll([])
			// return true and silently match everything (e.g. a `matches:`
			// typo or an omitted match block). Reject it at load time.
			if len(rules) == 0 {
				return nil, fmt.Errorf("condition #%d has no match rules; use 'default: true' for an unconditional arm", i+1)
			}
			cc.rules = rules
		}
		out = append(out, cc)
	}
	return out, nil
}

// compileMatchRules turns a condition's dotted match map into ordered match
// rules. A key must be prefixed with "body.", "query.", or "headers."; any
// other prefix is a load-time error. A value of "*" is recorded as a wildcard
// (present/non-empty) rule.
func compileMatchRules(match map[string]string) ([]matchRule, error) {
	rules := make([]matchRule, 0, len(match))
	for rawKey, want := range match {
		kind, key, err := parseMatchKey(rawKey)
		if err != nil {
			return nil, err
		}
		rules = append(rules, matchRule{
			kind:     kind,
			key:      key,
			wildcard: want == "*",
			want:     want,
		})
	}
	return rules, nil
}

// parseMatchKey splits a dotted match key into its kind and bare key, rejecting
// any key that is not prefixed with "body.", "query.", or "headers.".
func parseMatchKey(rawKey string) (matchKind, string, error) {
	switch {
	case strings.HasPrefix(rawKey, "body."):
		return matchBody, strings.TrimPrefix(rawKey, "body."), nil
	case strings.HasPrefix(rawKey, "query."):
		return matchQuery, strings.TrimPrefix(rawKey, "query."), nil
	case strings.HasPrefix(rawKey, "headers."):
		return matchHeader, strings.TrimPrefix(rawKey, "headers."), nil
	default:
		return 0, "", fmt.Errorf("invalid match key %q: must be prefixed with %q, %q, or %q",
			rawKey, "body.", "query.", "headers.")
	}
}

// parseRouteTemplate parses a template string with the route FuncMap installed.
// seq is the owning route's {{seq}} counter, captured by the FuncMap's seq
// closure so the helper increments that specific route's sequence.
func parseRouteTemplate(name, text string, seq *atomic.Uint64) (*template.Template, error) {
	t, err := template.New(name).Funcs(routeFuncMap(seq)).Parse(text)
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
//
// The request body is parsed exactly once (via buildTemplateData, bounded by
// maxMockBodyBytes) and the resulting data context is shared by both condition
// matching and response templating, so conditions and templates always see
// identical data. When the route has conditions they are evaluated in order and
// the first satisfied arm's response is served; selectResponse documents the
// precedence (winning arm > default arm > top-level response > 404).
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

	resp, ok := cr.selectResponse(data)
	if !ok {
		// Route had conditions but no arm matched and no top-level fallback.
		http.NotFound(w, r)
		return
	}

	body, err := resp.renderBody(data, baseDir)
	if err != nil {
		http.Error(w, fmt.Sprintf("mock: render response: %v", err), http.StatusInternalServerError)
		return
	}

	for k, v := range resp.headers {
		w.Header().Set(k, v)
	}
	w.WriteHeader(resp.status)
	_, _ = w.Write(body)
}

// selectResponse chooses the response to serve for the given request data.
//
// Precedence:
//  1. With conditions: the first arm whose every match rule is satisfied wins
//     (a default arm always matches). The arms are evaluated in file order.
//  2. If no arm matches but the route has a top-level response, that response
//     is used as the fallback.
//  3. Otherwise ok is false and the caller responds 404.
//
// A route with no conditions simply returns its top-level response.
func (cr *compiledRoute) selectResponse(data map[string]any) (compiledResponse, bool) {
	for i := range cr.conditions {
		c := &cr.conditions[i]
		if c.isDefault || matchAll(c.rules, data) {
			return c.resp, true
		}
	}
	if cr.hasResp {
		return cr.resp, true
	}
	return compiledResponse{}, false
}

// matchAll reports whether every rule is satisfied by the request data.
func matchAll(rules []matchRule, data map[string]any) bool {
	for i := range rules {
		if !rules[i].matches(data) {
			return false
		}
	}
	return true
}

// matches reports whether a single rule is satisfied by the request data. A
// wildcard ("*") rule requires the resolved value to be present and non-empty;
// any other rule requires an exact string match.
func (mr *matchRule) matches(data map[string]any) bool {
	val, ok := resolveMatchValue(mr.kind, mr.key, data)
	if !ok {
		return false
	}
	if mr.wildcard {
		return val != ""
	}
	return val == mr.want
}

// resolveMatchValue extracts the string value addressed by a match rule from
// the shared template data. For body keys it resolves a top-level field of the
// parsed JSON object or a form-urlencoded value (first value); nested paths are
// not supported, and only scalar fields are useful match targets. Query and
// header keys resolve the first value (header names are canonical-cased,
// matching buildTemplateData). ok is false when the addressed key is absent.
func resolveMatchValue(kind matchKind, key string, data map[string]any) (string, bool) {
	switch kind {
	case matchBody:
		return resolveBodyField(data["body"], key)
	case matchQuery:
		return lookupStringMap(data["query"], key)
	case matchHeader:
		return lookupStringMap(data["headers"], http.CanonicalHeaderKey(key))
	default:
		return "", false
	}
}

// resolveBodyField resolves a top-level field of the parsed request body. The
// body is either a JSON object (map[string]any) whose values are stringified, or
// a form-urlencoded map (map[string]string of first values). Only scalar
// top-level fields are meaningful match targets: a field whose value is a nested
// JSON object or array stringifies to a Go-rendered form that is not a useful
// match target (matching only against scalars is intended).
func resolveBodyField(body any, field string) (string, bool) {
	switch b := body.(type) {
	case map[string]any:
		v, ok := b[field]
		if !ok {
			return "", false
		}
		return stringifyJSONValue(v), true
	case map[string]string:
		v, ok := b[field]
		if !ok {
			return "", false
		}
		return v, true
	default:
		return "", false
	}
}

// stringifyJSONValue renders a decoded JSON scalar as the string used for
// condition matching. The body is decoded with json.Number (see
// parseJSONBodyForRoutes), so numbers carry their exact source text and a body
// like {"id":1000000} matches body.id: "1000000" (not float64's "1e+06").
//
// Scalars are handled as: strings pass through; json.Number renders its exact
// text; a float64 fallback (should the body ever be decoded without UseNumber)
// is formatted without an exponent; booleans render "true"/"false"; JSON null
// renders "" (so null compares equal to an exact empty-string match, the same as
// an explicit ""). Objects/arrays are not meaningful scalar match targets and
// fall through to fmt's default rendering.
func stringifyJSONValue(v any) string {
	switch val := v.(type) {
	case string:
		return val
	case json.Number:
		return val.String()
	case float64:
		return strconv.FormatFloat(val, 'f', -1, 64)
	case nil:
		return ""
	default:
		return fmt.Sprintf("%v", val)
	}
}

// lookupStringMap reads key from a map[string]string value (the query/headers
// maps built by buildTemplateData), reporting ok=false when absent.
func lookupStringMap(m any, key string) (string, bool) {
	mm, ok := m.(map[string]string)
	if !ok {
		return "", false
	}
	v, ok := mm[key]
	return v, ok
}

// renderBody produces the response body, rendering either the precompiled
// inline template or the (templated) contents of the response's file.
func (resp *compiledResponse) renderBody(data map[string]any, baseDir string) ([]byte, error) {
	if resp.bodyTmpl != nil {
		return execTemplate(resp.bodyTmpl, data)
	}
	if resp.filePath != "" {
		return resp.renderFileBody(data, baseDir)
	}
	return nil, nil // no body
}

// renderFileBody reads the response's file (guarded against path traversal),
// renders it as a template, and returns the result.
func (resp *compiledResponse) renderFileBody(data map[string]any, baseDir string) ([]byte, error) {
	resolved, err := resolveWithinBase(baseDir, resp.filePath)
	if err != nil {
		return nil, err
	}
	// #nosec G304 - path is validated to stay within the routes-file directory.
	raw, err := os.ReadFile(resolved)
	if err != nil {
		return nil, fmt.Errorf("read file body %q: %w", resp.filePath, err)
	}
	tmpl, err := parseRouteTemplate("file", string(raw), resp.seq)
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

// buildTemplateData assembles the dot-accessible data context shared by both
// condition matching and response templating: method, path, params, query,
// headers, and the parsed request body (a JSON value with numbers as
// json.Number, a form-urlencoded map[string]string of first values, or nil).
// Because matching and templating read this single parsed body, {{.body.field}}
// renders exactly the value a condition matches. The request body read is
// bounded by maxMockBodyBytes; an oversized body returns errMockBodyTooLarge so
// the caller can respond with a 413.
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
		bodyVal = parseRequestBody(raw, r.Header.Get("Content-Type"))
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

// parseRequestBody decodes a request body for the template/condition data
// context, returning a JSON value for JSON content, a form-urlencoded
// map[string]string (first value per key) for form content, or nil otherwise.
// JSON is tried first so a request advertising JSON is never misread as a form.
//
// JSON numbers are decoded as json.Number (via UseNumber) so a body like
// {"id":1000000} stringifies to its exact source text ("1000000") rather than
// float64's "1e+06"; this keeps condition matching and {{.body.field}}
// templating consistent and exact. Form bodies collapse to first-value strings
// so {{.body.username}} renders "admin" (not "[admin]") and matches the same
// value condition matching reads.
func parseRequestBody(raw []byte, contentType string) any {
	if v := parseJSONBodyForRoutes(raw, contentType); v != nil {
		return v
	}
	return parseFormBodyFirstValues(raw, contentType)
}

// parseJSONBodyForRoutes decodes a JSON body using a decoder with UseNumber so
// numeric values become json.Number (exact source text) instead of float64. It
// returns nil for non-JSON content or a parse failure.
func parseJSONBodyForRoutes(raw []byte, contentType string) any {
	if len(raw) == 0 || !strings.Contains(contentType, "json") {
		return nil
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	var parsed any
	if err := dec.Decode(&parsed); err != nil {
		return nil
	}
	return parsed
}

// parseFormBodyFirstValues parses a form-urlencoded body into a
// map[string]string of first values per key, or nil for non-form content or a
// parse failure. Representing the form as first-value strings (rather than
// url.Values) makes templating ({{.body.k}} -> "v") and condition matching
// (body.k: v) read the identical value.
func parseFormBodyFirstValues(raw []byte, contentType string) any {
	if len(raw) == 0 || !strings.Contains(contentType, "form-urlencoded") {
		return nil
	}
	values, err := url.ParseQuery(string(raw))
	if err != nil {
		return nil
	}
	out := make(map[string]string, len(values))
	for k, vs := range values {
		if len(vs) > 0 {
			out[k] = vs[0]
		}
	}
	return out
}

// routeFuncMap returns the template helper functions available in response
// bodies. Generators use math/rand/v2 (acceptable for mock data; not security
// sensitive). uuid wraps uuidV4 so a RNG error surfaces as a template error
// (which the handler turns into a 500) rather than crashing.
//
// seq is the owning route's private counter, captured by the {{seq}} closure so
// the helper increments that specific route's sequence. Because a hot-reload
// rebuilds every compiledRoute (and thus allocates a fresh counter), {{seq}}
// naturally restarts at 1 after a reload.
func routeFuncMap(seq *atomic.Uint64) template.FuncMap {
	return template.FuncMap{
		"uuid": func() (string, error) {
			u, err := uuidV4()
			if err != nil {
				return "", fmt.Errorf("uuid: %w", err)
			}
			return u, nil
		},
		// now formats the current UTC time. Called with no args it returns RFC3339
		// (backward-compatible); an optional first argument is a Go reference layout
		// (e.g. "2006-01-02") used instead. Extra args are ignored.
		"now": func(layout ...string) string {
			t := time.Now().UTC()
			if len(layout) > 0 && layout[0] != "" {
				return t.Format(layout[0])
			}
			return t.Format(time.RFC3339)
		},
		"timestamp": func() int64 { return time.Now().Unix() },
		"random": func(low, high int) (int, error) {
			if high <= low {
				return 0, fmt.Errorf("random: high (%d) must be greater than low (%d)", high, low)
			}
			return low + mathrand.IntN(high-low), nil
		},
		// randomFloat returns a float64 in [min, max). min and max may be given in
		// either order (they are swapped if min > max); equal bounds return that
		// value.
		"randomFloat": func(minVal, maxVal float64) float64 {
			if minVal > maxVal {
				minVal, maxVal = maxVal, minVal
			}
			return minVal + mathrand.Float64()*(maxVal-minVal)
		},
		// randomChoice returns one of its arguments at random. It errors when given
		// no arguments so an empty {{randomChoice}} surfaces as a 500 rather than
		// silently rendering "".
		"randomChoice": func(choices ...string) (string, error) {
			if len(choices) == 0 {
				return "", errors.New("randomChoice: at least one argument is required")
			}
			return choices[mathrand.IntN(len(choices))], nil
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
		// lorem returns n space-separated lorem-ipsum words drawn at random from a
		// small static pool. n<=0 yields an empty string.
		"lorem": func(n int) string {
			if n <= 0 {
				return ""
			}
			words := make([]string, n)
			for i := range words {
				words[i] = loremWords[mathrand.IntN(len(loremWords))]
			}
			return strings.Join(words, " ")
		},
		// seq returns this route's next sequence value, starting at 1 and
		// incrementing by 1 per call. It is concurrency-safe (atomic) and private to
		// the route, and resets to 1 on hot-reload (a reload allocates a fresh
		// counter).
		"seq": func() uint64 { return seq.Add(1) },
		// hash returns the lowercase hex digest of text under the named algorithm:
		// "sha256", "sha1", or "md5". An unknown algorithm is an error (a 500 at
		// request time). md5/sha1 are provided only for non-security fixture data.
		"hash": hashHex,
		// faker returns a fresh map of plausible placeholder fields so a template
		// can index it as {{faker.name}}, {{faker.email}}, {{faker.phone}}, or
		// {{faker.address}}. Go's text/template evaluates {{faker.name}} as
		// "call faker, then index the result by 'name'", so this niladic
		// map-returning form (rather than dotted FuncMap keys, which are illegal)
		// is what makes the documented {{faker.name}} syntax work. Each call draws
		// fresh values, so two {{faker.name}} references render independently.
		"faker":  fakerData,
		"env":    os.Getenv,
		"base64": func(s string) string { return base64.StdEncoding.EncodeToString([]byte(s)) },
	}
}

// hashHex computes the lowercase hex digest of text under the named algorithm.
// Supported algorithms are "sha256", "sha1", and "md5"; any other name is an
// error. md5 and sha1 are offered only for generating non-security mock
// fixtures (e.g. faking an ETag or legacy checksum) and must never be relied on
// for authentication or integrity.
func hashHex(algo, text string) (string, error) {
	switch strings.ToLower(algo) {
	case "sha256":
		sum := sha256.Sum256([]byte(text))
		return hex.EncodeToString(sum[:]), nil
	case "sha1":
		sum := sha1.Sum([]byte(text)) //nolint:gosec // non-security fixture digest only.
		return hex.EncodeToString(sum[:]), nil
	case "md5":
		sum := md5.Sum([]byte(text)) //nolint:gosec // non-security fixture digest only.
		return hex.EncodeToString(sum[:]), nil
	default:
		return "", fmt.Errorf("hash: unknown algorithm %q (want %q, %q, or %q)", algo, "sha256", "sha1", "md5")
	}
}

// fakerData builds one record of plausible placeholder identity fields, drawing
// each component at random from the static faker pools. It returns a fresh map
// per call so {{faker.name}} and a later {{faker.email}} render independently.
// Returning a map (a niladic function) is what lets the documented dotted
// {{faker.name}} syntax evaluate under Go's text/template (which treats the form
// as "call faker, then index by name").
func fakerData() map[string]string {
	first := fakerFirstNames[mathrand.IntN(len(fakerFirstNames))]
	last := fakerLastNames[mathrand.IntN(len(fakerLastNames))]
	name := first + " " + last
	email := fmt.Sprintf("%s.%s@%s",
		strings.ToLower(first), strings.ToLower(last),
		fakerEmailDomains[mathrand.IntN(len(fakerEmailDomains))])
	// US-style 10-digit number: (NXX) NXX-XXXX with N in 2-9.
	phone := fmt.Sprintf("(%d%d%d) %d%d%d-%04d",
		2+mathrand.IntN(8), mathrand.IntN(10), mathrand.IntN(10),
		2+mathrand.IntN(8), mathrand.IntN(10), mathrand.IntN(10),
		mathrand.IntN(10000))
	address := fmt.Sprintf("%d %s %s, %s",
		1+mathrand.IntN(9999),
		fakerStreets[mathrand.IntN(len(fakerStreets))],
		fakerStreetSuffixes[mathrand.IntN(len(fakerStreetSuffixes))],
		fakerCities[mathrand.IntN(len(fakerCities))])
	return map[string]string{
		"name":    name,
		"email":   email,
		"phone":   phone,
		"address": address,
	}
}
