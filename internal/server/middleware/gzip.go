package middleware

import (
	"compress/gzip"
	"io"
	"net/http"
	"strings"
)

// gzipResponseWriter wraps http.ResponseWriter with a gzip writer.
type gzipResponseWriter struct {
	http.ResponseWriter
	writer io.Writer
}

func (grw *gzipResponseWriter) Write(b []byte) (int, error) {
	return grw.writer.Write(b)
}

// Flush flushes any buffered gzip data and then flushes the underlying
// ResponseWriter, keeping the wrapper transparent to streaming handlers. The
// gzip.Writer is flushed first so its compressed bytes reach the underlying
// writer before that writer's own Flush pushes them to the client; both steps
// are best-effort (no-ops when the writer does not support flushing).
func (grw *gzipResponseWriter) Flush() {
	if f, ok := grw.writer.(interface{ Flush() error }); ok {
		_ = f.Flush()
	}
	if f, ok := grw.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

// Unwrap exposes the wrapped ResponseWriter so http.NewResponseController (and
// any other chain-walker) can reach the underlying writer's capabilities.
func (grw *gzipResponseWriter) Unwrap() http.ResponseWriter {
	return grw.ResponseWriter
}

// Gzip returns middleware that compresses responses with gzip when the client supports it.
func Gzip() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !strings.Contains(r.Header.Get("Accept-Encoding"), "gzip") {
				next.ServeHTTP(w, r)
				return
			}

			gz, err := gzip.NewWriterLevel(w, gzip.DefaultCompression)
			if err != nil {
				next.ServeHTTP(w, r)
				return
			}
			defer func() { _ = gz.Close() }()

			w.Header().Set("Content-Encoding", "gzip")
			w.Header().Del("Content-Length")

			grw := &gzipResponseWriter{
				ResponseWriter: w,
				writer:         gz,
			}
			next.ServeHTTP(grw, r)
		})
	}
}
