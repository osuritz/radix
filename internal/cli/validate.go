package cli

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/osuritz/radix/internal/config"
	"github.com/osuritz/radix/internal/server"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

var (
	configType string
	strictMode bool
)

// validateCmd represents the validate command
var validateCmd = &cobra.Command{
	Use:   "validate [config-file]",
	Short: "Validate configuration files",
	Long: `Validate configuration files for syntax and correctness.

Checks YAML syntax, validates schema, verifies file paths exist,
validates port ranges, and checks TLS certificate paths. Non-fatal
issues are reported as warnings; --strict turns warnings into failures.`,
	Example: `  radix validate                    # Validate ./radix.yml
  radix validate ./custom.yml       # Validate a specific file
  radix validate ./radix.yml --strict  # Fail on warnings too
  radix validate -c ./radix.yml     # Using the --config flag
  radix validate ./mock-routes.yml  # Routes files are auto-detected
  radix validate ./routes.yml --type mock-routes  # Force mock-routes mode`,
	RunE: runValidate,
}

func init() {
	validateCmd.Flags().StringVar(&configType, "type", "auto", "config type (main, mock-routes; default: auto-detect)")
	validateCmd.Flags().BoolVar(&strictMode, "strict", false, "strict validation mode (fail on warnings)")
}

func runValidate(cmd *cobra.Command, args []string) error {
	// Validate the --type value up front so a typo fails before any file I/O.
	switch configType {
	case "auto", "main", "mock-routes":
	default:
		return fmt.Errorf("invalid --type %q (must be \"main\", \"mock-routes\", or \"auto\")", configType)
	}

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

	absPath, err := filepath.Abs(configPath)
	if err != nil {
		return fmt.Errorf("failed to resolve config path %q: %w", configPath, err)
	}
	fmt.Fprintf(cmd.OutOrStdout(), "Validating configuration: %s\n\n", absPath)

	// Read and parse the config file
	// #nosec G304 - config file path is user-provided and validated
	data, err := os.ReadFile(absPath)
	if err != nil {
		return fmt.Errorf("failed to read config file %s: %w", absPath, err)
	}

	// Parse YAML
	var rawConfig map[string]interface{}
	if unmarshalErr := yaml.Unmarshal(data, &rawConfig); unmarshalErr != nil {
		return fmt.Errorf("✗ Syntax error: %w", unmarshalErr)
	}
	fmt.Fprintln(cmd.OutOrStdout(), "✓ Syntax: OK")

	// Branch on the config type. --type forces the mode; auto-detect treats a
	// file whose top level carries the mock-routes `routes` key as a routes
	// file. Without this, validating a mock-routes file as a main config would
	// false-positively pass: viper silently ignores unknown keys.
	if resolveConfigType(rawConfig) == "mock-routes" {
		return validateMockRoutes(cmd, absPath, data)
	}

	// Load config through Viper to validate structure
	loadedCfg, err := config.Load(absPath)
	if err != nil {
		return fmt.Errorf("✗ Schema validation failed: %w", err)
	}
	fmt.Fprintln(cmd.OutOrStdout(), "✓ Schema: OK")

	// Validate file paths if TLS is enabled
	warnings := []string{}

	tlsWarnings, err := validateLoadedTLS(cmd, loadedCfg)
	if err != nil {
		return err
	}
	warnings = append(warnings, tlsWarnings...)

	// Validate port range
	if loadedCfg.Port < 1 || loadedCfg.Port > 65535 {
		return fmt.Errorf("✗ Invalid port: %d (must be 1-65535)", loadedCfg.Port)
	}
	fmt.Fprintf(cmd.OutOrStdout(), "✓ Port: %d (valid)\n", loadedCfg.Port)

	// Validate the metrics/admin port (in range and not colliding with the app
	// port). This is the same check the server commands run at startup, so a bad
	// config file is caught here instead of only failing at runtime with a
	// confusing bind error.
	if err := config.ValidateMetrics(loadedCfg); err != nil {
		return fmt.Errorf("✗ Metrics configuration: %w", err)
	}

	// Validate serve TLS-coupling rules (HSTS/redirect require TLS, redirect
	// port must differ from the main port, non-negative HSTS max-age). This is
	// the same check the serve command runs at startup, so a bad config file is
	// caught here instead of only failing at runtime.
	if err := config.ValidateServeTLS(loadedCfg); err != nil {
		return fmt.Errorf("✗ Serve configuration: %w", err)
	}

	// Validate serve directory if specified
	if loadedCfg.Serve.Dir != "" && loadedCfg.Serve.Dir != "." {
		if info, err := os.Stat(loadedCfg.Serve.Dir); err != nil {
			warnings = append(warnings, fmt.Sprintf("Serve directory not found: %s", loadedCfg.Serve.Dir))
		} else if !info.IsDir() {
			warnings = append(warnings, fmt.Sprintf("Serve path is not a directory: %s", loadedCfg.Serve.Dir))
		}
	}

	// Validate mock routes file if specified
	if loadedCfg.Mock.Routes != "" {
		if err := validatePath(loadedCfg.Mock.Routes, "routes.yml"); err != nil {
			warnings = append(warnings, fmt.Sprintf("Mock routes file not found: %s", loadedCfg.Mock.Routes))
		}
	}

	// Print warnings
	if err := reportWarnings(cmd, warnings, strictMode); err != nil {
		return err
	}

	fmt.Fprintf(cmd.OutOrStdout(), "\n✓ Configuration is valid: %s\n", absPath)
	return nil
}

