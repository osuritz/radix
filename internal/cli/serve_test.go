package cli

import (
	"testing"

	"github.com/osuritz/radix/internal/config"
)

func TestServeCmd_Registered(t *testing.T) {
	found := false
	for _, cmd := range rootCmd.Commands() {
		if cmd.Name() == "serve" {
			found = true
			break
		}
	}
	if !found {
		t.Error("serve command not registered on root command")
	}
}

func TestServeCmd_Flags(t *testing.T) {
	flags := []string{"dir", "index", "spa", "cors", "gzip", "cache", "hsts", "hsts-max-age", "http-redirect", "http-port"}
	for _, name := range flags {
		if serveCmd.Flags().Lookup(name) == nil {
			t.Errorf("flag %q not registered on serve command", name)
		}
	}
}

func TestServeCmd_FlagDefaults(t *testing.T) {
	tests := []struct {
		flag string
		want string
	}{
		{"hsts", "false"},
		{"hsts-max-age", "31536000"},
		{"http-redirect", "false"},
		{"http-port", "8080"},
	}
	for _, tt := range tests {
		f := serveCmd.Flags().Lookup(tt.flag)
		if f == nil {
			t.Errorf("flag %q not registered", tt.flag)
			continue
		}
		if f.DefValue != tt.want {
			t.Errorf("flag %q default = %q, want %q", tt.flag, f.DefValue, tt.want)
		}
	}
}

func TestServeCmd_HSTSRequiresTLS(t *testing.T) {
	oldCfg := cfg
	defer func() { cfg = oldCfg }()

	cfg = &config.Config{
		Port: 8443,
		Host: "localhost",
		Serve: config.ServeConfig{
			Dir:        ".",
			Index:      "index.html",
			HSTS:       true,
			HSTSMaxAge: 31536000,
		},
		TLS:     config.TLSConfig{Enabled: false},
		Metrics: config.MetricsConfig{Enabled: false},
	}

	err := runServe(serveCmd, nil)
	if err == nil {
		t.Fatal("expected error when --hsts set without --tls")
	}
	if got := err.Error(); got != "--hsts requires --tls" {
		t.Errorf("error = %q, want %q", got, "--hsts requires --tls")
	}
}

func TestServeCmd_HTTPRedirectRequiresTLS(t *testing.T) {
	oldCfg := cfg
	defer func() { cfg = oldCfg }()

	cfg = &config.Config{
		Port: 8443,
		Host: "localhost",
		Serve: config.ServeConfig{
			Dir:          ".",
			Index:        "index.html",
			HTTPRedirect: true,
			HTTPPort:     8080,
		},
		TLS:     config.TLSConfig{Enabled: false},
		Metrics: config.MetricsConfig{Enabled: false},
	}

	err := runServe(serveCmd, nil)
	if err == nil {
		t.Fatal("expected error when --http-redirect set without --tls")
	}
	if got := err.Error(); got != "--http-redirect requires --tls" {
		t.Errorf("error = %q, want %q", got, "--http-redirect requires --tls")
	}
}

func TestServeCmd_HTTPRedirectSamePortRejected(t *testing.T) {
	oldCfg := cfg
	defer func() { cfg = oldCfg }()

	cfg = &config.Config{
		Port: 8443,
		Host: "localhost",
		Serve: config.ServeConfig{
			Dir:          ".",
			Index:        "index.html",
			HTTPRedirect: true,
			HTTPPort:     8443, // same as Port
		},
		TLS:     config.TLSConfig{Enabled: true},
		Metrics: config.MetricsConfig{Enabled: false},
	}

	err := runServe(serveCmd, nil)
	if err == nil {
		t.Fatal("expected error when --http-port equals --port")
	}
	want := "--http-port (8443) must differ from --port (8443)"
	if got := err.Error(); got != want {
		t.Errorf("error = %q, want %q", got, want)
	}
}

func TestServeCmd_AcceptsPositionalArg(t *testing.T) {
	if err := serveCmd.Args(serveCmd, []string{"./dist"}); err != nil {
		t.Errorf("serve should accept one positional arg: %v", err)
	}
}

func TestServeCmd_RejectsTooManyArgs(t *testing.T) {
	if err := serveCmd.Args(serveCmd, []string{"a", "b"}); err == nil {
		t.Error("serve should reject more than one positional arg")
	}
}

func TestServeCmd_InvalidDirectory(t *testing.T) {
	oldCfg := cfg
	defer func() { cfg = oldCfg }()

	cfg = &config.Config{
		Port: 0,
		Host: "localhost",
		Serve: config.ServeConfig{
			Dir:   "/nonexistent/path/that/does/not/exist",
			Index: "index.html",
		},
		Metrics: config.MetricsConfig{
			Enabled: false,
		},
	}

	err := runServe(serveCmd, nil)
	if err == nil {
		t.Error("expected error for non-existent directory")
	}
}
