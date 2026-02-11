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
	flags := []string{"dir", "index", "spa", "cors", "gzip", "cache"}
	for _, name := range flags {
		if serveCmd.Flags().Lookup(name) == nil {
			t.Errorf("flag %q not registered on serve command", name)
		}
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
