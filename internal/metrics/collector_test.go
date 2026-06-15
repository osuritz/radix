package metrics

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestNewCollector(t *testing.T) {
	c := NewCollector("test", "1.0.0")

	if c == nil {
		t.Fatal("NewCollector returned nil")
	}

	if c.command != "test" {
		t.Errorf("command = %s, want test", c.command)
	}

	if c.version != "1.0.0" {
		t.Errorf("version = %s, want 1.0.0", c.version)
	}
}

func TestCollectorRecordRequest(t *testing.T) {
	c := NewCollector("test", "1.0.0")

	// Record a successful request
	c.RecordRequest(200, "GET", 50*time.Millisecond, 100, 1024)

	snapshot := c.Snapshot()

	if snapshot.Requests.Total != 1 {
		t.Errorf("total requests = %d, want 1", snapshot.Requests.Total)
	}

	if snapshot.Requests.Success != 1 {
		t.Errorf("successful requests = %d, want 1", snapshot.Requests.Success)
	}

	if snapshot.Requests.Errors != 0 {
		t.Errorf("error requests = %d, want 0", snapshot.Requests.Errors)
	}

	if snapshot.Bandwidth.BytesSent != 1024 {
		t.Errorf("bytes sent = %d, want 1024", snapshot.Bandwidth.BytesSent)
	}

	if snapshot.Bandwidth.BytesReceived != 100 {
		t.Errorf("bytes received = %d, want 100", snapshot.Bandwidth.BytesReceived)
	}
}

func TestCollectorStatusCodes(t *testing.T) {
	c := NewCollector("test", "1.0.0")

	// Record requests with different status codes
	c.RecordRequest(200, "GET", 10*time.Millisecond, 0, 100)
	c.RecordRequest(200, "GET", 20*time.Millisecond, 0, 200)
	c.RecordRequest(404, "GET", 15*time.Millisecond, 0, 50)
	c.RecordRequest(500, "POST", 30*time.Millisecond, 0, 75)

	snapshot := c.Snapshot()

	// Check total
	if snapshot.Requests.Total != 4 {
		t.Errorf("total = %d, want 4", snapshot.Requests.Total)
	}

	// Check success/errors
	if snapshot.Requests.Success != 2 {
		t.Errorf("success = %d, want 2", snapshot.Requests.Success)
	}

	if snapshot.Requests.Errors != 2 {
		t.Errorf("errors = %d, want 2", snapshot.Requests.Errors)
	}

	// Check status code breakdown
	if len(snapshot.StatusCodes) == 0 {
		t.Fatal("status codes map is empty")
	}
}

func TestCollectorMethods(t *testing.T) {
	c := NewCollector("test", "1.0.0")

	// Record requests with different methods
	c.RecordRequest(200, "GET", 10*time.Millisecond, 0, 100)
	c.RecordRequest(200, "GET", 10*time.Millisecond, 0, 100)
	c.RecordRequest(201, "POST", 20*time.Millisecond, 0, 200)
	c.RecordRequest(204, "DELETE", 15*time.Millisecond, 0, 0)

	snapshot := c.Snapshot()

	// Check methods
	if len(snapshot.Methods) != 3 {
		t.Errorf("methods count = %d, want 3", len(snapshot.Methods))
	}

	if snapshot.Methods["GET"] != 2 {
		t.Errorf("GET count = %d, want 2", snapshot.Methods["GET"])
	}

	if snapshot.Methods["POST"] != 1 {
		t.Errorf("POST count = %d, want 1", snapshot.Methods["POST"])
	}

	if snapshot.Methods["DELETE"] != 1 {
		t.Errorf("DELETE count = %d, want 1", snapshot.Methods["DELETE"])
	}
}

func TestCollectorResponseTimes(t *testing.T) {
	c := NewCollector("test", "1.0.0")

	// Record requests with different response times
	durations := []time.Duration{
		10 * time.Millisecond,
		20 * time.Millisecond,
		30 * time.Millisecond,
		40 * time.Millisecond,
		50 * time.Millisecond,
	}

	for _, d := range durations {
		c.RecordRequest(200, "GET", d, 0, 100)
	}

	snapshot := c.Snapshot()

	// Check that response times were recorded
	if snapshot.ResponseTimes.Count != 5 {
		t.Errorf("response time count = %d, want 5", snapshot.ResponseTimes.Count)
	}

	// Check average
	if snapshot.ResponseTimes.Avg < 29 || snapshot.ResponseTimes.Avg > 31 {
		t.Errorf("avg response time = %.2f, want ~30", snapshot.ResponseTimes.Avg)
	}
}

