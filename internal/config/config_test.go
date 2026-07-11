package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestValidateMetrics(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		cfg     *Config
		wantErr string // substring; "" means no error expected
	}{
		{
			name:    "valid distinct ports",
			cfg:     &Config{Port: 8080, Metrics: MetricsConfig{Enabled: true, Port: 9090, Path: "/_metrics"}},
			wantErr: "",
		},
		{
			name:    "valid custom path",
			cfg:     &Config{Port: 8080, Metrics: MetricsConfig{Enabled: true, Port: 9090, Path: "/stats"}},
			wantErr: "",
		},
		{
			name:    "disabled ignores port collision",
			cfg:     &Config{Port: 8080, Metrics: MetricsConfig{Enabled: false, Port: 8080}},
			wantErr: "",
		},
		{
			name:    "disabled ignores out-of-range port",
			cfg:     &Config{Port: 8080, Metrics: MetricsConfig{Enabled: false, Port: 0}},
			wantErr: "",
		},
		{
			name:    "disabled ignores empty path",
			cfg:     &Config{Port: 8080, Metrics: MetricsConfig{Enabled: false, Path: ""}},
			wantErr: "",
		},
		{
			name:    "port collision with app port",
			cfg:     &Config{Port: 8080, Metrics: MetricsConfig{Enabled: true, Port: 8080, Path: "/_metrics"}},
			wantErr: "must differ from the app port",
		},
		{
			name:    "port too low",
			cfg:     &Config{Port: 8080, Metrics: MetricsConfig{Enabled: true, Port: 0, Path: "/_metrics"}},
			wantErr: "between 1 and 65535",
		},
		{
			name:    "port too high",
			cfg:     &Config{Port: 8080, Metrics: MetricsConfig{Enabled: true, Port: 70000, Path: "/_metrics"}},
			wantErr: "between 1 and 65535",
		},
		{
			name:    "port negative",
			cfg:     &Config{Port: 8080, Metrics: MetricsConfig{Enabled: true, Port: -1, Path: "/_metrics"}},
			wantErr: "between 1 and 65535",
		},
		{
			name:    "empty path rejected",
			cfg:     &Config{Port: 8080, Metrics: MetricsConfig{Enabled: true, Port: 9090, Path: ""}},
			wantErr: "metrics.path must not be empty",
		},
		{
			name:    "relative path rejected",
			cfg:     &Config{Port: 8080, Metrics: MetricsConfig{Enabled: true, Port: 9090, Path: "metrics"}},
			wantErr: "must start with",
		},
		{
			name:    "path collides with healthz",
			cfg:     &Config{Port: 8080, Metrics: MetricsConfig{Enabled: true, Port: 9090, Path: HealthzPath}},
			wantErr: "reserved",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := ValidateMetrics(tt.cfg)
			if tt.wantErr == "" {
				if err != nil {
					t.Errorf("ValidateMetrics() = %v, want nil", err)
				}
				return
			}
			if err == nil {
				t.Fatalf("ValidateMetrics() = nil, want error containing %q", tt.wantErr)
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("ValidateMetrics() error = %q, want substring %q", err.Error(), tt.wantErr)
			}
		})
	}
}

func TestValidateServeTLS(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		cfg     *Config
		wantErr string // substring; "" means no error expected
	}{
		{
			name:    "defaults valid without TLS",
			cfg:     &Config{Port: 8080, Serve: ServeConfig{HTTPPort: 8080, HSTSMaxAge: 31536000}},
			wantErr: "",
		},
		{
			name: "hsts with tls enabled",
			cfg: &Config{
				Port:  8443,
				TLS:   TLSConfig{Enabled: true},
				Serve: ServeConfig{HSTS: true, HSTSMaxAge: 31536000},
			},
			wantErr: "",
		},
		{
			name: "redirect with tls and distinct ports",
			cfg: &Config{
				Port:  8443,
				TLS:   TLSConfig{Enabled: true},
				Serve: ServeConfig{HTTPRedirect: true, HTTPPort: 8080},
			},
			wantErr: "",
		},
		{
			name:    "hsts max-age zero is valid",
			cfg:     &Config{Port: 8443, TLS: TLSConfig{Enabled: true}, Serve: ServeConfig{HSTS: true, HSTSMaxAge: 0}},
			wantErr: "",
		},
		{
			name:    "negative hsts max-age rejected",
			cfg:     &Config{Port: 8080, Serve: ServeConfig{HSTSMaxAge: -1}},
			wantErr: "must not be negative",
		},
		{
			name:    "hsts without tls rejected",
			cfg:     &Config{Port: 8080, Serve: ServeConfig{HSTS: true}},
			wantErr: "--hsts requires --tls",
		},
		{
			name:    "redirect without tls rejected",
			cfg:     &Config{Port: 8080, Serve: ServeConfig{HTTPRedirect: true, HTTPPort: 8081}},
			wantErr: "--http-redirect requires --tls",
		},
		{
			name: "redirect port collides with app port",
			cfg: &Config{
				Port:  8443,
				TLS:   TLSConfig{Enabled: true},
				Serve: ServeConfig{HTTPRedirect: true, HTTPPort: 8443},
			},
			wantErr: "must differ from --port",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := ValidateServeTLS(tt.cfg)
			if tt.wantErr == "" {
				if err != nil {
					t.Errorf("ValidateServeTLS() = %v, want nil", err)
				}
				return
			}
			if err == nil {
				t.Fatalf("ValidateServeTLS() = nil, want error containing %q", tt.wantErr)
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("ValidateServeTLS() error = %q, want substring %q", err.Error(), tt.wantErr)
			}
		})
	}
}

