package server

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/hex"
	"encoding/json"
	"io"
	"math/big"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	radixtls "github.com/osuritz/radix/internal/tls"
)

// makeRouteClientCert builds a self-signed certificate with a fixed validity
// window for exercising the mock routes' client-certificate template data and
// condition matching against a synthetic tls.ConnectionState. Self-signed means
// issuer == subject, which the issuer_cn/issuer_o assertions rely on. The
// organization is fixed to "Acme"; only the CN varies across tests.
func makeRouteClientCert(t *testing.T, cn string) *x509.Certificate {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(4242),
		Subject:      pkix.Name{CommonName: cn, Organization: []string{"Acme"}},
		NotBefore:    time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
		NotAfter:     time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatalf("create certificate: %v", err)
	}
	cert, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatalf("parse certificate: %v", err)
	}
	return cert
}

// tlsStateWithCert wraps a certificate in a synthetic connection state, as the
// net/http server would populate r.TLS under client auth.
func tlsStateWithCert(cert *x509.Certificate) *tls.ConnectionState {
	return &tls.ConnectionState{
		Version:          tls.VersionTLS13,
		CipherSuite:      tls.TLS_AES_128_GCM_SHA256,
		PeerCertificates: []*x509.Certificate{cert},
	}
}

func TestRoutes_TLSClientCertTemplateFields(t *testing.T) {
	const src = `
routes:
  - path: /whoami
    method: GET
    response:
      status: 200
      headers: { Content-Type: application/json }
      body: '{"cn":"{{.tls.client_cert.cn}}","o":"{{.tls.client_cert.o}}","serial":"{{.tls.client_cert.serial}}","not_before":"{{.tls.client_cert.not_before}}","not_after":"{{.tls.client_cert.not_after}}","fingerprint":"{{.tls.client_cert.fingerprint}}","issuer_cn":"{{.tls.client_cert.issuer_cn}}","issuer_o":"{{.tls.client_cert.issuer_o}}"}'
`
	cert := makeRouteClientCert(t, "client.example.test")
	req := httptest.NewRequest(http.MethodGet, "/whoami", nil)
	req.TLS = tlsStateWithCert(cert)

	rec := doRouted(t, src, false, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body %q)", rec.Code, rec.Body.String())
	}
	var out map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("invalid JSON body %q: %v", rec.Body.String(), err)
	}

	sum := sha256.Sum256(cert.Raw)
	want := map[string]string{
		"cn":          "client.example.test",
		"o":           "Acme",
		"serial":      "4242",
		"not_before":  "2026-01-01T00:00:00Z",
		"not_after":   "2027-01-01T00:00:00Z",
		"fingerprint": hex.EncodeToString(sum[:]),
		"issuer_cn":   "client.example.test", // self-signed: issuer == subject
		"issuer_o":    "Acme",
	}
	for k, w := range want {
		if out[k] != w {
			t.Errorf("%s = %q, want %q", k, out[k], w)
		}
	}
}

