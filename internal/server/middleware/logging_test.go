package middleware

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"os"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"
	"unicode/utf8"
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

func TestPadRight_RuneAwareForMultibyte(t *testing.T) {
	// A short multibyte string must pad to the same visual column width as an
	// ASCII string of equal rune length. Counting bytes (the old behavior) would
	// under-pad the multibyte string and shift the next column left.
	const width = 10
	multibyte := "café" // 4 runes, 5 bytes (é is 2 bytes)
	ascii := "cafe"     // 4 runes, 4 bytes

	gotMB := padRight(multibyte, width)
	gotASCII := padRight(ascii, width)

	if rc := utf8.RuneCountInString(gotMB); rc != width {
		t.Errorf("padRight(%q) rune width = %d, want %d (byte len %d)", multibyte, rc, width, len(gotMB))
	}
	if rc := utf8.RuneCountInString(gotASCII); rc != width {
		t.Errorf("padRight(%q) rune width = %d, want %d", ascii, rc, width)
	}
	// Both must occupy the same number of visual columns (rune count), proving the
	// status column starts at the same offset for either path.
	if utf8.RuneCountInString(gotMB) != utf8.RuneCountInString(gotASCII) {
		t.Errorf("multibyte and ASCII padding misaligned: %q (%d runes) vs %q (%d runes)",
			gotMB, utf8.RuneCountInString(gotMB), gotASCII, utf8.RuneCountInString(gotASCII))
	}
}

func TestFormatDevLine_MultibytePathColumnWidth(t *testing.T) {
	// A short multibyte path must pad the path column to exactly devPathWidth
	// runes, keeping the status column aligned with an ASCII path of equal rune
	// length. With byte-based padding the multibyte path column would be short.
	got := formatDevLine(fixedNow, "GET", "/café", 200, 0, time.Millisecond, false)
	runes := []rune(got)
	start := 9 + devMethodWidth + 1 // "HH:MM:SS " (9) + padded method + " " (1)
	pathCol := runes[start : start+devPathWidth]
	if len(pathCol) != devPathWidth {
		t.Fatalf("path column width = %d runes, want %d (%q)", len(pathCol), devPathWidth, string(pathCol))
	}
	// The status column must immediately follow the single space after the path
	// column, i.e. at a fixed rune offset independent of the path's byte length.
	statusRune := runes[start+devPathWidth+1]
	if statusRune != '2' {
		t.Errorf("status column not at expected rune offset; got %q at offset %d in %q",
			string(statusRune), start+devPathWidth+1, got)
	}
}

func TestFormatDevLine_LongMethodCapped(t *testing.T) {
	// A pathological over-long custom method must be capped to devMethodWidth so
	// it cannot push the path/status columns past their fixed offsets. Compare a
	// normal short method against the long one: the path column must start at the
	// same byte offset for both.
	longMethod := "THISISAVERYLONGCUSTOMMETHOD"
	short := formatDevLine(fixedNow, "GET", "/a", 200, 0, time.Millisecond, false)
	long := formatDevLine(fixedNow, longMethod, "/a", 200, 0, time.Millisecond, false)

	if got, want := strings.Index(long, "/a"), strings.Index(short, "/a"); got != want {
		t.Errorf("long method shifted the path column: long=%d short=%d\n long: %q\nshort: %q",
			got, want, long, short)
	}
	if got, want := strings.Index(long, "200"), strings.Index(short, "200"); got != want {
		t.Errorf("long method shifted the status column: long=%d short=%d", got, want)
	}
	// The emitted method must be truncated to devMethodWidth runes.
	wantMethod := longMethod[:devMethodWidth]
	if !strings.HasPrefix(strings.TrimPrefix(long, "14:23:01 "), wantMethod) {
		t.Errorf("method not capped to %d runes; line: %q", devMethodWidth, long)
	}
}

