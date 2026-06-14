package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"time"

	"github.com/osuritz/radix/internal/metrics"
	"github.com/osuritz/radix/internal/server"
	"github.com/osuritz/radix/internal/server/middleware"
	radixTLS "github.com/osuritz/radix/internal/tls"
	"github.com/osuritz/radix/internal/version"
	"github.com/spf13/cobra"
)

var (
	echoStatus         int
	echoDelay          time.Duration
	echoDelayJitter    time.Duration
	echoBody           string
	echoContentType    string
	echoHeaders        []string
	echoEchoBody       bool
	echoEchoHeaders    bool
	echoEchoQuery      bool
	echoBodyLimit      int
	echoPretty         bool
	echoStatusFromPath bool
	echoDelayFromPath  bool
	echoCORS           bool
)

var echoCmd = &cobra.Command{
	Use:   "echo",
	Short: "Echo HTTP requests back as JSON",
	Long: `Start a server that responds to every request with a JSON description
of that request: method, headers, query, body, client/server info, TLS state,
and timing. Useful for debugging webhooks and HTTP clients.

Supports response delays, custom status/body/headers, path-based status and
delay, CORS, TLS, and metrics.

Examples:
  radix echo                                  # Echo server on :8080
  radix echo --delay 2s                       # Simulate a slow API
  radix echo --delay 500ms --delay-jitter 200ms
  radix echo --status 201                     # Custom status code
  radix echo --status-from-path              # GET /404 returns 404
  radix echo --delay-from-path               # GET /delay/500ms delays 500ms
  radix echo --body '{"message":"OK"}'       # Fixed response body
  radix echo --tls --cert c.pem --key k.pem  # HTTPS echo`,
	Args: cobra.NoArgs,
	RunE: runEcho,
}

func init() {
	echoCmd.Flags().IntVarP(&echoStatus, "status", "s", 200, "default response status code")
	echoCmd.Flags().DurationVar(&echoDelay, "delay", 0, "delay before responding (e.g. 2s, 500ms)")
	echoCmd.Flags().DurationVar(&echoDelayJitter, "delay-jitter", 0, "random jitter added to delay")
	echoCmd.Flags().StringVar(&echoBody, "body", "", "fixed response body (overrides echo JSON)")
	echoCmd.Flags().StringVar(&echoContentType, "content-type", "application/json", "response Content-Type")
	echoCmd.Flags().StringArrayVar(&echoHeaders, "header", nil, "add response header (Key: Value)")
	echoCmd.Flags().BoolVar(&echoEchoBody, "echo-body", true, "include request body in response")
	echoCmd.Flags().BoolVar(&echoEchoHeaders, "echo-headers", true, "include request headers in response")
	echoCmd.Flags().BoolVar(&echoEchoQuery, "echo-query", true, "include query parameters in response")
	echoCmd.Flags().IntVar(&echoBodyLimit, "body-limit", 1048576, "max request body size in bytes")
	echoCmd.Flags().BoolVar(&echoPretty, "pretty", true, "pretty-print JSON response")
	echoCmd.Flags().BoolVar(&echoStatusFromPath, "status-from-path", false, "derive status from path (e.g. /404 -> 404)")
	echoCmd.Flags().BoolVar(&echoDelayFromPath, "delay-from-path", false, "derive delay from path (e.g. /delay/500ms)")
	echoCmd.Flags().BoolVar(&echoCORS, "cors", false, "enable CORS headers")
}

func runEcho(cmd *cobra.Command, _ []string) error {
	// Override echo config from flags.
	if cmd.Flags().Changed("status") {
		cfg.Echo.Status = echoStatus
	}
	if cmd.Flags().Changed("delay") {
		cfg.Echo.Delay = echoDelay
	}
	if cmd.Flags().Changed("body") {
		cfg.Echo.Body = echoBody
	}
	if cmd.Flags().Changed("header") {
		cfg.Echo.Headers = echoHeaders
	}
	if cmd.Flags().Changed("delay-jitter") {
		cfg.Echo.DelayJitter = echoDelayJitter
	}
	if cmd.Flags().Changed("content-type") {
		cfg.Echo.ContentType = echoContentType
	}
	if cmd.Flags().Changed("echo-body") {
		cfg.Echo.EchoBody = echoEchoBody
	}
	if cmd.Flags().Changed("echo-headers") {
		cfg.Echo.EchoHeaders = echoEchoHeaders
	}
	if cmd.Flags().Changed("echo-query") {
		cfg.Echo.EchoQuery = echoEchoQuery
	}
	if cmd.Flags().Changed("body-limit") {
		cfg.Echo.BodyLimit = echoBodyLimit
	}
	if cmd.Flags().Changed("pretty") {
		cfg.Echo.Pretty = echoPretty
	}
	if cmd.Flags().Changed("status-from-path") {
		cfg.Echo.StatusFromPath = echoStatusFromPath
	}
	if cmd.Flags().Changed("delay-from-path") {
		cfg.Echo.DelayFromPath = echoDelayFromPath
	}

	// Build echo handler.
	echoHandler := server.NewEchoHandler(server.EchoConfig{
		Status:         cfg.Echo.Status,
		Delay:          cfg.Echo.Delay,
		DelayJitter:    cfg.Echo.DelayJitter,
		Body:           cfg.Echo.Body,
		ContentType:    cfg.Echo.ContentType,
		Headers:        cfg.Echo.Headers,
		EchoBody:       cfg.Echo.EchoBody,
		EchoHeaders:    cfg.Echo.EchoHeaders,
		EchoQuery:      cfg.Echo.EchoQuery,
		BodyLimit:      int64(cfg.Echo.BodyLimit),
		Pretty:         cfg.Echo.Pretty,
		StatusFromPath: cfg.Echo.StatusFromPath,
		DelayFromPath:  cfg.Echo.DelayFromPath,
	})

	// Build handler chain using a mux.
	mux := http.NewServeMux()

	// Set up metrics if enabled.
	var collector *metrics.Collector
	if cfg.Metrics.Enabled {
		collector = metrics.NewCollector("echo", version.Version)
		mux.Handle(cfg.Metrics.Path, collector.Handler(cfg.Metrics.Format))
	}

	// Health and readiness endpoints (not echoed).
	mux.HandleFunc("/_health", func(w http.ResponseWriter, _ *http.Request) {
		writeJSONStatus(w, "ok")
	})
	mux.HandleFunc("/_ready", func(w http.ResponseWriter, _ *http.Request) {
		writeJSONStatus(w, "ready")
	})

	mux.Handle("/", echoHandler)

	// Apply middleware chain (outermost first).
	var finalHandler http.Handler = mux

	if echoCORS {
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

	// Build server configuration.
	addr := net.JoinHostPort(cfg.Host, fmt.Sprintf("%d", cfg.Port))
	srvCfg := &server.Config{
		Addr:    addr,
		Handler: finalHandler,
	}

	// Configure TLS if enabled.
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

	srvCfg.Banner = fmt.Sprintf("Echoing requests on %s://%s", scheme, addr)

	srv := server.NewServer(srvCfg)
	return srv.Start(context.Background())
}

// writeJSONStatus writes a small {"status": <status>} JSON body with a 200 status.
func writeJSONStatus(w http.ResponseWriter, status string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]string{"status": status})
}
