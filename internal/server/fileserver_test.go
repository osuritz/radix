package server_test

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/osuritz/radix/internal/server"
	"github.com/osuritz/radix/internal/server/middleware"
)

func setupTestDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()

	// Create test files
	if err := os.WriteFile(filepath.Join(dir, "index.html"), []byte("<html>home</html>"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "style.css"), []byte("body{}"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Create a subdirectory with its own index
	subdir := filepath.Join(dir, "sub")
	if err := os.Mkdir(subdir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(subdir, "index.html"), []byte("<html>sub</html>"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Create a subdirectory without index (for directory listing)
	noindex := filepath.Join(dir, "noindex")
	if err := os.Mkdir(noindex, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(noindex, "file.txt"), []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}

	return dir
}

func TestFileServer_ServesFiles(t *testing.T) {
	dir := setupTestDir(t)
	handler := server.NewFileServer(server.FileServerConfig{
		Dir:   dir,
		Index: "index.html",
	})

	tests := []struct {
		name       string
		path       string
		wantStatus int
		wantBody   string
	}{
		{
			name:       "root index",
			path:       "/",
			wantStatus: http.StatusOK,
			wantBody:   "<html>home</html>",
		},
		{
			name:       "css file",
			path:       "/style.css",
			wantStatus: http.StatusOK,
			wantBody:   "body{}",
		},
		{
			name:       "subdirectory index",
			path:       "/sub/",
			wantStatus: http.StatusOK,
			wantBody:   "<html>sub</html>",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, tt.path, nil)
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)

			if rec.Code != tt.wantStatus {
				t.Errorf("status = %d, want %d", rec.Code, tt.wantStatus)
			}
			if tt.wantBody != "" && rec.Body.String() != tt.wantBody {
				t.Errorf("body = %q, want %q", rec.Body.String(), tt.wantBody)
			}
		})
	}
}

