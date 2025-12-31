package middleware

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/osuritz/radix/internal/metrics"
)

func TestMetricsMiddleware(t *testing.T) {
	collector := metrics.NewCollector("test", "1.0.0")

	// Create test handler
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("Hello, World!"))
	})

	// Wrap with metrics middleware
	wrapped := Metrics(collector)(handler)

	// Create test request
	req := httptest.NewRequest("GET", "/test", nil)
	rec := httptest.NewRecorder()

	// Execute request
	wrapped.ServeHTTP(rec, req)

	// Check that metrics were recorded
	snapshot := collector.Snapshot()

	if snapshot.Requests.Total != 1 {
		t.Errorf("total requests = %d, want 1", snapshot.Requests.Total)
	}

	if snapshot.Methods["GET"] != 1 {
		t.Errorf("GET requests = %d, want 1", snapshot.Methods["GET"])
	}

	// Response should be successful
	if rec.Code != http.StatusOK {
		t.Errorf("status code = %d, want %d", rec.Code, http.StatusOK)
	}
}

func TestMetricsMiddlewareStatusCodes(t *testing.T) {
	tests := []struct {
		name       string
		statusCode int
	}{
		{"200 OK", http.StatusOK},
		{"404 Not Found", http.StatusNotFound},
		{"500 Internal Server Error", http.StatusInternalServerError},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			collector := metrics.NewCollector("test", "1.0.0")

			// Create handler that returns the specific status
			handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tt.statusCode)
			})

			// Wrap with metrics middleware
			wrapped := Metrics(collector)(handler)

			// Execute request
			req := httptest.NewRequest("GET", "/test", nil)
			rec := httptest.NewRecorder()
			wrapped.ServeHTTP(rec, req)

			// Verify status code was captured
			if rec.Code != tt.statusCode {
				t.Errorf("status code = %d, want %d", rec.Code, tt.statusCode)
			}

			// Verify metrics were recorded
			snapshot := collector.Snapshot()
			if snapshot.Requests.Total != 1 {
				t.Errorf("total requests = %d, want 1", snapshot.Requests.Total)
			}
		})
	}
}

func TestMetricsMiddlewareMethods(t *testing.T) {
	methods := []string{"GET", "POST", "PUT", "DELETE", "PATCH"}

	for _, method := range methods {
		t.Run(method, func(t *testing.T) {
			collector := metrics.NewCollector("test", "1.0.0")

			handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
			})

			wrapped := Metrics(collector)(handler)

			req := httptest.NewRequest(method, "/test", nil)
			rec := httptest.NewRecorder()
			wrapped.ServeHTTP(rec, req)

			// Verify method was recorded
			snapshot := collector.Snapshot()
			if snapshot.Methods[method] != 1 {
				t.Errorf("%s count = %d, want 1", method, snapshot.Methods[method])
			}
		})
	}
}

func TestMetricsMiddlewareBytesWritten(t *testing.T) {
	collector := metrics.NewCollector("test", "1.0.0")

	testData := "Hello, World! This is a test response."
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(testData))
	})

	wrapped := Metrics(collector)(handler)

	req := httptest.NewRequest("GET", "/test", nil)
	rec := httptest.NewRecorder()
	wrapped.ServeHTTP(rec, req)

	// Verify bytes written were recorded
	snapshot := collector.Snapshot()
	expectedBytes := uint64(len(testData))

	if snapshot.Bandwidth.BytesSent != expectedBytes {
		t.Errorf("bytes sent = %d, want %d", snapshot.Bandwidth.BytesSent, expectedBytes)
	}
}

func TestMetricsMiddlewareBytesReceived(t *testing.T) {
	collector := metrics.NewCollector("test", "1.0.0")

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	wrapped := Metrics(collector)(handler)

	// Create request with body and Content-Length
	body := "test request body"
	req := httptest.NewRequest("POST", "/test", strings.NewReader(body))
	req.ContentLength = int64(len(body))
	rec := httptest.NewRecorder()
	wrapped.ServeHTTP(rec, req)

	// Verify bytes received were recorded
	snapshot := collector.Snapshot()
	expectedBytes := uint64(len(body))

	if snapshot.Bandwidth.BytesReceived != expectedBytes {
		t.Errorf("bytes received = %d, want %d", snapshot.Bandwidth.BytesReceived, expectedBytes)
	}
}

func TestMetricsMiddlewareDefaultStatus(t *testing.T) {
	collector := metrics.NewCollector("test", "1.0.0")

	// Handler that doesn't explicitly call WriteHeader
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("test"))
	})

	wrapped := Metrics(collector)(handler)

	req := httptest.NewRequest("GET", "/test", nil)
	rec := httptest.NewRecorder()
	wrapped.ServeHTTP(rec, req)

	// Should default to 200
	if rec.Code != http.StatusOK {
		t.Errorf("status code = %d, want %d", rec.Code, http.StatusOK)
	}

	// Metrics should record it as success
	snapshot := collector.Snapshot()
	if snapshot.Requests.Success != 1 {
		t.Errorf("success requests = %d, want 1", snapshot.Requests.Success)
	}
}

func TestMetricsMiddlewareResponseTiming(t *testing.T) {
	collector := metrics.NewCollector("test", "1.0.0")

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("test"))
	})

	wrapped := Metrics(collector)(handler)

	req := httptest.NewRequest("GET", "/test", nil)
	rec := httptest.NewRecorder()
	wrapped.ServeHTTP(rec, req)

	// Verify response time was recorded
	snapshot := collector.Snapshot()
	if snapshot.ResponseTimes.Count != 1 {
		t.Errorf("response time count = %d, want 1", snapshot.ResponseTimes.Count)
	}

	// Response time should be >= 0
	if snapshot.ResponseTimes.Min < 0 {
		t.Errorf("min response time = %.2f, should be >= 0", snapshot.ResponseTimes.Min)
	}
}

func TestMetricsMiddlewareMultipleRequests(t *testing.T) {
	collector := metrics.NewCollector("test", "1.0.0")

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("test"))
	})

	wrapped := Metrics(collector)(handler)

	// Make multiple requests
	for i := 0; i < 10; i++ {
		req := httptest.NewRequest("GET", "/test", nil)
		rec := httptest.NewRecorder()
		wrapped.ServeHTTP(rec, req)
	}

	// Verify all requests were recorded
	snapshot := collector.Snapshot()
	if snapshot.Requests.Total != 10 {
		t.Errorf("total requests = %d, want 10", snapshot.Requests.Total)
	}
}
