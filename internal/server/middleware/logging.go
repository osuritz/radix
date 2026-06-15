// Package middleware provides HTTP middleware components for logging, metrics, and request handling.
package middleware

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"
	"unicode/utf8"
)

// LogFormat represents the logging output format
type LogFormat string

const (
	// LogFormatCLF is Common Log Format
	LogFormatCLF LogFormat = "clf"
	// LogFormatExtendedCLF is Extended Common Log Format (includes referrer and user-agent)
	LogFormatExtendedCLF LogFormat = "extended_clf"
	// LogFormatDev is developer-friendly colored format
	LogFormatDev LogFormat = "dev"
)

// LogAnnotation carries request-scoped context that the generic logging
// middleware cannot infer on its own — chiefly what a request was routed to.
//
// The logging middleware seeds a fresh *LogAnnotation into each request's
// context before invoking the next handler; downstream handlers (e.g. the
// reverse proxy or the SPA file server) may then populate it. Because each
// request gets its own pointer, there is no shared mutable state and no need
// for synchronization: the handler that mutates the annotation runs to
// completion (synchronously, on the request goroutine) before the middleware
// reads it back. Only the dev access-log format renders these fields; CLF and
// Extended CLF ignore the annotation entirely.
type LogAnnotation struct {
	// Kind labels the request handler that produced the response (e.g.
	// "proxy", "fallback"). It is currently informational and reserved for
	// future use; the dev format renders Target, not Kind.
	Kind string
	// Target identifies what the request was routed to, rendered in the dev
	// format as a "→ <target>" column (e.g. an upstream host for the proxy, or
	// "fallback" for an SPA index fallback). Empty means no target column.
	Target string
}

// logAnnotationKey is the unexported context key under which a *LogAnnotation
// is stored. Using a dedicated unexported type avoids collisions with any other
// context value.
type logAnnotationKey struct{}

// withLogAnnotation returns a copy of ctx carrying a fresh, empty
// *LogAnnotation, along with that pointer so the caller can pass it to
// downstream handlers. The logging middleware seeds one per request.
func withLogAnnotation(ctx context.Context) (context.Context, *LogAnnotation) {
	a := &LogAnnotation{}
	return context.WithValue(ctx, logAnnotationKey{}, a), a
}

// LogAnnotationFromContext returns the *LogAnnotation seeded into ctx by the
// logging middleware, or nil if none is present (e.g. when the logging
// middleware is not installed). Callers must nil-check the result before
// mutating it.
func LogAnnotationFromContext(ctx context.Context) *LogAnnotation {
	a, _ := ctx.Value(logAnnotationKey{}).(*LogAnnotation)
	return a
}

// Dev-format column layout constants.
//
// The dev line is a single, aligned row tuned to read like a Vite-style dev
// log. Methods are padded to a fixed width so request paths line up across
// rows; paths are padded (and truncated with a single ellipsis) so the status,
// latency and size columns stay aligned regardless of path length.
const (
	// devMethodWidth left-justifies the HTTP method (e.g. "GET    ") so paths
	// align across methods of differing length. "DELETE" (6) and "OPTIONS" (7)
	// are the longest common methods, so 7 keeps every standard method padded
	// without an extra space.
	devMethodWidth = 7
	// devPathWidth left-justifies the request path so the status column starts
	// at a fixed offset. Paths longer than this are truncated with a single
	// ellipsis; shorter paths are space-padded.
	devPathWidth = 28
)

// ANSI escape codes used by the dev format.
const (
	ansiReset = "\033[0m"
	ansiDim   = "\033[2m"
)

// LoggingConfig holds configuration for the logging middleware
type LoggingConfig struct {
	// Format selects the access-log output format (dev, clf, extended_clf).
	Format LogFormat
	// NoColor disables ANSI coloring of the dev format unconditionally. It is
	// the highest-priority color control; see resolveColor for full precedence.
	NoColor bool
	// Output is the destination for all log lines (dev, CLF and Extended CLF).
	// When nil it defaults to os.Stdout. Injecting a writer makes the middleware
	// testable and lets callers redirect logs.
	Output io.Writer
}

