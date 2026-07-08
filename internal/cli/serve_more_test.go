package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/osuritz/radix/internal/config"
	"github.com/spf13/cobra"
)

// newServeFlagsCmd builds a fresh command carrying the serve flag set bound to
// the same package globals, so tests can mark flags Changed without mutating
// the shared serveCmd. Registering the flags resets the globals to defaults;
// callers must save/restore any globals they care about.
func newServeFlagsCmd() *cobra.Command {
	c := &cobra.Command{Use: "serve"}
	c.Flags().StringVarP(&serveDir, "dir", "d", "", "")
	c.Flags().StringVar(&serveIndex, "index", "", "")
	c.Flags().BoolVar(&serveSPA, "spa", false, "")
	c.Flags().BoolVar(&serveCORS, "cors", false, "")
	c.Flags().BoolVar(&serveGzip, "gzip", false, "")
	c.Flags().StringVar(&serveCache, "cache", "", "")
	c.Flags().BoolVar(&serveHSTS, "hsts", false, "")
	c.Flags().IntVar(&serveHSTSMaxAge, "hsts-max-age", 31536000, "")
	c.Flags().BoolVar(&serveHTTPRedirect, "http-redirect", false, "")
	c.Flags().IntVar(&serveHTTPPort, "http-port", 8080, "")
	return c
}

// restoreServeFlagGlobals snapshots all serve flag globals and restores them
// on test cleanup.
func restoreServeFlagGlobals(t *testing.T) {
	t.Helper()
	oldDir, oldIndex, oldSPA, oldCORS := serveDir, serveIndex, serveSPA, serveCORS
	oldGzip, oldCache, oldHSTS, oldMaxAge := serveGzip, serveCache, serveHSTS, serveHSTSMaxAge
	oldRedirect, oldHTTPPort := serveHTTPRedirect, serveHTTPPort
	t.Cleanup(func() {
		serveDir, serveIndex, serveSPA, serveCORS = oldDir, oldIndex, oldSPA, oldCORS
		serveGzip, serveCache, serveHSTS, serveHSTSMaxAge = oldGzip, oldCache, oldHSTS, oldMaxAge
		serveHTTPRedirect, serveHTTPPort = oldRedirect, oldHTTPPort
	})
}

func TestApplyServeFlagOverrides_AllFlags(t *testing.T) {
	restoreServeFlagGlobals(t)
	cmd := newServeFlagsCmd()

	flagValues := map[string]string{
		"dir":           "/flag/dir",
		"index":         "main.html",
		"spa":           "true",
		"cors":          "true",
		"gzip":          "true",
		"cache":         "max-age=60",
		"hsts":          "true",
		"hsts-max-age":  "600",
		"http-redirect": "true",
		"http-port":     "8081",
	}
	for name, value := range flagValues {
		if err := cmd.Flags().Set(name, value); err != nil {
			t.Fatalf("set --%s: %v", name, err)
		}
	}

	withCfg(t, &config.Config{Serve: config.ServeConfig{Dir: "/file/dir"}})

	// Positional arg is applied first, then the explicit --dir flag wins.
	applyServeFlagOverrides(cmd, []string{"/positional/dir"})

	s := cfg.Serve
	if s.Dir != "/flag/dir" {
		t.Errorf("Dir = %q, want the --dir flag value", s.Dir)
	}
	if s.Index != "main.html" {
		t.Errorf("Index = %q, want main.html", s.Index)
	}
	if !s.SPA || !s.CORS || !s.Gzip || !s.HSTS || !s.HTTPRedirect {
		t.Errorf("SPA/CORS/Gzip/HSTS/HTTPRedirect = %v/%v/%v/%v/%v, want all true",
			s.SPA, s.CORS, s.Gzip, s.HSTS, s.HTTPRedirect)
	}
	if s.Cache != "max-age=60" {
		t.Errorf("Cache = %q, want max-age=60", s.Cache)
	}
	if s.HSTSMaxAge != 600 {
		t.Errorf("HSTSMaxAge = %d, want 600", s.HSTSMaxAge)
	}
	if s.HTTPPort != 8081 {
		t.Errorf("HTTPPort = %d, want 8081", s.HTTPPort)
	}
}

