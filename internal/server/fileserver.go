// Package server provides HTTP server implementations for radix.
package server

import (
	"net/http"
	"os"
	"path"
	"strings"

	"github.com/osuritz/radix/internal/server/middleware"
)

// FileServerConfig holds configuration for the static file server.
type FileServerConfig struct {
	Dir   string // Root directory to serve
	Index string // Index file name (e.g. "index.html")
	SPA   bool   // Single Page Application mode: serve index for missing files
}

// NewFileServer creates an http.Handler that serves static files.
//
// It wraps http.FileServer with a custom filesystem that supports SPA mode
// (returning the index file for paths that don't match a real file) and
// configurable index files.
//
// In SPA mode the handler annotates the access log with Target="fallback" on
// exactly those requests that are served the index file because the requested
// path did not exist — and only after the index file actually opens, so a miss
// whose index is itself missing/unreadable (a real 404/error) is never
// mislabelled as a fallback. Plain static-asset and real-file hits get no
// target. The fallback decision is made deep inside http.FileServer (which does
// not thread the request context to FileSystem.Open), so a fresh per-request
// spaFileSystem is constructed to capture the annotation pointer — this keeps
// the fallback signal exact (it mirrors the real Open branch) rather than
// re-deriving it in the handler. Constructing the wrapper is cheap (it only
// wraps the shared http.Dir root).
func NewFileServer(cfg FileServerConfig) http.Handler {
	root := http.Dir(cfg.Dir)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fs := &spaFileSystem{
			root:       root,
			index:      cfg.Index,
			spa:        cfg.SPA,
			annotation: middleware.LogAnnotationFromContext(r.Context()),
		}
		http.FileServer(fs).ServeHTTP(w, r)
	})
}

// spaFileSystem implements http.FileSystem with SPA fallback support. It is
// constructed per request so the optional annotation pointer is request-scoped;
// no field is shared across concurrent requests.
type spaFileSystem struct {
	root  http.FileSystem
	index string
	spa   bool
	// annotation, when non-nil, is the access-log annotation for the current
	// request; it is set to Target="fallback" only when the SPA fallback fires.
	annotation *middleware.LogAnnotation
}

// Open implements http.FileSystem. It opens the requested path, falling back
// to the root index file in SPA mode when the path does not exist.
func (fs *spaFileSystem) Open(name string) (http.File, error) {
	// Clean the path to prevent directory traversal
	name = path.Clean("/" + name)

	f, err := fs.root.Open(name)
	if err != nil {
		if os.IsNotExist(err) && fs.spa && !isStaticAsset(name) {
			// SPA fallback: serve the root index file for non-asset paths.
			indexFile, indexErr := fs.root.Open("/" + fs.index)
			if indexErr != nil {
				// The index itself is missing/unreadable: this request still
				// 404s (or errors), so it was NOT served the fallback. Do not
				// annotate — labelling it "→ fallback" would mislabel the line.
				return nil, indexErr
			}
			// Annotate the access log (nil-safe) only after the fallback index
			// actually opened, so the dev format shows "→ fallback" exactly when
			// the fallback was served.
			if fs.annotation != nil {
				fs.annotation.Kind = "fileserver"
				fs.annotation.Target = "fallback"
			}
			return indexFile, nil
		}
		return nil, err
	}

	return f, nil
}

// isStaticAsset returns true if the path looks like a static asset request
// (has a file extension), which should 404 normally rather than falling back
// to the SPA index.
func isStaticAsset(name string) bool {
	ext := path.Ext(name)
	if ext == "" {
		return false
	}
	// Common static asset extensions that should not SPA-fallback
	switch strings.ToLower(ext) {
	case ".js", ".css", ".png", ".jpg", ".jpeg", ".gif", ".svg", ".ico",
		".woff", ".woff2", ".ttf", ".eot", ".map", ".json", ".xml",
		".webp", ".avif", ".mp4", ".webm", ".pdf":
		return true
	}
	return false
}
