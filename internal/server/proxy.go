package server

import (
	"crypto/tls"
	"fmt"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
	"time"
)

// ProxyConfig holds configuration for the reverse proxy handler.
type ProxyConfig struct {
	Target      *url.URL      // Backend target URL
	Timeout     time.Duration // Response header timeout for backend
	StripPrefix string        // Path prefix to strip before forwarding
	Rewrite     string        // Path rewrite rule in "from:to" format
	TLSConfig   *tls.Config   // TLS config for backend connections (nil for plain HTTP)

	// FlushInterval controls how often the proxy flushes buffered response data
	// to the client while copying the backend response body. A value of -1
	// flushes immediately after each write (best for streaming dev proxies);
	// 0 keeps Go's default buffered behavior; a positive value flushes
	// periodically at that interval. Note that Server-Sent Events
	// (text/event-stream) and chunked / unknown-length responses are always
	// flushed immediately by Go's httputil.ReverseProxy regardless of this
	// setting.
	FlushInterval time.Duration
}

// NewReverseProxy creates an http.Handler that proxies requests to the target URL.
//
// It supports path prefix stripping, path rewriting, custom backend TLS
// configuration, and returns 502 Bad Gateway on proxy errors.
func NewReverseProxy(cfg ProxyConfig) http.Handler {
	proxy := httputil.NewSingleHostReverseProxy(cfg.Target)

	// Custom director for path rewriting
	originalDirector := proxy.Director
	proxy.Director = func(req *http.Request) {
		// Capture the original client-facing values before the default
		// director mutates the request to point at the backend. The Director
		// runs after the request has been received, so the inbound req.Host
		// and req.TLS are still the client's values here.
		originalHost := req.Host
		proto := "http"
		if req.TLS != nil {
			proto = "https"
		}

		originalDirector(req)

		// Strip prefix
		if cfg.StripPrefix != "" {
			req.URL.Path = strings.TrimPrefix(req.URL.Path, cfg.StripPrefix)
			if req.URL.Path == "" {
				req.URL.Path = "/"
			}
			req.URL.RawPath = strings.TrimPrefix(req.URL.RawPath, cfg.StripPrefix)
		}

		// Path rewrite ("from:to" format)
		if cfg.Rewrite != "" {
			from, to, ok := strings.Cut(cfg.Rewrite, ":")
			if ok {
				req.URL.Path = strings.Replace(req.URL.Path, from, to, 1)
			}
		}

		// X-Forwarded-Host / X-Forwarded-Proto describe the original client
		// request. X-Forwarded-For is intentionally left to Go's
		// httputil.ReverseProxy, which appends the client IP (derived from
		// req.RemoteAddr) to any existing value after this Director runs.
		if originalHost != "" {
			req.Header.Set("X-Forwarded-Host", originalHost)
		}
		req.Header.Set("X-Forwarded-Proto", proto)

		// Preserve Host header for the backend
		req.Host = cfg.Target.Host
	}

	// Flush interval for streaming responses. -1 flushes immediately after
	// every write; 0 leaves the default; >0 flushes periodically.
	proxy.FlushInterval = cfg.FlushInterval

	// ModifyResponse ensures streaming responses are not buffered by an
	// upstream reverse proxy (e.g. nginx) sitting in front of radix.
	proxy.ModifyResponse = func(resp *http.Response) error {
		if isStreamingContentType(resp.Header.Get("Content-Type")) {
			if resp.Header.Get("X-Accel-Buffering") == "" {
				resp.Header.Set("X-Accel-Buffering", "no")
			}
			if resp.Header.Get("Cache-Control") == "" {
				resp.Header.Set("Cache-Control", "no-cache")
			}
		}
		return nil
	}

	// Custom transport with timeout and TLS
	transport := &http.Transport{
		ResponseHeaderTimeout: cfg.Timeout,
		TLSClientConfig:       cfg.TLSConfig, //#nosec G402 - TLS config is user-controlled
	}
	proxy.Transport = transport

	// Custom error handler
	proxy.ErrorHandler = func(w http.ResponseWriter, _ *http.Request, err error) {
		http.Error(w, fmt.Sprintf("proxy error: %v", err), http.StatusBadGateway)
	}

	return proxy
}

// isStreamingContentType reports whether the given Content-Type indicates a
// streaming response (SSE, newline-delimited JSON, or JSON streams) that
// should be flushed to the client incrementally and not buffered upstream.
func isStreamingContentType(ct string) bool {
	if ct == "" {
		return false
	}
	// Use only the base media type, ignoring any parameters (e.g. charset).
	base := ct
	if i := strings.IndexByte(base, ';'); i >= 0 {
		base = base[:i]
	}
	base = strings.ToLower(strings.TrimSpace(base))

	switch base {
	case "text/event-stream", "application/x-ndjson", "application/stream+json":
		return true
	default:
		return false
	}
}
