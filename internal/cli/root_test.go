package cli

import (
	"bytes"
	"testing"
)

// resetRootCmdFlags resets the root command's flag state between tests.
// This is necessary because cobra caches flag parsing state on the global rootCmd.
func resetRootCmdFlags() {
	rootCmd.SetArgs([]string{})
	cfg = nil

	// Reset TLS flag variables to their defaults
	tlsEnabled = false
	tlsCert = ""
	tlsKey = ""
	tlsCA = ""
	tlsClientAuth = false
	tlsMinVersion = "1.2"

	// Reset other flag variables to their defaults
	cfgFile = ""
	port = 8080
	host = "localhost"
	verbose = false
	noColor = false
	metricsEnabled = true
	metricsPath = "/_metrics"
	metricsFormat = "json"
}

func TestTLSFlagsExistWithDefaults(t *testing.T) {
	resetRootCmdFlags()

	tests := []struct {
		name         string
		flagName     string
		wantDefault  string
		wantExist    bool
	}{
		{"tls flag exists", "tls", "false", true},
		{"cert flag exists", "cert", "", true},
		{"key flag exists", "key", "", true},
		{"ca flag exists", "ca", "", true},
		{"client-auth flag exists", "client-auth", "false", true},
		{"tls-min-version flag exists", "tls-min-version", "1.2", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			flag := rootCmd.PersistentFlags().Lookup(tt.flagName)
			if flag == nil {
				t.Fatalf("flag %q does not exist on root command", tt.flagName)
			}
			if flag.DefValue != tt.wantDefault {
				t.Errorf("flag %q default = %q, want %q", tt.flagName, flag.DefValue, tt.wantDefault)
			}
		})
	}
}

func TestTLSFlagSetsConfig(t *testing.T) {
	resetRootCmdFlags()

	var buf bytes.Buffer
	rootCmd.SetOut(&buf)
	rootCmd.SetErr(&buf)
	rootCmd.SetArgs([]string{"version", "--tls"})

	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg == nil {
		t.Fatal("config was not loaded")
	}
	if !cfg.TLS.Enabled {
		t.Error("expected TLS.Enabled to be true after --tls flag")
	}
}

func TestTLSCertAndKeyFlagsSetConfig(t *testing.T) {
	resetRootCmdFlags()

	var buf bytes.Buffer
	rootCmd.SetOut(&buf)
	rootCmd.SetErr(&buf)
	rootCmd.SetArgs([]string{"version", "--cert", "/path/to/cert.pem", "--key", "/path/to/key.pem"})

	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg == nil {
		t.Fatal("config was not loaded")
	}
	if cfg.TLS.Cert != "/path/to/cert.pem" {
		t.Errorf("TLS.Cert = %q, want %q", cfg.TLS.Cert, "/path/to/cert.pem")
	}
	if cfg.TLS.Key != "/path/to/key.pem" {
		t.Errorf("TLS.Key = %q, want %q", cfg.TLS.Key, "/path/to/key.pem")
	}
}

func TestTLSMinVersionFlagSetsConfig(t *testing.T) {
	resetRootCmdFlags()

	var buf bytes.Buffer
	rootCmd.SetOut(&buf)
	rootCmd.SetErr(&buf)
	rootCmd.SetArgs([]string{"version", "--tls-min-version", "1.3"})

	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg == nil {
		t.Fatal("config was not loaded")
	}
	if cfg.TLS.MinVersion != "1.3" {
		t.Errorf("TLS.MinVersion = %q, want %q", cfg.TLS.MinVersion, "1.3")
	}
}

func TestTLSCAAndClientAuthFlagsSetConfig(t *testing.T) {
	resetRootCmdFlags()

	var buf bytes.Buffer
	rootCmd.SetOut(&buf)
	rootCmd.SetErr(&buf)
	rootCmd.SetArgs([]string{"version", "--ca", "/path/to/ca.pem", "--client-auth"})

	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg == nil {
		t.Fatal("config was not loaded")
	}
	if cfg.TLS.CA != "/path/to/ca.pem" {
		t.Errorf("TLS.CA = %q, want %q", cfg.TLS.CA, "/path/to/ca.pem")
	}
	if !cfg.TLS.ClientAuth {
		t.Error("expected TLS.ClientAuth to be true after --client-auth flag")
	}
}

