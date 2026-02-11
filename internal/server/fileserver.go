// Package server provides HTTP server implementations for radix.
package server

import (
	"net/http"
	"os"
	"path"
	"strings"
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
func NewFileServer(cfg FileServerConfig) http.Handler {
	fs := &spaFileSystem{
		root:  http.Dir(cfg.Dir),
		index: cfg.Index,
		spa:   cfg.SPA,
	}
	return http.FileServer(fs)
}

// spaFileSystem implements http.FileSystem with SPA fallback support.
type spaFileSystem struct {
	root  http.FileSystem
	index string
	spa   bool
}

// Open implements http.FileSystem. It opens the requested path, falling back
// to the root index file in SPA mode when the path does not exist.
func (fs *spaFileSystem) Open(name string) (http.File, error) {
	// Clean the path to prevent directory traversal
	name = path.Clean("/" + name)

	f, err := fs.root.Open(name)
	if err != nil {
		if os.IsNotExist(err) && fs.spa && !isStaticAsset(name) {
			// SPA fallback: serve the root index file for non-asset paths
			return fs.root.Open("/" + fs.index)
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
