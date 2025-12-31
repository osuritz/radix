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
		oldVal := val.(uint64)
		if m.CompareAndSwap(key, oldVal, oldVal+1) {
			return
		}
		// If CAS failed, retry
	}
}

// Metrics represents the complete metrics snapshot
type Metrics struct {
	Server        ServerMetrics     `json:"server"`
	Requests      RequestMetrics    `json:"requests"`
	StatusCodes   map[string]uint64 `json:"status_codes"`
	Methods       map[string]uint64 `json:"methods"`
	ResponseTimes HistogramSnapshot `json:"response_times"`
	Bandwidth     BandwidthMetrics  `json:"bandwidth"`
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
		statusCodes[http.StatusText(key.(int))] = value.(uint64)
		return true
	})

	// Convert methods map
	methods := make(map[string]uint64)
	c.methods.Range(func(key, value interface{}) bool {
		methods[key.(string)] = value.(uint64)
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
	c.startTime = time.Now()
}
