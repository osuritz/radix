package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// writeTempConfig writes the given YAML to a temp file and returns its path.
func writeTempConfig(t *testing.T, yaml string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "radix.yml")
	if err := os.WriteFile(path, []byte(yaml), 0o600); err != nil {
		t.Fatalf("failed to write temp config: %v", err)
	}
	return path
}

// runValidateOn invokes the validate command against the given config file path.
func runValidateOn(t *testing.T, path string) error {
	t.Helper()
	var buf bytes.Buffer
	validateCmd.SetOut(&buf)
	validateCmd.SetErr(&buf)
	return runValidate(validateCmd, []string{path})
}

func TestValidate_HSTSWithoutTLSFails(t *testing.T) {
	path := writeTempConfig(t, `
port: 8443
serve:
  hsts: true
tls:
  enabled: false
`)

	err := runValidateOn(t, path)
	if err == nil {
		t.Fatal("expected validation error for serve.hsts without TLS")
	}
	if !strings.Contains(err.Error(), "--hsts requires --tls") {
		t.Errorf("error = %q, want it to mention '--hsts requires --tls'", err.Error())
	}
}

func TestValidate_HTTPRedirectSamePortFails(t *testing.T) {
	// Create real cert/key files so the TLS checks pass and validation reaches
	// the serve TLS-coupling rules.
	dir := t.TempDir()
	certPath := filepath.Join(dir, "cert.pem")
	keyPath := filepath.Join(dir, "key.pem")
	if err := os.WriteFile(certPath, []byte("cert"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(keyPath, []byte("key"), 0o600); err != nil {
		t.Fatal(err)
	}

	cfgPath := filepath.Join(dir, "radix.yml")
	yaml := "" +
		"port: 8443\n" +
		"tls:\n" +
		"  enabled: true\n" +
		"  min_version: \"1.3\"\n" +
		"  cert: " + certPath + "\n" +
		"  key: " + keyPath + "\n" +
		"serve:\n" +
		"  http_redirect: true\n" +
		"  http_port: 8443\n"
	if err := os.WriteFile(cfgPath, []byte(yaml), 0o600); err != nil {
		t.Fatal(err)
	}

	err := runValidateOn(t, cfgPath)
	if err == nil {
		t.Fatal("expected validation error for http_redirect with http_port == port")
	}
	if !strings.Contains(err.Error(), "must differ from --port") {
		t.Errorf("error = %q, want it to mention 'must differ from --port'", err.Error())
	}
}

func TestValidate_NegativeHSTSMaxAgeFails(t *testing.T) {
	path := writeTempConfig(t, `
port: 8443
serve:
  hsts_max_age: -1
`)

	err := runValidateOn(t, path)
	if err == nil {
		t.Fatal("expected validation error for negative hsts_max_age")
	}
	if !strings.Contains(err.Error(), "must not be negative") {
		t.Errorf("error = %q, want it to mention 'must not be negative'", err.Error())
	}
}
