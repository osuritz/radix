package tls

import (
	cryptotls "crypto/tls"
	"crypto/x509"
	"os"
	"path/filepath"
	"testing"
)

// testCA holds a generated CA for use across tests.
type testCA struct {
	cert    *Certificate
	x509CA  *x509.Certificate
	certPEM []byte
}

// generateTestCA creates a CA certificate for test use.
func generateTestCA(t *testing.T, keyType KeyType) *testCA {
	t.Helper()

	cfg := &CertConfig{
		Organization: "Radix Test",
		Days:         1,
		KeyType:      keyType,
		KeySize:      2048,
		ECDSACurve:   CurveP256,
		IsCA:         true,
	}

	ca, err := GenerateCA(cfg)
	if err != nil {
		t.Fatalf("GenerateCA: %v", err)
	}

	x509CA, err := ParseCertificate(ca.CertPEM)
	if err != nil {
		t.Fatalf("ParseCertificate: %v", err)
	}

	return &testCA{cert: ca, x509CA: x509CA, certPEM: ca.CertPEM}
}

// writeTestCert writes a certificate and key to the given directory and returns
// the file paths.
func writeTestCert(t *testing.T, dir string, cert *Certificate) (certPath, keyPath string) {
	t.Helper()

	certPath = filepath.Join(dir, "cert.pem")
	keyPath = filepath.Join(dir, "key.pem")

	if err := os.WriteFile(certPath, cert.CertPEM, 0o644); err != nil {
		t.Fatalf("writing cert file: %v", err)
	}
	// #nosec G306
	if err := os.WriteFile(keyPath, cert.KeyPEM, 0o600); err != nil {
		t.Fatalf("writing key file: %v", err)
	}

	return certPath, keyPath
}

// writeCAFile writes a CA certificate PEM to the given directory and returns
// the file path.
func writeCAFile(t *testing.T, dir string, caPEM []byte) string {
	t.Helper()

	caPath := filepath.Join(dir, "ca.pem")
	if err := os.WriteFile(caPath, caPEM, 0o644); err != nil {
		t.Fatalf("writing CA file: %v", err)
	}
	return caPath
}

// generateSignedServerCert generates a server cert signed by the given CA.
func generateSignedServerCert(t *testing.T, ca *testCA, keyType KeyType) *Certificate {
	t.Helper()

	caKey, err := ParsePrivateKey(ca.cert.KeyPEM)
	if err != nil {
		t.Fatalf("ParsePrivateKey: %v", err)
	}

	cfg := &CertConfig{
		Hosts:        []string{"localhost", "127.0.0.1"},
		Days:         1,
		Organization: "Radix Test",
		KeyType:      keyType,
		KeySize:      2048,
		ECDSACurve:   CurveP256,
	}

	cert, err := GenerateServerCert(cfg, ca.x509CA, caKey)
	if err != nil {
		t.Fatalf("GenerateServerCert: %v", err)
	}

	return cert
}

// generateSignedClientCert generates a client cert signed by the given CA.
func generateSignedClientCert(t *testing.T, ca *testCA, keyType KeyType) *Certificate {
	t.Helper()

	caKey, err := ParsePrivateKey(ca.cert.KeyPEM)
	if err != nil {
		t.Fatalf("ParsePrivateKey: %v", err)
	}

	cfg := &CertConfig{
		Hosts:        []string{"client"},
		Days:         1,
		Organization: "Radix Test Client",
		KeyType:      keyType,
		KeySize:      2048,
		ECDSACurve:   CurveP256,
		IsClient:     true,
	}

	cert, err := GenerateClientCert(cfg, ca.x509CA, caKey)
	if err != nil {
		t.Fatalf("GenerateClientCert: %v", err)
	}

	return cert
}

