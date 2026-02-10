package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/osuritz/radix/internal/tls"
	"github.com/spf13/cobra"
)

var (
	gencertHosts      string
	gencertOutput     string
	gencertDays       int
	gencertOrg        string
	gencertKeySize    int
	gencertKeyType    string
	gencertECDSACurve string
	gencertCA         bool
	gencertCACert     string
	gencertCAKey      string
	gencertClient     bool
	gencertOverwrite  bool
)

// gencertCmd represents the gencert command
var gencertCmd = &cobra.Command{
	Use:   "gencert",
	Short: "Generate TLS certificates for local development",
	Long: `Generate self-signed TLS certificates for local HTTPS development.

By default, generates a CA certificate and a server certificate signed by that CA.
The CA can be imported into your browser or OS trust store to avoid security warnings.

Examples:
  radix gencert                                    # Generate certs for localhost
  radix gencert --host localhost,127.0.0.1,myapp   # Multiple hosts/IPs
  radix gencert --output ./my-certs                # Custom output directory
  radix gencert --days 730                         # 2-year validity
  radix gencert --key-type ecdsa --ecdsa-curve P-384
  radix gencert --ca-cert ./ca.pem --ca-key ./ca-key.pem  # Use existing CA
  radix gencert --client                           # Generate client certificate`,
	RunE: runGencert,
}

func init() {
	gencertCmd.Flags().StringVar(&gencertHosts, "host", "localhost", "comma-separated hostnames and/or IPs for the certificate SANs")
	gencertCmd.Flags().StringVarP(&gencertOutput, "output", "o", "./certs", "output directory for generated certificates")
	gencertCmd.Flags().IntVar(&gencertDays, "days", 365, "certificate validity period in days")
	gencertCmd.Flags().StringVar(&gencertOrg, "org", "Radix Development", "organization name in certificate subject")
	gencertCmd.Flags().IntVar(&gencertKeySize, "key-size", 2048, "RSA key size in bits (2048, 4096)")
	gencertCmd.Flags().StringVar(&gencertKeyType, "key-type", "rsa", "key type: rsa, ecdsa")
	gencertCmd.Flags().StringVar(&gencertECDSACurve, "ecdsa-curve", "P-256", "ECDSA curve: P-256, P-384, P-521")
	gencertCmd.Flags().BoolVar(&gencertCA, "ca", true, "generate a CA certificate")
	gencertCmd.Flags().StringVar(&gencertCACert, "ca-cert", "", "path to existing CA certificate PEM file")
	gencertCmd.Flags().StringVar(&gencertCAKey, "ca-key", "", "path to existing CA private key PEM file")
	gencertCmd.Flags().BoolVar(&gencertClient, "client", false, "generate a client certificate instead of a server certificate")
	gencertCmd.Flags().BoolVar(&gencertOverwrite, "overwrite", false, "overwrite existing certificate files")
}