func TestRoutes_TLSClientCertTemplateEmptyWithoutCert(t *testing.T) {
	const src = `
routes:
  - path: /whoami
    response:
      status: 200
      body: 'cn=[{{.tls.client_cert.cn}}] fp=[{{.tls.client_cert.fingerprint}}] enabled={{.tls.enabled}} present={{.tls.client_cert_present}}'
`
	tests := []struct {
		name  string
		state *tls.ConnectionState
		want  string
	}{
		{
			name:  "plain http",
			state: nil,
			want:  "cn=[] fp=[] enabled=false present=false",
		},
		{
			name:  "tls without client cert",
			state: &tls.ConnectionState{Version: tls.VersionTLS13},
			want:  "cn=[] fp=[] enabled=true present=false",
		},
		{
			// Real crypto/tls states never contain nil entries, but the data
			// builder must not panic on a synthetic one.
			name:  "tls with nil first peer cert",
			state: &tls.ConnectionState{Version: tls.VersionTLS13, PeerCertificates: []*x509.Certificate{nil}},
			want:  "cn=[] fp=[] enabled=true present=false",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/whoami", nil)
			req.TLS = tt.state
			rec := doRouted(t, src, false, req)
			if rec.Code != http.StatusOK {
				t.Fatalf("status = %d, want 200 (body %q)", rec.Code, rec.Body.String())
			}
			if got := rec.Body.String(); got != tt.want {
				t.Errorf("body = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestRoutes_TLSConditionMatching(t *testing.T) {
	const src = `
routes:
  - path: /svc
    method: GET
    conditions:
      - match: { tls.cn: service-a }
        response: { status: 200, body: "service-a" }
      - match: { tls.cn: "*" }
        response: { status: 200, body: "some-cert" }
      - default: true
        response: { status: 401, body: "no-cert" }
`
	tests := []struct {
		name     string
		state    *tls.ConnectionState
		wantCode int
		wantBody string
	}{
		{
			name:     "exact CN match",
			state:    tlsStateWithCert(makeRouteClientCert(t, "service-a")),
			wantCode: http.StatusOK,
			wantBody: "service-a",
		},
		{
			name:     "other CN hits wildcard arm",
			state:    tlsStateWithCert(makeRouteClientCert(t, "service-b")),
			wantCode: http.StatusOK,
			wantBody: "some-cert",
		},
		{
			name:     "plain http falls to default arm",
			state:    nil,
			wantCode: http.StatusUnauthorized,
			wantBody: "no-cert",
		},
		{
			// A "*" wildcard must NOT match a certless TLS connection: the
			// always-populated empty template fields are not match targets.
			name:     "tls without cert falls to default arm",
			state:    &tls.ConnectionState{Version: tls.VersionTLS13},
			wantCode: http.StatusUnauthorized,
			wantBody: "no-cert",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/svc", nil)
			req.TLS = tt.state
			rec := doRouted(t, src, false, req)
			if rec.Code != tt.wantCode {
				t.Errorf("status = %d, want %d", rec.Code, tt.wantCode)
			}
			if got := rec.Body.String(); got != tt.wantBody {
				t.Errorf("body = %q, want %q", got, tt.wantBody)
			}
		})
	}
}

func TestRoutes_TLSConditionMatchesFingerprint(t *testing.T) {
	cert := makeRouteClientCert(t, "service-a")
	sum := sha256.Sum256(cert.Raw)
	src := `
routes:
  - path: /pinned
    conditions:
      - match: { tls.fingerprint: "` + hex.EncodeToString(sum[:]) + `" }
        response: { status: 200, body: "pinned" }
      - default: true
        response: { status: 403, body: "wrong-cert" }
`
	req := httptest.NewRequest(http.MethodGet, "/pinned", nil)
	req.TLS = tlsStateWithCert(cert)
	if rec := doRouted(t, src, false, req); rec.Code != http.StatusOK || rec.Body.String() != "pinned" {
		t.Errorf("pinned cert: status = %d body = %q, want 200 %q", rec.Code, rec.Body.String(), "pinned")
	}

	other := httptest.NewRequest(http.MethodGet, "/pinned", nil)
	other.TLS = tlsStateWithCert(makeRouteClientCert(t, "service-a")) // different key/DER
	if rec := doRouted(t, src, false, other); rec.Code != http.StatusForbidden {
		t.Errorf("other cert: status = %d, want 403", rec.Code)
	}
}

func TestRoutes_TLSMatchKeyValidation(t *testing.T) {
	tests := []struct {
		name    string
		key     string
		wantSub string
	}{
		{name: "unknown tls field", key: "tls.common_name", wantSub: "unknown tls field"},
		{name: "bare tls prefix rejected", key: "tls", wantSub: "must be prefixed"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			src := `
routes:
  - path: /x
    conditions:
      - match: { "` + tt.key + `": y }
        response: { status: 200 }
`
			_, err := CompileRoutes([]byte(src), t.TempDir())
			if err == nil {
				t.Fatalf("expected load error for match key %q, got nil", tt.key)
			}
			if !strings.Contains(err.Error(), tt.wantSub) {
				t.Errorf("error = %v, want substring %q", err, tt.wantSub)
			}
		})
	}
}

func TestRoutes_RequireClientCert(t *testing.T) {
	const src = `
routes:
  - path: /secure
    method: GET
    require_client_cert: true
    response: { status: 200, body: "top secret" }
`
	t.Run("without cert is 403 with JSON error", func(t *testing.T) {
		rec := doRouted(t, src, false, httptest.NewRequest(http.MethodGet, "/secure", nil))
		if rec.Code != http.StatusForbidden {
			t.Fatalf("status = %d, want 403", rec.Code)
		}
		if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
			t.Errorf("content-type = %q, want application/json", ct)
		}
		var out map[string]any
		if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
			t.Fatalf("expected JSON error body, got %q (err %v)", rec.Body.String(), err)
		}
		if _, ok := out["error"]; !ok {
			t.Errorf("expected error field in 403 body, got %#v", out)
		}
	})

	t.Run("with cert is served", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/secure", nil)
		req.TLS = tlsStateWithCert(makeRouteClientCert(t, "service-a"))
		rec := doRouted(t, src, false, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200", rec.Code)
		}
		if got := rec.Body.String(); got != "top secret" {
			t.Errorf("body = %q, want %q", got, "top secret")
		}
	})
}

