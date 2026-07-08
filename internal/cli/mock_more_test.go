package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/osuritz/radix/internal/config"
	"github.com/osuritz/radix/internal/server"
	"github.com/spf13/cobra"
)

// newMockFlagsCmd builds a fresh command carrying the mock flag set bound to
// the same package globals, so tests can mark flags Changed without mutating
// the shared mockCmd. Registering the flags resets the globals to defaults;
// callers must save/restore any globals they care about.
func newMockFlagsCmd() *cobra.Command {
	c := &cobra.Command{Use: "mock"}
	c.Flags().StringVar(&mockLatency, "latency", "", "")
	c.Flags().StringVar(&mockLatencyJitter, "latency-jitter", "", "")
	c.Flags().Float64Var(&mockFailRate, "fail-rate", 0, "")
	c.Flags().IntVar(&mockFailStatus, "fail-status", 500, "")
	c.Flags().BoolVar(&mockCORS, "cors", false, "")
	c.Flags().BoolVar(&mockBuiltin, "builtin", true, "")
	c.Flags().StringVar(&mockPrefix, "prefix", "", "")
	c.Flags().StringVarP(&mockRoutes, "routes", "r", "", "")
	c.Flags().BoolVarP(&mockWatch, "watch", "w", false, "")
	return c
}

// restoreMockFlagGlobals snapshots all mock flag globals and restores them on
// test cleanup.
func restoreMockFlagGlobals(t *testing.T) {
	t.Helper()
	oldLatency, oldJitter := mockLatency, mockLatencyJitter
	oldRate, oldStatus := mockFailRate, mockFailStatus
	oldCORS, oldBuiltin, oldPrefix := mockCORS, mockBuiltin, mockPrefix
	oldRoutes, oldWatch := mockRoutes, mockWatch
	t.Cleanup(func() {
		mockLatency, mockLatencyJitter = oldLatency, oldJitter
		mockFailRate, mockFailStatus = oldRate, oldStatus
		mockCORS, mockBuiltin, mockPrefix = oldCORS, oldBuiltin, oldPrefix
		mockRoutes, mockWatch = oldRoutes, oldWatch
	})
}

// writeRoutesFile writes a routes YAML file into a temp dir and returns its path.
func writeRoutesFile(t *testing.T, yaml string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "routes.yml")
	if err := os.WriteFile(path, []byte(yaml), 0o600); err != nil {
		t.Fatalf("write routes file: %v", err)
	}
	return path
}

func TestApplyMockFlags_AllOverrides(t *testing.T) {
	restoreMockFlagGlobals(t)
	cmd := newMockFlagsCmd()

	flagValues := map[string]string{
		"latency":        "200ms",
		"latency-jitter": "100ms",
		"fail-rate":      "10",
		"fail-status":    "503",
		"cors":           "true",
		"builtin":        "false",
		"prefix":         "/_test",
		"routes":         "routes.yml",
		"watch":          "true",
	}
	for name, value := range flagValues {
		if err := cmd.Flags().Set(name, value); err != nil {
			t.Fatalf("set --%s: %v", name, err)
		}
	}

	withCfg(t, &config.Config{Mock: config.MockConfig{FailStatus: 500, Builtin: true}})

	applyMockFlags(cmd)

	m := cfg.Mock
	if m.Latency != "200ms" {
		t.Errorf("Latency = %q, want 200ms", m.Latency)
	}
	if m.LatencyJitter != "100ms" {
		t.Errorf("LatencyJitter = %q, want 100ms", m.LatencyJitter)
	}
	if m.FailRate != 10 {
		t.Errorf("FailRate = %g, want 10", m.FailRate)
	}
	if m.FailStatus != 503 {
		t.Errorf("FailStatus = %d, want 503", m.FailStatus)
	}
	if !m.CORS {
		t.Error("CORS = false, want true")
	}
	if m.Builtin {
		t.Error("Builtin = true, want false after --builtin=false")
	}
	if m.Prefix != "/_test" {
		t.Errorf("Prefix = %q, want /_test", m.Prefix)
	}
	if m.Routes != "routes.yml" {
		t.Errorf("Routes = %q, want routes.yml", m.Routes)
	}
	if !m.Watch {
		t.Error("Watch = false, want true")
	}
}

