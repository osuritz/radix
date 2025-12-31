package version

import (
	"fmt"
	"runtime"
)

var (
	// Version is the current version of radix
	// Set during build: -ldflags "-X github.com/osuritz/radix/internal/version.Version=x.y.z"
	Version = "dev"

	// Commit is the git commit hash
	// Set during build: -ldflags "-X github.com/osuritz/radix/internal/version.Commit=abc123"
	Commit = "unknown"

	// Date is the build date
	// Set during build: -ldflags "-X github.com/osuritz/radix/internal/version.Date=2025-12-31"
	Date = "unknown"

	// GoVersion is the Go version used to build
	GoVersion = runtime.Version()

	// Platform is the OS/Arch combination
	Platform = fmt.Sprintf("%s/%s", runtime.GOOS, runtime.GOARCH)
)

// Info represents version information
type Info struct {
	Version   string `json:"version"`
	Commit    string `json:"commit"`
	BuildDate string `json:"build_date"`
	GoVersion string `json:"go_version"`
	Platform  string `json:"platform"`
}

// GetInfo returns the version information
func GetInfo() Info {
	return Info{
		Version:   Version,
		Commit:    Commit,
		BuildDate: Date,
		GoVersion: GoVersion,
		Platform:  Platform,
	}
}

// String returns a formatted version string
func (i Info) String() string {
	return fmt.Sprintf("radix version %s (commit: %s, built: %s, go: %s, platform: %s)",
		i.Version, i.Commit, i.BuildDate, i.GoVersion, i.Platform)
}

// Short returns just the version number
func (i Info) Short() string {
	return i.Version
}
