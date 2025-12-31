package cli

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/osuritz/radix/internal/config"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

var (
	configType   string
	strictMode   bool
	validateFile string
)

// validateCmd represents the validate command
var validateCmd = &cobra.Command{
	Use:   "validate [config-file]",
	Short: "Validate configuration files",
	Long: `Validate configuration files for syntax and correctness.

Checks YAML/JSON syntax, validates schema, verifies file paths exist,
validates port ranges, and checks TLS certificate paths.

Examples:
  radix validate                    # Validate ./radix.yml
  radix validate ./custom.yml       # Validate specific file
  radix validate --strict           # Fail on warnings
  radix validate -c ./radix.yml     # Using --config flag`,
	RunE: runValidate,
}

func init() {
	validateCmd.Flags().StringVar(&configType, "type", "auto", "config type (main, mock-routes; default: auto-detect)")
	validateCmd.Flags().BoolVar(&strictMode, "strict", false, "strict validation mode (fail on warnings)")
}

func runValidate(cmd *cobra.Command, args []string) error {
	// Determine config file to validate
	configPath := cfgFile
	if len(args) > 0 {
		configPath = args[0]
	}
	if configPath == "" {
		configPath = "./radix.yml"
	}

	// Validate file exists and is readable
	if err := config.ValidateFile(configPath); err != nil {
		return err
	}

	absPath, _ := filepath.Abs(configPath)
	fmt.Fprintf(cmd.OutOrStdout(), "Validating configuration: %s\n\n", absPath)

	// Read and parse the config file
	data, err := os.ReadFile(absPath)
	if err != nil {
		return fmt.Errorf("failed to read config file: %w", err)
	}

	// Parse YAML
	var rawConfig map[string]interface{}
	if err := yaml.Unmarshal(data, &rawConfig); err != nil {
		return fmt.Errorf("✗ Syntax error: %w", err)
	}
	fmt.Fprintln(cmd.OutOrStdout(), "✓ Syntax: OK")

	// Load config through Viper to validate structure
	cfg, err := config.Load(absPath)
	if err != nil {
		return fmt.Errorf("✗ Schema validation failed: %w", err)
	}
	fmt.Fprintln(cmd.OutOrStdout(), "✓ Schema: OK")

	// Validate file paths if TLS is enabled
	warnings := []string{}

	if cfg.TLS.Enabled {
		if cfg.TLS.Cert == "" {
			return fmt.Errorf("✗ TLS enabled but cert file not specified")
		}
		if cfg.TLS.Key == "" {
			return fmt.Errorf("✗ TLS enabled but key file not specified")
		}

		// Check cert file
		if err := validatePath(cfg.TLS.Cert, "cert.pem"); err != nil {
			return fmt.Errorf("✗ Certificate file: %w", err)
		}

		// Check key file
		if err := validatePath(cfg.TLS.Key, "key.pem"); err != nil {
			return fmt.Errorf("✗ Key file: %w", err)
		}

		// Check CA file if specified
		if cfg.TLS.CA != "" {
			if err := validatePath(cfg.TLS.CA, "ca.pem"); err != nil {
				warnings = append(warnings, fmt.Sprintf("CA file not found: %s", cfg.TLS.CA))
			}
		}

		fmt.Fprintln(cmd.OutOrStdout(), "✓ TLS certificates: OK")

		// Check TLS version
		if cfg.TLS.MinVersion != "1.2" && cfg.TLS.MinVersion != "1.3" {
			return fmt.Errorf("✗ Invalid TLS min_version: %s (must be '1.2' or '1.3')", cfg.TLS.MinVersion)
		}

		// Warn if using TLS 1.2
		if cfg.TLS.MinVersion == "1.2" {
			warnings = append(warnings, "Consider setting tls.min_version to '1.3' for better security")
		}
	}

	// Validate port range
	if cfg.Port < 1 || cfg.Port > 65535 {
		return fmt.Errorf("✗ Invalid port: %d (must be 1-65535)", cfg.Port)
	}
	fmt.Fprintf(cmd.OutOrStdout(), "✓ Port: %d (valid)\n", cfg.Port)

	// Validate serve directory if specified
	if cfg.Serve.Dir != "" && cfg.Serve.Dir != "." {
		if info, err := os.Stat(cfg.Serve.Dir); err != nil {
			warnings = append(warnings, fmt.Sprintf("Serve directory not found: %s", cfg.Serve.Dir))
		} else if !info.IsDir() {
			warnings = append(warnings, fmt.Sprintf("Serve path is not a directory: %s", cfg.Serve.Dir))
		}
	}

	// Validate mock routes file if specified
	if cfg.Mock.Routes != "" {
		if err := validatePath(cfg.Mock.Routes, "routes.yml"); err != nil {
			warnings = append(warnings, fmt.Sprintf("Mock routes file not found: %s", cfg.Mock.Routes))
		}
	}

	// Print warnings
	if len(warnings) > 0 {
		fmt.Fprintln(cmd.OutOrStdout(), "\nWarnings:")
		for _, warning := range warnings {
			fmt.Fprintf(cmd.OutOrStdout(), "  ⚠ %s\n", warning)
		}

		if strictMode {
			return fmt.Errorf("validation failed in strict mode due to warnings")
		}
	}

	fmt.Fprintf(cmd.OutOrStdout(), "\n✓ Configuration is valid: %s\n", absPath)
	return nil
}

// validatePath checks if a file path exists and is readable
func validatePath(path, description string) error {
	if path == "" {
		return fmt.Errorf("%s path is empty", description)
	}

	absPath, err := filepath.Abs(path)
	if err != nil {
		return fmt.Errorf("invalid %s path: %w", description, err)
	}

	info, err := os.Stat(absPath)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("%s not found: %s", description, absPath)
		}
		return fmt.Errorf("cannot access %s: %w", description, err)
	}

	if info.IsDir() {
		return fmt.Errorf("%s is a directory: %s", description, absPath)
	}

	// Check if file is readable
	file, err := os.Open(absPath)
	if err != nil {
		return fmt.Errorf("%s is not readable: %w", description, err)
	}
	file.Close()

	return nil
}