func TestValidateEffectiveSettings(t *testing.T) {
	tests := []struct {
		name     string
		settings server.RouteSettings
		wantSub  string
	}{
		{"valid", server.RouteSettings{FailRate: 0, FailStatus: 500}, ""},
		{"fail rate over 100", server.RouteSettings{FailRate: 150, FailStatus: 500}, "invalid fail_rate"},
		{"fail rate negative", server.RouteSettings{FailRate: -1, FailStatus: 500}, "invalid fail_rate"},
		{"fail status too low", server.RouteSettings{FailStatus: 199}, "invalid fail_status"},
		{"fail status too high", server.RouteSettings{FailStatus: 600}, "invalid fail_status"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateEffectiveSettings(tt.settings)
			if tt.wantSub == "" {
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
				return
			}
			if err == nil {
				t.Fatalf("expected error containing %q, got nil", tt.wantSub)
			}
			if !strings.Contains(err.Error(), tt.wantSub) {
				t.Errorf("error = %v, want it to contain %q", err, tt.wantSub)
			}
		})
	}
}

func TestMockLatencyDurations(t *testing.T) {
	restoreMockFlagGlobals(t)

	mockLatency = "200ms"
	if got := mockLatencyDuration(); got != 200*time.Millisecond {
		t.Errorf("mockLatencyDuration() = %v, want 200ms", got)
	}
	mockLatency = "garbage"
	if got := mockLatencyDuration(); got != 0 {
		t.Errorf("mockLatencyDuration() for garbage = %v, want 0", got)
	}

	mockLatencyJitter = "50ms"
	if got := mockLatencyJitterDuration(); got != 50*time.Millisecond {
		t.Errorf("mockLatencyJitterDuration() = %v, want 50ms", got)
	}
	mockLatencyJitter = "garbage"
	if got := mockLatencyJitterDuration(); got != 0 {
		t.Errorf("mockLatencyJitterDuration() for garbage = %v, want 0", got)
	}
}

func TestCliSettingsOverride(t *testing.T) {
	restoreMockFlagGlobals(t)
	cmd := newMockFlagsCmd()

	for name, value := range map[string]string{
		"latency":        "200ms",
		"latency-jitter": "100ms",
		"fail-rate":      "25",
		"fail-status":    "503",
		"cors":           "true",
	} {
		if err := cmd.Flags().Set(name, value); err != nil {
			t.Fatalf("set --%s: %v", name, err)
		}
	}

	settings := server.RouteSettings{
		Latency:    time.Second, // file value, must be overridden
		FailRate:   1,
		FailStatus: 500,
		CORS:       false,
	}
	cliSettingsOverride(cmd)(&settings)

	if settings.Latency != 200*time.Millisecond {
		t.Errorf("Latency = %v, want 200ms", settings.Latency)
	}
	if settings.LatencyJitter != 100*time.Millisecond {
		t.Errorf("LatencyJitter = %v, want 100ms", settings.LatencyJitter)
	}
	if settings.FailRate != 25 {
		t.Errorf("FailRate = %g, want 25", settings.FailRate)
	}
	if settings.FailStatus != 503 {
		t.Errorf("FailStatus = %d, want 503", settings.FailStatus)
	}
	if !settings.CORS {
		t.Error("CORS = false, want true")
	}
}

func TestCliSettingsOverride_NoFlagsLeavesFileValues(t *testing.T) {
	restoreMockFlagGlobals(t)
	cmd := newMockFlagsCmd()

	settings := server.RouteSettings{
		Latency:    time.Second,
		FailRate:   42,
		FailStatus: 418,
		CORS:       true,
	}
	original := settings
	cliSettingsOverride(cmd)(&settings)

	if settings != original {
		t.Errorf("settings changed with no flags set: got %+v, want %+v", settings, original)
	}
}

func TestRunMock_FullPathWithRoutesAndWatchBindFailure(t *testing.T) {
	routesPath := writeRoutesFile(t, `
settings:
  cors: true
routes:
  - path: /api/health
    method: GET
    response:
      status: 200
      body: '{"status":"ok"}'
`)

	withCfg(t, &config.Config{
		Port:    occupiedPort(t),
		Host:    "127.0.0.1",
		Verbose: true,
		Mock: config.MockConfig{
			FailStatus: 500,
			Builtin:    true,
			Routes:     routesPath,
			Watch:      true,
			Latency:    "1ms",
			Prefix:     "/_test",
		},
		Metrics: config.MetricsConfig{Enabled: true, Port: freePort(t), Path: "/_metrics", Format: "json"},
	})

	err := runMock(mockCmd, nil)
	if err == nil {
		t.Fatal("expected bind error from occupied port, got nil")
	}
	assertBindFailure(t, err)
	// The file's cors: true must have been reflected into cfg (no CLI override).
	if !cfg.Mock.CORS {
		t.Error("expected file-level cors: true to be reflected into cfg.Mock.CORS")
	}
}

