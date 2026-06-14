package server

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"errors"
	"io"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// defaultEchoConfig returns an EchoConfig with the same defaults the CLI uses,
// so tests exercise realistic behavior unless they override specific fields.
func defaultEchoConfig() EchoConfig {
	return EchoConfig{
		Status:      http.StatusOK,
		ContentType: "application/json",
		EchoBody:    true,
		EchoHeaders: true,
		EchoQuery:   true,
		BodyLimit:   1 << 20,
		Pretty:      false,
	}
}

// doEcho runs a request through a handler built from cfg and returns the recorder.
func doEcho(cfg EchoConfig, req *http.Request) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	NewEchoHandler(cfg).ServeHTTP(rec, req)
	return rec
}

// decodeEcho decodes the recorder body into a generic map, failing the test on error.
func decodeEcho(t *testing.T, rec *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var out map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("failed to decode echo JSON: %v\nbody: %s", err, rec.Body.String())
	}
	return out
}

func TestEcho_BasicGET(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/api/users?limit=10&limit=20", nil)
	req.Header.Set("X-Custom", "abc")

	rec := doEcho(defaultEchoConfig(), req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("expected application/json content type, got %q", ct)
	}

	out := decodeEcho(t, rec)
	request, ok := out["request"].(map[string]any)
	if !ok {
		t.Fatalf("request section missing or wrong type: %#v", out["request"])
	}
	if request["method"] != http.MethodGet {
		t.Errorf("expected method GET, got %v", request["method"])
	}
	if request["path"] != "/api/users" {
		t.Errorf("expected path /api/users, got %v", request["path"])
	}

	query, ok := request["query"].(map[string]any)
	if !ok {
		t.Fatalf("query section missing: %#v", request["query"])
	}
	limit, ok := query["limit"].([]any)
	if !ok || len(limit) != 2 || limit[0] != "10" || limit[1] != "20" {
		t.Errorf("expected limit=[10 20], got %#v", query["limit"])
	}

	headers, ok := request["headers"].(map[string]any)
	if !ok {
		t.Fatalf("headers section missing: %#v", request["headers"])
	}
	custom, ok := headers["X-Custom"].([]any)
	if !ok || len(custom) != 1 || custom[0] != "abc" {
		t.Errorf("expected X-Custom=[abc], got %#v", headers["X-Custom"])
	}
}

