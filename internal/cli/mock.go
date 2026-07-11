package cli

import (
	"context"
	"errors"
	"fmt"
	"math"
	"net"
	"net/http"
	"strings"
	"time"
	"unicode"

	"github.com/osuritz/radix/internal/config"
	"github.com/osuritz/radix/internal/metrics"
	"github.com/osuritz/radix/internal/server"
	"github.com/osuritz/radix/internal/server/middleware"
	radixTLS "github.com/osuritz/radix/internal/tls"
	"github.com/osuritz/radix/internal/version"
	"github.com/spf13/cobra"
)

var (
	mockLatency            string
	mockLatencyJitter      string
	mockFailRate           float64
	mockFailStatus         int
	mockCORS               bool
	mockBuiltin            bool
	mockPrefix             string
	mockRoutes             string
	mockWatch              bool
	mockOptionalClientAuth bool
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
and a 404 or proxy fallback for unmatched requests. regex: patterns use Go
regexp semantics and are NOT auto-anchored; use ^...$ to match the whole path.

With --watch the routes file is reloaded on change: routes, the fallback, and
the global latency/fail-rate settings take effect on save (a broken edit is
rejected; the previous good config keeps serving). CORS is applied once at
startup and is not hot-reloaded. Explicitly-set CLI flags always win over the
file. Pass the routes file positionally or via --routes (not both).

Over HTTPS, custom routes can inspect the TLS client certificate: templates
expose {{.tls.client_cert.cn}} (plus o, serial, not_before, not_after,
fingerprint, issuer_cn, issuer_o), conditions can match certificate fields via
tls.-prefixed keys (e.g. tls.cn), and require_client_cert: true rejects
certless requests with a 403. Use --optional-client-auth (with --tls and --ca)
to request but not require a certificate, so require_client_cert routes are
reachable; the global --client-auth flag instead rejects certless connections
at the TLS layer.`,
	Example: `  radix mock                                   # Built-in endpoints on :8080
  radix mock --latency 200ms --latency-jitter 100ms
  radix mock --fail-rate 25 --fail-status 503  # Fail 25% of requests with 503
  radix mock --prefix /_test                   # Built-ins under /_test (GET /_test/get)
  radix mock --routes routes.yml --watch       # Custom routes with hot-reload
  radix mock routes.yml --tls --cert ./certs/cert.pem --key ./certs/key.pem \
    --ca ./certs/ca.pem --optional-client-auth # Per-route client-cert rules`,
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
	mockCmd.Flags().BoolVar(&mockOptionalClientAuth, "optional-client-auth", false,
		"request but do not require a client certificate (verified when presented); enables require_client_cert routes")
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
	// --optional-client-auth is mock-only, so its TLS config override lives
	// here rather than with the persistent TLS flags in the root command. The
	// tls.client_auth_optional key remains available to all server commands
	// via config file or environment.
	if cmd.Flags().Changed("optional-client-auth") {
		cfg.TLS.ClientAuthOptional = mockOptionalClientAuth
	}
}

