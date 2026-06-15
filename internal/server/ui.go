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
func serveUIFromFS(mux *http.ServeMux, fsys fs.FS) {
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

		// SPA fallback: serve index.html for deep-links and the root path.
		serveIndexOrPlaceholder(w, fsys)
	}))
}

// serveStaticFile serves a single file from fsys with the correct Content-Type.
// cleanURLPath is the URL-derived path (always forward-slash) used for MIME
// detection and cache-control decisions. fi is the already-obtained FileInfo.
func serveStaticFile(w http.ResponseWriter, r *http.Request, fsys fs.FS, fsPath, cleanURLPath string, fi fs.FileInfo) {
	// Use path.Ext (URL-aware, forward-slash only) rather than filepath.Ext
	// which is OS-aware and mishandles backslashes on Windows.
	ct := mime.TypeByExtension(path.Ext(cleanURLPath))
	if ct == "" {
		ct = "application/octet-stream"
	}
	w.Header().Set("Content-Type", ct)

	// Vite content-hashes JS/CSS bundles so assets under /assets/ are immutable.
	if strings.HasPrefix(cleanURLPath, "/assets/") {
		w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	} else if cleanURLPath == "/index.html" || cleanURLPath == "/" {
		// index.html must never be aggressively cached so app updates are picked up.
		w.Header().Set("Cache-Control", "no-cache")
	}

	f, err := fsys.Open(fsPath)
	if err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	defer func() { _ = f.Close() }()

	// http.ServeContent handles range requests, conditional GETs (If-Modified-Since,
	// ETag), and Content-Length. embed.FS files implement io.ReadSeeker so this
	// path is always taken in production. Pass fi.ModTime() for correct
	// conditional-GET support.
	if rs, ok := f.(interface {
		fs.File
		Seek(int64, int) (int64, error)
	}); ok {
		http.ServeContent(w, r, fsPath, fi.ModTime(), rs)
		return
	}

	// Fallback for fs.FS implementations that do not satisfy io.ReadSeeker
	// (e.g. custom in-memory FS in tests). Copy directly without delegating to
	// http.FileServerFS, which could trigger unwanted redirects.
	w.WriteHeader(http.StatusOK)
	_, _ = io.Copy(w, f)
}

// serveIndexOrPlaceholder serves index.html when present, or a minimal
// developer-oriented placeholder page when the UI has not been built yet.
func serveIndexOrPlaceholder(w http.ResponseWriter, fsys fs.FS) {
	data, err := fs.ReadFile(fsys, "index.html")
	if err == nil {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		// index.html must not be aggressively cached so app updates are picked up.
		w.Header().Set("Cache-Control", "no-cache")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(data)
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
		w.Header().Add("Vary", "Origin")
		w.Header().Set("Access-Control-Allow-Methods", "GET, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")

		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}

		h.ServeHTTP(w, r)
	})
}
