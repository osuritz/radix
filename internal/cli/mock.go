package cli

import (
	"context"
	"fmt"
	"math"
	"net"
	"net/http"
	"strings"
	"time"
	"unicode"

	"github.com/osuritz/radix/internal/metrics"
	"github.com/osuritz/radix/internal/server"
	"github.com/osuritz/radix/internal/server/middleware"
	radixTLS "github.com/osuritz/radix/internal/tls"
	"github.com/osuritz/radix/internal/version"
	"github.com/spf13/cobra"
)

var (
	mockLatency       string
	mockLatencyJitter string
	mockFailRate      float64
	mockFailStatus    int
	mockCORS          bool
	mockBuiltin       bool
	mockPrefix        string
)

var mockCmd = &cobra.Command{
	Use:   "mock",
	Short: "Mock API server with built-in httpbin-style endpoints",
	Long: `Start a zero-config API mock server exposing httpbin-style built-in
endpoints, with global latency and chaos (random failure) knobs.

Built-in endpoints include /get, /post, /put, /patch, /delete, /anything,
/headers, /ip, /user-agent, /uuid, /status/{code}, /delay/{n}, /bytes/{n},
/json, /html, and /xml. Use --prefix to mount them under a path, --latency to
add artificial latency, and --fail-rate to inject random failures.

Examples:
  radix mock                                  # Built-in endpoints on :8080
  radix mock --latency 200ms                  # Add 200ms latency to all responses
  radix mock --latency 200ms --latency-jitter 100ms
  radix mock --fail-rate 10                    # Fail 10% of requests with 500
  radix mock --fail-rate 25 --fail-status 503  # Fail 25% with 503
  radix mock --prefix /_test                   # GET /_test/get
  radix mock --cors                            # Enable CORS headers
  radix mock --tls --cert c.pem --key k.pem    # HTTPS mock`,
	Args: cobra.NoArgs,
	RunE: runMock,
}

func init() {
	mockCmd.Flags().StringVar(&mockLatency, "latency", "", "global artificial latency (e.g. 200ms, 1s)")
	mockCmd.Flags().StringVar(&mockLatencyJitter, "latency-jitter", "", "random jitter added to latency (e.g. 100ms)")
	mockCmd.Flags().Float64Var(&mockFailRate, "fail-rate", 0, "random failure rate, percentage 0-100")
	mockCmd.Flags().IntVar(&mockFailStatus, "fail-status", 500, "status code returned for random failures")
	mockCmd.Flags().BoolVar(&mockCORS, "cors", false, "enable CORS headers")
	mockCmd.Flags().BoolVar(&mockBuiltin, "builtin", true, "enable built-in httpbin-style endpoints")
	mockCmd.Flags().StringVar(&mockPrefix, "prefix", "", "path prefix for built-in endpoints (e.g. /_test)")
}

// applyMockFlags overrides mock config fields from CLI flags that were
// explicitly set, parsing duration strings later in runMock.
func applyMockFlags(cmd *cobra.Command) {
	if cmd.Flags().Changed("latency") {
		cfg.Mock.Latency = mockLatency
	}
	if cmd.Flags().Changed("latency-jitter") {
		cfg.Mock.LatencyJitter = mockLatencyJitter
	}
	if cmd.Flags().Changed("fail-rate") {
		cfg.Mock.FailRate = mockFailRate
	}
	if cmd.Flags().Changed("fail-status") {
		cfg.Mock.FailStatus = mockFailStatus
	}
	if cmd.Flags().Changed("cors") {
		cfg.Mock.CORS = mockCORS
	}
	if cmd.Flags().Changed("builtin") {
		cfg.Mock.Builtin = mockBuiltin
	}
	if cmd.Flags().Changed("prefix") {
		cfg.Mock.Prefix = mockPrefix
	}
}

func runMock(cmd *cobra.Command, _ []string) error {
	applyMockFlags(cmd)

	// Validate fail-rate, fail-status, and prefix at the CLI boundary.
	if r := cfg.Mock.FailRate; math.IsNaN(r) || math.IsInf(r, 0) || r < 0 || r > 100 {
		return fmt.Errorf("invalid --fail-rate %g: must be between 0 and 100", r)
	}
	// 1xx codes are excluded: net/http treats WriteHeader(1xx) as informational.
	if s := cfg.Mock.FailStatus; s < 200 || s > 599 {
		return fmt.Errorf("invalid --fail-status %d: must be between 200 and 599", s)
	}
	if err := validateMockPrefix(cfg.Mock.Prefix); err != nil {
		return err
	}

	// Parse latency durations from their config string form.
	latency, err := parseOptionalDuration("latency", cfg.Mock.Latency)
	if err != nil {
		return err
	}
	jitter, err := parseOptionalDuration("latency-jitter", cfg.Mock.LatencyJitter)
	if err != nil {
		return err
	}

	// Build the mock handler.
	mockHandler := server.NewMockHandler(server.MockConfig{
		Builtin:       cfg.Mock.Builtin,
		Prefix:        cfg.Mock.Prefix,
		Latency:       latency,
		LatencyJitter: jitter,
		FailRate:      cfg.Mock.FailRate,
		FailStatus:    cfg.Mock.FailStatus,
	})

	// Build handler chain using a mux.
	mux := http.NewServeMux()

	// Set up metrics if enabled.
	var collector *metrics.Collector
	if cfg.Metrics.Enabled {
		collector = metrics.NewCollector("mock", version.Version)
		mux.Handle(cfg.Metrics.Path, collector.Handler(cfg.Metrics.Format))
	}

	// Health and readiness endpoints (kept at root regardless of --prefix).
	mux.HandleFunc("/_health", func(w http.ResponseWriter, _ *http.Request) {
		writeJSONStatus(w, "ok")
	})
	mux.HandleFunc("/_ready", func(w http.ResponseWriter, _ *http.Request) {
		writeJSONStatus(w, "ready")
	})

	mux.Handle("/", mockHandler)

	// Apply middleware chain (outermost first).
	var finalHandler http.Handler = mux

	if cfg.Mock.CORS {
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

	srvCfg.Banner = fmt.Sprintf("Mocking API on %s://%s", scheme, addr)

	srv := server.NewServer(srvCfg)
	return srv.Start(context.Background())
}

// validateMockPrefix rejects a --prefix that is not a simple path-segment
// sequence. After normalization an empty prefix (root mount) is allowed;
// otherwise it must start with "/" and contain no ServeMux wildcard/pattern
// characters ('{', '}') or whitespace, any of which can panic at route
// registration.
func validateMockPrefix(prefix string) error {
	normalized := server.NormalizePrefix(prefix)
	if normalized == "" {
		return nil
	}
	if !strings.HasPrefix(normalized, "/") ||
		strings.ContainsAny(normalized, "{}") ||
		strings.ContainsFunc(normalized, unicode.IsSpace) {
		return fmt.Errorf("invalid --prefix %q: must be a simple path like /_test (no braces or whitespace)", prefix)
	}
	return nil
}

// parseOptionalDuration parses a duration string, treating an empty string as a
// zero duration. The flag name is used to produce a helpful error message.
func parseOptionalDuration(flag, value string) (time.Duration, error) {
	if value == "" {
		return 0, nil
	}
	d, err := time.ParseDuration(value)
	if err != nil {
		return 0, fmt.Errorf("invalid --%s %q: %w", flag, value, err)
	}
	if d < 0 {
		return 0, fmt.Errorf("invalid --%s %q: must not be negative", flag, value)
	}
	return d, nil
}
