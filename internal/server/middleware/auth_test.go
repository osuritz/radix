package middleware_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/osuritz/radix/internal/server/middleware"
)

// mockProvider is a test HeaderProvider.
type mockProvider struct {
	headers http.Header
	err     error
	name    string
}

func (m *mockProvider) Headers(_ context.Context, _ *http.Request) (http.Header, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.headers.Clone(), nil
}

func (m *mockProvider) Name() string { return m.name }

func TestInjectHeaders_SetsHeaders(t *testing.T) {
	hdrs := http.Header{}
	hdrs.Set("Authorization", "Bearer token123")
	hdrs.Set("X-Custom", "value")

	provider := &mockProvider{headers: hdrs, name: "test"}

	var capturedAuth, capturedCustom string
	inner := http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		capturedAuth = r.Header.Get("Authorization")
		capturedCustom = r.Header.Get("X-Custom")
	})

	handler := middleware.InjectHeaders(provider)(inner)

	req := httptest.NewRequest(http.MethodGet, "/api/data", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if capturedAuth != "Bearer token123" {
		t.Errorf("Authorization = %q, want %q", capturedAuth, "Bearer token123")
	}
	if capturedCustom != "value" {
		t.Errorf("X-Custom = %q, want %q", capturedCustom, "value")
	}
}

func TestInjectHeaders_Returns502OnError(t *testing.T) {
	provider := &mockProvider{
		err:  errors.New("token refresh failed"),
		name: "failing",
	}

	called := false
	inner := http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
		called = true
	})

	handler := middleware.InjectHeaders(provider)(inner)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadGateway {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusBadGateway)
	}
	if called {
		t.Error("next handler should not be called on provider error")
	}
}

func TestInjectHeaders_ConcurrentAccess(t *testing.T) {
	hdrs := http.Header{}
	hdrs.Set("Authorization", "Bearer concurrent-token")

	provider := &mockProvider{headers: hdrs, name: "concurrent"}

	inner := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	handler := middleware.InjectHeaders(provider)(inner)

	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			req := httptest.NewRequest(http.MethodGet, "/", nil)
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)
			if rec.Code != http.StatusOK {
				t.Errorf("status = %d, want %d", rec.Code, http.StatusOK)
			}
		}()
	}
	wg.Wait()
}