// TestRoutes_ClientCertOverTLSIntegration exercises the full stack over a real
// TLS handshake: certificates generated by internal/tls, a server config built
// by the loader's optional client-auth mode (VerifyClientCertIfGiven), and the
// routed mock handler serving CN-matched conditions, a require_client_cert
// route, and certificate fields rendered into a response body.
func TestRoutes_ClientCertOverTLSIntegration(t *testing.T) {
	dir := t.TempDir()

	// CA, server cert, and a client cert with CN "service-a".
	ca, err := radixtls.GenerateCA(&radixtls.CertConfig{
		Organization: "Radix Mock Test", Days: 1, KeyType: radixtls.KeyTypeECDSA, IsCA: true,
	})
	if err != nil {
		t.Fatalf("GenerateCA: %v", err)
	}
	caX509, err := radixtls.ParseCertificate(ca.CertPEM)
	if err != nil {
		t.Fatalf("ParseCertificate(CA): %v", err)
	}
	caKey, err := radixtls.ParsePrivateKey(ca.KeyPEM)
	if err != nil {
		t.Fatalf("ParsePrivateKey(CA): %v", err)
	}
	serverCert, err := radixtls.GenerateServerCert(&radixtls.CertConfig{
		Hosts: []string{"127.0.0.1", "localhost"}, Days: 1,
		Organization: "Radix Mock Test", KeyType: radixtls.KeyTypeECDSA,
	}, caX509, caKey)
	if err != nil {
		t.Fatalf("GenerateServerCert: %v", err)
	}
	clientCert, err := radixtls.GenerateClientCert(&radixtls.CertConfig{
		Hosts: []string{"service-a"}, Days: 1,
		Organization: "Radix Clients", KeyType: radixtls.KeyTypeECDSA, IsClient: true,
	}, caX509, caKey)
	if err != nil {
		t.Fatalf("GenerateClientCert: %v", err)
	}

	writeFile := func(name string, data []byte) string {
		t.Helper()
		p := filepath.Join(dir, name)
		if wErr := os.WriteFile(p, data, 0o600); wErr != nil {
			t.Fatalf("write %s: %v", name, wErr)
		}
		return p
	}
	certPath := writeFile("cert.pem", serverCert.CertPEM)
	keyPath := writeFile("key.pem", serverCert.KeyPEM)
	caPath := writeFile("ca.pem", ca.CertPEM)

	// Optional client auth: a presented cert is verified against the CA, but a
	// certless connection still completes the handshake so the mock's per-route
	// rules can answer 401/403 at the HTTP layer.
	serverTLS, err := radixtls.NewServerTLSConfig(radixtls.ServerTLSOptions{
		CertFile: certPath, KeyFile: keyPath, CAFile: caPath,
		ClientAuthOptional: true, MinVersion: "1.2",
	})
	if err != nil {
		t.Fatalf("NewServerTLSConfig: %v", err)
	}

	const src = `
routes:
  - path: /whoami
    method: GET
    conditions:
      - match: { tls.cn: service-a }
        response: { status: 200, body: 'hello {{.tls.client_cert.cn}} ({{.tls.client_cert.o}})' }
      - default: true
        response: { status: 401, body: "unknown client" }
  - path: /secure
    method: GET
    require_client_cert: true
    response: { status: 200, body: 'sha256={{.tls.client_cert.fingerprint}}' }
`
	store := newStore(t, src, dir)
	ts := httptest.NewUnstartedServer(NewRoutedHandler(RoutedHandlerConfig{Store: store}))
	ts.TLS = serverTLS
	ts.StartTLS()
	defer ts.Close()

	caPool := x509.NewCertPool()
	if !caPool.AppendCertsFromPEM(ca.CertPEM) {
		t.Fatal("failed to add CA to pool")
	}
	clientPair, err := tls.X509KeyPair(clientCert.CertPEM, clientCert.KeyPEM)
	if err != nil {
		t.Fatalf("X509KeyPair(client): %v", err)
	}
	withCert := &http.Client{Transport: &http.Transport{TLSClientConfig: &tls.Config{
		RootCAs: caPool, Certificates: []tls.Certificate{clientPair}, MinVersion: tls.VersionTLS12,
	}}}
	withoutCert := &http.Client{Transport: &http.Transport{TLSClientConfig: &tls.Config{
		RootCAs: caPool, MinVersion: tls.VersionTLS12,
	}}}

	get := func(c *http.Client, path string) (int, string) {
		t.Helper()
		resp, gErr := c.Get(ts.URL + path)
		if gErr != nil {
			t.Fatalf("GET %s: %v", path, gErr)
		}
		defer func() { _ = resp.Body.Close() }()
		body, rErr := io.ReadAll(resp.Body)
		if rErr != nil {
			t.Fatalf("read body of %s: %v", path, rErr)
		}
		return resp.StatusCode, string(body)
	}

	if code, body := get(withCert, "/whoami"); code != http.StatusOK || body != "hello service-a (Radix Clients)" {
		t.Errorf("with cert /whoami = %d %q, want 200 %q", code, body, "hello service-a (Radix Clients)")
	}
	if code, body := get(withoutCert, "/whoami"); code != http.StatusUnauthorized || body != "unknown client" {
		t.Errorf("without cert /whoami = %d %q, want 401 %q", code, body, "unknown client")
	}
	if code, body := get(withoutCert, "/secure"); code != http.StatusForbidden || !strings.Contains(body, "client certificate required") {
		t.Errorf("without cert /secure = %d %q, want 403 with client-certificate error", code, body)
	}

	parsedClient, err := radixtls.ParseCertificate(clientCert.CertPEM)
	if err != nil {
		t.Fatalf("ParseCertificate(client): %v", err)
	}
	sum := sha256.Sum256(parsedClient.Raw)
	if code, body := get(withCert, "/secure"); code != http.StatusOK || body != "sha256="+hex.EncodeToString(sum[:]) {
		t.Errorf("with cert /secure = %d %q, want 200 with fingerprint %s", code, body, hex.EncodeToString(sum[:]))
	}
}

