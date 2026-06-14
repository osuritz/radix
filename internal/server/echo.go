package server

import (
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"math"
	mathrand "math/rand/v2"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/osuritz/radix/internal/version"
)

// EchoConfig configures the behavior of the echo handler returned by
// NewEchoHandler. It controls the default response, optional delays,
// which request sections are echoed, and request body limits.
type EchoConfig struct {
	// Status is the default HTTP status code returned (e.g. 200).
	Status int

	// Delay is a fixed duration to sleep before responding.
	Delay time.Duration

	// DelayJitter, when positive, adds a random duration in [0, DelayJitter)
	// to Delay before responding.
	DelayJitter time.Duration

	// Body, when non-empty, is returned verbatim instead of the echo JSON.
	// Status, ContentType, and Headers are still applied.
	Body string

	// ContentType is the response Content-Type header (e.g. "application/json").
	ContentType string

	// Headers are extra response headers to set, as "Key: Value" strings.
	Headers []string

	// EchoBody includes the request body sections (body, body_raw, body_size)
	// in the echo JSON when true.
	EchoBody bool

	// EchoHeaders includes the request headers section in the echo JSON when true.
	EchoHeaders bool

	// EchoQuery includes the request query section in the echo JSON when true.
	EchoQuery bool

	// BodyLimit is the maximum number of request body bytes accepted. When the
	// limit is exceeded the handler responds with 413. Zero or negative means
	// no limit is enforced.
	BodyLimit int64

	// Pretty controls whether the echo JSON is indented (true) or compact (false).
	Pretty bool

	// StatusFromPath, when true, derives the response status from request paths
	// matching /<code> or /status/<code> (code in [100,599]).
	StatusFromPath bool

	// DelayFromPath, when true, derives a response delay from request paths
	// matching /delay/<dur>, where <dur> is a Go duration or a bare number of
	// seconds. The derived delay is capped at maxPathDelay.
	DelayFromPath bool
}

// maxPathDelay caps the delay derived from /delay/<dur> paths.
const maxPathDelay = 10 * time.Second

var (
	statusPathRe = regexp.MustCompile(`^/(?:status/)?(\d{3})$`)
	delayPathRe  = regexp.MustCompile(`^/delay/(.+)$`)
)

// echoHandler is the concurrency-safe http.Handler implementation produced by
// NewEchoHandler. Its fields are treated as immutable after construction.
type echoHandler struct {
	cfg     EchoConfig
	headers http.Header
}

// NewEchoHandler returns an http.Handler that responds to every request with a
// JSON description of that request (method, headers, body, client/server info,
// TLS state, and timing). It is safe for concurrent use.
//
// When cfg.Body is non-empty, that literal body is returned instead of the echo
// JSON, while the configured status, content type, and headers are still applied.
//
// EchoConfig is passed by value (matching the sibling NewReverseProxy/NewFileServer
// constructors); this runs once at startup, not on a hot path.
//
//nolint:gocritic // hugeParam: by-value config is intentional, see doc comment above.
func NewEchoHandler(cfg EchoConfig) http.Handler {
	if cfg.Status == 0 {
		cfg.Status = http.StatusOK
	}
	if cfg.ContentType == "" {
		cfg.ContentType = "application/json"
	}

	h := http.Header{}
	for _, raw := range cfg.Headers {
		key, value, ok := strings.Cut(raw, ":")
		if !ok {
			continue
		}
		h.Add(strings.TrimSpace(key), strings.TrimSpace(value))
	}

	return &echoHandler{cfg: cfg, headers: h}
}

