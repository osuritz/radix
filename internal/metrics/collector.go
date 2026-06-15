// Package metrics provides HTTP request metrics collection and reporting.
package metrics

import (
	"encoding/json"
	"net/http"
	"sync"
	"sync/atomic"
	"time"
)

// Collector collects and aggregates HTTP metrics
type Collector struct {
	startTime time.Time
	command   string
	version   string

	// Atomic counters
	totalRequests atomic.Uint64
	totalSuccess  atomic.Uint64
	totalErrors   atomic.Uint64
	bytesSent     atomic.Uint64
	bytesReceived atomic.Uint64

	// Maps for tracking counts (protected by RWMutex for reads, using sync.Map for writes)
	statusCodes sync.Map // map[int]uint64
	methods     sync.Map // map[string]uint64

	// Response time histogram
	responseTimes *Histogram

	// Per-command counters. radix runs exactly one command per process, so a
	// single Collector only ever accumulates the counters for whichever command
	// created it (NewCollector is called with that command's name). These live
	// directly on the shared Collector — rather than a separate per-command type —
	// because there is only ever one command, every Record* call is lock-free
	// (atomic), and the snapshot emits only the active command's section. The
	// unused families for the other commands simply stay zero.
	echo  echoCounters
	mock  mockCounters
	proxy proxyCounters
}

// echoCounters holds the echo command's per-command counters. All fields are
// lock-free atomics, written from request handlers and read in Snapshot.
type echoCounters struct {
	delays     atomic.Uint64 // responses where a delay was applied
	customBody atomic.Uint64 // responses served from the configured literal body
	pathStatus atomic.Uint64 // responses whose status was derived from the request path
}

// mockCounters holds the mock command's per-command counters.
type mockCounters struct {
	routeMatchesBuiltin atomic.Uint64 // requests served by a built-in httpbin-style endpoint
	routeMatchesCustom  atomic.Uint64 // requests served by a custom YAML route
	templateRenders     atomic.Uint64 // successful response/file/SSE template renders
	templateErrors      atomic.Uint64 // template render failures
	reloads             atomic.Uint64 // successful routes-file hot reloads
	failInjections      atomic.Uint64 // requests short-circuited by the random fail-rate
	fallbackNotFound    atomic.Uint64 // unmatched requests served the 404 fallback
	fallbackProxy       atomic.Uint64 // unmatched requests served the proxy fallback
}

// proxyCounters holds the proxy command's per-command counters.
type proxyCounters struct {
	authInjections    atomic.Uint64 // requests that had auth headers injected
	streamConnections atomic.Uint64 // streaming (SSE/ndjson) responses observed
}

// NewCollector creates a new metrics collector
func NewCollector(command, version string) *Collector {
	return &Collector{
		startTime:     time.Now(),
		command:       command,
		version:       version,
		responseTimes: NewHistogram(10000), // Keep last 10k response times
	}
}

// RecordRequest records a completed HTTP request
func (c *Collector) RecordRequest(status int, method string, duration time.Duration, bytesIn, bytesOut int64) {
	// Update atomic counters
	c.totalRequests.Add(1)

	if status >= 200 && status < 400 {
		c.totalSuccess.Add(1)
	} else if status >= 400 {
		c.totalErrors.Add(1)
	}

	if bytesOut > 0 {
		c.bytesSent.Add(uint64(bytesOut))
	}
	if bytesIn > 0 {
		c.bytesReceived.Add(uint64(bytesIn))
	}

	// Update status code count
	c.incrementMapCounter(&c.statusCodes, status)

	// Update method count
	c.incrementMapCounter(&c.methods, method)

	// Record response time
	c.responseTimes.Record(duration)
}

// incrementMapCounter atomically increments a counter in a sync.Map
func (c *Collector) incrementMapCounter(m *sync.Map, key interface{}) {
	for {
		val, loaded := m.LoadOrStore(key, uint64(1))
		if !loaded {
			// Successfully stored initial value
			return
		}

		// Try to increment the existing value
		oldVal, ok := val.(uint64)
		if !ok {
			// This should never happen since we control what goes into the map
			return
		}
		if m.CompareAndSwap(key, oldVal, oldVal+1) {
			return
		}
		// If CAS failed, retry
	}
}

