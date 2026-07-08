package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// writeTempYAML writes the given YAML to a file with the given name in a fresh
// temp dir and returns its path. It is a local helper so this file does not
// depend on helpers owned by other validate test files.
func writeTempYAML(t *testing.T, name, yamlText string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, []byte(yamlText), 0o600); err != nil {
		t.Fatalf("failed to write temp file: %v", err)
	}
	return path
}

// runValidateTyped invokes the validate command against path with the given
// --type value and strict mode, restoring the package-level flag variables
// afterwards. It returns the captured output and the command error.
func runValidateTyped(t *testing.T, path, typ string, strict bool) (string, error) {
	t.Helper()
	prevType, prevStrict := configType, strictMode
	t.Cleanup(func() {
		configType = prevType
		strictMode = prevStrict
	})
	configType = typ
	strictMode = strict

	var buf bytes.Buffer
	validateCmd.SetOut(&buf)
	validateCmd.SetErr(&buf)
	err := runValidate(validateCmd, []string{path})
	return buf.String(), err
}

func TestValidateType_AutoDetectExampleRoutesFile(t *testing.T) {
	out, err := runValidateTyped(t, filepath.Join("..", "..", "examples", "mock-routes.yml"), "auto", false)
	if err != nil {
		t.Fatalf("expected examples/mock-routes.yml to validate, got: %v", err)
	}
	if !strings.Contains(out, "✓ Routes:") {
		t.Errorf("output missing routes-compiled line:\n%s", out)
	}
	if !strings.Contains(out, "✓ Mock routes file is valid") {
		t.Errorf("output missing mock-routes success line:\n%s", out)
	}
	if strings.Contains(out, "Configuration is valid") {
		t.Errorf("routes file was validated as a main config:\n%s", out)
	}
}

func TestValidateType_AutoDetect(t *testing.T) {
	tests := []struct {
		name       string
		yaml       string
		wantErr    string // substring of the returned error; empty = expect success
		wantOutput string // substring of the command output
	}{
		{
			name: "valid routes file via routes key",
			yaml: `
routes:
  - path: /api/health
    method: GET
    response:
      status: 200
      body: '{"status":"ok"}'
`,
			wantOutput: "✓ Routes: 1 compiled",
		},
		{
			name: "settings-only file detected as mock-routes",
			yaml: `
settings:
  latency: 0
`,
			wantOutput: "✓ Mock routes file is valid",
		},
		{
			name: "bad routes file fails with compile error",
			yaml: `
routes:
  - path: "regex:^/api/[unclosed"
    response:
      status: 200
`,
			wantErr: "✗ Routes:",
		},
		{
			name: "route missing path fails",
			yaml: `
routes:
  - method: GET
    response:
      status: 200
`,
			wantErr: "path is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := writeTempYAML(t, "routes.yml", tt.yaml)
			out, err := runValidateTyped(t, path, "auto", false)

			if tt.wantErr != "" {
				if err == nil {
					t.Fatalf("expected error containing %q, got nil\noutput:\n%s", tt.wantErr, out)
				}
				if !strings.Contains(err.Error(), tt.wantErr) {
					t.Errorf("error = %q, want it to contain %q", err.Error(), tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if tt.wantOutput != "" && !strings.Contains(out, tt.wantOutput) {
				t.Errorf("output = %q, want it to contain %q", out, tt.wantOutput)
			}
		})
	}
}

func TestValidateType_RoutesFileNoLongerFalsePositivesAsMainConfig(t *testing.T) {
	// This routes file is BROKEN (invalid regex). Validated as a main config,
	// viper would silently ignore the unknown `routes` key and report the file
	// as valid — the false positive the --type flag exists to prevent. In auto
	// mode it must be detected as mock-routes and fail.
	path := writeTempYAML(t, "broken-routes.yml", `
routes:
  - path: "regex:^/api/[unclosed"
    response:
      status: 200
`)

	out, err := runValidateTyped(t, path, "auto", false)
	if err == nil {
		t.Fatalf("expected a broken routes file to fail validation, got success\noutput:\n%s", out)
	}
	if strings.Contains(out, "Configuration is valid") {
		t.Errorf("broken routes file was reported valid as a main config:\n%s", out)
	}
}

func TestValidateType_ForceMockRoutes(t *testing.T) {
	// No routes/settings key, so auto-detect would treat this as a main config;
	// --type mock-routes must force the routes path (compiling zero routes).
	path := writeTempYAML(t, "empty.yml", "# nothing here\n")

	out, err := runValidateTyped(t, path, "mock-routes", false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out, "✓ Routes: 0 compiled") {
		t.Errorf("output = %q, want zero-routes compile line", out)
	}
	if !strings.Contains(out, "⚠ No routes defined") {
		t.Errorf("output = %q, want no-routes warning", out)
	}

	// The zero-routes warning becomes fatal under --strict.
	_, err = runValidateTyped(t, path, "mock-routes", true)
	if err == nil {
		t.Fatal("expected --strict to fail on the no-routes warning")
	}
	if !strings.Contains(err.Error(), "--strict") {
		t.Errorf("error = %q, want it to mention --strict", err.Error())
	}
}

func TestValidateType_ForceMain(t *testing.T) {
	// A routes file forced to --type main goes down the main-config path (and,
	// because viper ignores unknown keys, passes). Forcing is explicit user
	// intent, so this documents rather than prevents that behavior.
	path := writeTempYAML(t, "routes.yml", `
routes:
  - path: /api/health
    response:
      status: 200
`)

	out, err := runValidateTyped(t, path, "main", false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out, "Configuration is valid") {
		t.Errorf("output = %q, want main-config success line", out)
	}
	if strings.Contains(out, "Mock routes file is valid") {
		t.Errorf("--type main still took the mock-routes path:\n%s", out)
	}
}

func TestValidateType_InvalidTypeValue(t *testing.T) {
	path := writeTempYAML(t, "radix.yml", "port: 8080\n")

	_, err := runValidateTyped(t, path, "bogus", false)
	if err == nil {
		t.Fatal("expected an error for an invalid --type value")
	}
	if !strings.Contains(err.Error(), "--type") {
		t.Errorf("error = %q, want it to name the --type flag", err.Error())
	}
	if !strings.Contains(err.Error(), "bogus") {
		t.Errorf("error = %q, want it to include the bad value", err.Error())
	}
}