func runMock(cmd *cobra.Command, args []string) error {
	applyMockFlags(cmd)

	// Resolve the routes file from either the positional arg or --routes. If both
	// are given and differ, it is ambiguous which the user meant, so reject it
	// rather than silently picking one.
	if err := resolveRoutesArg(cmd, args); err != nil {
		return err
	}

	// Validate the CLI-supplied values (fail-rate, fail-status, prefix, latency)
	// up front so any value baked into the routes store is already validated.
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
	if err := validateMetricsConfig(); err != nil {
		return err
	}

	// Optional client auth (--optional-client-auth or tls.client_auth_optional)
	// requests (but does not require) a client certificate so per-route
	// require_client_cert rules can decide. It only makes sense over TLS, and
	// combining it with the stricter global --client-auth (which already
	// rejects certless connections at the TLS layer) is ambiguous, so both
	// misuses are rejected up front. A client CA (--ca) is required too:
	// without one Go would verify presented certificates against the SYSTEM
	// root store, silently rejecting the user's own gencert client certs while
	// accepting any WebPKI clientAuth certificate. These checks read the
	// post-merge config, so config-file/env users get the same friendly errors
	// as flag users (the TLS loader enforces the same invariants as a backstop).
	if cfg.TLS.ClientAuthOptional {
		if !cfg.TLS.Enabled {
			return errors.New("--optional-client-auth requires --tls")
		}
		if cfg.TLS.ClientAuth {
			return errors.New("--optional-client-auth cannot be combined with --client-auth (which already requires a client certificate)")
		}
		if cfg.TLS.CA == "" {
			return errors.New("--optional-client-auth requires --ca (the CA file presented client certificates are verified against)")
		}
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

	// A long-lived context drives both graceful shutdown (via the server's own
	// signal handling) and the routes-file watcher goroutine, which stops when
	// this context is canceled.
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Set up metrics if enabled. The collector is shared with the admin server;
	// the /_metrics endpoint is exposed there, not on the app mux. It is built
	// before the routes store and handlers so they can record per-command mock
	// counters (route matches, template renders/errors, reloads, fail injections,
	// fallback hits).
	var collector *metrics.Collector
	if cfg.Metrics.Enabled {
		collector = metrics.NewCollector("mock", version.Version)
	}
	// mockRec is the recorder passed to the handlers/store. It stays a nil
	// interface when metrics are disabled (no typed-nil), so the per-command
	// recording is a true no-op rather than relying solely on the nil-safe methods.
	var mockRec server.MockMetricsRecorder
	if collector != nil {
		mockRec = collector
	}

	// Load custom routes (if any). The store overlays explicitly-set CLI flags
	// over the file values so CLI flags win; the effective merged settings are
	// then validated below so file-supplied fail-rate/fail-status are checked too.
	var routesStore *server.RoutesStore
	if cfg.Mock.Routes != "" {
		store, sErr := buildRoutesStore(ctx, cmd, mockRec)
		if sErr != nil {
			return sErr
		}
		if vErr := validateEffectiveSettings(store.Load().Settings()); vErr != nil {
			return vErr
		}
		routesStore = store

		// The effective TLS mode is fully merged at this point (applyMockFlags ran
		// first), so warn now if the loaded routes depend on a client certificate
		// the TLS layer will never request. Warning only — hot reload can change
		// the routes, so the server still starts.
		warnUnreachableClientCertRoutes(cmd, store.Load())
	}

	mockCfg := server.MockConfig{
		Builtin:       cfg.Mock.Builtin,
		Prefix:        cfg.Mock.Prefix,
		Latency:       latency,
		LatencyJitter: jitter,
		FailRate:      cfg.Mock.FailRate,
		FailStatus:    cfg.Mock.FailStatus,
		Metrics:       mockRec,
	}

	// Build the mock handler: routed (custom routes -> built-ins -> fallback)
	// when a routes file is configured, otherwise the built-ins-only handler.
	//
	// The routed handler applies global latency/fail-rate itself, reading the
	// effective values from the store snapshot per request so they hot-reload
	// with the file (CLI overrides are baked into the store by buildRoutesStore).
	// It must therefore NOT be wrapped in WithLatencyAndFailures here, or latency
	// and failures would be applied twice.
	var mockHandler http.Handler
	if routesStore != nil {
		mockHandler = server.NewRoutedHandler(server.RoutedHandlerConfig{
			Store:   routesStore,
			Builtin: cfg.Mock.Builtin,
			Prefix:  cfg.Mock.Prefix,
			Metrics: mockRec,
		})
	} else {
		mockHandler = server.NewMockHandler(mockCfg)
	}

	// Build handler chain using a mux.
	mux := http.NewServeMux()

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
		tlsCfg, tlsErr := radixTLS.NewServerTLSConfig(mockServerTLSOptions(cfg))
		if tlsErr != nil {
			return fmt.Errorf("TLS configuration error: %w", tlsErr)
		}
		srvCfg.TLSConfig = tlsCfg
	}

	srvCfg.Banner = fmt.Sprintf("Mocking API on %s://%s", scheme, addr)

	srv := server.NewServer(srvCfg)

	// Build the loopback admin server (metrics + /healthz) sharing the collector.
	// It joins the same ctx that drives graceful shutdown and the routes watcher.
	admin, err := buildAdminServer("mock", collector)
	if err != nil {
		return err
	}

	return runServers(ctx, srv, admin)
}

