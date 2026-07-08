package cli

import (
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/osuritz/radix/internal/config"
	"github.com/spf13/cobra"
)

// newProxyFlagsCmd builds a fresh command carrying the proxy flag set bound to
// the same package globals, so tests can mark flags Changed without mutating
// the shared proxyCmd. Registering the flags resets the globals to defaults;
// callers must save/restore any globals they care about.
func newProxyFlagsCmd() *cobra.Command {
	c := &cobra.Command{Use: "proxy"}
	c.Flags().StringVar(&proxyTarget, "target", "", "")
	c.Flags().StringVar(&proxyRewrite, "rewrite", "", "")
	c.Flags().StringVar(&proxyStripPrefix, "strip-prefix", "", "")
	c.Flags().StringVar(&proxyTimeout, "timeout", "", "")
	c.Flags().DurationVar(&proxyFlushInterval, "flush-interval", -1*time.Nanosecond, "")
	c.Flags().BoolVar(&proxyWebSocket, "websocket", false, "")
	c.Flags().BoolVar(&proxyTLSSkipVerify, "tls-skip-verify", false, "")
	c.Flags().StringArrayVar(&proxyHeaders, "header", nil, "")
	c.Flags().BoolVar(&proxyCORS, "cors", false, "")
	return c
}

// restoreProxyFlagGlobals snapshots all proxy flag globals and restores them
// on test cleanup.
func restoreProxyFlagGlobals(t *testing.T) {
	t.Helper()
	oldTarget, oldRewrite, oldStrip, oldTimeout := proxyTarget, proxyRewrite, proxyStripPrefix, proxyTimeout
	oldFlush, oldWS, oldSkip := proxyFlushInterval, proxyWebSocket, proxyTLSSkipVerify
	oldHeaders, oldCORS := proxyHeaders, proxyCORS
	t.Cleanup(func() {
		proxyTarget, proxyRewrite, proxyStripPrefix, proxyTimeout = oldTarget, oldRewrite, oldStrip, oldTimeout
		proxyFlushInterval, proxyWebSocket, proxyTLSSkipVerify = oldFlush, oldWS, oldSkip
		proxyHeaders, proxyCORS = oldHeaders, oldCORS
	})
}

func TestApplyProxyFlags_AllOverrides(t *testing.T) {
	restoreProxyFlagGlobals(t)
	cmd := newProxyFlagsCmd()

	flagValues := map[string]string{
		"target":          "http://flag.example:9000",
		"rewrite":         "/v1:/v2",
		"strip-prefix":    "/api",
		"timeout":         "45s",
		"flush-interval":  "250ms",
		"websocket":       "true",
		"tls-skip-verify": "true",
		"header":          "X-Custom: value",
		"cors":            "true",
	}
	for name, value := range flagValues {
		if err := cmd.Flags().Set(name, value); err != nil {
			t.Fatalf("set --%s: %v", name, err)
		}
	}

	withCfg(t, &config.Config{Proxy: config.ProxyConfig{Target: "http://file.example"}})

	// Positional arg is applied first, then the explicit --target flag wins.
	if err := applyProxyFlags(cmd, []string{"http://positional.example"}); err != nil {
		t.Fatalf("applyProxyFlags returned error: %v", err)
	}

	p := cfg.Proxy
	if p.Target != "http://flag.example:9000" {
		t.Errorf("Target = %q, want the --target flag value", p.Target)
	}
	if p.Rewrite != "/v1:/v2" {
		t.Errorf("Rewrite = %q, want /v1:/v2", p.Rewrite)
	}
	if p.StripPrefix != "/api" {
		t.Errorf("StripPrefix = %q, want /api", p.StripPrefix)
	}
	if p.Timeout != 45*time.Second {
		t.Errorf("Timeout = %v, want 45s", p.Timeout)
	}
	if p.FlushInterval != 250*time.Millisecond {
		t.Errorf("FlushInterval = %v, want 250ms", p.FlushInterval)
	}
	if !p.WebSocket {
		t.Error("WebSocket = false, want true")
	}
	if !p.TLSSkipVerify {
		t.Error("TLSSkipVerify = false, want true")
	}
	if len(p.Headers) != 1 || p.Headers[0] != "X-Custom: value" {
		t.Errorf("Headers = %v, want [X-Custom: value]", p.Headers)
	}
	if !p.CORS {
		t.Error("CORS = false, want true")
	}
}

func TestApplyProxyFlags_PositionalTargetOnly(t *testing.T) {
	withCfg(t, &config.Config{Proxy: config.ProxyConfig{Target: "http://file.example"}})

	if err := applyProxyFlags(newProxyFlagsCmd(), []string{"http://positional.example"}); err != nil {
		t.Fatalf("applyProxyFlags returned error: %v", err)
	}
	if cfg.Proxy.Target != "http://positional.example" {
		t.Errorf("Target = %q, want the positional value", cfg.Proxy.Target)
	}
}

