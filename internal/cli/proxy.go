package cli

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os/signal"
	"syscall"
	"time"

	cryptotls "crypto/tls"

	"github.com/osuritz/radix/internal/metrics"
	"github.com/osuritz/radix/internal/server"
	"github.com/osuritz/radix/internal/server/middleware"
	radixTLS "github.com/osuritz/radix/internal/tls"
	"github.com/osuritz/radix/internal/version"
	"github.com/spf13/cobra"
)

var (
	proxyTarget        string
	proxyRewrite       string
	proxyStripPrefix   string
	proxyTimeout       string
	proxyWebSocket     bool
	proxyTLSSkipVerify bool
	proxyHeaders       []string
	proxyCORS          bool
)

var proxyCmd = &cobra.Command{
	Use:   "proxy [target]",
	Short: "Reverse proxy to a backend server",
	Long: `Start a reverse proxy server that forwards requests to a backend target.

Supports path rewriting, prefix stripping, header injection, CORS,
TLS termination, and backend TLS (including mTLS).

Examples:
  radix proxy http://localhost:3000          # Proxy to local backend
  radix proxy --target http://api.local:8000 # Same, using --target flag
  radix proxy http://localhost:3000 --cors   # Enable CORS headers
  radix proxy http://localhost:3000 --strip-prefix /api
  radix proxy http://localhost:3000 --rewrite /v1:/v2
  radix proxy https://backend:443 --tls-skip-verify
  radix proxy http://localhost:3000 --header "X-Custom: value"
  radix proxy http://localhost:3000 --tls --cert c.pem --key k.pem`,
	Args: cobra.MaximumNArgs(1),
	RunE: runProxy,
}

func init() {
	proxyCmd.Flags().StringVar(&proxyTarget, "target", "", "backend target URL (e.g., http://localhost:3000)")
	proxyCmd.Flags().StringVar(&proxyRewrite, "rewrite", "", "path rewrite rule (from:to format)")
	proxyCmd.Flags().StringVar(&proxyStripPrefix, "strip-prefix", "", "strip path prefix before forwarding")
	proxyCmd.Flags().StringVar(&proxyTimeout, "timeout", "", "backend response timeout (e.g., 30s, 1m)")
	proxyCmd.Flags().BoolVar(&proxyWebSocket, "websocket", false, "enable explicit WebSocket support")
	proxyCmd.Flags().BoolVar(&proxyTLSSkipVerify, "tls-skip-verify", false, "skip TLS certificate verification for backend")
	proxyCmd.Flags().StringArrayVar(&proxyHeaders, "header", nil, "add header to proxy requests (Key: Value)")
	proxyCmd.Flags().BoolVar(&proxyCORS, "cors", false, "enable CORS headers")
}

// applyProxyFlags overrides proxy config fields from CLI flags and positional args.
func applyProxyFlags(cmd *cobra.Command, args []string) error {
	if len(args) > 0 {
		cfg.Proxy.Target = args[0]
	}
	if cmd.Flags().Changed("target") {
		cfg.Proxy.Target = proxyTarget
	}
	if cmd.Flags().Changed("rewrite") {
		cfg.Proxy.Rewrite = proxyRewrite
	}
	if cmd.Flags().Changed("strip-prefix") {
		cfg.Proxy.StripPrefix = proxyStripPrefix
	}
	if cmd.Flags().Changed("timeout") {
		d, err := time.ParseDuration(proxyTimeout)
		if err != nil {
			return fmt.Errorf("invalid timeout %q: %w", proxyTimeout, err)
		}
		cfg.Proxy.Timeout = d
	}
	if cmd.Flags().Changed("websocket") {
		cfg.Proxy.WebSocket = proxyWebSocket
	}
	if cmd.Flags().Changed("tls-skip-verify") {
		cfg.Proxy.TLSSkipVerify = proxyTLSSkipVerify
	}
	if cmd.Flags().Changed("header") {
		cfg.Proxy.Headers = proxyHeaders
	}
	if cmd.Flags().Changed("cors") {
		cfg.Proxy.CORS = proxyCORS
	}
	return nil
}

// buildBackendTLS creates a TLS config for backend connections when needed.
func buildBackendTLS(targetURL *url.URL) (*cryptotls.Config, error) {
	if targetURL.Scheme != "https" && cfg.Proxy.BackendCA == "" && cfg.Proxy.BackendCert == "" {
		return nil, nil
	}
	return radixTLS.NewClientTLSConfig(&radixTLS.ClientTLSOptions{
		CAFile:     cfg.Proxy.BackendCA,
		CertFile:   cfg.Proxy.BackendCert,
		KeyFile:    cfg.Proxy.BackendKey,
		SkipVerify: cfg.Proxy.TLSSkipVerify,
	})
}

func runProxy(cmd *cobra.Command, args []string) error {
	// Override proxy config from flags / positional arg
	if err := applyProxyFlags(cmd, args); err != nil {
		return err
	}

	// Validate target
	if cfg.Proxy.Target == "" {
		return fmt.Errorf("target URL is required (use positional arg or --target flag)")
	}

	targetURL, err := url.Parse(cfg.Proxy.Target)
	if err != nil {
		return fmt.Errorf("invalid target URL %q: %w", cfg.Proxy.Target, err)
	}
	if targetURL.Scheme != "http" && targetURL.Scheme != "https" {
		return fmt.Errorf("target URL must have http:// or https:// scheme, got %q", cfg.Proxy.Target)
	}

	// Build backend TLS config if needed
	backendTLS, err := buildBackendTLS(targetURL)
	if err != nil {
		return fmt.Errorf("backend TLS configuration error: %w", err)
	}

	// Build reverse proxy handler
	proxyHandler := server.NewReverseProxy(server.ProxyConfig{
		Target:      targetURL,
		Timeout:     cfg.Proxy.Timeout,
		StripPrefix: cfg.Proxy.StripPrefix,
		Rewrite:     cfg.Proxy.Rewrite,
		TLSConfig:   backendTLS,
	})

	// Build handler chain using a mux
	mux := http.NewServeMux()

	// Set up metrics if enabled
	var collector *metrics.Collector
	if cfg.Metrics.Enabled {
		collector = metrics.NewCollector("proxy", version.Version)
		mux.Handle(cfg.Metrics.Path, collector.Handler(cfg.Metrics.Format))
	}

	mux.Handle("/", proxyHandler)

	// Apply middleware chain (outermost first)
	var finalHandler http.Handler = mux

	// Auth header injection
	provider := middleware.ResolveProvider(cfg.Proxy.Auth.Provider, cfg.Proxy.Headers)
	if provider != nil {
		finalHandler = middleware.InjectHeaders(provider)(finalHandler)
	}

	if cfg.Proxy.CORS {
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
		tlsCfg, tlsErr := radixTLS.NewServerTLSConfig(radixTLS.ServerTLSOptions{
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

	fmt.Printf("Proxying to %s on %s://%s\n", cfg.Proxy.Target, scheme, addr)

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
