package server

import "mime"

// webContentTypes is the single canonical web-MIME table for radix. Keys are
// lowercased file extensions including the leading dot; values are the
// Content-Type that radix wants to serve for that extension, regardless of the
// host OS.
//
// Why this exists: http.FileServer (used by the static file server in
// fileserver.go) and mime.TypeByExtension both consult the stdlib mime table,
// which on Windows is seeded from the system registry. A misconfigured or
// third-party-modified registry can map well-known web extensions to the wrong
// type — most damagingly .css -> text/plain and .js -> text/plain — which makes
// browsers refuse to apply stylesheets or evaluate module scripts (strict MIME
// checking). Forcing canonical values into the stdlib table at init time makes
// radix serve correct, portable Content-Types on every platform. See issue #68.
//
// This map is the single source of truth: contentTypeForPath (ui.go) looks up
// the same table, and registerCanonicalMIMETypes pushes every entry into the
// stdlib mime table via mime.AddExtensionType so http.FileServer picks them up.
var webContentTypes = map[string]string{
	".html":  "text/html; charset=utf-8",
	".js":    "text/javascript; charset=utf-8",
	".mjs":   "text/javascript; charset=utf-8",
	".css":   "text/css; charset=utf-8",
	".json":  "application/json",
	".map":   "application/json",
	".svg":   "image/svg+xml",
	".ico":   "image/x-icon",
	".png":   "image/png",
	".webp":  "image/webp",
	".gif":   "image/gif",
	".jpg":   "image/jpeg",
	".jpeg":  "image/jpeg",
	".woff2": "font/woff2",
	".woff":  "font/woff",
	".ttf":   "font/ttf",
	".txt":   "text/plain; charset=utf-8",
}

func init() {
	registerCanonicalMIMETypes()
}

// registerCanonicalMIMETypes seeds the stdlib mime table with radix's canonical
// web-MIME values so that mime.TypeByExtension — and therefore http.FileServer —
// returns correct, portable Content-Types even when the host OS (notably the
// Windows registry) would otherwise override them. It is invoked from init() so
// every consumer of the server package benefits binary-wide with no per-call
// wiring. mime.AddExtensionType is safe to call repeatedly; the last write wins.
func registerCanonicalMIMETypes() {
	for ext, ct := range webContentTypes {
		// mime.AddExtensionType only errors if ext does not start with a dot;
		// every key in webContentTypes is dot-prefixed, so the error cannot
		// occur. Ignore it to keep init() side-effect-free and panic-free.
		_ = mime.AddExtensionType(ext, ct)
	}
}
