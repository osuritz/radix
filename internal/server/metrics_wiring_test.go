package server

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
)

// fakeRecorder is a test double satisfying the EchoMetricsRecorder,
// MockMetricsRecorder, and ProxyMetricsRecorder interfaces at once. All counters
// are atomic so the recorder is safe to share across request goroutines under
// -race.
type fakeRecorder struct {
	echoDelays     atomic.Uint64
	echoCustomBody atomic.Uint64
	echoPathStatus atomic.Uint64

	mockMatchCustom  atomic.Uint64
	mockMatchBuiltin atomic.Uint64
	mockRenders      atomic.Uint64
	mockErrors       atomic.Uint64
	mockReloads      atomic.Uint64
	mockFails        atomic.Uint64
	mockFbNotFound   atomic.Uint64
	mockFbProxy      atomic.Uint64

	proxyAuth   atomic.Uint64
	proxyStream atomic.Uint64
}

func (f *fakeRecorder) RecordEchoDelay()      { f.echoDelays.Add(1) }
func (f *fakeRecorder) RecordEchoCustomBody() { f.echoCustomBody.Add(1) }
func (f *fakeRecorder) RecordEchoPathStatus() { f.echoPathStatus.Add(1) }

func (f *fakeRecorder) RecordMockRouteMatch(custom bool) {
	if custom {
		f.mockMatchCustom.Add(1)
		return
	}
	f.mockMatchBuiltin.Add(1)
}
func (f *fakeRecorder) RecordMockTemplateRender() { f.mockRenders.Add(1) }
func (f *fakeRecorder) RecordMockTemplateError()  { f.mockErrors.Add(1) }
func (f *fakeRecorder) RecordMockReload()         { f.mockReloads.Add(1) }
func (f *fakeRecorder) RecordMockFailInjection()  { f.mockFails.Add(1) }
func (f *fakeRecorder) RecordMockFallback(kind string) {
	switch kind {
	case "404", "not_found":
		f.mockFbNotFound.Add(1)
	case "proxy":
		f.mockFbProxy.Add(1)
	}
}

func (f *fakeRecorder) RecordProxyAuthInjection() { f.proxyAuth.Add(1) }
func (f *fakeRecorder) RecordProxyStream()        { f.proxyStream.Add(1) }

func TestEchoMetricsWiring(t *testing.T) {
	rec := &fakeRecorder{}
	cfg := defaultEchoConfig()
	cfg.Metrics = rec
	cfg.StatusFromPath = true
	cfg.DelayFromPath = true

	// /delay/<dur> applies a delay; status-from-path is not triggered by /delay.
	req := httptest.NewRequest(http.MethodGet, "/delay/1ms", nil)
	NewEchoHandler(cfg).ServeHTTP(httptest.NewRecorder(), req)
	if got := rec.echoDelays.Load(); got != 1 {
		t.Errorf("echoDelays = %d, want 1", got)
	}

	// /404 derives the status from the path.
	req = httptest.NewRequest(http.MethodGet, "/404", nil)
	NewEchoHandler(cfg).ServeHTTP(httptest.NewRecorder(), req)
	if got := rec.echoPathStatus.Load(); got != 1 {
		t.Errorf("echoPathStatus = %d, want 1", got)
	}

	// A custom literal body counts a custom-body response.
	bodyCfg := defaultEchoConfig()
	bodyCfg.Metrics = rec
	bodyCfg.Body = "hello"
	NewEchoHandler(bodyCfg).ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/", nil))
	if got := rec.echoCustomBody.Load(); got != 1 {
		t.Errorf("echoCustomBody = %d, want 1", got)
	}
}

func TestEchoMetricsNilRecorderNoPanic(_ *testing.T) {
	cfg := defaultEchoConfig()
	cfg.Metrics = nil // metrics disabled
	cfg.StatusFromPath = true
	cfg.Body = "hi"

	// Must not panic with a nil recorder.
	NewEchoHandler(cfg).ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/418", nil))
}

func TestMockBuiltinMetricsWiring(t *testing.T) {
	rec := &fakeRecorder{}
	h := NewMockHandler(MockConfig{Builtin: true, Metrics: rec})

	// A matching built-in endpoint records a built-in route match.
	h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/get", nil))
	if got := rec.mockMatchBuiltin.Load(); got != 1 {
		t.Errorf("mockMatchBuiltin = %d, want 1", got)
	}

	// A non-matching path records no match.
	h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/nope", nil))
	if got := rec.mockMatchBuiltin.Load(); got != 1 {
		t.Errorf("mockMatchBuiltin after miss = %d, want 1", got)
	}
}

func TestMockFailInjectionMetricsWiring(t *testing.T) {
	rec := &fakeRecorder{}
	// FailRate 100 always injects a failure.
	h := NewMockHandler(MockConfig{Builtin: true, FailRate: 100, FailStatus: 503, Metrics: rec})

	resp := httptest.NewRecorder()
	h.ServeHTTP(resp, httptest.NewRequest(http.MethodGet, "/get", nil))
	if resp.Code != 503 {
		t.Fatalf("status = %d, want 503", resp.Code)
	}
	if got := rec.mockFails.Load(); got != 1 {
		t.Errorf("mockFails = %d, want 1", got)
	}
	// A fail injection short-circuits before any route match is recorded.
	if got := rec.mockMatchBuiltin.Load(); got != 0 {
		t.Errorf("mockMatchBuiltin = %d, want 0 (fail injected)", got)
	}
}

