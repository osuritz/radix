package server

import (
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/osuritz/radix/internal/metrics"
)

// startAdmin builds an admin server on an ephemeral loopback port, serves it on
// a background goroutine, and waits until it is reachable. It returns the server
// (for its resolved Addr), a cancel func, and a channel carrying Serve's result.
func startAdmin(t *testing.T, cfg *AdminConfig) (*AdminServer, context.CancelFunc, chan error) {
	t.Helper()
	if cfg.Output == nil {
		cfg.Output = io.Discard
	}
	admin, err := NewAdminServer(cfg)
	if err != nil {
		t.Fatalf("NewAdminServer: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	errCh := make(chan error, 1)
	go func() { errCh <- admin.Serve(ctx) }()

	if !waitForTCP(admin.Addr(), 2*time.Second) {
		cancel()
		<-errCh
		t.Fatalf("admin server at %s did not become ready", admin.Addr())
	}
	return admin, cancel, errCh
}

// waitForTCP polls until a TCP connect to addr succeeds or the timeout elapses.
func waitForTCP(addr string, timeout time.Duration) bool {
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

func TestAdminServer_BindsLoopback(t *testing.T) {
	t.Parallel()

	collector := metrics.NewCollector("test", "1.2.3")
	admin, cancel, errCh := startAdmin(t, &AdminConfig{
		Port:          0, // ephemeral
		Collector:     collector,
		MetricsPath:   "/_metrics",
		MetricsFormat: "json",
		Version:       "1.2.3",
	})
	defer func() {
		cancel()
		<-errCh
	}()

	host, _, err := net.SplitHostPort(admin.Addr())
	if err != nil {
		t.Fatalf("SplitHostPort(%q): %v", admin.Addr(), err)
	}
	if host != AdminLoopbackHost {
		t.Errorf("admin bound host = %q, want %q", host, AdminLoopbackHost)
	}
}

func TestAdminServer_Healthz(t *testing.T) {
	t.Parallel()

	admin, cancel, errCh := startAdmin(t, &AdminConfig{
		Port:          0,
		Collector:     metrics.NewCollector("test", "9.9.9"),
		MetricsPath:   "/_metrics",
		MetricsFormat: "json",
		Version:       "9.9.9",
	})
	defer func() {
		cancel()
		<-errCh
	}()

	resp, err := http.Get("http://" + admin.Addr() + "/healthz")
	if err != nil {
		t.Fatalf("GET /healthz: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("status = %d, want %d", resp.StatusCode, http.StatusOK)
	}
	if ct := resp.Header.Get("Content-Type"); !strings.Contains(ct, "application/json") {
		t.Errorf("Content-Type = %q, want application/json", ct)
	}

	var body healthzResponse
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if body.Status != "ok" {
		t.Errorf("status field = %q, want %q", body.Status, "ok")
	}
	if body.Version != "9.9.9" {
		t.Errorf("version field = %q, want %q", body.Version, "9.9.9")
	}
	if body.Uptime == "" {
		t.Error("uptime field is empty")
	}
}

func TestAdminServer_MetricsJSON(t *testing.T) {
	t.Parallel()

	collector := metrics.NewCollector("test", "1.0.0")
	collector.RecordRequest(200, "GET", 5*time.Millisecond, 10, 20)

	admin, cancel, errCh := startAdmin(t, &AdminConfig{
		Port:          0,
		Collector:     collector,
		MetricsPath:   "/_metrics",
		MetricsFormat: "json",
		Version:       "1.0.0",
	})
	defer func() {
		cancel()
		<-errCh
	}()

	resp, err := http.Get("http://" + admin.Addr() + "/_metrics")
	if err != nil {
		t.Fatalf("GET /_metrics: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("status = %d, want %d", resp.StatusCode, http.StatusOK)
	}
	if ct := resp.Header.Get("Content-Type"); !strings.Contains(ct, "application/json") {
		t.Errorf("Content-Type = %q, want application/json", ct)
	}

	data, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(data), `"total": 1`) {
		t.Errorf("metrics JSON should report the recorded request, got: %s", data)
	}
}

func TestAdminServer_MetricsPrometheus(t *testing.T) {
	t.Parallel()

	collector := metrics.NewCollector("test", "1.0.0")
	collector.RecordRequest(200, "GET", 5*time.Millisecond, 10, 20)

	admin, cancel, errCh := startAdmin(t, &AdminConfig{
		Port:          0,
		Collector:     collector,
		MetricsPath:   "/_metrics",
		MetricsFormat: "prometheus",
		Version:       "1.0.0",
	})
	defer func() {
		cancel()
		<-errCh
	}()

	resp, err := http.Get("http://" + admin.Addr() + "/_metrics")
	if err != nil {
		t.Fatalf("GET /_metrics: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if ct := resp.Header.Get("Content-Type"); !strings.Contains(ct, "text/plain") {
		t.Errorf("Content-Type = %q, want text/plain (Prometheus)", ct)
	}
	data, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(data), "radix_") {
		t.Errorf("Prometheus output should contain radix_ metrics, got: %s", data)
	}
}

func TestAdminServer_CustomMetricsPath(t *testing.T) {
	t.Parallel()

	admin, cancel, errCh := startAdmin(t, &AdminConfig{
		Port:          0,
		Collector:     metrics.NewCollector("test", "1.0.0"),
		MetricsPath:   "/stats",
		MetricsFormat: "json",
		Version:       "1.0.0",
	})
	defer func() {
		cancel()
		<-errCh
	}()

	resp, err := http.Get("http://" + admin.Addr() + "/stats")
	if err != nil {
		t.Fatalf("GET /stats: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("status = %d, want %d", resp.StatusCode, http.StatusOK)
	}
}

func TestAdminServer_PortConflict(t *testing.T) {
	t.Parallel()

	// Occupy a loopback port, then try to bind the admin server to it.
	occupied, err := net.Listen("tcp", AdminLoopbackHost+":0")
	if err != nil {
		t.Fatalf("failed to occupy port: %v", err)
	}
	defer func() { _ = occupied.Close() }()
	port := occupied.Addr().(*net.TCPAddr).Port

	_, err = NewAdminServer(&AdminConfig{
		Port:          port,
		Collector:     metrics.NewCollector("test", "1.0.0"),
		MetricsPath:   "/_metrics",
		MetricsFormat: "json",
		Output:        io.Discard,
	})
	if err == nil {
		t.Fatal("expected bind error on port conflict, got nil")
	}
	if !strings.Contains(err.Error(), "admin") {
		t.Errorf("error should mention the admin server, got: %v", err)
	}
}

func TestAdminServer_GracefulShutdownOnContextCancel(t *testing.T) {
	t.Parallel()

	admin, cancel, errCh := startAdmin(t, &AdminConfig{
		Port:          0,
		Collector:     metrics.NewCollector("test", "1.0.0"),
		MetricsPath:   "/_metrics",
		MetricsFormat: "json",
		Version:       "1.0.0",
	})

	cancel()
	select {
	case err := <-errCh:
		if err != nil {
			t.Errorf("Serve returned error on clean shutdown: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("admin server did not shut down after context cancel")
	}

	// After shutdown the listener should be released.
	if waitForTCP(admin.Addr(), 200*time.Millisecond) {
		t.Error("admin listener still reachable after shutdown")
	}
}

func TestAdminServer_ShutdownWithoutServe(t *testing.T) {
	t.Parallel()

	// Simulates the main server failing to start: the admin listener was bound
	// eagerly but Serve was never called. Shutdown must still release it.
	admin, err := NewAdminServer(&AdminConfig{
		Port:          0,
		Collector:     metrics.NewCollector("test", "1.0.0"),
		MetricsPath:   "/_metrics",
		MetricsFormat: "json",
		Output:        io.Discard,
	})
	if err != nil {
		t.Fatalf("NewAdminServer: %v", err)
	}
	addr := admin.Addr()

	if err := admin.Shutdown(); err != nil {
		t.Fatalf("Shutdown: %v", err)
	}
	if waitForTCP(addr, 200*time.Millisecond) {
		t.Error("admin listener still reachable after Shutdown without Serve")
	}

	// Shutdown is idempotent.
	if err := admin.Shutdown(); err != nil {
		t.Errorf("second Shutdown returned error: %v", err)
	}
}
