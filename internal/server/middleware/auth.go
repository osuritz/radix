package middleware

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
)

// HeaderProvider supplies headers to inject into outbound proxy requests.
// Implementations handle credential fetching, caching, and refresh.
// Called per-request — implementations should cache internally.
type HeaderProvider interface {
	// Headers returns additional headers to set on the request.
	// The request is provided for context (e.g., to vary headers by path).
	Headers(ctx context.Context, req *http.Request) (http.Header, error)

	// Name returns a human-readable name for logging/metrics (e.g., "okta", "static").
	Name() string
}

// AuthMetricsRecorder records the proxy command's auth-injection counter. It is
// satisfied by *metrics.Collector; the middleware depends on this narrow
// interface rather than the metrics package so it stays decoupled. The method
// must be safe to call on a nil receiver (the concrete collector's is).
type AuthMetricsRecorder interface {
	RecordProxyAuthInjection()
}

// injectOptions holds optional behavior for InjectHeaders.
type injectOptions struct {
	logw    io.Writer           // when non-nil, a redacted injection summary is written here
	metrics AuthMetricsRecorder // when non-nil, counts auth-header injections
}

// InjectOption configures InjectHeaders.
type InjectOption func(*injectOptions)

// WithVerboseLogging makes the middleware write a redacted, names-only summary
// of each injection (and any provider error) to w. Header values are never
// written — only header names and the provider name — so secrets cannot leak
// into logs. Passing a nil writer disables logging.
func WithVerboseLogging(w io.Writer) InjectOption {
	return func(o *injectOptions) { o.logw = w }
}

// WithMetrics makes the middleware record a per-command auth-injection counter
// on rec for every request that has at least one header injected. Passing a nil
// recorder disables counting; the recorder's method is also nil-safe.
func WithMetrics(rec AuthMetricsRecorder) InjectOption {
	return func(o *injectOptions) { o.metrics = rec }
}

// InjectHeaders returns middleware that uses a HeaderProvider to inject headers into requests.
// On provider error, returns 502 Bad Gateway.
func InjectHeaders(provider HeaderProvider, opts ...InjectOption) func(http.Handler) http.Handler {
	var o injectOptions
	for _, apply := range opts {
		apply(&o)
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			hdrs, err := provider.Headers(r.Context(), r)
			if err != nil {
				// Fail loud (caveat: never silently proxy without required
				// credentials), but never echo the provider error to the
				// client — it may reference credential sources. Server-side
				// logging is safe: provider/resolver errors carry only source
				// names, never secret values.
				if o.logw != nil {
					fmt.Fprintf(o.logw, "auth: provider %q error: %v\n", provider.Name(), err)
				}
				http.Error(w, "auth provider error", http.StatusBadGateway)
				return
			}
			for key, vals := range hdrs {
				r.Header.Del(key)
				for _, v := range vals {
					r.Header.Add(key, v)
				}
			}
			if len(hdrs) > 0 {
				if o.metrics != nil {
					o.metrics.RecordProxyAuthInjection()
				}
				if o.logw != nil {
					fmt.Fprintf(o.logw, "auth: injected %s via %q\n", redactedHeaderList(hdrs), provider.Name())
				}
			}
			next.ServeHTTP(w, r)
		})
	}
}

// redactedHeaderList returns a stable summary of header names only. Values are
// deliberately never included so injected credentials cannot leak into logs.
func redactedHeaderList(h http.Header) string {
	names := make([]string, 0, len(h))
	for k := range h {
		names = append(names, k)
	}
	sort.Strings(names)
	return "[" + strings.Join(names, " ") + "]"
}