func TestApplyServeFlagOverrides_PositionalDirOnly(t *testing.T) {
	withCfg(t, &config.Config{Serve: config.ServeConfig{Dir: "/file/dir"}})

	applyServeFlagOverrides(newServeFlagsCmd(), []string{"/positional/dir"})
	if cfg.Serve.Dir != "/positional/dir" {
		t.Errorf("Dir = %q, want the positional value", cfg.Serve.Dir)
	}
}

func TestRunServe_NotADirectory(t *testing.T) {
	filePath := filepath.Join(t.TempDir(), "file.txt")
	if err := os.WriteFile(filePath, []byte("x"), 0o600); err != nil {
		t.Fatalf("write file: %v", err)
	}

	withCfg(t, &config.Config{
		Port:    occupiedPort(t),
		Host:    "127.0.0.1",
		Serve:   config.ServeConfig{Dir: filePath, Index: "index.html"},
		Metrics: config.MetricsConfig{Enabled: false},
	})

	err := runServe(serveCmd, nil)
	if err == nil {
		t.Fatal("expected error for a non-directory path, got nil")
	}
	if !strings.Contains(err.Error(), "not a directory") {
		t.Errorf("error = %v, want a not-a-directory message", err)
	}
}

func TestRunServe_FullServerPathBindFailure(t *testing.T) {
	// Gzip + CORS + cache + verbose + metrics make runServe walk its middleware
	// and collector construction lines; only the bind error is observed here,
	// so this covers line paths, not middleware behavior (see
	// TestRunServe_LiveGzipAndCORS for the behavioral check).
	withCfg(t, &config.Config{
		Port:    occupiedPort(t),
		Host:    "127.0.0.1",
		Verbose: true,
		Serve: config.ServeConfig{
			Dir:   t.TempDir(),
			Index: "index.html",
			Gzip:  true,
			CORS:  true,
			Cache: "max-age=60",
		},
		Metrics: config.MetricsConfig{Enabled: true, Port: freePort(t), Path: "/_metrics", Format: "json"},
	})

	err := runServe(serveCmd, nil)
	if err == nil {
		t.Fatal("expected bind error from occupied port, got nil")
	}
	if !strings.Contains(err.Error(), "already in use") {
		t.Errorf("error = %v, want an address-in-use bind failure", err)
	}
}

func TestRunServe_TLSConfigError(t *testing.T) {
	withCfg(t, &config.Config{
		Port:  occupiedPort(t),
		Host:  "127.0.0.1",
		Serve: config.ServeConfig{Dir: t.TempDir(), Index: "index.html"},
		TLS: config.TLSConfig{
			Enabled: true,
			Cert:    "/nonexistent/cert.pem",
			Key:     "/nonexistent/key.pem",
		},
		Metrics: config.MetricsConfig{Enabled: false},
	})

	err := runServe(serveCmd, nil)
	if err == nil {
		t.Fatal("expected TLS configuration error, got nil")
	}
	if !strings.Contains(err.Error(), "TLS configuration error") {
		t.Errorf("error = %v, want a TLS configuration error", err)
	}
}

func TestRunServe_TLSWithHSTSAndRedirectBindFailure(t *testing.T) {
	certPath, keyPath, _ := genTestCerts(t)

	withCfg(t, &config.Config{
		Port: occupiedPort(t),
		Host: "127.0.0.1",
		Serve: config.ServeConfig{
			Dir:          t.TempDir(),
			Index:        "index.html",
			HSTS:         true,
			HSTSMaxAge:   600,
			HTTPRedirect: true,
			HTTPPort:     freePort(t),
		},
		TLS: config.TLSConfig{
			Enabled:    true,
			Cert:       certPath,
			Key:        keyPath,
			MinVersion: "1.3",
		},
		Metrics: config.MetricsConfig{Enabled: false},
	})

	err := runServe(serveCmd, nil)
	if err == nil {
		t.Fatal("expected bind error from occupied port, got nil")
	}
	if strings.Contains(err.Error(), "TLS configuration error") {
		t.Errorf("valid certs should pass TLS setup; got %v", err)
	}
	if !strings.Contains(err.Error(), "already in use") {
		t.Errorf("error = %v, want an address-in-use bind failure", err)
	}
}
