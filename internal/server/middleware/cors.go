package middleware

import "net/http"

// CORS returns middleware that sets permissive CORS headers and handles preflight requests.
func CORS() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Access-Control-Allow-Origin", "*")
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, PATCH, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, X-Requested-With")

			if r.Method == http.MethodOptions {
				w.WriteHeader(http.StatusNoContent)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

// MetricsCORS returns middleware that adds permissive, read-only CORS headers so
// a browser app served from another origin (e.g. the Vite dev server on :5173)
// can fetch the metrics endpoint directly during local development.
//
// It differs from CORS in that the allowed methods are scoped to GET and OPTIONS,
// matching the read-only surface of the metrics endpoint. Preflight OPTIONS
// requests receive 204. No Vary: Origin header is emitted: it is contradictory
// with a wildcard Access-Control-Allow-Origin and would only bloat caches.
func MetricsCORS() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Access-Control-Allow-Origin", "*")
			w.Header().Set("Access-Control-Allow-Methods", "GET, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type")

			if r.Method == http.MethodOptions {
				w.WriteHeader(http.StatusNoContent)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}