func TestEcho_PostJSONBody(t *testing.T) {
	payload := `{"name":"John Doe","age":30}`
	req := httptest.NewRequest(http.MethodPost, "/submit", strings.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")

	rec := doEcho(defaultEchoConfig(), req)
	out := decodeEcho(t, rec)
	request := out["request"].(map[string]any)

	body, ok := request["body"].(map[string]any)
	if !ok {
		t.Fatalf("expected parsed body object, got %#v", request["body"])
	}
	if body["name"] != "John Doe" {
		t.Errorf("expected name=John Doe, got %v", body["name"])
	}
	if body["age"] != float64(30) {
		t.Errorf("expected age=30, got %v", body["age"])
	}
	if request["body_raw"] != payload {
		t.Errorf("expected body_raw %q, got %v", payload, request["body_raw"])
	}
	if size, ok := request["body_size"].(float64); !ok || int(size) != len(payload) {
		t.Errorf("expected body_size %d, got %v", len(payload), request["body_size"])
	}
}

func TestEcho_PostFormBody(t *testing.T) {
	form := "name=John&role=admin&role=dev"
	req := httptest.NewRequest(http.MethodPost, "/form", strings.NewReader(form))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	rec := doEcho(defaultEchoConfig(), req)
	out := decodeEcho(t, rec)
	request := out["request"].(map[string]any)

	body, ok := request["body"].(map[string]any)
	if !ok {
		t.Fatalf("expected parsed form body, got %#v", request["body"])
	}
	name, ok := body["name"].([]any)
	if !ok || len(name) != 1 || name[0] != "John" {
		t.Errorf("expected name=[John], got %#v", body["name"])
	}
	role, ok := body["role"].([]any)
	if !ok || len(role) != 2 {
		t.Errorf("expected role with 2 values, got %#v", body["role"])
	}
}

func TestEcho_BodyOverride(t *testing.T) {
	cfg := defaultEchoConfig()
	cfg.Body = "plain text response"
	cfg.ContentType = "text/plain"
	cfg.Status = http.StatusAccepted

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := doEcho(cfg, req)

	if rec.Code != http.StatusAccepted {
		t.Errorf("expected 202, got %d", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "text/plain" {
		t.Errorf("expected text/plain, got %q", ct)
	}
	if rec.Body.String() != "plain text response" {
		t.Errorf("expected literal body, got %q", rec.Body.String())
	}
	// Ensure it is NOT JSON-wrapped.
	if strings.Contains(rec.Body.String(), "request") {
		t.Errorf("body should not be echo JSON: %q", rec.Body.String())
	}
}

func TestEcho_CustomStatus(t *testing.T) {
	cfg := defaultEchoConfig()
	cfg.Status = http.StatusCreated

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := doEcho(cfg, req)

	if rec.Code != http.StatusCreated {
		t.Errorf("expected 201, got %d", rec.Code)
	}
}

func TestEcho_StatusFromPath(t *testing.T) {
	tests := []struct {
		name       string
		path       string
		wantStatus int
	}{
		{"bare code", "/404", http.StatusNotFound},
		{"status prefix", "/status/500", http.StatusInternalServerError},
		{"non-numeric falls back to default", "/abc", http.StatusOK},
		{"out of range falls back", "/999", http.StatusOK},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := defaultEchoConfig()
			cfg.StatusFromPath = true
			req := httptest.NewRequest(http.MethodGet, tt.path, nil)
			rec := doEcho(cfg, req)
			if rec.Code != tt.wantStatus {
				t.Errorf("path %q: expected %d, got %d", tt.path, tt.wantStatus, rec.Code)
			}
		})
	}
}

func TestEcho_StatusFromPathDisabled(t *testing.T) {
	cfg := defaultEchoConfig()
	cfg.StatusFromPath = false
	req := httptest.NewRequest(http.MethodGet, "/404", nil)
	rec := doEcho(cfg, req)
	if rec.Code != http.StatusOK {
		t.Errorf("expected default 200 when status-from-path disabled, got %d", rec.Code)
	}
}

func TestEcho_Delay(t *testing.T) {
	cfg := defaultEchoConfig()
	cfg.Delay = 50 * time.Millisecond

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	start := time.Now()
	rec := doEcho(cfg, req)
	elapsed := time.Since(start)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	if elapsed < 40*time.Millisecond {
		t.Errorf("expected delay of at least ~50ms, elapsed %v", elapsed)
	}
}

func TestEcho_DelayFromPath(t *testing.T) {
	tests := []struct {
		name    string
		path    string
		wantMin time.Duration
	}{
		{"duration form", "/delay/40ms", 30 * time.Millisecond},
		{"bare seconds capped", "/delay/0.05", 30 * time.Millisecond},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := defaultEchoConfig()
			cfg.DelayFromPath = true
			req := httptest.NewRequest(http.MethodGet, tt.path, nil)
			start := time.Now()
			rec := doEcho(cfg, req)
			elapsed := time.Since(start)
			if rec.Code != http.StatusOK {
				t.Fatalf("expected 200, got %d", rec.Code)
			}
			if elapsed < tt.wantMin {
				t.Errorf("path %q: expected delay >= %v, elapsed %v", tt.path, tt.wantMin, elapsed)
			}
		})
	}
}

func TestEcho_EchoSectionsOmitted(t *testing.T) {
	cfg := defaultEchoConfig()
	cfg.EchoHeaders = false
	cfg.EchoQuery = false
	cfg.EchoBody = false

	req := httptest.NewRequest(http.MethodPost, "/x?a=1", strings.NewReader("data"))
	rec := doEcho(cfg, req)
	out := decodeEcho(t, rec)
	request := out["request"].(map[string]any)

	if _, ok := request["headers"]; ok {
		t.Error("headers should be omitted when echo-headers=false")
	}
	if _, ok := request["cookies"]; ok {
		t.Error("cookies should be omitted when echo-headers=false")
	}
	if _, ok := request["query"]; ok {
		t.Error("query should be omitted when echo-query=false")
	}
	if _, ok := request["body"]; ok {
		t.Error("body should be omitted when echo-body=false")
	}
	if _, ok := request["body_raw"]; ok {
		t.Error("body_raw should be omitted when echo-body=false")
	}
}