func TestRunMock_FileSuppliedFailRateValidated(t *testing.T) {
	routesPath := writeRoutesFile(t, `
settings:
  fail_rate: 200
`)

	withCfg(t, &config.Config{
		Port:    occupiedPort(t),
		Host:    "127.0.0.1",
		Mock:    config.MockConfig{FailStatus: 500, Builtin: true, Routes: routesPath},
		Metrics: config.MetricsConfig{Enabled: false},
	})

	err := runMock(mockCmd, nil)
	if err == nil {
		t.Fatal("expected error for file-supplied fail_rate 200, got nil")
	}
	if !strings.Contains(err.Error(), "invalid fail_rate") {
		t.Errorf("error = %v, want an invalid fail_rate message", err)
	}
}

func TestRunMock_BuiltinsOnlyBindFailure(t *testing.T) {
	// CORS + metrics make runMock walk its middleware and collector
	// construction lines; only the bind error is observed here, so this covers
	// line paths, not middleware behavior.
	withCfg(t, &config.Config{
		Port:    occupiedPort(t),
		Host:    "127.0.0.1",
		Mock:    config.MockConfig{FailStatus: 500, Builtin: true, CORS: true},
		Metrics: config.MetricsConfig{Enabled: true, Port: freePort(t), Path: "/_metrics", Format: "json"},
	})

	err := runMock(mockCmd, nil)
	if err == nil {
		t.Fatal("expected bind error from occupied port, got nil")
	}
	assertBindFailure(t, err)
}

func TestRunMock_TLSConfigError(t *testing.T) {
	withCfg(t, &config.Config{
		Port: occupiedPort(t),
		Host: "127.0.0.1",
		Mock: config.MockConfig{FailStatus: 500, Builtin: true},
		TLS: config.TLSConfig{
			Enabled: true,
			Cert:    "/nonexistent/cert.pem",
			Key:     "/nonexistent/key.pem",
		},
		Metrics: config.MetricsConfig{Enabled: false},
	})

	err := runMock(mockCmd, nil)
	if err == nil {
		t.Fatal("expected TLS configuration error, got nil")
	}
	if !strings.Contains(err.Error(), "TLS configuration error") {
		t.Errorf("error = %v, want a TLS configuration error", err)
	}
}

func TestRunMock_TLSOptionalClientAuthBindFailure(t *testing.T) {
	certPath, keyPath, caPath := genTestCerts(t)

	oldOpt := mockOptionalClientAuth
	t.Cleanup(func() { mockOptionalClientAuth = oldOpt })
	mockOptionalClientAuth = true

	withCfg(t, &config.Config{
		Port: occupiedPort(t),
		Host: "127.0.0.1",
		Mock: config.MockConfig{FailStatus: 500, Builtin: true},
		TLS: config.TLSConfig{
			Enabled:    true,
			Cert:       certPath,
			Key:        keyPath,
			CA:         caPath,
			MinVersion: "1.3",
		},
		Metrics: config.MetricsConfig{Enabled: false},
	})

	err := runMock(mockCmd, nil)
	if err == nil {
		t.Fatal("expected bind error from occupied port, got nil")
	}
	if strings.Contains(err.Error(), "TLS configuration error") {
		t.Errorf("valid certs should pass TLS setup; got %v", err)
	}
}

func TestMockServerTLSOptions(t *testing.T) {
	oldOpt := mockOptionalClientAuth
	t.Cleanup(func() { mockOptionalClientAuth = oldOpt })

	tlsCfg := config.TLSConfig{
		Cert:       "/certs/cert.pem",
		Key:        "/certs/key.pem",
		CA:         "/certs/ca.pem",
		MinVersion: "1.3",
	}

	tests := []struct {
		name     string
		optional bool
	}{
		{"optional client auth enabled", true},
		{"optional client auth disabled", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockOptionalClientAuth = tt.optional

			opts := mockServerTLSOptions(&config.Config{TLS: tlsCfg})

			if opts.CertFile != tlsCfg.Cert {
				t.Errorf("CertFile = %q, want %q", opts.CertFile, tlsCfg.Cert)
			}
			if opts.KeyFile != tlsCfg.Key {
				t.Errorf("KeyFile = %q, want %q", opts.KeyFile, tlsCfg.Key)
			}
			if opts.CAFile != tlsCfg.CA {
				t.Errorf("CAFile = %q, want %q", opts.CAFile, tlsCfg.CA)
			}
			if opts.MinVersion != tlsCfg.MinVersion {
				t.Errorf("MinVersion = %q, want %q", opts.MinVersion, tlsCfg.MinVersion)
			}
			if opts.ClientAuth {
				t.Error("ClientAuth = true, want false (config did not enable it)")
			}
			if opts.ClientAuthOptional != tt.optional {
				t.Errorf("ClientAuthOptional = %v, want %v (must follow --optional-client-auth)",
					opts.ClientAuthOptional, tt.optional)
			}
		})
	}
}