func TestParseMinVersion(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		version string
		want    uint16
		wantErr bool
	}{
		{
			name:    "TLS 1.2",
			version: "1.2",
			want:    cryptotls.VersionTLS12,
		},
		{
			name:    "TLS 1.3",
			version: "1.3",
			want:    cryptotls.VersionTLS13,
		},
		{
			name:    "empty defaults to 1.2",
			version: "",
			want:    cryptotls.VersionTLS12,
		},
		{
			name:    "invalid version",
			version: "1.1",
			wantErr: true,
		},
		{
			name:    "garbage input",
			version: "abc",
			wantErr: true,
		},
		{
			name:    "numeric only",
			version: "13",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := ParseMinVersion(tt.version)
			if tt.wantErr {
				if err == nil {
					t.Errorf("ParseMinVersion(%q) expected error, got nil", tt.version)
				}
				return
			}
			if err != nil {
				t.Errorf("ParseMinVersion(%q) unexpected error: %v", tt.version, err)
				return
			}
			if got != tt.want {
				t.Errorf("ParseMinVersion(%q) = %d, want %d", tt.version, got, tt.want)
			}
		})
	}
}

func TestNewServerTLSConfig(t *testing.T) {
	t.Parallel()

	t.Run("valid RSA server config", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()
		ca := generateTestCA(t, KeyTypeRSA)
		serverCert := generateSignedServerCert(t, ca, KeyTypeRSA)
		certPath, keyPath := writeTestCert(t, dir, serverCert)

		cfg := ServerTLSOptions{
			CertFile:   certPath,
			KeyFile:    keyPath,
			MinVersion: "1.2",
		}

		tlsCfg, err := NewServerTLSConfig(cfg)
		if err != nil {
			t.Fatalf("NewServerTLSConfig: %v", err)
		}

		if tlsCfg.MinVersion != cryptotls.VersionTLS12 {
			t.Errorf("MinVersion = %d, want %d", tlsCfg.MinVersion, cryptotls.VersionTLS12)
		}
		if len(tlsCfg.Certificates) != 1 {
			t.Errorf("Certificates length = %d, want 1", len(tlsCfg.Certificates))
		}
		if len(tlsCfg.CipherSuites) == 0 {
			t.Error("CipherSuites should be set for TLS 1.2")
		}
		if tlsCfg.ClientAuth != cryptotls.NoClientCert {
			t.Errorf("ClientAuth = %d, want NoClientCert", tlsCfg.ClientAuth)
		}
	})

	t.Run("valid ECDSA server config", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()
		ca := generateTestCA(t, KeyTypeECDSA)
		serverCert := generateSignedServerCert(t, ca, KeyTypeECDSA)
		certPath, keyPath := writeTestCert(t, dir, serverCert)

		cfg := ServerTLSOptions{
			CertFile:   certPath,
			KeyFile:    keyPath,
			MinVersion: "1.2",
		}

		tlsCfg, err := NewServerTLSConfig(cfg)
		if err != nil {
			t.Fatalf("NewServerTLSConfig: %v", err)
		}

		if len(tlsCfg.Certificates) != 1 {
			t.Errorf("Certificates length = %d, want 1", len(tlsCfg.Certificates))
		}
	})

	t.Run("TLS 1.3 minimum", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()
		ca := generateTestCA(t, KeyTypeRSA)
		serverCert := generateSignedServerCert(t, ca, KeyTypeRSA)
		certPath, keyPath := writeTestCert(t, dir, serverCert)

		cfg := ServerTLSOptions{
			CertFile:   certPath,
			KeyFile:    keyPath,
			MinVersion: "1.3",
		}

		tlsCfg, err := NewServerTLSConfig(cfg)
		if err != nil {
			t.Fatalf("NewServerTLSConfig: %v", err)
		}

		if tlsCfg.MinVersion != cryptotls.VersionTLS13 {
			t.Errorf("MinVersion = %d, want %d", tlsCfg.MinVersion, cryptotls.VersionTLS13)
		}
		if tlsCfg.CipherSuites != nil {
			t.Error("CipherSuites should be nil for TLS 1.3 (Go uses optimal defaults)")
		}
	})

	t.Run("mTLS with client CA", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()
		ca := generateTestCA(t, KeyTypeRSA)
		serverCert := generateSignedServerCert(t, ca, KeyTypeRSA)
		certPath, keyPath := writeTestCert(t, dir, serverCert)
		caPath := writeCAFile(t, dir, ca.certPEM)

		cfg := ServerTLSOptions{
			CertFile:   certPath,
			KeyFile:    keyPath,
			CAFile:     caPath,
			ClientAuth: true,
			MinVersion: "1.2",
		}

		tlsCfg, err := NewServerTLSConfig(cfg)
		if err != nil {
			t.Fatalf("NewServerTLSConfig: %v", err)
		}

		if tlsCfg.ClientAuth != cryptotls.RequireAndVerifyClientCert {
			t.Errorf("ClientAuth = %d, want RequireAndVerifyClientCert", tlsCfg.ClientAuth)
		}
		if tlsCfg.ClientCAs == nil {
			t.Error("ClientCAs should be set when CAFile is provided")
		}
	})

	t.Run("missing cert file", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()
		cfg := ServerTLSOptions{
			CertFile:   filepath.Join(dir, "nonexistent.pem"),
			KeyFile:    filepath.Join(dir, "key.pem"),
			MinVersion: "1.2",
		}

		_, err := NewServerTLSConfig(cfg)
		if err == nil {
			t.Fatal("expected error for missing cert file")
		}
	})

	t.Run("missing key file", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()
		ca := generateTestCA(t, KeyTypeRSA)
		serverCert := generateSignedServerCert(t, ca, KeyTypeRSA)
		certPath, _ := writeTestCert(t, dir, serverCert)

		cfg := ServerTLSOptions{
			CertFile:   certPath,
			KeyFile:    filepath.Join(dir, "nonexistent-key.pem"),
			MinVersion: "1.2",
		}

		_, err := NewServerTLSConfig(cfg)
		if err == nil {
			t.Fatal("expected error for missing key file")
		}
	})

	t.Run("invalid PEM cert content", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()
		certPath := filepath.Join(dir, "bad-cert.pem")
		keyPath := filepath.Join(dir, "key.pem")

		if err := os.WriteFile(certPath, []byte("not a PEM file"), 0o644); err != nil {
			t.Fatalf("writing bad cert: %v", err)
		}
		if err := os.WriteFile(keyPath, []byte("not a PEM file"), 0o600); err != nil {
			t.Fatalf("writing bad key: %v", err)
		}

		cfg := ServerTLSOptions{
			CertFile:   certPath,
			KeyFile:    keyPath,
			MinVersion: "1.2",
		}

		_, err := NewServerTLSConfig(cfg)
		if err == nil {
			t.Fatal("expected error for invalid PEM content")
		}
	})

	t.Run("empty cert file path", func(t *testing.T) {
		t.Parallel()

		cfg := ServerTLSOptions{
			KeyFile:    "/some/key.pem",
			MinVersion: "1.2",
		}

		_, err := NewServerTLSConfig(cfg)
		if err == nil {
			t.Fatal("expected error for empty cert file path")
		}
	})

	t.Run("empty key file path", func(t *testing.T) {
		t.Parallel()

		cfg := ServerTLSOptions{
			CertFile:   "/some/cert.pem",
			MinVersion: "1.2",
		}

		_, err := NewServerTLSConfig(cfg)
		if err == nil {
			t.Fatal("expected error for empty key file path")
		}
	})

	t.Run("invalid min version", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()
		ca := generateTestCA(t, KeyTypeRSA)
		serverCert := generateSignedServerCert(t, ca, KeyTypeRSA)
		certPath, keyPath := writeTestCert(t, dir, serverCert)

		cfg := ServerTLSOptions{
			CertFile:   certPath,
			KeyFile:    keyPath,
			MinVersion: "1.1",
		}

		_, err := NewServerTLSConfig(cfg)
		if err == nil {
			t.Fatal("expected error for invalid min version")
		}
	})

	t.Run("default min version when empty", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()
		ca := generateTestCA(t, KeyTypeRSA)
		serverCert := generateSignedServerCert(t, ca, KeyTypeRSA)
		certPath, keyPath := writeTestCert(t, dir, serverCert)

		cfg := ServerTLSOptions{
			CertFile: certPath,
			KeyFile:  keyPath,
			// MinVersion intentionally empty
		}

		tlsCfg, err := NewServerTLSConfig(cfg)
		if err != nil {
			t.Fatalf("NewServerTLSConfig: %v", err)
		}

		if tlsCfg.MinVersion != cryptotls.VersionTLS12 {
			t.Errorf("MinVersion = %d, want %d (TLS 1.2 default)", tlsCfg.MinVersion, cryptotls.VersionTLS12)
		}
	})

	t.Run("client auth without CA file", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()
		ca := generateTestCA(t, KeyTypeRSA)
		serverCert := generateSignedServerCert(t, ca, KeyTypeRSA)
		certPath, keyPath := writeTestCert(t, dir, serverCert)

		cfg := ServerTLSOptions{
			CertFile:   certPath,
			KeyFile:    keyPath,
			ClientAuth: true,
			MinVersion: "1.2",
		}

		tlsCfg, err := NewServerTLSConfig(cfg)
		if err != nil {
			t.Fatalf("NewServerTLSConfig: %v", err)
		}

		if tlsCfg.ClientAuth != cryptotls.RequireAndVerifyClientCert {
			t.Errorf("ClientAuth = %d, want RequireAndVerifyClientCert", tlsCfg.ClientAuth)
		}
		if tlsCfg.ClientCAs != nil {
			t.Error("ClientCAs should be nil when no CAFile is provided")
		}
	})
}