// responseWriter wraps http.ResponseWriter to capture status code and response size
type responseWriter struct {
	http.ResponseWriter
	status int
	size   int
}

func (rw *responseWriter) WriteHeader(status int) {
	rw.status = status
	rw.ResponseWriter.WriteHeader(status)
}

func (rw *responseWriter) Write(b []byte) (int, error) {
	if rw.status == 0 {
		rw.status = http.StatusOK
	}
	size, err := rw.ResponseWriter.Write(b)
	rw.size += size
	return size, err
}

// Flush forwards to the underlying ResponseWriter's Flush when it supports
// http.Flusher, keeping the wrapper transparent to streaming handlers (e.g. the
// SSE mock route). It is a no-op when the underlying writer is not flushable.
//
// A flush triggers an implicit HTTP 200 to the client if no header was written
// yet, so default the recorded status to 200 first — mirroring Write — so a
// flush-first response is logged as 200 rather than 0.
func (rw *responseWriter) Flush() {
	if rw.status == 0 {
		rw.status = http.StatusOK
	}
	if f, ok := rw.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

// Unwrap exposes the wrapped ResponseWriter so http.NewResponseController (and
// any other chain-walker) can reach the underlying writer's capabilities.
func (rw *responseWriter) Unwrap() http.ResponseWriter {
	return rw.ResponseWriter
}

// Logging returns a middleware that logs HTTP requests.
//
// The output writer and the dev-format color decision are resolved once, here,
// and captured in the returned closure: environment (NO_COLOR) and TTY state do
// not change at runtime, so there is no reason to re-evaluate them per request.
// Writes are serialized under a mutex and each line is assembled into a single
// string before being written, so concurrent requests never interleave output.
func Logging(config LoggingConfig) func(http.Handler) http.Handler {
	out := config.Output
	if out == nil {
		out = os.Stdout
	}
	color := resolveColor(config.NoColor, out)

	var mu sync.Mutex
	write := func(line string) {
		mu.Lock()
		defer mu.Unlock()
		_, _ = io.WriteString(out, line)
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()

			// Wrap response writer to capture status and size
			wrapped := &responseWriter{
				ResponseWriter: w,
				status:         0,
				size:           0,
			}

			// Seed a fresh per-request annotation into the context so downstream
			// handlers (proxy, SPA file server) can record what the request was
			// routed to. Each request owns its own pointer, so there is no shared
			// state to synchronize.
			ctx, annotation := withLogAnnotation(r.Context())
			r = r.WithContext(ctx)

			// Process request
			next.ServeHTTP(wrapped, r)

			// Log after request is complete
			duration := time.Since(start)
			write(formatRequest(r, wrapped.status, wrapped.size, duration, config.Format, color, annotation.Target))
		})
	}
}

// resolveColor decides whether dev-format ANSI coloring is enabled.
//
// Precedence (first match wins):
//  1. noColor (--no-color / cfg.NoColor) true  -> OFF.
//  2. else NO_COLOR env set and non-empty       -> OFF (https://no-color.org).
//  3. else FORCE_COLOR or CLICOLOR_FORCE set    -> ON (overrides only the TTY
//     check below; it can never re-enable color past steps 1 or 2).
//  4. else writer fails the TTY heuristic        -> OFF.
//  5. else                                       -> ON.
//
// "TTY" here is the stdlib-only char-device heuristic in isTerminal, not a true
// isatty; see that function for the heuristic's exact trade-offs and why a real
// terminal check is avoided (it would pull in an external dependency).
func resolveColor(noColor bool, w io.Writer) bool {
	if noColor {
		return false
	}
	if os.Getenv("NO_COLOR") != "" {
		return false
	}
	if os.Getenv("FORCE_COLOR") != "" || os.Getenv("CLICOLOR_FORCE") != "" {
		return true
	}
	return isTerminal(w)
}

