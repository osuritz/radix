package server

import (
	"net"
	"net/http"
	"strconv"
)

// RedirectToHTTPS returns an http.Handler that permanently redirects every
// incoming (plain HTTP) request to its HTTPS equivalent on the given port.
//
// The target host is derived from the request's Host header (any existing port
// is stripped) and recombined with httpsPort. The original request path and
// raw query string are preserved. A 308 Permanent Redirect is issued so that
// the request method and body are preserved by conforming clients.
func RedirectToHTTPS(httpsPort int) http.Handler {
	portStr := strconv.Itoa(httpsPort)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		host := r.Host
		// Strip any existing port from the Host header; SplitHostPort errors
		// when no port is present, in which case the whole value is the host.
		if h, _, err := net.SplitHostPort(host); err == nil {
			host = h
		}

		target := "https://" + net.JoinHostPort(host, portStr) + r.URL.RequestURI()
		http.Redirect(w, r, target, http.StatusPermanentRedirect)
	})
}
