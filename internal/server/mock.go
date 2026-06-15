package server

import (
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	mathrand "math/rand/v2"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// maxMockDelay caps the delay derived from /delay/{n} paths.
const maxMockDelay = 10 * time.Second

// maxMockBytes caps the number of bytes returned by /bytes/{n}.
const maxMockBytes = 102400

// maxMockBodyBytes caps the number of request body bytes read by the
// body-reflecting endpoints (/post, /put, /patch, /delete, /anything). Bodies
// larger than this yield a 413, preventing an unbounded read from exhausting
// memory.
const maxMockBodyBytes = 1 << 20 // 1MB

// MockConfig configures the behavior of the mock handler returned by
// NewMockHandler. It controls whether the built-in httpbin-style endpoints
// are registered, an optional path prefix for them, and global latency and
// chaos (random failure) behavior applied to every built-in response.
type MockConfig struct {
	// Builtin enables registration of the built-in httpbin-style endpoints.
	// When false, NewMockHandler returns a handler that serves nothing (all
	// requests yield 404), leaving room for custom routes in a later layer.
	Builtin bool

	// Prefix mounts the built-in endpoints under this path prefix (e.g.
	// "/_test" exposes the /get endpoint at /_test/get). An empty prefix mounts
	// them at the root. The prefix is normalized to start with "/" and not end
	// with "/".
	Prefix string

	// Latency is a fixed duration to sleep before responding to any built-in
	// request. Zero means no latency.
	Latency time.Duration

	// LatencyJitter, when positive, adds a random duration in [0, LatencyJitter)
	// to Latency before responding.
	LatencyJitter time.Duration

	// FailRate is the probability, expressed as a percentage in [0, 100], that
	// any given request fails immediately with FailStatus instead of running
	// the normal handler. Zero disables random failures.
	FailRate float64

	// FailStatus is the HTTP status code returned for randomly-injected
	// failures. If zero, http.StatusInternalServerError (500) is used.
	FailStatus int

	// Metrics, when non-nil, records per-command mock counters (route matches,
	// template renders/errors, fail injections, fallback hits). It is nil when
	// metrics are disabled; MockMetricsRecorder methods are also nil-safe so
	// recording is always side-effect-free w.r.t. the response.
	Metrics MockMetricsRecorder
}

// MockMetricsRecorder records the mock command's per-command counters. It is
// satisfied by *metrics.Collector; the server package depends on this narrow
// interface rather than the metrics package so the handlers stay decoupled. All
// methods must be safe to call on a nil receiver (the concrete collector's are).
type MockMetricsRecorder interface {
	RecordMockRouteMatch(custom bool)
	RecordMockTemplateRender()
	RecordMockTemplateError()
	RecordMockReload()
	RecordMockFailInjection()
	RecordMockFallback(kind string)
}

// NewMockHandler returns an http.Handler exposing built-in httpbin-style
// endpoints (when cfg.Builtin is true), wrapped in middleware that applies the
// configured global latency and random-failure behavior. It is safe for
// concurrent use; the configuration is treated as immutable after construction.
//
// MockConfig is passed by value (matching the sibling NewEchoHandler /
// NewReverseProxy / NewFileServer constructors); this runs once at startup,
// not on a hot path.
func NewMockHandler(cfg MockConfig) http.Handler {
	if cfg.FailStatus == 0 {
		cfg.FailStatus = http.StatusInternalServerError
	}

	mux := http.NewServeMux()
	if cfg.Builtin {
		registerBuiltins(mux, NormalizePrefix(cfg.Prefix))
	}

	// In the builtins-only handler, record a built-in route match whenever a
	// request actually matches a registered endpoint (mux.Handler reports a
	// non-empty pattern). The routed handler records matches in its own dispatch.
	var handler http.Handler = mux
	if cfg.Metrics != nil && cfg.Builtin {
		handler = recordBuiltinMatches(mux, cfg.Metrics)
	}

	return withLatencyAndFailures(handler, cfg)
}

// recordBuiltinMatches wraps a built-ins ServeMux so a built-in route match is
// recorded whenever the mux matches a registered endpoint for the request. It
// uses mux.Handler to detect a match without serving twice: an empty returned
// pattern means no built-in matched (the mux would serve its own 404), so no
// match is recorded.
func recordBuiltinMatches(mux *http.ServeMux, rec MockMetricsRecorder) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, pattern := mux.Handler(r); pattern != "" {
			rec.RecordMockRouteMatch(false)
		}
		mux.ServeHTTP(w, r)
	})
}