// All Record* methods below are safe to call on a nil *Collector: metrics are
// opt-out (--metrics=false), in which case no collector exists and the handlers
// hold a nil one. A nil-receiver guard makes every recording call a cheap no-op
// when metrics are disabled, so call sites need no extra nil checks and there is
// zero behavior change.

// RecordEchoDelay records that an echo response applied a (non-zero) delay.
func (c *Collector) RecordEchoDelay() {
	if c == nil {
		return
	}
	c.echo.delays.Add(1)
}

// RecordEchoCustomBody records that an echo response served the configured
// literal body instead of the echo JSON.
func (c *Collector) RecordEchoCustomBody() {
	if c == nil {
		return
	}
	c.echo.customBody.Add(1)
}

// RecordEchoPathStatus records that an echo response derived its status code
// from the request path (e.g. /404).
func (c *Collector) RecordEchoPathStatus() {
	if c == nil {
		return
	}
	c.echo.pathStatus.Add(1)
}

// RecordMockRouteMatch records a mock route match. custom selects the custom
// YAML route family; otherwise the built-in httpbin-style endpoint family.
func (c *Collector) RecordMockRouteMatch(custom bool) {
	if c == nil {
		return
	}
	if custom {
		c.mock.routeMatchesCustom.Add(1)
		return
	}
	c.mock.routeMatchesBuiltin.Add(1)
}

// RecordMockTemplateRender records a successful mock response template render.
func (c *Collector) RecordMockTemplateRender() {
	if c == nil {
		return
	}
	c.mock.templateRenders.Add(1)
}

// RecordMockTemplateError records a failed mock response template render.
func (c *Collector) RecordMockTemplateError() {
	if c == nil {
		return
	}
	c.mock.templateErrors.Add(1)
}

// RecordMockReload records a successful mock routes-file hot reload.
func (c *Collector) RecordMockReload() {
	if c == nil {
		return
	}
	c.mock.reloads.Add(1)
}

// RecordMockFailInjection records a request short-circuited by the mock
// random fail-rate.
func (c *Collector) RecordMockFailInjection() {
	if c == nil {
		return
	}
	c.mock.failInjections.Add(1)
}

// RecordMockFallback records an unmatched mock request served by the fallback.
// kind is the fallback type ("404" or "proxy"); any other value is ignored.
func (c *Collector) RecordMockFallback(kind string) {
	if c == nil {
		return
	}
	switch kind {
	case "404", "not_found":
		c.mock.fallbackNotFound.Add(1)
	case "proxy":
		c.mock.fallbackProxy.Add(1)
	}
}

// RecordProxyAuthInjection records a proxied request that had auth headers
// injected by a HeaderProvider.
func (c *Collector) RecordProxyAuthInjection() {
	if c == nil {
		return
	}
	c.proxy.authInjections.Add(1)
}

// RecordProxyStream records a proxied response detected as a streaming
// (SSE/ndjson) connection.
func (c *Collector) RecordProxyStream() {
	if c == nil {
		return
	}
	c.proxy.streamConnections.Add(1)
}

// Metrics represents the complete metrics snapshot
type Metrics struct {
	Server        ServerMetrics     `json:"server"`
	Requests      RequestMetrics    `json:"requests"`
	StatusCodes   map[string]uint64 `json:"status_codes"`
	Methods       map[string]uint64 `json:"methods"`
	ResponseTimes HistogramSnapshot `json:"response_times"`
	Bandwidth     BandwidthMetrics  `json:"bandwidth"`

	// Command holds command-specific counters. Exactly one of its fields is
	// populated — the one matching Server.Command — and the rest are nil, so the
	// JSON snapshot carries a single nested object for the running command (e.g.
	// "echo", "mock", or "proxy") and omits the sections that do not apply. For
	// commands without per-command counters (e.g. serve) the whole field is nil
	// and the "command" object is omitted entirely (it is a pointer so
	// encoding/json's omitempty drops it).
	Command *CommandMetrics `json:"command,omitempty"`
}

