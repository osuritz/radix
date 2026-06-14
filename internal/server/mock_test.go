package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"
	"time"
)

// uuidV4Re matches an RFC 4122 version 4 UUID.
var uuidV4Re = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)

// defaultMockConfig returns a MockConfig with the built-ins enabled and no
// latency/chaos, matching the CLI defaults for the happy path.
func defaultMockConfig() MockConfig {
	return MockConfig{Builtin: true, FailStatus: http.StatusInternalServerError}
}

// doMock runs a request through a handler built from cfg and returns the recorder.
func doMock(cfg MockConfig, req *http.Request) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	NewMockHandler(cfg).ServeHTTP(rec, req)
	return rec
}

// decodeMock decodes the recorder body into a generic map, failing on error.
func decodeMock(t *testing.T, rec *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var out map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("failed to decode JSON: %v\nbody: %s", err, rec.Body.String())
	}
	return out
}

func TestMock_Get(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/get?foo=bar&foo=baz", nil)
	req.Header.Set("X-Custom", "abc")
	req.RemoteAddr = "203.0.113.7:54321"

	rec := doMock(defaultMockConfig(), req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("expected application/json, got %q", ct)
	}

	out := decodeMock(t, rec)
	if out["method"] != http.MethodGet {
		t.Errorf("expected method GET, got %v", out["method"])
	}
	if out["origin"] != "203.0.113.7" {
		t.Errorf("expected origin 203.0.113.7, got %v", out["origin"])
	}
	url, _ := out["url"].(string)
	if !strings.HasSuffix(url, "/get?foo=bar&foo=baz") {
		t.Errorf("expected url ending with the request URI, got %v", out["url"])
	}

	args, ok := out["args"].(map[string]any)
	if !ok {
		t.Fatalf("args missing: %#v", out["args"])
	}
	foo, ok := args["foo"].([]any)
	if !ok || len(foo) != 2 || foo[0] != "bar" || foo[1] != "baz" {
		t.Errorf("expected foo=[bar baz], got %#v", args["foo"])
	}

	headers, ok := out["headers"].(map[string]any)
	if !ok {
		t.Fatalf("headers missing: %#v", out["headers"])
	}
	custom, ok := headers["X-Custom"].([]any)
	if !ok || len(custom) != 1 || custom[0] != "abc" {
		t.Errorf("expected X-Custom=[abc], got %#v", headers["X-Custom"])
	}

	// GET responses must not include body fields.
	if _, ok := out["data"]; ok {
		t.Error("GET response should not include data field")
	}
}

