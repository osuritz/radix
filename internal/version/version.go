// Package version provides build information for radix.
package version

import (
	"fmt"
	"runtime"
)

// Build information. Populated at build-time via ldflags.
var (
	Version = "dev"
	Commit  = "none"
	Date    = "unknown"
	BuiltBy = "unknown"
)

// Info contains version and build information.
type Info struct {
	Version  string `json:"version"`
	Commit   string `json:"commit"`
	Date     string `json:"date"`
	BuiltBy  string `json:"built_by"`
	GoOS     string `json:"goos"`
	GoArch   string `json:"goarch"`
	Compiler string `json:"compiler"`
}

// GetInfo returns the version information.
func GetInfo() *Info {
	return &Info{
		Version:  Version,
		Commit:   Commit,
		Date:     Date,
		BuiltBy:  BuiltBy,
		GoOS:     runtime.GOOS,
		GoArch:   runtime.GOARCH,
		Compiler: runtime.Version(),
	}
}

// String returns formatted version information.
func (i *Info) String() string {
	return fmt.Sprintf(
		"radix %s\ncommit: %s\nbuilt at: %s\nbuilt by: %s\ngoos: %s\ngoarch: %s\ncompiler: %s",
		i.Version,
		i.Commit,
		i.Date,
		i.BuiltBy,
		i.GoOS,
		i.GoArch,
		i.Compiler,
	)
}

// Short returns a short version string.
func (i *Info) Short() string {
	return i.Version
}

// UserAgent returns a user agent string for HTTP requests.
func UserAgent() string {
	return fmt.Sprintf("radix/%s", Version)
}
