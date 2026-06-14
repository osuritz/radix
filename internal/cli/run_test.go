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
	"github.com/osuritz/radix/internal/metrics"
	"github.com/osuritz/radix/internal/server"
	"github.com/osuritz/radix/internal/server/middleware"
)

// withCfg swaps the package-level cfg for the duration of a test.
func withCfg(t *testing.T, c *config.Config) {
	t.Helper()
	old := cfg
	cfg = c
	t.Cleanup(func() { cfg = old })
}

func TestRunServers_AdminExposesMetricsAndHealthz(t *testing.T) {
	appPort := freePort(t)
	adminPort := freePort(t)

	withCfg(t, &config.Config{
		Port: appPort,
		Host: "127.0.0.1",
		Metrics: config.MetricsConfig{
			Enabled: true,
			Path:    "/_metrics",
			Format:  "json",
			Port:    adminPort,
		},
	})

	collector := metrics.NewCollector("test", "1.0.0")

	// The app handler records metrics and serves a trivial app route. Crucially
	// it does NOT mount /_metrics — that now lives only on the admin port.
	appMux := http.NewServeMux()
	appMux.HandleFunc("/", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, "app")
	})
	appHandler := middleware.Metrics(collector)(appMux)

	mainSrv := server.NewServer(&server.Config{
		Addr:    net.JoinHostPort("127.0.0.1", fmt.Sprintf("%d", appPort)),
		Handler: appHandler,
		Output:  io.Discard,
	})

	admin, err := buildAdminServer("test", collector)
	if err != nil {
		t.Fatalf("buildAdminServer: %v", err)
	}
	if admin == nil {
		t.Fatal("expected an admin server when metrics enabled")
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- runServers(ctx, mainSrv, admin) }()

	appAddr := net.JoinHostPort("127.0.0.1", fmt.Sprintf("%d", appPort))
	adminAddr := admin.Addr()
	if !waitForServer(appAddr, 5*time.Second) || !waitForServer(adminAddr, 5*time.Second) {
		cancel()
		<-done
		t.Fatal("servers did not become ready")
	}

	// Drive one request through the app so the collector records it.
	if resp, gerr := http.Get("http://" + appAddr + "/"); gerr == nil {
		_ = resp.Body.Close()
	} else {
		t.Fatalf("app request failed: %v", gerr)
	}

	// The admin port serves /healthz.
	healthz, err := http.Get("http://" + adminAddr + "/healthz")
	if err != nil {
		t.Fatalf("GET admin /healthz: %v", err)
	}
	_ = healthz.Body.Close()
	if healthz.StatusCode != http.StatusOK {
		t.Errorf("/healthz status = %d, want %d", healthz.StatusCode, http.StatusOK)
	}

	// The admin port serves /_metrics.
	adminMetrics, err := http.Get("http://" + adminAddr + "/_metrics")
	if err != nil {
		t.Fatalf("GET admin /_metrics: %v", err)
	}
	_ = adminMetrics.Body.Close()
	if adminMetrics.StatusCode != http.StatusOK {
		t.Errorf("admin /_metrics status = %d, want %d", adminMetrics.StatusCode, http.StatusOK)
	}

	// The app port no longer serves /_metrics: the app mux falls through to "/",
	// which returns the app body, not metrics. We assert it is NOT the metrics
	// payload by checking the body equals the app response.
	appMetrics, err := http.Get("http://" + appAddr + "/_metrics")
	if err != nil {
		t.Fatalf("GET app /_metrics: %v", err)
	}
	body, _ := io.ReadAll(appMetrics.Body)
	_ = appMetrics.Body.Close()
	if string(body) != "app" {
		t.Errorf("app port should not serve metrics; /_metrics body = %q, want %q", string(body), "app")
	}

	// Both servers shut down cleanly on context cancel.
	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Errorf("runServers returned error on clean shutdown: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("runServers did not return after context cancel")
	}

	// The admin listener should be released after shutdown.
	if waitForServer(adminAddr, 200*time.Millisecond) {
		t.Error("admin listener still reachable after shutdown")
	}
}

func TestRunServers_MainBindFailureReleasesAdmin(t *testing.T) {
	adminPort := freePort(t)

	// Occupy a port and point the main server at it so its bind fails.
	occupied, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("occupy port: %v", err)
	}
	defer func() { _ = occupied.Close() }()
	appPort := occupied.Addr().(*net.TCPAddr).Port

	withCfg(t, &config.Config{
		Port: appPort,
		Host: "127.0.0.1",
		Metrics: config.MetricsConfig{
			Enabled: true,
			Path:    "/_metrics",
			Format:  "json",
			Port:    adminPort,
		},
	})

	collector := metrics.NewCollector("test", "1.0.0")
	admin, err := buildAdminServer("test", collector)
	if err != nil {
		t.Fatalf("buildAdminServer: %v", err)
	}
	adminAddr := admin.Addr()

	mainSrv := server.NewServer(&server.Config{
		Addr:    net.JoinHostPort("127.0.0.1", fmt.Sprintf("%d", appPort)),
		Handler: http.NotFoundHandler(),
		Output:  io.Discard,
	})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err = runServers(ctx, mainSrv, admin)
	if err == nil {
		t.Fatal("expected error from main bind failure, got nil")
	}

	// The eagerly-bound admin listener must have been released, not leaked.
	if waitForServer(adminAddr, 500*time.Millisecond) {
		t.Error("admin listener leaked after main server bind failure")
	}
}

func TestBuildAdminServer_DisabledMetrics(t *testing.T) {
	withCfg(t, &config.Config{
		Port:    freePort(t),
		Metrics: config.MetricsConfig{Enabled: false},
	})

	admin, err := buildAdminServer("test", nil)
	if err != nil {
		t.Fatalf("buildAdminServer: %v", err)
	}
	if admin != nil {
		t.Error("expected nil admin server when metrics disabled")
		_ = admin.Shutdown()
	}
}

func TestServerCommands_RejectMetricsPortCollision(t *testing.T) {
	port := freePort(t)

	for _, tc := range []struct {
		name string
		run  func() error
		cfg  *config.Config
	}{
		{
			name: "echo",
			cfg: &config.Config{
				Port:    port,
				Host:    "127.0.0.1",
				Echo:    config.EchoConfig{Status: 200},
				Metrics: config.MetricsConfig{Enabled: true, Port: port, Path: "/_metrics", Format: "json"},
			},
			run: func() error { return runEcho(echoCmd, nil) },
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			withCfg(t, tc.cfg)
			err := tc.run()
			if err == nil {
				t.Fatal("expected error on metrics/app port collision, got nil")
			}
		})
	}
}
