package server

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
)

// DefaultShutdownTimeout is the default maximum time to wait for in-flight
// requests to complete during graceful shutdown.
const DefaultShutdownTimeout = 5 * time.Second

// DefaultReadHeaderTimeout is the default timeout for reading request headers.
const DefaultReadHeaderTimeout = 10 * time.Second

// Config holds the configuration for creating a new Server.
type Config struct {
	// Addr is the address to listen on in "host:port" format (e.g. "localhost:8080").
	// Ignored when Listener is set (the listener's own address is used instead).
	Addr string

	// Listener, when non-nil, is an already-bound listener the server serves on
	// instead of binding Addr itself. This lets a caller (notably tests) bind an
	// ephemeral "127.0.0.1:0" port and read the resolved address back via Addr(),
	// avoiding a check-then-bind race. When nil, the server binds Addr itself.
	Listener net.Listener

	// Handler is the HTTP handler that serves requests.
	Handler http.Handler

	// TLSConfig is an optional TLS configuration. When non-nil, the server
	// listens with TLS (HTTPS). The cert and key must already be loaded into
	// the tls.Config (via tls.Config.Certificates or GetCertificate).
	TLSConfig *tls.Config

	// ReadTimeout is the maximum duration for reading the entire request,
	// including the body. Zero means no timeout.
	ReadTimeout time.Duration

	// ReadHeaderTimeout is the maximum duration for reading request headers.
	// If zero, DefaultReadHeaderTimeout is used.
	ReadHeaderTimeout time.Duration

	// WriteTimeout is the maximum duration before timing out writes of the
	// response. Zero means no timeout.
	WriteTimeout time.Duration

	// IdleTimeout is the maximum amount of time to wait for the next request
	// when keep-alives are enabled. Zero means no timeout.
	IdleTimeout time.Duration

	// ShutdownTimeout is the maximum duration to wait for in-flight requests
	// to complete during graceful shutdown. If zero, DefaultShutdownTimeout is used.
	ShutdownTimeout time.Duration

	// Banner is an optional message printed to Output when the server starts.
	// If empty, no banner is printed.
	Banner string

	// Output is the writer for startup/shutdown messages. If nil, os.Stdout is used.
	Output io.Writer
}

// Server wraps an *http.Server with graceful shutdown, signal handling,
// and startup banner support. It is the shared base for all radix server
// commands (serve, proxy, echo, mock).
type Server struct {
	httpServer      *http.Server
	listener        net.Listener
	tlsConfig       *tls.Config
	shutdownTimeout time.Duration
	banner          string
	output          io.Writer
}

// NewServer creates a new Server from the given configuration.
//
// The returned Server is ready to be started with Start. It does not
// listen on any port until Start is called.
func NewServer(cfg *Config) *Server {
	readHeaderTimeout := cfg.ReadHeaderTimeout
	if readHeaderTimeout == 0 {
		readHeaderTimeout = DefaultReadHeaderTimeout
	}

	shutdownTimeout := cfg.ShutdownTimeout
	if shutdownTimeout == 0 {
		shutdownTimeout = DefaultShutdownTimeout
	}

	output := cfg.Output
	if output == nil {
		output = os.Stdout
	}

	srv := &http.Server{
		Addr:              cfg.Addr,
		Handler:           cfg.Handler,
		ReadTimeout:       cfg.ReadTimeout,
		ReadHeaderTimeout: readHeaderTimeout,
		WriteTimeout:      cfg.WriteTimeout,
		IdleTimeout:       cfg.IdleTimeout,
	}

	return &Server{
		httpServer:      srv,
		listener:        cfg.Listener,
		tlsConfig:       cfg.TLSConfig,
		shutdownTimeout: shutdownTimeout,
		banner:          cfg.Banner,
		output:          output,
	}
}

