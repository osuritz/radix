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
	_, _ = fmt.Fprintf(w, "# HELP radix_server_info Server information\n")
	_, _ = fmt.Fprintf(w, "# TYPE radix_server_info gauge\n")
	_, _ = fmt.Fprintf(w, "radix_server_info{command=\"%s\",version=\"%s\"} 1\n",
		e.command, metrics.Server.Version)
	_, _ = fmt.Fprintln(w)

	// Uptime
	_, _ = fmt.Fprintf(w, "# HELP radix_server_uptime_seconds Server uptime in seconds\n")
	_, _ = fmt.Fprintf(w, "# TYPE radix_server_uptime_seconds counter\n")
	_, _ = fmt.Fprintf(w, "radix_server_uptime_seconds{command=\"%s\"} %.2f\n",
		e.command, metrics.Server.UptimeSeconds)
	_, _ = fmt.Fprintln(w)

	// Total requests
	_, _ = fmt.Fprintf(w, "# HELP radix_requests_total Total number of HTTP requests\n")
	_, _ = fmt.Fprintf(w, "# TYPE radix_requests_total counter\n")
	_, _ = fmt.Fprintf(w, "radix_requests_total{command=\"%s\"} %d\n",
		e.command, metrics.Requests.Total)
	_, _ = fmt.Fprintln(w)

	// Requests by status code
	_, _ = fmt.Fprintf(w, "# HELP radix_requests_by_status_total Total requests by HTTP status code\n")
	_, _ = fmt.Fprintf(w, "# TYPE radix_requests_by_status_total counter\n")
	e.writeStatusCodeMetrics(w, metrics.StatusCodes)
	_, _ = fmt.Fprintln(w)

	// Requests by method
	_, _ = fmt.Fprintf(w, "# HELP radix_requests_by_method_total Total requests by HTTP method\n")
	_, _ = fmt.Fprintf(w, "# TYPE radix_requests_by_method_total counter\n")
	e.writeMethodMetrics(w, metrics.Methods)
	_, _ = fmt.Fprintln(w)

	// Response time histogram
	_, _ = fmt.Fprintf(w, "# HELP radix_response_time_milliseconds HTTP request duration in milliseconds\n")
	_, _ = fmt.Fprintf(w, "# TYPE radix_response_time_milliseconds summary\n")
	e.writeResponseTimeMetrics(w, metrics.ResponseTimes)
	_, _ = fmt.Fprintln(w)

	// Bandwidth
	_, _ = fmt.Fprintf(w, "# HELP radix_bytes_sent_total Total bytes sent\n")
	_, _ = fmt.Fprintf(w, "# TYPE radix_bytes_sent_total counter\n")
	_, _ = fmt.Fprintf(w, "radix_bytes_sent_total{command=\"%s\"} %d\n",
		e.command, metrics.Bandwidth.BytesSent)
	_, _ = fmt.Fprintln(w)

	_, _ = fmt.Fprintf(w, "# HELP radix_bytes_received_total Total bytes received\n")
	_, _ = fmt.Fprintf(w, "# TYPE radix_bytes_received_total counter\n")
	_, _ = fmt.Fprintf(w, "radix_bytes_received_total{command=\"%s\"} %d\n",
		e.command, metrics.Bandwidth.BytesReceived)
	_, _ = fmt.Fprintln(w)

	// Request rate
	_, _ = fmt.Fprintf(w, "# HELP radix_request_rate_per_second Current request rate per second\n")
	_, _ = fmt.Fprintf(w, "# TYPE radix_request_rate_per_second gauge\n")
	_, _ = fmt.Fprintf(w, "radix_request_rate_per_second{command=\"%s\"} %.4f\n",
		e.command, metrics.Requests.RatePerSecond)
}

// writeComment writes a comment line
func (e *PrometheusExporter) writeComment(w io.Writer, comment string) {
	_, _ = fmt.Fprintf(w, "# %s\n", comment)
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
		_, _ = fmt.Fprintf(w, "radix_requests_by_status_total{command=\"%s\",status=\"%s\"} %d\n",
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
		_, _ = fmt.Fprintf(w, "radix_requests_by_method_total{command=\"%s\",method=\"%s\"} %d\n",
			e.command, method, count)
	}
}

// writeResponseTimeMetrics writes response time histogram as Prometheus summary
func (e *PrometheusExporter) writeResponseTimeMetrics(w io.Writer, hist HistogramSnapshot) {
	if hist.Count == 0 {
		return
	}

	// Summary format with quantiles
	_, _ = fmt.Fprintf(w, "radix_response_time_milliseconds{command=\"%s\",quantile=\"0.5\"} %.2f\n",
		e.command, hist.P50)
	_, _ = fmt.Fprintf(w, "radix_response_time_milliseconds{command=\"%s\",quantile=\"0.95\"} %.2f\n",
		e.command, hist.P95)
	_, _ = fmt.Fprintf(w, "radix_response_time_milliseconds{command=\"%s\",quantile=\"0.99\"} %.2f\n",
		e.command, hist.P99)

	// Summary sum and count
	sum := hist.Avg * float64(hist.Count)
	_, _ = fmt.Fprintf(w, "radix_response_time_milliseconds_sum{command=\"%s\"} %.2f\n",
		e.command, sum)
	_, _ = fmt.Fprintf(w, "radix_response_time_milliseconds_count{command=\"%s\"} %d\n",
		e.command, hist.Count)
}
