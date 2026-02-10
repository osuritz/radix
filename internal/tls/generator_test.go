package tls

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"
)

// helperGenerateCA is a test helper that generates a CA certificate and parses it
// for use in tests that need a signing CA. It fails the test on error.
func helperGenerateCA(t *testing.T, cfg *CertConfig) (*x509.Certificate, interface{}) {
	t.Helper()
	caCert, err := GenerateCA(cfg)
	if err != nil {
		t.Fatalf("GenerateCA() unexpected error: %v", err)
	}
	parsedCA, err := ParseCertificate(caCert.CertPEM)
	if err != nil {
		t.Fatalf("ParseCertificate(CA) unexpected error: %v", err)
	}
	caKey, err := ParsePrivateKey(caCert.KeyPEM)
	if err != nil {
		t.Fatalf("ParsePrivateKey(CA) unexpected error: %v", err)
	}
	return parsedCA, caKey
}

func TestGenerateCA(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		cfg          *CertConfig
		wantKeyType  string // "rsa" or "ecdsa"
		wantKeySize  int    // RSA bit size or 0 for ECDSA
		wantCurve    elliptic.Curve
		wantOrg      string
		wantCN       string
		wantDayRange int
	}{
		{
			name: "RSA 2048",
			cfg: &CertConfig{
				Days:         365,
				Organization: "Test Org",
				KeyType:      KeyTypeRSA,
				KeySize:      2048,
				IsCA:         true,
			},
			wantKeyType:  "rsa",
			wantKeySize:  2048,
			wantOrg:      "Test Org",
			wantCN:       "Test Org CA",
			wantDayRange: 365,
		},
		{
			name: "RSA 4096",
			cfg: &CertConfig{
				Days:         730,
				Organization: "Big Key Org",
				KeyType:      KeyTypeRSA,
				KeySize:      4096,
				IsCA:         true,
			},
			wantKeyType:  "rsa",
			wantKeySize:  4096,
			wantOrg:      "Big Key Org",
			wantCN:       "Big Key Org CA",
			wantDayRange: 730,
		},
		{
			name: "ECDSA P-256",
			cfg: &CertConfig{
				Days:         365,
				Organization: "EC Org",
				KeyType:      KeyTypeECDSA,
				ECDSACurve:   CurveP256,
				IsCA:         true,
			},
			wantKeyType:  "ecdsa",
			wantCurve:    elliptic.P256(),
			wantOrg:      "EC Org",
			wantCN:       "EC Org CA",
			wantDayRange: 365,
		},
		{
			name: "ECDSA P-384",
			cfg: &CertConfig{
				Days:         365,
				Organization: "EC384 Org",
				KeyType:      KeyTypeECDSA,
				ECDSACurve:   CurveP384,
				IsCA:         true,
			},
			wantKeyType:  "ecdsa",
			wantCurve:    elliptic.P384(),
			wantOrg:      "EC384 Org",
			wantCN:       "EC384 Org CA",
			wantDayRange: 365,
		},
		{
			name: "ECDSA P-521",
			cfg: &CertConfig{
				Days:         365,
				Organization: "EC521 Org",
				KeyType:      KeyTypeECDSA,
				ECDSACurve:   CurveP521,
				IsCA:         true,
			},
			wantKeyType:  "ecdsa",
			wantCurve:    elliptic.P521(),
			wantOrg:      "EC521 Org",
			wantCN:       "EC521 Org CA",
			wantDayRange: 365,
		},
		{
			name: "ECDSA default curve (empty string)",
			cfg: &CertConfig{
				Days:         365,
				Organization: "Default Curve",
				KeyType:      KeyTypeECDSA,
				ECDSACurve:   "",
				IsCA:         true,
			},
			wantKeyType:  "ecdsa",
			wantCurve:    elliptic.P256(),
			wantOrg:      "Default Curve",
			wantCN:       "Default Curve CA",
			wantDayRange: 365,
		},
		{
			name: "RSA default key size (0 defaults to 2048)",
			cfg: &CertConfig{
				Days:         30,
				Organization: "Default RSA",
				KeyType:      KeyTypeRSA,
				KeySize:      0,
				IsCA:         true,
			},
			wantKeyType:  "rsa",
			wantKeySize:  2048,
			wantOrg:      "Default RSA",
			wantCN:       "Default RSA CA",
			wantDayRange: 30,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			cert, err := GenerateCA(tt.cfg)
			if err != nil {
				t.Fatalf("GenerateCA() unexpected error: %v", err)
			}

			if len(cert.CertPEM) == 0 {
				t.Fatal("CertPEM is empty")
			}
			if len(cert.KeyPEM) == 0 {
				t.Fatal("KeyPEM is empty")
			}

			// Parse and verify the certificate
			parsed, err := ParseCertificate(cert.CertPEM)
			if err != nil {
				t.Fatalf("ParseCertificate() error: %v", err)
			}

			// Verify IsCA
			if !parsed.IsCA {
				t.Error("IsCA = false, want true")
			}
			if !parsed.BasicConstraintsValid {
				t.Error("BasicConstraintsValid = false, want true")
			}

			// Verify key usage
			if parsed.KeyUsage&x509.KeyUsageCertSign == 0 {
				t.Error("KeyUsage missing CertSign")
			}
			if parsed.KeyUsage&x509.KeyUsageCRLSign == 0 {
				t.Error("KeyUsage missing CRLSign")
			}

			// Verify subject
			if len(parsed.Subject.Organization) == 0 || parsed.Subject.Organization[0] != tt.wantOrg {
				t.Errorf("Organization = %v, want [%s]", parsed.Subject.Organization, tt.wantOrg)
			}
			if parsed.Subject.CommonName != tt.wantCN {
				t.Errorf("CommonName = %q, want %q", parsed.Subject.CommonName, tt.wantCN)
			}

			// Verify validity period
			expectedNotAfter := time.Now().AddDate(0, 0, tt.wantDayRange)
			delta := expectedNotAfter.Sub(parsed.NotAfter)
			if delta < -time.Minute || delta > time.Minute {
				t.Errorf("NotAfter = %v, expected approximately %v", parsed.NotAfter, expectedNotAfter)
			}

			// Verify key type
			key, err := ParsePrivateKey(cert.KeyPEM)
			if err != nil {
				t.Fatalf("ParsePrivateKey() error: %v", err)
			}

			switch tt.wantKeyType {
			case "rsa":
				rsaKey, ok := key.(*rsa.PrivateKey)
				if !ok {
					t.Fatalf("key type = %T, want *rsa.PrivateKey", key)
				}
				if rsaKey.N.BitLen() != tt.wantKeySize {
					t.Errorf("RSA key size = %d, want %d", rsaKey.N.BitLen(), tt.wantKeySize)
				}
			case "ecdsa":
				ecKey, ok := key.(*ecdsa.PrivateKey)
				if !ok {
					t.Fatalf("key type = %T, want *ecdsa.PrivateKey", key)
				}
				if ecKey.Curve != tt.wantCurve {
					t.Errorf("ECDSA curve = %v, want %v", ecKey.Curve.Params().Name, tt.wantCurve.Params().Name)
				}
			}

			// Verify MaxPathLen
			if parsed.MaxPathLen != 1 {
				t.Errorf("MaxPathLen = %d, want 1", parsed.MaxPathLen)
			}
		})
	}
}

