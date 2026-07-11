package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestGencertCmd_Registered(t *testing.T) {
	found := false
	for _, cmd := range rootCmd.Commands() {
		if cmd.Name() == "gencert" {
			found = true
			break
		}
	}
	if !found {
		t.Error("gencert command not registered on root command")
	}
}

func TestGencertCmd_Flags(t *testing.T) {
	flags := []string{
		"host", "output", "days", "org", "key-size", "key-type",
		"ecdsa-curve", "ca", "ca-cert", "ca-key", "client", "overwrite",
	}
	for _, name := range flags {
		if gencertCmd.Flags().Lookup(name) == nil {
			t.Errorf("flag %q not registered on gencert command", name)
		}
	}
}

func TestParseHosts(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  []string
	}{
		{"single host", "localhost", []string{"localhost"}},
		{"multiple hosts", "localhost,127.0.0.1,myapp", []string{"localhost", "127.0.0.1", "myapp"}},
		{"whitespace trimmed", " localhost , 127.0.0.1 ", []string{"localhost", "127.0.0.1"}},
		{"empty segments dropped", "localhost,,127.0.0.1,", []string{"localhost", "127.0.0.1"}},
		{"only separators", ", ,", nil},
		{"empty string", "", nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseHosts(tt.input)
			if len(got) != len(tt.want) {
				t.Fatalf("parseHosts(%q) = %v, want %v", tt.input, got, tt.want)
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("parseHosts(%q)[%d] = %q, want %q", tt.input, i, got[i], tt.want[i])
				}
			}
		})
	}
}

