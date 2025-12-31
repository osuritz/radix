package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestResponseWriter(t *testing.T) {
	rec := httptest.NewRecorder()
	rw := &responseWriter{ResponseWriter: rec}

	// Test initial state
	if rw.status != 0 {
		t.Errorf("Initial status should be 0, got %d", rw.status)
	}

	if rw.size != 0 {
		t.Errorf("Initial size should be 0, got %d", rw.size)
	}

	// Test WriteHeader
	rw.WriteHeader(http.StatusOK)
	if rw.status != http.StatusOK {
		t.Errorf("Status should be 200, got %d", rw.status)
	}

	// Test Write
	data := []byte("Hello, World!")
	n, err := rw.Write(data)
	if err != nil {
		t.Errorf("Write error: %v", err)
	}

	if n != len(data) {
		t.Errorf("Write size mismatch: got %d, want %d", n, len(data))
	}

	if rw.size != len(data) {
		t.Errorf("Response size should be %d, got %d", len(data), rw.size)
	}
}

func TestResponseWriterDefaultStatus(t *testing.T) {
	rec := httptest.NewRecorder()
	rw := &responseWriter{ResponseWriter: rec}

	// Write without calling WriteHeader
	rw.Write([]byte("test"))

	// Status should default to 200
	if rw.status != http.StatusOK {
		t.Errorf("Default status should be 200, got %d", rw.status)
	}
}

func TestLoggingMiddleware(t *testing.T) {
	tests := []struct {
		name       string
		format     LogFormat
		noColor    bool
		method     string
		path       string
		wantStatus int
	}{
		{
			name:       "CLF format GET",
			format:     LogFormatCLF,
			method:     "GET",
			path:       "/index.html",
			wantStatus: http.StatusOK,
		},
		{
			name:       "Extended CLF format POST",
			format:     LogFormatExtendedCLF,
			method:     "POST",
			path:       "/api/users",
			wantStatus: http.StatusCreated,
		},
		{
			name:       "Dev format with color",
			format:     LogFormatDev,
			noColor:    false,
			method:     "GET",
			path:       "/test",
			wantStatus: http.StatusOK,
		},
		{
			name:       "Dev format without color",
			format:     LogFormatDev,
			noColor:    true,
			method:     "DELETE",
			path:       "/users/123",
			wantStatus: http.StatusNoContent,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create test handler
			handler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(tt.wantStatus)
				w.Write([]byte("test response"))
			})

			// Wrap with logging middleware
			config := LoggingConfig{
				Format:  tt.format,
				NoColor: tt.noColor,
			}
			wrapped := Logging(config)(handler)

			// Create test request
			req := httptest.NewRequest(tt.method, tt.path, nil)
			req.Header.Set("User-Agent", "test-agent")
			req.Header.Set("Referer", "http://example.com")

			// Execute request
			rec := httptest.NewRecorder()
			wrapped.ServeHTTP(rec, req)

			// Verify response
			if rec.Code != tt.wantStatus {
				t.Errorf("Status code mismatch: got %d, want %d", rec.Code, tt.wantStatus)
			}
		})
	}
}

func TestFormatDuration(t *testing.T) {
	tests := []struct {
		name     string
		duration time.Duration
		want     string
	}{
		{"nanoseconds", 500 * time.Nanosecond, "500ns"},
		{"microseconds", 250 * time.Microsecond, "250µs"},
		{"milliseconds", 45 * time.Millisecond, "45ms"},
		{"seconds", 2 * time.Second, "2.00s"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := formatDuration(tt.duration)
			if got != tt.want {
				t.Errorf("formatDuration(%v) = %s, want %s", tt.duration, got, tt.want)
			}
		})
	}
}

func TestFormatSize(t *testing.T) {
	tests := []struct {
		name string
		size int
		want string
	}{
		{"zero", 0, "-"},
		{"bytes", 100, "100B"},
		{"kilobytes", 1024, "1.0KB"},
		{"megabytes", 1024 * 1024, "1.0MB"},
		{"gigabytes", 1024 * 1024 * 1024, "1.0GB"},
		{"mixed", 2500, "2.4KB"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := formatSize(tt.size)
			if got != tt.want {
				t.Errorf("formatSize(%d) = %s, want %s", tt.size, got, tt.want)
			}
		})
	}
}

func TestGetMethodColor(t *testing.T) {
	tests := []struct {
		method string
		want   string
	}{
		{"GET", "\033[36m"},    // Cyan
		{"POST", "\033[32m"},   // Green
		{"PUT", "\033[33m"},    // Yellow
		{"DELETE", "\033[31m"}, // Red
		{"PATCH", "\033[35m"},  // Magenta
		{"HEAD", "\033[37m"},   // White (default)
	}

	for _, tt := range tests {
		t.Run(tt.method, func(t *testing.T) {
			got := getMethodColor(tt.method)
			if got != tt.want {
				t.Errorf("getMethodColor(%s) = %s, want %s", tt.method, got, tt.want)
			}
		})
	}
}

func TestGetStatusColor(t *testing.T) {
	tests := []struct {
		status int
		want   string
	}{
		{200, "\033[32m"}, // Green (success)
		{299, "\033[32m"}, // Green (success)
		{301, "\033[36m"}, // Cyan (redirect)
		{404, "\033[33m"}, // Yellow (client error)
		{500, "\033[31m"}, // Red (server error)
		{100, "\033[37m"}, // White (default)
	}

	for _, tt := range tests {
		t.Run(string(rune(tt.status)), func(t *testing.T) {
			got := getStatusColor(tt.status)
			if got != tt.want {
				t.Errorf("getStatusColor(%d) = %s, want %s", tt.status, got, tt.want)
			}
		})
	}
}