func TestCollectorBandwidth(t *testing.T) {
	c := NewCollector("test", "1.0.0")

	// Record requests with different sizes
	c.RecordRequest(200, "GET", 10*time.Millisecond, 100, 1000)
	c.RecordRequest(200, "POST", 20*time.Millisecond, 200, 2000)
	c.RecordRequest(200, "PUT", 15*time.Millisecond, 300, 3000)

	snapshot := c.Snapshot()

	// Check total bandwidth
	if snapshot.Bandwidth.BytesSent != 6000 {
		t.Errorf("bytes sent = %d, want 6000", snapshot.Bandwidth.BytesSent)
	}

	if snapshot.Bandwidth.BytesReceived != 600 {
		t.Errorf("bytes received = %d, want 600", snapshot.Bandwidth.BytesReceived)
	}

	// Check averages
	expectedAvgReq := 200.0   // 600 / 3
	expectedAvgResp := 2000.0 // 6000 / 3

	if snapshot.Bandwidth.AvgRequestSizeBytes != expectedAvgReq {
		t.Errorf("avg request size = %.2f, want %.2f",
			snapshot.Bandwidth.AvgRequestSizeBytes, expectedAvgReq)
	}

	if snapshot.Bandwidth.AvgResponseSizeBytes != expectedAvgResp {
		t.Errorf("avg response size = %.2f, want %.2f",
			snapshot.Bandwidth.AvgResponseSizeBytes, expectedAvgResp)
	}
}

func TestCollectorRequestRate(t *testing.T) {
	c := NewCollector("test", "1.0.0")

	// Record some requests
	for i := 0; i < 10; i++ {
		c.RecordRequest(200, "GET", 10*time.Millisecond, 0, 100)
	}

	// Wait a bit to get measurable uptime
	time.Sleep(100 * time.Millisecond)

	snapshot := c.Snapshot()

	// Rate should be > 0
	if snapshot.Requests.RatePerSecond <= 0 {
		t.Errorf("rate per second = %.2f, want > 0", snapshot.Requests.RatePerSecond)
	}

	// Rate should be reasonable (less than 1000 req/s for 10 requests in 100ms)
	if snapshot.Requests.RatePerSecond > 1000 {
		t.Errorf("rate per second = %.2f, seems too high", snapshot.Requests.RatePerSecond)
	}
}

func TestCollectorReset(t *testing.T) {
	c := NewCollector("test", "1.0.0")

	// Record some requests
	c.RecordRequest(200, "GET", 10*time.Millisecond, 100, 1000)
	c.RecordRequest(404, "POST", 20*time.Millisecond, 200, 500)

	// Reset
	c.Reset()

	snapshot := c.Snapshot()

	// Everything should be zero
	if snapshot.Requests.Total != 0 {
		t.Errorf("total after reset = %d, want 0", snapshot.Requests.Total)
	}

	if snapshot.Requests.Success != 0 {
		t.Errorf("success after reset = %d, want 0", snapshot.Requests.Success)
	}

	if snapshot.Requests.Errors != 0 {
		t.Errorf("errors after reset = %d, want 0", snapshot.Requests.Errors)
	}

	if snapshot.Bandwidth.BytesSent != 0 {
		t.Errorf("bytes sent after reset = %d, want 0", snapshot.Bandwidth.BytesSent)
	}
}

func TestCollectorHandlerJSON(t *testing.T) {
	c := NewCollector("test", "1.0.0")

	// Record some requests
	c.RecordRequest(200, "GET", 10*time.Millisecond, 100, 1000)

	// Create test request
	req := httptest.NewRequest("GET", "/_metrics", nil)
	rec := httptest.NewRecorder()

	// Get handler
	handler := c.Handler("json")
	handler(rec, req)

	// Check status code
	if rec.Code != http.StatusOK {
		t.Errorf("status code = %d, want %d", rec.Code, http.StatusOK)
	}

	// Check content type
	contentType := rec.Header().Get("Content-Type")
	if contentType != "application/json" {
		t.Errorf("content type = %s, want application/json", contentType)
	}

	// Parse JSON response
	var metrics Metrics
	if err := json.Unmarshal(rec.Body.Bytes(), &metrics); err != nil {
		t.Fatalf("failed to parse JSON: %v", err)
	}

	// Check data
	if metrics.Requests.Total != 1 {
		t.Errorf("total in JSON = %d, want 1", metrics.Requests.Total)
	}
}