func TestCapitalize(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"", ""},
		{"server", "Server"},
		{"client", "Client"},
		{"S", "S"},
		{"already Capitalized", "Already Capitalized"},
	}
	for _, tt := range tests {
		if got := capitalize(tt.input); got != tt.want {
			t.Errorf("capitalize(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestRunGencert_ValidationErrors(t *testing.T) {
	tests := []struct {
		name    string
		mutate  func()
		wantSub string
	}{
		{
			name:    "no hosts",
			mutate:  func() { gencertHosts = " , ," },
			wantSub: "at least one host",
		},
		{
			name:    "unsupported key type",
			mutate:  func() { gencertKeyType = "dsa" },
			wantSub: "unsupported key type",
		},
		{
			name: "unsupported ecdsa curve",
			mutate: func() {
				gencertKeyType = "ecdsa"
				gencertECDSACurve = "P-999"
			},
			wantSub: "unsupported ECDSA curve",
		},
		{
			name:    "unsupported rsa key size",
			mutate:  func() { gencertKeySize = 1024 },
			wantSub: "unsupported RSA key size",
		},
		{
			name:    "non-positive days",
			mutate:  func() { gencertDays = 0 },
			wantSub: "must be positive",
		},
		{
			name:    "ca-cert without ca-key",
			mutate:  func() { gencertCACert = "/tmp/ca.pem" },
			wantSub: "must be provided together",
		},
		{
			name:    "ca-key without ca-cert",
			mutate:  func() { gencertCAKey = "/tmp/ca-key.pem" },
			wantSub: "must be provided together",
		},
		{
			name:    "no CA source at all",
			mutate:  func() { gencertCA = false },
			wantSub: "either --ca must be true",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resetGencertFlags(t)
			gencertOutput = t.TempDir()
			tt.mutate()

			var buf bytes.Buffer
			gencertCmd.SetOut(&buf)
			t.Cleanup(func() { gencertCmd.SetOut(nil) })

			err := runGencert(gencertCmd, nil)
			if err == nil {
				t.Fatalf("expected error for %s, got nil", tt.name)
			}
			if !strings.Contains(err.Error(), tt.wantSub) {
				t.Errorf("error = %v, want it to contain %q", err, tt.wantSub)
			}
		})
	}
}

func TestRunGencert_GeneratesCAAndServerCert(t *testing.T) {
	resetGencertFlags(t)
	dir := t.TempDir()
	gencertOutput = dir
	gencertKeyType = "ecdsa"
	gencertHosts = "localhost,127.0.0.1"

	var buf bytes.Buffer
	gencertCmd.SetOut(&buf)
	t.Cleanup(func() { gencertCmd.SetOut(nil) })

	if err := runGencert(gencertCmd, nil); err != nil {
		t.Fatalf("runGencert failed: %v", err)
	}

	for _, name := range []string{"ca.pem", "ca-key.pem", "cert.pem", "key.pem", "README.txt"} {
		if _, err := os.Stat(filepath.Join(dir, name)); err != nil {
			t.Errorf("expected %s to exist: %v", name, err)
		}
	}

	out := buf.String()
	if !strings.Contains(out, "Certificate generation complete!") {
		t.Errorf("output missing completion banner; got:\n%s", out)
	}
	if !strings.Contains(out, "To trust the CA certificate") {
		t.Errorf("output missing generated-CA trust hint; got:\n%s", out)
	}
}

func TestRunGencert_RefusesOverwriteWithoutFlag(t *testing.T) {
	resetGencertFlags(t)
	dir := t.TempDir()
	gencertOutput = dir
	gencertKeyType = "ecdsa"

	var buf bytes.Buffer
	gencertCmd.SetOut(&buf)
	t.Cleanup(func() { gencertCmd.SetOut(nil) })

	if err := runGencert(gencertCmd, nil); err != nil {
		t.Fatalf("first runGencert failed: %v", err)
	}

	// Second run into the same directory must refuse without --overwrite.
	if err := runGencert(gencertCmd, nil); err == nil {
		t.Fatal("expected error when regenerating without --overwrite, got nil")
	}

	// With --overwrite it must succeed.
	gencertOverwrite = true
	if err := runGencert(gencertCmd, nil); err != nil {
		t.Errorf("runGencert with --overwrite failed: %v", err)
	}
}

func TestRunGencert_ClientCertWithExistingCA(t *testing.T) {
	// First generate a CA to reuse.
	_, _, caPath := genTestCerts(t)
	caKeyPath := filepath.Join(filepath.Dir(caPath), "ca-key.pem")

	resetGencertFlags(t)
	dir := t.TempDir()
	gencertOutput = dir
	gencertKeyType = "ecdsa"
	gencertCACert = caPath
	gencertCAKey = caKeyPath
	gencertClient = true

	var buf bytes.Buffer
	gencertCmd.SetOut(&buf)
	t.Cleanup(func() { gencertCmd.SetOut(nil) })

	if err := runGencert(gencertCmd, nil); err != nil {
		t.Fatalf("runGencert (client cert, existing CA) failed: %v", err)
	}

	for _, name := range []string{"cert.pem", "key.pem"} {
		if _, err := os.Stat(filepath.Join(dir, name)); err != nil {
			t.Errorf("expected %s to exist: %v", name, err)
		}
	}
	// A reused CA must not be regenerated into the output directory.
	if _, err := os.Stat(filepath.Join(dir, "ca.pem")); !os.IsNotExist(err) {
		t.Error("ca.pem should not be written when reusing an existing CA")
	}

	out := buf.String()
	if !strings.Contains(out, "Loading existing CA certificate") {
		t.Errorf("output missing existing-CA message; got:\n%s", out)
	}
	if !strings.Contains(out, "client certificate") {
		t.Errorf("output missing client certificate message; got:\n%s", out)
	}
}

func TestRunGencert_ExistingCAErrors(t *testing.T) {
	// A valid CA pair to mix and match with broken counterparts.
	_, _, caPath := genTestCerts(t)
	caKeyPath := filepath.Join(filepath.Dir(caPath), "ca-key.pem")

	dir := t.TempDir()
	garbage := filepath.Join(dir, "garbage.pem")
	if err := os.WriteFile(garbage, []byte("not a pem"), 0o600); err != nil {
		t.Fatalf("write garbage file: %v", err)
	}
	missing := filepath.Join(dir, "missing.pem")

	tests := []struct {
		name    string
		caCert  string
		caKey   string
		wantSub string
	}{
		{"missing ca cert file", missing, caKeyPath, "reading CA certificate"},
		{"missing ca key file", caPath, missing, "reading CA private key"},
		{"unparsable ca cert", garbage, caKeyPath, "parsing CA certificate"},
		{"unparsable ca key", caPath, garbage, "parsing CA private key"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resetGencertFlags(t)
			gencertOutput = t.TempDir()
			gencertKeyType = "ecdsa"
			gencertCACert = tt.caCert
			gencertCAKey = tt.caKey

			var buf bytes.Buffer
			gencertCmd.SetOut(&buf)
			t.Cleanup(func() { gencertCmd.SetOut(nil) })

			err := runGencert(gencertCmd, nil)
			if err == nil {
				t.Fatalf("expected error for %s, got nil", tt.name)
			}
			if !strings.Contains(err.Error(), tt.wantSub) {
				t.Errorf("error = %v, want it to contain %q", err, tt.wantSub)
			}
		})
	}
}
