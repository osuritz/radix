package server

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestRedirectToHTTPS_HostWithoutPort(t *testing.T) {
	handler := RedirectToHTTPS(8443)

	req := httptest.NewRequest(http.MethodGet, "/path/to/page?q=1&x=2", nil)
	req.Host = "example.com"
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusPermanentRedirect {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusPermanentRedirect)
	}
	want := "https://example.com:8443/path/to/page?q=1&x=2"
	if got := rec.Header().Get("Location"); got != want {
		t.Errorf("Location = %q, want %q", got, want)
	}
}

func TestRedirectToHTTPS_HostWithPort(t *testing.T) {
	handler := RedirectToHTTPS(8443)

	req := httptest.NewRequest(http.MethodGet, "/api?key=value", nil)
	req.Host = "localhost:8080"
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusPermanentRedirect {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusPermanentRedirect)
	}
	want := "https://localhost:8443/api?key=value"
	if got := rec.Header().Get("Location"); got != want {
		t.Errorf("Location = %q, want %q", got, want)
	}
}

func TestRedirectToHTTPS_PreservesMethodWithPOST(t *testing.T) {
	handler := RedirectToHTTPS(443)

	req := httptest.NewRequest(http.MethodPost, "/submit", nil)
	req.Host = "example.com"
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	// 308 Permanent Redirect preserves the request method and body, unlike 301.
	if rec.Code != http.StatusPermanentRedirect {
		t.Errorf("status = %d, want %d (method-preserving)", rec.Code, http.StatusPermanentRedirect)
	}
	want := "https://example.com:443/submit"
	if got := rec.Header().Get("Location"); got != want {
		t.Errorf("Location = %q, want %q", got, want)
	}
}

func TestRedirectToHTTPS_RootPath(t *testing.T) {
	handler := RedirectToHTTPS(8443)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Host = "example.com"
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	want := "https://example.com:8443/"
	if got := rec.Header().Get("Location"); got != want {
		t.Errorf("Location = %q, want %q", got, want)
	}
}