func (e *echoHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Determine status, honoring status-from-path when enabled.
	status := e.cfg.Status
	if e.cfg.StatusFromPath {
		if s, ok := statusFromPath(r.URL.Path); ok {
			status = s
		}
	}

	// Apply delay (fixed + jitter + path-derived).
	delayApplied := e.cfg.Delay
	if e.cfg.DelayJitter > 0 {
		delayApplied += time.Duration(mathrand.Int64N(int64(e.cfg.DelayJitter)))
	}
	if e.cfg.DelayFromPath {
		if d, ok := delayFromPath(r.URL.Path); ok {
			delayApplied = d
		}
	}
	if delayApplied > 0 {
		t := time.NewTimer(delayApplied)
		defer t.Stop()
		select {
		case <-r.Context().Done():
			return
		case <-t.C:
		}
	}

	// Read the request body, enforcing the configured limit.
	var (
		bodyBytes []byte
		readErr   error
	)
	if r.Body != nil {
		reader := r.Body
		if e.cfg.BodyLimit > 0 {
			reader = http.MaxBytesReader(w, r.Body, e.cfg.BodyLimit)
		}
		bodyBytes, readErr = io.ReadAll(reader)
		if readErr != nil {
			var maxErr *http.MaxBytesError
			if errors.As(readErr, &maxErr) {
				e.writeBodyTooLarge(w)
				return
			}
			// Other read errors: continue with whatever was read, but surface
			// the error in the echoed JSON instead of swallowing it.
		}
	}

	// Set configured response headers before writing the body.
	for key, vals := range e.headers {
		for _, v := range vals {
			w.Header().Add(key, v)
		}
	}

	// Literal body override: return verbatim, no echo JSON.
	if e.cfg.Body != "" {
		w.Header().Set("Content-Type", e.cfg.ContentType)
		w.WriteHeader(status)
		_, _ = io.WriteString(w, e.cfg.Body)
		return
	}

	resp := e.buildResponse(r, bodyBytes, readErr, delayApplied)

	var (
		payload []byte
		err     error
	)
	if e.cfg.Pretty {
		payload, err = json.MarshalIndent(resp, "", "  ")
	} else {
		payload, err = json.Marshal(resp)
	}
	if err != nil {
		http.Error(w, "failed to encode echo response", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", e.cfg.ContentType)
	w.WriteHeader(status)
	_, _ = w.Write(payload)
}

// writeBodyTooLarge responds with a 413 and a small JSON error payload.
func (e *echoHandler) writeBodyTooLarge(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusRequestEntityTooLarge)
	_, _ = w.Write([]byte(`{"error":"request body too large"}`))
}

// buildResponse assembles the echo response map for a request. The body bytes
// are passed in (already read and limited) to keep the handler the sole reader
// of the request body. A non-nil readErr (other than the 413 case, handled
// earlier) is surfaced via the echo section's body_read_error field.
func (e *echoHandler) buildResponse(r *http.Request, bodyBytes []byte, readErr error, delayApplied time.Duration) map[string]any {
	now := time.Now()

	request := map[string]any{
		"method": r.Method,
		"url":    r.URL.RequestURI(),
		"path":   r.URL.Path,
	}
	if e.cfg.EchoQuery {
		request["query"] = map[string][]string(r.URL.Query())
	}
	if e.cfg.EchoHeaders {
		request["headers"] = map[string][]string(r.Header)
		request["cookies"] = cookieMap(r)
	}
	if e.cfg.EchoBody {
		parsed := parseEchoBody(bodyBytes, r.Header.Get("Content-Type"))
		request["body"] = parsed
		request["body_raw"] = string(bodyBytes)
		request["body_size"] = len(bodyBytes)
	}

	echoSection := map[string]any{
		"version":       version.Version,
		"delay_applied": delayApplied.String(),
		"request_id":    requestID(),
	}
	if readErr != nil {
		// Surface a partial-read error without failing the response.
		echoSection["body_read_error"] = readErr.Error()
	}

	resp := map[string]any{
		"request": request,
		"client":  clientInfo(r),
		"server": map[string]any{
			"host":     r.Host,
			"protocol": r.Proto,
		},
		"tls": tlsInfo(r.TLS),
		"timing": map[string]any{
			"timestamp": now.Format(time.RFC3339Nano),
			"unix":      now.Unix(),
			"unix_nano": now.UnixNano(),
		},
		"echo": echoSection,
	}
	return resp
}

// clientInfo extracts client address details from the request's RemoteAddr.
func clientInfo(r *http.Request) map[string]any {
	info := map[string]any{
		"ip":          "",
		"port":        "",
		"remote_addr": r.RemoteAddr,
	}
	if host, port, err := net.SplitHostPort(r.RemoteAddr); err == nil {
		info["ip"] = host
		info["port"] = port
	} else {
		info["ip"] = r.RemoteAddr
	}
	return info
}

// cookieMap returns request cookies as a name->value map.
func cookieMap(r *http.Request) map[string]string {
	cookies := r.Cookies()
	m := make(map[string]string, len(cookies))
	for _, c := range cookies {
		m[c.Name] = c.Value
	}
	return m
}

// tlsInfo describes the TLS connection state. A nil state yields enabled:false.
//
// The client_cert key is always present so the response shape is stable: it
// reports the presented client certificate under client-auth/mTLS, and is nil
// (JSON null) when no peer certificate was presented (including a nil state).
func tlsInfo(state *tls.ConnectionState) map[string]any {
	if state == nil {
		return map[string]any{
			"enabled":      false,
			"version":      "",
			"cipher_suite": "",
			"server_name":  "",
			"client_cert":  nil,
		}
	}

	// Use an untyped nil (not a typed nil map) so the field is a true JSON null
	// and an absent cert compares == nil for callers inspecting the map.
	var clientCert any
	if len(state.PeerCertificates) > 0 {
		clientCert = clientCertInfo(state.PeerCertificates[0])
	}

	return map[string]any{
		"enabled":      true,
		"version":      tlsVersionName(state.Version),
		"cipher_suite": tls.CipherSuiteName(state.CipherSuite),
		"server_name":  state.ServerName,
		"client_cert":  clientCert,
	}
}

// clientCertInfo summarizes a presented client certificate for the echo
// response: subject and issuer distinguished-name fields (CN and O), serial,
// validity window, and subject-alternative names (DNS and IP).
//
// Validity timestamps use time.RFC3339 (second precision), which is the natural
// granularity for certificate NotBefore/NotAfter; the echo response's timing
// section uses RFC3339Nano because that data is sub-second.
func clientCertInfo(cert *x509.Certificate) map[string]any {
	ipAddrs := make([]string, 0, len(cert.IPAddresses))
	for _, ip := range cert.IPAddresses {
		ipAddrs = append(ipAddrs, ip.String())
	}

	return map[string]any{
		"subject": map[string]any{
			"cn": cert.Subject.CommonName,
			"o":  cert.Subject.Organization,
		},
		"issuer": map[string]any{
			"cn": cert.Issuer.CommonName,
			"o":  cert.Issuer.Organization,
		},
		"serial":       cert.SerialNumber.String(),
		"not_before":   cert.NotBefore.Format(time.RFC3339),
		"not_after":    cert.NotAfter.Format(time.RFC3339),
		"dns_names":    cert.DNSNames,
		"ip_addresses": ipAddrs,
	}
}

// tlsVersionName maps a TLS version constant to a short string (e.g. "1.2").
func tlsVersionName(v uint16) string {
	switch v {
	case tls.VersionTLS10:
		return "1.0"
	case tls.VersionTLS11:
		return "1.1"
	case tls.VersionTLS12:
		return "1.2"
	case tls.VersionTLS13:
		return "1.3"
	default:
		return ""
	}
}

// parseEchoBody attempts to parse a request body for structured echo. JSON
// bodies are unmarshaled into a generic value; form-urlencoded bodies become a
// url.Values map. Anything else (or a parse failure) yields nil.
func parseEchoBody(body []byte, contentType string) any {
	if len(body) == 0 {
		return nil
	}
	if strings.Contains(contentType, "json") {
		var parsed any
		if err := json.Unmarshal(body, &parsed); err == nil {
			return parsed
		}
		return nil
	}
	if strings.Contains(contentType, "form-urlencoded") {
		if values, err := url.ParseQuery(string(body)); err == nil {
			return map[string][]string(values)
		}
		return nil
	}
	return nil
}

// statusFromPath returns a status code parsed from paths matching /<code> or
// /status/<code> when the code is in the valid HTTP range [100, 600).
func statusFromPath(path string) (int, bool) {
	if matches := statusPathRe.FindStringSubmatch(path); matches != nil {
		status, err := strconv.Atoi(matches[1])
		if err == nil && status >= 100 && status < 600 {
			return status, true
		}
	}
	return 0, false
}

// delayFromPath returns a delay parsed from paths matching /delay/<dur>, where
// <dur> is a Go duration (e.g. "500ms") or a bare number of seconds (e.g. "2").
// The result is capped at maxPathDelay.
func delayFromPath(path string) (time.Duration, bool) {
	matches := delayPathRe.FindStringSubmatch(path)
	if matches == nil {
		return 0, false
	}
	seg := matches[1]

	// Go-duration form (e.g. "500ms", "1h"): capped at maxPathDelay, negatives rejected.
	if parsed, err := time.ParseDuration(seg); err == nil {
		if parsed < 0 {
			return 0, false
		}
		if parsed > maxPathDelay {
			return maxPathDelay, true
		}
		return parsed, true
	}

	// Bare-seconds form (e.g. "2", "0.5"). Compare against the cap in float
	// seconds BEFORE converting to a Duration, so large finite values cannot
	// overflow int64 nanoseconds and defeat the cap.
	secs, err := strconv.ParseFloat(seg, 64)
	if err != nil {
		return 0, false
	}
	if math.IsNaN(secs) || math.IsInf(secs, 0) || secs < 0 {
		return 0, false
	}
	if secs >= maxPathDelay.Seconds() {
		return maxPathDelay, true
	}
	return time.Duration(secs * float64(time.Second)), true
}

// requestID returns a short random hex identifier for an echoed request.
func requestID() string {
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		// Fall back to a timestamp-derived value; collisions are acceptable
		// for a debugging identifier.
		return strconv.FormatInt(time.Now().UnixNano(), 16)
	}
	return hex.EncodeToString(b[:])
}
