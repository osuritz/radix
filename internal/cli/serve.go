package cli

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"

	"github.com/osuritz/radix/internal/config"
	"github.com/osuritz/radix/internal/metrics"
	"github.com/osuritz/radix/internal/server"
	"github.com/osuritz/radix/internal/server/middleware"
	"github.com/osuritz/radix/internal/tls"
	"github.com/osuritz/radix/internal/version"
	"github.com/spf13/cobra"
)

var (
	serveDir          string
	serveIndex        string
	serveSPA          bool
	serveCORS         bool
	serveGzip         bool
	serveCache        string
	serveHSTS         bool
	serveHSTSMaxAge   int
	serveHTTPRedirect bool
	serveHTTPPort     int
)

var serveCmd = &cobra.Command{
	Use:   "serve [directory]",
	Short: "Serve static files over HTTP",
	Long: `Serve static files from a directory over HTTP(S).

Supports SPA routing, CORS headers, gzip compression, TLS,
directory listing, and metrics.

Examples:
  radix serve                          # Serve current directory on :8080
  radix serve ./dist                   # Serve ./dist directory
  radix serve --spa --port 3000        # SPA mode on port 3000
  radix serve --cors --gzip            # Enable CORS and gzip
  radix serve --tls --cert c.pem --key k.pem  # HTTPS`,
	Args: cobra.MaximumNArgs(1),
	RunE: runServe,
}

func init() {
	serveCmd.Flags().StringVarP(&serveDir, "dir", "d", "", "directory to serve (default: current directory)")
	serveCmd.Flags().StringVar(&serveIndex, "index", "", "index file name (default: index.html)")
	serveCmd.Flags().BoolVar(&serveSPA, "spa", false, "single page application mode")
	serveCmd.Flags().BoolVar(&serveCORS, "cors", false, "enable CORS headers")
	serveCmd.Flags().BoolVar(&serveGzip, "gzip", false, "enable gzip compression")
	serveCmd.Flags().StringVar(&serveCache, "cache", "", "Cache-Control header value")
	serveCmd.Flags().BoolVar(&serveHSTS, "hsts", false, "send Strict-Transport-Security header (requires --tls)")
	serveCmd.Flags().IntVar(&serveHSTSMaxAge, "hsts-max-age", 31536000, "HSTS max-age in seconds")
	serveCmd.Flags().BoolVar(&serveHTTPRedirect, "http-redirect", false, "redirect plain HTTP to HTTPS (requires --tls)")
	serveCmd.Flags().IntVar(&serveHTTPPort, "http-port", 8080, "port for the HTTP→HTTPS redirect listener")
}

// applyServeFlagOverrides overrides the loaded serve config with any
// command-line flags that were explicitly set.
func applyServeFlagOverrides(cmd *cobra.Command, args []string) {
	if len(args) > 0 {
		cfg.Serve.Dir = args[0]
	}
	if cmd.Flags().Changed("dir") {
		cfg.Serve.Dir = serveDir
	}
	if cmd.Flags().Changed("index") {
		cfg.Serve.Index = serveIndex
	}
	if cmd.Flags().Changed("spa") {
		cfg.Serve.SPA = serveSPA
	}
	if cmd.Flags().Changed("cors") {
		cfg.Serve.CORS = serveCORS
	}
	if cmd.Flags().Changed("gzip") {
		cfg.Serve.Gzip = serveGzip
	}
	if cmd.Flags().Changed("cache") {
		cfg.Serve.Cache = serveCache
	}
	if cmd.Flags().Changed("hsts") {
		cfg.Serve.HSTS = serveHSTS
	}
	if cmd.Flags().Changed("hsts-max-age") {
		cfg.Serve.HSTSMaxAge = serveHSTSMaxAge
	}
	if cmd.Flags().Changed("http-redirect") {
		cfg.Serve.HTTPRedirect = serveHTTPRedirect
	}
	if cmd.Flags().Changed("http-port") {
		cfg.Serve.HTTPPort = serveHTTPPort
	}
}

