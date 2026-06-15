package middleware_test

import (
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/osuritz/radix/internal/server/middleware"
)

// authRecorder counts auth-header injections; it satisfies
// middleware.AuthMetricsRecorder.
type authRecorder struct{ injections atomic.Uint64 }

func (a *authRecorder) RecordProxyAuthInjection() { a.injections.Add(1) }

func TestInjectHeaders_RecordsMetricsOnInjection(t *testing.T) {
	hdrs := http.Header{}
	hdrs.Set("Authorization", "Bearer token123")
	provider := &mockProvider{headers: hdrs, name: "test"}

	rec := &authRecorder{}
	handler := middleware.InjectHeaders(provider, middleware.WithMetrics(rec))(
		http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}),
	)

	handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/api", nil))
	if got := rec.injections.Load(); got != 1 {
		t.Errorf("auth injections = %d, want 1", got)
	}
}

func TestInjectHeaders_NoMetricsWhenNoHeaders(t *testing.T) {
	// Provider returns no headers, so no injection should be counted.
	provider := &mockProvider{headers: http.Header{}, name: "empty"}

	rec := &authRecorder{}
	handler := middleware.InjectHeaders(provider, middleware.WithMetrics(rec))(
		http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}),
	)

	handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/api", nil))
	if got := rec.injections.Load(); got != 0 {
		t.Errorf("auth injections = %d, want 0 (no headers injected)", got)
	}
}

func TestInjectHeaders_NilMetricsRecorderNoPanic(_ *testing.T) {
	hdrs := http.Header{}
	hdrs.Set("Authorization", "Bearer x")
	provider := &mockProvider{headers: hdrs, name: "test"}

	// A nil recorder (metrics disabled) must not panic.
	handler := middleware.InjectHeaders(provider, middleware.WithMetrics(nil))(
		http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}),
	)
	handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/api", nil))
}
