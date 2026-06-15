package metrics

import (
	"fmt"
	"io"
	"sort"
)

// PrometheusExporter exports metrics in Prometheus text format
type PrometheusExporter struct {
	command string
}

// NewPrometheusExporter creates a new Prometheus exporter
func NewPrometheusExporter(command string) *PrometheusExporter {
	return &PrometheusExporter{
		command: command,
	}
}

// Export writes metrics in Prometheus text format
func (e *PrometheusExporter) Export(w io.Writer, metrics *Metrics) {
	// Server info (as labels on other metrics)
	e.writeComment(w, "Server information")
	fmt.Fprintf(w, "# HELP radix_server_info Server information\n")
	fmt.Fprintf(w, "# TYPE radix_server_info gauge\n")
	fmt.Fprintf(w, "radix_server_info{command=\"%s\",version=\"%s\"} 1\n",
		e.command, metrics.Server.Version)
	fmt.Fprintln(w)

	// Uptime
	fmt.Fprintf(w, "# HELP radix_server_uptime_seconds Server uptime in seconds\n")
	fmt.Fprintf(w, "# TYPE radix_server_uptime_seconds counter\n")
	fmt.Fprintf(w, "radix_server_uptime_seconds{command=\"%s\"} %.2f\n",
		e.command, metrics.Server.UptimeSeconds)
	fmt.Fprintln(w)

	// Total requests
	fmt.Fprintf(w, "# HELP radix_requests_total Total number of HTTP requests\n")
	fmt.Fprintf(w, "# TYPE radix_requests_total counter\n")
	fmt.Fprintf(w, "radix_requests_total{command=\"%s\"} %d\n",
		e.command, metrics.Requests.Total)
	fmt.Fprintln(w)

	// Requests by status code
	fmt.Fprintf(w, "# HELP radix_requests_by_status_total Total requests by HTTP status code\n")
	fmt.Fprintf(w, "# TYPE radix_requests_by_status_total counter\n")
	e.writeStatusCodeMetrics(w, metrics.StatusCodes)
	fmt.Fprintln(w)

	// Requests by method
	fmt.Fprintf(w, "# HELP radix_requests_by_method_total Total requests by HTTP method\n")
	fmt.Fprintf(w, "# TYPE radix_requests_by_method_total counter\n")
	e.writeMethodMetrics(w, metrics.Methods)
	fmt.Fprintln(w)

	// Response time histogram
	fmt.Fprintf(w, "# HELP radix_response_time_milliseconds HTTP request duration in milliseconds\n")
	fmt.Fprintf(w, "# TYPE radix_response_time_milliseconds summary\n")
	e.writeResponseTimeMetrics(w, metrics.ResponseTimes)
	fmt.Fprintln(w)

	// Bandwidth
	fmt.Fprintf(w, "# HELP radix_bytes_sent_total Total bytes sent\n")
	fmt.Fprintf(w, "# TYPE radix_bytes_sent_total counter\n")
	fmt.Fprintf(w, "radix_bytes_sent_total{command=\"%s\"} %d\n",
		e.command, metrics.Bandwidth.BytesSent)
	fmt.Fprintln(w)

	fmt.Fprintf(w, "# HELP radix_bytes_received_total Total bytes received\n")
	fmt.Fprintf(w, "# TYPE radix_bytes_received_total counter\n")
	fmt.Fprintf(w, "radix_bytes_received_total{command=\"%s\"} %d\n",
		e.command, metrics.Bandwidth.BytesReceived)
	fmt.Fprintln(w)

	// Request rate
	fmt.Fprintf(w, "# HELP radix_request_rate_per_second Current request rate per second\n")
	fmt.Fprintf(w, "# TYPE radix_request_rate_per_second gauge\n")
	fmt.Fprintf(w, "radix_request_rate_per_second{command=\"%s\"} %.4f\n",
		e.command, metrics.Requests.RatePerSecond)

	// Per-command counters. Only the active command's section is non-nil, so a
	// proxy process exports only the proxy_* families, a mock process only the
	// mock_* families, etc. Commands with no per-command counters (e.g. serve)
	// have a nil section and emit nothing here.
	e.writeCommandMetrics(w, metrics.Command)
}

// writeCommandMetrics writes the per-command counter families for whichever
// command is active. Each family carries the command="..." label for parity with
// the generic radix_* metrics. A nil section emits nothing.
func (e *PrometheusExporter) writeCommandMetrics(w io.Writer, cmd *CommandMetrics) {
	if cmd == nil {
		return
	}
	switch {
	case cmd.Echo != nil:
		e.writeEchoMetrics(w, cmd.Echo)
	case cmd.Mock != nil:
		e.writeMockMetrics(w, cmd.Mock)
	case cmd.Proxy != nil:
		e.writeProxyMetrics(w, cmd.Proxy)
	}
}

// writeEchoMetrics writes the echo command's per-command counter families.
func (e *PrometheusExporter) writeEchoMetrics(w io.Writer, m *EchoMetrics) {
	fmt.Fprintln(w)
	e.counter(w, "radix_echo_delays_total", "Echo responses that applied a delay", m.DelaysApplied)
	e.counter(w, "radix_echo_custom_body_total", "Echo responses served from the configured literal body", m.CustomBodyResponse)
	e.counter(w, "radix_echo_path_status_total", "Echo responses whose status was derived from the request path", m.PathStatusHits)
}