// NormalizePrefix cleans a user-supplied prefix so it starts with "/" and does
// not end with "/" (except for the empty/root case, which yields ""). It is
// exported so the CLI can validate a prefix using the exact normalization that
// route registration applies.
func NormalizePrefix(prefix string) string {
	prefix = strings.TrimSpace(prefix)
	if prefix == "" || prefix == "/" {
		return ""
	}
	if !strings.HasPrefix(prefix, "/") {
		prefix = "/" + prefix
	}
	return strings.TrimRight(prefix, "/")
}

// WithLatencyAndFailures wraps next with the global latency (fixed + jitter) and
// random-failure behavior described by cfg, exactly as NewMockHandler applies it
// to the built-ins. It is exported so the routed mock handler can share the same
// global chaos behavior. FailStatus defaults to 500 when zero.
func WithLatencyAndFailures(next http.Handler, cfg MockConfig) http.Handler {
	if cfg.FailStatus == 0 {
		cfg.FailStatus = http.StatusInternalServerError
	}
	return withLatencyAndFailures(next, cfg)
}

// withLatencyAndFailures wraps next so that every request first incurs the
// configured latency (fixed + jitter, context-aware) and may be short-circuited
// with a random failure response based on cfg.FailRate.
//
// By design, latency and fail-rate apply to ALL mock requests, including
// unmatched paths that would otherwise 404. This is intentional chaos-testing
// behavior: a client exercising an unknown path still sees realistic latency
// and failures.
func withLatencyAndFailures(next http.Handler, cfg MockConfig) http.Handler {
	failStatus := cfg.FailStatus
	if failStatus == 0 {
		failStatus = http.StatusInternalServerError
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		applyLatencyAndFailures(w, r, latencyFailSettings{
			latency:    cfg.Latency,
			jitter:     cfg.LatencyJitter,
			failRate:   cfg.FailRate,
			failStatus: failStatus,
			metrics:    cfg.Metrics,
		}, next.ServeHTTP)
	})
}

// latencyFailSettings is the minimal set of fields applyLatencyAndFailures needs.
// Both MockConfig (built-ins-only path) and RouteSettings (routed path) project
// onto it.
type latencyFailSettings struct {
	latency    time.Duration
	jitter     time.Duration
	failRate   float64
	failStatus int
	metrics    MockMetricsRecorder // nil when metrics are disabled
}

// latencyFail adapts a RouteSettings value into latencyFailSettings, defaulting
// failStatus to 500 when unset. rec records the fail-rate injection; it is nil
// when metrics are disabled.
func (rs RouteSettings) latencyFail(rec MockMetricsRecorder) latencyFailSettings {
	fs := rs.FailStatus
	if fs == 0 {
		fs = http.StatusInternalServerError
	}
	return latencyFailSettings{
		latency:    rs.Latency,
		jitter:     rs.LatencyJitter,
		failRate:   rs.FailRate,
		failStatus: fs,
		metrics:    rec,
	}
}

// applyLatencyAndFailures applies the random-failure short-circuit and the
// context-aware latency described by s, then invokes next when the request was
// neither failed nor canceled. It is the shared core of both the built-ins-only
// (MockConfig) and routed (RouteSettings) handlers, so a routed handler can read
// effective latency/fail values from the live store snapshot per request.
func applyLatencyAndFailures(w http.ResponseWriter, r *http.Request, s latencyFailSettings, next http.HandlerFunc) {
	// Random failure injection: with probability failRate/100 respond with
	// failStatus immediately. mathrand.Float64 returns [0.0, 1.0).
	if s.failRate > 0 && mathrand.Float64()*100 < s.failRate {
		if s.metrics != nil {
			s.metrics.RecordMockFailInjection()
		}
		w.WriteHeader(s.failStatus)
		return
	}

	// Apply latency (fixed + jitter), honoring request cancellation.
	delay := s.latency
	if s.jitter > 0 {
		delay += time.Duration(mathrand.Int64N(int64(s.jitter)))
	}
	if delay > 0 {
		t := time.NewTimer(delay)
		defer t.Stop()
		select {
		case <-r.Context().Done():
			return
		case <-t.C:
		}
	}

	next(w, r)
}