func TestValidateFile(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	valid := filepath.Join(dir, "radix.yml")
	if err := os.WriteFile(valid, []byte("port: 3000\n"), 0o600); err != nil {
		t.Fatalf("write config fixture: %v", err)
	}

	tests := []struct {
		name    string
		path    string
		wantErr string // substring; "" means no error expected
	}{
		{
			name:    "valid readable file",
			path:    valid,
			wantErr: "",
		},
		{
			name:    "empty path",
			path:    "",
			wantErr: "config file path is empty",
		},
		{
			name:    "missing file",
			path:    filepath.Join(dir, "nope.yml"),
			wantErr: "does not exist",
		},
		{
			name:    "directory instead of file",
			path:    dir,
			wantErr: "is a directory",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := ValidateFile(tt.path)
			if tt.wantErr == "" {
				if err != nil {
					t.Errorf("ValidateFile(%q) = %v, want nil", tt.path, err)
				}
				return
			}
			if err == nil {
				t.Fatalf("ValidateFile(%q) = nil, want error containing %q", tt.path, tt.wantErr)
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("ValidateFile(%q) error = %q, want substring %q", tt.path, err.Error(), tt.wantErr)
			}
		})
	}
}

func TestLoad_ExplicitFile(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "radix.yml")
	content := "port: 3000\nhost: 0.0.0.0\nserve:\n  dir: /srv/www\n  spa: true\n"
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write config fixture: %v", err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Port != 3000 {
		t.Errorf("port = %d, want 3000", cfg.Port)
	}
	if cfg.Host != "0.0.0.0" {
		t.Errorf("host = %q, want %q", cfg.Host, "0.0.0.0")
	}
	if cfg.Serve.Dir != "/srv/www" {
		t.Errorf("serve.dir = %q, want %q", cfg.Serve.Dir, "/srv/www")
	}
	if !cfg.Serve.SPA {
		t.Error("serve.spa = false, want true")
	}
	// Untouched keys keep their defaults.
	if cfg.Metrics.Port != 9090 {
		t.Errorf("metrics.port = %d, want default 9090", cfg.Metrics.Port)
	}
	if cfg.Serve.Index != "index.html" {
		t.Errorf("serve.index = %q, want default %q", cfg.Serve.Index, "index.html")
	}
}

func TestLoad_TLSClientAuthOptional(t *testing.T) {
	t.Parallel()

	t.Run("defaults to false", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()
		path := filepath.Join(dir, "radix.yml")
		if err := os.WriteFile(path, []byte("port: 3000\n"), 0o600); err != nil {
			t.Fatalf("write config fixture: %v", err)
		}

		cfg, err := Load(path)
		if err != nil {
			t.Fatalf("Load: %v", err)
		}
		if cfg.TLS.ClientAuthOptional {
			t.Error("tls.client_auth_optional = true, want default false")
		}
	})

	t.Run("set from config file", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()
		path := filepath.Join(dir, "radix.yml")
		content := "tls:\n  enabled: true\n  ca: ./certs/ca.pem\n  client_auth_optional: true\n"
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			t.Fatalf("write config fixture: %v", err)
		}

		cfg, err := Load(path)
		if err != nil {
			t.Fatalf("Load: %v", err)
		}
		if !cfg.TLS.ClientAuthOptional {
			t.Error("tls.client_auth_optional = false, want true from config file")
		}
		if cfg.TLS.ClientAuth {
			t.Error("tls.client_auth = true, want default false")
		}
	})
}

func TestLoad_MalformedYAML(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "radix.yml")
	if err := os.WriteFile(path, []byte("port: [unclosed\n  bad yaml"), 0o600); err != nil {
		t.Fatalf("write config fixture: %v", err)
	}

	if _, err := Load(path); err == nil {
		t.Fatal("Load() = nil, want error for malformed YAML")
	} else if !strings.Contains(err.Error(), "failed to read config file") {
		t.Errorf("Load() error = %q, want substring %q", err.Error(), "failed to read config file")
	}
}

func TestLoad_ExplicitFileMissing(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "nope.yml")
	if _, err := Load(path); err == nil {
		t.Fatal("Load() = nil, want error for missing explicit config file")
	}
}

func TestLoad_EnvOverride(t *testing.T) {
	// t.Setenv forbids t.Parallel.
	t.Setenv("RADIX_PORT", "4242")
	t.Setenv("RADIX_HOST", "0.0.0.0")

	cfg, err := Load("")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Port != 4242 {
		t.Errorf("port = %d, want env override 4242", cfg.Port)
	}
	if cfg.Host != "0.0.0.0" {
		t.Errorf("host = %q, want env override %q", cfg.Host, "0.0.0.0")
	}
}

func TestLoad_EnvOverridesFile(t *testing.T) {
	// t.Setenv forbids t.Parallel.
	dir := t.TempDir()
	path := filepath.Join(dir, "radix.yml")
	if err := os.WriteFile(path, []byte("port: 3000\n"), 0o600); err != nil {
		t.Fatalf("write config fixture: %v", err)
	}

	t.Setenv("RADIX_PORT", "5555")

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Port != 5555 {
		t.Errorf("port = %d, want env (5555) to override file (3000)", cfg.Port)
	}
}

func TestLoad_DefaultMetricsPort(t *testing.T) {
	t.Parallel()

	// No config file: defaults apply. Load tolerates a missing file.
	cfg, err := Load("")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Metrics.Port != 9090 {
		t.Errorf("default metrics.port = %d, want 9090", cfg.Metrics.Port)
	}
}
