package config

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/spf13/viper"
)

// Config represents the main configuration structure
type Config struct {
	Port    int    `mapstructure:"port"`
	Host    string `mapstructure:"host"`
	Verbose bool   `mapstructure:"verbose"`
	NoColor bool   `mapstructure:"no_color"`

	// TLS configuration (global)
	TLS TLSConfig `mapstructure:"tls"`

	// Command-specific configs
	Serve ServeConfig `mapstructure:"serve"`
	Proxy ProxyConfig `mapstructure:"proxy"`
	Echo  EchoConfig  `mapstructure:"echo"`
	Mock  MockConfig  `mapstructure:"mock"`
}

// TLSConfig represents TLS/HTTPS configuration
type TLSConfig struct {
	Enabled    bool   `mapstructure:"enabled"`
	Cert       string `mapstructure:"cert"`
	Key        string `mapstructure:"key"`
	CA         string `mapstructure:"ca"`
	ClientAuth bool   `mapstructure:"client_auth"`
	MinVersion string `mapstructure:"min_version"` // "1.2" or "1.3"
}

// ServeConfig represents configuration for the serve command
type ServeConfig struct {
	Dir          string `mapstructure:"dir"`
	Index        string `mapstructure:"index"`
	SPA          bool   `mapstructure:"spa"`
	CORS         bool   `mapstructure:"cors"`
	Gzip         bool   `mapstructure:"gzip"`
	Cache        string `mapstructure:"cache"`
	HTTPRedirect bool   `mapstructure:"http_redirect"`
	HTTPPort     int    `mapstructure:"http_port"`
	HSTS         bool   `mapstructure:"hsts"`
}

// ProxyConfig represents configuration for the proxy command
type ProxyConfig struct {
	Target         string        `mapstructure:"target"`
	Timeout        time.Duration `mapstructure:"timeout"`
	WebSocket      bool          `mapstructure:"websocket"`
	TLSSkipVerify  bool          `mapstructure:"tls_skip_verify"`
	BackendCA      string        `mapstructure:"backend_ca"`
	BackendCert    string        `mapstructure:"backend_cert"`
	BackendKey     string        `mapstructure:"backend_key"`
	Rewrite        string        `mapstructure:"rewrite"`
	StripPrefix    string        `mapstructure:"strip_prefix"`
	Headers        []string      `mapstructure:"headers"`
}

// EchoConfig represents configuration for the echo command
type EchoConfig struct {
	Delay   time.Duration `mapstructure:"delay"`
	Status  int           `mapstructure:"status"`
	Body    string        `mapstructure:"body"`
	Headers []string      `mapstructure:"headers"`
}

// MockConfig represents configuration for the mock command
type MockConfig struct {
	Routes   string  `mapstructure:"routes"`
	Watch    bool    `mapstructure:"watch"`
	Latency  string  `mapstructure:"latency"`
	FailRate float64 `mapstructure:"fail_rate"`
}

// Load loads configuration from file, environment variables, and defaults
func Load(cfgFile string) (*Config, error) {
	v := viper.New()

	// Set defaults
	setDefaults(v)

	// Set config file if specified
	if cfgFile != "" {
		v.SetConfigFile(cfgFile)
	} else {
		// Search for config in current directory, home directory, and /etc/radix
		v.SetConfigName("radix")
		v.SetConfigType("yaml")
		v.AddConfigPath(".")

		home, err := os.UserHomeDir()
		if err == nil {
			v.AddConfigPath(home)
		}

		v.AddConfigPath("/etc/radix")
	}

	// Read config file (optional)
	if err := v.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); !ok {
			return nil, fmt.Errorf("failed to read config file: %w", err)
		}
		// Config file not found is OK, we'll use defaults
	}

	// Environment variables (with RADIX_ prefix)
	v.SetEnvPrefix("RADIX")
	v.AutomaticEnv()

	// Unmarshal config
	var cfg Config
	if err := v.Unmarshal(&cfg); err != nil {
		return nil, fmt.Errorf("failed to unmarshal config: %w", err)
	}

	return &cfg, nil
}

// setDefaults sets default configuration values
func setDefaults(v *viper.Viper) {
	// Global defaults
	v.SetDefault("port", 8080)
	v.SetDefault("host", "localhost")
	v.SetDefault("verbose", false)
	v.SetDefault("no_color", false)

	// TLS defaults
	v.SetDefault("tls.enabled", false)
	v.SetDefault("tls.client_auth", false)
	v.SetDefault("tls.min_version", "1.2")

	// Serve defaults
	v.SetDefault("serve.dir", ".")
	v.SetDefault("serve.index", "index.html")
	v.SetDefault("serve.spa", false)
	v.SetDefault("serve.cors", false)
	v.SetDefault("serve.gzip", false)
	v.SetDefault("serve.http_redirect", false)
	v.SetDefault("serve.http_port", 8080)
	v.SetDefault("serve.hsts", false)

	// Proxy defaults
	v.SetDefault("proxy.timeout", "30s")
	v.SetDefault("proxy.websocket", false)
	v.SetDefault("proxy.tls_skip_verify", false)

	// Echo defaults
	v.SetDefault("echo.delay", "0")
	v.SetDefault("echo.status", 200)

	// Mock defaults
	v.SetDefault("mock.watch", false)
	v.SetDefault("mock.fail_rate", 0.0)
}

// ValidateFile validates a configuration file exists and is readable
func ValidateFile(path string) error {
	if path == "" {
		return fmt.Errorf("config file path is empty")
	}

	absPath, err := filepath.Abs(path)
	if err != nil {
		return fmt.Errorf("invalid config file path: %w", err)
	}

	info, err := os.Stat(absPath)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("config file does not exist: %s", absPath)
		}
		return fmt.Errorf("cannot access config file: %w", err)
	}

	if info.IsDir() {
		return fmt.Errorf("config file path is a directory: %s", absPath)
	}

	// Check if file is readable
	file, err := os.Open(absPath)
	if err != nil {
		return fmt.Errorf("config file is not readable: %w", err)
	}
	file.Close()

	return nil
}
