package server

import (
	"mime"
	"testing"
)

// TestCanonicalMIMETypesRegistered verifies that registerCanonicalMIMETypes
// (invoked from init) seeded the stdlib mime table with radix's canonical
// web-MIME values, so mime.TypeByExtension — and therefore http.FileServer —
// returns correct, portable Content-Types regardless of any host OS registry
// overrides. This is the binary-wide fix for issue #68.
func TestCanonicalMIMETypesRegistered(t *testing.T) {
	t.Parallel()

	tests := []struct {
		ext    string
		wantCT string
	}{
		{".css", "text/css; charset=utf-8"},
		{".js", "text/javascript; charset=utf-8"},
		{".mjs", "text/javascript; charset=utf-8"},
		{".svg", "image/svg+xml"},
		{".json", "application/json"},
		{".woff2", "font/woff2"},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.ext, func(t *testing.T) {
			t.Parallel()
			got := mime.TypeByExtension(tt.ext)
			if got != tt.wantCT {
				t.Errorf("mime.TypeByExtension(%q) = %q, want %q", tt.ext, got, tt.wantCT)
			}
		})
	}
}
