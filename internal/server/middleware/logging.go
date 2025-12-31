// Package middleware provides HTTP middleware components for logging, metrics, and request handling.
package middleware

import (
	"fmt"
	"net/http"
	"time"
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

// LoggingConfig holds configuration for the logging middleware
type LoggingConfig struct {
	Format  LogFormat
	NoColor bool
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

// Logging returns a middleware that logs HTTP requests
func Logging(config LoggingConfig) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()

			// Wrap response writer to capture status and size
			wrapped := &responseWriter{
				ResponseWriter: w,
				status:         0,
				size:           0,
			}

			// Process request
			next.ServeHTTP(wrapped, r)

			// Log after request is complete
			duration := time.Since(start)
			logRequest(r, wrapped.status, wrapped.size, duration, config)
		})
	}
}

// logRequest logs the request in the specified format
func logRequest(r *http.Request, status, size int, duration time.Duration, config LoggingConfig) {
	switch config.Format {
	case LogFormatCLF:
		logCLF(r, status, size)
	case LogFormatExtendedCLF:
		logExtendedCLF(r, status, size)
	case LogFormatDev:
		logDev(r, status, size, duration, config.NoColor)
	default:
		logDev(r, status, size, duration, config.NoColor)
	}
}

// logCLF logs in Common Log Format
// Format: host ident authuser date request status bytes
// Example: 127.0.0.1 - - [10/Oct/2000:13:55:36 -0700] "GET /index.html HTTP/1.0" 200 2326
func logCLF(r *http.Request, status, size int) {
	// Extract client IP
	host := r.RemoteAddr
	if colon := len(host) - 1; colon >= 0 {
		for i := colon; i >= 0; i-- {
			if host[i] == ':' {
				host = host[:i]
				break
			}
		}
	}

	// Format timestamp in CLF format
	timestamp := time.Now().Format("02/Jan/2006:15:04:05 -0700")

	// Build request line
	requestLine := fmt.Sprintf("%s %s %s", r.Method, r.RequestURI, r.Proto)

	// CLF format
	fmt.Printf("%s - - [%s] \"%s\" %d %d\n",
		host,
		timestamp,
		requestLine,
		status,
		size,
	)
}

// logExtendedCLF logs in Extended Common Log Format
// Format: CLF + "referrer" "user-agent"
// Example: 127.0.0.1 - - [10/Oct/2000:13:55:36 -0700] "GET /index.html HTTP/1.0" 200 2326 "http://example.com" "Mozilla/5.0"
func logExtendedCLF(r *http.Request, status, size int) {
	// Extract client IP
	host := r.RemoteAddr
	if colon := len(host) - 1; colon >= 0 {
		for i := colon; i >= 0; i-- {
			if host[i] == ':' {
				host = host[:i]
				break
			}
		}
	}

	// Format timestamp in CLF format
	timestamp := time.Now().Format("02/Jan/2006:15:04:05 -0700")

	// Build request line
	requestLine := fmt.Sprintf("%s %s %s", r.Method, r.RequestURI, r.Proto)

	// Get referrer and user-agent
	referrer := r.Header.Get("Referer")
	if referrer == "" {
		referrer = "-"
	}

	userAgent := r.Header.Get("User-Agent")
	if userAgent == "" {
		userAgent = "-"
	}

	// Extended CLF format
	fmt.Printf("%s - - [%s] \"%s\" %d %d \"%s\" \"%s\"\n",
		host,
		timestamp,
		requestLine,
		status,
		size,
		referrer,
		userAgent,
	)
}

// logDev logs in developer-friendly format with optional colors
// Format: METHOD /path STATUS SIZE DURATION
// Example: GET /index.html 200 2326 12ms
func logDev(r *http.Request, status, size int, duration time.Duration, noColor bool) {
	// Format duration
	durationStr := formatDuration(duration)

	// Format size
	sizeStr := formatSize(size)

	if noColor {
		// No color output
		fmt.Printf("%s %s %d %s %s\n",
			r.Method,
			r.RequestURI,
			status,
			sizeStr,
			durationStr,
		)
	} else {
		// Colored output
		methodColor := getMethodColor(r.Method)
		statusColor := getStatusColor(status)

		fmt.Printf("%s%s\033[0m %s %s%d\033[0m %s %s\n",
			methodColor,
			r.Method,
			r.RequestURI,
			statusColor,
			status,
			sizeStr,
			durationStr,
		)
	}
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
	if d < time.Microsecond {
		return fmt.Sprintf("%dns", d.Nanoseconds())
	} else if d < time.Millisecond {
		return fmt.Sprintf("%dµs", d.Microseconds())
	} else if d < time.Second {
		return fmt.Sprintf("%dms", d.Milliseconds())
	}
	return fmt.Sprintf("%.2fs", d.Seconds())
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
