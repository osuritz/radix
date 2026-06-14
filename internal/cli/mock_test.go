package cli

import (
	"math"
	"strings"
	"testing"

	"github.com/osuritz/radix/internal/config"
)

func TestMockCmd_Registered(t *testing.T) {
	found := false
	for _, cmd := range rootCmd.Commands() {
		if cmd.Name() == "mock" {
			found = true
			break
		}
	}
	if !found {
		t.Error("mock command not registered on root command")
	}
}

func TestMockCmd_Flags(t *testing.T) {
	flags := []string{
		"latency", "latency-jitter", "fail-rate", "fail-status",
		"cors", "builtin", "prefix",
	}
	for _, name := range flags {
		if mockCmd.Flags().Lookup(name) == nil {
			t.Errorf("flag %q not registered on mock command", name)
		}
	}
}

// newMockCfg returns a Config whose Mock section is valid except for the
// overrides applied by the caller. Metrics are disabled so runMock never tries
// to build a collector before reaching the failing validation.
func newMockCfg(mock config.MockConfig) *config.Config {
	return &config.Config{
		Port:    0,
		Host:    "localhost",
		Mock:    mock,
		Metrics: config.MetricsConfig{Enabled: false},
	}
}

func TestMockCmd_InvalidInputRejected(t *testing.T) {
	// A valid baseline; each case overrides exactly one field to make it invalid.
	valid := config.MockConfig{
		FailStatus: 500,
		Builtin:    true,
	}

	tests := []struct {
		name    string
		mutate  func(m *config.MockConfig)
		wantSub string
	}{
		{
			name:    "fail-rate negative",
			mutate:  func(m *config.MockConfig) { m.FailRate = -1 },
			wantSub: "invalid --fail-rate",
		},
		{
			name:    "fail-rate over 100",
			mutate:  func(m *config.MockConfig) { m.FailRate = 150 },
			wantSub: "invalid --fail-rate",
		},
		{
			name:    "fail-rate NaN",
			mutate:  func(m *config.MockConfig) { m.FailRate = math.NaN() },
			wantSub: "invalid --fail-rate",
		},
		{
			name:    "fail-rate Inf",
			mutate:  func(m *config.MockConfig) { m.FailRate = math.Inf(1) },
			wantSub: "invalid --fail-rate",
		},
		{
			name:    "fail-status too low (1xx)",
			mutate:  func(m *config.MockConfig) { m.FailStatus = 100 },
			wantSub: "invalid --fail-status",
		},
		{
			name:    "fail-status below 200",
			mutate:  func(m *config.MockConfig) { m.FailStatus = 199 },
			wantSub: "invalid --fail-status",
		},
		{
			name:    "fail-status too high",
			mutate:  func(m *config.MockConfig) { m.FailStatus = 600 },
			wantSub: "invalid --fail-status",
		},
		{
			name:    "bad latency duration",
			mutate:  func(m *config.MockConfig) { m.Latency = "not-a-duration" },
			wantSub: "invalid --latency",
		},
		{
			name:    "negative latency duration",
			mutate:  func(m *config.MockConfig) { m.Latency = "-5s" },
			wantSub: "invalid --latency",
		},
		{
			name:    "bad latency-jitter duration",
			mutate:  func(m *config.MockConfig) { m.LatencyJitter = "garbage" },
			wantSub: "invalid --latency-jitter",
		},
		{
			name:    "prefix with brace",
			mutate:  func(m *config.MockConfig) { m.Prefix = "/api/{id}" },
			wantSub: "invalid --prefix",
		},
		{
			name:    "prefix with whitespace",
			mutate:  func(m *config.MockConfig) { m.Prefix = "/a b" },
			wantSub: "invalid --prefix",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			oldCfg := cfg
			defer func() { cfg = oldCfg }()

			m := valid
			tt.mutate(&m)
			cfg = newMockCfg(m)

			err := runMock(mockCmd, nil)
			if err == nil {
				t.Fatalf("expected error for %s, got nil", tt.name)
			}
			if !strings.Contains(err.Error(), tt.wantSub) {
				t.Errorf("expected error containing %q, got %v", tt.wantSub, err)
			}
		})
	}
}

func TestMockCmd_ValidPrefixAccepted(t *testing.T) {
	// A simple path prefix must pass validation (it then proceeds past the
	// boundary checks; we only assert it is not rejected as a bad prefix).
	for _, p := range []string{"", "/", "/_test", "_test", "/api/v1"} {
		if err := validateMockPrefix(p); err != nil {
			t.Errorf("validateMockPrefix(%q) = %v, want nil", p, err)
		}
	}
}

func TestMockCmd_InvalidPrefixRejected(t *testing.T) {
	for _, p := range []string{"/api/{id}", "/a b", "/has\ttab", "/{x}"} {
		if err := validateMockPrefix(p); err == nil {
			t.Errorf("validateMockPrefix(%q) = nil, want error", p)
		}
	}
}
