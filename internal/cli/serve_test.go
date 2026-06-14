package cli

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"testing"
	"time"

	"github.com/osuritz/radix/internal/config"
	"github.com/osuritz/radix/internal/server"
)

// boundListener binds an ephemeral TCP listener on 127.0.0.1:0 and returns it
// together with its resolved address. The listener is registered for cleanup.
// Passing this listener into server.Config.Listener lets a test learn the real
// bound address (via Server.Addr()) without a check-then-bind race: the port is
// held by the listener for the lifetime of the test, never closed-then-rebound.
func boundListener(t *testing.T) (net.Listener, string) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to bind ephemeral listener: %v", err)
	}
	t.Cleanup(func() { _ = ln.Close() })
	return ln, ln.Addr().String()
}

// freePort returns an ephemeral TCP port on 127.0.0.1 that is free at the
// moment of the call. The listener is closed before returning, so there is an
// inherent (small) race window; callers should treat the port as "very likely
// free" and tolerate bind failures only where explicitly testing for them.
//
// Prefer boundListener for the main/auxiliary server lifecycle tests; freePort
// remains only for cases that deliberately need a (likely) free *number*, such
// as asserting a port-collision error.
func freePort(t *testing.T) int {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to reserve ephemeral port: %v", err)
	}
	port := ln.Addr().(*net.TCPAddr).Port
	_ = ln.Close()
	return port
}

// waitForServer polls the given address until a TCP connection succeeds or the
// timeout elapses, returning whether the server became reachable.
func waitForServer(addr string, timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		conn, err := net.DialTimeout("tcp", addr, 100*time.Millisecond)
		if err == nil {
			_ = conn.Close()
			return true
		}
		time.Sleep(10 * time.Millisecond)
	}
	return false
}

func TestRunServers_RedirectsPlainHTTP(t *testing.T) {
	mainLn, _ := boundListener(t)
	mainPort := mainLn.Addr().(*net.TCPAddr).Port
	redirectLn, redirectAddr := boundListener(t)

	mainSrv := server.NewServer(&server.Config{
		Listener: mainLn,
		Handler: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
		}),
		Output: io.Discard,
	})
	redirectSrv := server.NewServer(&server.Config{
		Listener: redirectLn,
		Handler:  server.RedirectToHTTPS(mainPort),
		Output:   io.Discard,
	})

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- runServers(ctx, mainSrv, nil, redirectSrv) }()

	if !waitForServer(redirectAddr, 5*time.Second) {
		cancel()
		<-done
		t.Fatal("redirect server did not become ready")
	}

	client := &http.Client{
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
		Timeout:       3 * time.Second,
	}

	wantLocation := fmt.Sprintf("https://127.0.0.1:%d/path?query=1", mainPort)

	for _, method := range []string{http.MethodGet, http.MethodPost} {
		url := fmt.Sprintf("http://%s/path?query=1", redirectAddr)
		req, err := http.NewRequest(method, url, nil)
		if err != nil {
			cancel()
			<-done
			t.Fatalf("new request (%s): %v", method, err)
		}
		resp, err := client.Do(req)
		if err != nil {
			cancel()
			<-done
			t.Fatalf("request (%s): %v", method, err)
		}
		if resp.StatusCode != http.StatusPermanentRedirect {
			resp.Body.Close()
			cancel()
			<-done
			t.Fatalf("%s status = %d, want %d", method, resp.StatusCode, http.StatusPermanentRedirect)
		}
		if loc := resp.Header.Get("Location"); loc != wantLocation {
			resp.Body.Close()
			cancel()
			<-done
			t.Fatalf("%s Location = %q, want %q", method, loc, wantLocation)
		}
		resp.Body.Close()
	}

	// Cancelling the context must make the orchestrator return promptly.
	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("runServers returned error on clean shutdown: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("runServers did not return after context cancel")
	}
}

func TestRunServers_RedirectFailureTearsDownMain(t *testing.T) {
	mainLn, _ := boundListener(t)
	mainPort := mainLn.Addr().(*net.TCPAddr).Port

	// Force the auxiliary (redirect) server's Serve to fail immediately by handing
	// it an already-closed listener: http.Server.Serve returns an error at once.
	// This is the listener-based analogue of a bind failure and avoids the old
	// check-then-bind race (occupying a port then hoping it stays occupied).
	deadLn, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to bind dead listener: %v", err)
	}
	_ = deadLn.Close()

	mainSrv := server.NewServer(&server.Config{
		Listener: mainLn,
		Handler: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
		}),
		Output: io.Discard,
	})
	redirectSrv := server.NewServer(&server.Config{
		Listener: deadLn,
		Handler:  server.RedirectToHTTPS(mainPort),
		Output:   io.Discard,
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done := make(chan error, 1)
	go func() { done <- runServers(ctx, mainSrv, nil, redirectSrv) }()

	select {
	case err := <-done:
		if err == nil {
			t.Fatal("expected error from redirect serve failure, got nil")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("runServers did not return after redirect serve failure (main not torn down)")
	}
}

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