// isTerminal is a stdlib-only, best-effort terminal check — NOT a true isatty.
//
// It type-asserts w to *os.File and reports whether the underlying file is a
// character device (os.ModeCharDevice). A real isatty() would issue a terminal
// ioctl (TIOCGETA / TCGETS), which the standard library does not expose; doing
// so would require golang.org/x/term or golang.org/x/sys, dependencies this
// project deliberately avoids. The trade-off of the char-device heuristic:
//
//   - Correctly classified as non-TTY: the common non-interactive targets —
//     regular files and pipes (e.g. `radix ... > out.log` or `| tee`) — so color
//     is auto-disabled exactly where it would corrupt captured output.
//   - Falsely classified as a TTY: any character device, e.g. /dev/null and
//     /dev/tty. These are uncommon log destinations, and the escape hatches
//     cover them: NO_COLOR / --no-color force color off, FORCE_COLOR /
//     CLICOLOR_FORCE force it on (see resolveColor).
//
// Any non-*os.File writer (e.g. bytes.Buffer) is treated as non-TTY.
func isTerminal(w io.Writer) bool {
	f, ok := w.(*os.File)
	if !ok {
		return false
	}
	fi, err := f.Stat()
	if err != nil {
		return false
	}
	return (fi.Mode() & os.ModeCharDevice) != 0
}

// formatRequest renders a request into a single log line (newline-terminated)
// for the given format. CLF and Extended CLF are byte-identical to their
// historical output and ignore target; only the dev format renders the
// "→ target" column (and only when target is non-empty), with coloring gated
// on color.
func formatRequest(r *http.Request, status, size int, duration time.Duration, format LogFormat, color bool, target string) string {
	switch format {
	case LogFormatCLF:
		return formatCLF(r, status, size)
	case LogFormatExtendedCLF:
		return formatExtendedCLF(r, status, size)
	case LogFormatDev:
		return formatDevLine(time.Now(), r.Method, r.RequestURI, status, size, duration, color, target)
	default:
		return formatDevLine(time.Now(), r.Method, r.RequestURI, status, size, duration, color, target)
	}
}

// clientHost extracts the client IP (host portion) from r.RemoteAddr, dropping
// the port. Preserved verbatim from the original CLF implementation.
func clientHost(remoteAddr string) string {
	host := remoteAddr
	if colon := len(host) - 1; colon >= 0 {
		for i := colon; i >= 0; i-- {
			if host[i] == ':' {
				host = host[:i]
				break
			}
		}
	}
	return host
}

// formatCLF renders Common Log Format.
// Format: host ident authuser date request status bytes
// Example: 127.0.0.1 - - [10/Oct/2000:13:55:36 -0700] "GET /index.html HTTP/1.0" 200 2326
func formatCLF(r *http.Request, status, size int) string {
	return formatCLFAt(time.Now(), r, status, size)
}

// formatCLFAt renders Common Log Format at the given timestamp. now is injected
// so the formatter is deterministically testable; formatCLF calls it with
// time.Now(). The annotation is intentionally not a parameter: CLF must stay
// byte-identical regardless of any per-request log annotation.
func formatCLFAt(now time.Time, r *http.Request, status, size int) string {
	host := clientHost(r.RemoteAddr)
	timestamp := now.Format("02/Jan/2006:15:04:05 -0700")
	requestLine := fmt.Sprintf("%s %s %s", r.Method, r.RequestURI, r.Proto)

	// %q is intentionally NOT used here: the CLF output must stay byte-identical
	// to the historical format, and %q would escape special characters in the
	// request line differently. The explicit \"...\" quoting is load-bearing.
	//nolint:gocritic // sprintfQuotedString: literal quoting preserves byte-identical CLF output.
	return fmt.Sprintf("%s - - [%s] \"%s\" %d %d\n",
		host,
		timestamp,
		requestLine,
		status,
		size,
	)
}

