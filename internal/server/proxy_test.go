package server

import (
	"io"
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
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

// TestNewReverseProxy_StreamingIncrementalFlush proves SSE pass-through
// streaming: SSE (text/event-stream) chunks reach the client incrementally as
// the backend produces them, rather than being buffered until the handler
// returns. This exercises end-to-end streaming behavior, not the FlushInterval
// knob itself (which is covered by TestNewReverseProxy_FlushIntervalWired).
func TestNewReverseProxy_StreamingIncrementalFlush(t *testing.T) {
	const chunkCount = 3
	// chunkSent is signaled by the backend after it writes & flushes each chunk.
	chunkSent := make(chan time.Time, chunkCount)
	// release blocks the backend from writing the next chunk until the client
	// has observed the previous one, proving incremental (not buffered) delivery.
	release := make(chan struct{})

	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		flusher, ok := w.(http.Flusher)
		if !ok {
			t.Errorf("backend ResponseWriter is not an http.Flusher")
			return
		}
		for i := 0; i < chunkCount; i++ {
			_, _ = w.Write([]byte("data: chunk\n\n"))
			flusher.Flush()
			chunkSent <- time.Now()
			// Wait until the test allows the next chunk to be produced.
			<-release
		}
	}))
	defer backend.Close()

	targetURL, _ := url.Parse(backend.URL)
	proxy := NewReverseProxy(ProxyConfig{
		Target:        targetURL,
		Timeout:       5 * time.Second,
		FlushInterval: -1, // flush immediately
	})

	// A real server is required: httptest.NewRecorder buffers and cannot
	// observe streaming/flushing behavior.
	front := httptest.NewServer(proxy)
	defer front.Close()

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Get(front.URL + "/stream")
	if err != nil {
		t.Fatalf("client request failed: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if ct := resp.Header.Get("Content-Type"); ct != "text/event-stream" {
		t.Errorf("expected Content-Type text/event-stream, got %q", ct)
	}

	buf := make([]byte, 64)
	for i := 0; i < chunkCount; i++ {
		// The backend must have sent this chunk.
		select {
		case <-chunkSent:
		case <-time.After(5 * time.Second):
			t.Fatalf("timed out waiting for backend to send chunk %d", i)
		}

		// The client must be able to read it BEFORE we release the backend to
		// produce later chunks. If responses were buffered until the handler
		// returned, this read would block forever (the handler is stuck on
		// <-release) and the test would time out.
		readDone := make(chan error, 1)
		go func() {
			_, rerr := resp.Body.Read(buf)
			readDone <- rerr
		}()

		select {
		case rerr := <-readDone:
			if rerr != nil {
				t.Fatalf("reading chunk %d failed: %v", i, rerr)
			}
		case <-time.After(5 * time.Second):
			t.Fatalf("timed out reading chunk %d from client; response appears buffered, not streamed", i)
		}

		// Allow the backend to write the next chunk.
		release <- struct{}{}
	}
}

func TestNewReverseProxy_ForwardedHeaders(t *testing.T) {
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
	req.Host = "client.example.com"
	req.RemoteAddr = "203.0.113.7:54321"
	rec := httptest.NewRecorder()
	proxy.ServeHTTP(rec, req)

	if got := receivedHeaders.Get("X-Forwarded-For"); got != "203.0.113.7" {
		t.Errorf("expected X-Forwarded-For %q, got %q", "203.0.113.7", got)
	}
	if got := receivedHeaders.Get("X-Forwarded-Host"); got != "client.example.com" {
		t.Errorf("expected X-Forwarded-Host %q, got %q", "client.example.com", got)
	}
	if got := receivedHeaders.Get("X-Forwarded-Proto"); got != "http" {
		t.Errorf("expected X-Forwarded-Proto %q, got %q", "http", got)
	}
}