func TestEcho_BodyLimitExceeded(t *testing.T) {
	cfg := defaultEchoConfig()
	cfg.BodyLimit = 16

	big := strings.Repeat("x", 1024)
	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(big))
	rec := doEcho(cfg, req)

	if rec.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("expected 413, got %d", rec.Code)
	}
	var out map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("expected JSON error body, got %q (err %v)", rec.Body.String(), err)
	}
	if _, ok := out["error"]; !ok {
		t.Errorf("expected error field in 413 body, got %#v", out)
	}
}

func TestEcho_TLSSectionEnabled(t *testing.T) {
	cfg := defaultEchoConfig()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.TLS = &tls.ConnectionState{
		Version:     tls.VersionTLS13,
		CipherSuite: tls.TLS_AES_128_GCM_SHA256,
		ServerName:  "example.test",
	}

	rec := doEcho(cfg, req)
	out := decodeEcho(t, rec)
	tlsSection, ok := out["tls"].(map[string]any)
	if !ok {
		t.Fatalf("tls section missing: %#v", out["tls"])
	}
	if tlsSection["enabled"] != true {
		t.Errorf("expected tls.enabled=true, got %v", tlsSection["enabled"])
	}
	if tlsSection["version"] != "1.3" {
		t.Errorf("expected tls.version=1.3, got %v", tlsSection["version"])
	}
	if tlsSection["server_name"] != "example.test" {
		t.Errorf("expected server_name=example.test, got %v", tlsSection["server_name"])
	}
	if cs, _ := tlsSection["cipher_suite"].(string); cs == "" {
		t.Errorf("expected non-empty cipher_suite, got %v", tlsSection["cipher_suite"])
	}
}

func TestEcho_TLSSectionDisabled(t *testing.T) {
	cfg := defaultEchoConfig()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.TLS = nil

	rec := doEcho(cfg, req)
	out := decodeEcho(t, rec)
	tlsSection := out["tls"].(map[string]any)
	if tlsSection["enabled"] != false {
		t.Errorf("expected tls.enabled=false, got %v", tlsSection["enabled"])
	}
	if v, ok := tlsSection["client_cert"]; !ok || v != nil {
		t.Errorf("expected client_cert present and null when TLS disabled, got (present=%v) %#v", ok, v)
	}
}

// makeTestClientCert builds a client certificate with known fields, signed by a
// distinct in-test CA so the parsed issuer differs from the subject, and returns
// the parsed *x509.Certificate for exercising clientCertInfo/tlsInfo.
//
// Organization is a single value per RDN: pkix marshaling does not preserve the
// order of a multi-value Organization set, so single-element slices keep the
// assertions deterministic while still exercising the []string shape.
func makeTestClientCert(t *testing.T) *x509.Certificate {
	t.Helper()

	caKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generate CA key: %v", err)
	}
	caTmpl := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject: pkix.Name{
			CommonName:   "Radix Test CA",
			Organization: []string{"Radix CA Org"},
		},
		NotBefore:             time.Date(2024, time.January, 1, 0, 0, 0, 0, time.UTC),
		NotAfter:              time.Date(2026, time.January, 1, 0, 0, 0, 0, time.UTC),
		IsCA:                  true,
		BasicConstraintsValid: true,
		KeyUsage:              x509.KeyUsageCertSign,
	}
	caDER, err := x509.CreateCertificate(rand.Reader, caTmpl, caTmpl, &caKey.PublicKey, caKey)
	if err != nil {
		t.Fatalf("create CA certificate: %v", err)
	}
	caCert, err := x509.ParseCertificate(caDER)
	if err != nil {
		t.Fatalf("parse CA certificate: %v", err)
	}

	clientKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generate client key: %v", err)
	}
	clientTmpl := &x509.Certificate{
		SerialNumber: big.NewInt(0x1f2e3d4c),
		Subject: pkix.Name{
			CommonName:   "client.example.test",
			Organization: []string{"Radix Test Org"},
		},
		NotBefore:   time.Date(2024, time.January, 2, 3, 4, 5, 0, time.UTC),
		NotAfter:    time.Date(2025, time.January, 2, 3, 4, 5, 0, time.UTC),
		DNSNames:    []string{"client.example.test", "alt.example.test"},
		IPAddresses: []net.IP{net.ParseIP("192.0.2.10"), net.ParseIP("2001:db8::1")},
	}
	clientDER, err := x509.CreateCertificate(rand.Reader, clientTmpl, caCert, &clientKey.PublicKey, caKey)
	if err != nil {
		t.Fatalf("create client certificate: %v", err)
	}
	clientCert, err := x509.ParseCertificate(clientDER)
	if err != nil {
		t.Fatalf("parse client certificate: %v", err)
	}
	return clientCert
}