// writeMockMetrics writes the mock command's per-command counter families.
func (e *PrometheusExporter) writeMockMetrics(w io.Writer, m *MockMetrics) {
	fmt.Fprintln(w)
	fmt.Fprintf(w, "# HELP radix_mock_route_matches_total Mock route matches by kind\n")
	fmt.Fprintf(w, "# TYPE radix_mock_route_matches_total counter\n")
	fmt.Fprintf(w, "radix_mock_route_matches_total{command=\"%s\",kind=\"builtin\"} %d\n", e.command, m.RouteMatchesBuiltin)
	fmt.Fprintf(w, "radix_mock_route_matches_total{command=\"%s\",kind=\"custom\"} %d\n", e.command, m.RouteMatchesCustom)

	e.counter(w, "radix_mock_template_renders_total", "Successful mock response template renders", m.TemplateRenders)
	e.counter(w, "radix_mock_template_errors_total", "Failed mock response template renders", m.TemplateErrors)
	e.counter(w, "radix_mock_reloads_total", "Successful mock routes-file hot reloads", m.Reloads)
	e.counter(w, "radix_mock_fail_injections_total", "Mock requests short-circuited by the random fail-rate", m.FailInjections)

	fmt.Fprintf(w, "# HELP radix_mock_fallback_total Unmatched mock requests served by the fallback, by type\n")
	fmt.Fprintf(w, "# TYPE radix_mock_fallback_total counter\n")
	fmt.Fprintf(w, "radix_mock_fallback_total{command=\"%s\",type=\"not_found\"} %d\n", e.command, m.FallbackNotFound)
	fmt.Fprintf(w, "radix_mock_fallback_total{command=\"%s\",type=\"proxy\"} %d\n", e.command, m.FallbackProxy)
}

// writeProxyMetrics writes the proxy command's per-command counter families.
func (e *PrometheusExporter) writeProxyMetrics(w io.Writer, m *ProxyMetrics) {
	fmt.Fprintln(w)
	e.counter(w, "radix_proxy_auth_injections_total", "Proxied requests that had auth headers injected", m.AuthInjections)
	e.counter(w, "radix_proxy_stream_connections_total", "Proxied responses detected as streaming (SSE/ndjson) connections", m.StreamConnections)
}

// counter writes a single command-labeled counter family (HELP, TYPE, and the
// sample line) for the per-command metrics.
func (e *PrometheusExporter) counter(w io.Writer, name, help string, value uint64) {
	fmt.Fprintf(w, "# HELP %s %s\n", name, help)
	fmt.Fprintf(w, "# TYPE %s counter\n", name)
	fmt.Fprintf(w, "%s{command=\"%s\"} %d\n", name, e.command, value)
}

// writeComment writes a comment line
func (e *PrometheusExporter) writeComment(w io.Writer, comment string) {
	fmt.Fprintf(w, "# %s\n", comment)
}

// writeStatusCodeMetrics writes status code metrics
func (e *PrometheusExporter) writeStatusCodeMetrics(w io.Writer, statusCodes map[string]uint64) {
	// Sort status codes for consistent output
	codes := make([]string, 0, len(statusCodes))
	for code := range statusCodes {
		codes = append(codes, code)
	}
	sort.Strings(codes)

	for _, statusText := range codes {
		count := statusCodes[statusText]
		fmt.Fprintf(w, "radix_requests_by_status_total{command=\"%s\",status=\"%s\"} %d\n",
			e.command, statusText, count)
	}
}

// writeMethodMetrics writes HTTP method metrics
func (e *PrometheusExporter) writeMethodMetrics(w io.Writer, methods map[string]uint64) {
	// Sort methods for consistent output
	methodNames := make([]string, 0, len(methods))
	for method := range methods {
		methodNames = append(methodNames, method)
	}
	sort.Strings(methodNames)

	for _, method := range methodNames {
		count := methods[method]
		fmt.Fprintf(w, "radix_requests_by_method_total{command=\"%s\",method=\"%s\"} %d\n",
			e.command, method, count)
	}
}

// writeResponseTimeMetrics writes response time histogram as Prometheus summary
func (e *PrometheusExporter) writeResponseTimeMetrics(w io.Writer, hist HistogramSnapshot) {
	if hist.Count == 0 {
		return
	}

	// Summary format with quantiles
	fmt.Fprintf(w, "radix_response_time_milliseconds{command=\"%s\",quantile=\"0.5\"} %.2f\n",
		e.command, hist.P50)
	fmt.Fprintf(w, "radix_response_time_milliseconds{command=\"%s\",quantile=\"0.95\"} %.2f\n",
		e.command, hist.P95)
	fmt.Fprintf(w, "radix_response_time_milliseconds{command=\"%s\",quantile=\"0.99\"} %.2f\n",
		e.command, hist.P99)

	// Summary sum and count
	sum := hist.Avg * float64(hist.Count)
	fmt.Fprintf(w, "radix_response_time_milliseconds_sum{command=\"%s\"} %.2f\n",
		e.command, sum)
	fmt.Fprintf(w, "radix_response_time_milliseconds_count{command=\"%s\"} %d\n",
		e.command, hist.Count)
}
