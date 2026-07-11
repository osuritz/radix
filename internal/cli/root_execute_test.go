package cli

import (
	"bytes"
	"strconv"
	"testing"
)

// resetPersistentFlagChanged clears the Changed state of the named persistent
// flags on the root command when the test finishes, so later tests that rely
// on Changed(...) precedence are not affected.
func resetPersistentFlagChanged(t *testing.T, names ...string) {
	t.Helper()
	t.Cleanup(func() {
		for _, name := range names {
			if f := rootCmd.PersistentFlags().Lookup(name); f != nil {
				f.Changed = false
			}
		}
	})
}

func TestExecute_RunsVersionCommand(t *testing.T) {
	resetRootCmdFlags()

	var buf bytes.Buffer
	rootCmd.SetOut(&buf)
	rootCmd.SetErr(&buf)
	rootCmd.SetArgs([]string{"version"})

	if err := Execute(); err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if buf.Len() == 0 {
		t.Error("expected version output, got nothing")
	}
	if GetConfig() == nil {
		t.Error("GetConfig returned nil after Execute loaded the config")
	}
}

func TestConfigPrecedence_EnvOverridesFile(t *testing.T) {
	resetRootCmdFlags()
	resetPersistentFlagChanged(t, "config", "port")
	t.Cleanup(func() { cfgFile = ""; port = 8080 })

	path := writeTempConfig(t, "port: 1234\n")
	t.Setenv("RADIX_PORT", "5678")

	var buf bytes.Buffer
	rootCmd.SetOut(&buf)
	rootCmd.SetErr(&buf)
	rootCmd.SetArgs([]string{"version", "--config", path})

	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if cfg == nil {
		t.Fatal("config was not loaded")
	}
	if cfg.Port != 5678 {
		t.Errorf("cfg.Port = %d, want 5678 (env RADIX_PORT should override the config file)", cfg.Port)
	}
}

func TestConfigPrecedence_FlagOverridesEnvAndFile(t *testing.T) {
	resetRootCmdFlags()
	resetPersistentFlagChanged(t, "config", "port")
	t.Cleanup(func() { cfgFile = ""; port = 8080 })

	path := writeTempConfig(t, "port: 1234\n")
	t.Setenv("RADIX_PORT", "5678")

	var buf bytes.Buffer
	rootCmd.SetOut(&buf)
	rootCmd.SetErr(&buf)
	rootCmd.SetArgs([]string{"version", "--config", path, "--port", "9999"})

	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if cfg == nil {
		t.Fatal("config was not loaded")
	}
	if cfg.Port != 9999 {
		t.Errorf("cfg.Port = %d, want 9999 (--port flag should win over env and file)", cfg.Port)
	}
}

func TestConfigPrecedence_FileOverridesDefault(t *testing.T) {
	resetRootCmdFlags()
	resetPersistentFlagChanged(t, "config")
	t.Cleanup(func() { cfgFile = "" })

	path := writeTempConfig(t, "port: 4321\nhost: 0.0.0.0\n")

	var buf bytes.Buffer
	rootCmd.SetOut(&buf)
	rootCmd.SetErr(&buf)
	rootCmd.SetArgs([]string{"version", "--config", path})

	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if cfg == nil {
		t.Fatal("config was not loaded")
	}
	if cfg.Port != 4321 {
		t.Errorf("cfg.Port = %d, want 4321 from config file", cfg.Port)
	}
	if cfg.Host != "0.0.0.0" {
		t.Errorf("cfg.Host = %q, want %q from config file", cfg.Host, "0.0.0.0")
	}
}

func TestRootCmd_GlobalFlagOverrides(t *testing.T) {
	resetRootCmdFlags()
	resetPersistentFlagChanged(t, "host", "verbose", "no-color",
		"metrics", "metrics-path", "metrics-format", "metrics-port")
	t.Cleanup(func() {
		host = "localhost"
		verbose = false
		noColor = false
		metricsEnabled = true
		metricsPath = "/_metrics"
		metricsFormat = "json"
		metricsPort = 9090
	})

	var buf bytes.Buffer
	rootCmd.SetOut(&buf)
	rootCmd.SetErr(&buf)
	rootCmd.SetArgs([]string{
		"version",
		"--host", "0.0.0.0",
		"--verbose",
		"--no-color",
		"--metrics=false",
		"--metrics-path", "/custom",
		"--metrics-format", "prometheus",
		"--metrics-port", strconv.Itoa(9191),
	})

	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if cfg == nil {
		t.Fatal("config was not loaded")
	}
	if cfg.Host != "0.0.0.0" {
		t.Errorf("cfg.Host = %q, want %q", cfg.Host, "0.0.0.0")
	}
	if !cfg.Verbose {
		t.Error("cfg.Verbose = false, want true after --verbose")
	}
	if !cfg.NoColor {
		t.Error("cfg.NoColor = false, want true after --no-color")
	}
	if cfg.Metrics.Enabled {
		t.Error("cfg.Metrics.Enabled = true, want false after --metrics=false")
	}
	if cfg.Metrics.Path != "/custom" {
		t.Errorf("cfg.Metrics.Path = %q, want %q", cfg.Metrics.Path, "/custom")
	}
	if cfg.Metrics.Format != "prometheus" {
		t.Errorf("cfg.Metrics.Format = %q, want %q", cfg.Metrics.Format, "prometheus")
	}
	if cfg.Metrics.Port != 9191 {
		t.Errorf("cfg.Metrics.Port = %d, want 9191", cfg.Metrics.Port)
	}
}
