package middleware

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
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

			// Wrap with logging middleware (route output to a buffer so the
			// test does not write to stdout).
			var buf bytes.Buffer
			config := LoggingConfig{
				Format:  tt.format,
				NoColor: tt.noColor,
				Output:  &buf,
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

// fixedNow is a deterministic timestamp for golden dev-line assertions.
// 14:23:01 in the dev format.
var fixedNow = time.Date(2026, 6, 14, 14, 23, 1, 0, time.UTC)

func TestFormatDevLine_NoColorGolden(t *testing.T) {
	got := formatDevLine(fixedNow, "GET", "/index.html", 200, 2358, 12*time.Millisecond, false)
	// 14:23:01 | "GET    " (pad 7) | "/index.html" padded to 28 | 200 | 12ms | 2.3KB
	want := "14:23:01 GET     /index.html                  200 12ms 2.3KB\n"
	if got != want {
		t.Errorf("formatDevLine no-color mismatch:\n got: %q\nwant: %q", got, want)
	}
}

func TestFormatDevLine_NoColorOmitsZeroSize(t *testing.T) {
	got := formatDevLine(fixedNow, "DELETE", "/users/123", 204, 0, 5*time.Millisecond, false)
	// size == 0 -> no trailing size column (and no "-").
	want := "14:23:01 DELETE  /users/123                   204 5ms\n"
	if got != want {
		t.Errorf("formatDevLine zero-size mismatch:\n got: %q\nwant: %q", got, want)
	}
	if strings.HasSuffix(strings.TrimRight(got, "\n"), "-") {
		t.Errorf("zero size must not emit a trailing '-': %q", got)
	}
}

func TestFormatDevLine_ColoredGolden(t *testing.T) {
	got := formatDevLine(fixedNow, "GET", "/index.html", 200, 2358, 12*time.Millisecond, true)
	want := ansiDim + "14:23:01" + ansiReset + " " +
		getMethodColor("GET") + "GET    " + ansiReset + " " +
		"/index.html                  " +
		getStatusColor(200) + "200" + ansiReset + " " +
		"12ms 2.3KB\n"
	if got != want {
		t.Errorf("formatDevLine colored mismatch:\n got: %q\nwant: %q", got, want)
	}
}

func TestFormatDevLine_LongPathTruncated(t *testing.T) {
	longPath := "/api/v1/some/extremely/long/resource/path/that/overflows"
	got := formatDevLine(fixedNow, "GET", longPath, 200, 0, time.Millisecond, false)
	// Path column must be exactly devPathWidth runes and end with the ellipsis.
	// Work in runes since the ellipsis is multibyte; the prefix before the path
	// is "HH:MM:SS " (9) + method padded to devMethodWidth + " " (1), all ASCII.
	runes := []rune(got)
	start := 9 + devMethodWidth + 1
	pathCol := runes[start : start+devPathWidth]
	if !strings.HasSuffix(string(pathCol), "…") {
		t.Errorf("truncated path column should end with ellipsis, got %q", string(pathCol))
	}
	if len(pathCol) != devPathWidth {
		t.Errorf("path column width = %d runes, want %d (%q)", len(pathCol), devPathWidth, string(pathCol))
	}
}

func TestFormatDevLine_ColumnAlignment(t *testing.T) {
	// Paths must start at the same byte offset regardless of method length,
	// and the status column must start at the same offset for paths within the
	// fixed path width.
	get := formatDevLine(fixedNow, "GET", "/a", 200, 0, time.Millisecond, false)
	del := formatDevLine(fixedNow, "DELETE", "/a", 200, 0, time.Millisecond, false)

	if got, want := strings.Index(get, "/a"), strings.Index(del, "/a"); got != want {
		t.Errorf("path column misaligned across methods: GET=%d DELETE=%d", got, want)
	}
	if got, want := strings.Index(get, "200"), strings.Index(del, "200"); got != want {
		t.Errorf("status column misaligned across methods: GET=%d DELETE=%d", got, want)
	}
}

func TestResolveColor_Precedence(t *testing.T) {
	nonTTY := &bytes.Buffer{}

	// A real character device obtained from os.DevNull. isTerminal type-asserts
	// to *os.File, so the TTY branch cannot be exercised with a fake writer.
	// Subtests that need a positive TTY skip when no char device is available
	// (e.g. some Windows configurations).
	tty := openCharDevice(t)
	requireTTY := func(t *testing.T) {
		t.Helper()
		if tty == nil {
			t.Skip("no character device available to simulate a TTY")
		}
	}

	t.Run("noColor wins over everything", func(t *testing.T) {
		t.Setenv("NO_COLOR", "")
		t.Setenv("FORCE_COLOR", "1")
		if resolveColor(true, nonTTY) {
			t.Error("noColor=true must disable color")
		}
	})

	t.Run("noColor wins even on a TTY", func(t *testing.T) {
		requireTTY(t)
		t.Setenv("NO_COLOR", "")
		t.Setenv("FORCE_COLOR", "")
		t.Setenv("CLICOLOR_FORCE", "")
		if resolveColor(true, tty) {
			t.Error("noColor=true must disable color even on a TTY")
		}
	})

	t.Run("NO_COLOR disables color even on a TTY", func(t *testing.T) {
		requireTTY(t)
		t.Setenv("FORCE_COLOR", "")
		t.Setenv("CLICOLOR_FORCE", "")
		t.Setenv("NO_COLOR", "1")
		if resolveColor(false, tty) {
			t.Error("NO_COLOR set must disable color")
		}
	})

	t.Run("NO_COLOR beats FORCE_COLOR", func(t *testing.T) {
		t.Setenv("NO_COLOR", "1")
		t.Setenv("FORCE_COLOR", "1")
		if resolveColor(false, nonTTY) {
			t.Error("NO_COLOR must take precedence over FORCE_COLOR")
		}
	})

	t.Run("FORCE_COLOR forces color on a non-TTY", func(t *testing.T) {
		t.Setenv("NO_COLOR", "")
		t.Setenv("CLICOLOR_FORCE", "")
		t.Setenv("FORCE_COLOR", "1")
		if !resolveColor(false, nonTTY) {
			t.Error("FORCE_COLOR must enable color on a non-TTY")
		}
	})

	t.Run("CLICOLOR_FORCE forces color on a non-TTY", func(t *testing.T) {
		t.Setenv("NO_COLOR", "")
		t.Setenv("FORCE_COLOR", "")
		t.Setenv("CLICOLOR_FORCE", "1")
		if !resolveColor(false, nonTTY) {
			t.Error("CLICOLOR_FORCE must enable color on a non-TTY")
		}
	})

	t.Run("non-TTY writer disables color", func(t *testing.T) {
		t.Setenv("NO_COLOR", "")
		t.Setenv("FORCE_COLOR", "")
		t.Setenv("CLICOLOR_FORCE", "")
		if resolveColor(false, nonTTY) {
			t.Error("non-TTY writer must disable color")
		}
	})

	t.Run("TTY writer enables color", func(t *testing.T) {
		requireTTY(t)
		t.Setenv("NO_COLOR", "")
		t.Setenv("FORCE_COLOR", "")
		t.Setenv("CLICOLOR_FORCE", "")
		if !resolveColor(false, tty) {
			t.Error("TTY writer must enable color")
		}
	})
}

func TestResolveColor_BytesBufferNonTTY(t *testing.T) {
	t.Setenv("NO_COLOR", "")
	t.Setenv("FORCE_COLOR", "")
	t.Setenv("CLICOLOR_FORCE", "")
	if isTerminal(&bytes.Buffer{}) {
		t.Error("bytes.Buffer must not be detected as a terminal")
	}
}

func TestLogging_WritesToInjectedOutput(t *testing.T) {
	t.Setenv("NO_COLOR", "")
	t.Setenv("FORCE_COLOR", "")
	t.Setenv("CLICOLOR_FORCE", "")

	var buf bytes.Buffer
	handler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("hello"))
	})
	wrapped := Logging(LoggingConfig{Format: LogFormatDev, Output: &buf})(handler)

	req := httptest.NewRequest("GET", "/ping", nil)
	wrapped.ServeHTTP(httptest.NewRecorder(), req)

	out := buf.String()
	if out == "" {
		t.Fatal("expected dev log output on injected writer, got empty")
	}
	// Output to a bytes.Buffer is a non-TTY -> color auto-disabled.
	if strings.Contains(out, "\033[") {
		t.Errorf("expected no ANSI codes for non-TTY output, got %q", out)
	}
	if !strings.Contains(out, "GET") || !strings.Contains(out, "/ping") || !strings.Contains(out, "200") {
		t.Errorf("dev line missing expected fields: %q", out)
	}
	if !strings.HasSuffix(out, "\n") {
		t.Errorf("dev line must be newline-terminated: %q", out)
	}
}