func TestGenerateCA_Errors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		cfg  *CertConfig
	}{
		{
			name: "unsupported key type",
			cfg: &CertConfig{
				Days:         365,
				Organization: "Test",
				KeyType:      KeyType("ed25519"),
				IsCA:         true,
			},
		},
		{
			name: "unsupported ECDSA curve",
			cfg: &CertConfig{
				Days:         365,
				Organization: "Test",
				KeyType:      KeyTypeECDSA,
				ECDSACurve:   ECDSACurve("P-192"),
				IsCA:         true,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			_, err := GenerateCA(tt.cfg)
			if err == nil {
				t.Error("GenerateCA() expected error, got nil")
			}
		})
	}
}

func TestGenerateServerCert(t *testing.T) {
	t.Parallel()

	// Generate a CA for signing
	caCfg := &CertConfig{
		Days:         365,
		Organization: "Test CA",
		KeyType:      KeyTypeECDSA,
		ECDSACurve:   CurveP256,
		IsCA:         true,
	}
	parsedCA, caKey := helperGenerateCA(t, caCfg)

	tests := []struct {
		name         string
		cfg          *CertConfig
		wantDNS      []string
		wantIPs      []net.IP
		wantCN       string
		wantOrg      string
		wantKeyType  string
		wantKeySize  int
		wantCurve    elliptic.Curve
		wantDayRange int
	}{
		{
			name: "single DNS host",
			cfg: &CertConfig{
				Hosts:        []string{"example.local"},
				Days:         90,
				Organization: "Server Org",
				KeyType:      KeyTypeECDSA,
				ECDSACurve:   CurveP256,
			},
			wantDNS:      []string{"example.local"},
			wantCN:       "example.local",
			wantOrg:      "Server Org",
			wantKeyType:  "ecdsa",
			wantCurve:    elliptic.P256(),
			wantDayRange: 90,
		},
		{
			name: "multiple DNS and IP hosts",
			cfg: &CertConfig{
				Hosts:        []string{"localhost", "myapp.local", "127.0.0.1", "::1"},
				Days:         365,
				Organization: "Multi Org",
				KeyType:      KeyTypeRSA,
				KeySize:      2048,
			},
			wantDNS:      []string{"localhost", "myapp.local"},
			wantIPs:      []net.IP{net.ParseIP("127.0.0.1"), net.ParseIP("::1")},
			wantCN:       "localhost",
			wantOrg:      "Multi Org",
			wantKeyType:  "rsa",
			wantKeySize:  2048,
			wantDayRange: 365,
		},
		{
			name: "IP only host",
			cfg: &CertConfig{
				Hosts:        []string{"10.0.0.1"},
				Days:         30,
				Organization: "IP Org",
				KeyType:      KeyTypeECDSA,
				ECDSACurve:   CurveP384,
			},
			wantIPs:      []net.IP{net.ParseIP("10.0.0.1")},
			wantCN:       "10.0.0.1",
			wantOrg:      "IP Org",
			wantKeyType:  "ecdsa",
			wantCurve:    elliptic.P384(),
			wantDayRange: 30,
		},
		{
			name: "empty hosts defaults CN to localhost",
			cfg: &CertConfig{
				Hosts:        []string{},
				Days:         365,
				Organization: "Empty Org",
				KeyType:      KeyTypeECDSA,
				ECDSACurve:   CurveP256,
			},
			wantCN:       "localhost",
			wantOrg:      "Empty Org",
			wantKeyType:  "ecdsa",
			wantCurve:    elliptic.P256(),
			wantDayRange: 365,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			cert, err := GenerateServerCert(tt.cfg, parsedCA, caKey)
			if err != nil {
				t.Fatalf("GenerateServerCert() unexpected error: %v", err)
			}

			parsed, err := ParseCertificate(cert.CertPEM)
			if err != nil {
				t.Fatalf("ParseCertificate() error: %v", err)
			}

			// Verify not a CA
			if parsed.IsCA {
				t.Error("IsCA = true, want false")
			}

			// Verify ExtKeyUsage is ServerAuth
			if len(parsed.ExtKeyUsage) != 1 || parsed.ExtKeyUsage[0] != x509.ExtKeyUsageServerAuth {
				t.Errorf("ExtKeyUsage = %v, want [ServerAuth]", parsed.ExtKeyUsage)
			}

			// Verify key usage
			if parsed.KeyUsage&x509.KeyUsageDigitalSignature == 0 {
				t.Error("KeyUsage missing DigitalSignature")
			}
			if parsed.KeyUsage&x509.KeyUsageKeyEncipherment == 0 {
				t.Error("KeyUsage missing KeyEncipherment")
			}

			// Verify subject
			if parsed.Subject.CommonName != tt.wantCN {
				t.Errorf("CommonName = %q, want %q", parsed.Subject.CommonName, tt.wantCN)
			}
			if len(parsed.Subject.Organization) == 0 || parsed.Subject.Organization[0] != tt.wantOrg {
				t.Errorf("Organization = %v, want [%s]", parsed.Subject.Organization, tt.wantOrg)
			}

			// Verify DNS SANs
			if len(tt.wantDNS) > 0 {
				if len(parsed.DNSNames) != len(tt.wantDNS) {
					t.Errorf("DNSNames count = %d, want %d", len(parsed.DNSNames), len(tt.wantDNS))
				} else {
					for i, dns := range tt.wantDNS {
						if parsed.DNSNames[i] != dns {
							t.Errorf("DNSNames[%d] = %q, want %q", i, parsed.DNSNames[i], dns)
						}
					}
				}
			}

			// Verify IP SANs
			if len(tt.wantIPs) > 0 {
				if len(parsed.IPAddresses) != len(tt.wantIPs) {
					t.Errorf("IPAddresses count = %d, want %d", len(parsed.IPAddresses), len(tt.wantIPs))
				} else {
					for i, ip := range tt.wantIPs {
						if !parsed.IPAddresses[i].Equal(ip) {
							t.Errorf("IPAddresses[%d] = %v, want %v", i, parsed.IPAddresses[i], ip)
						}
					}
				}
			}

			// Verify validity
			expectedNotAfter := time.Now().AddDate(0, 0, tt.wantDayRange)
			delta := expectedNotAfter.Sub(parsed.NotAfter)
			if delta < -time.Minute || delta > time.Minute {
				t.Errorf("NotAfter = %v, expected approximately %v", parsed.NotAfter, expectedNotAfter)
			}

			// Verify key type
			key, err := ParsePrivateKey(cert.KeyPEM)
			if err != nil {
				t.Fatalf("ParsePrivateKey() error: %v", err)
			}

			switch tt.wantKeyType {
			case "rsa":
				rsaKey, ok := key.(*rsa.PrivateKey)
				if !ok {
					t.Fatalf("key type = %T, want *rsa.PrivateKey", key)
				}
				if tt.wantKeySize > 0 && rsaKey.N.BitLen() != tt.wantKeySize {
					t.Errorf("RSA key size = %d, want %d", rsaKey.N.BitLen(), tt.wantKeySize)
				}
			case "ecdsa":
				ecKey, ok := key.(*ecdsa.PrivateKey)
				if !ok {
					t.Fatalf("key type = %T, want *ecdsa.PrivateKey", key)
				}
				if tt.wantCurve != nil && ecKey.Curve != tt.wantCurve {
					t.Errorf("ECDSA curve = %v, want %v", ecKey.Curve.Params().Name, tt.wantCurve.Params().Name)
				}
			}
		})
	}
}

