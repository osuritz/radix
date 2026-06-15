package server

import (
	"context"
	"crypto/tls"
	"fmt"
	"net/http"
	"net/url"
	"path/filepath"
	"sync/atomic"
	"time"

	"github.com/fsnotify/fsnotify"
)

// watchDebounce is the quiet period the watcher waits after the last fsnotify
// event before reloading. It coalesces the burst of events an editor emits
// while saving (and the partial-write WRITE events fsnotify fires mid-write) so
// the reload reads a fully-written file rather than a truncated one.
const watchDebounce = 100 * time.Millisecond

// RoutesStore holds the active CompiledRoutes behind an atomic pointer so that
// request handling reads the current configuration lock-free while a background
// watcher swaps in reloaded configurations. The zero value is not usable; use
// NewRoutesStore.
type RoutesStore struct {
	current   atomic.Pointer[CompiledRoutes]
	path      string
	logf      func(format string, args ...any)
	overrides func(*RouteSettings) // applied to freshly-loaded settings on each (re)load
	metrics   MockMetricsRecorder  // records hot reloads; nil when metrics are disabled
}

// NewRoutesStore creates a store seeded with an initial CompiledRoutes. logf, if
// nil, discards reload messages; otherwise it receives human-readable reload and
// error notices (e.g. wired to the server's stdout).
func NewRoutesStore(path string, initial *CompiledRoutes, logf func(format string, args ...any)) *RoutesStore {
	if logf == nil {
		logf = func(string, ...any) {}
	}
	s := &RoutesStore{path: path, logf: logf}
	s.current.Store(initial)
	return s
}

// SetMetricsRecorder installs the recorder that counts successful hot reloads.
// It is nil-safe to leave unset (no reload counting); pass the shared collector
// to enable it. It must be called before the watcher starts.
func (s *RoutesStore) SetMetricsRecorder(rec MockMetricsRecorder) {
	s.metrics = rec
}

// SetSettingsOverride installs a function that adjusts the effective settings of
// every loaded configuration, including the initial one and every hot reload. It
// is used by the CLI to bake explicitly-set flags (latency, fail-rate, etc.)
// over the file values so the store always holds the effective settings, and so
// those overrides survive a reload (otherwise an edit to the watched file would
// drop them). It must be called before the watcher starts. If fn is nil this is
// a no-op. The override is applied immediately to the current configuration.
func (s *RoutesStore) SetSettingsOverride(fn func(*RouteSettings)) {
	s.overrides = fn
	if fn == nil {
		return
	}
	cur := s.current.Load()
	if cur == nil {
		return
	}
	merged := *cur
	fn(&merged.settings)
	s.current.Store(&merged)
}

// Load returns the currently active compiled routes. It is safe for concurrent
// use and never blocks on a reload.
func (s *RoutesStore) Load() *CompiledRoutes {
	return s.current.Load()
}

// Reload re-reads and recompiles the routes file, validating it before swapping
// it in. On any parse/compile error the previous good configuration is kept and
// the error is returned (and logged), so a broken edit never takes the server
// down. It is safe to call concurrently and directly from tests.
func (s *RoutesStore) Reload() error {
	compiled, err := LoadRoutes(s.path)
	if err != nil {
		s.logf("mock: routes reload failed, keeping previous config: %v", err)
		return err
	}
	if s.overrides != nil {
		s.overrides(&compiled.settings)
	}
	s.current.Store(compiled)
	if s.metrics != nil {
		s.metrics.RecordMockReload()
	}
	s.logf("mock: routes reloaded from %s", s.path)
	return nil
}

// Watch starts a background goroutine that watches the routes file (and its
// parent directory, to survive editor rename-on-save) and reloads the store on
// change. It returns once the watcher is established; the goroutine runs until
// ctx is canceled, at which point the underlying fsnotify watcher is closed.
func (s *RoutesStore) Watch(ctx context.Context) error {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return fmt.Errorf("mock: create file watcher: %w", err)
	}

	abs, err := filepath.Abs(s.path)
	if err != nil {
		_ = watcher.Close()
		return fmt.Errorf("mock: resolve routes path: %w", err)
	}
	// Watch the parent directory: many editors replace files on save (rename),
	// which removes the watch on the file itself. Watching the directory lets us
	// see writes/creates/renames affecting the target file.
	dir := filepath.Dir(abs)
	if err := watcher.Add(dir); err != nil {
		_ = watcher.Close()
		return fmt.Errorf("mock: watch directory %q: %w", dir, err)
	}

	go s.watchLoop(ctx, watcher, abs)
	return nil
}