func TestTLSInfo_ClientCertPresented(t *testing.T) {
	cert := makeTestClientCert(t)
	state := &tls.ConnectionState{
		Version:          tls.VersionTLS13,
		CipherSuite:      tls.TLS_AES_128_GCM_SHA256,
		ServerName:       "example.test",
		PeerCertificates: []*x509.Certificate{cert},
	}

	info := tlsInfo(state)
	clientCert, ok := info["client_cert"].(map[string]any)
	if !ok {
		t.Fatalf("expected populated client_cert map, got %#v", info["client_cert"])
	}

	subject, ok := clientCert["subject"].(map[string]any)
	if !ok {
		t.Fatalf("expected subject map, got %#v", clientCert["subject"])
	}
	if subject["cn"] != "client.example.test" {
		t.Errorf("subject cn = %v, want client.example.test", subject["cn"])
	}
	if o, _ := subject["o"].([]string); len(o) != 1 || o[0] != "Radix Test Org" {
		t.Errorf("subject o = %#v, want [Radix Test Org]", subject["o"])
	}

	issuer, ok := clientCert["issuer"].(map[string]any)
	if !ok {
		t.Fatalf("expected issuer map, got %#v", clientCert["issuer"])
	}
	if issuer["cn"] != "Radix Test CA" {
		t.Errorf("issuer cn = %v, want Radix Test CA", issuer["cn"])
	}
	if o, _ := issuer["o"].([]string); len(o) != 1 || o[0] != "Radix CA Org" {
		t.Errorf("issuer o = %#v, want [Radix CA Org]", issuer["o"])
	}

	if got := clientCert["serial"]; got != cert.SerialNumber.String() {
		t.Errorf("serial = %v, want %v", got, cert.SerialNumber.String())
	}
	if got := clientCert["not_before"]; got != "2024-01-02T03:04:05Z" {
		t.Errorf("not_before = %v, want 2024-01-02T03:04:05Z", got)
	}
	if got := clientCert["not_after"]; got != "2025-01-02T03:04:05Z" {
		t.Errorf("not_after = %v, want 2025-01-02T03:04:05Z", got)
	}

	dns, _ := clientCert["dns_names"].([]string)
	if len(dns) != 2 || dns[0] != "client.example.test" || dns[1] != "alt.example.test" {
		t.Errorf("dns_names = %#v, want [client.example.test alt.example.test]", clientCert["dns_names"])
	}

	ips, _ := clientCert["ip_addresses"].([]string)
	if len(ips) != 2 || ips[0] != "192.0.2.10" || ips[1] != "2001:db8::1" {
		t.Errorf("ip_addresses = %#v, want [192.0.2.10 2001:db8::1]", clientCert["ip_addresses"])
	}
}

func TestTLSInfo_ClientCertAbsent(t *testing.T) {
	tests := []struct {
		name  string
		state *tls.ConnectionState
	}{
		{name: "nil state", state: nil},
		{
			name: "tls without peer certs",
			state: &tls.ConnectionState{
				Version:          tls.VersionTLS13,
				PeerCertificates: nil,
			},
		},
		{
			name: "tls with empty peer certs",
			state: &tls.ConnectionState{
				Version:          tls.VersionTLS13,
				PeerCertificates: []*x509.Certificate{},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			info := tlsInfo(tt.state)
			v, present := info["client_cert"]
			if !present {
				t.Fatalf("client_cert key should always be present, got %#v", info)
			}
			if v != nil {
				t.Errorf("expected client_cert=nil when no peer cert, got %#v", v)
			}
		})
	}
}