func TestFormatCLF_ByteIdentical(t *testing.T) {
	req := httptest.NewRequest("GET", "/index.html", nil)
	req.RemoteAddr = "127.0.0.1:54321"
	req.Proto = "HTTP/1.0"

	got := formatCLF(req, 200, 2326)

	// Reconstruct the exact historical format string independently.
	host := "127.0.0.1"
	ts := time.Now().Format("02/Jan/2006:15:04:05 -0700")
	want := host + " - - [" + ts + "] \"GET /index.html HTTP/1.0\" 200 2326\n"
	if got != want {
		t.Errorf("CLF output drifted:\n got: %q\nwant: %q", got, want)
	}
}

func TestFormatExtendedCLF_ByteIdentical(t *testing.T) {
	req := httptest.NewRequest("POST", "/api/users", nil)
	req.RemoteAddr = "10.0.0.5:1234"
	req.Proto = "HTTP/1.1"
	req.Header.Set("Referer", "http://example.com")
	req.Header.Set("User-Agent", "Mozilla/5.0")

	got := formatExtendedCLF(req, 201, 512)

	host := "10.0.0.5"
	ts := time.Now().Format("02/Jan/2006:15:04:05 -0700")
	want := host + " - - [" + ts + "] \"POST /api/users HTTP/1.1\" 201 512 \"http://example.com\" \"Mozilla/5.0\"\n"
	if got != want {
		t.Errorf("Extended CLF output drifted:\n got: %q\nwant: %q", got, want)
	}
}

func TestFormatExtendedCLF_DefaultsForMissingHeaders(t *testing.T) {
	req := httptest.NewRequest("GET", "/", nil)
	req.RemoteAddr = "1.2.3.4:9999"
	req.Proto = "HTTP/1.1"
	// No Referer / User-Agent -> both default to "-".

	got := formatExtendedCLF(req, 200, 0)
	if !strings.Contains(got, `"-" "-"`) {
		t.Errorf("missing headers should default to \"-\": %q", got)
	}
}

// openCharDevice returns an *os.File backed by os.DevNull when that file is a
// character device on this platform (true on Unix; varies on Windows), so the
// TTY-positive branch of resolveColor can be exercised deterministically. It
// returns nil when no character device is available, letting callers t.Skip.
// The file is closed automatically at the end of the test.
func openCharDevice(t *testing.T) *os.File {
	t.Helper()
	f, err := os.OpenFile(os.DevNull, os.O_WRONLY, 0)
	if err != nil {
		return nil
	}
	t.Cleanup(func() { _ = f.Close() })
	fi, err := f.Stat()
	if err != nil || (fi.Mode()&os.ModeCharDevice) == 0 {
		return nil
	}
	return f
}
