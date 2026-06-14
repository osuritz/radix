package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"strconv"
	"syscall"
	"time"

	"github.com/osuritz/radix/internal/metrics"
)

// AdminLoopbackHost is the host the admin server always binds. Telemetry and
// health are intentionally not exposed beyond loopback, even when the app binds
// 0.0.0.0; there is deliberately no config knob to widen this.
const AdminLoopbackHost = "127.0.0.1"

// AdminConfig holds the configuration for the admin server.
//
// The admin server runs alongside the main application server and exposes
// request-oriented telemetry and a liveness endpoint on a dedicated loopback
// port, keeping them off the application listener.
type AdminConfig struct {
	// Port is the TCP port to bind on the loopback interface.
	Port int

	// Collector is the shared metrics collector. The request-recording
	// middleware stays on the app handler; the admin server only EXPOSES this
	// collector's snapshot, so the same collector instance must be passed here.
	Collector *metrics.Collector

	// MetricsPath is the path the metrics endpoint is served at (e.g. "/_metrics").
	MetricsPath string

	// MetricsFormat selects the metrics output format ("json" or "prometheus").
	MetricsFormat string

	// Version is reported in the /healthz body.
	Version string

	// ShutdownTimeout is the maximum duration to wait for in-flight admin
	// requests during graceful shutdown. If zero, DefaultShutdownTimeout is used.
	ShutdownTimeout time.Duration

	// Banner is an optional message printed to Output once the admin listener is
	// bound. If empty, no banner is printed.
	Banner string

	// Output is the writer for startup messages. If nil, os.Stdout is used.
	Output io.Writer
}

// AdminServer is the dedicated, loopback-bound HTTP server that exposes the
// shared metrics collector and a /healthz liveness endpoint. It is shared by all
// radix server commands (serve, proxy, echo, mock) so every command gets the
// admin port wired identically.
//
// Unlike Server, AdminServer installs no signal handler of its own: it is a
// subordinate of the main server and shuts down only when its context is
// canceled or Shutdown is called explicitly. This avoids a second SIGINT/SIGTERM
// handler racing the main server's.
type AdminServer struct {
	httpServer      *http.Server
	listener        net.Listener
	shutdownTimeout time.Duration
	banner          string
	output          io.Writer
	startTime       time.Time
}

// NewAdminServer creates an admin server and immediately binds its loopback
// listener so a port conflict surfaces here, before the caller starts the main
// server. The returned server does not begin serving until Serve is called; the
// caller is responsible for closing it via Shutdown (typically with defer) so a
// failure to start the main server cannot leak the bound listener.
func NewAdminServer(cfg *AdminConfig) (*AdminServer, error) {
	shutdownTimeout := cfg.ShutdownTimeout
	if shutdownTimeout == 0 {
		shutdownTimeout = DefaultShutdownTimeout
	}

	output := cfg.Output
	if output == nil {
		output = os.Stdout
	}

	startTime := time.Now()

	mux := http.NewServeMux()
	if cfg.Collector != nil {
		mux.Handle(cfg.MetricsPath, cfg.Collector.Handler(cfg.MetricsFormat))
	}
	mux.HandleFunc("/healthz", healthzHandler(startTime, cfg.Version))

	addr := net.JoinHostPort(AdminLoopbackHost, strconv.Itoa(cfg.Port))

	// Bind eagerly so a port collision is reported before the main server starts.
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return nil, classifyAdminListenError(addr, err)
	}

	return &AdminServer{
		httpServer: &http.Server{
			Addr:              addr,
			Handler:           mux,
			ReadHeaderTimeout: DefaultReadHeaderTimeout,
		},
		listener:        ln,
		shutdownTimeout: shutdownTimeout,
		banner:          cfg.Banner,
		output:          output,
		startTime:       startTime,
	}, nil
}

// Serve begins serving admin requests on the already-bound listener and blocks
// until the provided context is canceled, then performs a graceful shutdown. It
// returns nil on a clean shutdown (context canceled) and an error only if the
// underlying server fails for a reason other than a normal close.
//
// Serve is intended to run in its own goroutine driven by the command's shared
// context; it installs no signal handler.
func (s *AdminServer) Serve(ctx context.Context) error {
	if s.banner != "" {
		fmt.Fprintln(s.output, s.banner)
	}

	errCh := make(chan error, 1)
	go func() {
		errCh <- s.httpServer.Serve(s.listener)
	}()

	select {
	case <-ctx.Done():
		return s.Shutdown()
	case err := <-errCh:
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			return fmt.Errorf("admin server error: %w", err)
		}
		return nil
	}
}

// Shutdown gracefully shuts the admin server down, waiting up to the configured
// shutdown timeout for in-flight requests. It is safe to call even if Serve was
// never invoked (e.g. the main server failed to start): the eagerly-bound
// listener is closed directly so the admin port is always released, never
// leaked. Shutdown is idempotent.
func (s *AdminServer) Shutdown() error {
	shutdownCtx, cancel := context.WithTimeout(context.Background(), s.shutdownTimeout)
	defer cancel()

	// http.Server.Shutdown only closes listeners it is actively serving; if
	// Serve was never called the eagerly-bound listener would otherwise leak, so
	// close it explicitly. A double close (after Serve already returned) yields
	// ErrClosed, which we ignore to keep Shutdown idempotent.
	shutdownErr := s.httpServer.Shutdown(shutdownCtx)
	if s.listener != nil {
		if closeErr := s.listener.Close(); closeErr != nil && !errors.Is(closeErr, net.ErrClosed) && shutdownErr == nil {
			shutdownErr = closeErr
		}
	}
	if shutdownErr != nil {
		return fmt.Errorf("admin shutdown error: %w", shutdownErr)
	}
	return nil
}

// Addr returns the loopback address the admin server is bound to, including the
// resolved port (useful when the caller requested an ephemeral ":0" port).
func (s *AdminServer) Addr() string {
	if s.listener != nil {
		return s.listener.Addr().String()
	}
	return s.httpServer.Addr
}

// healthzResponse is the liveness payload returned by /healthz.
type healthzResponse struct {
	Status  string `json:"status"`
	Uptime  string `json:"uptime"`
	Version string `json:"version"`
}

// healthzHandler returns a liveness handler that reports uptime and version.
func healthzHandler(startTime time.Time, version string) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(healthzResponse{
			Status:  "ok",
			Uptime:  time.Since(startTime).Round(time.Second).String(),
			Version: version,
		})
	}
}

// classifyAdminListenError gives a friendlier message for common admin-bind
// failures (mirroring Server.classifyError) so a port collision is clear.
func classifyAdminListenError(addr string, err error) error {
	var opErr *net.OpError
	if errors.As(err, &opErr) {
		if errors.Is(opErr.Err, syscall.EADDRINUSE) {
			return fmt.Errorf("admin address %s is already in use: %w", addr, err)
		}
	}
	return fmt.Errorf("failed to bind admin server on %s: %w", addr, err)
}
