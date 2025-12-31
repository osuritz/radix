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