// registerBuiltins registers the built-in httpbin-style endpoints on mux under
// the given (already-normalized) prefix.
func registerBuiltins(mux *http.ServeMux, prefix string) {
	p := func(path string) string { return prefix + path }

	// HTTP methods: return the httpbin-style request description.
	mux.HandleFunc("GET "+p("/get"), handleReflect)
	mux.HandleFunc("POST "+p("/post"), handleReflect)
	mux.HandleFunc("PUT "+p("/put"), handleReflect)
	mux.HandleFunc("PATCH "+p("/patch"), handleReflect)
	mux.HandleFunc("DELETE "+p("/delete"), handleReflect)

	// Anything: any method, with or without a trailing path.
	mux.HandleFunc(p("/anything"), handleReflect)
	mux.HandleFunc(p("/anything/"), handleReflect)

	// Request inspection.
	mux.HandleFunc("GET "+p("/headers"), handleHeaders)
	mux.HandleFunc("GET "+p("/ip"), handleIP)
	mux.HandleFunc("GET "+p("/user-agent"), handleUserAgent)
	mux.HandleFunc("GET "+p("/uuid"), handleUUID)

	// Status codes (any method).
	mux.HandleFunc(p("/status/{code}"), handleStatus)

	// Dynamic data (any method for delay, GET for bytes).
	mux.HandleFunc(p("/delay/{n}"), handleDelay)
	mux.HandleFunc("GET "+p("/bytes/{n}"), handleBytes)

	// Response formats.
	mux.HandleFunc("GET "+p("/json"), handleJSON)
	mux.HandleFunc("GET "+p("/html"), handleHTML)
	mux.HandleFunc("GET "+p("/xml"), handleXML)
}

// errMockBodyTooLarge signals that a request body exceeded maxMockBodyBytes and
// the handler should respond with 413.
var errMockBodyTooLarge = errors.New("mock: request body too large")

// reflectResponse builds the common httpbin-style request description shared by
// /get, /post, /anything, etc. For body-bearing methods it also includes the
// raw body, parsed JSON, and parsed form. The body read is bounded by
// maxMockBodyBytes via http.MaxBytesReader; a body exceeding the limit returns
// errMockBodyTooLarge so the caller can respond with a 413. The response writer
// is required by MaxBytesReader to close the connection on overflow.
func reflectResponse(w http.ResponseWriter, r *http.Request) (map[string]any, error) {
	resp := map[string]any{
		"args":    map[string][]string(r.URL.Query()),
		"headers": map[string][]string(r.Header),
		"origin":  clientIP(r),
		"url":     requestURL(r),
		"method":  r.Method,
	}

	if methodHasBody(r.Method) && r.Body != nil {
		body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxMockBodyBytes))
		if err != nil {
			var maxErr *http.MaxBytesError
			if errors.As(err, &maxErr) {
				return nil, errMockBodyTooLarge
			}
			// Other read errors: continue with whatever was read rather than
			// failing the response.
		}
		resp["data"] = string(body)
		resp["json"] = parseJSONBody(body, r.Header.Get("Content-Type"))
		resp["form"] = parseFormBody(body, r.Header.Get("Content-Type"))
	}

	return resp, nil
}

// handleReflect serves the httpbin-style request description.
func handleReflect(w http.ResponseWriter, r *http.Request) {
	resp, err := reflectResponse(w, r)
	if err != nil {
		writeBodyTooLarge(w)
		return
	}
	writeJSON(w, resp)
}

// writeBodyTooLarge responds with a 413 and a small JSON error payload.
func writeBodyTooLarge(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusRequestEntityTooLarge)
	_, _ = w.Write([]byte(`{"error":"request body too large"}`))
}