func TestCollectorHandlerPrometheus(t *testing.T) {
	c := NewCollector("test", "1.0.0")

	// Record some requests
	c.RecordRequest(200, "GET", 10*time.Millisecond, 100, 1000)

	// Create test request
	req := httptest.NewRequest("GET", "/_metrics", nil)
	rec := httptest.NewRecorder()

	// Get handler
	handler := c.Handler("prometheus")
	handler(rec, req)

	// Check status code
	if rec.Code != http.StatusOK {
		t.Errorf("status code = %d, want %d", rec.Code, http.StatusOK)
	}

	// Check content type
	contentType := rec.Header().Get("Content-Type")
	if contentType != "text/plain; version=0.0.4" {
		t.Errorf("content type = %s, want text/plain", contentType)
	}

	// Check that response contains Prometheus metrics
	body := rec.Body.String()
	if len(body) == 0 {
		t.Error("Prometheus output is empty")
	}

	// Check for expected metric names
	expectedMetrics := []string{
		"radix_server_info",
		"radix_requests_total",
		"radix_bytes_sent_total",
	}

	for _, metric := range expectedMetrics {
		if !contains(body, metric) {
			t.Errorf("Prometheus output missing metric: %s", metric)
		}
	}
}

func TestCollectorConcurrency(t *testing.T) {
	c := NewCollector("test", "1.0.0")

	// Record from multiple goroutines
	done := make(chan bool)
	for i := 0; i < 10; i++ {
		go func() {
			for j := 0; j < 100; j++ {
				c.RecordRequest(200, "GET", 10*time.Millisecond, 100, 1000)
			}
			done <- true
		}()
	}

	// Wait for all goroutines
	for i := 0; i < 10; i++ {
		<-done
	}

	snapshot := c.Snapshot()

	// Should have 1000 requests
	if snapshot.Requests.Total != 1000 {
		t.Errorf("total = %d, want 1000", snapshot.Requests.Total)
	}
}

// Helper function to check if string contains substring
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > len(substr) && (s[:len(substr)] == substr || contains(s[1:], substr)))
}

// prometheusOutput renders the collector's Prometheus exposition for assertions.
func prometheusOutput(t *testing.T, c *Collector) string {
	t.Helper()
	req := httptest.NewRequest("GET", "/_metrics", nil)
	rec := httptest.NewRecorder()
	c.Handler("prometheus")(rec, req)
	return rec.Body.String()
}

// TestCommandMetricsEcho verifies the echo per-command counters increment and
// surface in both the JSON snapshot and the Prometheus output, and that only the
// echo section is emitted.
func TestCommandMetricsEcho(t *testing.T) {
	c := NewCollector("echo", "1.0.0")

	c.RecordEchoDelay()
	c.RecordEchoDelay()
	c.RecordEchoCustomBody()
	c.RecordEchoPathStatus()
	c.RecordEchoPathStatus()
	c.RecordEchoPathStatus()

	snap := c.Snapshot()
	if snap.Command.Echo == nil {
		t.Fatal("snapshot Command.Echo is nil for echo command")
	}
	if snap.Command.Mock != nil || snap.Command.Proxy != nil {
		t.Error("non-echo command sections should be nil for echo command")
	}
	if got := snap.Command.Echo.DelaysApplied; got != 2 {
		t.Errorf("DelaysApplied = %d, want 2", got)
	}
	if got := snap.Command.Echo.CustomBodyResponse; got != 1 {
		t.Errorf("CustomBodyResponse = %d, want 1", got)
	}
	if got := snap.Command.Echo.PathStatusHits; got != 3 {
		t.Errorf("PathStatusHits = %d, want 3", got)
	}

	out := prometheusOutput(t, c)
	for _, want := range []string{
		`radix_echo_delays_total{command="echo"} 2`,
		`radix_echo_custom_body_total{command="echo"} 1`,
		`radix_echo_path_status_total{command="echo"} 3`,
	} {
		if !contains(out, want) {
			t.Errorf("Prometheus output missing %q\n%s", want, out)
		}
	}
	// The other commands' families must not appear.
	for _, unexpected := range []string{"radix_mock_", "radix_proxy_"} {
		if contains(out, unexpected) {
			t.Errorf("Prometheus output unexpectedly contains %q", unexpected)
		}
	}
}

