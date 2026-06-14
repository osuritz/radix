package middleware

import (
	"net/http"
	"strconv"
)

// HSTS returns middleware that sets the HTTP Strict-Transport-Security header
// on every response, instructing browsers to access the site over HTTPS only
// for the given max-age (in seconds), including subdomains.
//
// HSTS is only meaningful over HTTPS; browsers ignore the header when it is
// received over a plain HTTP connection. The header is set on the response
// before the next handler writes the body.
func HSTS(maxAge int) func(http.Handler) http.Handler {
	value := "max-age=" + strconv.Itoa(maxAge) + "; includeSubDomains"
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Strict-Transport-Security", value)
			next.ServeHTTP(w, r)
		})
	}
}