// resolveConfigType decides which schema a parsed YAML document should be
// validated against. An explicit --type (main / mock-routes) wins; in auto
// mode detection is delegated to server.IsRoutesDocument, which keys on the
// top-level `routes` key only (a `settings` key alone is ambiguous — a main
// config may carry a stray one). A settings-only routes file can still be
// validated explicitly with --type mock-routes.
func resolveConfigType(rawConfig map[string]interface{}) string {
	if configType != "auto" {
		return configType
	}
	if server.IsRoutesDocument(rawConfig) {
		return "mock-routes"
	}
	return "main"
}

// validateMockRoutes validates data as a mock-routes file by compiling it with
// the same loader the mock command uses (server.CompileRoutes), so validation
// and runtime agree exactly: route paths, methods, regex patterns, templates,
// conditions, selectors, and settings are all checked. File-backed response
// bodies are resolved relative to the routes file's directory, matching
// server.LoadRoutes.
func validateMockRoutes(cmd *cobra.Command, absPath string, data []byte) error {
	compiled, err := server.CompileRoutes(data, filepath.Dir(absPath))
	if err != nil {
		return fmt.Errorf("✗ Routes: %w", err)
	}
	fmt.Fprintf(cmd.OutOrStdout(), "✓ Routes: %d compiled\n", compiled.RouteCount())

	// Apply the same settings range checks runMock enforces at startup (with no
	// CLI flags set, the file's settings ARE the effective settings), so a file
	// that validates here cannot be refused by `radix mock`.
	if err := validateEffectiveSettings(compiled.Settings()); err != nil {
		return fmt.Errorf("✗ Settings: %w", err)
	}

	// The compiler is strict (any real problem is an error above); the only
	// advisory case is a file that compiles but defines no routes.
	warnings := []string{}
	if compiled.RouteCount() == 0 {
		warnings = append(warnings, "No routes defined (the file compiles but matches no requests)")
	}

	if err := reportWarnings(cmd, warnings, strictMode); err != nil {
		return err
	}

	fmt.Fprintf(cmd.OutOrStdout(), "\n✓ Mock routes file is valid: %s\n", absPath)
	return nil
}

// reportWarnings prints the shared warnings block used by both the main-config
// and mock-routes validation paths and, when strict is set, converts the
// warnings into a validation error. It is a no-op returning nil when there are
// no warnings.
func reportWarnings(cmd *cobra.Command, warnings []string, strict bool) error {
	if len(warnings) == 0 {
		return nil
	}
	fmt.Fprintln(cmd.OutOrStdout(), "\nWarnings:")
	for _, warning := range warnings {
		fmt.Fprintf(cmd.OutOrStdout(), "  ⚠ %s\n", warning)
	}
	if strict {
		return fmt.Errorf("validation failed: --strict treats the %d warning(s) above as errors", len(warnings))
	}
	return nil
}

// validateLoadedTLS checks the TLS-coupled file paths and version for a loaded
// config when TLS is enabled. It returns any non-fatal warnings (CA-file
// missing, TLS 1.2 advisory) and a fatal error for misconfiguration. Extracted
// from runValidate to keep that function's cyclomatic complexity in check.
func validateLoadedTLS(cmd *cobra.Command, loadedCfg *config.Config) ([]string, error) {
	if !loadedCfg.TLS.Enabled {
		return nil, nil
	}

	var warnings []string

	if loadedCfg.TLS.Cert == "" {
		return nil, fmt.Errorf("✗ TLS enabled but cert file not specified")
	}
	if loadedCfg.TLS.Key == "" {
		return nil, fmt.Errorf("✗ TLS enabled but key file not specified")
	}

	// Check cert file
	if err := validatePath(loadedCfg.TLS.Cert, "cert.pem"); err != nil {
		return nil, fmt.Errorf("✗ Certificate file: %w", err)
	}

	// Check key file
	if err := validatePath(loadedCfg.TLS.Key, "key.pem"); err != nil {
		return nil, fmt.Errorf("✗ Key file: %w", err)
	}

	// Check CA file if specified
	if loadedCfg.TLS.CA != "" {
		if err := validatePath(loadedCfg.TLS.CA, "ca.pem"); err != nil {
			warnings = append(warnings, fmt.Sprintf("CA file not found: %s", loadedCfg.TLS.CA))
		}
	}

	fmt.Fprintln(cmd.OutOrStdout(), "✓ TLS certificates: OK")

	// Check TLS version
	if loadedCfg.TLS.MinVersion != "1.2" && loadedCfg.TLS.MinVersion != "1.3" {
		return nil, fmt.Errorf("✗ Invalid TLS min_version: %s (must be '1.2' or '1.3')", loadedCfg.TLS.MinVersion)
	}

	// Warn if using TLS 1.2
	if loadedCfg.TLS.MinVersion == "1.2" {
		warnings = append(warnings, "Consider setting tls.min_version to '1.3' for better security")
	}

	return warnings, nil
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
	// #nosec G304 - path is user-provided for validation purposes
	file, err := os.Open(absPath)
	if err != nil {
		return fmt.Errorf("%s is not readable: %w", description, err)
	}
	defer func() { _ = file.Close() }()

	return nil
}