func runServe(cmd *cobra.Command, args []string) error {
	applyServeFlagOverrides(cmd, args)

	if err := config.ValidateServeTLS(cfg); err != nil {
		return err
	}

	// Resolve directory to absolute path
	dir, err := filepath.Abs(cfg.Serve.Dir)
	if err != nil {
		return fmt.Errorf("invalid directory path: %w", err)
	}

	info, err := os.Stat(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("directory does not exist: %s", dir)
		}
		return fmt.Errorf("cannot access directory: %w", err)
	}
	if !info.IsDir() {
		return fmt.Errorf("not a directory: %s", dir)
	}

	// Build file server handler
	fileHandler := server.NewFileServer(server.FileServerConfig{
		Dir:   dir,
		Index: cfg.Serve.Index,
		SPA:   cfg.Serve.SPA,
	})

	// Build handler chain using a mux
	mux := http.NewServeMux()

	// Set up metrics if enabled
	var collector *metrics.Collector
	if cfg.Metrics.Enabled {
		collector = metrics.NewCollector("serve", version.Version)
		mux.Handle(cfg.Metrics.Path, collector.Handler(cfg.Metrics.Format))
	}

	// Wrap file handler with Cache-Control if configured
	handler := fileHandler
	if cfg.Serve.Cache != "" {
		cacheVal := cfg.Serve.Cache
		handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Cache-Control", cacheVal)
			fileHandler.ServeHTTP(w, r)
		})
	}

	mux.Handle("/", handler)

	// Apply middleware chain (outermost first)
	var finalHandler http.Handler = mux

	if cfg.Serve.Gzip {
		finalHandler = middleware.Gzip()(finalHandler)
	}
	if cfg.Serve.CORS {
		finalHandler = middleware.CORS()(finalHandler)
	}
	if cfg.Serve.HSTS {
		finalHandler = middleware.HSTS(cfg.Serve.HSTSMaxAge)(finalHandler)
	}

	logCfg := middleware.LoggingConfig{
		Format:  middleware.LogFormatDev,
		NoColor: cfg.NoColor,
	}
	if cfg.Verbose {
		logCfg.Format = middleware.LogFormatExtendedCLF
	}
	finalHandler = middleware.Logging(logCfg)(finalHandler)

	if collector != nil {
		finalHandler = middleware.Metrics(collector)(finalHandler)
	}

	// Build server configuration
	addr := net.JoinHostPort(cfg.Host, fmt.Sprintf("%d", cfg.Port))
	srvCfg := &server.Config{
		Addr:    addr,
		Handler: finalHandler,
	}

	// Configure TLS if enabled
	scheme := "http"
	if cfg.TLS.Enabled {
		scheme = "https"
		tlsCfg, tlsErr := tls.NewServerTLSConfig(tls.ServerTLSOptions{
			CertFile:   cfg.TLS.Cert,
			KeyFile:    cfg.TLS.Key,
			CAFile:     cfg.TLS.CA,
			ClientAuth: cfg.TLS.ClientAuth,
			MinVersion: cfg.TLS.MinVersion,
		})
		if tlsErr != nil {
			return fmt.Errorf("TLS configuration error: %w", tlsErr)
		}
		srvCfg.TLSConfig = tlsCfg
	}

	srvCfg.Banner = fmt.Sprintf("Serving %s on %s://%s", dir, scheme, addr)

	srv := server.NewServer(srvCfg)

	// Build the optional HTTP→HTTPS redirect server.
	var redirectSrv *server.Server
	if cfg.Serve.HTTPRedirect {
		redirectAddr := net.JoinHostPort(cfg.Host, fmt.Sprintf("%d", cfg.Serve.HTTPPort))
		redirectSrv = server.NewServer(&server.Config{
			Addr:    redirectAddr,
			Handler: server.RedirectToHTTPS(cfg.Port),
			Banner:  fmt.Sprintf("Redirecting http://%s to %s", redirectAddr, addr),
			// No TLSConfig: the redirect listener speaks plain HTTP.
		})
	}

	// A shared, cancelable context ties the two servers together: when either
	// stops (signal or error), the other is asked to shut down too.
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	return runServeServers(ctx, srv, redirectSrv)
}

// runServeServers starts the main server and (if non-nil) the redirect server,
// blocking until ctx is canceled or either server fails.
//
// A redirect-listener failure (e.g. its port is already in use) cancels the
// shared context, tearing down the main server too, and is returned to the
// caller (taking precedence only when the main server itself shut down cleanly).
// Because Server.Start returns nil on a clean signal/context shutdown, a normal
// SIGINT yields no spurious error. Waiting on the redirect goroutine's channel
// also guarantees its graceful Shutdown has completed before this returns.
func runServeServers(ctx context.Context, main, redirect *server.Server) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	var redirectErrCh chan error
	if redirect != nil {
		redirectErrCh = make(chan error, 1)
		go func() {
			rerr := redirect.Start(ctx)
			if rerr != nil {
				cancel() // a redirect failure tears down the main server too
			}
			redirectErrCh <- rerr
		}()
	}

	err := main.Start(ctx)
	cancel()

	if redirectErrCh != nil {
		if rerr := <-redirectErrCh; rerr != nil && err == nil {
			err = fmt.Errorf("http redirect listener: %w", rerr)
		}
	}
	return err
}
