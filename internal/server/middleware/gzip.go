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
