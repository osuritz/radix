package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestValidateCmd_Registered(t *testing.T) {
	found := false
	for _, cmd := range rootCmd.Commands() {
		if cmd.Name() == "validate" {
			found = true
			break
		}
	}
	if !found {
		t.Error("validate command not registered on root command")
	}
}

// runValidateCapture invokes runValidate on the given path and returns the
// error along with everything written to the command output.
func runValidateCapture(t *testing.T, path string) (string, error) {
	t.Helper()
	var buf bytes.Buffer
	validateCmd.SetOut(&buf)
	validateCmd.SetErr(&buf)
	t.Cleanup(func() {
		validateCmd.SetOut(nil)
		validateCmd.SetErr(nil)
	})
	err := runValidate(validateCmd, []string{path})
	return buf.String(), err
}

func TestValidatePath(t *testing.T) {
	dir := t.TempDir()
	okFile := filepath.Join(dir, "ok.yml")
	if err := os.WriteFile(okFile, []byte("port: 8080\n"), 0o600); err != nil {
		t.Fatalf("write file: %v", err)
	}

	tests := []struct {
		name    string
		path    string
		wantSub string
	}{
		{"readable file", okFile, ""},
		{"empty path", "", "path is empty"},
		{"missing file", filepath.Join(dir, "missing.yml"), "not found"},
		{"directory", dir, "is a directory"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validatePath(tt.path, "test-file")
			if tt.wantSub == "" {
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
				return
			}
			if err == nil {
				t.Fatalf("expected error containing %q, got nil", tt.wantSub)
			}
			if !strings.Contains(err.Error(), tt.wantSub) {
				t.Errorf("error = %v, want it to contain %q", err, tt.wantSub)
			}
		})
	}
}

func TestRunValidate_ValidMinimalConfig(t *testing.T) {
	path := writeTempConfig(t, "port: 8080\nhost: localhost\n")

	out, err := runValidateCapture(t, path)
	if err != nil {
		t.Fatalf("expected valid config, got error: %v", err)
	}
	for _, want := range []string{"Syntax: OK", "Schema: OK", "Configuration is valid"} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q; got:\n%s", want, out)
		}
	}
}

func TestRunValidate_ErrorCases(t *testing.T) {
	dir := t.TempDir()
	certPath := filepath.Join(dir, "cert.pem")
	keyPath := filepath.Join(dir, "key.pem")
	for _, p := range []string{certPath, keyPath} {
		if err := os.WriteFile(p, []byte("pem"), 0o600); err != nil {
			t.Fatalf("write %s: %v", p, err)
		}
	}

	tests := []struct {
		name    string
		yaml    string
		wantSub string
	}{
		{
			name:    "syntax error",
			yaml:    "port: [unclosed\n  nested: {",
			wantSub: "Syntax error",
		},
		{
			name:    "port too low",
			yaml:    "port: 0\n",
			wantSub: "Invalid port",
		},
		{
			name:    "port too high",
			yaml:    "port: 70000\n",
			wantSub: "Invalid port",
		},
		{
			name:    "metrics port collides with app port",
			yaml:    "port: 8081\nmetrics:\n  enabled: true\n  port: 8081\n",
			wantSub: "Metrics configuration",
		},
		{
			name:    "tls enabled without cert",
			yaml:    "port: 8443\ntls:\n  enabled: true\n",
			wantSub: "cert file not specified",
		},
		{
			name:    "tls enabled without key",
			yaml:    "port: 8443\ntls:\n  enabled: true\n  cert: " + certPath + "\n",
			wantSub: "key file not specified",
		},
		{
			name:    "tls cert file missing",
			yaml:    "port: 8443\ntls:\n  enabled: true\n  cert: /nonexistent/cert.pem\n  key: " + keyPath + "\n",
			wantSub: "Certificate file",
		},
		{
			name:    "tls key file missing",
			yaml:    "port: 8443\ntls:\n  enabled: true\n  cert: " + certPath + "\n  key: /nonexistent/key.pem\n",
			wantSub: "Key file",
		},
		{
			name: "invalid tls min version",
			yaml: "port: 8443\ntls:\n  enabled: true\n  cert: " + certPath +
				"\n  key: " + keyPath + "\n  min_version: \"1.1\"\n",
			wantSub: "Invalid TLS min_version",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := writeTempConfig(t, tt.yaml)
			_, err := runValidateCapture(t, path)
			if err == nil {
				t.Fatalf("expected error containing %q, got nil", tt.wantSub)
			}
			if !strings.Contains(err.Error(), tt.wantSub) {
				t.Errorf("error = %v, want it to contain %q", err, tt.wantSub)
			}
		})
	}
}