func TestNewReverseProxy_ForwardedForNotSpoofable(t *testing.T) {
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
	req.Host = "client.example.com"
	req.RemoteAddr = "203.0.113.7:54321"
	// Simulate a malicious client attempting to spoof forwarding headers.
	req.Header.Set("X-Forwarded-For", "198.51.100.1")
	req.Header.Set("X-Forwarded-Host", "evil.example")
	req.Header.Set("X-Forwarded-Proto", "https")
	rec := httptest.NewRecorder()
	proxy.ServeHTTP(rec, req)

	// The Rewrite path strips client-supplied forwarding headers and sets
	// fresh values, so the spoofed client IP must not appear in XFF.
	if got := receivedHeaders.Get("X-Forwarded-For"); strings.Contains(got, "198.51.100.1") {
		t.Errorf("X-Forwarded-For must not contain spoofed client value, got %q", got)
	}
	if got := receivedHeaders.Get("X-Forwarded-For"); got != "203.0.113.7" {
		t.Errorf("expected X-Forwarded-For to be the real client IP %q, got %q", "203.0.113.7", got)
	}
	// XFH must reflect the real inbound Host, not the spoofed value.
	if got := receivedHeaders.Get("X-Forwarded-Host"); got != "client.example.com" {
		t.Errorf("expected X-Forwarded-Host %q, got %q", "client.example.com", got)
	}
	// XFP must reflect the real (plain HTTP) inbound scheme, not the spoofed "https".
	if got := receivedHeaders.Get("X-Forwarded-Proto"); got != "http" {
		t.Errorf("expected X-Forwarded-Proto %q, got %q", "http", got)
	}
}

func TestNewReverseProxy_FlushIntervalWired(t *testing.T) {
	targetURL, _ := url.Parse("http://backend.example")

	tests := []struct {
		name     string
		interval time.Duration
	}{
		{"periodic", 250 * time.Millisecond},
		{"immediate", -1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := NewReverseProxy(ProxyConfig{
				Target:        targetURL,
				FlushInterval: tt.interval,
			})
			rp, ok := h.(*httputil.ReverseProxy)
			if !ok {
				t.Fatalf("expected *httputil.ReverseProxy, got %T", h)
			}
			if rp.FlushInterval != tt.interval {
				t.Errorf("FlushInterval = %v, want %v", rp.FlushInterval, tt.interval)
			}
		})
	}
}

func TestIsStreamingContentType(t *testing.T) {
	tests := []struct {
		ct   string
		want bool
	}{
		{"text/event-stream", true},
		{"text/event-stream; charset=utf-8", true},
		{"application/x-ndjson", true},
		{"application/stream+json", true},
		{"application/json", false},
		{"text/html", false},
		{"", false},
	}

	for _, tt := range tests {
		if got := isStreamingContentType(tt.ct); got != tt.want {
			t.Errorf("isStreamingContentType(%q) = %v, want %v", tt.ct, got, tt.want)
		}
	}
}

func TestNewReverseProxy_ModifyResponseStreamingHeaders(t *testing.T) {
	// SSE response: should gain X-Accel-Buffering: no and Cache-Control: no-cache.
	sseBackend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("data: hi\n\n"))
	}))
	defer sseBackend.Close()

	sseURL, _ := url.Parse(sseBackend.URL)
	sseProxy := NewReverseProxy(ProxyConfig{Target: sseURL, Timeout: 5 * time.Second})

	req := httptest.NewRequest(http.MethodGet, "/events", nil)
	rec := httptest.NewRecorder()
	sseProxy.ServeHTTP(rec, req)

	if got := rec.Header().Get("X-Accel-Buffering"); got != "no" {
		t.Errorf("expected X-Accel-Buffering %q for SSE, got %q", "no", got)
	}
	if got := rec.Header().Get("Cache-Control"); got != "no-cache" {
		t.Errorf("expected Cache-Control %q for SSE, got %q", "no-cache", got)
	}

	// Normal JSON response: should NOT gain the streaming headers.
	jsonBackend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer jsonBackend.Close()

	jsonURL, _ := url.Parse(jsonBackend.URL)
	jsonProxy := NewReverseProxy(ProxyConfig{Target: jsonURL, Timeout: 5 * time.Second})

	req2 := httptest.NewRequest(http.MethodGet, "/data", nil)
	rec2 := httptest.NewRecorder()
	jsonProxy.ServeHTTP(rec2, req2)

	if got := rec2.Header().Get("X-Accel-Buffering"); got != "" {
		t.Errorf("expected no X-Accel-Buffering for JSON, got %q", got)
	}
	if got := rec2.Header().Get("Cache-Control"); got != "" {
		t.Errorf("expected no Cache-Control override for JSON, got %q", got)
	}
}
