package cli

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/osuritz/radix/internal/version"
)

// setVersionFlags sets the version command's output-format globals for the
// duration of a test, restoring the previous values on cleanup.
func setVersionFlags(t *testing.T, short, jsonOut bool) {
	t.Helper()
	oldShort, oldJSON := shortVersion, jsonOutput
	t.Cleanup(func() { shortVersion, jsonOutput = oldShort, oldJSON })
	shortVersion = short
	jsonOutput = jsonOut
}

func TestVersionCmd_Registered(t *testing.T) {
	found := false
	for _, cmd := range rootCmd.Commands() {
		if cmd.Name() == "version" {
			found = true
			break
		}
	}
	if !found {
		t.Error("version command not registered on root command")
	}
}

func TestVersionCmd_Flags(t *testing.T) {
	for _, name := range []string{"short", "json"} {
		if versionCmd.Flags().Lookup(name) == nil {
			t.Errorf("flag %q not registered on version command", name)
		}
	}
}

func TestRunVersion_Formats(t *testing.T) {
	info := version.GetInfo()

	tests := []struct {
		name    string
		short   bool
		jsonOut bool
		check   func(t *testing.T, out string)
	}{
		{
			name: "full output",
			check: func(t *testing.T, out string) {
				t.Helper()
				if strings.TrimSuffix(out, "\n") != info.String() {
					t.Errorf("output = %q, want %q", out, info.String())
				}
			},
		},
		{
			name:  "short output",
			short: true,
			check: func(t *testing.T, out string) {
				t.Helper()
				if strings.TrimSuffix(out, "\n") != info.Short() {
					t.Errorf("output = %q, want %q", out, info.Short())
				}
			},
		},
		{
			name:    "json output",
			jsonOut: true,
			check: func(t *testing.T, out string) {
				t.Helper()
				var decoded map[string]any
				if err := json.Unmarshal([]byte(out), &decoded); err != nil {
					t.Fatalf("output is not valid JSON: %v\n%s", err, out)
				}
				if len(decoded) == 0 {
					t.Error("JSON output decoded to an empty object")
				}
			},
		},
		{
			name:    "json wins over short",
			short:   true,
			jsonOut: true,
			check: func(t *testing.T, out string) {
				t.Helper()
				if !json.Valid([]byte(out)) {
					t.Errorf("expected JSON output when both --json and --short set, got %q", out)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			setVersionFlags(t, tt.short, tt.jsonOut)

			var buf bytes.Buffer
			versionCmd.SetOut(&buf)
			t.Cleanup(func() { versionCmd.SetOut(nil) })

			if err := runVersion(versionCmd, nil); err != nil {
				t.Fatalf("runVersion returned error: %v", err)
			}
			tt.check(t, buf.String())
		})
	}
}