// TestCommandMetricsMock verifies the mock per-command counters increment and
// surface in both the JSON snapshot and the Prometheus output.
func TestCommandMetricsMock(t *testing.T) {
	c := NewCollector("mock", "1.0.0")

	c.RecordMockRouteMatch(true)  // custom
	c.RecordMockRouteMatch(true)  // custom
	c.RecordMockRouteMatch(false) // builtin
	c.RecordMockTemplateRender()
	c.RecordMockTemplateError()
	c.RecordMockReload()
	c.RecordMockFailInjection()
	c.RecordMockFallback("not_found")
	c.RecordMockFallback("404") // alias for not_found
	c.RecordMockFallback("proxy")

	snap := c.Snapshot()
	m := snap.Command.Mock
	if m == nil {
		t.Fatal("snapshot Command.Mock is nil for mock command")
	}
	if m.RouteMatchesCustom != 2 {
		t.Errorf("RouteMatchesCustom = %d, want 2", m.RouteMatchesCustom)
	}
	if m.RouteMatchesBuiltin != 1 {
		t.Errorf("RouteMatchesBuiltin = %d, want 1", m.RouteMatchesBuiltin)
	}
	if m.TemplateRenders != 1 {
		t.Errorf("TemplateRenders = %d, want 1", m.TemplateRenders)
	}
	if m.TemplateErrors != 1 {
		t.Errorf("TemplateErrors = %d, want 1", m.TemplateErrors)
	}
	if m.Reloads != 1 {
		t.Errorf("Reloads = %d, want 1", m.Reloads)
	}
	if m.FailInjections != 1 {
		t.Errorf("FailInjections = %d, want 1", m.FailInjections)
	}
	if m.FallbackNotFound != 2 {
		t.Errorf("FallbackNotFound = %d, want 2", m.FallbackNotFound)
	}
	if m.FallbackProxy != 1 {
		t.Errorf("FallbackProxy = %d, want 1", m.FallbackProxy)
	}

	out := prometheusOutput(t, c)
	for _, want := range []string{
		`radix_mock_route_matches_total{command="mock",kind="builtin"} 1`,
		`radix_mock_route_matches_total{command="mock",kind="custom"} 2`,
		`radix_mock_template_renders_total{command="mock"} 1`,
		`radix_mock_template_errors_total{command="mock"} 1`,
		`radix_mock_reloads_total{command="mock"} 1`,
		`radix_mock_fail_injections_total{command="mock"} 1`,
		`radix_mock_fallback_total{command="mock",type="not_found"} 2`,
		`radix_mock_fallback_total{command="mock",type="proxy"} 1`,
	} {
		if !contains(out, want) {
			t.Errorf("Prometheus output missing %q\n%s", want, out)
		}
	}
}

// TestCommandMetricsProxy verifies the proxy per-command counters increment and
// surface in both the JSON snapshot and the Prometheus output.
func TestCommandMetricsProxy(t *testing.T) {
	c := NewCollector("proxy", "1.0.0")

	c.RecordProxyAuthInjection()
	c.RecordProxyAuthInjection()
	c.RecordProxyStream()

	snap := c.Snapshot()
	p := snap.Command.Proxy
	if p == nil {
		t.Fatal("snapshot Command.Proxy is nil for proxy command")
	}
	if p.AuthInjections != 2 {
		t.Errorf("AuthInjections = %d, want 2", p.AuthInjections)
	}
	if p.StreamConnections != 1 {
		t.Errorf("StreamConnections = %d, want 1", p.StreamConnections)
	}

	out := prometheusOutput(t, c)
	for _, want := range []string{
		`radix_proxy_auth_injections_total{command="proxy"} 2`,
		`radix_proxy_stream_connections_total{command="proxy"} 1`,
	} {
		if !contains(out, want) {
			t.Errorf("Prometheus output missing %q\n%s", want, out)
		}
	}
}