func TestCompiledRoutes_HasClientCertRequirements(t *testing.T) {
	tests := []struct {
		name string
		yaml string
		want bool
	}{
		{
			name: "require_client_cert route",
			yaml: `
routes:
  - path: /secure
    require_client_cert: true
    response: { status: 200, body: ok }
`,
			want: true,
		},
		{
			name: "tls condition match",
			yaml: `
routes:
  - path: /whoami
    conditions:
      - match: { tls.cn: service-a }
        response: { status: 200, body: hi }
      - default: true
        response: { status: 401, body: nope }
`,
			want: true,
		},
		{
			name: "no client-cert dependence",
			yaml: `
routes:
  - path: /open
    conditions:
      - match: { headers.X-Token: secret }
        response: { status: 200, body: ok }
    response: { status: 401, body: nope }
`,
			want: false,
		},
		{
			name: "no routes",
			yaml: `
settings:
  latency: 0
routes: []
`,
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			compiled, err := CompileRoutes([]byte(tt.yaml), t.TempDir())
			if err != nil {
				t.Fatalf("CompileRoutes: %v", err)
			}
			if got := compiled.HasClientCertRequirements(); got != tt.want {
				t.Errorf("HasClientCertRequirements() = %v, want %v", got, tt.want)
			}
		})
	}
}