// warnUnreachableClientCertRoutes prints a startup warning to the command's
// error stream when the compiled routes contain require_client_cert or tls.*
// match conditions but the effective TLS mode will never request a client
// certificate — plain HTTP, or --tls without --client-auth /
// --optional-client-auth. In that combination every request hits a
// require_client_cert route as certless (a permanent 403) and tls.* conditions
// never match, which otherwise looks like a broken client. It only warns (the
// server still starts): a hot reload can change the routes, and the diagnostic
// is what turns the silent 403s into an actionable message.
func warnUnreachableClientCertRoutes(cmd *cobra.Command, compiled *server.CompiledRoutes) {
	if compiled == nil || !compiled.HasClientCertRequirements() {
		return
	}
	// A client certificate is requested only over TLS with client auth enabled
	// (required via --client-auth or requested via --optional-client-auth).
	if cfg.TLS.Enabled && (cfg.TLS.ClientAuth || cfg.TLS.ClientAuthOptional) {
		return
	}
	reason := "the server is plain HTTP and never requests one"
	if cfg.TLS.Enabled {
		reason = "TLS is enabled without --client-auth or --optional-client-auth, so one is never requested"
	}
	fmt.Fprintf(cmd.ErrOrStderr(),
		"WARNING: the routes file uses require_client_cert or tls.* match conditions, "+
			"but %s; require_client_cert routes will return 403 and tls.* conditions will "+
			"never match. Run with --tls --ca <ca.pem> and --optional-client-auth (or --client-auth) "+
			"to make them reachable.\n", reason)
}

// mockServerTLSOptions builds the TLS loader options for the mock server from
// the loaded config (the --optional-client-auth flag is already merged into
// cfg.TLS.ClientAuthOptional by applyMockFlags). It is factored out of runMock
// so the wiring — notably ClientAuthOptional — can be asserted directly in
// tests.
func mockServerTLSOptions(c *config.Config) radixTLS.ServerTLSOptions {
	return radixTLS.ServerTLSOptions{
		CertFile:           c.TLS.Cert,
		KeyFile:            c.TLS.Key,
		CAFile:             c.TLS.CA,
		ClientAuth:         c.TLS.ClientAuth,
		ClientAuthOptional: c.TLS.ClientAuthOptional,
		MinVersion:         c.TLS.MinVersion,
	}
}

// resolveRoutesArg reconciles the positional config-file argument with the
// --routes flag. A positional argument is shorthand for --routes; if both are
// explicitly provided and disagree it is ambiguous which the user meant, so an
// error is returned. Otherwise whichever was set is used (--routes when it was
// explicitly changed, else the positional arg).
func resolveRoutesArg(cmd *cobra.Command, args []string) error {
	if len(args) == 0 {
		return nil // only --routes (if any) applies; already in cfg.Mock.Routes
	}
	positional := args[0]
	if cmd.Flags().Changed("routes") && mockRoutes != positional {
		return fmt.Errorf(
			"specify the routes file either positionally (%q) or via --routes (%q), not both",
			positional, mockRoutes)
	}
	cfg.Mock.Routes = positional
	return nil
}

