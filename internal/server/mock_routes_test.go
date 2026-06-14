package server

import (
	"context"
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

// newStore compiles YAML into a RoutesStore rooted at baseDir, failing the test
// on a compile error.
func newStore(t *testing.T, yamlSrc, baseDir string) *RoutesStore {
	t.Helper()
	compiled, err := CompileRoutes([]byte(yamlSrc), baseDir)
	if err != nil {
		t.Fatalf("CompileRoutes: %v", err)
	}
	return NewRoutesStore(filepath.Join(baseDir, "routes.yml"), compiled, nil)
}

// doRouted runs a request through a routed handler built from yamlSrc.
func doRouted(t *testing.T, yamlSrc string, builtin bool, req *http.Request) *httptest.ResponseRecorder {
	t.Helper()
	store := newStore(t, yamlSrc, t.TempDir())
	h := NewRoutedHandler(RoutedHandlerConfig{Store: store, Builtin: builtin})
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func TestRoutes_ExactMatch(t *testing.T) {
	const src = `
routes:
  - path: /api/health
    method: GET
    response:
      status: 200
      headers: { Content-Type: application/json }
      body: '{"status":"ok"}'
`
	rec := doRouted(t, src, false, httptest.NewRequest(http.MethodGet, "/api/health", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if got := rec.Body.String(); got != `{"status":"ok"}` {
		t.Errorf("body = %q", got)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("content-type = %q", ct)
	}
}

func TestRoutes_MethodMismatchFallsThrough(t *testing.T) {
	const src = `
routes:
  - path: /only-get
    method: GET
    response: { status: 200, body: "yes" }
`
	// POST does not match the GET-only route; with builtins off the fallback 404s.
	rec := doRouted(t, src, false, httptest.NewRequest(http.MethodPost, "/only-get", nil))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rec.Code)
	}
}

func TestRoutes_AnyMethodWhenUnset(t *testing.T) {
	const src = `
routes:
  - path: /any
    response: { status: 200, body: "ok" }
`
	for _, m := range []string{http.MethodGet, http.MethodPost, http.MethodDelete} {
		rec := doRouted(t, src, false, httptest.NewRequest(m, "/any", nil))
		if rec.Code != http.StatusOK {
			t.Errorf("method %s: status = %d, want 200", m, rec.Code)
		}
	}
}

func TestRoutes_MethodsList(t *testing.T) {
	const src = `
routes:
  - path: /multi
    methods: [GET, POST]
    response: { status: 200, body: "ok" }
`
	if rec := doRouted(t, src, false, httptest.NewRequest(http.MethodPost, "/multi", nil)); rec.Code != http.StatusOK {
		t.Errorf("POST status = %d, want 200", rec.Code)
	}
	if rec := doRouted(t, src, false, httptest.NewRequest(http.MethodPut, "/multi", nil)); rec.Code != http.StatusNotFound {
		t.Errorf("PUT status = %d, want 404", rec.Code)
	}
}

func TestRoutes_ParamExtraction(t *testing.T) {
	const src = `
routes:
  - path: /api/users/:id
    method: GET
    response: { status: 200, body: '{"id":"{{.params.id}}"}' }
`
	rec := doRouted(t, src, false, httptest.NewRequest(http.MethodGet, "/api/users/42", nil))
	if got := rec.Body.String(); got != `{"id":"42"}` {
		t.Errorf("body = %q, want id 42", got)
	}
}

func TestRoutes_ExactBeatsParam(t *testing.T) {
	// Exact /api/users/me must win over the :id param route regardless of order.
	const src = `
routes:
  - path: /api/users/:id
    method: GET
    response: { status: 200, body: "param" }
  - path: /api/users/me
    method: GET
    response: { status: 200, body: "exact" }
`
	rec := doRouted(t, src, false, httptest.NewRequest(http.MethodGet, "/api/users/me", nil))
	if got := rec.Body.String(); got != "exact" {
		t.Errorf("body = %q, want exact (exact route should outrank param)", got)
	}
}

func TestRoutes_ExactMethodBeatsAnyMethod(t *testing.T) {
	const src = `
routes:
  - path: /res
    response: { status: 200, body: "any" }
  - path: /res
    method: GET
    response: { status: 200, body: "get" }
`
	rec := doRouted(t, src, false, httptest.NewRequest(http.MethodGet, "/res", nil))
	if got := rec.Body.String(); got != "get" {
		t.Errorf("body = %q, want get (method-specific exact should win)", got)
	}
}

func TestRoutes_Regex(t *testing.T) {
	const src = `
routes:
  - path: "regex:^/api/v[0-9]+/x$"
    response: { status: 200, body: "ok" }
`
	if rec := doRouted(t, src, false, httptest.NewRequest(http.MethodGet, "/api/v2/x", nil)); rec.Code != http.StatusOK {
		t.Errorf("matching path status = %d, want 200", rec.Code)
	}
	if rec := doRouted(t, src, false, httptest.NewRequest(http.MethodGet, "/api/vX/x", nil)); rec.Code != http.StatusNotFound {
		t.Errorf("non-matching path status = %d, want 404", rec.Code)
	}
}

func TestRoutes_Glob(t *testing.T) {
	const src = `
routes:
  - path: /assets/*
    response: { status: 200, body: "asset" }
`
	for _, p := range []string{"/assets", "/assets/", "/assets/img/logo.png"} {
		rec := doRouted(t, src, false, httptest.NewRequest(http.MethodGet, p, nil))
		if rec.Code != http.StatusOK || rec.Body.String() != "asset" {
			t.Errorf("path %q: status=%d body=%q, want 200 asset", p, rec.Code, rec.Body.String())
		}
	}
	if rec := doRouted(t, src, false, httptest.NewRequest(http.MethodGet, "/assetsX", nil)); rec.Code != http.StatusNotFound {
		t.Errorf("/assetsX status = %d, want 404 (glob must not match prefix without /)", rec.Code)
	}
}

func TestRoutes_PriorityRegexBeforeGlob(t *testing.T) {
	const src = `
routes:
  - path: /a/*
    response: { status: 200, body: "glob" }
  - path: "regex:^/a/special$"
    response: { status: 200, body: "regex" }
`
	rec := doRouted(t, src, false, httptest.NewRequest(http.MethodGet, "/a/special", nil))
	if got := rec.Body.String(); got != "regex" {
		t.Errorf("body = %q, want regex (regex outranks glob)", got)
	}
}

func TestRoutes_CustomShadowsBuiltin(t *testing.T) {
	const src = `
routes:
  - path: /get
    method: GET
    response: { status: 200, body: "custom-get" }
`
	rec := doRouted(t, src, true, httptest.NewRequest(http.MethodGet, "/get", nil))
	if got := rec.Body.String(); got != "custom-get" {
		t.Errorf("body = %q, want custom-get (custom route should shadow built-in)", got)
	}
}

func TestRoutes_FallThroughToBuiltin(t *testing.T) {
	const src = `
routes:
  - path: /custom
    response: { status: 200, body: "custom" }
`
	// /headers is a built-in; no custom route matches, so the built-in serves it.
	rec := doRouted(t, src, true, httptest.NewRequest(http.MethodGet, "/headers", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 from built-in /headers", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "headers") {
		t.Errorf("body = %q, want built-in /headers payload", rec.Body.String())
	}
}

func TestRoutes_Fallback404WhenBuiltinsDisabled(t *testing.T) {
	const src = `routes: []`
	rec := doRouted(t, src, false, httptest.NewRequest(http.MethodGet, "/nope", nil))
	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", rec.Code)
	}
}

func TestRoutes_FallbackProxy(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTeapot)
		_, _ = w.Write([]byte("from-backend:" + r.URL.Path))
	}))
	defer backend.Close()

	src := "settings:\n  fallback:\n    type: proxy\n    proxy_target: " + backend.URL + "\nroutes: []\n"
	rec := doRouted(t, src, false, httptest.NewRequest(http.MethodGet, "/unmatched", nil))
	if rec.Code != http.StatusTeapot {
		t.Fatalf("status = %d, want 418 from backend", rec.Code)
	}
	if got := rec.Body.String(); !strings.Contains(got, "from-backend:/unmatched") {
		t.Errorf("body = %q, want backend response", got)
	}
}

