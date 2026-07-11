package server

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
)

// benchmarkReverseProxy proxies GET requests end-to-end to a real httptest
// backend that responds with body, failing on any non-200 response.
func benchmarkReverseProxy(b *testing.B, body []byte) {
	b.Helper()
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(body)
	}))
	defer backend.Close()

	target, err := url.Parse(backend.URL)
	if err != nil {
		b.Fatalf("parse backend URL: %v", err)
	}
	handler := NewReverseProxy(ProxyConfig{Target: target})

	b.ReportAllocs()
	for b.Loop() {
		req := httptest.NewRequest(http.MethodGet, "/api/thing", nil)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			b.Fatalf("status = %d, want 200 (body: %s)", rec.Code, rec.Body.String())
		}
	}
}

func BenchmarkReverseProxy_SmallBody(b *testing.B) {
	benchmarkReverseProxy(b, []byte(`{"ok":true}`))
}

func BenchmarkReverseProxy_64KBody(b *testing.B) {
	benchmarkReverseProxy(b, bytes.Repeat([]byte("x"), 64<<10))
}