// buildRoutesStore loads the configured routes file, installs a CLI-override
// overlay so explicitly-set flags win over the file (and survive hot reloads),
// seeds an atomic store, and — when --watch is set — starts the hot-reload
// watcher bound to ctx. It also reflects the effective CORS setting back into
// cfg so the startup CORS middleware sees the merged value.
func buildRoutesStore(ctx context.Context, cmd *cobra.Command, rec server.MockMetricsRecorder) (*server.RoutesStore, error) {
	compiled, err := server.LoadRoutes(cfg.Mock.Routes)
	if err != nil {
		return nil, fmt.Errorf("failed to load routes file %q: %w", cfg.Mock.Routes, err)
	}

	logf := func(format string, args ...any) {
		fmt.Fprintf(cmd.OutOrStdout(), format+"\n", args...)
	}
	store := server.NewRoutesStore(cfg.Mock.Routes, compiled, logf)

	// Record hot reloads on the shared collector (nil when metrics are disabled).
	store.SetMetricsRecorder(rec)

	// Bake CLI overrides into the store's settings on every (re)load so CLI flags
	// always win over the file and survive an edit to the watched settings block.
	store.SetSettingsOverride(cliSettingsOverride(cmd))

	// Reflect the effective CORS value (file unless a CLI flag overrode it) into
	// cfg for the startup CORS middleware; CORS is not hot-reloaded.
	if !cmd.Flags().Changed("cors") {
		cfg.Mock.CORS = store.Load().Settings().CORS
	}

	if cfg.Mock.Watch {
		if watchErr := store.Watch(ctx); watchErr != nil {
			return nil, fmt.Errorf("failed to start routes watcher: %w", watchErr)
		}
		fmt.Fprintf(cmd.OutOrStdout(), "Watching %s for changes (routes, fallback, latency, fail-rate; CORS is set at startup)\n", cfg.Mock.Routes)
	}

	return store, nil
}

// cliSettingsOverride returns a function that overlays explicitly-set CLI flags
// onto a freshly-loaded RouteSettings. Precedence is CLI flag (when Changed) >
// file setting > built-in default: only flags the user actually set replace the
// file value, so the store always holds the effective settings. It is applied to
// the initial config and to every hot reload.
func cliSettingsOverride(cmd *cobra.Command) func(*server.RouteSettings) {
	changed := func(name string) bool { return cmd.Flags().Changed(name) }
	return func(s *server.RouteSettings) {
		if changed("latency") {
			s.Latency = mockLatencyDuration()
		}
		if changed("latency-jitter") {
			s.LatencyJitter = mockLatencyJitterDuration()
		}
		if changed("fail-rate") {
			s.FailRate = mockFailRate
		}
		if changed("fail-status") {
			s.FailStatus = mockFailStatus
		}
		if changed("cors") {
			s.CORS = mockCORS
		}
	}
}

// mockLatencyDuration parses the --latency flag value; a parse error cannot
// occur here because runMock validated it before the store was built, but on
// the off chance it does the value is treated as zero.
func mockLatencyDuration() time.Duration {
	d, _ := time.ParseDuration(mockLatency)
	return d
}

// mockLatencyJitterDuration parses the --latency-jitter flag value (see
// mockLatencyDuration for the error rationale).
func mockLatencyJitterDuration() time.Duration {
	d, _ := time.ParseDuration(mockLatencyJitter)
	return d
}

// validateEffectiveSettings checks the merged routes-store settings (file values
// overlaid with CLI flags) so a fail-rate/fail-status supplied only by the file
// is validated too. Latency negativity is already validated during compilation.
func validateEffectiveSettings(s server.RouteSettings) error {
	if r := s.FailRate; math.IsNaN(r) || math.IsInf(r, 0) || r < 0 || r > 100 {
		return fmt.Errorf("invalid fail_rate %g: must be between 0 and 100", r)
	}
	if c := s.FailStatus; c < 200 || c > 599 {
		return fmt.Errorf("invalid fail_status %d: must be between 200 and 599", c)
	}
	return nil
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