// handleHeaders returns the request headers.
func handleHeaders(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, map[string]any{
		"headers": map[string][]string(r.Header),
	})
}

// handleIP returns the client's origin IP address.
func handleIP(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, map[string]any{"origin": clientIP(r)})
}

// handleUserAgent returns the request's User-Agent header.
func handleUserAgent(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, map[string]any{"user-agent": r.UserAgent()})
}

// handleUUID returns a freshly generated RFC 4122 version 4 UUID. If the system
// RNG fails it responds with a 500 rather than emitting a malformed UUID.
func handleUUID(w http.ResponseWriter, _ *http.Request) {
	u, err := uuidV4()
	if err != nil {
		http.Error(w, `{"error":"failed to generate uuid"}`, http.StatusInternalServerError)
		return
	}
	writeJSON(w, map[string]any{"uuid": u})
}

// handleStatus responds with the status code(s) named in the {code} path
// segment. A single code is returned directly; a comma-separated list selects
// one at random. Each code must be in [200, 599]; otherwise a 400 is returned.
func handleStatus(w http.ResponseWriter, r *http.Request) {
	code, ok := statusFromCodes(r.PathValue("code"))
	if !ok {
		http.Error(w, "invalid status code", http.StatusBadRequest)
		return
	}
	w.WriteHeader(code)
}

// handleDelay sleeps for the duration named in the {n} path segment (capped at
// maxMockDelay), honoring request cancellation, then returns the httpbin-style
// request description. Invalid durations yield a 400.
func handleDelay(w http.ResponseWriter, r *http.Request) {
	delay, ok := mockDelayFromValue(r.PathValue("n"))
	if !ok {
		http.Error(w, "invalid delay", http.StatusBadRequest)
		return
	}
	if delay > 0 {
		t := time.NewTimer(delay)
		defer t.Stop()
		select {
		case <-r.Context().Done():
			return
		case <-t.C:
		}
	}
	resp, rErr := reflectResponse(w, r)
	if rErr != nil {
		writeBodyTooLarge(w)
		return
	}
	writeJSON(w, resp)
}