// TestCommandMetricsJSONRoundTrip confirms the command section serializes with
// the documented JSON keys and round-trips for the active command only.
func TestCommandMetricsJSONRoundTrip(t *testing.T) {
	c := NewCollector("mock", "1.0.0")
	c.RecordMockRouteMatch(true)

	req := httptest.NewRequest("GET", "/_metrics", nil)
	rec := httptest.NewRecorder()
	c.Handler("json")(rec, req)

	// Decode into a generic map to assert the on-the-wire key shape.
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(rec.Body.Bytes(), &raw); err != nil {
		t.Fatalf("failed to parse JSON: %v", err)
	}
	cmdRaw, ok := raw["command"]
	if !ok {
		t.Fatal("JSON snapshot missing top-level \"command\" object")
	}
	var cmd map[string]json.RawMessage
	if err := json.Unmarshal(cmdRaw, &cmd); err != nil {
		t.Fatalf("failed to parse command object: %v", err)
	}
	if _, ok := cmd["mock"]; !ok {
		t.Error("command object missing \"mock\" section")
	}
	if _, ok := cmd["echo"]; ok {
		t.Error("command object should omit \"echo\" section for mock command")
	}
	if _, ok := cmd["proxy"]; ok {
		t.Error("command object should omit \"proxy\" section for mock command")
	}
}

// TestCommandMetricsOmittedForServe confirms a command without per-command
// counters (e.g. serve) omits the "command" object entirely.
func TestCommandMetricsOmittedForServe(t *testing.T) {
	c := NewCollector("serve", "1.0.0")

	snap := c.Snapshot()
	if snap.Command != nil {
		t.Error("Command section should be nil for the serve command")
	}

	req := httptest.NewRequest("GET", "/_metrics", nil)
	rec := httptest.NewRecorder()
	c.Handler("json")(rec, req)

	var raw map[string]json.RawMessage
	if err := json.Unmarshal(rec.Body.Bytes(), &raw); err != nil {
		t.Fatalf("failed to parse JSON: %v", err)
	}
	if _, ok := raw["command"]; ok {
		t.Error("serve snapshot should omit the \"command\" object")
	}
}

// TestCommandMetricsReset confirms Reset clears the per-command counters.
func TestCommandMetricsReset(t *testing.T) {
	c := NewCollector("mock", "1.0.0")
	c.RecordMockRouteMatch(true)
	c.RecordMockTemplateRender()
	c.RecordMockReload()

	c.Reset()

	m := c.Snapshot().Command.Mock
	if m == nil {
		t.Fatal("Command.Mock is nil after reset")
	}
	if m.RouteMatchesCustom != 0 || m.TemplateRenders != 0 || m.Reloads != 0 {
		t.Errorf("per-command counters not cleared after reset: %+v", m)
	}
}

// TestNilCollectorRecordingIsNoOp confirms every Record* method is safe to call
// on a nil *Collector (the disabled-metrics case) and never panics.
func TestNilCollectorRecordingIsNoOp(_ *testing.T) {
	var c *Collector // nil, as when metrics are disabled

	// None of these may panic.
	c.RecordEchoDelay()
	c.RecordEchoCustomBody()
	c.RecordEchoPathStatus()
	c.RecordMockRouteMatch(true)
	c.RecordMockRouteMatch(false)
	c.RecordMockTemplateRender()
	c.RecordMockTemplateError()
	c.RecordMockReload()
	c.RecordMockFailInjection()
	c.RecordMockFallback("not_found")
	c.RecordMockFallback("proxy")
	c.RecordProxyAuthInjection()
	c.RecordProxyStream()

	// RecordRequest is also exercised concurrently with the per-command counters
	// elsewhere; the no-op guarantee is the focus here.
}

// TestCommandMetricsConcurrency exercises the per-command counters under
// concurrent writers to confirm they are race-free (run with -race).
func TestCommandMetricsConcurrency(t *testing.T) {
	c := NewCollector("mock", "1.0.0")

	const goroutines = 10
	const iterations = 100
	done := make(chan struct{})
	for i := 0; i < goroutines; i++ {
		go func() {
			for j := 0; j < iterations; j++ {
				c.RecordMockRouteMatch(true)
				c.RecordMockTemplateRender()
			}
			done <- struct{}{}
		}()
	}
	for i := 0; i < goroutines; i++ {
		<-done
	}

	m := c.Snapshot().Command.Mock
	if m == nil {
		t.Fatal("Command.Mock is nil")
	}
	want := uint64(goroutines * iterations)
	if m.RouteMatchesCustom != want {
		t.Errorf("RouteMatchesCustom = %d, want %d", m.RouteMatchesCustom, want)
	}
	if m.TemplateRenders != want {
		t.Errorf("TemplateRenders = %d, want %d", m.TemplateRenders, want)
	}
}
