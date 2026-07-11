//revive:disable:var-naming // "version" is intentional for an internal package
package version

import (
	"runtime"
	"strings"
	"testing"
)

func TestGetInfo(t *testing.T) {
	info := GetInfo()

	// Check that fields are populated
	if info.Version == "" {
		t.Error("Version should not be empty")
	}

	if info.Compiler == "" {
		t.Error("Compiler should not be empty")
	}

	if info.GoOS == "" {
		t.Error("GoOS should not be empty")
	}

	if info.GoArch == "" {
		t.Error("GoArch should not be empty")
	}

	// Verify Compiler matches runtime
	if info.Compiler != runtime.Version() {
		t.Errorf("Compiler mismatch: got %s, want %s", info.Compiler, runtime.Version())
	}

	// Verify GoOS and GoArch
	if info.GoOS != runtime.GOOS {
		t.Errorf("GoOS mismatch: got %s, want %s", info.GoOS, runtime.GOOS)
	}

	if info.GoArch != runtime.GOARCH {
		t.Errorf("GoArch mismatch: got %s, want %s", info.GoArch, runtime.GOARCH)
	}
}

func TestInfoString(t *testing.T) {
	// Set test values
	Version = "1.0.0"
	Commit = "abc123"
	Date = "2025-12-31"

	info := GetInfo()
	result := info.String()

	// Check that the string contains expected components
	if !strings.Contains(result, "1.0.0") {
		t.Error("String should contain version")
	}

	if !strings.Contains(result, "abc123") {
		t.Error("String should contain commit")
	}

	if !strings.Contains(result, "2025-12-31") {
		t.Error("String should contain build date")
	}

	if !strings.Contains(result, "radix") {
		t.Error("String should contain 'radix'")
	}
}

func TestInfoShort(t *testing.T) {
	Version = "1.2.3"
	info := GetInfo()
	result := info.Short()

	if result != "1.2.3" {
		t.Errorf("Short() = %s, want 1.2.3", result)
	}
}

func TestUserAgent(t *testing.T) {
	tests := []struct {
		name    string
		version string
		want    string
	}{
		{name: "dev version", version: "dev", want: "radix/dev"},
		{name: "semver version", version: "1.2.3", want: "radix/1.2.3"},
		{name: "tagged version", version: "v0.7.1", want: "radix/v0.7.1"},
	}

	orig := Version
	defer func() { Version = orig }()

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			Version = tt.version
			if got := UserAgent(); got != tt.want {
				t.Errorf("UserAgent() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestVersionDefaults(t *testing.T) {
	// Reset to defaults
	Version = "dev"
	Commit = "none"
	Date = "unknown"

	info := GetInfo()

	if info.Version != "dev" {
		t.Errorf("Default version should be 'dev', got %s", info.Version)
	}

	if info.Commit != "none" {
		t.Errorf("Default commit should be 'none', got %s", info.Commit)
	}

	if info.Date != "unknown" {
		t.Errorf("Default date should be 'unknown', got %s", info.Date)
	}
}
