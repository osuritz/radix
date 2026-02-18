package server

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"fmt"
	"io"
	"math/big"
	"net"
	"net/http"
	"strings"
	"testing"
	"time"
)

// getFreePort asks the OS for a free port and returns it.
func getFreePort(t *testing.T) int {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to get free port: %v", err)
	}
	port := l.Addr().(*net.TCPAddr).Port
	if err := l.Close(); err != nil {
		t.Fatalf("failed to close listener: %v", err)
	}
	return port
}

// selfSignedTLSConfig generates a self-signed TLS config for testing.
func selfSignedTLSConfig(t *testing.T) *tls.Config {
	t.Helper()

	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("failed to generate key: %v", err)
	}

	template := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{Organization: []string{"Test"}},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		IPAddresses:  []net.IP{net.ParseIP("127.0.0.1")},
		DNSNames:     []string{"localhost"},
	}

	certDER, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		t.Fatalf("failed to create certificate: %v", err)
	}

	cert := tls.Certificate{
		Certificate: [][]byte{certDER},
		PrivateKey:  key,
	}

	return &tls.Config{
		Certificates: []tls.Certificate{cert},
		MinVersion:   tls.VersionTLS12,
	}
}

func TestNewServer_Defaults(t *testing.T) {
	t.Parallel()

	handler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	srv := NewServer(&Config{
		Addr:    "localhost:0",
		Handler: handler,
	})

	if srv.httpServer.ReadHeaderTimeout != DefaultReadHeaderTimeout {
		t.Errorf("ReadHeaderTimeout = %v, want %v", srv.httpServer.ReadHeaderTimeout, DefaultReadHeaderTimeout)
	}
	if srv.shutdownTimeout != DefaultShutdownTimeout {
		t.Errorf("ShutdownTimeout = %v, want %v", srv.shutdownTimeout, DefaultShutdownTimeout)
	}
	if srv.output == nil {
		t.Error("Output should default to a non-nil writer")
	}
}

func TestNewServer_CustomTimeouts(t *testing.T) {
	t.Parallel()

	srv := NewServer(&Config{
		Addr:              "localhost:0",
		Handler:           http.NotFoundHandler(),
		ReadTimeout:       1 * time.Second,
		ReadHeaderTimeout: 2 * time.Second,
		WriteTimeout:      3 * time.Second,
		IdleTimeout:       4 * time.Second,
		ShutdownTimeout:   5 * time.Second,
	})

	if srv.httpServer.ReadTimeout != 1*time.Second {
		t.Errorf("ReadTimeout = %v, want %v", srv.httpServer.ReadTimeout, 1*time.Second)
	}
	if srv.httpServer.ReadHeaderTimeout != 2*time.Second {
		t.Errorf("ReadHeaderTimeout = %v, want %v", srv.httpServer.ReadHeaderTimeout, 2*time.Second)
	}
	if srv.httpServer.WriteTimeout != 3*time.Second {
		t.Errorf("WriteTimeout = %v, want %v", srv.httpServer.WriteTimeout, 3*time.Second)
	}
	if srv.httpServer.IdleTimeout != 4*time.Second {
		t.Errorf("IdleTimeout = %v, want %v", srv.httpServer.IdleTimeout, 4*time.Second)
	}
	if srv.shutdownTimeout != 5*time.Second {
		t.Errorf("ShutdownTimeout = %v, want %v", srv.shutdownTimeout, 5*time.Second)
	}
}

