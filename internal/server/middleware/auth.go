package middleware

import (
	"context"
	"net/http"
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

// InjectHeaders returns middleware that uses a HeaderProvider to inject headers into requests.
// On provider error, returns 502 Bad Gateway.
func InjectHeaders(provider HeaderProvider) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			hdrs, err := provider.Headers(r.Context(), r)
			if err != nil {
				http.Error(w, "auth provider error", http.StatusBadGateway)
				return
			}
			for key, vals := range hdrs {
				r.Header.Del(key)
				for _, v := range vals {
					r.Header.Add(key, v)
				}
			}
			next.ServeHTTP(w, r)
		})
	}
}
