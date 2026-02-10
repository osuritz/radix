// Package tls provides TLS certificate generation for local development.
package tls

import (
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"time"
)

// KeyType represents the type of private key to generate.
type KeyType string

const (
	// KeyTypeRSA generates an RSA private key.
	KeyTypeRSA KeyType = "rsa"
	// KeyTypeECDSA generates an ECDSA private key.
	KeyTypeECDSA KeyType = "ecdsa"
)

// ECDSACurve represents an ECDSA elliptic curve.
type ECDSACurve string

const (
	// CurveP256 is the NIST P-256 curve.
	CurveP256 ECDSACurve = "P-256"
	// CurveP384 is the NIST P-384 curve.
	CurveP384 ECDSACurve = "P-384"
	// CurveP521 is the NIST P-521 curve.
	CurveP521 ECDSACurve = "P-521"
)

// CertConfig holds configuration for certificate generation.
type CertConfig struct {
	Hosts        []string   // Hostnames and IPs for SANs
	Days         int        // Validity period in days
	Organization string     // Organization name for subject
	KeyType      KeyType    // rsa or ecdsa
	KeySize      int        // RSA key size (2048, 4096)
	ECDSACurve   ECDSACurve // ECDSA curve (P-256, P-384, P-521)
	IsCA         bool       // Generate a CA certificate
	IsClient     bool       // Generate a client certificate
}

// Certificate holds generated certificate and key data in PEM format.
type Certificate struct {
	CertPEM []byte
	KeyPEM  []byte
}

// GenerateCA generates a self-signed CA certificate.
func GenerateCA(cfg *CertConfig) (*Certificate, error) {
	privateKey, publicKey, err := generateKeyPair(cfg)
	if err != nil {
		return nil, fmt.Errorf("generating CA key pair: %w", err)
	}

	serialNumber, err := generateSerialNumber()
	if err != nil {
		return nil, fmt.Errorf("generating serial number: %w", err)
	}

	template := &x509.Certificate{
		SerialNumber: serialNumber,
		Subject: pkix.Name{
			Organization: []string{cfg.Organization},
			CommonName:   cfg.Organization + " CA",
		},
		NotBefore:             time.Now(),
		NotAfter:              time.Now().AddDate(0, 0, cfg.Days),
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
		BasicConstraintsValid: true,
		IsCA:                  true,
		MaxPathLen:            1,
	}

	certDER, err := x509.CreateCertificate(rand.Reader, template, template, publicKey, privateKey)
	if err != nil {
		return nil, fmt.Errorf("creating CA certificate: %w", err)
	}

	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certDER})
	keyPEM, err := marshalPrivateKey(privateKey)
	if err != nil {
		return nil, fmt.Errorf("encoding CA private key: %w", err)
	}

	return &Certificate{CertPEM: certPEM, KeyPEM: keyPEM}, nil
}

// GenerateServerCert generates a server certificate signed by the given CA.
func GenerateServerCert(cfg *CertConfig, caCert *x509.Certificate, caKey crypto.PrivateKey) (*Certificate, error) {
	return generateEndEntityCert(cfg, caCert, caKey, false)
}

// GenerateClientCert generates a client certificate signed by the given CA.
func GenerateClientCert(cfg *CertConfig, caCert *x509.Certificate, caKey crypto.PrivateKey) (*Certificate, error) {
	return generateEndEntityCert(cfg, caCert, caKey, true)
}

func generateEndEntityCert(cfg *CertConfig, caCert *x509.Certificate, caKey crypto.PrivateKey, isClient bool) (*Certificate, error) {
	privateKey, publicKey, err := generateKeyPair(cfg)
	if err != nil {
		return nil, fmt.Errorf("generating key pair: %w", err)
	}

	serialNumber, err := generateSerialNumber()
	if err != nil {
		return nil, fmt.Errorf("generating serial number: %w", err)
	}

	cn := "localhost"
	if len(cfg.Hosts) > 0 {
		cn = cfg.Hosts[0]
	}

	template := &x509.Certificate{
		SerialNumber: serialNumber,
		Subject: pkix.Name{
			Organization: []string{cfg.Organization},
			CommonName:   cn,
		},
		NotBefore:             time.Now(),
		NotAfter:              time.Now().AddDate(0, 0, cfg.Days),
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		BasicConstraintsValid: true,
	}

	if isClient {
		template.ExtKeyUsage = []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth}
	} else {
		template.ExtKeyUsage = []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}
	}

	// Add SANs
	for _, h := range cfg.Hosts {
		if ip := net.ParseIP(h); ip != nil {
			template.IPAddresses = append(template.IPAddresses, ip)
		} else {
			template.DNSNames = append(template.DNSNames, h)
		}
	}

	certDER, err := x509.CreateCertificate(rand.Reader, template, caCert, publicKey, caKey)
	if err != nil {
		return nil, fmt.Errorf("creating certificate: %w", err)
	}

	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certDER})
	keyPEM, err := marshalPrivateKey(privateKey)
	if err != nil {
		return nil, fmt.Errorf("encoding private key: %w", err)
	}

	return &Certificate{CertPEM: certPEM, KeyPEM: keyPEM}, nil
}

// ParseCertificate parses a PEM-encoded certificate.
func ParseCertificate(certPEM []byte) (*x509.Certificate, error) {
	block, _ := pem.Decode(certPEM)
	if block == nil {
		return nil, fmt.Errorf("failed to decode PEM block")
	}
	return x509.ParseCertificate(block.Bytes)
}

