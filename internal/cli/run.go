package cli

import (
	"context"
	"fmt"

	"github.com/osuritz/radix/internal/config"
	"github.com/osuritz/radix/internal/metrics"
	"github.com/osuritz/radix/internal/server"
	"github.com/osuritz/radix/internal/version"
)

// buildAdminServer builds the loopback-bound admin server (metrics + /healthz)
// for the given command when metrics are enabled, sharing the supplied
// collector. It returns (nil, nil) when metrics are disabled so the caller can
// run without an admin server. The listener is bound eagerly, so a bind error
// (e.g. the admin port is taken) is reported here before the main server starts.
func buildAdminServer(command string, collector *metrics.Collector) (*server.AdminServer, error) {
	if !cfg.Metrics.Enabled || collector == nil {
		return nil, nil
	}
	admin, err := server.NewAdminServer(&server.AdminConfig{
		Port:          cfg.Metrics.Port,
		Collector:     collector,
		MetricsPath:   cfg.Metrics.Path,
		MetricsFormat: cfg.Metrics.Format,
		Version:       version.Version,
		Banner: fmt.Sprintf("Admin (%s + /healthz) on http://%s:%d%s",
			command, server.AdminLoopbackHost, cfg.Metrics.Port, cfg.Metrics.Path),
	})
	if err != nil {
		return nil, err
	}
	return admin, nil
}

// runServers runs the main application server together with an optional admin
// server (metrics + /healthz) and any optional auxiliary servers (e.g. the
// serve command's HTTP→HTTPS redirect listener), tying their lifecycles to a
// single shared context.
//
// Lifecycle / leak-safety design:
//   - The admin listener is bound eagerly by buildAdminServer, so an admin port
//     collision surfaces before this is called (the main server never starts).
//   - The admin server is always torn down via defer: even if the main server
//     fails to bind, its eagerly-bound listener is released, never leaked.
//   - All servers share one cancelable context. The main server installs the
//     sole SIGINT/SIGTERM handler (via server.Server.Start); when it returns —
//     on signal, ctx cancel, or its own bind error — this cancels the shared
//     context, which stops the admin Serve goroutine and any auxiliary server.
//   - The admin and auxiliary goroutines are joined before returning, so their
//     graceful shutdowns complete (no lingering goroutines or listeners).
//
// The main server's error takes precedence; an admin/auxiliary error is returned
// only when the main server shut down cleanly. Because Server.Start and
// AdminServer.Serve both return nil on a clean ctx-cancel shutdown, a normal
// SIGINT yields no spurious error.
func runServers(ctx context.Context, main *server.Server, admin *server.AdminServer, aux ...*server.Server) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	// Always release the admin listener on the way out, even if the main server
	// fails to start. Shutdown is idempotent, so the explicit join below (which
	// also shuts it down via ctx cancel) does not conflict with this safety net.
	if admin != nil {
		defer func() { _ = admin.Shutdown() }()
	}

	var adminErrCh chan error
	if admin != nil {
		adminErrCh = make(chan error, 1)
		go func() {
			aerr := admin.Serve(ctx)
			if aerr != nil {
				cancel() // an admin failure tears the main server down too
			}
			adminErrCh <- aerr
		}()
	}

	var auxErrChs []chan error
	for _, a := range aux {
		if a == nil {
			continue
		}
		errCh := make(chan error, 1)
		auxErrChs = append(auxErrChs, errCh)
		go func(srv *server.Server) {
			rerr := srv.Start(ctx)
			if rerr != nil {
				cancel() // an auxiliary failure tears the main server down too
			}
			errCh <- rerr
		}(a)
	}

	err := main.Start(ctx)
	cancel()

	// Join the admin goroutine so its graceful shutdown completes before return.
	if adminErrCh != nil {
		if aerr := <-adminErrCh; aerr != nil && err == nil {
			err = fmt.Errorf("admin server: %w", aerr)
		}
	}

	// Join auxiliary goroutines likewise.
	for _, errCh := range auxErrChs {
		if rerr := <-errCh; rerr != nil && err == nil {
			err = rerr
		}
	}

	return err
}

// validateMetricsConfig is a thin wrapper so each server command can fail fast
// on a bad metrics/admin configuration before binding anything.
func validateMetricsConfig() error {
	return config.ValidateMetrics(cfg)
}