func TestFileServer_NotFoundWithoutSPA(t *testing.T) {
	dir := setupTestDir(t)
	handler := server.NewFileServer(server.FileServerConfig{
		Dir:   dir,
		Index: "index.html",
		SPA:   false,
	})

	req := httptest.NewRequest(http.MethodGet, "/nonexistent", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

func TestFileServer_SPAFallback(t *testing.T) {
	dir := setupTestDir(t)
	handler := server.NewFileServer(server.FileServerConfig{
		Dir:   dir,
		Index: "index.html",
		SPA:   true,
	})

	// Non-existent path should return index.html in SPA mode
	req := httptest.NewRequest(http.MethodGet, "/about", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if got := rec.Body.String(); got != "<html>home</html>" {
		t.Errorf("body = %q, want root index", got)
	}
}

func TestFileServer_SPADoesNotFallbackForAssets(t *testing.T) {
	dir := setupTestDir(t)
	handler := server.NewFileServer(server.FileServerConfig{
		Dir:   dir,
		Index: "index.html",
		SPA:   true,
	})

	// Missing .js file should still 404 even in SPA mode
	req := httptest.NewRequest(http.MethodGet, "/missing.js", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

// devLog wraps h in the dev logging middleware writing to buf, with color
// auto-disabled (bytes.Buffer is a non-TTY). It centralizes the env hygiene the
// color resolver depends on.
func devLog(t *testing.T, buf *bytes.Buffer, h http.Handler) http.Handler {
	t.Helper()
	t.Setenv("NO_COLOR", "")
	t.Setenv("FORCE_COLOR", "")
	t.Setenv("CLICOLOR_FORCE", "")
	return middleware.Logging(middleware.LoggingConfig{
		Format: middleware.LogFormatDev,
		Output: buf,
	})(h)
}

// TestFileServer_SPAFallbackAnnotatesTarget verifies that an SPA index fallback
// records Target="fallback", so the dev access log shows a "→ fallback" column.
func TestFileServer_SPAFallbackAnnotatesTarget(t *testing.T) {
	dir := setupTestDir(t)
	handler := server.NewFileServer(server.FileServerConfig{
		Dir:   dir,
		Index: "index.html",
		SPA:   true,
	})

	var buf bytes.Buffer
	logged := devLog(t, &buf, handler)

	req := httptest.NewRequest(http.MethodGet, "/about", nil)
	logged.ServeHTTP(httptest.NewRecorder(), req)

	if out := buf.String(); !strings.Contains(out, "→ fallback") {
		t.Errorf("SPA fallback dev log should show \"→ fallback\": %q", out)
	}
}

// TestFileServer_PlainAssetNoTarget verifies that serving a real static asset
// adds no target column to the dev access log (no arrow).
func TestFileServer_PlainAssetNoTarget(t *testing.T) {
	dir := setupTestDir(t)
	handler := server.NewFileServer(server.FileServerConfig{
		Dir:   dir,
		Index: "index.html",
		SPA:   true,
	})

	var buf bytes.Buffer
	logged := devLog(t, &buf, handler)

	// style.css is a real file -> no fallback -> no target column.
	req := httptest.NewRequest(http.MethodGet, "/style.css", nil)
	logged.ServeHTTP(httptest.NewRecorder(), req)

	if out := buf.String(); strings.Contains(out, "→") {
		t.Errorf("plain asset hit must not show a target column: %q", out)
	}
}

// TestFileServer_RootIndexNoTarget verifies that serving the real root index
// (a normal hit, not a fallback) adds no target column.
func TestFileServer_RootIndexNoTarget(t *testing.T) {
	dir := setupTestDir(t)
	handler := server.NewFileServer(server.FileServerConfig{
		Dir:   dir,
		Index: "index.html",
		SPA:   true,
	})

	var buf bytes.Buffer
	logged := devLog(t, &buf, handler)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	logged.ServeHTTP(httptest.NewRecorder(), req)

	if out := buf.String(); strings.Contains(out, "→") {
		t.Errorf("real root index hit must not show a target column: %q", out)
	}
}

// TestFileServer_AnnotationNilSafe verifies the file server does not panic when
// no logging middleware seeded an annotation, including on the SPA fallback path.
func TestFileServer_AnnotationNilSafe(t *testing.T) {
	dir := setupTestDir(t)
	handler := server.NewFileServer(server.FileServerConfig{
		Dir:   dir,
		Index: "index.html",
		SPA:   true,
	})

	req := httptest.NewRequest(http.MethodGet, "/about", nil) // triggers fallback
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req) // no logging middleware -> no annotation
	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusOK)
	}
}

// TestFileServer_FailedFallbackNoTarget verifies that when an SPA path misses
// AND the configured index is itself missing, the request is NOT served the
// fallback (so it does not 200) and the dev access log does NOT show
// "→ fallback" — the annotation must only fire after the index actually opens.
func TestFileServer_FailedFallbackNoTarget(t *testing.T) {
	dir := t.TempDir() // deliberately empty: no index.html exists
	handler := server.NewFileServer(server.FileServerConfig{
		Dir:   dir,
		Index: "index.html",
		SPA:   true,
	})

	var buf bytes.Buffer
	logged := devLog(t, &buf, handler)

	// /about misses; SPA fallback is attempted but index.html is absent, so the
	// fallback open fails -> the response is a 404, not a served fallback.
	req := httptest.NewRequest(http.MethodGet, "/about", nil)
	rec := httptest.NewRecorder()
	logged.ServeHTTP(rec, req)

	if rec.Code == http.StatusOK {
		t.Errorf("missing fallback index must not yield 200, got %d", rec.Code)
	}
	if out := buf.String(); strings.Contains(out, "→ fallback") {
		t.Errorf("a failed fallback (missing index) must not log \"→ fallback\": %q", out)
	}
	if out := buf.String(); strings.Contains(out, "→") {
		t.Errorf("a failed fallback must not emit any target column: %q", out)
	}
}

func TestFileServer_DirectoryListing(t *testing.T) {
	dir := setupTestDir(t)
	handler := server.NewFileServer(server.FileServerConfig{
		Dir:   dir,
		Index: "index.html",
	})

	// Directory without index.html should get a listing
	req := httptest.NewRequest(http.MethodGet, "/noindex/", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	// http.FileServer generates HTML with a link to the file
	if body := rec.Body.String(); !contains(body, "file.txt") {
		t.Errorf("directory listing should contain file.txt, got: %s", body)
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && searchString(s, substr)
}

func searchString(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
