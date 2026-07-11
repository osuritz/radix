// Package tls provides TLS certificate generation and configuration loading
// for local development.
package tls

import (
	cryptotls "crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"os"
)

// ServerTLSOptions holds configuration for building a server TLS config.
type ServerTLSOptions struct {
	CertFile   string // Path to PEM-encoded server certificate
	KeyFile    string // Path to PEM-encoded server private key
	CAFile     string // Optional: CA cert for client certificate verification
	ClientAuth bool   // Require client certificates (mTLS)

	// ClientAuthOptional requests client certificates without requiring them:
	// a presented certificate is verified against CAFile
	// (crypto/tls.VerifyClientCertIfGiven), but a connection without one is
	// still accepted at the TLS layer, letting the application enforce
	// per-route requirements (e.g. mock routes' require_client_cert).
	// Setting it together with ClientAuth is a configuration error, as is
	// setting it without CAFile.
	ClientAuthOptional bool

	MinVersion string // Minimum TLS version: "1.2" or "1.3"
}

// ClientTLSOptions holds configuration for building a client TLS config.
type ClientTLSOptions struct {
	CertFile   string // Optional: client cert for mTLS
	KeyFile    string // Optional: client key for mTLS
	CAFile     string // Optional: custom CA for server verification
	SkipVerify bool   // Skip server certificate verification
	MinVersion string // Minimum TLS version: "1.2" or "1.3"
	ServerName string // Optional SNI override
}

// defaultCipherSuites returns PFS-only cipher suites for TLS 1.2.
// For TLS 1.3, Go automatically uses optimal cipher suites and this list
// is not applied.
func defaultCipherSuites() []uint16 {
	return []uint16{
		cryptotls.TLS_ECDHE_ECDSA_WITH_AES_256_GCM_SHA384,
		cryptotls.TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384,
		cryptotls.TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256,
		cryptotls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256,
		cryptotls.TLS_ECDHE_ECDSA_WITH_CHACHA20_POLY1305,
		cryptotls.TLS_ECDHE_RSA_WITH_CHACHA20_POLY1305,
	}
}

// ParseMinVersion parses a TLS version string ("1.2" or "1.3") into the
// corresponding crypto/tls constant.
func ParseMinVersion(version string) (uint16, error) {
	switch version {
	case "1.2", "":
		return cryptotls.VersionTLS12, nil
	case "1.3":
		return cryptotls.VersionTLS13, nil
	default:
		return 0, fmt.Errorf("unsupported TLS version %q: must be \"1.2\" or \"1.3\"", version)
	}
}

// NewServerTLSConfig builds a *crypto/tls.Config for an HTTPS server.
//
// It loads the server certificate and key from PEM files, optionally loads a CA
// certificate for client verification, configures the minimum TLS version, and
// selects strong PFS cipher suites for TLS 1.2.
func NewServerTLSConfig(cfg ServerTLSOptions) (*cryptotls.Config, error) {
	// Validate and load server certificate and key
	if cfg.CertFile == "" {
		return nil, fmt.Errorf("server certificate file is required")
	}
	if cfg.KeyFile == "" {
		return nil, fmt.Errorf("server key file is required")
	}

	if err := validatePEMFile(cfg.CertFile, "CERTIFICATE"); err != nil {
		return nil, fmt.Errorf("invalid server certificate: %w", err)
	}
	if err := validatePEMFile(cfg.KeyFile, ""); err != nil {
		return nil, fmt.Errorf("invalid server key: %w", err)
	}

	cert, err := cryptotls.LoadX509KeyPair(cfg.CertFile, cfg.KeyFile)
	if err != nil {
		return nil, fmt.Errorf("loading server certificate and key: %w", err)
	}

	minVersion, err := ParseMinVersion(cfg.MinVersion)
	if err != nil {
		return nil, err
	}

	tlsConfig := &cryptotls.Config{ //#nosec G402 - MinVersion is user-configurable, minimum 1.2
		Certificates: []cryptotls.Certificate{cert},
		MinVersion:   minVersion,
	}

	// Only set cipher suites for TLS 1.2; for TLS 1.3, Go's defaults are optimal
	if minVersion < cryptotls.VersionTLS13 {
		tlsConfig.CipherSuites = defaultCipherSuites()
	}

	// Configure client authentication. Requiring (ClientAuth) and merely
	// requesting (ClientAuthOptional) a client certificate are mutually
	// exclusive: silently picking one would mask a misconfiguration, so the
	// ambiguity is rejected here rather than resolved. Optional client auth
	// also demands an explicit client CA — with an empty CAFile Go would
	// verify presented certificates against the SYSTEM root store, silently
	// rejecting locally generated (gencert) client certs while accepting any
	// WebPKI clientAuth certificate.
	if cfg.ClientAuth && cfg.ClientAuthOptional {
		return nil, fmt.Errorf("client authentication cannot be both required (ClientAuth) and optional (ClientAuthOptional): set only one")
	}
	if cfg.ClientAuthOptional && cfg.CAFile == "" {
		return nil, fmt.Errorf("optional client authentication requires a client CA file: without one, presented certificates would be verified against the system root store instead of your own CA")
	}
	if cfg.ClientAuth || cfg.ClientAuthOptional {
		if cfg.ClientAuth {
			tlsConfig.ClientAuth = cryptotls.RequireAndVerifyClientCert
		} else {
			tlsConfig.ClientAuth = cryptotls.VerifyClientCertIfGiven
		}

		if cfg.CAFile != "" {
			caPool, poolErr := loadCACertPool(cfg.CAFile)
			if poolErr != nil {
				return nil, fmt.Errorf("loading client CA: %w", poolErr)
			}
			tlsConfig.ClientCAs = caPool
		}
	}

	return tlsConfig, nil
}

