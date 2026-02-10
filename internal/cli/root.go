// Package cli implements the command-line interface for radix.
package cli

import (
	"fmt"

	"github.com/osuritz/radix/internal/config"
	"github.com/spf13/cobra"
)

var (
	cfgFile string
	cfg     *config.Config

	// Global flags
	port    int
	host    string
	verbose bool
	noColor bool

	// Metrics flags
	metricsEnabled bool
	metricsPath    string
	metricsFormat  string
)

// rootCmd represents the base command when called without any subcommands
var rootCmd = &cobra.Command{
	Use:   "radix",
	Short: "Multi-mode HTTP server for local development",
	Long: `radix is a multi-mode HTTP server for local development.
It provides static file serving, reverse proxy, request echo, and API mocking
capabilities—all running locally with no external services or data leakage.

Examples:
  radix serve                    # Serve current directory
  radix serve --dir ./dist --spa # Serve SPA with routing
  radix proxy --target http://localhost:3000
  radix echo --delay 2s          # Echo server with delay
  radix mock --routes ./api.yml  # Mock API server
  radix gencert --host localhost # Generate TLS certificates
  radix version                  # Show version information
  radix validate ./radix.yml     # Validate configuration`,
	SilenceUsage:  true,
	SilenceErrors: true,
	PersistentPreRunE: func(cmd *cobra.Command, _ []string) error {
		// Load configuration
		var err error
		cfg, err = config.Load(cfgFile)
		if err != nil {
			return fmt.Errorf("failed to load config: %w", err)
		}

		// Override config with flags if they were explicitly set
		if cmd.Flags().Changed("port") {
			cfg.Port = port
		}
		if cmd.Flags().Changed("host") {
			cfg.Host = host
		}
		if cmd.Flags().Changed("verbose") {
			cfg.Verbose = verbose
		}
		if cmd.Flags().Changed("no-color") {
			cfg.NoColor = noColor
		}
		if cmd.Flags().Changed("metrics") {
			cfg.Metrics.Enabled = metricsEnabled
		}
		if cmd.Flags().Changed("metrics-path") {
			cfg.Metrics.Path = metricsPath
		}
		if cmd.Flags().Changed("metrics-format") {
			cfg.Metrics.Format = metricsFormat
		}

		return nil
	},
}

// Execute adds all child commands to the root command and sets flags appropriately.
func Execute() error {
	return rootCmd.Execute()
}

func init() {
	// Global persistent flags (available to all subcommands)
	rootCmd.PersistentFlags().StringVarP(&cfgFile, "config", "c", "", "config file (default is ./radix.yml, ~/.radix.yml, or /etc/radix/radix.yml)")
	rootCmd.PersistentFlags().IntVarP(&port, "port", "p", 8080, "port to listen on")
	rootCmd.PersistentFlags().StringVar(&host, "host", "localhost", "host to bind to")
	rootCmd.PersistentFlags().BoolVarP(&verbose, "verbose", "v", false, "verbose logging")
	rootCmd.PersistentFlags().BoolVar(&noColor, "no-color", false, "disable colored output")

	// Metrics flags
	rootCmd.PersistentFlags().BoolVar(&metricsEnabled, "metrics", true, "enable metrics endpoint")
	rootCmd.PersistentFlags().StringVar(&metricsPath, "metrics-path", "/_metrics", "metrics endpoint path")
	rootCmd.PersistentFlags().StringVar(&metricsFormat, "metrics-format", "json", "metrics output format (json, prometheus)")

	// Add subcommands
	rootCmd.AddCommand(versionCmd)
	rootCmd.AddCommand(validateCmd)
	rootCmd.AddCommand(gencertCmd)
}

// GetConfig returns the loaded configuration
func GetConfig() *config.Config {
	return cfg
}