func runGencert(cmd *cobra.Command, _ []string) error {
	out := cmd.OutOrStdout()

	// Parse hosts
	hosts := parseHosts(gencertHosts)
	if len(hosts) == 0 {
		return fmt.Errorf("at least one host is required")
	}

	// Validate key type
	keyType := tls.KeyType(strings.ToLower(gencertKeyType))
	if keyType != tls.KeyTypeRSA && keyType != tls.KeyTypeECDSA {
		return fmt.Errorf("unsupported key type %q: must be rsa or ecdsa", gencertKeyType)
	}

	// Validate ECDSA curve
	ecdsaCurve := tls.ECDSACurve(gencertECDSACurve)
	if keyType == tls.KeyTypeECDSA {
		switch ecdsaCurve {
		case tls.CurveP256, tls.CurveP384, tls.CurveP521:
			// valid
		default:
			return fmt.Errorf("unsupported ECDSA curve %q: must be P-256, P-384, or P-521", gencertECDSACurve)
		}
	}

	// Validate RSA key size
	if keyType == tls.KeyTypeRSA {
		switch gencertKeySize {
		case 2048, 4096:
			// valid
		default:
			return fmt.Errorf("unsupported RSA key size %d: must be 2048 or 4096", gencertKeySize)
		}
	}

	// Validate days
	if gencertDays <= 0 {
		return fmt.Errorf("certificate validity must be positive, got %d", gencertDays)
	}

	// Validate that ca-cert and ca-key are both provided or both absent
	if (gencertCACert == "") != (gencertCAKey == "") {
		return fmt.Errorf("--ca-cert and --ca-key must be provided together")
	}

	certCfg := &tls.CertConfig{
		Hosts:        hosts,
		Days:         gencertDays,
		Organization: gencertOrg,
		KeyType:      keyType,
		KeySize:      gencertKeySize,
		ECDSACurve:   ecdsaCurve,
		IsClient:     gencertClient,
	}

	outputDir, err := filepath.Abs(gencertOutput)
	if err != nil {
		return fmt.Errorf("resolving output directory: %w", err)
	}

	var caCert *tls.Certificate
	var generatedCA bool

	if gencertCACert != "" && gencertCAKey != "" {
		// Load existing CA
		fmt.Fprintln(out, "Loading existing CA certificate...")

		caCertPEM, err := os.ReadFile(gencertCACert)
		if err != nil {
			return fmt.Errorf("reading CA certificate: %w", err)
		}
		caKeyPEM, err := os.ReadFile(gencertCAKey)
		if err != nil {
			return fmt.Errorf("reading CA private key: %w", err)
		}

		caCert = &tls.Certificate{CertPEM: caCertPEM, KeyPEM: caKeyPEM}
		fmt.Fprintf(out, "  CA certificate: %s\n", gencertCACert)
		fmt.Fprintf(out, "  CA private key: %s\n", gencertCAKey)
	} else if gencertCA {
		// Generate new CA
		fmt.Fprintln(out, "Generating CA certificate...")

		caCfg := &tls.CertConfig{
			Hosts:        hosts,
			Days:         gencertDays,
			Organization: gencertOrg,
			KeyType:      keyType,
			KeySize:      gencertKeySize,
			ECDSACurve:   ecdsaCurve,
			IsCA:         true,
		}

		caCert, err = tls.GenerateCA(caCfg)
		if err != nil {
			return fmt.Errorf("generating CA certificate: %w", err)
		}

		if err := tls.WriteCertFiles(outputDir, "ca.pem", "ca-key.pem", caCert, gencertOverwrite); err != nil {
			return fmt.Errorf("writing CA certificate files: %w", err)
		}

		generatedCA = true
		fmt.Fprintf(out, "  CA certificate: %s\n", filepath.Join(outputDir, "ca.pem"))
		fmt.Fprintf(out, "  CA private key: %s\n", filepath.Join(outputDir, "ca-key.pem"))
	} else {
		return fmt.Errorf("either --ca must be true or --ca-cert and --ca-key must be provided")
	}

	// Parse the CA certificate and key for signing
	parsedCACert, err := tls.ParseCertificate(caCert.CertPEM)
	if err != nil {
		return fmt.Errorf("parsing CA certificate: %w", err)
	}
	parsedCAKey, err := tls.ParsePrivateKey(caCert.KeyPEM)
	if err != nil {
		return fmt.Errorf("parsing CA private key: %w", err)
	}

	// Generate server or client certificate
	certType := "server"
	if gencertClient {
		certType = "client"
	}
	fmt.Fprintf(out, "Generating %s certificate...\n", certType)

	var cert *tls.Certificate
	if gencertClient {
		cert, err = tls.GenerateClientCert(certCfg, parsedCACert, parsedCAKey)
	} else {
		cert, err = tls.GenerateServerCert(certCfg, parsedCACert, parsedCAKey)
	}
	if err != nil {
		return fmt.Errorf("generating %s certificate: %w", certType, err)
	}

	if err := tls.WriteCertFiles(outputDir, "cert.pem", "key.pem", cert, gencertOverwrite); err != nil {
		return fmt.Errorf("writing %s certificate files: %w", certType, err)
	}

	certTypeLabel := capitalize(certType)
	fmt.Fprintf(out, "  %s certificate: %s\n", certTypeLabel, filepath.Join(outputDir, "cert.pem"))
	fmt.Fprintf(out, "  %s private key: %s\n", certTypeLabel, filepath.Join(outputDir, "key.pem"))

	// Write README.txt
	if err := tls.WriteReadme(outputDir, hosts, gencertOverwrite); err != nil {
		return fmt.Errorf("writing README.txt: %w", err)
	}

	// Print summary
	fmt.Fprintln(out)
	fmt.Fprintln(out, "Certificate generation complete!")
	fmt.Fprintf(out, "  Output directory: %s\n", outputDir)
	fmt.Fprintf(out, "  Hosts:           %s\n", strings.Join(hosts, ", "))
	fmt.Fprintf(out, "  Key type:        %s\n", keyType)
	fmt.Fprintf(out, "  Validity:        %d days\n", gencertDays)
	fmt.Fprintf(out, "  Organization:    %s\n", gencertOrg)

	if generatedCA {
		fmt.Fprintln(out)
		fmt.Fprintln(out, "To trust the CA certificate, see README.txt in the output directory.")
	}

	return nil
}

// capitalize returns the string with the first letter uppercased.
func capitalize(s string) string {
	if s == "" {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}

// parseHosts splits a comma-separated string of hosts and trims whitespace.
func parseHosts(hosts string) []string {
	parts := strings.Split(hosts, ",")
	var result []string
	for _, h := range parts {
		h = strings.TrimSpace(h)
		if h != "" {
			result = append(result, h)
		}
	}
	return result
}