// Start begins listening for HTTP(S) requests and blocks until the server
// shuts down. It installs signal handlers for SIGINT and SIGTERM to trigger
// graceful shutdown.
//
// The provided context controls the server lifetime: when the context is
// canceled, the server initiates graceful shutdown, waiting up to
// ShutdownTimeout for in-flight requests to complete.
//
// Start returns nil on clean shutdown (context canceled or signal received).
// It returns an error if the server fails to start (e.g. port already in use)
// or if graceful shutdown fails.
//
// Start is the sole signal owner: it installs the only SIGINT/SIGTERM handler in
// the process and is reserved for the MAIN server. Subordinate servers (admin,
// HTTP→HTTPS redirect) must use Serve, which is signal-free and shuts down purely
// on context cancellation — otherwise multiple servers would race to handle the
// same signal.
func (s *Server) Start(ctx context.Context) error {
	// Layer signal handling on top of the provided context, then delegate to the
	// signal-free Serve. When a signal arrives this cancels ctx, so Serve shuts
	// the server down gracefully and returns nil.
	ctx, stop := signal.NotifyContext(ctx, syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	return s.Serve(ctx)
}

// Serve begins listening for HTTP(S) requests and blocks until the provided
// context is canceled, then performs a graceful shutdown. Unlike Start, it
// installs NO signal handler: it is the form used by subordinate servers (e.g.
// the HTTP→HTTPS redirect listener) that must shut down only when the shared
// context is canceled, leaving the main server as the single SIGINT/SIGTERM
// owner.
//
// Serve returns nil on clean shutdown (context canceled). It returns an error if
// the server fails to start (e.g. port already in use) or if graceful shutdown
// fails.
func (s *Server) Serve(ctx context.Context) error {
	// Start the server in a goroutine.
	errCh := make(chan error, 1)
	go func() { errCh <- s.listenAndServe() }()

	// Print startup banner after kicking off the listener.
	if s.banner != "" {
		fmt.Fprintln(s.output, s.banner)
	}

	// Wait for context cancellation or server error.
	select {
	case <-ctx.Done():
		fmt.Fprintln(s.output, "\nShutting down...")
		return s.shutdown()
	case err := <-errCh:
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			return s.classifyError(err)
		}
		return nil
	}
}

// listenAndServe begins serving and blocks until the server stops, choosing the
// right primitive for the four combinations of {TLS or not} × {pre-bound
// listener or bind Addr}. It returns http.ErrServerClosed on a normal shutdown.
func (s *Server) listenAndServe() error {
	if s.tlsConfig != nil {
		s.httpServer.TLSConfig = s.tlsConfig
		// Empty cert/key strings: certs are already in TLSConfig.
		if s.listener != nil {
			return s.httpServer.ServeTLS(s.listener, "", "")
		}
		return s.httpServer.ListenAndServeTLS("", "")
	}
	if s.listener != nil {
		return s.httpServer.Serve(s.listener)
	}
	return s.httpServer.ListenAndServe()
}

// shutdown performs a graceful shutdown of the HTTP server, waiting up to
// shutdownTimeout for in-flight requests to complete.
func (s *Server) shutdown() error {
	shutdownCtx, cancel := context.WithTimeout(context.Background(), s.shutdownTimeout)
	defer cancel()

	if err := s.httpServer.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("shutdown error: %w", err)
	}
	return nil
}

// classifyError inspects a server error and returns a more helpful message
// for common failure modes (e.g. port already in use).
func (s *Server) classifyError(err error) error {
	var opErr *net.OpError
	if errors.As(err, &opErr) {
		if errors.Is(opErr.Err, syscall.EADDRINUSE) {
			return fmt.Errorf("address %s is already in use: %w", s.httpServer.Addr, err)
		}
		// Provide a friendlier message for other bind errors
		if opErr.Op == "listen" {
			return fmt.Errorf("failed to listen on %s: %w", s.httpServer.Addr, err)
		}
	}
	return fmt.Errorf("server error: %w", err)
}

// Addr returns the address the server listens on. When a pre-bound listener was
// supplied, this is the listener's resolved address (so an ephemeral ":0" port
// reads back as the actual port); otherwise it is the configured Addr.
func (s *Server) Addr() string {
	if s.listener != nil {
		return s.listener.Addr().String()
	}
	return s.httpServer.Addr
}

// Scheme returns "https" if TLS is configured, "http" otherwise.
func (s *Server) Scheme() string {
	if s.tlsConfig != nil {
		return "https"
	}
	return "http"
}