func TestMock_PostJSON(t *testing.T) {
	payload := `{"name":"Ada","age":36}`
	req := httptest.NewRequest(http.MethodPost, "/post", strings.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")

	rec := doMock(defaultMockConfig(), req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	out := decodeMock(t, rec)

	if out["data"] != payload {
		t.Errorf("expected data=%q, got %v", payload, out["data"])
	}
	parsed, ok := out["json"].(map[string]any)
	if !ok {
		t.Fatalf("expected parsed json object, got %#v", out["json"])
	}
	if parsed["name"] != "Ada" {
		t.Errorf("expected json.name=Ada, got %v", parsed["name"])
	}
	if parsed["age"] != float64(36) {
		t.Errorf("expected json.age=36, got %v", parsed["age"])
	}
	if out["form"] != nil {
		t.Errorf("expected form=null for JSON body, got %#v", out["form"])
	}
}

func TestMock_PostForm(t *testing.T) {
	form := "name=Ada&role=admin&role=dev"
	req := httptest.NewRequest(http.MethodPost, "/post", strings.NewReader(form))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	rec := doMock(defaultMockConfig(), req)
	out := decodeMock(t, rec)

	formMap, ok := out["form"].(map[string]any)
	if !ok {
		t.Fatalf("expected parsed form, got %#v", out["form"])
	}
	role, ok := formMap["role"].([]any)
	if !ok || len(role) != 2 {
		t.Errorf("expected role with 2 values, got %#v", formMap["role"])
	}
	if out["json"] != nil {
		t.Errorf("expected json=null for form body, got %#v", out["json"])
	}
}

func TestMock_PutPatchDelete(t *testing.T) {
	for _, method := range []string{http.MethodPut, http.MethodPatch, http.MethodDelete} {
		t.Run(method, func(t *testing.T) {
			path := "/" + strings.ToLower(method)
			req := httptest.NewRequest(method, path, strings.NewReader("payload"))
			rec := doMock(defaultMockConfig(), req)
			if rec.Code != http.StatusOK {
				t.Fatalf("%s %s: expected 200, got %d", method, path, rec.Code)
			}
			out := decodeMock(t, rec)
			if out["method"] != method {
				t.Errorf("expected method %s, got %v", method, out["method"])
			}
			if out["data"] != "payload" {
				t.Errorf("expected data=payload, got %v", out["data"])
			}
		})
	}
}

func TestMock_Anything(t *testing.T) {
	tests := []struct {
		name   string
		method string
		path   string
	}{
		{"plain anything", http.MethodGet, "/anything"},
		{"subpath", "REPORT", "/anything/foo/bar"},
		{"post anything", http.MethodPost, "/anything/x"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(tt.method, tt.path, nil)
			rec := doMock(defaultMockConfig(), req)
			if rec.Code != http.StatusOK {
				t.Fatalf("%s %s: expected 200, got %d", tt.method, tt.path, rec.Code)
			}
			out := decodeMock(t, rec)
			if out["method"] != tt.method {
				t.Errorf("expected method %s, got %v", tt.method, out["method"])
			}
		})
	}
}

func TestMock_Headers(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/headers", nil)
	req.Header.Set("X-Test", "yes")
	rec := doMock(defaultMockConfig(), req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	out := decodeMock(t, rec)
	headers, ok := out["headers"].(map[string]any)
	if !ok {
		t.Fatalf("headers missing: %#v", out["headers"])
	}
	if _, ok := headers["X-Test"]; !ok {
		t.Errorf("expected X-Test header present, got %#v", headers)
	}
}

func TestMock_IP(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/ip", nil)
	req.RemoteAddr = "198.51.100.23:9999"
	rec := doMock(defaultMockConfig(), req)
	out := decodeMock(t, rec)
	if out["origin"] != "198.51.100.23" {
		t.Errorf("expected origin 198.51.100.23, got %v", out["origin"])
	}
}

func TestMock_UserAgent(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/user-agent", nil)
	req.Header.Set("User-Agent", "radix-test/1.0")
	rec := doMock(defaultMockConfig(), req)
	out := decodeMock(t, rec)
	if out["user-agent"] != "radix-test/1.0" {
		t.Errorf("expected user-agent radix-test/1.0, got %v", out["user-agent"])
	}
}

func TestMock_UUID(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/uuid", nil)
	rec := doMock(defaultMockConfig(), req)
	out := decodeMock(t, rec)
	u, _ := out["uuid"].(string)
	if !uuidV4Re.MatchString(u) {
		t.Errorf("expected a v4 UUID, got %q", u)
	}
}

func TestMock_Status(t *testing.T) {
	tests := []struct {
		name      string
		path      string
		wantCode  int
		wantOneOf []int
	}{
		{"single", "/status/418", http.StatusTeapot, nil},
		{"list", "/status/200,201", 0, []int{200, 201}},
		{"out of range", "/status/999", http.StatusBadRequest, nil},
		{"non-numeric", "/status/abc", http.StatusBadRequest, nil},
		{"low out of range", "/status/99", http.StatusBadRequest, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, tt.path, nil)
			rec := doMock(defaultMockConfig(), req)
			if tt.wantOneOf != nil {
				ok := false
				for _, c := range tt.wantOneOf {
					if rec.Code == c {
						ok = true
						break
					}
				}
				if !ok {
					t.Errorf("path %q: code %d not in %v", tt.path, rec.Code, tt.wantOneOf)
				}
				return
			}
			if rec.Code != tt.wantCode {
				t.Errorf("path %q: expected %d, got %d", tt.path, tt.wantCode, rec.Code)
			}
		})
	}
}

