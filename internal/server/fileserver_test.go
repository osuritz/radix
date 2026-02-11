package server_test

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/osuritz/radix/internal/server"
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
