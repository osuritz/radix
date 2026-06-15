package middleware

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/osuritz/radix/internal/metrics"
)

// flushRecorder is a minimal http.ResponseWriter + http.Flusher that records
// whether Flush() was called. It lets a test assert that a wrapper's Flush()
// propagates to the underlying writer (httptest.ResponseRecorder also satisfies
// http.Flusher, but it does not expose whether Flush was invoked).
type flushRecorder struct {
	http.ResponseWriter
	flushed bool
}

func newFlushRecorder() *flushRecorder {
	return &flushRecorder{ResponseWriter: httptest.NewRecorder()}
}

func (f *flushRecorder) Flush() { f.flushed = true }

// TestWrappersAreFlushTransparent asserts that each middleware response-writer
// wrapper satisfies http.Flusher and that Flush() propagates to the underlying
// (flushable) writer. Without this, streaming handlers (e.g. the SSE mock route)
// that type-assert the writer to http.Flusher fail through the middleware chain.
func TestWrappersAreFlushTransparent(t *testing.T) {
	tests := []struct {
		name string
		wrap func(http.ResponseWriter) http.ResponseWriter
	}{
		{
			name: "logging responseWriter",
			wrap: func(w http.ResponseWriter) http.ResponseWriter {
				return &responseWriter{ResponseWriter: w}
			},
		},
		{
			name: "metrics metricsResponseWriter",
			wrap: func(w http.ResponseWriter) http.ResponseWriter {
				return &metricsResponseWriter{ResponseWriter: w}
			},
		},
		{
			name: "gzip gzipResponseWriter",
			wrap: func(w http.ResponseWriter) http.ResponseWriter {
				// writer is the same underlying writer here; the gzip wrapper's
				// Flush must reach the underlying Flusher (after flushing gzip).
				return &gzipResponseWriter{ResponseWriter: w, writer: w}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			under := newFlushRecorder()
			wrapped := tt.wrap(under)

			flusher, ok := wrapped.(http.Flusher)
			if !ok {
				t.Fatalf("%s does not implement http.Flusher", tt.name)
			}
			flusher.Flush()
			if !under.flushed {
				t.Errorf("%s.Flush() did not propagate to the underlying writer", tt.name)
			}
		})
	}
}

// TestWrappersUnwrap asserts that each wrapper exposes Unwrap() so
// http.NewResponseController (and any other chain-walker) can traverse to the
// underlying writer.
func TestWrappersUnwrap(t *testing.T) {
	under := httptest.NewRecorder()

	tests := []struct {
		name    string
		wrapped interface{ Unwrap() http.ResponseWriter }
	}{
		{"logging responseWriter", &responseWriter{ResponseWriter: under}},
		{"metrics metricsResponseWriter", &metricsResponseWriter{ResponseWriter: under}},
		{"gzip gzipResponseWriter", &gzipResponseWriter{ResponseWriter: under, writer: under}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.wrapped.Unwrap(); got != http.ResponseWriter(under) {
				t.Errorf("%s.Unwrap() = %v, want the underlying recorder", tt.name, got)
			}
		})
	}
}

// TestSSEThroughMiddlewareChain exercises a streaming (Flusher-dependent)
// handler through the real middleware chain the `mock` command builds: Metrics
// wraps Logging wraps the handler (mock.go applies Logging first, then Metrics,
// making Metrics the outer wrapper). Before the Flush/Unwrap passthrough fix,
// the handler's w.(http.Flusher) assertion failed (the outer wrapper shadowed
// the underlying Flusher) and it returned 500 instead of a stream.
func TestSSEThroughMiddlewareChain(t *testing.T) {
	// A handler that requires an http.Flusher to stream a text/event-stream,
	// mirroring server.serveSSE.
	stream := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		flusher, ok := w.(http.Flusher)
		if !ok {
			http.Error(w, "streaming unsupported", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		flusher.Flush()
		_, _ = w.Write([]byte("data: hello\n\n"))
		flusher.Flush()
	})

	collector := metrics.NewCollector("test", "1.0.0")
	// Wrap exactly as the mock command does: Metrics (outer) wraps Logging
	// (inner) wraps the handler.
	handler := Metrics(collector)(
		Logging(LoggingConfig{Format: LogFormatDev, Output: io.Discard})(stream),
	)

	srv := httptest.NewServer(handler)
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/api/stream/42")
	if err != nil {
		t.Fatalf("GET failed: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200 (SSE returned an error through the chain)", resp.StatusCode)
	}
	if ct := resp.Header.Get("Content-Type"); !strings.HasPrefix(ct, "text/event-stream") {
		t.Fatalf("Content-Type = %q, want text/event-stream", ct)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	if !strings.Contains(string(body), "data: hello") {
		t.Fatalf("body = %q, want a data: line", string(body))
	}
}
