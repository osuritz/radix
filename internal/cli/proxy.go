package cli

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
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
	proxyFlushInterval time.Duration
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
	proxyCmd.Flags().DurationVar(&proxyFlushInterval, "flush-interval", -1*time.Nanosecond,
		"response flush interval for streaming; negative (e.g. -1ns) flushes immediately, 0 uses default (default -1ns)")
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
	if cmd.Flags().Changed("flush-interval") {
		cfg.Proxy.FlushInterval = proxyFlushInterval
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

	if err := validateMetricsConfig(); err != nil {
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
		Target:        targetURL,
		Timeout:       cfg.Proxy.Timeout,
		StripPrefix:   cfg.Proxy.StripPrefix,
		Rewrite:       cfg.Proxy.Rewrite,
		TLSConfig:     backendTLS,
		FlushInterval: cfg.Proxy.FlushInterval,
	})

	// Build handler chain using a mux
	mux := http.NewServeMux()

	// Set up metrics if enabled. The collector is shared with the admin server;
	// the /_metrics endpoint is exposed there, not on the app mux.
	var collector *metrics.Collector
	if cfg.Metrics.Enabled {
		collector = metrics.NewCollector("proxy", version.Version)
	}

	mux.Handle("/", proxyHandler)

	// Apply middleware chain (outermost first)
	var finalHandler http.Handler = mux

	// Auth header injection. The provider is chosen from the full auth settings:
	// the reserved "headers" provider builds the built-in structured provider
	// from proxy.auth.config (Surface B); an explicit fork name selects a
	// compiled-in provider; otherwise the registry auto-detects a single
	// provider or falls back to the --header / proxy.headers values, resolving
	// any ${env:...} / ${keychain:...} tokens per request (Surface A).
	provider, err := middleware.ResolveAuthProvider(middleware.AuthSettings{
		Provider:      cfg.Proxy.Auth.Provider,
		Config:        cfg.Proxy.Auth.Config,
		StaticHeaders: cfg.Proxy.Headers,
	})
	if err != nil {
		return fmt.Errorf("auth provider resolution failed: %w", err)
	}
	if provider != nil {
		var injectOpts []middleware.InjectOption
		if cfg.Verbose {
			// Names-only summary; injected secret values are never logged.
			injectOpts = append(injectOpts, middleware.WithVerboseLogging(os.Stdout))
		}
		finalHandler = middleware.InjectHeaders(provider, injectOpts...)(finalHandler)
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
		srvCfg.TLSConfig = tlsCfg
	}

	srvCfg.Banner = fmt.Sprintf("Proxying to %s on %s://%s", cfg.Proxy.Target, scheme, addr)

	srv := server.NewServer(srvCfg)

	// Build the loopback admin server (metrics + /healthz) sharing the collector.
	admin, err := buildAdminServer("proxy", collector)
	if err != nil {
		return err
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	return runServers(ctx, srv, admin)
}
