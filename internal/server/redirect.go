package server

import (
	"net"
	"net/http"
	"strconv"
	"strings"
)

// RedirectToHTTPS returns an http.Handler that permanently redirects every
// incoming (plain HTTP) request to its HTTPS equivalent on the given port.
//
// The target host is derived from the request's Host header (any existing port
// is stripped) and recombined with httpsPort. The original request path and
// raw query string are preserved. A 308 Permanent Redirect is issued so that
// the request method and body are preserved by conforming clients.
//
// If the request has no Host header, an absolute redirect URL cannot be built,
// so the handler responds with 400 Bad Request.
func RedirectToHTTPS(httpsPort int) http.Handler {
	portStr := strconv.Itoa(httpsPort)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Host == "" {
			http.Error(w, "400 Bad Request: missing Host header", http.StatusBadRequest)
			return
		}

		host := r.Host
		if h, _, err := net.SplitHostPort(host); err == nil {
			// Host already carried a port (e.g. "example.com:8080" or
			// "[::1]:8080"); SplitHostPort returns the bare host.
			host = h
		} else {
			// No port present. The host may still be a bracketed IPv6 literal
			// (e.g. "[::1]"); strip the brackets so JoinHostPort can re-add
			// them, otherwise we'd produce "[[::1]]:port".
			host = strings.Trim(host, "[]")
		}

		target := "https://" + net.JoinHostPort(host, portStr) + r.URL.RequestURI()
		http.Redirect(w, r, target, http.StatusPermanentRedirect)
	})
}
