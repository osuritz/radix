package cli

import (
	"strings"
	"testing"

	"github.com/osuritz/radix/internal/config"
)

func TestEchoCmd_Registered(t *testing.T) {
	found := false
	for _, cmd := range rootCmd.Commands() {
		if cmd.Name() == "echo" {
			found = true
			break
		}
	}
	if !found {
		t.Error("echo command not registered on root command")
	}
}

func TestEchoCmd_Flags(t *testing.T) {
	flags := []string{
		"status", "delay", "delay-jitter", "body", "content-type", "header",
		"echo-body", "echo-headers", "echo-query", "body-limit", "pretty",
		"status-from-path", "delay-from-path", "cors",
	}
	for _, name := range flags {
		if echoCmd.Flags().Lookup(name) == nil {
			t.Errorf("flag %q not registered on echo command", name)
		}
	}
}

func TestEchoCmd_InvalidStatusRejected(t *testing.T) {
	tests := []struct {
		name   string
		status int
	}{
		{"too low", 99},
		{"too high", 600},
		{"way too high", 1000},
		{"zero treated as unset is invalid here", 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			oldCfg := cfg
			defer func() { cfg = oldCfg }()

			cfg = &config.Config{
				Port: 0,
				Host: "localhost",
				Echo: config.EchoConfig{
					Status:      tt.status,
					ContentType: "application/json",
				},
				Metrics: config.MetricsConfig{Enabled: false},
			}

			err := runEcho(echoCmd, nil)
			if err == nil {
				t.Fatalf("expected error for status %d, got nil", tt.status)
			}
			if !strings.Contains(err.Error(), "invalid --status") {
				t.Errorf("expected invalid --status error, got %v", err)
			}
		})
	}
}
