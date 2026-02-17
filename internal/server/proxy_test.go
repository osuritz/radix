package server

import (
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

func TestNewReverseProxy_BasicForwarding(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("X-Backend", "true")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("hello from backend"))
	}))
	defer backend.Close()

	targetURL, _ := url.Parse(backend.URL)
	proxy := NewReverseProxy(ProxyConfig{
		Target:  targetURL,
		Timeout: 5 * time.Second,
	})

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	rec := httptest.NewRecorder()
	proxy.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", rec.Code)
	}
	if body := rec.Body.String(); body != "hello from backend" {
		t.Errorf("expected body %q, got %q", "hello from backend", body)
	}
	if rec.Header().Get("X-Backend") != "true" {
		t.Error("expected X-Backend response header from backend")
	}
}

func TestNewReverseProxy_PostWithBody(t *testing.T) {
	var receivedBody string
	var receivedMethod string
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedMethod = r.Method
		b, _ := io.ReadAll(r.Body)
		receivedBody = string(b)
		w.WriteHeader(http.StatusCreated)
	}))
	defer backend.Close()

	targetURL, _ := url.Parse(backend.URL)
	proxy := NewReverseProxy(ProxyConfig{
		Target:  targetURL,
		Timeout: 5 * time.Second,
	})

	req := httptest.NewRequest(http.MethodPost, "/api/data", strings.NewReader(`{"key":"value"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	proxy.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Errorf("expected status 201, got %d", rec.Code)
	}
	if receivedMethod != http.MethodPost {
		t.Errorf("expected method POST, got %s", receivedMethod)
	}
	if receivedBody != `{"key":"value"}` {
		t.Errorf("expected body %q, got %q", `{"key":"value"}`, receivedBody)
	}
}

func TestNewReverseProxy_PathPreservation(t *testing.T) {
	var receivedPath string
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedPath = r.URL.Path
		w.WriteHeader(http.StatusOK)
	}))
	defer backend.Close()

	targetURL, _ := url.Parse(backend.URL)
	proxy := NewReverseProxy(ProxyConfig{
		Target:  targetURL,
		Timeout: 5 * time.Second,
	})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/users?page=1", nil)
	rec := httptest.NewRecorder()
	proxy.ServeHTTP(rec, req)

	if receivedPath != "/api/v1/users" {
		t.Errorf("expected path %q, got %q", "/api/v1/users", receivedPath)
	}
}

func TestNewReverseProxy_StripPrefix(t *testing.T) {
	var receivedPath string
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedPath = r.URL.Path
		w.WriteHeader(http.StatusOK)
	}))
	defer backend.Close()

	targetURL, _ := url.Parse(backend.URL)
	proxy := NewReverseProxy(ProxyConfig{
		Target:      targetURL,
		Timeout:     5 * time.Second,
		StripPrefix: "/api",
	})

	req := httptest.NewRequest(http.MethodGet, "/api/users", nil)
	rec := httptest.NewRecorder()
	proxy.ServeHTTP(rec, req)

	if receivedPath != "/users" {
		t.Errorf("expected path %q after strip prefix, got %q", "/users", receivedPath)
	}
}

func TestNewReverseProxy_StripPrefixRootFallback(t *testing.T) {
	var receivedPath string
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedPath = r.URL.Path
		w.WriteHeader(http.StatusOK)
	}))
	defer backend.Close()

	targetURL, _ := url.Parse(backend.URL)
	proxy := NewReverseProxy(ProxyConfig{
		Target:      targetURL,
		Timeout:     5 * time.Second,
		StripPrefix: "/api",
	})

	req := httptest.NewRequest(http.MethodGet, "/api", nil)
	rec := httptest.NewRecorder()
	proxy.ServeHTTP(rec, req)

	if receivedPath != "/" {
		t.Errorf("expected path %q when prefix equals full path, got %q", "/", receivedPath)
	}
}

func TestNewReverseProxy_PathRewrite(t *testing.T) {
	var receivedPath string
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedPath = r.URL.Path
		w.WriteHeader(http.StatusOK)
	}))
	defer backend.Close()

	targetURL, _ := url.Parse(backend.URL)
	proxy := NewReverseProxy(ProxyConfig{
		Target:  targetURL,
		Timeout: 5 * time.Second,
		Rewrite: "/old:/new",
	})

	req := httptest.NewRequest(http.MethodGet, "/old/path", nil)
	rec := httptest.NewRecorder()
	proxy.ServeHTTP(rec, req)

	if receivedPath != "/new/path" {
		t.Errorf("expected rewritten path %q, got %q", "/new/path", receivedPath)
	}
}

func TestNewReverseProxy_HeadersForwarded(t *testing.T) {
	var receivedHeaders http.Header
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedHeaders = r.Header.Clone()
		w.WriteHeader(http.StatusOK)
	}))
	defer backend.Close()

	targetURL, _ := url.Parse(backend.URL)
	proxy := NewReverseProxy(ProxyConfig{
		Target:  targetURL,
		Timeout: 5 * time.Second,
	})

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set("X-Custom-Header", "custom-value")
	req.Header.Set("Authorization", "Bearer token123")
	rec := httptest.NewRecorder()
	proxy.ServeHTTP(rec, req)

	if receivedHeaders.Get("X-Custom-Header") != "custom-value" {
		t.Error("expected X-Custom-Header to be forwarded to backend")
	}
	if receivedHeaders.Get("Authorization") != "Bearer token123" {
		t.Error("expected Authorization header to be forwarded to backend")
	}
}

func TestNewReverseProxy_BackendDown(t *testing.T) {
	// Use a URL that will fail to connect
	targetURL, _ := url.Parse("http://127.0.0.1:1") // port 1 is almost certainly not listening
	proxy := NewReverseProxy(ProxyConfig{
		Target:  targetURL,
		Timeout: 1 * time.Second,
	})

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	rec := httptest.NewRecorder()
	proxy.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadGateway {
		t.Errorf("expected status 502 when backend is down, got %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "proxy error") {
		t.Error("expected error body to contain 'proxy error'")
	}
}

func TestNewReverseProxy_HostHeader(t *testing.T) {
	var receivedHost string
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedHost = r.Host
		w.WriteHeader(http.StatusOK)
	}))
	defer backend.Close()

	targetURL, _ := url.Parse(backend.URL)
	proxy := NewReverseProxy(ProxyConfig{
		Target:  targetURL,
		Timeout: 5 * time.Second,
	})

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	rec := httptest.NewRecorder()
	proxy.ServeHTTP(rec, req)

	if receivedHost != targetURL.Host {
		t.Errorf("expected Host header %q, got %q", targetURL.Host, receivedHost)
	}
}
