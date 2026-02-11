package cli

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/osuritz/radix/internal/metrics"
	"github.com/osuritz/radix/internal/server"
	"github.com/osuritz/radix/internal/server/middleware"
	"github.com/osuritz/radix/internal/tls"
	"github.com/osuritz/radix/internal/version"
	"github.com/spf13/cobra"
)

var (
	serveDir   string
	serveIndex string
	serveSPA   bool
	serveCORS  bool
	serveGzip  bool
	serveCache string
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
}

func runServe(cmd *cobra.Command, args []string) error {
	// Override serve config from flags
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

	// Build HTTP server
	addr := net.JoinHostPort(cfg.Host, fmt.Sprintf("%d", cfg.Port))
	srv := &http.Server{
		Addr:              addr,
		Handler:           finalHandler,
		ReadHeaderTimeout: 10 * time.Second,
	}

	// Graceful shutdown
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	// Start server
	scheme := "http"
	errCh := make(chan error, 1)

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
		srv.TLSConfig = tlsCfg

		go func() {
			errCh <- srv.ListenAndServeTLS("", "")
		}()
	} else {
		go func() {
			errCh <- srv.ListenAndServe()
		}()
	}

	fmt.Printf("Serving %s on %s://%s\n", dir, scheme, addr)

	// Wait for shutdown signal or server error
	select {
	case <-ctx.Done():
		fmt.Println("\nShutting down...")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if shutdownErr := srv.Shutdown(shutdownCtx); shutdownErr != nil {
			return fmt.Errorf("shutdown error: %w", shutdownErr)
		}
		return nil
	case err := <-errCh:
		if err != nil && err != http.ErrServerClosed {
			return fmt.Errorf("server error: %w", err)
		}
		return nil
	}
}
