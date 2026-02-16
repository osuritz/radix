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

		// Preserve Host header for the backend
		req.Host = cfg.Target.Host
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