// watchLoop consumes fsnotify events until ctx is canceled, reloading the store
// whenever the watched routes file is written, created, or renamed.
//
// Events are debounced: rather than reloading on every WRITE, it (re)arms a
// short quiet-period timer on each relevant event and reloads only once the
// timer fires. This coalesces the event burst an editor emits while saving and,
// critically, avoids reading a partially-written file mid-WRITE (which would
// otherwise swap in a valid-but-empty config). It also reduces redundant
// reloads. Rename-on-save is still observed because the parent directory is
// watched (see Watch).
func (s *RoutesStore) watchLoop(ctx context.Context, watcher *fsnotify.Watcher, target string) {
	defer func() { _ = watcher.Close() }()

	// A stopped timer with a drained channel; armed on the first relevant event.
	timer := time.NewTimer(watchDebounce)
	if !timer.Stop() {
		<-timer.C
	}
	defer timer.Stop()
	pending := false

	for {
		select {
		case <-ctx.Done():
			return
		case event, ok := <-watcher.Events:
			if !ok {
				return
			}
			if filepath.Clean(event.Name) != target {
				continue
			}
			if event.Op&(fsnotify.Write|fsnotify.Create|fsnotify.Rename) != 0 {
				// (Re)arm the debounce timer; the reload happens once events go
				// quiet, by which point the writer has finished.
				if pending && !timer.Stop() {
					<-timer.C
				}
				timer.Reset(watchDebounce)
				pending = true
			}
		case <-timer.C:
			pending = false
			// Reload errors are logged inside Reload; the previous config stays.
			_ = s.Reload()
		case watchErr, ok := <-watcher.Errors:
			if !ok {
				return
			}
			if watchErr != nil {
				s.logf("mock: file watcher error: %v", watchErr)
			}
		}
	}
}

// RoutedHandlerConfig configures NewRoutedHandler.
type RoutedHandlerConfig struct {
	// Store provides the active compiled routes (supports hot reload).
	Store *RoutesStore

	// Builtin enables the built-in httpbin-style endpoints as the layer beneath
	// custom routes (matched only when no custom route matches).
	Builtin bool

	// Prefix mounts the built-in endpoints under this path prefix.
	Prefix string

	// FallbackProxyTLS is an optional TLS config applied to a fallback proxy
	// (used when the active settings select a TLS backend). Usually nil.
	FallbackProxyTLS *tls.Config

	// Metrics, when non-nil, records per-command mock counters for the routed
	// handler (custom/built-in route matches, template renders/errors, fail
	// injections, and fallback hits). It is nil when metrics are disabled;
	// MockMetricsRecorder methods are nil-safe so recording never affects
	// responses.
	Metrics MockMetricsRecorder
}

// NewRoutedHandler builds the layered mock handler: custom routes (from the
// store) take precedence; unmatched requests fall through to the built-in
// endpoints (when enabled); anything still unmatched hits the configured
// fallback (404 or proxy).
//
// Global latency and fail-rate are read from the active store snapshot on every
// request (routes.settings), so editing the `settings:` block under --watch
// hot-reloads them along with the routes and fallback. The CLI bakes its
// explicitly-set flags into the stored settings on each (re)load, so CLI flags
// still win over the file. CORS is NOT applied here: it is an outer startup
// middleware and does not hot-reload.
//
// The handler reads the active routes from the store on every request, so a
// hot-reload swap is observed immediately and without locking.
func NewRoutedHandler(cfg RoutedHandlerConfig) http.Handler {
	// Built-in endpoints. Latency/failures are applied by the outer closure
	// below using the live store settings, so they are not wrapped here.
	var builtins *http.ServeMux
	if cfg.Builtin {
		builtins = http.NewServeMux()
		registerBuiltins(builtins, NormalizePrefix(cfg.Prefix))
	}

	dispatch := func(w http.ResponseWriter, r *http.Request, routes *CompiledRoutes) {
		// 1-5: custom route match wins.
		if cr, params, ok := routes.match(r.Method, r.URL.Path); ok {
			if cfg.Metrics != nil {
				cfg.Metrics.RecordMockRouteMatch(true)
			}
			cr.serve(w, r, params, routes.baseDir, cfg.Metrics)
			return
		}

		// 6: built-in endpoints (when enabled). ServeMux.Handler reports the
		// matched pattern; an empty pattern means no built-in matched, so we
		// fall through to the configured fallback instead of serving the mux's
		// own 404.
		if builtins != nil {
			if _, pattern := builtins.Handler(r); pattern != "" {
				if cfg.Metrics != nil {
					cfg.Metrics.RecordMockRouteMatch(false)
				}
				builtins.ServeHTTP(w, r)
				return
			}
		}

		// 7: fallback.
		serveFallback(w, r, routes.settings.Fallback, cfg.FallbackProxyTLS, cfg.Metrics)
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		routes := cfg.Store.Load()
		// Apply the effective global latency/fail-rate from the live snapshot so
		// they hot-reload with the file (CLI overrides are already baked in).
		applyLatencyAndFailures(w, r, routes.settings.latencyFail(cfg.Metrics), func(w http.ResponseWriter, r *http.Request) {
			dispatch(w, r, routes)
		})
	})
}

// serveFallback handles requests that matched neither a custom route nor a
// built-in endpoint, per the configured fallback policy. rec, when non-nil,
// records the fallback hit (404 vs proxy).
func serveFallback(w http.ResponseWriter, r *http.Request, fb FallbackConfig, tlsConfig *tls.Config, rec MockMetricsRecorder) {
	if fb.Type == FallbackProxy {
		if rec != nil {
			rec.RecordMockFallback("proxy")
		}
		target, err := url.Parse(fb.ProxyTarget)
		if err != nil {
			http.Error(w, fmt.Sprintf("mock: invalid fallback proxy target: %v", err), http.StatusBadGateway)
			return
		}
		NewReverseProxy(ProxyConfig{Target: target, TLSConfig: tlsConfig}).ServeHTTP(w, r)
		return
	}
	if rec != nil {
		rec.RecordMockFallback("not_found")
	}
	http.NotFound(w, r)
}