func TestEcho_TLSClientCertEndToEnd(t *testing.T) {
	clientCert := makeTestClientCert(t)
	state := &tls.ConnectionState{
		Version:          tls.VersionTLS13,
		CipherSuite:      tls.TLS_AES_128_GCM_SHA256,
		ServerName:       "example.test",
		PeerCertificates: []*x509.Certificate{clientCert},
	}

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.TLS = state

	rec := doEcho(defaultEchoConfig(), req)
	out := decodeEcho(t, rec)
	tlsSection, ok := out["tls"].(map[string]any)
	if !ok {
		t.Fatalf("tls section missing: %#v", out["tls"])
	}

	// After a JSON round-trip, sub-objects decode as map[string]any and string
	// slices as []any, so assert against the serialized shape clients observe.
	cc, ok := tlsSection["client_cert"].(map[string]any)
	if !ok {
		t.Fatalf("expected client_cert object in echoed JSON, got %#v", tlsSection["client_cert"])
	}
	subject := cc["subject"].(map[string]any)
	if subject["cn"] != "client.example.test" {
		t.Errorf("echoed subject cn = %v, want client.example.test", subject["cn"])
	}
	if cc["serial"] != clientCert.SerialNumber.String() {
		t.Errorf("echoed serial = %v, want %v", cc["serial"], clientCert.SerialNumber.String())
	}
	dns, _ := cc["dns_names"].([]any)
	if len(dns) != 2 || dns[0] != "client.example.test" {
		t.Errorf("echoed dns_names = %#v", cc["dns_names"])
	}
	ips, _ := cc["ip_addresses"].([]any)
	if len(ips) != 2 || ips[0] != "192.0.2.10" {
		t.Errorf("echoed ip_addresses = %#v", cc["ip_addresses"])
	}
}

func TestEcho_CustomResponseHeaders(t *testing.T) {
	cfg := defaultEchoConfig()
	cfg.Headers = []string{"X-Echo-Server: Radix", "X-Trace: 123"}

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := doEcho(cfg, req)

	if got := rec.Header().Get("X-Echo-Server"); got != "Radix" {
		t.Errorf("expected X-Echo-Server=Radix, got %q", got)
	}
	if got := rec.Header().Get("X-Trace"); got != "123" {
		t.Errorf("expected X-Trace=123, got %q", got)
	}
}

func TestEcho_InvalidJSONBodyYieldsNull(t *testing.T) {
	cfg := defaultEchoConfig()
	bad := `{not valid json`
	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(bad))
	req.Header.Set("Content-Type", "application/json")

	rec := doEcho(cfg, req)
	out := decodeEcho(t, rec)
	request := out["request"].(map[string]any)

	if request["body"] != nil {
		t.Errorf("expected body=null for invalid JSON, got %#v", request["body"])
	}
	if request["body_raw"] != bad {
		t.Errorf("expected body_raw preserved, got %v", request["body_raw"])
	}
}

func TestEcho_PrettyPrint(t *testing.T) {
	cfg := defaultEchoConfig()
	cfg.Pretty = true
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := doEcho(cfg, req)
	if !strings.Contains(rec.Body.String(), "\n  ") {
		t.Errorf("expected indented JSON when pretty=true, got %q", rec.Body.String())
	}
}

func TestDelayFromPath(t *testing.T) {
	tests := []struct {
		name    string
		path    string
		wantDur time.Duration
		wantOK  bool
	}{
		{"bare seconds", "/delay/2", 2 * time.Second, true},
		{"go duration ms", "/delay/500ms", 500 * time.Millisecond, true},
		{"bare seconds over cap", "/delay/100", maxPathDelay, true},
		{"huge finite overflow guard", "/delay/1e9", maxPathDelay, true},
		{"go duration over cap", "/delay/1h", maxPathDelay, true},
		{"negative rejected", "/delay/-1", 0, false},
		{"non-numeric rejected", "/delay/abc", 0, false},
		{"nan rejected", "/delay/NaN", 0, false},
		{"inf rejected", "/delay/Inf", 0, false},
		{"no match", "/notdelay/2", 0, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := delayFromPath(tt.path)
			if ok != tt.wantOK {
				t.Fatalf("path %q: ok = %v, want %v (got dur %v)", tt.path, ok, tt.wantOK, got)
			}
			if ok && got != tt.wantDur {
				t.Errorf("path %q: dur = %v, want %v", tt.path, got, tt.wantDur)
			}
		})
	}
}