func TestMock_DelayFast(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/delay/0", nil)
	start := time.Now()
	rec := doMock(defaultMockConfig(), req)
	elapsed := time.Since(start)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	if elapsed > 500*time.Millisecond {
		t.Errorf("expected /delay/0 to be fast, elapsed %v", elapsed)
	}
	out := decodeMock(t, rec)
	if out["method"] != http.MethodGet {
		t.Errorf("expected /get-style body, got %#v", out)
	}
}

func TestMock_DelayInvalid(t *testing.T) {
	for _, path := range []string{"/delay/-1", "/delay/abc", "/delay/NaN", "/delay/Inf"} {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		rec := doMock(defaultMockConfig(), req)
		if rec.Code != http.StatusBadRequest {
			t.Errorf("path %q: expected 400, got %d", path, rec.Code)
		}
	}
}

func TestMockDelayFromValue(t *testing.T) {
	tests := []struct {
		name    string
		value   string
		wantDur time.Duration
		wantOK  bool
	}{
		{"bare seconds", "2", 2 * time.Second, true},
		{"go duration", "500ms", 500 * time.Millisecond, true},
		{"bare seconds over cap", "100", maxMockDelay, true},
		{"huge finite overflow guard", "1e9", maxMockDelay, true},
		{"go duration over cap", "1h", maxMockDelay, true},
		{"zero", "0", 0, true},
		{"negative rejected", "-1", 0, false},
		{"non-numeric rejected", "abc", 0, false},
		{"nan rejected", "NaN", 0, false},
		{"inf rejected", "Inf", 0, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := mockDelayFromValue(tt.value)
			if ok != tt.wantOK {
				t.Fatalf("value %q: ok=%v want %v (dur %v)", tt.value, ok, tt.wantOK, got)
			}
			if ok && got != tt.wantDur {
				t.Errorf("value %q: dur=%v want %v", tt.value, got, tt.wantDur)
			}
		})
	}
}

func TestStatusFromCodes(t *testing.T) {
	tests := []struct {
		name   string
		raw    string
		want   int
		wantOK bool
	}{
		{"single valid", "418", 418, true},
		{"lower bound", "100", 100, true},
		{"upper bound", "599", 599, true},
		{"too high", "600", 0, false},
		{"too low", "99", 0, false},
		{"non-numeric", "abc", 0, false},
		{"empty", "", 0, false},
		{"list with invalid", "200,abc", 0, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := statusFromCodes(tt.raw)
			if ok != tt.wantOK {
				t.Fatalf("raw %q: ok=%v want %v (got %d)", tt.raw, ok, tt.wantOK, got)
			}
			if ok && tt.want != 0 && got != tt.want {
				t.Errorf("raw %q: got %d want %d", tt.raw, got, tt.want)
			}
		})
	}
	// A list selects only from the provided codes.
	for i := 0; i < 50; i++ {
		got, ok := statusFromCodes("200,201")
		if !ok || (got != 200 && got != 201) {
			t.Fatalf("list selection out of range: got %d ok %v", got, ok)
		}
	}
}

func TestMock_JSON(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/json", nil)
	rec := doMock(defaultMockConfig(), req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("expected application/json, got %q", ct)
	}
	var out map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("expected valid JSON, got error %v: %s", err, rec.Body.String())
	}
}

func TestMock_HTML(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/html", nil)
	rec := doMock(defaultMockConfig(), req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/html") {
		t.Errorf("expected text/html, got %q", ct)
	}
	if !strings.Contains(rec.Body.String(), "<html") {
		t.Errorf("expected HTML body, got %q", rec.Body.String())
	}
}