func TestTLSFlagsOverrideConfigDefaults(t *testing.T) {
	resetRootCmdFlags()

	// Without flags, config should have defaults from Viper
	var buf bytes.Buffer
	rootCmd.SetOut(&buf)
	rootCmd.SetErr(&buf)
	rootCmd.SetArgs([]string{"version"})

	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg == nil {
		t.Fatal("config was not loaded")
	}

	// Defaults from config/viper: TLS disabled, min version 1.2
	if cfg.TLS.Enabled {
		t.Error("expected TLS.Enabled to be false by default")
	}
	if cfg.TLS.MinVersion != "1.2" {
		t.Errorf("default TLS.MinVersion = %q, want %q", cfg.TLS.MinVersion, "1.2")
	}

	// Now run with flags to override
	resetRootCmdFlags()
	buf.Reset()
	rootCmd.SetOut(&buf)
	rootCmd.SetErr(&buf)
	rootCmd.SetArgs([]string{
		"version",
		"--tls",
		"--cert", "/override/cert.pem",
		"--key", "/override/key.pem",
		"--ca", "/override/ca.pem",
		"--client-auth",
		"--tls-min-version", "1.3",
	})

	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg == nil {
		t.Fatal("config was not loaded after flag override")
	}
	if !cfg.TLS.Enabled {
		t.Error("expected TLS.Enabled to be true after --tls flag override")
	}
	if cfg.TLS.Cert != "/override/cert.pem" {
		t.Errorf("TLS.Cert = %q, want %q", cfg.TLS.Cert, "/override/cert.pem")
	}
	if cfg.TLS.Key != "/override/key.pem" {
		t.Errorf("TLS.Key = %q, want %q", cfg.TLS.Key, "/override/key.pem")
	}
	if cfg.TLS.CA != "/override/ca.pem" {
		t.Errorf("TLS.CA = %q, want %q", cfg.TLS.CA, "/override/ca.pem")
	}
	if !cfg.TLS.ClientAuth {
		t.Error("expected TLS.ClientAuth to be true after --client-auth flag override")
	}
	if cfg.TLS.MinVersion != "1.3" {
		t.Errorf("TLS.MinVersion = %q, want %q", cfg.TLS.MinVersion, "1.3")
	}
}

func TestTLSDefaultsNotOverriddenWhenFlagsNotSet(t *testing.T) {
	resetRootCmdFlags()

	var buf bytes.Buffer
	rootCmd.SetOut(&buf)
	rootCmd.SetErr(&buf)
	rootCmd.SetArgs([]string{"version"})

	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg == nil {
		t.Fatal("config was not loaded")
	}

	// When no TLS flags are passed, config values should come from viper defaults
	if cfg.TLS.Enabled {
		t.Error("TLS.Enabled should be false when --tls flag not set")
	}
	if cfg.TLS.Cert != "" {
		t.Errorf("TLS.Cert should be empty when --cert flag not set, got %q", cfg.TLS.Cert)
	}
	if cfg.TLS.Key != "" {
		t.Errorf("TLS.Key should be empty when --key flag not set, got %q", cfg.TLS.Key)
	}
	if cfg.TLS.CA != "" {
		t.Errorf("TLS.CA should be empty when --ca flag not set, got %q", cfg.TLS.CA)
	}
	if cfg.TLS.ClientAuth {
		t.Error("TLS.ClientAuth should be false when --client-auth flag not set")
	}
	if cfg.TLS.MinVersion != "1.2" {
		t.Errorf("TLS.MinVersion = %q, want %q when --tls-min-version flag not set", cfg.TLS.MinVersion, "1.2")
	}
}