func TestNewClientTLSConfig(t *testing.T) {
	t.Parallel()

	t.Run("basic client config", func(t *testing.T) {
		t.Parallel()

		cfg := ClientTLSOptions{
			MinVersion: "1.2",
		}

		tlsCfg, err := NewClientTLSConfig(&cfg)
		if err != nil {
			t.Fatalf("NewClientTLSConfig: %v", err)
		}

		if tlsCfg.MinVersion != cryptotls.VersionTLS12 {
			t.Errorf("MinVersion = %d, want %d", tlsCfg.MinVersion, cryptotls.VersionTLS12)
		}
		if tlsCfg.InsecureSkipVerify {
			t.Error("InsecureSkipVerify should be false by default")
		}
	})

	t.Run("skip verify", func(t *testing.T) {
		t.Parallel()

		cfg := ClientTLSOptions{
			SkipVerify: true,
			MinVersion: "1.2",
		}

		tlsCfg, err := NewClientTLSConfig(&cfg)
		if err != nil {
			t.Fatalf("NewClientTLSConfig: %v", err)
		}

		if !tlsCfg.InsecureSkipVerify {
			t.Error("InsecureSkipVerify should be true")
		}
	})

	t.Run("custom CA", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()
		ca := generateTestCA(t, KeyTypeRSA)
		caPath := writeCAFile(t, dir, ca.certPEM)

		cfg := ClientTLSOptions{
			CAFile:     caPath,
			MinVersion: "1.2",
		}

		tlsCfg, err := NewClientTLSConfig(&cfg)
		if err != nil {
			t.Fatalf("NewClientTLSConfig: %v", err)
		}

		if tlsCfg.RootCAs == nil {
			t.Error("RootCAs should be set when CAFile is provided")
		}
	})

	t.Run("client cert for mTLS", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()
		ca := generateTestCA(t, KeyTypeRSA)
		clientCert := generateSignedClientCert(t, ca, KeyTypeRSA)
		certPath, keyPath := writeTestCert(t, dir, clientCert)

		cfg := ClientTLSOptions{
			CertFile:   certPath,
			KeyFile:    keyPath,
			MinVersion: "1.2",
		}

		tlsCfg, err := NewClientTLSConfig(&cfg)
		if err != nil {
			t.Fatalf("NewClientTLSConfig: %v", err)
		}

		if len(tlsCfg.Certificates) != 1 {
			t.Errorf("Certificates length = %d, want 1", len(tlsCfg.Certificates))
		}
	})

	t.Run("client cert without key errors", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()
		ca := generateTestCA(t, KeyTypeRSA)
		clientCert := generateSignedClientCert(t, ca, KeyTypeRSA)
		certPath, _ := writeTestCert(t, dir, clientCert)

		cfg := ClientTLSOptions{
			CertFile:   certPath,
			MinVersion: "1.2",
		}

		_, err := NewClientTLSConfig(&cfg)
		if err == nil {
			t.Fatal("expected error when cert is provided without key")
		}
	})

	t.Run("client key without cert errors", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()
		ca := generateTestCA(t, KeyTypeRSA)
		clientCert := generateSignedClientCert(t, ca, KeyTypeRSA)
		_, keyPath := writeTestCert(t, dir, clientCert)

		cfg := ClientTLSOptions{
			KeyFile:    keyPath,
			MinVersion: "1.2",
		}

		_, err := NewClientTLSConfig(&cfg)
		if err == nil {
			t.Fatal("expected error when key is provided without cert")
		}
	})

	t.Run("server name override", func(t *testing.T) {
		t.Parallel()

		cfg := ClientTLSOptions{
			ServerName: "custom.example.com",
			MinVersion: "1.2",
		}

		tlsCfg, err := NewClientTLSConfig(&cfg)
		if err != nil {
			t.Fatalf("NewClientTLSConfig: %v", err)
		}

		if tlsCfg.ServerName != "custom.example.com" {
			t.Errorf("ServerName = %q, want %q", tlsCfg.ServerName, "custom.example.com")
		}
	})

	t.Run("TLS 1.3 minimum", func(t *testing.T) {
		t.Parallel()

		cfg := ClientTLSOptions{
			MinVersion: "1.3",
		}

		tlsCfg, err := NewClientTLSConfig(&cfg)
		if err != nil {
			t.Fatalf("NewClientTLSConfig: %v", err)
		}

		if tlsCfg.MinVersion != cryptotls.VersionTLS13 {
			t.Errorf("MinVersion = %d, want %d", tlsCfg.MinVersion, cryptotls.VersionTLS13)
		}
		if tlsCfg.CipherSuites != nil {
			t.Error("CipherSuites should be nil for TLS 1.3")
		}
	})

	t.Run("invalid min version", func(t *testing.T) {
		t.Parallel()

		cfg := ClientTLSOptions{
			MinVersion: "1.0",
		}

		_, err := NewClientTLSConfig(&cfg)
		if err == nil {
			t.Fatal("expected error for invalid min version")
		}
	})

	t.Run("missing client cert file", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()

		cfg := ClientTLSOptions{
			CertFile:   filepath.Join(dir, "nonexistent.pem"),
			KeyFile:    filepath.Join(dir, "nonexistent-key.pem"),
			MinVersion: "1.2",
		}

		_, err := NewClientTLSConfig(&cfg)
		if err == nil {
			t.Fatal("expected error for missing client cert file")
		}
	})

	t.Run("invalid CA file", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()
		badCA := filepath.Join(dir, "bad-ca.pem")
		if err := os.WriteFile(badCA, []byte("not a cert"), 0o644); err != nil {
			t.Fatalf("writing bad CA: %v", err)
		}

		cfg := ClientTLSOptions{
			CAFile:     badCA,
			MinVersion: "1.2",
		}

		_, err := NewClientTLSConfig(&cfg)
		if err == nil {
			t.Fatal("expected error for invalid CA file")
		}
	})

	t.Run("ECDSA client cert for mTLS", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()
		ca := generateTestCA(t, KeyTypeECDSA)
		clientCert := generateSignedClientCert(t, ca, KeyTypeECDSA)
		certPath, keyPath := writeTestCert(t, dir, clientCert)

		cfg := ClientTLSOptions{
			CertFile:   certPath,
			KeyFile:    keyPath,
			MinVersion: "1.2",
		}

		tlsCfg, err := NewClientTLSConfig(&cfg)
		if err != nil {
			t.Fatalf("NewClientTLSConfig: %v", err)
		}

		if len(tlsCfg.Certificates) != 1 {
			t.Errorf("Certificates length = %d, want 1", len(tlsCfg.Certificates))
		}
	})
}

