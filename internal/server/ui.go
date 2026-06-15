package server

import (
	"fmt"
	"io"
	"io/fs"
	"mime"
	"net/http"
	"path"
	"strings"

	radix "github.com/osuritz/radix"
)

// ServeUI registers the compiled React SPA (embedded under ui/dist) onto mux at
// "/". Requests for known static assets are served directly; all other paths
// fall back to index.html so deep-links work in the SPA. When no real build is
// present (only the placeholder .gitkeep), a minimal HTML page instructs the
// developer to run `make ui`.
//
// /_metrics and /healthz are registered on the same mux before ServeUI is
// called; Go's ServeMux longest-prefix matching gives exact paths precedence
// over the "/" catch-all, so API routes are never shadowed by the SPA handler.
func ServeUI(mux *http.ServeMux) error {
	sub, err := fs.Sub(radix.UIAssets, "ui/dist")
	if err != nil {
		return fmt.Errorf("serveUI: failed to sub ui/dist from embedded FS: %w", err)
	}
	serveUIFromFS(mux, sub)
	return nil
}

// serveUIFromFS registers the SPA handler on mux at "/" using the given fsys.
// It is separated from ServeUI to allow unit tests to inject an in-memory FS.
// index.html is read once at registration time and cached in the closure to
// avoid per-request re-reads/allocations on SPA fallback.
func serveUIFromFS(mux *http.ServeMux, fsys fs.FS) {
	// Cache index.html bytes at registration time to avoid per-request re-reads.
	// If index.html is absent (placeholder-only build), indexHTML remains nil and
	// the handler falls through to the developer placeholder page.
	var indexHTML []byte
	if data, err := fs.ReadFile(fsys, "index.html"); err == nil {
		indexHTML = data
	}

	mux.Handle("/", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Clean the path to prevent traversal (fs.FS already enforces this, but
		// being explicit keeps the intent obvious).
		cleanPath := path.Clean("/" + r.URL.Path)
		// path.Clean returns "/" for the root; strip the leading slash for fs.FS.
		fsPath := cleanPath[1:]
		if fsPath == "" {
			fsPath = "."
		}

		// Check whether a regular file exists at the cleaned path.
		if fi, err := fs.Stat(fsys, fsPath); err == nil && !fi.IsDir() {
			// Serve the exact file with a proper Content-Type.
			serveStaticFile(w, r, fsys, fsPath, cleanPath, fi)
			return
		}

		// If the path looks like a static asset (has a file extension) but
		// the file was not found above, return 404 rather than serving the
		// SPA index — a missing .js/.css file should not silently return HTML.
		if isStaticAsset(cleanPath) {
			http.NotFound(w, r)
			return
		}

		// SPA fallback: serve index.html for deep-links and the root path.
		serveIndexOrPlaceholder(w, indexHTML)
	}))
}

// serveStaticFile serves a single file from fsys with the correct Content-Type.
// cleanURLPath is the URL-derived path (always forward-slash) used for MIME
// detection and cache-control decisions. fi is the already-obtained FileInfo.
func serveStaticFile(w http.ResponseWriter, r *http.Request, fsys fs.FS, fsPath, cleanURLPath string, fi fs.FileInfo) {
	// Use contentTypeForPath (URL-aware, forward-slash only) rather than
	// mime.TypeByExtension directly, which can return wrong types on Windows
	// when the registry overrides known extensions (e.g. .css → text/plain).
	ct := contentTypeForPath(cleanURLPath)
	w.Header().Set("Content-Type", ct)

	f, err := fsys.Open(fsPath)
	if err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	defer func() { _ = f.Close() }()

	// Vite content-hashes JS/CSS bundles so assets under /assets/ are immutable.
	// Cache-Control is set after a successful Open so a failed open / 404 path
	// does not emit a 1-year immutable directive.
	if strings.HasPrefix(cleanURLPath, "/assets/") {
		w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	} else if cleanURLPath == "/index.html" {
		// index.html must never be aggressively cached so app updates are picked up.
		w.Header().Set("Cache-Control", "no-cache")
	}

	// http.ServeContent handles range requests and Content-Length. Both embed.FS
	// and fstest.MapFS implement io.ReadSeeker, so the assertion below succeeds in
	// all production and test scenarios. NOTE: embed.FS ModTime is always zero, so
	// Last-Modified / 304 conditional-GET responses never occur for embedded
	// assets — Cache-Control is the effective caching mechanism here.
	if rs, ok := f.(io.ReadSeeker); ok {
		http.ServeContent(w, r, fsPath, fi.ModTime(), rs)
		return
	}

	// Minimal fallback for a hypothetical non-seekable fs.FS implementation.
	// This branch is not reached by embed.FS or fstest.MapFS.
	w.WriteHeader(http.StatusOK)
	_, _ = io.Copy(w, f)
}

