package middleware

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/osuritz/radix/internal/metrics"
)

// newBenchStack chains the production middleware stack (logging → metrics →
// gzip) over a trivial handler, discarding log output so the benchmark measures
// middleware overhead, not terminal I/O.
func newBenchStack(b *testing.B) http.Handler {
	b.Helper()
	base := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		_, _ = w.Write([]byte("OK"))
	})
	collector := metrics.NewCollector("bench", "test")
	logging := Logging(LoggingConfig{Format: LogFormatDev, NoColor: true, Output: io.Discard})
	return logging(Metrics(collector)(Gzip()(base)))
}

// benchmarkStack drives the chained stack with GET requests, optionally
// advertising gzip support, and fails on any non-200 response.
func benchmarkStack(b *testing.B, acceptGzip bool) {
	b.Helper()
	handler := newBenchStack(b)
	b.ReportAllocs()
	for b.Loop() {
		req := httptest.NewRequest(http.MethodGet, "/bench", nil)
		if acceptGzip {
			req.Header.Set("Accept-Encoding", "gzip")
		}
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			b.Fatalf("status = %d, want 200", rec.Code)
		}
	}
}

func BenchmarkMiddlewareStack(b *testing.B) {
	benchmarkStack(b, false)
}

func BenchmarkMiddlewareStack_GzipAccepted(b *testing.B) {
	benchmarkStack(b, true)
}