func TestApplyProxyFlags_InvalidTimeout(t *testing.T) {
	restoreProxyFlagGlobals(t)
	cmd := newProxyFlagsCmd()
	if err := cmd.Flags().Set("timeout", "not-a-duration"); err != nil {
		t.Fatalf("set --timeout: %v", err)
	}

	withCfg(t, &config.Config{})

	err := applyProxyFlags(cmd, nil)
	if err == nil {
		t.Fatal("expected error for invalid --timeout, got nil")
	}
	if !strings.Contains(err.Error(), "invalid timeout") {
		t.Errorf("error = %v, want an invalid-timeout message", err)
	}
}

func TestBuildBackendTLS(t *testing.T) {
	mustURL := func(raw string) *url.URL {
		u, err := url.Parse(raw)
		if err != nil {
			t.Fatalf("parse %q: %v", raw, err)
		}
		return u
	}

	t.Run("plain http target needs no TLS", func(t *testing.T) {
		withCfg(t, &config.Config{})
		tlsCfg, err := buildBackendTLS(mustURL("http://backend:3000"))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if tlsCfg != nil {
			t.Error("expected nil TLS config for plain http backend")
		}
	})

	t.Run("https target builds a TLS config", func(t *testing.T) {
		withCfg(t, &config.Config{})
		tlsCfg, err := buildBackendTLS(mustURL("https://backend:8443"))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if tlsCfg == nil {
			t.Error("expected a TLS config for https backend")
		}
	})

	t.Run("missing backend CA file fails", func(t *testing.T) {
		withCfg(t, &config.Config{
			Proxy: config.ProxyConfig{BackendCA: "/nonexistent/ca.pem"},
		})
		_, err := buildBackendTLS(mustURL("http://backend:3000"))
		if err == nil {
			t.Fatal("expected error for missing backend CA file, got nil")
		}
	})
}

func TestRunProxy_BackendTLSError(t *testing.T) {
	withCfg(t, &config.Config{
		Port: occupiedPort(t),
		Host: "127.0.0.1",
		Proxy: config.ProxyConfig{
			Target:    "https://backend.example",
			BackendCA: "/nonexistent/ca.pem",
		},
		Metrics: config.MetricsConfig{Enabled: false},
	})

	err := runProxy(proxyCmd, nil)
	if err == nil {
		t.Fatal("expected backend TLS configuration error, got nil")
	}
	if !strings.Contains(err.Error(), "backend TLS configuration error") {
		t.Errorf("error = %v, want a backend TLS configuration error", err)
	}
}

func TestRunProxy_FullServerPathBindFailure(t *testing.T) {
	// Static headers select the built-in header provider; verbose + metrics +
	// CORS make runProxy walk the remaining middleware construction lines.
	// Only the bind error is observed here, so this covers line paths, not
	// middleware behavior.
	withCfg(t, &config.Config{
		Port:    occupiedPort(t),
		Host:    "127.0.0.1",
		Verbose: true,
		Proxy: config.ProxyConfig{
			Target:      "http://backend.example:3000",
			Headers:     []string{"X-Injected: yes"},
			CORS:        true,
			StripPrefix: "/api",
			Timeout:     5 * time.Second,
		},
		Metrics: config.MetricsConfig{Enabled: true, Port: freePort(t), Path: "/_metrics", Format: "json"},
	})

	err := runProxy(proxyCmd, nil)
	if err == nil {
		t.Fatal("expected bind error from occupied port, got nil")
	}
	if !strings.Contains(err.Error(), "already in use") {
		t.Errorf("error = %v, want an address-in-use bind failure", err)
	}
}

func TestRunProxy_TLSConfigError(t *testing.T) {
	withCfg(t, &config.Config{
		Port:  occupiedPort(t),
		Host:  "127.0.0.1",
		Proxy: config.ProxyConfig{Target: "http://backend.example:3000"},
		TLS: config.TLSConfig{
			Enabled: true,
			Cert:    "/nonexistent/cert.pem",
			Key:     "/nonexistent/key.pem",
		},
		Metrics: config.MetricsConfig{Enabled: false},
	})

	err := runProxy(proxyCmd, nil)
	if err == nil {
		t.Fatal("expected TLS configuration error, got nil")
	}
	if !strings.Contains(err.Error(), "TLS configuration error") {
		t.Errorf("error = %v, want a TLS configuration error", err)
	}
}

func TestRunProxy_TLSSuccessPathBindFailure(t *testing.T) {
	certPath, keyPath, _ := genTestCerts(t)

	withCfg(t, &config.Config{
		Port:  occupiedPort(t),
		Host:  "127.0.0.1",
		Proxy: config.ProxyConfig{Target: "http://backend.example:3000"},
		TLS: config.TLSConfig{
			Enabled:    true,
			Cert:       certPath,
			Key:        keyPath,
			MinVersion: "1.3",
		},
		Metrics: config.MetricsConfig{Enabled: false},
	})

	err := runProxy(proxyCmd, nil)
	if err == nil {
		t.Fatal("expected bind error from occupied port, got nil")
	}
	if strings.Contains(err.Error(), "TLS configuration error") {
		t.Errorf("valid certs should pass TLS setup; got %v", err)
	}
}