// contentTypeForPath returns the canonical MIME type for the given path by
// switching on its extension. This avoids relying on mime.TypeByExtension,
// which on Windows can return incorrect types when the registry overrides known
// extensions (e.g. .css → text/plain, which breaks stylesheets and module
// scripts). For extensions not in the switch, it falls back to
// mime.TypeByExtension and finally "application/octet-stream".
func contentTypeForPath(p string) string {
	switch strings.ToLower(path.Ext(p)) {
	case ".html":
		return "text/html; charset=utf-8"
	case ".js", ".mjs":
		return "text/javascript; charset=utf-8"
	case ".css":
		return "text/css; charset=utf-8"
	case ".json", ".map":
		return "application/json"
	case ".svg":
		return "image/svg+xml"
	case ".ico":
		return "image/x-icon"
	case ".png":
		return "image/png"
	case ".webp":
		return "image/webp"
	case ".woff2":
		return "font/woff2"
	case ".woff":
		return "font/woff"
	case ".ttf":
		return "font/ttf"
	case ".txt":
		return "text/plain; charset=utf-8"
	default:
		ext := strings.ToLower(path.Ext(p))
		if ct := mime.TypeByExtension(ext); ct != "" {
			return ct
		}
		return "application/octet-stream"
	}
}

// serveIndexOrPlaceholder serves the cached index.html bytes when available,
// or a minimal developer-oriented placeholder page when the UI has not been
// built yet (indexHTML is nil).
func serveIndexOrPlaceholder(w http.ResponseWriter, indexHTML []byte) {
	if indexHTML != nil {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		// index.html must not be aggressively cached so app updates are picked up.
		w.Header().Set("Cache-Control", "no-cache")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(indexHTML)
		return
	}

	// No index.html — the binary contains only the placeholder build.
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(`<!DOCTYPE html>
<html lang="en">
<head><meta charset="utf-8"><title>Radix UI</title>
<style>body{font-family:sans-serif;display:flex;align-items:center;justify-content:center;min-height:100vh;margin:0;background:#f5f5f5}
.box{background:#fff;border-radius:8px;padding:2rem 3rem;box-shadow:0 2px 8px rgba(0,0,0,.1);text-align:center}
code{background:#f0f0f0;padding:.2em .4em;border-radius:4px;font-size:.95em}</style>
</head>
<body><div class="box">
<h2>Radix Web UI</h2>
<p>The UI has not been built yet.</p>
<p>Run <code>make ui</code> and rebuild the binary to embed the React app.</p>
</div></body></html>
`))
}

// withMetricsCORS returns an http.Handler that adds permissive CORS headers so
// the Vite dev server (typically on :5173) can fetch /_metrics directly. Only
// the metrics endpoint uses this; all other admin routes are unaffected.
//
// Allowed methods: GET, OPTIONS. Preflight OPTIONS requests receive 204.
func withMetricsCORS(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")

		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}

		h.ServeHTTP(w, r)
	})
}
