package middleware

import (
	"net/http"
	"time"

	"github.com/osuritz/radix/internal/metrics"
)

// metricsResponseWriter wraps http.ResponseWriter to capture response metrics
type metricsResponseWriter struct {
	http.ResponseWriter
	status       int
	bytesWritten int64
}

func (mrw *metricsResponseWriter) WriteHeader(status int) {
	mrw.status = status
	mrw.ResponseWriter.WriteHeader(status)
}

func (mrw *metricsResponseWriter) Write(b []byte) (int, error) {
	if mrw.status == 0 {
		mrw.status = http.StatusOK
	}
	n, err := mrw.ResponseWriter.Write(b)
	mrw.bytesWritten += int64(n)
	return n, err
}

// Flush forwards to the underlying ResponseWriter's Flush when it supports
// http.Flusher, keeping the wrapper transparent to streaming handlers (e.g. the
// SSE mock route). It is a no-op when the underlying writer is not flushable.
//
// A flush triggers an implicit HTTP 200 to the client if no header was written
// yet, so default the recorded status to 200 first — mirroring Write — so a
// flush-first response is recorded as 200 rather than 0.
func (mrw *metricsResponseWriter) Flush() {
	if mrw.status == 0 {
		mrw.status = http.StatusOK
	}
	if f, ok := mrw.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

// Unwrap exposes the wrapped ResponseWriter so http.NewResponseController (and
// any other chain-walker) can reach the underlying writer's capabilities.
func (mrw *metricsResponseWriter) Unwrap() http.ResponseWriter {
	return mrw.ResponseWriter
}

// Metrics returns middleware that collects HTTP metrics
func Metrics(collector *metrics.Collector) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()

			// Wrap response writer to capture metrics
			mrw := &metricsResponseWriter{
				ResponseWriter: w,
				status:         0,
				bytesWritten:   0,
			}

			// Get request body size (approximation from Content-Length header)
			var bytesIn int64
			if r.ContentLength > 0 {
				bytesIn = r.ContentLength
			}

			// Process request
			next.ServeHTTP(mrw, r)

			// Record metrics
			duration := time.Since(start)
			collector.RecordRequest(
				mrw.status,
				r.Method,
				duration,
				bytesIn,
				mrw.bytesWritten,
			)
		})
	}
}