func TestGenerateServerCert_Errors(t *testing.T) {
	t.Parallel()

	caCfg := &CertConfig{
		Days:         365,
		Organization: "Test CA",
		KeyType:      KeyTypeECDSA,
		ECDSACurve:   CurveP256,
		IsCA:         true,
	}
	parsedCA, caKey := helperGenerateCA(t, caCfg)

	tests := []struct {
		name string
		cfg  *CertConfig
	}{
		{
			name: "unsupported key type",
			cfg: &CertConfig{
				Hosts:        []string{"localhost"},
				Days:         365,
				Organization: "Test",
				KeyType:      KeyType("unknown"),
			},
		},
		{
			name: "unsupported ECDSA curve",
			cfg: &CertConfig{
				Hosts:        []string{"localhost"},
				Days:         365,
				Organization: "Test",
				KeyType:      KeyTypeECDSA,
				ECDSACurve:   ECDSACurve("P-128"),
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			_, err := GenerateServerCert(tt.cfg, parsedCA, caKey)
			if err == nil {
				t.Error("GenerateServerCert() expected error, got nil")
			}
		})
	}
}

func TestGenerateClientCert(t *testing.T) {
	t.Parallel()

	caCfg := &CertConfig{
		Days:         365,
		Organization: "Test CA",
		KeyType:      KeyTypeECDSA,
		ECDSACurve:   CurveP256,
		IsCA:         true,
	}
	parsedCA, caKey := helperGenerateCA(t, caCfg)

	tests := []struct {
		name    string
		cfg     *CertConfig
		wantCN  string
		wantOrg string
	}{
		{
			name: "client cert with hosts",
			cfg: &CertConfig{
				Hosts:        []string{"client.local"},
				Days:         90,
				Organization: "Client Org",
				KeyType:      KeyTypeECDSA,
				ECDSACurve:   CurveP256,
				IsClient:     true,
			},
			wantCN:  "client.local",
			wantOrg: "Client Org",
		},
		{
			name: "client cert RSA",
			cfg: &CertConfig{
				Hosts:        []string{"admin"},
				Days:         365,
				Organization: "Admin Org",
				KeyType:      KeyTypeRSA,
				KeySize:      2048,
				IsClient:     true,
			},
			wantCN:  "admin",
			wantOrg: "Admin Org",
		},
		{
			name: "client cert empty hosts",
			cfg: &CertConfig{
				Days:         365,
				Organization: "No Host Org",
				KeyType:      KeyTypeECDSA,
				ECDSACurve:   CurveP256,
				IsClient:     true,
			},
			wantCN:  "localhost",
			wantOrg: "No Host Org",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			cert, err := GenerateClientCert(tt.cfg, parsedCA, caKey)
			if err != nil {
				t.Fatalf("GenerateClientCert() unexpected error: %v", err)
			}

			parsed, err := ParseCertificate(cert.CertPEM)
			if err != nil {
				t.Fatalf("ParseCertificate() error: %v", err)
			}

			// Verify ExtKeyUsage is ClientAuth
			if len(parsed.ExtKeyUsage) != 1 || parsed.ExtKeyUsage[0] != x509.ExtKeyUsageClientAuth {
				t.Errorf("ExtKeyUsage = %v, want [ClientAuth]", parsed.ExtKeyUsage)
			}

			// Verify not a CA
			if parsed.IsCA {
				t.Error("IsCA = true, want false")
			}

			// Verify subject
			if parsed.Subject.CommonName != tt.wantCN {
				t.Errorf("CommonName = %q, want %q", parsed.Subject.CommonName, tt.wantCN)
			}
			if len(parsed.Subject.Organization) == 0 || parsed.Subject.Organization[0] != tt.wantOrg {
				t.Errorf("Organization = %v, want [%s]", parsed.Subject.Organization, tt.wantOrg)
			}

			// Verify key usage
			if parsed.KeyUsage&x509.KeyUsageDigitalSignature == 0 {
				t.Error("KeyUsage missing DigitalSignature")
			}
			if parsed.KeyUsage&x509.KeyUsageKeyEncipherment == 0 {
				t.Error("KeyUsage missing KeyEncipherment")
			}
		})
	}
}

