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
	mockRoutes        string
	mockWatch         bool
)

var mockCmd = &cobra.Command{
	Use:   "mock [config-file]",
	Short: "Mock API server with built-in httpbin-style endpoints and custom routes",
	Long: `Start a zero-config API mock server exposing httpbin-style built-in
endpoints, with global latency and chaos (random failure) knobs. Optionally
load a YAML routes file defining custom routes that take precedence over the
built-ins, with optional hot-reload.

Built-in endpoints include /get, /post, /put, /patch, /delete, /anything,
/headers, /ip, /user-agent, /uuid, /status/{code}, /delay/{n}, /bytes/{n},
/json, /html, and /xml. Use --prefix to mount them under a path, --latency to
add artificial latency, and --fail-rate to inject random failures.

Custom routes support exact, :param, regex:, and trailing /* glob paths,
templated response bodies ({{.params.id}}, {{uuid}}, etc.), per-route delays,
and a 404 or proxy fallback for unmatched requests. With --watch the routes
file is reloaded on change (a broken edit is rejected; the previous good
config keeps serving).

Examples:
  radix mock                                  # Built-in endpoints on :8080
  radix mock --latency 200ms                  # Add 200ms latency to all responses
  radix mock --latency 200ms --latency-jitter 100ms
  radix mock --fail-rate 10                    # Fail 10% of requests with 500
  radix mock --fail-rate 25 --fail-status 503  # Fail 25% with 503
  radix mock --prefix /_test                   # GET /_test/get
  radix mock --cors                            # Enable CORS headers
  radix mock routes.yml                         # Load custom routes (positional)
  radix mock --routes routes.yml --watch       # Custom routes with hot-reload
  radix mock --tls --cert c.pem --key k.pem    # HTTPS mock`,
	Args: cobra.MaximumNArgs(1),
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
	mockCmd.Flags().StringVarP(&mockRoutes, "routes", "r", "", "YAML routes file defining custom routes")
	mockCmd.Flags().BoolVarP(&mockWatch, "watch", "w", false, "reload the routes file on change (hot-reload)")
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
	if cmd.Flags().Changed("routes") {
		cfg.Mock.Routes = mockRoutes
	}
	if cmd.Flags().Changed("watch") {
		cfg.Mock.Watch = mockWatch
	}
}

func runMock(cmd *cobra.Command, args []string) error {
	applyMockFlags(cmd)

	// A positional config-file argument is shorthand for --routes.
	if len(args) > 0 {
		cfg.Mock.Routes = args[0]
	}

	// A long-lived context drives both graceful shutdown (via the server's own
	// signal handling) and the routes-file watcher goroutine, which stops when
	// this context is canceled.
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Load custom routes (if any) and merge file settings under CLI flags. CLI
	// flags win whenever the corresponding flag was explicitly set.
	var routesStore *server.RoutesStore
	if cfg.Mock.Routes != "" {
		store, err := buildRoutesStore(ctx, cmd)
		if err != nil {
			return err
		}
		routesStore = store
	}

	// Validate fail-rate, fail-status, and prefix at the CLI boundary (after the
	// routes file may have supplied settings, so file values are validated too).
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

	mockCfg := server.MockConfig{
		Builtin:       cfg.Mock.Builtin,
		Prefix:        cfg.Mock.Prefix,
		Latency:       latency,
		LatencyJitter: jitter,
		FailRate:      cfg.Mock.FailRate,
		FailStatus:    cfg.Mock.FailStatus,
	}

	// Build the mock handler: routed (custom routes -> built-ins -> fallback)
	// when a routes file is configured, otherwise the built-ins-only handler.
	var mockHandler http.Handler
	if routesStore != nil {
		routed := server.NewRoutedHandler(server.RoutedHandlerConfig{
			Store:   routesStore,
			Builtin: cfg.Mock.Builtin,
			Prefix:  cfg.Mock.Prefix,
		})
		// Apply global latency/fail-rate around the routed handler, matching the
		// built-ins-only behavior.
		mockHandler = server.WithLatencyAndFailures(routed, mockCfg)
	} else {
		mockHandler = server.NewMockHandler(mockCfg)
	}

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
	return srv.Start(ctx)
}

// buildRoutesStore loads the configured routes file, merges its settings under
// the CLI flags (CLI wins when a flag was explicitly set), seeds an atomic
// store, and — when --watch is set — starts the hot-reload watcher bound to ctx.
func buildRoutesStore(ctx context.Context, cmd *cobra.Command) (*server.RoutesStore, error) {
	compiled, err := server.LoadRoutes(cfg.Mock.Routes)
	if err != nil {
		return nil, fmt.Errorf("failed to load routes file %q: %w", cfg.Mock.Routes, err)
	}

	settings := compiled.Settings()
	mergeRouteSettings(cmd, &settings)

	logf := func(format string, args ...any) {
		fmt.Fprintf(cmd.OutOrStdout(), format+"\n", args...)
	}
	store := server.NewRoutesStore(cfg.Mock.Routes, compiled, logf)

	if cfg.Mock.Watch {
		if watchErr := store.Watch(ctx); watchErr != nil {
			return nil, fmt.Errorf("failed to start routes watcher: %w", watchErr)
		}
		fmt.Fprintf(cmd.OutOrStdout(), "Watching %s for changes\n", cfg.Mock.Routes)
	}

	return store, nil
}

// mergeRouteSettings applies routes-file settings to the active config for any
// option whose CLI flag was not explicitly set. CLI flags therefore take
// precedence over the file, while the file overrides the built-in defaults.
func mergeRouteSettings(cmd *cobra.Command, s *server.RouteSettings) {
	if !cmd.Flags().Changed("latency") && s.Latency > 0 {
		cfg.Mock.Latency = s.Latency.String()
	}
	if !cmd.Flags().Changed("latency-jitter") && s.LatencyJitter > 0 {
		cfg.Mock.LatencyJitter = s.LatencyJitter.String()
	}
	if !cmd.Flags().Changed("fail-rate") && s.FailRate > 0 {
		cfg.Mock.FailRate = s.FailRate
	}
	if !cmd.Flags().Changed("fail-status") && s.FailStatus != 0 {
		cfg.Mock.FailStatus = s.FailStatus
	}
	if !cmd.Flags().Changed("cors") && s.CORS {
		cfg.Mock.CORS = true
	}
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