// formatExtendedCLF renders Extended Common Log Format.
// Format: CLF + "referrer" "user-agent"
// Example: 127.0.0.1 - - [10/Oct/2000:13:55:36 -0700] "GET /index.html HTTP/1.0" 200 2326 "http://example.com" "Mozilla/5.0"
func formatExtendedCLF(r *http.Request, status, size int) string {
	return formatExtendedCLFAt(time.Now(), r, status, size)
}

// formatExtendedCLFAt renders Extended Common Log Format at the given
// timestamp. now is injected so the formatter is deterministically testable;
// formatExtendedCLF calls it with time.Now(). As with formatCLFAt, no
// annotation is threaded in: Extended CLF must stay byte-identical regardless
// of any per-request log annotation.
func formatExtendedCLFAt(now time.Time, r *http.Request, status, size int) string {
	host := clientHost(r.RemoteAddr)
	timestamp := now.Format("02/Jan/2006:15:04:05 -0700")
	requestLine := fmt.Sprintf("%s %s %s", r.Method, r.RequestURI, r.Proto)

	referrer := r.Header.Get("Referer")
	if referrer == "" {
		referrer = "-"
	}

	userAgent := r.Header.Get("User-Agent")
	if userAgent == "" {
		userAgent = "-"
	}

	// See formatCLF: literal \"...\" quoting is required for byte-identical output.
	//nolint:gocritic // sprintfQuotedString: literal quoting preserves byte-identical Extended CLF output.
	return fmt.Sprintf("%s - - [%s] \"%s\" %d %d \"%s\" \"%s\"\n",
		host,
		timestamp,
		requestLine,
		status,
		size,
		referrer,
		userAgent,
	)
}

// formatDevLine renders the polished, developer-friendly dev line.
//
// Layout (newline-terminated), columns in order:
//
//	<dim HH:MM:SS> <method,padded> <path,padded/truncated> [<dim →> <target> ]<status> <latency>[ <size>]
//
// e.g. (no color, no target):
//
//	14:23:01 GET     /index.html                  200 12ms 2.3KB
//
// and with a target (e.g. a proxy upstream or an SPA fallback):
//
//	14:23:01 GET     /api/users                   → localhost:3000 200 12ms 2.3KB
//
// The timestamp is dimmed, the method is colored via getMethodColor, and the
// status is colored via getStatusColor when color is true. The "→ target"
// segment is rendered only when target is non-empty; the arrow itself is
// dimmed (when color is on) and the target value is left uncolored. When target
// is empty the line is byte-identical to the no-target layout. The size column
// is omitted entirely when size == 0 (no "-" placeholder). now is injected so
// the formatter is deterministically testable.
func formatDevLine(now time.Time, method, uri string, status, size int, d time.Duration, color bool, target string) string {
	ts := now.Format("15:04:05")
	// Cap pathological custom methods to the method column width so they cannot
	// shift the path/status columns to their right. Standard methods are <= 7
	// runes (devMethodWidth) and are unaffected.
	method = truncateRunes(method, devMethodWidth)
	paddedMethod := padRight(method, devMethodWidth)
	paddedPath := padRight(truncatePath(uri, devPathWidth), devPathWidth)
	durationStr := formatDuration(d)

	var b strings.Builder

	// Timestamp (dimmed).
	if color {
		b.WriteString(ansiDim)
		b.WriteString(ts)
		b.WriteString(ansiReset)
	} else {
		b.WriteString(ts)
	}
	b.WriteByte(' ')

	// Method (colored, padded).
	if color {
		b.WriteString(getMethodColor(method))
		b.WriteString(paddedMethod)
		b.WriteString(ansiReset)
	} else {
		b.WriteString(paddedMethod)
	}
	b.WriteByte(' ')

	// Path (padded / truncated).
	b.WriteString(paddedPath)
	b.WriteByte(' ')

	// Target ("→ <target>"), only when meaningful. The arrow is dimmed; the
	// target value is left uncolored so it reads as a quiet annotation. Omitted
	// entirely (no extra spacing) when target is empty, keeping the no-target
	// line byte-identical to the original layout.
	if target != "" {
		if color {
			b.WriteString(ansiDim)
			b.WriteString("→")
			b.WriteString(ansiReset)
		} else {
			b.WriteString("→")
		}
		b.WriteByte(' ')
		b.WriteString(target)
		b.WriteByte(' ')
	}

	// Status (colored).
	if color {
		b.WriteString(getStatusColor(status))
		fmt.Fprintf(&b, "%d", status)
		b.WriteString(ansiReset)
	} else {
		fmt.Fprintf(&b, "%d", status)
	}
	b.WriteByte(' ')

	// Latency.
	b.WriteString(durationStr)

	// Size (optional; omitted when zero).
	if size > 0 {
		b.WriteByte(' ')
		b.WriteString(formatSize(size))
	}

	b.WriteByte('\n')
	return b.String()
}