// newRoutesStoreFromYAML writes a routes file and returns a seeded store.
func newRoutesStoreFromYAML(t *testing.T, yaml string) *RoutesStore {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "routes.yml")
	if err := os.WriteFile(path, []byte(yaml), 0o600); err != nil {
		t.Fatalf("write routes file: %v", err)
	}
	compiled, err := LoadRoutes(path)
	if err != nil {
		t.Fatalf("load routes: %v", err)
	}
	return NewRoutesStore(path, compiled, nil)
}

func TestRoutedMockMetricsWiring(t *testing.T) {
	rec := &fakeRecorder{}
	const yaml = `
routes:
  - path: /hello
    response:
      status: 200
      body: "hi {{.path}}"
`
	store := newRoutesStoreFromYAML(t, yaml)
	h := NewRoutedHandler(RoutedHandlerConfig{Store: store, Builtin: true, Metrics: rec})

	// Custom route match + template render.
	h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/hello", nil))
	if got := rec.mockMatchCustom.Load(); got != 1 {
		t.Errorf("mockMatchCustom = %d, want 1", got)
	}
	if got := rec.mockRenders.Load(); got != 1 {
		t.Errorf("mockRenders = %d, want 1", got)
	}

	// Built-in fall-through.
	h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/get", nil))
	if got := rec.mockMatchBuiltin.Load(); got != 1 {
		t.Errorf("mockMatchBuiltin = %d, want 1", got)
	}

	// Unmatched -> 404 fallback.
	h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/missing", nil))
	if got := rec.mockFbNotFound.Load(); got != 1 {
		t.Errorf("mockFbNotFound = %d, want 1", got)
	}
}

func TestRoutedMockFallbackProxyMetricsWiring(t *testing.T) {
	rec := &fakeRecorder{}

	// Backend stands in for the configured fallback proxy target.
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTeapot)
		_, _ = w.Write([]byte("from-backend:" + r.URL.Path))
	}))
	defer backend.Close()

	// A routes file with a proxy fallback and no custom routes: any unmatched
	// request falls through to the proxy.
	yaml := "settings:\n  fallback:\n    type: proxy\n    proxy_target: " + backend.URL + "\nroutes: []\n"
	store := newRoutesStoreFromYAML(t, yaml)
	h := NewRoutedHandler(RoutedHandlerConfig{Store: store, Metrics: rec})

	resp := httptest.NewRecorder()
	h.ServeHTTP(resp, httptest.NewRequest(http.MethodGet, "/unmatched", nil))

	// The request reached the backend via the fallback proxy.
	if resp.Code != http.StatusTeapot {
		t.Fatalf("status = %d, want 418 from backend (proxy fallback)", resp.Code)
	}
	if got := rec.mockFbProxy.Load(); got != 1 {
		t.Errorf("mockFbProxy = %d, want 1", got)
	}
	// The 404 fallback path must not be taken when a proxy fallback is configured.
	if got := rec.mockFbNotFound.Load(); got != 0 {
		t.Errorf("mockFbNotFound = %d, want 0 (proxy fallback configured)", got)
	}
}

func TestRoutedMockTemplateErrorMetricsWiring(t *testing.T) {
	rec := &fakeRecorder{}
	// randomChoice with no args errors at render time -> template error + 500.
	const yaml = `
routes:
  - path: /boom
    response:
      status: 200
      body: "{{randomChoice}}"
`
	store := newRoutesStoreFromYAML(t, yaml)
	h := NewRoutedHandler(RoutedHandlerConfig{Store: store, Metrics: rec})

	resp := httptest.NewRecorder()
	h.ServeHTTP(resp, httptest.NewRequest(http.MethodGet, "/boom", nil))
	if resp.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", resp.Code)
	}
	if got := rec.mockErrors.Load(); got != 1 {
		t.Errorf("mockErrors = %d, want 1", got)
	}
	if got := rec.mockRenders.Load(); got != 0 {
		t.Errorf("mockRenders = %d, want 0 on render error", got)
	}
}

func TestRoutesStoreReloadMetricsWiring(t *testing.T) {
	rec := &fakeRecorder{}
	store := newRoutesStoreFromYAML(t, "routes: []\n")
	store.SetMetricsRecorder(rec)

	if err := store.Reload(); err != nil {
		t.Fatalf("reload: %v", err)
	}
	if got := rec.mockReloads.Load(); got != 1 {
		t.Errorf("mockReloads = %d, want 1", got)
	}
}

func TestProxyStreamMetricsWiring(t *testing.T) {
	rec := &fakeRecorder{}

	// Backend returns a text/event-stream response.
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("data: hi\n\n"))
	}))
	defer backend.Close()

	target, _ := url.Parse(backend.URL)
	h := NewReverseProxy(ProxyConfig{Target: target, FlushInterval: -1, Metrics: rec})

	resp := httptest.NewRecorder()
	h.ServeHTTP(resp, httptest.NewRequest(http.MethodGet, "/stream", nil))
	if got := rec.proxyStream.Load(); got != 1 {
		t.Errorf("proxyStream = %d, want 1", got)
	}
}

func TestProxyNonStreamMetricsWiring(t *testing.T) {
	rec := &fakeRecorder{}

	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer backend.Close()

	target, _ := url.Parse(backend.URL)
	h := NewReverseProxy(ProxyConfig{Target: target, Metrics: rec})

	resp := httptest.NewRecorder()
	h.ServeHTTP(resp, httptest.NewRequest(http.MethodGet, "/data", nil))
	// ModifyResponse runs synchronously during ServeHTTP, so the counter is final.
	if got := rec.proxyStream.Load(); got != 0 {
		t.Errorf("proxyStream = %d, want 0 for non-streaming response", got)
	}
}
