package server

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/osuritz/radix/internal/metrics"
)

// newTestMux builds a ServeMux with metrics + healthz + SPA wired up, exactly
// as NewAdminServer does, but using an in-memory FS for the SPA layer.
func newTestMux(t *testing.T, fsys fstest.MapFS, collector *metrics.Collector) *http.ServeMux {
	t.Helper()
	mux := http.NewServeMux()
	if collector != nil {
		mux.Handle("/_metrics", withMetricsCORS(collector.Handler("json")))
	}
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, `{"status":"ok"}`)
	})
	serveUIFromFS(mux, fsys)
	return mux
}

// TestServeUI_ExistingAssetServedWithContentType verifies that a file present
// in the FS is served directly with the correct Content-Type.
func TestServeUI_ExistingAssetServedWithContentType(t *testing.T) {
	t.Parallel()

	fsys := fstest.MapFS{
		"index.html": {Data: []byte("<html>hello</html>"), ModTime: time.Now()},
		"assets/app.js": {
			Data:    []byte("console.log('hi')"),
			ModTime: time.Now(),
		},
	}

	mux := newTestMux(t, fsys, nil)

	req := httptest.NewRequest(http.MethodGet, "/assets/app.js", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	resp := rec.Result()
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("status = %d, want %d", resp.StatusCode, http.StatusOK)
	}
	ct := resp.Header.Get("Content-Type")
	if !strings.Contains(ct, "javascript") {
		t.Errorf("Content-Type = %q, want it to contain 'javascript'", ct)
	}
	body, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(body), "console.log") {
		t.Errorf("body = %q, expected JS content", body)
	}
}

// TestServeUI_UnknownPathFallsBackToIndexHTML verifies that an unknown deep-link
// path is served as index.html with HTTP 200 (SPA fallback).
func TestServeUI_UnknownPathFallsBackToIndexHTML(t *testing.T) {
	t.Parallel()

	fsys := fstest.MapFS{
		"index.html": {Data: []byte("<html>spa</html>"), ModTime: time.Now()},
	}

	mux := newTestMux(t, fsys, nil)

	for _, p := range []string{"/some/deep/link", "/dashboard/overview", "/unknown"} {
		p := p
		t.Run(p, func(t *testing.T) {
			t.Parallel()
			req := httptest.NewRequest(http.MethodGet, p, nil)
			rec := httptest.NewRecorder()
			mux.ServeHTTP(rec, req)

			resp := rec.Result()
			defer func() { _ = resp.Body.Close() }()

			if resp.StatusCode != http.StatusOK {
				t.Errorf("path %s: status = %d, want %d", p, resp.StatusCode, http.StatusOK)
			}
			ct := resp.Header.Get("Content-Type")
			if !strings.Contains(ct, "text/html") {
				t.Errorf("path %s: Content-Type = %q, want text/html", p, ct)
			}
			body, _ := io.ReadAll(resp.Body)
			if !strings.Contains(string(body), "spa") {
				t.Errorf("path %s: body = %q, expected index.html content", p, body)
			}
		})
	}
}

// TestServeUI_PlaceholderBehavior verifies that when the FS contains no
// index.html (placeholder-only build), the handler returns a 200 page that
// instructs the developer to run `make ui`.
func TestServeUI_PlaceholderBehavior(t *testing.T) {
	t.Parallel()

	// No index.html — only the gitkeep placeholder.
	fsys := fstest.MapFS{
		".gitkeep": {Data: []byte{}, ModTime: time.Now()},
	}

	mux := newTestMux(t, fsys, nil)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	resp := rec.Result()
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("status = %d, want %d", resp.StatusCode, http.StatusOK)
	}
	body, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(body), "make ui") {
		t.Errorf("placeholder body should mention 'make ui', got: %s", body)
	}
}

