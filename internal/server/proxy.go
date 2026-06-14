package server

import (
	"crypto/tls"
	"fmt"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
	"time"

	"github.com/osuritz/radix/internal/server/middleware"
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
	// setting. A negative value additionally flushes large non-streaming
	// (known Content-Length) bodies after every copied write chunk.
	FlushInterval time.Duration
}

// NewReverseProxy creates an http.Handler that proxies requests to the target URL.
//
// It supports path prefix stripping, path rewriting, custom backend TLS
// configuration, and returns 502 Bad Gateway on proxy errors.
//
// It sets the standard X-Forwarded-For / X-Forwarded-Host / X-Forwarded-Proto
// headers from the inbound request and strips any client-supplied forwarding
// headers first, so spoofed values are never passed through to the backend
// (this proxy does not trust client-provided X-Forwarded-* values).
func NewReverseProxy(cfg ProxyConfig) http.Handler {
	proxy := &httputil.ReverseProxy{
		// Rewrite is the stdlib-recommended replacement for Director. The
		// ReverseProxy deletes any inbound X-Forwarded-* headers before Rewrite
		// runs, so SetXForwarded() below always emits fresh, trustworthy values
		// derived from the actual inbound connection (not client-spoofable).
		Rewrite: func(pr *httputil.ProxyRequest) {
			// Annotate the access log with the upstream identity (the target
			// host, e.g. "localhost:3000" or "users:8080"), if the logging
			// middleware seeded an annotation on this request. Nil-safe: the
			// annotation is absent when logging is not installed.
			if a := middleware.LogAnnotationFromContext(pr.In.Context()); a != nil {
				a.Kind = "proxy"
				a.Target = cfg.Target.Host
			}

			// SetURL routes to the single-host target: it sets the outbound
			// scheme/host and joins the target path with the request path
			// (the Rewrite-equivalent of NewSingleHostReverseProxy).
			pr.SetURL(cfg.Target)

			// Strip prefix
			if cfg.StripPrefix != "" {
				pr.Out.URL.Path = strings.TrimPrefix(pr.Out.URL.Path, cfg.StripPrefix)
				if pr.Out.URL.Path == "" {
					pr.Out.URL.Path = "/"
				}
				pr.Out.URL.RawPath = strings.TrimPrefix(pr.Out.URL.RawPath, cfg.StripPrefix)
			}

			// Path rewrite ("from:to" format)
			if cfg.Rewrite != "" {
				from, to, ok := strings.Cut(cfg.Rewrite, ":")
				if ok {
					pr.Out.URL.Path = strings.Replace(pr.Out.URL.Path, from, to, 1)
				}
			}

			// Set X-Forwarded-For/-Host/-Proto from the inbound request. The
			// ReverseProxy already removed any client-provided forwarding
			// headers, so this strips spoofed values and sets fresh ones.
			pr.SetXForwarded()

			// Preserve sending the backend's Host for single-host routing.
			pr.Out.Host = cfg.Target.Host
		},
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