func TestMock_XML(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/xml", nil)
	rec := doMock(defaultMockConfig(), req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "application/xml") {
		t.Errorf("expected application/xml, got %q", ct)
	}
	if !strings.Contains(rec.Body.String(), "<?xml") {
		t.Errorf("expected XML body, got %q", rec.Body.String())
	}
}

func TestMock_Bytes(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/bytes/16", nil)
	rec := doMock(defaultMockConfig(), req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/octet-stream" {
		t.Errorf("expected application/octet-stream, got %q", ct)
	}
	if cl := rec.Header().Get("Content-Length"); cl != "16" {
		t.Errorf("expected Content-Length 16, got %q", cl)
	}
	if got := rec.Body.Len(); got != 16 {
		t.Errorf("expected 16 bytes, got %d", got)
	}
}

func TestMock_BytesCapped(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/bytes/999999", nil)
	rec := doMock(defaultMockConfig(), req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	if got := rec.Body.Len(); got != maxMockBytes {
		t.Errorf("expected capped %d bytes, got %d", maxMockBytes, got)
	}
}

func TestMock_FailRateAlways(t *testing.T) {
	cfg := defaultMockConfig()
	cfg.FailRate = 100
	cfg.FailStatus = http.StatusServiceUnavailable

	for i := 0; i < 20; i++ {
		req := httptest.NewRequest(http.MethodGet, "/get", nil)
		rec := doMock(cfg, req)
		if rec.Code != http.StatusServiceUnavailable {
			t.Fatalf("expected always-fail with 503, got %d", rec.Code)
		}
	}
}

func TestMock_FailRateNever(t *testing.T) {
	cfg := defaultMockConfig()
	cfg.FailRate = 0

	for i := 0; i < 20; i++ {
		req := httptest.NewRequest(http.MethodGet, "/get", nil)
		rec := doMock(cfg, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("expected never-fail (200), got %d", rec.Code)
		}
	}
}

func TestMock_Latency(t *testing.T) {
	cfg := defaultMockConfig()
	cfg.Latency = 50 * time.Millisecond

	req := httptest.NewRequest(http.MethodGet, "/get", nil)
	start := time.Now()
	rec := doMock(cfg, req)
	elapsed := time.Since(start)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	if elapsed < 40*time.Millisecond {
		t.Errorf("expected latency of at least ~50ms, elapsed %v", elapsed)
	}
}

func TestMock_Prefix(t *testing.T) {
	cfg := defaultMockConfig()
	cfg.Prefix = "/_test"

	t.Run("prefixed path served", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/_test/get", nil)
		rec := doMock(cfg, req)
		if rec.Code != http.StatusOK {
			t.Errorf("expected 200 for /_test/get, got %d", rec.Code)
		}
	})

	t.Run("unprefixed path 404", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/get", nil)
		rec := doMock(cfg, req)
		if rec.Code != http.StatusNotFound {
			t.Errorf("expected 404 for /get with prefix set, got %d", rec.Code)
		}
	})
}

func TestMock_BuiltinDisabled(t *testing.T) {
	cfg := defaultMockConfig()
	cfg.Builtin = false

	req := httptest.NewRequest(http.MethodGet, "/get", nil)
	rec := doMock(cfg, req)
	if rec.Code != http.StatusNotFound {
		t.Errorf("expected 404 when builtin disabled, got %d", rec.Code)
	}
}

func TestNormalizePrefix(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		{"", ""},
		{"/", ""},
		{"/_test", "/_test"},
		{"_test", "/_test"},
		{"/_test/", "/_test"},
		{"  /api  ", "/api"},
	}
	for _, tt := range tests {
		if got := normalizePrefix(tt.in); got != tt.want {
			t.Errorf("normalizePrefix(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}