func TestResolveColor_Precedence(t *testing.T) {
	nonTTY := &bytes.Buffer{}

	// charDev is an *os.File backed by os.DevNull, a real character device. It is
	// a stand-in that exercises the ModeCharDevice branch of isTerminal — it is
	// NOT a real terminal (isTerminal is a char-device heuristic, not a true
	// isatty). isTerminal type-asserts to *os.File, so the heuristic's positive
	// branch cannot be exercised with a fake writer. Subtests that need the
	// char-device branch skip when no char device is available (e.g. some Windows
	// configurations).
	charDev := openCharDevice(t)
	requireCharDevice := func(t *testing.T) {
		t.Helper()
		if charDev == nil {
			t.Skip("no character device available to exercise the TTY heuristic")
		}
	}

	t.Run("noColor wins over everything", func(t *testing.T) {
		t.Setenv("NO_COLOR", "")
		t.Setenv("FORCE_COLOR", "1")
		if resolveColor(true, nonTTY) {
			t.Error("noColor=true must disable color")
		}
	})

	t.Run("noColor wins even on a character device (TTY heuristic)", func(t *testing.T) {
		requireCharDevice(t)
		t.Setenv("NO_COLOR", "")
		t.Setenv("FORCE_COLOR", "")
		t.Setenv("CLICOLOR_FORCE", "")
		if resolveColor(true, charDev) {
			t.Error("noColor=true must disable color even on a char device (TTY heuristic)")
		}
	})

	t.Run("NO_COLOR disables color even on a character device (TTY heuristic)", func(t *testing.T) {
		requireCharDevice(t)
		t.Setenv("FORCE_COLOR", "")
		t.Setenv("CLICOLOR_FORCE", "")
		t.Setenv("NO_COLOR", "1")
		if resolveColor(false, charDev) {
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

	t.Run("character device (TTY heuristic) enables color", func(t *testing.T) {
		// os.DevNull is a character device and is treated as a TTY by the
		// ModeCharDevice heuristic — it is NOT a real terminal. This asserts the
		// heuristic's positive branch: char device + no override -> color on.
		requireCharDevice(t)
		t.Setenv("NO_COLOR", "")
		t.Setenv("FORCE_COLOR", "")
		t.Setenv("CLICOLOR_FORCE", "")
		if !resolveColor(false, charDev) {
			t.Error("character device must enable color under the TTY heuristic")
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

// bracketTimestamp extracts the CLF "[...]" timestamp segment (the only
// wall-clock part of a CLF line) from got. Reusing this exact value when
// constructing want makes the byte-identity assertion deterministic: it cannot
// fail spuriously when the clock ticks between formatCLF's internal time.Now()
// and the test's, because the test no longer calls time.Now() at all.
func bracketTimestamp(t *testing.T, got string) string {
	t.Helper()
	open := strings.IndexByte(got, '[')
	closeIdx := strings.IndexByte(got, ']')
	if open < 0 || closeIdx < 0 || closeIdx < open {
		t.Fatalf("could not locate [timestamp] in CLF output: %q", got)
	}
	return got[open+1 : closeIdx]
}

// devLineShape matches a no-color dev line: "HH:MM:SS METHOD... /path... STATUS
// LATENCY[ SIZE]". It anchors the whole line so a torn/interleaved write (a line
// missing its timestamp prefix, or two lines fused without a newline) fails to
// match. The path token is non-greedy and stops at the status code.
var devLineShape = regexp.MustCompile(`^\d{2}:\d{2}:\d{2} [A-Z]+ +\S.* +\d{3} \S+( \S+)?$`)

func TestLogging_ConcurrentWritesNoInterleave(t *testing.T) {
	t.Setenv("NO_COLOR", "")
	t.Setenv("FORCE_COLOR", "")
	t.Setenv("CLICOLOR_FORCE", "")

	const n = 50

	var buf bytes.Buffer
	handler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})
	// Output to a bytes.Buffer is a non-TTY -> color auto-disabled, so each line
	// is plain text and matchable by devLineShape.
	wrapped := Logging(LoggingConfig{Format: LogFormatDev, Output: &buf})(handler)

	var wg sync.WaitGroup
	wg.Add(n)
	for i := 0; i < n; i++ {
		go func() {
			defer wg.Done()
			req := httptest.NewRequest("GET", "/ping", nil)
			wrapped.ServeHTTP(httptest.NewRecorder(), req)
		}()
	}
	wg.Wait()

	out := buf.String()
	if !strings.HasSuffix(out, "\n") {
		t.Fatalf("output must end with a newline (last line torn?): %q", out[max(0, len(out)-40):])
	}

	// Exactly n newline-terminated lines, no empty/torn lines.
	lines := strings.Split(strings.TrimSuffix(out, "\n"), "\n")
	if len(lines) != n {
		t.Fatalf("expected exactly %d lines, got %d", n, len(lines))
	}
	for i, line := range lines {
		if line == "" {
			t.Errorf("line %d is empty (interleaved/torn write)", i)
			continue
		}
		if !strings.Contains(line, "GET") || !strings.Contains(line, "/ping") {
			t.Errorf("line %d missing method/path (interleaved write): %q", i, line)
		}
		if !devLineShape.MatchString(line) {
			t.Errorf("line %d malformed (torn/interleaved): %q", i, line)
		}
	}
}

func TestFormatCLF_ByteIdentical(t *testing.T) {
	req := httptest.NewRequest("GET", "/index.html", nil)
	req.RemoteAddr = "127.0.0.1:54321"
	req.Proto = "HTTP/1.0"

	got := formatCLF(req, 200, 2326)

	// Reconstruct the exact historical format string independently, reusing the
	// timestamp emitted by formatCLF so every non-clock byte is asserted strictly.
	host := "127.0.0.1"
	ts := bracketTimestamp(t, got)
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
	ts := bracketTimestamp(t, got)
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
// character device on this platform (true on Unix; varies on Windows). It is a
// stand-in for the ModeCharDevice branch of isTerminal — os.DevNull is NOT a
// real terminal, it merely happens to be a character device, which is exactly
// what the stdlib char-device heuristic keys on. This lets the heuristic's
// positive branch be exercised deterministically without a real TTY. It returns
// nil when no character device is available, letting callers t.Skip. The file is
// closed automatically at the end of the test.
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
