package server

import (
	"crypto/tls"
	"encoding/json"
	"io"
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
