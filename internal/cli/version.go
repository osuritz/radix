package cli

import (
	"encoding/json"
	"fmt"

	"github.com/osuritz/radix/internal/version"
	"github.com/spf13/cobra"
)

var (
	shortVersion bool
	jsonOutput   bool
)

// versionCmd represents the version command
var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Display version information",
	Long: `Display version information for radix.

Shows the version number, git commit hash, build date, Go version,
and platform information.

Examples:
  radix version              # Full version information
  radix version --short      # Just the version number
  radix version --json       # JSON formatted output`,
	RunE: runVersion,
}

func init() {
	versionCmd.Flags().BoolVar(&shortVersion, "short", false, "show only version number")
	versionCmd.Flags().BoolVar(&jsonOutput, "json", false, "output as JSON")
}

func runVersion(cmd *cobra.Command, _ []string) error {
	info := version.GetInfo()

	if jsonOutput {
		// JSON output
		encoder := json.NewEncoder(cmd.OutOrStdout())
		encoder.SetIndent("", "  ")
		if err := encoder.Encode(info); err != nil {
			return fmt.Errorf("failed to encode version info as JSON: %w", err)
		}
		return nil
	}

	if shortVersion {
		// Short version output
		fmt.Fprintln(cmd.OutOrStdout(), info.Short())
		return nil
	}

	// Full version output
	fmt.Fprintln(cmd.OutOrStdout(), info.String())
	return nil
}