// padRight left-justifies s to width with trailing spaces, where width is a
// count of runes (not bytes). Using the rune count keeps multibyte content
// (accented or CJK paths) aligned to the same visual column as ASCII content of
// equal rune length; counting bytes would under-pad multibyte strings and shift
// every column to its right. Strings already at or beyond width are returned
// unchanged (callers truncate first where needed).
func padRight(s string, width int) string {
	n := utf8.RuneCountInString(s)
	if n >= width {
		return s
	}
	return s + strings.Repeat(" ", width-n)
}

// truncatePath shortens path to at most width runes, replacing the trailing
// overflow with a single ellipsis ('…') so columns stay aligned. Paths within
// width are returned unchanged.
func truncatePath(path string, width int) string {
	runes := []rune(path)
	if len(runes) <= width {
		return path
	}
	if width <= 1 {
		return "…"
	}
	return string(runes[:width-1]) + "…"
}

// truncateRunes shortens s to at most width runes by hard-cutting the overflow
// (no ellipsis). It is used for the method column, where any over-long value is
// already pathological and a clean cut keeps the column width fixed. Strings
// within width are returned unchanged.
func truncateRunes(s string, width int) string {
	if width < 0 {
		width = 0
	}
	if utf8.RuneCountInString(s) <= width {
		return s
	}
	return string([]rune(s)[:width])
}

// getMethodColor returns ANSI color code for HTTP method
func getMethodColor(method string) string {
	switch method {
	case "GET":
		return "\033[36m" // Cyan
	case "POST":
		return "\033[32m" // Green
	case "PUT":
		return "\033[33m" // Yellow
	case "DELETE":
		return "\033[31m" // Red
	case "PATCH":
		return "\033[35m" // Magenta
	default:
		return "\033[37m" // White
	}
}

// getStatusColor returns ANSI color code for HTTP status
func getStatusColor(status int) string {
	switch {
	case status >= 200 && status < 300:
		return "\033[32m" // Green (success)
	case status >= 300 && status < 400:
		return "\033[36m" // Cyan (redirect)
	case status >= 400 && status < 500:
		return "\033[33m" // Yellow (client error)
	case status >= 500:
		return "\033[31m" // Red (server error)
	default:
		return "\033[37m" // White
	}
}

// formatDuration formats duration in human-readable format
func formatDuration(d time.Duration) string {
	switch {
	case d < time.Microsecond:
		return fmt.Sprintf("%dns", d.Nanoseconds())
	case d < time.Millisecond:
		return fmt.Sprintf("%dµs", d.Microseconds())
	case d < time.Second:
		return fmt.Sprintf("%dms", d.Milliseconds())
	default:
		return fmt.Sprintf("%.2fs", d.Seconds())
	}
}

// formatSize formats byte size in human-readable format
func formatSize(size int) string {
	if size == 0 {
		return "-"
	}
	const unit = 1024
	if size < unit {
		return fmt.Sprintf("%dB", size)
	}
	div, exp := int64(unit), 0
	for n := size / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f%cB", float64(size)/float64(div), "KMGTPE"[exp])
}