// TestServeUI_MetricsPrecedenceOverSPA verifies that /_metrics is served by the
// metrics handler (not the SPA catch-all) and carries the CORS header.
func TestServeUI_MetricsPrecedenceOverSPA(t *testing.T) {
	t.Parallel()

	fsys := fstest.MapFS{
		"index.html": {Data: []byte("<html>spa</html>"), ModTime: time.Now()},
	}
	collector := metrics.NewCollector("test", "0.0.1")
	collector.RecordRequest(200, "GET", 5*time.Millisecond, 10, 20)

	mux := newTestMux(t, fsys, collector)

	req := httptest.NewRequest(http.MethodGet, "/_metrics", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	resp := rec.Result()
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("status = %d, want %d", resp.StatusCode, http.StatusOK)
	}
	ct := resp.Header.Get("Content-Type")
	if !strings.Contains(ct, "application/json") {
		t.Errorf("Content-Type = %q, want application/json", ct)
	}
	cors := resp.Header.Get("Access-Control-Allow-Origin")
	if cors != "*" {
		t.Errorf("Access-Control-Allow-Origin = %q, want *", cors)
	}
	body, _ := io.ReadAll(resp.Body)
	if strings.Contains(string(body), "spa") {
		t.Error("/_metrics should NOT be served by the SPA handler")
	}
	if !strings.Contains(string(body), "total") {
		t.Errorf("/_metrics body should contain metrics JSON, got: %s", body)
	}
}

// TestServeUI_OptionsPreflight verifies that OPTIONS /_metrics returns 204 with
// the CORS header (preflight support for the Vite dev server).
func TestServeUI_OptionsPreflight(t *testing.T) {
	t.Parallel()

	fsys := fstest.MapFS{
		"index.html": {Data: []byte("<html>spa</html>"), ModTime: time.Now()},
	}
	collector := metrics.NewCollector("test", "0.0.1")
	mux := newTestMux(t, fsys, collector)

	req := httptest.NewRequest(http.MethodOptions, "/_metrics", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	resp := rec.Result()
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusNoContent {
		t.Errorf("OPTIONS status = %d, want %d (NoContent)", resp.StatusCode, http.StatusNoContent)
	}
	cors := resp.Header.Get("Access-Control-Allow-Origin")
	if cors != "*" {
		t.Errorf("Access-Control-Allow-Origin = %q, want *", cors)
	}
}

// TestServeUI_HealthzUnaffected verifies that /healthz is served by its own
// handler and is not intercepted by the SPA catch-all. CORS must NOT be set on
// non-metrics routes (it is scoped to /_metrics only).
func TestServeUI_HealthzUnaffected(t *testing.T) {
	t.Parallel()

	fsys := fstest.MapFS{
		"index.html": {Data: []byte("<html>spa</html>"), ModTime: time.Now()},
	}

	mux := newTestMux(t, fsys, nil)

	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	resp := rec.Result()
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("status = %d, want %d", resp.StatusCode, http.StatusOK)
	}
	ct := resp.Header.Get("Content-Type")
	if !strings.Contains(ct, "application/json") {
		t.Errorf("Content-Type = %q, want application/json", ct)
	}
	body, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(body), `"status"`) {
		t.Errorf("/healthz body should contain status JSON, got: %s", body)
	}
	if strings.Contains(string(body), "spa") {
		t.Error("/healthz should NOT be served by the SPA handler")
	}
	// CORS must NOT leak onto /healthz — it is scoped to /_metrics only.
	if cors := resp.Header.Get("Access-Control-Allow-Origin"); cors != "" {
		t.Errorf("/healthz Access-Control-Allow-Origin = %q, want empty (CORS must not leak)", cors)
	}
}

// TestServeUI_SPAFallbackNoCORS verifies that the SPA catch-all handler does
// not inject CORS headers — those are scoped to /_metrics only.
func TestServeUI_SPAFallbackNoCORS(t *testing.T) {
	t.Parallel()

	fsys := fstest.MapFS{
		"index.html": {Data: []byte("<html>spa</html>"), ModTime: time.Now()},
	}
	collector := metrics.NewCollector("test", "0.0.1")
	mux := newTestMux(t, fsys, collector)

	for _, p := range []string{"/some/deep/link", "/dashboard", "/"} {
		p := p
		t.Run(p, func(t *testing.T) {
			t.Parallel()
			req := httptest.NewRequest(http.MethodGet, p, nil)
			rec := httptest.NewRecorder()
			mux.ServeHTTP(rec, req)

			resp := rec.Result()
			defer func() { _ = resp.Body.Close() }()

			if resp.StatusCode != http.StatusOK {
				t.Errorf("path %s: status = %d, want %d", p, resp.StatusCode, http.StatusOK)
			}
			// CORS must NOT be present on SPA fallback routes.
			if cors := resp.Header.Get("Access-Control-Allow-Origin"); cors != "" {
				t.Errorf("path %s: Access-Control-Allow-Origin = %q, want empty (CORS must not leak onto SPA routes)", p, cors)
			}
		})
	}
}