// CommandMetrics is the per-command counters section of a snapshot. Only the
// field matching the active command is non-nil; the others are omitted from
// JSON. This keeps the snapshot shape stable per command and avoids emitting
// always-zero counters for the commands that are not running.
type CommandMetrics struct {
	Echo  *EchoMetrics  `json:"echo,omitempty"`
	Mock  *MockMetrics  `json:"mock,omitempty"`
	Proxy *ProxyMetrics `json:"proxy,omitempty"`
}

// EchoMetrics contains the echo command's per-command counters.
type EchoMetrics struct {
	DelaysApplied      uint64 `json:"delays_applied"`
	CustomBodyResponse uint64 `json:"custom_body_responses"`
	PathStatusHits     uint64 `json:"path_status_hits"`
}

// MockMetrics contains the mock command's per-command counters.
type MockMetrics struct {
	RouteMatchesBuiltin uint64 `json:"route_matches_builtin"`
	RouteMatchesCustom  uint64 `json:"route_matches_custom"`
	TemplateRenders     uint64 `json:"template_renders"`
	TemplateErrors      uint64 `json:"template_errors"`
	Reloads             uint64 `json:"reloads"`
	FailInjections      uint64 `json:"fail_injections"`
	FallbackNotFound    uint64 `json:"fallback_not_found"`
	FallbackProxy       uint64 `json:"fallback_proxy"`
}

// ProxyMetrics contains the proxy command's per-command counters.
type ProxyMetrics struct {
	AuthInjections    uint64 `json:"auth_injections"`
	StreamConnections uint64 `json:"stream_connections"`
}

// ServerMetrics contains server information
type ServerMetrics struct {
	Command       string    `json:"command"`
	UptimeSeconds float64   `json:"uptime_seconds"`
	StartTime     time.Time `json:"start_time"`
	Version       string    `json:"version"`
}

// RequestMetrics contains request statistics
type RequestMetrics struct {
	Total         uint64  `json:"total"`
	Success       uint64  `json:"success"`
	Errors        uint64  `json:"errors"`
	RatePerSecond float64 `json:"rate_per_second"`
}

// BandwidthMetrics contains bandwidth statistics
type BandwidthMetrics struct {
	BytesSent            uint64  `json:"bytes_sent"`
	BytesReceived        uint64  `json:"bytes_received"`
	AvgRequestSizeBytes  float64 `json:"avg_request_size_bytes,omitempty"`
	AvgResponseSizeBytes float64 `json:"avg_response_size_bytes,omitempty"`
}

// Snapshot returns a snapshot of the current metrics
func (c *Collector) Snapshot() Metrics {
	uptime := time.Since(c.startTime).Seconds()
	totalReqs := c.totalRequests.Load()
	successReqs := c.totalSuccess.Load()
	errorReqs := c.totalErrors.Load()
	bytesSent := c.bytesSent.Load()
	bytesRecv := c.bytesReceived.Load()

	// Calculate rate
	var rate float64
	if uptime > 0 {
		rate = float64(totalReqs) / uptime
	}

	// Convert status codes map
	statusCodes := make(map[string]uint64)
	c.statusCodes.Range(func(key, value interface{}) bool {
		statusCode, okKey := key.(int)
		count, okVal := value.(uint64)
		if okKey && okVal {
			statusCodes[http.StatusText(statusCode)] = count
		}
		return true
	})

	// Convert methods map
	methods := make(map[string]uint64)
	c.methods.Range(func(key, value interface{}) bool {
		method, okKey := key.(string)
		count, okVal := value.(uint64)
		if okKey && okVal {
			methods[method] = count
		}
		return true
	})

	// Calculate bandwidth averages
	var avgReqSize, avgRespSize float64
	if totalReqs > 0 {
		avgReqSize = float64(bytesRecv) / float64(totalReqs)
		avgRespSize = float64(bytesSent) / float64(totalReqs)
	}

	return Metrics{
		Server: ServerMetrics{
			Command:       c.command,
			UptimeSeconds: uptime,
			StartTime:     c.startTime,
			Version:       c.version,
		},
		Requests: RequestMetrics{
			Total:         totalReqs,
			Success:       successReqs,
			Errors:        errorReqs,
			RatePerSecond: rate,
		},
		StatusCodes:   statusCodes,
		Methods:       methods,
		ResponseTimes: c.responseTimes.Snapshot(),
		Bandwidth: BandwidthMetrics{
			BytesSent:            bytesSent,
			BytesReceived:        bytesRecv,
			AvgRequestSizeBytes:  avgReqSize,
			AvgResponseSizeBytes: avgRespSize,
		},
		Command: c.commandSnapshot(),
	}
}