func TestStatusFromPath(t *testing.T) {
	tests := []struct {
		name       string
		path       string
		wantStatus int
		wantOK     bool
	}{
		{"bare code in range", "/404", 404, true},
		{"status prefix in range", "/status/500", 500, true},
		{"out of range high", "/999", 0, false},
		{"out of range low not 3 digits", "/99", 0, false},
		{"non-numeric", "/abc", 0, false},
		{"no match extra segment", "/status/500/x", 0, false},
		{"upper bound 599", "/status/599", 599, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := statusFromPath(tt.path)
			if ok != tt.wantOK {
				t.Fatalf("path %q: ok = %v, want %v (got %d)", tt.path, ok, tt.wantOK, got)
			}
			if ok && got != tt.wantStatus {
				t.Errorf("path %q: status = %d, want %d", tt.path, got, tt.wantStatus)
			}
		})
	}
}

// errReader returns an error partway through to exercise the body_read_error path.
type errReader struct{}

func (errReader) Read(_ []byte) (int, error) { return 0, errors.New("simulated read failure") }
func (errReader) Close() error               { return nil }

func TestEcho_BodyReadErrorSurfaced(t *testing.T) {
	cfg := defaultEchoConfig()
	// No BodyLimit so the error is a plain read error, not a 413.
	cfg.BodyLimit = 0

	req := httptest.NewRequest(http.MethodPost, "/", nil)
	req.Body = errReader{}

	rec := doEcho(cfg, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 (error surfaced, not failed), got %d", rec.Code)
	}
	out := decodeEcho(t, rec)
	echoSection, ok := out["echo"].(map[string]any)
	if !ok {
		t.Fatalf("echo section missing: %#v", out["echo"])
	}
	msg, ok := echoSection["body_read_error"].(string)
	if !ok || !strings.Contains(msg, "simulated read failure") {
		t.Errorf("expected body_read_error to contain the read error, got %#v", echoSection["body_read_error"])
	}
}

func TestEcho_NoBodyReadErrorWhenClean(t *testing.T) {
	cfg := defaultEchoConfig()
	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader("hello"))
	rec := doEcho(cfg, req)
	out := decodeEcho(t, rec)
	echoSection := out["echo"].(map[string]any)
	if _, ok := echoSection["body_read_error"]; ok {
		t.Errorf("body_read_error should be absent on a clean read, got %#v", echoSection["body_read_error"])
	}
}

func TestEcho_DelayContextCancellation(t *testing.T) {
	cfg := defaultEchoConfig()
	cfg.Delay = 5 * time.Second // long enough that we'd notice if we waited

	ctx, cancel := context.WithCancel(context.Background())
	req := httptest.NewRequest(http.MethodGet, "/", nil).WithContext(ctx)

	// Cancel shortly after the request starts.
	go func() {
		time.Sleep(20 * time.Millisecond)
		cancel()
	}()

	rec := httptest.NewRecorder()
	start := time.Now()
	NewEchoHandler(cfg).ServeHTTP(rec, req)
	elapsed := time.Since(start)

	if elapsed >= cfg.Delay {
		t.Fatalf("expected ServeHTTP to return well before the %v delay, elapsed %v", cfg.Delay, elapsed)
	}
	if elapsed > time.Second {
		t.Errorf("expected prompt return on cancellation, elapsed %v", elapsed)
	}
	// On cancellation the handler returns before writing the echo body.
	if body := rec.Body.String(); strings.Contains(body, `"request"`) {
		t.Errorf("expected no echo body on cancellation, got %q", body)
	}
}

func TestEcho_HealthEndpointNotPartOfHandler(t *testing.T) {
	// The echo handler itself echoes everything; /_health is mounted on the mux
	// by the CLI. Here we verify the standalone mux wiring used by the command.
	mux := http.NewServeMux()
	mux.HandleFunc("/_health", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, `{"status":"ok"}`)
	})
	mux.Handle("/", NewEchoHandler(defaultEchoConfig()))

	req := httptest.NewRequest(http.MethodGet, "/_health", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 from /_health, got %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), `"status":"ok"`) {
		t.Errorf("expected health status ok, got %q", rec.Body.String())
	}
}