func TestParseCertificate(t *testing.T) {
	t.Parallel()

	// Generate a valid certificate for positive tests
	caCfg := &CertConfig{
		Days:         365,
		Organization: "Parse Test",
		KeyType:      KeyTypeECDSA,
		ECDSACurve:   CurveP256,
		IsCA:         true,
	}
	validCert, err := GenerateCA(caCfg)
	if err != nil {
		t.Fatalf("GenerateCA() for test setup: %v", err)
	}

	tests := []struct {
		name    string
		input   []byte
		wantErr bool
	}{
		{
			name:    "valid PEM certificate",
			input:   validCert.CertPEM,
			wantErr: false,
		},
		{
			name:    "nil input",
			input:   nil,
			wantErr: true,
		},
		{
			name:    "empty input",
			input:   []byte{},
			wantErr: true,
		},
		{
			name:    "invalid PEM data",
			input:   []byte("not a PEM block"),
			wantErr: true,
		},
		{
			name:    "wrong PEM block type (key instead of cert)",
			input:   validCert.KeyPEM,
			wantErr: true,
		},
		{
			name: "PEM with garbage DER data",
			input: pem.EncodeToMemory(&pem.Block{
				Type:  "CERTIFICATE",
				Bytes: []byte("this is not valid DER"),
			}),
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			parsed, err := ParseCertificate(tt.input)
			if tt.wantErr {
				if err == nil {
					t.Error("ParseCertificate() expected error, got nil")
				}
				return
			}

			if err != nil {
				t.Fatalf("ParseCertificate() unexpected error: %v", err)
			}
			if parsed == nil {
				t.Fatal("ParseCertificate() returned nil certificate")
			}
		})
	}
}