// ParsePrivateKey parses a PEM-encoded private key (RSA or ECDSA).
func ParsePrivateKey(keyPEM []byte) (crypto.PrivateKey, error) {
	block, _ := pem.Decode(keyPEM)
	if block == nil {
		return nil, fmt.Errorf("failed to decode PEM block")
	}

	switch block.Type {
	case "RSA PRIVATE KEY":
		return x509.ParsePKCS1PrivateKey(block.Bytes)
	case "EC PRIVATE KEY":
		return x509.ParseECPrivateKey(block.Bytes)
	default:
		return nil, fmt.Errorf("unsupported key type: %s", block.Type)
	}
}

// WriteCertFiles writes certificate and key PEM data to files in the given directory.
func WriteCertFiles(dir, certName, keyName string, cert *Certificate, overwrite bool) error {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("creating output directory: %w", err)
	}

	certPath := filepath.Join(dir, certName)
	keyPath := filepath.Join(dir, keyName)

	if !overwrite {
		if _, err := os.Stat(certPath); err == nil {
			return fmt.Errorf("file already exists: %s (use --overwrite to replace)", certPath)
		}
		if _, err := os.Stat(keyPath); err == nil {
			return fmt.Errorf("file already exists: %s (use --overwrite to replace)", keyPath)
		}
	}

	// #nosec G306 - certificate files are not sensitive (public key material)
	if err := os.WriteFile(certPath, cert.CertPEM, 0o644); err != nil {
		return fmt.Errorf("writing certificate file: %w", err)
	}

	// #nosec G306 - key file permissions are restricted
	if err := os.WriteFile(keyPath, cert.KeyPEM, 0o600); err != nil {
		return fmt.Errorf("writing key file: %w", err)
	}

	return nil
}

// WriteReadme writes a README.txt with usage instructions to the output directory.
func WriteReadme(dir string, _ []string, overwrite bool) error {
	readmePath := filepath.Join(dir, "README.txt")

	if !overwrite {
		if _, err := os.Stat(readmePath); err == nil {
			return nil // Skip silently if exists
		}
	}

	content := `Radix Development Certificates
==============================

These certificates were generated by "radix gencert" for local development.
They are NOT suitable for production use.

Files:
  ca.pem      - CA certificate (import into your browser/OS trust store)
  ca-key.pem  - CA private key (keep secure, used to sign new certificates)
  cert.pem    - Server certificate (used by radix serve/proxy/echo/mock --tls)
  key.pem     - Server private key

Usage with Radix:
  radix serve --tls --cert ./certs/cert.pem --key ./certs/key.pem

Trust the CA (so browsers don't warn):

  macOS:
    sudo security add-trusted-cert -d -r trustRoot \
      -k /Library/Keychains/System.keychain ./certs/ca.pem

  Linux:
    sudo cp ./certs/ca.pem /usr/local/share/ca-certificates/radix-ca.crt
    sudo update-ca-certificates

  Windows:
    certutil -addstore -f "ROOT" .\certs\ca.pem

  Firefox (all platforms):
    Preferences > Privacy & Security > View Certificates > Import ca.pem
`

	// #nosec G306 - readme is not sensitive
	return os.WriteFile(readmePath, []byte(content), 0o644)
}

func generateKeyPair(cfg *CertConfig) (crypto.PrivateKey, crypto.PublicKey, error) {
	switch cfg.KeyType {
	case KeyTypeRSA:
		keySize := cfg.KeySize
		if keySize == 0 {
			keySize = 2048
		}
		key, err := rsa.GenerateKey(rand.Reader, keySize)
		if err != nil {
			return nil, nil, fmt.Errorf("generating RSA key: %w", err)
		}
		return key, &key.PublicKey, nil

	case KeyTypeECDSA:
		curve, err := ellipticCurve(cfg.ECDSACurve)
		if err != nil {
			return nil, nil, err
		}
		key, err := ecdsa.GenerateKey(curve, rand.Reader)
		if err != nil {
			return nil, nil, fmt.Errorf("generating ECDSA key: %w", err)
		}
		return key, &key.PublicKey, nil

	default:
		return nil, nil, fmt.Errorf("unsupported key type: %s", cfg.KeyType)
	}
}

func ellipticCurve(curve ECDSACurve) (elliptic.Curve, error) {
	switch curve {
	case CurveP256, "":
		return elliptic.P256(), nil
	case CurveP384:
		return elliptic.P384(), nil
	case CurveP521:
		return elliptic.P521(), nil
	default:
		return nil, fmt.Errorf("unsupported ECDSA curve: %s", curve)
	}
}

func marshalPrivateKey(key crypto.PrivateKey) ([]byte, error) {
	switch k := key.(type) {
	case *rsa.PrivateKey:
		return pem.EncodeToMemory(&pem.Block{
			Type:  "RSA PRIVATE KEY",
			Bytes: x509.MarshalPKCS1PrivateKey(k),
		}), nil
	case *ecdsa.PrivateKey:
		der, err := x509.MarshalECPrivateKey(k)
		if err != nil {
			return nil, fmt.Errorf("marshaling ECDSA key: %w", err)
		}
		return pem.EncodeToMemory(&pem.Block{
			Type:  "EC PRIVATE KEY",
			Bytes: der,
		}), nil
	default:
		return nil, fmt.Errorf("unsupported key type: %T", key)
	}
}

func generateSerialNumber() (*big.Int, error) {
	serialNumberLimit := new(big.Int).Lsh(big.NewInt(1), 128)
	return rand.Int(rand.Reader, serialNumberLimit)
}