// commandSnapshot returns the per-command counters section for the active
// command, or nil for a command (e.g. serve) that has no per-command counters.
// Only the section matching c.command is populated; the rest stay nil so the
// snapshot omits sections that do not apply.
func (c *Collector) commandSnapshot() *CommandMetrics {
	switch c.command {
	case "echo":
		return &CommandMetrics{Echo: &EchoMetrics{
			DelaysApplied:      c.echo.delays.Load(),
			CustomBodyResponse: c.echo.customBody.Load(),
			PathStatusHits:     c.echo.pathStatus.Load(),
		}}
	case "mock":
		return &CommandMetrics{Mock: &MockMetrics{
			RouteMatchesBuiltin: c.mock.routeMatchesBuiltin.Load(),
			RouteMatchesCustom:  c.mock.routeMatchesCustom.Load(),
			TemplateRenders:     c.mock.templateRenders.Load(),
			TemplateErrors:      c.mock.templateErrors.Load(),
			Reloads:             c.mock.reloads.Load(),
			FailInjections:      c.mock.failInjections.Load(),
			FallbackNotFound:    c.mock.fallbackNotFound.Load(),
			FallbackProxy:       c.mock.fallbackProxy.Load(),
		}}
	case "proxy":
		return &CommandMetrics{Proxy: &ProxyMetrics{
			AuthInjections:    c.proxy.authInjections.Load(),
			StreamConnections: c.proxy.streamConnections.Load(),
		}}
	default:
		return nil
	}
}

// Handler returns an HTTP handler for the metrics endpoint
func (c *Collector) Handler(format string) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		snapshot := c.Snapshot()

		switch format {
		case "prometheus":
			c.writePrometheus(w, &snapshot)
		default: // json and others
			c.writeJSON(w, &snapshot)
		}
	}
}

// writeJSON writes metrics in JSON format
func (c *Collector) writeJSON(w http.ResponseWriter, metrics *Metrics) {
	w.Header().Set("Content-Type", "application/json")
	encoder := json.NewEncoder(w)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(metrics); err != nil {
		http.Error(w, "Failed to encode metrics", http.StatusInternalServerError)
	}
}

// writePrometheus writes metrics in Prometheus text format
func (c *Collector) writePrometheus(w http.ResponseWriter, metrics *Metrics) {
	w.Header().Set("Content-Type", "text/plain; version=0.0.4")

	// Use the dedicated Prometheus exporter
	exporter := NewPrometheusExporter(c.command)
	exporter.Export(w, metrics)
}

// Reset clears all metrics (useful for testing)
func (c *Collector) Reset() {
	c.totalRequests.Store(0)
	c.totalSuccess.Store(0)
	c.totalErrors.Store(0)
	c.bytesSent.Store(0)
	c.bytesReceived.Store(0)

	c.statusCodes = sync.Map{}
	c.methods = sync.Map{}
	c.responseTimes.Reset()

	// Per-command counters.
	c.echo.delays.Store(0)
	c.echo.customBody.Store(0)
	c.echo.pathStatus.Store(0)

	c.mock.routeMatchesBuiltin.Store(0)
	c.mock.routeMatchesCustom.Store(0)
	c.mock.templateRenders.Store(0)
	c.mock.templateErrors.Store(0)
	c.mock.reloads.Store(0)
	c.mock.failInjections.Store(0)
	c.mock.fallbackNotFound.Store(0)
	c.mock.fallbackProxy.Store(0)

	c.proxy.authInjections.Store(0)
	c.proxy.streamConnections.Store(0)

	c.startTime = time.Now()
}
