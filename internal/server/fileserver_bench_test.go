package server

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/osuritz/radix/internal/server/middleware"
)

// newBenchFileServerDir writes the benchmark fixture files (a small HTML file
// and a ~1MB compressible text file) into a temp dir and returns its path.
// The large file repeats a text line so the gzip variant measures realistic
// compression work rather than incompressible passthrough.
func newBenchFileServerDir(b *testing.B) string {
	b.Helper()
	dir := b.TempDir()

	small := []byte("<html><body>hello, radix</body></html>\n")
	if err := os.WriteFile(filepath.Join(dir, "small.html"), small, 0o600); err != nil {
		b.Fatalf("write small file: %v", err)
	}

	line := []byte("the quick brown fox jumps over the lazy dog 0123456789\n")
	large := bytes.Repeat(line, (1<<20)/len(line)+1) // ~1MB
	if err := os.WriteFile(filepath.Join(dir, "large.txt"), large, 0o600); err != nil {
		b.Fatalf("write large file: %v", err)
	}

	return dir
}

// benchmarkFileServer drives handler with GET requests for path, optionally
// advertising gzip support, and fails on any non-200 response.
func benchmarkFileServer(b *testing.B, handler http.Handler, path string, acceptGzip bool) {
	b.Helper()
	b.ReportAllocs()
	for b.Loop() {
		req := httptest.NewRequest(http.MethodGet, path, nil)
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

func BenchmarkFileServer_SmallFile(b *testing.B) {
	dir := newBenchFileServerDir(b)
	handler := NewFileServer(FileServerConfig{Dir: dir, Index: "index.html"})
	benchmarkFileServer(b, handler, "/small.html", false)
}

func BenchmarkFileServer_LargeFile(b *testing.B) {
	dir := newBenchFileServerDir(b)
	handler := NewFileServer(FileServerConfig{Dir: dir, Index: "index.html"})
	benchmarkFileServer(b, handler, "/large.txt", false)
}

func BenchmarkFileServer_LargeFileGzip(b *testing.B) {
	dir := newBenchFileServerDir(b)
	handler := middleware.Gzip()(NewFileServer(FileServerConfig{Dir: dir, Index: "index.html"}))
	benchmarkFileServer(b, handler, "/large.txt", true)
}