func TestParsePrivateKey(t *testing.T) {
	t.Parallel()

	// Generate valid keys for positive tests
	rsaCfg := &CertConfig{
		Days:         365,
		Organization: "RSA Parse Test",
		KeyType:      KeyTypeRSA,
		KeySize:      2048,
		IsCA:         true,
	}
	rsaCert, err := GenerateCA(rsaCfg)
	if err != nil {
		t.Fatalf("GenerateCA(RSA) for test setup: %v", err)
	}

	ecCfg := &CertConfig{
		Days:         365,
		Organization: "EC Parse Test",
		KeyType:      KeyTypeECDSA,
		ECDSACurve:   CurveP256,
		IsCA:         true,
	}
	ecCert, err := GenerateCA(ecCfg)
	if err != nil {
		t.Fatalf("GenerateCA(ECDSA) for test setup: %v", err)
	}

	tests := []struct {
		name        string
		input       []byte
		wantErr     bool
		wantKeyType string // "rsa", "ecdsa", or ""
	}{
		{
			name:        "valid RSA private key",
			input:       rsaCert.KeyPEM,
			wantErr:     false,
			wantKeyType: "rsa",
		},
		{
			name:        "valid ECDSA private key",
			input:       ecCert.KeyPEM,
			wantErr:     false,
			wantKeyType: "ecdsa",
		},
		{
			name:    "nil input",
			input:   nil,
			wantErr: true,
		},
		{
			name:    "empty input",
			input:   []byte{},
			wantErr: true,
		},
		{
			name:    "invalid PEM data",
			input:   []byte("not a PEM block"),
			wantErr: true,
		},
		{
			name: "unsupported PEM block type",
			input: pem.EncodeToMemory(&pem.Block{
				Type:  "PUBLIC KEY",
				Bytes: []byte("some data"),
			}),
			wantErr: true,
		},
		{
			name:    "certificate PEM instead of key",
			input:   rsaCert.CertPEM,
			wantErr: true,
		},
		{
			name: "RSA block with garbage DER",
			input: pem.EncodeToMemory(&pem.Block{
				Type:  "RSA PRIVATE KEY",
				Bytes: []byte("not valid DER"),
			}),
			wantErr: true,
		},
		{
			name: "EC block with garbage DER",
			input: pem.EncodeToMemory(&pem.Block{
				Type:  "EC PRIVATE KEY",
				Bytes: []byte("not valid DER"),
			}),
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			key, err := ParsePrivateKey(tt.input)
			if tt.wantErr {
				if err == nil {
					t.Error("ParsePrivateKey() expected error, got nil")
				}
				return
			}

			if err != nil {
				t.Fatalf("ParsePrivateKey() unexpected error: %v", err)
			}

			switch tt.wantKeyType {
			case "rsa":
				if _, ok := key.(*rsa.PrivateKey); !ok {
					t.Errorf("key type = %T, want *rsa.PrivateKey", key)
				}
			case "ecdsa":
				if _, ok := key.(*ecdsa.PrivateKey); !ok {
					t.Errorf("key type = %T, want *ecdsa.PrivateKey", key)
				}
			}
		})
	}
}