func TestRunValidate_MissingFile(t *testing.T) {
	_, err := runValidateCapture(t, filepath.Join(t.TempDir(), "missing.yml"))
	if err == nil {
		t.Fatal("expected error for missing config file, got nil")
	}
	if !strings.Contains(err.Error(), "does not exist") {
		t.Errorf("error = %v, want a does-not-exist message", err)
	}
}

func TestRunValidate_WarningsNonStrict(t *testing.T) {
	dir := t.TempDir()
	certPath := filepath.Join(dir, "cert.pem")
	keyPath := filepath.Join(dir, "key.pem")
	notADir := filepath.Join(dir, "file.txt")
	for _, p := range []string{certPath, keyPath, notADir} {
		if err := os.WriteFile(p, []byte("x"), 0o600); err != nil {
			t.Fatalf("write %s: %v", p, err)
		}
	}

	yaml := "port: 8443\n" +
		"tls:\n" +
		"  enabled: true\n" +
		"  cert: " + certPath + "\n" +
		"  key: " + keyPath + "\n" +
		"  ca: /nonexistent/ca.pem\n" + // warning: CA file not found
		"  min_version: \"1.2\"\n" + // warning: TLS 1.2 advisory
		"serve:\n" +
		"  dir: " + notADir + "\n" + // warning: serve path is not a directory
		"mock:\n" +
		"  routes: /nonexistent/routes.yml\n" // warning: routes file not found
	path := writeTempConfig(t, yaml)

	out, err := runValidateCapture(t, path)
	if err != nil {
		t.Fatalf("warnings must not fail outside strict mode; got %v", err)
	}
	for _, want := range []string{
		"Warnings:",
		"CA file not found",
		"min_version to '1.3'",
		"not a directory",
		"Mock routes file not found",
		"Configuration is valid",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q; got:\n%s", want, out)
		}
	}
}

func TestRunValidate_ServeDirNotFoundWarning(t *testing.T) {
	path := writeTempConfig(t, "port: 8080\nserve:\n  dir: /nonexistent/dir\n")

	out, err := runValidateCapture(t, path)
	if err != nil {
		t.Fatalf("warnings must not fail outside strict mode; got %v", err)
	}
	if !strings.Contains(out, "Serve directory not found") {
		t.Errorf("output missing serve-directory warning; got:\n%s", out)
	}
}

func TestRunValidate_StrictModeFailsOnWarnings(t *testing.T) {
	oldStrict := strictMode
	t.Cleanup(func() { strictMode = oldStrict })
	strictMode = true

	path := writeTempConfig(t, "port: 8080\nserve:\n  dir: /nonexistent/dir\n")

	_, err := runValidateCapture(t, path)
	if err == nil {
		t.Fatal("expected strict mode to fail on warnings, got nil")
	}
	if !strings.Contains(err.Error(), "--strict") && !strings.Contains(err.Error(), "strict mode") {
		t.Errorf("error = %v, want a strict-mode failure", err)
	}
}

func TestRunValidate_UsesGlobalConfigFlagWhenNoArg(t *testing.T) {
	oldCfgFile := cfgFile
	t.Cleanup(func() { cfgFile = oldCfgFile })
	cfgFile = writeTempConfig(t, "port: 8080\n")

	var buf bytes.Buffer
	validateCmd.SetOut(&buf)
	validateCmd.SetErr(&buf)
	t.Cleanup(func() {
		validateCmd.SetOut(nil)
		validateCmd.SetErr(nil)
	})

	if err := runValidate(validateCmd, nil); err != nil {
		t.Fatalf("expected valid config via --config path, got %v", err)
	}
	if !strings.Contains(buf.String(), "Configuration is valid") {
		t.Errorf("output missing validity confirmation; got:\n%s", buf.String())
	}
}