func TestValidatePEMFile(t *testing.T) {
	t.Parallel()

	t.Run("valid certificate PEM", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()
		ca := generateTestCA(t, KeyTypeRSA)
		caPath := writeCAFile(t, dir, ca.certPEM)

		if err := validatePEMFile(caPath, "CERTIFICATE"); err != nil {
			t.Errorf("validatePEMFile: unexpected error: %v", err)
		}
	})

	t.Run("wrong PEM type", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()
		ca := generateTestCA(t, KeyTypeRSA)
		// Write the key but expect CERTIFICATE type
		keyPath := filepath.Join(dir, "key.pem")
		if err := os.WriteFile(keyPath, ca.cert.KeyPEM, 0o600); err != nil {
			t.Fatalf("writing key: %v", err)
		}

		err := validatePEMFile(keyPath, "CERTIFICATE")
		if err == nil {
			t.Fatal("expected error for wrong PEM type")
		}
	})

	t.Run("nonexistent file", func(t *testing.T) {
		t.Parallel()

		err := validatePEMFile("/nonexistent/path/cert.pem", "CERTIFICATE")
		if err == nil {
			t.Fatal("expected error for nonexistent file")
		}
	})

	t.Run("non-PEM content", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()
		badPath := filepath.Join(dir, "bad.pem")
		if err := os.WriteFile(badPath, []byte("just some text"), 0o644); err != nil {
			t.Fatalf("writing bad file: %v", err)
		}

		err := validatePEMFile(badPath, "CERTIFICATE")
		if err == nil {
			t.Fatal("expected error for non-PEM content")
		}
	})

	t.Run("any PEM type accepted when expected is empty", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()
		ca := generateTestCA(t, KeyTypeRSA)
		keyPath := filepath.Join(dir, "key.pem")
		if err := os.WriteFile(keyPath, ca.cert.KeyPEM, 0o600); err != nil {
			t.Fatalf("writing key: %v", err)
		}

		if err := validatePEMFile(keyPath, ""); err != nil {
			t.Errorf("validatePEMFile with empty expectedType: unexpected error: %v", err)
		}
	})
}

func TestDefaultCipherSuites(t *testing.T) {
	t.Parallel()

	suites := defaultCipherSuites()

	if len(suites) != 6 {
		t.Fatalf("defaultCipherSuites() returned %d suites, want 6", len(suites))
	}

	expected := []uint16{
		cryptotls.TLS_ECDHE_ECDSA_WITH_AES_256_GCM_SHA384,
		cryptotls.TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384,
		cryptotls.TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256,
		cryptotls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256,
		cryptotls.TLS_ECDHE_ECDSA_WITH_CHACHA20_POLY1305,
		cryptotls.TLS_ECDHE_RSA_WITH_CHACHA20_POLY1305,
	}

	for i, suite := range suites {
		if suite != expected[i] {
			t.Errorf("suite[%d] = %#x, want %#x", i, suite, expected[i])
		}
	}
}