// NewClientTLSConfig builds a *crypto/tls.Config for outbound TLS connections.
//
// It optionally loads a client certificate for mTLS, a custom CA for server
// verification, and supports InsecureSkipVerify for development use.
func NewClientTLSConfig(cfg *ClientTLSOptions) (*cryptotls.Config, error) {
	minVersion, err := ParseMinVersion(cfg.MinVersion)
	if err != nil {
		return nil, err
	}

	tlsConfig := &cryptotls.Config{
		MinVersion:         minVersion,
		InsecureSkipVerify: cfg.SkipVerify, //#nosec G402 - user-controlled option for development
	}

	// Only set cipher suites for TLS 1.2
	if minVersion < cryptotls.VersionTLS13 {
		tlsConfig.CipherSuites = defaultCipherSuites()
	}

	// Load client certificate for mTLS
	if cfg.CertFile != "" && cfg.KeyFile != "" {
		if err := validatePEMFile(cfg.CertFile, "CERTIFICATE"); err != nil {
			return nil, fmt.Errorf("invalid client certificate: %w", err)
		}
		if err := validatePEMFile(cfg.KeyFile, ""); err != nil {
			return nil, fmt.Errorf("invalid client key: %w", err)
		}

		cert, loadErr := cryptotls.LoadX509KeyPair(cfg.CertFile, cfg.KeyFile)
		if loadErr != nil {
			return nil, fmt.Errorf("loading client certificate and key: %w", loadErr)
		}
		tlsConfig.Certificates = []cryptotls.Certificate{cert}
	} else if cfg.CertFile != "" || cfg.KeyFile != "" {
		return nil, fmt.Errorf("both client certificate and key must be provided for mTLS")
	}

	// Load custom CA for server verification
	if cfg.CAFile != "" {
		caPool, poolErr := loadCACertPool(cfg.CAFile)
		if poolErr != nil {
			return nil, fmt.Errorf("loading custom CA: %w", poolErr)
		}
		tlsConfig.RootCAs = caPool
	}

	// Set SNI server name override
	if cfg.ServerName != "" {
		tlsConfig.ServerName = cfg.ServerName
	}

	return tlsConfig, nil
}

// loadCACertPool reads a PEM-encoded CA certificate file and returns a
// certificate pool containing the CA certificate.
func loadCACertPool(caFile string) (*x509.CertPool, error) {
	if err := validatePEMFile(caFile, "CERTIFICATE"); err != nil {
		return nil, fmt.Errorf("invalid CA file: %w", err)
	}

	// #nosec G304 - CA file path is user-provided and validated above
	caPEM, err := os.ReadFile(caFile)
	if err != nil {
		return nil, fmt.Errorf("reading CA file %s: %w", caFile, err)
	}

	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(caPEM) {
		return nil, fmt.Errorf("failed to parse CA certificate from %s", caFile)
	}

	return pool, nil
}

// validatePEMFile checks that a file exists, is readable, and contains valid
// PEM data. If expectedType is non-empty, it verifies the PEM block type
// matches.
func validatePEMFile(path, expectedType string) error {
	// #nosec G304 - file path is user-provided configuration
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("file not found: %s", path)
		}
		return fmt.Errorf("reading file %s: %w", path, err)
	}

	block, _ := pem.Decode(data)
	if block == nil {
		return fmt.Errorf("file contains no valid PEM data: %s", path)
	}

	if expectedType != "" && block.Type != expectedType {
		return fmt.Errorf("expected PEM type %q, got %q in %s", expectedType, block.Type, path)
	}

	return nil
}