func TestServer_Scheme(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		tlsConfig *tls.Config
		want      string
	}{
		{
			name:      "no TLS",
			tlsConfig: nil,
			want:      "http",
		},
		{
			name:      "with TLS",
			tlsConfig: &tls.Config{MinVersion: tls.VersionTLS12},
			want:      "https",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			srv := NewServer(&Config{
				Addr:      "localhost:0",
				Handler:   http.NotFoundHandler(),
				TLSConfig: tt.tlsConfig,
			})
			if got := srv.Scheme(); got != tt.want {
				t.Errorf("Scheme() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestServer_Addr(t *testing.T) {
	t.Parallel()

	srv := NewServer(&Config{
		Addr:    "127.0.0.1:9999",
		Handler: http.NotFoundHandler(),
	})

	if got := srv.Addr(); got != "127.0.0.1:9999" {
		t.Errorf("Addr() = %q, want %q", got, "127.0.0.1:9999")
	}
}

func TestServer_StartAndAcceptConnections(t *testing.T) {
	t.Parallel()

	port := getFreePort(t)
	addr := fmt.Sprintf("127.0.0.1:%d", port)

	handler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, "ok")
	})

	var buf bytes.Buffer
	srv := NewServer(&Config{
		Addr:    addr,
		Handler: handler,
		Banner:  fmt.Sprintf("Serving on http://%s", addr),
		Output:  &buf,
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Start server in background.
	errCh := make(chan error, 1)
	go func() {
		errCh <- srv.Start(ctx)
	}()

	// Wait for server to be ready.
	waitForServer(t, "http://"+addr)

	// Make a request.
	resp, err := http.Get("http://" + addr + "/test")
	if err != nil {
		t.Fatalf("GET request failed: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("status = %d, want %d", resp.StatusCode, http.StatusOK)
	}

	body, _ := io.ReadAll(resp.Body)
	if string(body) != "ok" {
		t.Errorf("body = %q, want %q", string(body), "ok")
	}

	// Shut down.
	cancel()

	select {
	case err := <-errCh:
		if err != nil {
			t.Errorf("Start() returned error: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for server to stop")
	}
}

func TestServer_StartBanner(t *testing.T) {
	t.Parallel()

	port := getFreePort(t)
	addr := fmt.Sprintf("127.0.0.1:%d", port)

	var buf bytes.Buffer
	srv := NewServer(&Config{
		Addr:    addr,
		Handler: http.NotFoundHandler(),
		Banner:  "Test banner message",
		Output:  &buf,
	})

	ctx, cancel := context.WithCancel(context.Background())
	errCh := make(chan error, 1)
	go func() {
		errCh <- srv.Start(ctx)
	}()

	waitForServer(t, "http://"+addr)
	cancel()

	<-errCh

	output := buf.String()
	if !strings.Contains(output, "Test banner message") {
		t.Errorf("banner not printed, got output: %q", output)
	}
}

func TestServer_NoBanner(t *testing.T) {
	t.Parallel()

	port := getFreePort(t)
	addr := fmt.Sprintf("127.0.0.1:%d", port)

	var buf bytes.Buffer
	srv := NewServer(&Config{
		Addr:    addr,
		Handler: http.NotFoundHandler(),
		Banner:  "",
		Output:  &buf,
	})

	ctx, cancel := context.WithCancel(context.Background())
	errCh := make(chan error, 1)
	go func() {
		errCh <- srv.Start(ctx)
	}()

	waitForServer(t, "http://"+addr)
	cancel()

	<-errCh

	output := buf.String()
	// Should only contain the shutdown message, not a banner line.
	lines := strings.TrimSpace(output)
	if strings.Contains(lines, "\n\n") {
		t.Errorf("unexpected banner output: %q", output)
	}
}

func TestServer_GracefulShutdownOnContextCancel(t *testing.T) {
	t.Parallel()

	port := getFreePort(t)
	addr := fmt.Sprintf("127.0.0.1:%d", port)

	var buf bytes.Buffer
	srv := NewServer(&Config{
		Addr:    addr,
		Handler: http.NotFoundHandler(),
		Output:  &buf,
	})

	ctx, cancel := context.WithCancel(context.Background())
	errCh := make(chan error, 1)
	go func() {
		errCh <- srv.Start(ctx)
	}()

	waitForServer(t, "http://"+addr)

	// Cancel context to trigger shutdown.
	cancel()

	select {
	case err := <-errCh:
		if err != nil {
			t.Errorf("Start() returned error on graceful shutdown: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for graceful shutdown")
	}

	if !strings.Contains(buf.String(), "Shutting down") {
		t.Errorf("shutdown message not printed, got: %q", buf.String())
	}
}

func TestServer_PortConflict(t *testing.T) {
	t.Parallel()

	port := getFreePort(t)
	addr := fmt.Sprintf("127.0.0.1:%d", port)

	// Occupy the port.
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		t.Fatalf("failed to occupy port: %v", err)
	}
	defer func() { _ = listener.Close() }()

	var buf bytes.Buffer
	srv := NewServer(&Config{
		Addr:    addr,
		Handler: http.NotFoundHandler(),
		Output:  &buf,
	})

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	err = srv.Start(ctx)
	if err == nil {
		t.Fatal("expected error when port is already in use, got nil")
	}

	if !strings.Contains(err.Error(), "already in use") && !strings.Contains(err.Error(), "server error") {
		t.Errorf("error should indicate port conflict, got: %v", err)
	}
}

func TestServer_TLS(t *testing.T) {
	t.Parallel()

	port := getFreePort(t)
	addr := fmt.Sprintf("127.0.0.1:%d", port)

	handler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprint(w, "tls-ok")
	})

	tlsCfg := selfSignedTLSConfig(t)

	var buf bytes.Buffer
	srv := NewServer(&Config{
		Addr:      addr,
		Handler:   handler,
		TLSConfig: tlsCfg,
		Banner:    fmt.Sprintf("Serving on https://%s", addr),
		Output:    &buf,
	})

	if srv.Scheme() != "https" {
		t.Errorf("Scheme() = %q, want %q", srv.Scheme(), "https")
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	errCh := make(chan error, 1)
	go func() {
		errCh <- srv.Start(ctx)
	}()

	// Wait for TLS server with a client that skips verification.
	client := &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{
				InsecureSkipVerify: true, //#nosec G402 - test only
			},
		},
		Timeout: 2 * time.Second,
	}

	waitForServerWithClient(t, "https://"+addr, client, 3*time.Second)

	resp, err := client.Get("https://" + addr + "/test")
	if err != nil {
		t.Fatalf("GET request failed: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, _ := io.ReadAll(resp.Body)
	if string(body) != "tls-ok" {
		t.Errorf("body = %q, want %q", string(body), "tls-ok")
	}

	cancel()

	select {
	case err := <-errCh:
		if err != nil {
			t.Errorf("Start() returned error: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for server to stop")
	}
}

func TestServer_ShutdownCompletesInFlightRequests(t *testing.T) {
	t.Parallel()

	port := getFreePort(t)
	addr := fmt.Sprintf("127.0.0.1:%d", port)

	requestStarted := make(chan struct{})
	requestFinish := make(chan struct{})

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/slow" {
			close(requestStarted)
			<-requestFinish
			w.WriteHeader(http.StatusOK)
			fmt.Fprint(w, "completed")
			return
		}
		// Health-check path for waitForServer.
		w.WriteHeader(http.StatusOK)
	})

	var buf bytes.Buffer
	srv := NewServer(&Config{
		Addr:            addr,
		Handler:         handler,
		ShutdownTimeout: 5 * time.Second,
		Output:          &buf,
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	errCh := make(chan error, 1)
	go func() {
		errCh <- srv.Start(ctx)
	}()

	// Use a client that does not reuse connections to avoid lingering goroutines.
	client := &http.Client{
		Transport: &http.Transport{
			DisableKeepAlives: true,
		},
	}

	waitForServerWithClient(t, "http://"+addr, client, 2*time.Second)

	// Start an in-flight request.
	type result struct {
		body string
		err  error
	}
	resultCh := make(chan result, 1)
	go func() {
		resp, err := client.Get("http://" + addr + "/slow")
		if err != nil {
			resultCh <- result{err: err}
			return
		}
		body, _ := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		resultCh <- result{body: string(body)}
	}()

	// Wait for the handler to start processing.
	<-requestStarted

	// Trigger shutdown while request is in-flight.
	cancel()

	// Let the request complete after a brief pause so shutdown has started.
	time.Sleep(50 * time.Millisecond)
	close(requestFinish)

	// The in-flight request should complete successfully.
	select {
	case r := <-resultCh:
		if r.err != nil {
			t.Fatalf("in-flight request error: %v", r.err)
		}
		if r.body != "completed" {
			t.Errorf("in-flight request body = %q, want %q", r.body, "completed")
		}
	case <-time.After(10 * time.Second):
		t.Fatal("timed out waiting for in-flight request")
	}

	select {
	case err := <-errCh:
		if err != nil {
			t.Errorf("Start() returned error: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for server to stop")
	}
}

func TestServer_ClassifyError_AddressInUse(t *testing.T) {
	t.Parallel()

	srv := &Server{
		httpServer: &http.Server{Addr: "127.0.0.1:8080"},
	}

	// Simulate an EADDRINUSE error.
	opErr := &net.OpError{
		Op:  "listen",
		Net: "tcp",
		Addr: &net.TCPAddr{
			IP:   net.ParseIP("127.0.0.1"),
			Port: 8080,
		},
		Err: &net.AddrError{
			Err:  "bind: address already in use",
			Addr: "127.0.0.1:8080",
		},
	}

	err := srv.classifyError(opErr)
	if !strings.Contains(err.Error(), "failed to listen on 127.0.0.1:8080") {
		t.Errorf("error should reference the address, got: %v", err)
	}
}

func TestServer_ClassifyError_Generic(t *testing.T) {
	t.Parallel()

	srv := &Server{
		httpServer: &http.Server{Addr: "localhost:8080"},
	}

	err := srv.classifyError(fmt.Errorf("something unexpected"))
	if !strings.Contains(err.Error(), "server error") {
		t.Errorf("error should wrap as server error, got: %v", err)
	}
}

// waitForServer polls the given URL until it responds or a 2-second timeout is reached.
func waitForServer(t *testing.T, url string) {
	t.Helper()
	waitForServerWithClient(t, url, http.DefaultClient, 2*time.Second)
}

// waitForServerWithClient polls the given URL with the provided client.
func waitForServerWithClient(t *testing.T, url string, client *http.Client, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		resp, err := client.Get(url)
		if err == nil {
			_ = resp.Body.Close()
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("server at %s did not become ready within %v", url, timeout)
}