func TestWriteCertFiles(t *testing.T) {
	t.Parallel()

	// Generate a certificate for writing
	cfg := &CertConfig{
		Days:         365,
		Organization: "Write Test",
		KeyType:      KeyTypeECDSA,
		ECDSACurve:   CurveP256,
		IsCA:         true,
	}
	cert, err := GenerateCA(cfg)
	if err != nil {
		t.Fatalf("GenerateCA() for test setup: %v", err)
	}

	tests := []struct {
		name      string
		setup     func(t *testing.T, dir string)
		certName  string
		keyName   string
		overwrite bool
		wantErr   bool
	}{
		{
			name:      "write to new directory",
			certName:  "cert.pem",
			keyName:   "key.pem",
			overwrite: false,
			wantErr:   false,
		},
		{
			name:      "write with custom names",
			certName:  "server.crt",
			keyName:   "server.key",
			overwrite: false,
			wantErr:   false,
		},
		{
			name: "overwrite protection blocks existing cert",
			setup: func(t *testing.T, dir string) {
				t.Helper()
				if err := os.WriteFile(filepath.Join(dir, "cert.pem"), []byte("existing"), 0o644); err != nil {
					t.Fatalf("setup write: %v", err)
				}
			},
			certName:  "cert.pem",
			keyName:   "key.pem",
			overwrite: false,
			wantErr:   true,
		},
		{
			name: "overwrite protection blocks existing key",
			setup: func(t *testing.T, dir string) {
				t.Helper()
				if err := os.WriteFile(filepath.Join(dir, "key.pem"), []byte("existing"), 0o644); err != nil {
					t.Fatalf("setup write: %v", err)
				}
			},
			certName:  "cert.pem",
			keyName:   "key.pem",
			overwrite: false,
			wantErr:   true,
		},
		{
			name: "overwrite=true replaces existing files",
			setup: func(t *testing.T, dir string) {
				t.Helper()
				if err := os.WriteFile(filepath.Join(dir, "cert.pem"), []byte("old cert"), 0o644); err != nil {
					t.Fatalf("setup write cert: %v", err)
				}
				if err := os.WriteFile(filepath.Join(dir, "key.pem"), []byte("old key"), 0o600); err != nil {
					t.Fatalf("setup write key: %v", err)
				}
			},
			certName:  "cert.pem",
			keyName:   "key.pem",
			overwrite: true,
			wantErr:   false,
		},
		{
			name:      "creates subdirectory automatically",
			certName:  "cert.pem",
			keyName:   "key.pem",
			overwrite: false,
			wantErr:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			baseDir := t.TempDir()
			dir := filepath.Join(baseDir, "certs")

			// For tests that need existing files, create the dir first
			if tt.setup != nil {
				if err := os.MkdirAll(dir, 0o700); err != nil {
					t.Fatalf("MkdirAll: %v", err)
				}
				tt.setup(t, dir)
			}

			err := WriteCertFiles(dir, tt.certName, tt.keyName, cert, tt.overwrite)
			if tt.wantErr {
				if err == nil {
					t.Error("WriteCertFiles() expected error, got nil")
				}
				return
			}

			if err != nil {
				t.Fatalf("WriteCertFiles() unexpected error: %v", err)
			}

			// Verify cert file was written correctly
			certPath := filepath.Join(dir, tt.certName)
			certData, err := os.ReadFile(certPath)
			if err != nil {
				t.Fatalf("reading cert file: %v", err)
			}
			if string(certData) != string(cert.CertPEM) {
				t.Error("cert file content does not match expected PEM")
			}

			// Verify key file was written correctly
			keyPath := filepath.Join(dir, tt.keyName)
			keyData, err := os.ReadFile(keyPath)
			if err != nil {
				t.Fatalf("reading key file: %v", err)
			}
			if string(keyData) != string(cert.KeyPEM) {
				t.Error("key file content does not match expected PEM")
			}

			// Verify key file has restricted permissions (skip on Windows where
			// POSIX file permissions are not supported)
			if runtime.GOOS != "windows" {
				keyInfo, err := os.Stat(keyPath)
				if err != nil {
					t.Fatalf("stat key file: %v", err)
				}
				keyPerm := keyInfo.Mode().Perm()
				if keyPerm != 0o600 {
					t.Errorf("key file permissions = %o, want 600", keyPerm)
				}
			}
		})
	}
}