func TestRoutes_TemplateDataAccess(t *testing.T) {
	const src = `
routes:
  - path: /echo/:id
    method: POST
    response:
      status: 200
      body: 'm={{.method}} p={{.path}} id={{.params.id}} q={{.query.q}} h={{index .headers "X-Token"}} name={{.body.name}}'
`
	req := httptest.NewRequest(http.MethodPost, "/echo/7?q=hi", strings.NewReader(`{"name":"alice"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Token", "abc")
	rec := doRouted(t, src, false, req)
	want := "m=POST p=/echo/7 id=7 q=hi h=abc name=alice"
	if got := rec.Body.String(); got != want {
		t.Errorf("body = %q, want %q", got, want)
	}
}

func TestRoutes_HeaderDotAccess(t *testing.T) {
	// Headers without hyphens are reachable via dot access (canonical key).
	const src = `
routes:
  - path: /auth
    method: GET
    response: { status: 200, body: 'tok={{.headers.Authorization}}' }
`
	req := httptest.NewRequest(http.MethodGet, "/auth", nil)
	req.Header.Set("Authorization", "Bearer xyz")
	rec := doRouted(t, src, false, req)
	if got := rec.Body.String(); got != "tok=Bearer xyz" {
		t.Errorf("body = %q, want tok=Bearer xyz", got)
	}
}

func TestRoutes_TemplateFuncs(t *testing.T) {
	t.Setenv("RADIX_TEST_VAR", "envval")
	const src = `
routes:
  - path: /uuid
    response: { status: 200, body: '{{uuid}}' }
  - path: /now
    response: { status: 200, body: '{{now}}' }
  - path: /ts
    response: { status: 200, body: '{{timestamp}}' }
  - path: /rand
    response: { status: 200, body: '{{random 5 6}}' }
  - path: /randstr
    response: { status: 200, body: '{{randomString 12}}' }
  - path: /b64
    response: { status: 200, body: '{{base64 "text"}}' }
  - path: /envv
    response: { status: 200, body: '{{env "RADIX_TEST_VAR"}}' }
`
	get := func(p string) string {
		rec := doRouted(t, src, false, httptest.NewRequest(http.MethodGet, p, nil))
		if rec.Code != http.StatusOK {
			t.Fatalf("%s: status %d", p, rec.Code)
		}
		return rec.Body.String()
	}

	if !uuidV4Re.MatchString(get("/uuid")) {
		t.Errorf("uuid = %q, not a v4 UUID", get("/uuid"))
	}
	if _, err := time.Parse(time.RFC3339, get("/now")); err != nil {
		t.Errorf("now = %q, not RFC3339: %v", get("/now"), err)
	}
	if _, err := strconv.ParseInt(get("/ts"), 10, 64); err != nil {
		t.Errorf("timestamp = %q, not an int: %v", get("/ts"), err)
	}
	if r := get("/rand"); r != "5" {
		t.Errorf("random 5 6 = %q, want 5", r)
	}
	if rs := get("/randstr"); len(rs) != 12 {
		t.Errorf("randomString 12 length = %d, want 12", len(rs))
	}
	if b := get("/b64"); b != base64.StdEncoding.EncodeToString([]byte("text")) {
		t.Errorf("base64 = %q", b)
	}
	if e := get("/envv"); e != "envval" {
		t.Errorf("env = %q, want envval", e)
	}
}

func TestRoutes_MalformedTemplateInlineRejectedAtLoad(t *testing.T) {
	const src = `
routes:
  - path: /bad
    response: { status: 200, body: '{{.params.id' }
`
	if _, err := CompileRoutes([]byte(src), t.TempDir()); err == nil {
		t.Fatal("expected compile error for malformed inline template, got nil")
	}
}

func TestRoutes_TemplateExecErrorReturns500(t *testing.T) {
	// A function that errors at execution time (random with max<=min) must yield
	// a 500 and not crash the handler.
	const src = `
routes:
  - path: /boom
    response: { status: 200, body: '{{random 5 5}}' }
`
	rec := doRouted(t, src, false, httptest.NewRequest(http.MethodGet, "/boom", nil))
	if rec.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500", rec.Code)
	}
}

func TestRoutes_FileBody(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "products.json"), []byte(`{"id":"{{.params.id}}"}`), 0o600); err != nil {
		t.Fatalf("write file: %v", err)
	}
	const src = `
routes:
  - path: /p/:id
    method: GET
    response:
      file: ./products.json
      headers: { Content-Type: application/json }
`
	store := newStore(t, src, dir)
	h := NewRoutedHandler(RoutedHandlerConfig{Store: store, Builtin: false})
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/p/99", nil))
	if got := rec.Body.String(); got != `{"id":"99"}` {
		t.Errorf("file body = %q, want templated {\"id\":\"99\"}", got)
	}
}

func TestRoutes_FilePathTraversalRejected(t *testing.T) {
	dir := t.TempDir()
	// Create a secret file one level above the routes dir.
	parent := filepath.Dir(dir)
	secret := filepath.Join(parent, "secret.txt")
	if err := os.WriteFile(secret, []byte("TOPSECRET"), 0o600); err != nil {
		t.Fatalf("write secret: %v", err)
	}
	defer func() { _ = os.Remove(secret) }()

	src := "routes:\n  - path: /leak\n    response:\n      file: ../" + filepath.Base(secret) + "\n"
	store := newStore(t, src, dir)
	h := NewRoutedHandler(RoutedHandlerConfig{Store: store, Builtin: false})
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/leak", nil))
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500 for traversal", rec.Code)
	}
	if strings.Contains(rec.Body.String(), "TOPSECRET") {
		t.Fatal("path traversal leaked file contents")
	}
}

func TestRoutes_FileSymlinkEscapeRejected(t *testing.T) {
	dir := t.TempDir()
	// A secret file outside the routes dir, and a symlink INSIDE the routes dir
	// pointing at it. A purely lexical guard would clean to a path inside dir and
	// then ReadFile would follow the symlink, leaking the external content.
	parent := filepath.Dir(dir)
	secret := filepath.Join(parent, "symlink-secret.txt")
	if err := os.WriteFile(secret, []byte("TOPSECRET"), 0o600); err != nil {
		t.Fatalf("write secret: %v", err)
	}
	defer func() { _ = os.Remove(secret) }()

	link := filepath.Join(dir, "leak.json")
	if err := os.Symlink(secret, link); err != nil {
		t.Skipf("symlinks unsupported on this platform: %v", err)
	}

	const src = `
routes:
  - path: /leak
    response:
      file: ./leak.json
`
	store := newStore(t, src, dir)
	h := NewRoutedHandler(RoutedHandlerConfig{Store: store, Builtin: false})
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/leak", nil))
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500 for symlink escape", rec.Code)
	}
	if strings.Contains(rec.Body.String(), "TOPSECRET") {
		t.Fatal("symlink escape leaked external file contents")
	}
}

func TestRoutes_FileSymlinkWithinBaseAllowed(t *testing.T) {
	// A symlink that stays within the routes dir must still resolve and serve.
	dir := t.TempDir()
	target := filepath.Join(dir, "real.json")
	if err := os.WriteFile(target, []byte(`{"ok":true}`), 0o600); err != nil {
		t.Fatalf("write target: %v", err)
	}
	link := filepath.Join(dir, "alias.json")
	if err := os.Symlink(target, link); err != nil {
		t.Skipf("symlinks unsupported on this platform: %v", err)
	}

	const src = `
routes:
  - path: /aliased
    response:
      file: ./alias.json
`
	store := newStore(t, src, dir)
	h := NewRoutedHandler(RoutedHandlerConfig{Store: store, Builtin: false})
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/aliased", nil))
	if rec.Code != http.StatusOK || rec.Body.String() != `{"ok":true}` {
		t.Errorf("status=%d body=%q, want 200 {\"ok\":true} for in-base symlink", rec.Code, rec.Body.String())
	}
}

func TestRoutes_PerRouteDelayApplied(t *testing.T) {
	const src = `
routes:
  - path: /slow
    delay: 60ms
    response: { status: 200, body: "ok" }
`
	store := newStore(t, src, t.TempDir())
	h := NewRoutedHandler(RoutedHandlerConfig{Store: store, Builtin: false})
	rec := httptest.NewRecorder()
	start := time.Now()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/slow", nil))
	if elapsed := time.Since(start); elapsed < 50*time.Millisecond {
		t.Errorf("elapsed = %v, want >= 50ms (per-route delay)", elapsed)
	}
}

func TestRoutes_GlobalLatencyAndFailWithRoutes(t *testing.T) {
	store := newStore(t, "routes:\n  - path: /ok\n    response: { status: 200, body: \"ok\" }\n", t.TempDir())
	routed := NewRoutedHandler(RoutedHandlerConfig{Store: store, Builtin: false})

	// Global latency applied around the routed handler.
	h := WithLatencyAndFailures(routed, MockConfig{Latency: 60 * time.Millisecond})
	rec := httptest.NewRecorder()
	start := time.Now()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/ok", nil))
	if elapsed := time.Since(start); elapsed < 50*time.Millisecond {
		t.Errorf("elapsed = %v, want >= 50ms (global latency)", elapsed)
	}

	// 100% fail-rate short-circuits before the route runs.
	hf := WithLatencyAndFailures(routed, MockConfig{FailRate: 100, FailStatus: 503})
	recf := httptest.NewRecorder()
	hf.ServeHTTP(recf, httptest.NewRequest(http.MethodGet, "/ok", nil))
	if recf.Code != http.StatusServiceUnavailable {
		t.Errorf("status = %d, want 503 (global fail-rate)", recf.Code)
	}
}

func TestRoutes_BodyTooLarge413(t *testing.T) {
	const src = `
routes:
  - path: /big
    method: POST
    response: { status: 200, body: '{{.body.x}}' }
`
	big := strings.Repeat("a", maxMockBodyBytes+10)
	req := httptest.NewRequest(http.MethodPost, "/big", strings.NewReader(`{"x":"`+big+`"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := doRouted(t, src, false, req)
	if rec.Code != http.StatusRequestEntityTooLarge {
		t.Errorf("status = %d, want 413", rec.Code)
	}
}

func TestLoadRoutes_Errors(t *testing.T) {
	dir := t.TempDir()
	tests := []struct {
		name    string
		write   func() string // returns path to pass to LoadRoutes
		wantSub string
	}{
		{
			name:    "missing file",
			write:   func() string { return filepath.Join(dir, "does-not-exist.yml") },
			wantSub: "read",
		},
		{
			name: "bad yaml",
			write: func() string {
				p := filepath.Join(dir, "bad.yml")
				_ = os.WriteFile(p, []byte("routes: [::::"), 0o600)
				return p
			},
			wantSub: "parse YAML",
		},
		{
			name: "invalid fallback type",
			write: func() string {
				p := filepath.Join(dir, "fb.yml")
				_ = os.WriteFile(p, []byte("settings:\n  fallback:\n    type: bogus\n"), 0o600)
				return p
			},
			wantSub: "invalid fallback.type",
		},
		{
			name: "proxy without target",
			write: func() string {
				p := filepath.Join(dir, "px.yml")
				_ = os.WriteFile(p, []byte("settings:\n  fallback:\n    type: proxy\n"), 0o600)
				return p
			},
			wantSub: "proxy_target is required",
		},
		{
			name: "invalid proxy target",
			write: func() string {
				p := filepath.Join(dir, "pxbad.yml")
				_ = os.WriteFile(p, []byte("settings:\n  fallback:\n    type: proxy\n    proxy_target: \"not a url\"\n"), 0o600)
				return p
			},
			wantSub: "must be an http(s) URL",
		},
		{
			name: "bad regex route",
			write: func() string {
				p := filepath.Join(dir, "rx.yml")
				_ = os.WriteFile(p, []byte("routes:\n  - path: \"regex:(\"\n    response: { status: 200 }\n"), 0o600)
				return p
			},
			wantSub: "invalid regex",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := LoadRoutes(tt.write())
			if err == nil {
				t.Fatalf("expected error, got nil")
			}
			if !strings.Contains(err.Error(), tt.wantSub) {
				t.Errorf("error = %v, want substring %q", err, tt.wantSub)
			}
		})
	}
}

func TestRoutes_AdvancedKeysIgnored(t *testing.T) {
	// conditions/sequence/random/websocket/sse are not supported; they must be
	// ignored without error, leaving the basic response (if any) intact.
	const src = `
routes:
  - path: /adv
    method: GET
    response: { status: 200, body: "base" }
    conditions:
      - match: { foo: bar }
    sequence:
      - body: "x"
    websocket: true
`
	rec := doRouted(t, src, false, httptest.NewRequest(http.MethodGet, "/adv", nil))
	if rec.Code != http.StatusOK || rec.Body.String() != "base" {
		t.Errorf("status=%d body=%q, want 200 base (advanced keys ignored)", rec.Code, rec.Body.String())
	}
}

func TestCompileRoutes_SettingsPointerSemantics(t *testing.T) {
	// Explicit zero/false values in the file must be honored as-is (not treated
	// as "absent"), while an omitted fail_status falls back to the 500 default.
	const explicit = `
settings:
  fail_rate: 0
  fail_status: 503
  cors: false
  latency: 0
routes: []
`
	c, err := CompileRoutes([]byte(explicit), t.TempDir())
	if err != nil {
		t.Fatalf("CompileRoutes: %v", err)
	}
	s := c.Settings()
	if s.FailRate != 0 || s.FailStatus != 503 || s.CORS != false || s.Latency != 0 {
		t.Errorf("explicit settings = %+v, want fail_rate=0 fail_status=503 cors=false latency=0", s)
	}

	// Absent fail_status defaults to 500.
	c2, err := CompileRoutes([]byte("routes: []\n"), t.TempDir())
	if err != nil {
		t.Fatalf("CompileRoutes (empty): %v", err)
	}
	if got := c2.Settings().FailStatus; got != http.StatusInternalServerError {
		t.Errorf("absent fail_status = %d, want 500 default", got)
	}
}

func TestRoutesStore_SettingsOverridePrecedence(t *testing.T) {
	// File sets cors:true and fail_rate:50; an override emulating an explicit CLI
	// flag must win, while leaving an unset field (latency) at the file value.
	const src = `
settings:
  cors: true
  fail_rate: 50
  latency: 200ms
routes: []
`
	store := newStore(t, src, t.TempDir())
	// Sanity: file values present before override.
	if s := store.Load().Settings(); !s.CORS || s.FailRate != 50 || s.Latency != 200*time.Millisecond {
		t.Fatalf("pre-override settings = %+v", s)
	}

	// Emulate `--cors=false --fail-rate=0` while leaving latency unset.
	store.SetSettingsOverride(func(s *RouteSettings) {
		s.CORS = false
		s.FailRate = 0
	})
	s := store.Load().Settings()
	if s.CORS {
		t.Errorf("CORS = true, want false (override should win over file true)")
	}
	if s.FailRate != 0 {
		t.Errorf("FailRate = %v, want 0 (override should win over file 50)", s.FailRate)
	}
	if s.Latency != 200*time.Millisecond {
		t.Errorf("Latency = %v, want 200ms (unset override must not clobber file value)", s.Latency)
	}
}

func TestRoutesStore_ReloadHotReloadsSettings(t *testing.T) {
	// Editing latency/fail_rate in the watched file takes effect after Reload,
	// and a CLI override (baked via SetSettingsOverride) survives the reload.
	dir := t.TempDir()
	path := filepath.Join(dir, "routes.yml")
	write := func(body string) {
		if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
			t.Fatalf("write: %v", err)
		}
	}
	write("settings:\n  fail_rate: 0\n  latency: 0\nroutes:\n  - path: /v\n    response: { status: 200, body: \"v1\" }\n")
	compiled, err := LoadRoutes(path)
	if err != nil {
		t.Fatalf("LoadRoutes: %v", err)
	}
	store := NewRoutesStore(path, compiled, nil)
	// CLI override: fail-status pinned to 503; latency/fail-rate left to file.
	store.SetSettingsOverride(func(s *RouteSettings) { s.FailStatus = 503 })

	if s := store.Load().Settings(); s.FailRate != 0 || s.Latency != 0 || s.FailStatus != 503 {
		t.Fatalf("initial effective settings = %+v", s)
	}

	// Edit the file to add latency + a 100% fail-rate; Reload must apply them and
	// keep the CLI override (fail_status 503).
	write("settings:\n  fail_rate: 100\n  latency: 25ms\nroutes:\n  - path: /v\n    response: { status: 200, body: \"v1\" }\n")
	if err := store.Reload(); err != nil {
		t.Fatalf("Reload: %v", err)
	}
	s := store.Load().Settings()
	if s.FailRate != 100 {
		t.Errorf("FailRate = %v, want 100 (edited file value applied)", s.FailRate)
	}
	if s.Latency != 25*time.Millisecond {
		t.Errorf("Latency = %v, want 25ms (edited file value applied)", s.Latency)
	}
	if s.FailStatus != 503 {
		t.Errorf("FailStatus = %d, want 503 (CLI override must survive reload)", s.FailStatus)
	}

	// The routed handler reads settings live: a 100% fail-rate now short-circuits
	// every request with the overridden 503.
	h := NewRoutedHandler(RoutedHandlerConfig{Store: store, Builtin: false})
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/v", nil))
	if rec.Code != http.StatusServiceUnavailable {
		t.Errorf("status = %d, want 503 (hot-reloaded fail_rate + override fail_status)", rec.Code)
	}
}

func TestRoutesStore_ManualReload(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "routes.yml")
	if err := os.WriteFile(path, []byte("routes:\n  - path: /v\n    response: { status: 200, body: \"v1\" }\n"), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	compiled, err := LoadRoutes(path)
	if err != nil {
		t.Fatalf("LoadRoutes: %v", err)
	}
	store := NewRoutesStore(path, compiled, nil)
	h := NewRoutedHandler(RoutedHandlerConfig{Store: store, Builtin: false})

	body := func() string {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/v", nil))
		return rec.Body.String()
	}
	if body() != "v1" {
		t.Fatalf("initial body = %q, want v1", body())
	}

	// Valid edit, manual reload swaps it in.
	if err := os.WriteFile(path, []byte("routes:\n  - path: /v\n    response: { status: 200, body: \"v2\" }\n"), 0o600); err != nil {
		t.Fatalf("write v2: %v", err)
	}
	if err := store.Reload(); err != nil {
		t.Fatalf("Reload: %v", err)
	}
	if body() != "v2" {
		t.Errorf("after reload body = %q, want v2", body())
	}

	// Invalid edit is rejected; previous good config (v2) stays active.
	if err := os.WriteFile(path, []byte("routes: [::::"), 0o600); err != nil {
		t.Fatalf("write bad: %v", err)
	}
	if err := store.Reload(); err == nil {
		t.Fatal("Reload of invalid file: expected error, got nil")
	}
	if body() != "v2" {
		t.Errorf("after invalid reload body = %q, want v2 (previous config kept)", body())
	}
}

func TestRoutesStore_WatchHotReload(t *testing.T) {
	// Positive path only: a valid edit to the watched file is eventually applied.
	// The "invalid edit keeps previous good config" assertion is covered
	// deterministically by TestRoutesStore_ManualReload (and
	// TestRoutesStore_ReloadKeepsPreviousOnInvalid) via the direct Reload path,
	// not the timing-sensitive watcher, to avoid flakes.
	dir := t.TempDir()
	path := filepath.Join(dir, "routes.yml")
	if err := os.WriteFile(path, []byte("routes:\n  - path: /v\n    response: { status: 200, body: \"v1\" }\n"), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	compiled, err := LoadRoutes(path)
	if err != nil {
		t.Fatalf("LoadRoutes: %v", err)
	}
	store := NewRoutesStore(path, compiled, nil)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := store.Watch(ctx); err != nil {
		t.Fatalf("Watch: %v", err)
	}

	h := NewRoutedHandler(RoutedHandlerConfig{Store: store, Builtin: false})
	body := func() string {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/v", nil))
		return rec.Body.String()
	}

	// Modify the file; the watcher should eventually reload it.
	if err := os.WriteFile(path, []byte("routes:\n  - path: /v\n    response: { status: 200, body: \"v2\" }\n"), 0o600); err != nil {
		t.Fatalf("write v2: %v", err)
	}
	if !eventually(t, 2*time.Second, func() bool { return body() == "v2" }) {
		t.Errorf("watcher did not reload to v2 within timeout (last body %q)", body())
	}
}

// TestRoutesStore_ReloadKeepsPreviousOnInvalid asserts deterministically (via the
// direct Reload path, no timing) that an invalid edit is rejected and the
// previous good configuration keeps serving.
func TestRoutesStore_ReloadKeepsPreviousOnInvalid(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "routes.yml")
	if err := os.WriteFile(path, []byte("routes:\n  - path: /v\n    response: { status: 200, body: \"v1\" }\n"), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	compiled, err := LoadRoutes(path)
	if err != nil {
		t.Fatalf("LoadRoutes: %v", err)
	}
	store := NewRoutesStore(path, compiled, nil)
	h := NewRoutedHandler(RoutedHandlerConfig{Store: store, Builtin: false})
	body := func() string {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/v", nil))
		return rec.Body.String()
	}
	if body() != "v1" {
		t.Fatalf("initial body = %q, want v1", body())
	}

	// Replace with an invalid file and reload; Reload must error and the previous
	// good config (v1) must keep serving.
	if err := os.WriteFile(path, []byte("routes: [::::"), 0o600); err != nil {
		t.Fatalf("write bad: %v", err)
	}
	if err := store.Reload(); err == nil {
		t.Fatal("Reload of invalid file: expected error, got nil")
	}
	if got := body(); got != "v1" {
		t.Errorf("after invalid reload body = %q, want v1 (previous config kept)", got)
	}
}

// eventually polls fn until it returns true or the timeout elapses.
func eventually(t *testing.T, timeout time.Duration, fn func() bool) bool {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if fn() {
			return true
		}
		time.Sleep(20 * time.Millisecond)
	}
	return fn()
}