// handleBytes returns n random bytes (capped at maxMockBytes) as
// application/octet-stream with a correct Content-Length. Invalid counts yield
// a 400.
func handleBytes(w http.ResponseWriter, r *http.Request) {
	n, err := strconv.Atoi(r.PathValue("n"))
	if err != nil || n < 0 {
		http.Error(w, "invalid byte count", http.StatusBadRequest)
		return
	}
	if n > maxMockBytes {
		n = maxMockBytes
	}

	buf := make([]byte, n)
	// crypto/rand.Read never returns a short read without an error.
	if _, rErr := rand.Read(buf); rErr != nil {
		http.Error(w, "failed to generate bytes", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Length", strconv.Itoa(n))
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(buf)
}

// handleJSON returns a small sample JSON document.
func handleJSON(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, map[string]any{
		"slideshow": map[string]any{
			"title":  "Sample Slide Show",
			"author": "radix",
			"slides": []map[string]any{
				{"title": "Wake up to radix!", "type": "all"},
				{"title": "Overview", "type": "all"},
			},
		},
	})
}

// handleHTML returns a small valid HTML page.
func handleHTML(w http.ResponseWriter, _ *http.Request) {
	const page = `<!DOCTYPE html>
<html lang="en">
  <head><meta charset="utf-8"><title>radix mock</title></head>
  <body>
    <h1>radix mock</h1>
    <p>This is a sample HTML response.</p>
  </body>
</html>
`
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = io.WriteString(w, page)
}

// handleXML returns a small XML document.
func handleXML(w http.ResponseWriter, _ *http.Request) {
	const doc = `<?xml version="1.0" encoding="UTF-8"?>
<slideshow title="Sample Slide Show" author="radix">
  <slide type="all"><title>Wake up to radix!</title></slide>
  <slide type="all"><title>Overview</title></slide>
</slideshow>
`
	w.Header().Set("Content-Type", "application/xml; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = io.WriteString(w, doc)
}

// writeJSON marshals v and writes it as a 200 application/json response.
func writeJSON(w http.ResponseWriter, v any) {
	payload, err := json.Marshal(v)
	if err != nil {
		http.Error(w, "failed to encode response", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(payload)
}

// methodHasBody reports whether responses for the given method should include
// body-related fields (data/json/form).
func methodHasBody(method string) bool {
	switch method {
	case http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete:
		return true
	default:
		return false
	}
}

// clientIP extracts the client's IP address from the request's RemoteAddr,
// falling back to the raw RemoteAddr when it cannot be split.
func clientIP(r *http.Request) string {
	if host, _, err := net.SplitHostPort(r.RemoteAddr); err == nil {
		return host
	}
	return r.RemoteAddr
}

// requestURL reconstructs the full request URL, inferring the scheme from the
// presence of TLS on the connection.
func requestURL(r *http.Request) string {
	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	}
	return scheme + "://" + r.Host + r.URL.RequestURI()
}

// parseJSONBody returns the parsed JSON value for a JSON-typed body, or nil for
// non-JSON content or a parse failure.
func parseJSONBody(body []byte, contentType string) any {
	if len(body) == 0 || !strings.Contains(contentType, "json") {
		return nil
	}
	var parsed any
	if err := json.Unmarshal(body, &parsed); err != nil {
		return nil
	}
	return parsed
}

// parseFormBody returns a url.Values-style map for a form-urlencoded body, or
// nil for non-form content or a parse failure.
func parseFormBody(body []byte, contentType string) any {
	if len(body) == 0 || !strings.Contains(contentType, "form-urlencoded") {
		return nil
	}
	values, err := url.ParseQuery(string(body))
	if err != nil {
		return nil
	}
	return map[string][]string(values)
}

// statusFromCodes parses a single status code or a comma-separated list and
// returns one valid code in [200, 599]. For a list, a random element is chosen.
// It returns ok=false if the input is empty or any code is invalid. 1xx codes
// are rejected because net/http treats WriteHeader(1xx) as informational (the
// server can still finish with a 200), which is misleading for a mock.
func statusFromCodes(raw string) (int, bool) {
	parts := strings.Split(raw, ",")
	codes := make([]int, 0, len(parts))
	for _, part := range parts {
		code, err := strconv.Atoi(strings.TrimSpace(part))
		if err != nil || code < 200 || code > 599 {
			return 0, false
		}
		codes = append(codes, code)
	}
	if len(codes) == 0 {
		return 0, false
	}
	if len(codes) == 1 {
		return codes[0], true
	}
	return codes[mathrand.IntN(len(codes))], true
}

// mockDelayFromValue parses a delay from a /delay/{n} segment, where the value
// is a Go duration (e.g. "500ms") or a bare number of seconds (e.g. "2",
// "0.5"). The result is capped at maxMockDelay; negative, NaN, and Inf values
// are rejected. The float-seconds comparison happens before converting to a
// Duration so large finite values cannot overflow int64 nanoseconds.
func mockDelayFromValue(seg string) (time.Duration, bool) {
	// Go-duration form (e.g. "500ms", "1h").
	if parsed, err := time.ParseDuration(seg); err == nil {
		if parsed < 0 {
			return 0, false
		}
		if parsed > maxMockDelay {
			return maxMockDelay, true
		}
		return parsed, true
	}

	// Bare-seconds form (e.g. "2", "0.5").
	secs, err := strconv.ParseFloat(seg, 64)
	if err != nil {
		return 0, false
	}
	if math.IsNaN(secs) || math.IsInf(secs, 0) || secs < 0 {
		return 0, false
	}
	if secs >= maxMockDelay.Seconds() {
		return maxMockDelay, true
	}
	return time.Duration(secs * float64(time.Second)), true
}

// uuidV4 returns a random RFC 4122 version 4 UUID string. It propagates any
// crypto/rand read error to the caller rather than fabricating a value, so a
// malformed UUID is never emitted.
func uuidV4() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", fmt.Errorf("mock: read random for uuid: %w", err)
	}
	// Set version (4) and variant (RFC 4122) bits.
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16]), nil
}