func TestWriteReadme(t *testing.T) {
	t.Parallel()

	t.Run("writes readme to new directory", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()
		err := WriteReadme(dir, []string{"localhost", "127.0.0.1"}, false)
		if err != nil {
			t.Fatalf("WriteReadme() unexpected error: %v", err)
		}

		readmePath := filepath.Join(dir, "README.txt")
		data, err := os.ReadFile(readmePath)
		if err != nil {
			t.Fatalf("reading README.txt: %v", err)
		}
		if len(data) == 0 {
			t.Error("README.txt is empty")
		}
	})

	t.Run("skips silently if exists and overwrite=false", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()
		original := []byte("original content")
		readmePath := filepath.Join(dir, "README.txt")
		if err := os.WriteFile(readmePath, original, 0o644); err != nil {
			t.Fatalf("setup write: %v", err)
		}

		err := WriteReadme(dir, []string{"localhost"}, false)
		if err != nil {
			t.Fatalf("WriteReadme() unexpected error: %v", err)
		}

		// Verify content was not overwritten
		data, err := os.ReadFile(readmePath)
		if err != nil {
			t.Fatalf("reading README.txt: %v", err)
		}
		if string(data) != string(original) {
			t.Error("README.txt was overwritten when overwrite=false")
		}
	})

	t.Run("overwrites when overwrite=true", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()
		readmePath := filepath.Join(dir, "README.txt")
		if err := os.WriteFile(readmePath, []byte("old"), 0o644); err != nil {
			t.Fatalf("setup write: %v", err)
		}

		err := WriteReadme(dir, []string{"localhost"}, true)
		if err != nil {
			t.Fatalf("WriteReadme() unexpected error: %v", err)
		}

		data, err := os.ReadFile(readmePath)
		if err != nil {
			t.Fatalf("reading README.txt: %v", err)
		}
		if string(data) == "old" {
			t.Error("README.txt was not overwritten when overwrite=true")
		}
	})
}

func TestCertificateChainValidation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		caKeyType  KeyType
		caKeySize  int
		caCurve    ECDSACurve
		srvKeyType KeyType
		srvKeySize int
		srvCurve   ECDSACurve
		hosts      []string
		verifyHost string
	}{
		{
			name:       "RSA CA signs RSA server cert",
			caKeyType:  KeyTypeRSA,
			caKeySize:  2048,
			srvKeyType: KeyTypeRSA,
			srvKeySize: 2048,
			hosts:      []string{"localhost", "127.0.0.1"},
			verifyHost: "localhost",
		},
		{
			name:       "ECDSA CA signs ECDSA server cert",
			caKeyType:  KeyTypeECDSA,
			caCurve:    CurveP256,
			srvKeyType: KeyTypeECDSA,
			srvCurve:   CurveP256,
			hosts:      []string{"myapp.local", "10.0.0.1"},
			verifyHost: "myapp.local",
		},
		{
			name:       "RSA CA signs ECDSA server cert (cross-type)",
			caKeyType:  KeyTypeRSA,
			caKeySize:  2048,
			srvKeyType: KeyTypeECDSA,
			srvCurve:   CurveP384,
			hosts:      []string{"cross.local"},
			verifyHost: "cross.local",
		},
		{
			name:       "ECDSA CA signs RSA server cert (cross-type)",
			caKeyType:  KeyTypeECDSA,
			caCurve:    CurveP256,
			srvKeyType: KeyTypeRSA,
			srvKeySize: 2048,
			hosts:      []string{"reverse-cross.local"},
			verifyHost: "reverse-cross.local",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			// Generate CA
			caCfg := &CertConfig{
				Days:         365,
				Organization: "Chain Test CA",
				KeyType:      tt.caKeyType,
				KeySize:      tt.caKeySize,
				ECDSACurve:   tt.caCurve,
				IsCA:         true,
			}
			parsedCA, caKey := helperGenerateCA(t, caCfg)

			// Generate server cert
			srvCfg := &CertConfig{
				Hosts:        tt.hosts,
				Days:         90,
				Organization: "Chain Test Server",
				KeyType:      tt.srvKeyType,
				KeySize:      tt.srvKeySize,
				ECDSACurve:   tt.srvCurve,
			}
			serverCert, err := GenerateServerCert(srvCfg, parsedCA, caKey)
			if err != nil {
				t.Fatalf("GenerateServerCert() unexpected error: %v", err)
			}

			parsedServer, err := ParseCertificate(serverCert.CertPEM)
			if err != nil {
				t.Fatalf("ParseCertificate(server) error: %v", err)
			}

			// Build a cert pool with the CA
			pool := x509.NewCertPool()
			pool.AddCert(parsedCA)

			// Verify the chain
			opts := x509.VerifyOptions{
				Roots:   pool,
				DNSName: tt.verifyHost,
				KeyUsages: []x509.ExtKeyUsage{
					x509.ExtKeyUsageServerAuth,
				},
			}

			chains, err := parsedServer.Verify(opts)
			if err != nil {
				t.Fatalf("certificate chain validation failed: %v", err)
			}
			if len(chains) == 0 {
				t.Fatal("Verify() returned no chains")
			}
			if len(chains[0]) != 2 {
				t.Errorf("chain length = %d, want 2 (server + CA)", len(chains[0]))
			}
		})
	}
}

