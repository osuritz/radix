package cli

import (
	"testing"

	"github.com/osuritz/radix/internal/config"
)

func TestProxyCmd_Registered(t *testing.T) {
	found := false
	for _, cmd := range rootCmd.Commands() {
		if cmd.Name() == "proxy" {
			found = true
			break
		}
	}
	if !found {
		t.Error("proxy command not registered on root command")
	}
}

func TestProxyCmd_Flags(t *testing.T) {
	flags := []string{"target", "rewrite", "strip-prefix", "timeout", "flush-interval", "websocket", "tls-skip-verify", "header", "cors"}
	for _, name := range flags {
		if proxyCmd.Flags().Lookup(name) == nil {
			t.Errorf("flag %q not registered on proxy command", name)
		}
	}
}

func TestProxyCmd_AcceptsPositionalArg(t *testing.T) {
	if err := proxyCmd.Args(proxyCmd, []string{"http://localhost:3000"}); err != nil {
		t.Errorf("proxy should accept one positional arg: %v", err)
	}
}

func TestProxyCmd_RejectsTooManyArgs(t *testing.T) {
	if err := proxyCmd.Args(proxyCmd, []string{"a", "b"}); err == nil {
		t.Error("proxy should reject more than one positional arg")
	}
}

func TestProxyCmd_MissingTarget(t *testing.T) {
	oldCfg := cfg
	defer func() { cfg = oldCfg }()

	cfg = &config.Config{
		Port: 0,
		Host: "localhost",
		Proxy: config.ProxyConfig{
			Target: "",
		},
		Metrics: config.MetricsConfig{
			Enabled: false,
		},
	}

	err := runProxy(proxyCmd, nil)
	if err == nil {
		t.Error("expected error when target is empty")
	}
}

func TestProxyCmd_InvalidTargetURL(t *testing.T) {
	oldCfg := cfg
	defer func() { cfg = oldCfg }()

	cfg = &config.Config{
		Port: 0,
		Host: "localhost",
		Proxy: config.ProxyConfig{
			Target: "not-a-url",
		},
		Metrics: config.MetricsConfig{
			Enabled: false,
		},
	}

	err := runProxy(proxyCmd, nil)
	if err == nil {
		t.Error("expected error for invalid target URL (missing scheme)")
	}
}

func TestProxyCmd_TargetWithoutScheme(t *testing.T) {
	oldCfg := cfg
	defer func() { cfg = oldCfg }()

	cfg = &config.Config{
		Port: 0,
		Host: "localhost",
		Proxy: config.ProxyConfig{
			Target: "localhost:3000",
		},
		Metrics: config.MetricsConfig{
			Enabled: false,
		},
	}

	err := runProxy(proxyCmd, nil)
	if err == nil {
		t.Error("expected error for target URL without http/https scheme")
	}
}

func TestProxyCmd_CORSFlagOverridesConfig(t *testing.T) {
	oldCfg := cfg
	oldCORS := proxyCORS
	defer func() {
		cfg = oldCfg
		proxyCORS = oldCORS
		// Reset the flag's Changed state so other tests are unaffected.
		_ = proxyCmd.Flags().Set("cors", "false")
		proxyCmd.Flags().Lookup("cors").Changed = false
	}()

	// Config file has CORS disabled; the --cors flag must override it to true.
	cfg = &config.Config{Proxy: config.ProxyConfig{CORS: false}}
	if err := proxyCmd.Flags().Set("cors", "true"); err != nil {
		t.Fatalf("failed to set --cors flag: %v", err)
	}

	if err := applyProxyFlags(proxyCmd, nil); err != nil {
		t.Fatalf("applyProxyFlags returned error: %v", err)
	}
	if !cfg.Proxy.CORS {
		t.Error("expected --cors flag to override cfg.Proxy.CORS to true")
	}
}

func TestProxyCmd_CORSFromConfigWhenFlagUnset(t *testing.T) {
	oldCfg := cfg
	defer func() { cfg = oldCfg }()

	// No --cors flag set: the config value must be preserved (not clobbered).
	cfg = &config.Config{Proxy: config.ProxyConfig{CORS: true}}
	if err := applyProxyFlags(proxyCmd, nil); err != nil {
		t.Fatalf("applyProxyFlags returned error: %v", err)
	}
	if !cfg.Proxy.CORS {
		t.Error("expected cfg.Proxy.CORS to remain true when --cors flag is unset")
	}
}

func TestProxyCmd_FlagDefaults(t *testing.T) {
	tests := []struct {
		name        string
		flag        string
		wantDefault string
	}{
		{"target", "target", ""},
		{"rewrite", "rewrite", ""},
		{"strip-prefix", "strip-prefix", ""},
		{"timeout", "timeout", ""},
		{"flush-interval", "flush-interval", "-1ns"},
		{"websocket", "websocket", "false"},
		{"tls-skip-verify", "tls-skip-verify", "false"},
		{"cors", "cors", "false"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := proxyCmd.Flags().Lookup(tt.flag)
			if f == nil {
				t.Fatalf("flag %q not found", tt.flag)
			}
			if f.DefValue != tt.wantDefault {
				t.Errorf("flag %q default = %q, want %q", tt.flag, f.DefValue, tt.wantDefault)
			}
		})
	}
}
