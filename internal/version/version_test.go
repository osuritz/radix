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

	if info.GoVersion == "" {
		t.Error("GoVersion should not be empty")
	}

	if info.Platform == "" {
		t.Error("Platform should not be empty")
	}

	// Verify GoVersion matches runtime
	if info.GoVersion != runtime.Version() {
		t.Errorf("GoVersion mismatch: got %s, want %s", info.GoVersion, runtime.Version())
	}

	// Verify Platform format
	expectedPlatform := runtime.GOOS + "/" + runtime.GOARCH
	if info.Platform != expectedPlatform {
		t.Errorf("Platform mismatch: got %s, want %s", info.Platform, expectedPlatform)
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

	if !strings.Contains(result, "radix version") {
		t.Error("String should contain 'radix version'")
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

func TestVersionDefaults(t *testing.T) {
	// Reset to defaults
	Version = "dev"
	Commit = "unknown"
	Date = "unknown"

	info := GetInfo()

	if info.Version != "dev" {
		t.Errorf("Default version should be 'dev', got %s", info.Version)
	}

	if info.Commit != "unknown" {
		t.Errorf("Default commit should be 'unknown', got %s", info.Commit)
	}

	if info.BuildDate != "unknown" {
		t.Errorf("Default date should be 'unknown', got %s", info.BuildDate)
	}
}