func TestCertificateChainValidation_ClientCert(t *testing.T) {
	t.Parallel()

	caCfg := &CertConfig{
		Days:         365,
		Organization: "Client Chain CA",
		KeyType:      KeyTypeECDSA,
		ECDSACurve:   CurveP256,
		IsCA:         true,
	}
	parsedCA, caKey := helperGenerateCA(t, caCfg)

	clientCfg := &CertConfig{
		Hosts:        []string{"client.local"},
		Days:         90,
		Organization: "Client Chain",
		KeyType:      KeyTypeECDSA,
		ECDSACurve:   CurveP256,
		IsClient:     true,
	}
	clientCert, err := GenerateClientCert(clientCfg, parsedCA, caKey)
	if err != nil {
		t.Fatalf("GenerateClientCert() unexpected error: %v", err)
	}

	parsedClient, err := ParseCertificate(clientCert.CertPEM)
	if err != nil {
		t.Fatalf("ParseCertificate(client) error: %v", err)
	}

	pool := x509.NewCertPool()
	pool.AddCert(parsedCA)

	opts := x509.VerifyOptions{
		Roots: pool,
		KeyUsages: []x509.ExtKeyUsage{
			x509.ExtKeyUsageClientAuth,
		},
	}

	chains, err := parsedClient.Verify(opts)
	if err != nil {
		t.Fatalf("client certificate chain validation failed: %v", err)
	}
	if len(chains) == 0 {
		t.Fatal("Verify() returned no chains")
	}
}

func TestCertificateChainValidation_WrongCA(t *testing.T) {
	t.Parallel()

	// Generate two separate CAs
	ca1Cfg := &CertConfig{
		Days:         365,
		Organization: "CA One",
		KeyType:      KeyTypeECDSA,
		ECDSACurve:   CurveP256,
		IsCA:         true,
	}
	parsedCA1, ca1Key := helperGenerateCA(t, ca1Cfg)

	ca2Cfg := &CertConfig{
		Days:         365,
		Organization: "CA Two",
		KeyType:      KeyTypeECDSA,
		ECDSACurve:   CurveP256,
		IsCA:         true,
	}
	parsedCA2, _ := helperGenerateCA(t, ca2Cfg)

	// Sign server cert with CA1
	srvCfg := &CertConfig{
		Hosts:        []string{"localhost"},
		Days:         90,
		Organization: "Server",
		KeyType:      KeyTypeECDSA,
		ECDSACurve:   CurveP256,
	}
	serverCert, err := GenerateServerCert(srvCfg, parsedCA1, ca1Key)
	if err != nil {
		t.Fatalf("GenerateServerCert() error: %v", err)
	}

	parsedServer, err := ParseCertificate(serverCert.CertPEM)
	if err != nil {
		t.Fatalf("ParseCertificate() error: %v", err)
	}

	// Try to verify against CA2 - should fail
	pool := x509.NewCertPool()
	pool.AddCert(parsedCA2)

	opts := x509.VerifyOptions{
		Roots:   pool,
		DNSName: "localhost",
		KeyUsages: []x509.ExtKeyUsage{
			x509.ExtKeyUsageServerAuth,
		},
	}

	_, err = parsedServer.Verify(opts)
	if err == nil {
		t.Fatal("expected chain validation to fail with wrong CA, but it succeeded")
	}
}

func TestZeroDaysValidity(t *testing.T) {
	t.Parallel()

	cfg := &CertConfig{
		Days:         0,
		Organization: "Zero Days",
		KeyType:      KeyTypeECDSA,
		ECDSACurve:   CurveP256,
		IsCA:         true,
	}

	cert, err := GenerateCA(cfg)
	if err != nil {
		t.Fatalf("GenerateCA() unexpected error: %v", err)
	}

	parsed, err := ParseCertificate(cert.CertPEM)
	if err != nil {
		t.Fatalf("ParseCertificate() error: %v", err)
	}

	// With 0 days, NotBefore and NotAfter should be essentially the same time
	delta := parsed.NotAfter.Sub(parsed.NotBefore)
	if delta < -time.Second || delta > time.Second {
		t.Errorf("with 0 days, NotAfter-NotBefore = %v, expected ~0", delta)
	}
}

func TestGenerateCA_UniqueSerialNumbers(t *testing.T) {
	t.Parallel()

	cfg := &CertConfig{
		Days:         365,
		Organization: "Serial Test",
		KeyType:      KeyTypeECDSA,
		ECDSACurve:   CurveP256,
		IsCA:         true,
	}

	cert1, err := GenerateCA(cfg)
	if err != nil {
		t.Fatalf("GenerateCA() #1 error: %v", err)
	}
	cert2, err := GenerateCA(cfg)
	if err != nil {
		t.Fatalf("GenerateCA() #2 error: %v", err)
	}

	parsed1, err := ParseCertificate(cert1.CertPEM)
	if err != nil {
		t.Fatalf("ParseCertificate() #1 error: %v", err)
	}
	parsed2, err := ParseCertificate(cert2.CertPEM)
	if err != nil {
		t.Fatalf("ParseCertificate() #2 error: %v", err)
	}

	if parsed1.SerialNumber.Cmp(parsed2.SerialNumber) == 0 {
		t.Error("two generated certificates have the same serial number")
	}
}
