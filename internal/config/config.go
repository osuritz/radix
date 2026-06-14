// Package config provides configuration management for radix.
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

	// Metrics configuration (global)
	Metrics MetricsConfig `mapstructure:"metrics"`

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

// MetricsConfig represents metrics/observability configuration
type MetricsConfig struct {
	Enabled bool   `mapstructure:"enabled"`
	Path    string `mapstructure:"path"`
	Format  string `mapstructure:"format"` // "json" or "prometheus"
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
	HSTSMaxAge   int    `mapstructure:"hsts_max_age"`
}

// ProxyConfig represents configuration for the proxy command
type ProxyConfig struct {
	Target        string        `mapstructure:"target"`
	Timeout       time.Duration `mapstructure:"timeout"`
	WebSocket     bool          `mapstructure:"websocket"`
	TLSSkipVerify bool          `mapstructure:"tls_skip_verify"`
	BackendCA     string        `mapstructure:"backend_ca"`
	BackendCert   string        `mapstructure:"backend_cert"`
	BackendKey    string        `mapstructure:"backend_key"`
	Rewrite       string        `mapstructure:"rewrite"`
	StripPrefix   string        `mapstructure:"strip_prefix"`
	Headers       []string      `mapstructure:"headers"`
	CORS          bool          `mapstructure:"cors"`
	Auth          AuthConfig    `mapstructure:"auth"`

	// FlushInterval controls response flushing for streaming backends.
	// -1 flushes immediately after each write (default; best for SSE / agent
	// chat), 0 uses Go's default buffering, and a positive value flushes
	// periodically at that interval.
	FlushInterval time.Duration `mapstructure:"flush_interval"`
}

// EchoConfig represents configuration for the echo command
type EchoConfig struct {
	Delay          time.Duration `mapstructure:"delay"`
	DelayJitter    time.Duration `mapstructure:"delay_jitter"`
	Status         int           `mapstructure:"status"`
	Body           string        `mapstructure:"body"`
	ContentType    string        `mapstructure:"content_type"`
	Headers        []string      `mapstructure:"headers"`
	EchoBody       bool          `mapstructure:"echo_body"`
	EchoHeaders    bool          `mapstructure:"echo_headers"`
	EchoQuery      bool          `mapstructure:"echo_query"`
	BodyLimit      int           `mapstructure:"body_limit"`
	Pretty         bool          `mapstructure:"pretty"`
	StatusFromPath bool          `mapstructure:"status_from_path"`
	DelayFromPath  bool          `mapstructure:"delay_from_path"`
	CORS           bool          `mapstructure:"cors"`
}

// AuthConfig represents authentication provider configuration for the proxy.
type AuthConfig struct {
	Provider string         `mapstructure:"provider"`
	Config   map[string]any `mapstructure:"config"`
}

// MockConfig represents configuration for the mock command
type MockConfig struct {
	// Routes names a YAML routes file defining custom routes (also accepted as a
	// positional arg); Watch enables hot-reload of that file on change. They back
	// the mock command's --routes/-r and --watch/-w flags.
	Routes string `mapstructure:"routes"`
	Watch  bool   `mapstructure:"watch"`

	// Latency and LatencyJitter add artificial latency to every built-in
	// response (Go duration strings, e.g. "200ms").
	Latency       string `mapstructure:"latency"`
	LatencyJitter string `mapstructure:"latency_jitter"`

	// FailRate is the random failure rate as a percentage in [0, 100], and
	// FailStatus is the status code returned for those failures.
	FailRate   float64 `mapstructure:"fail_rate"`
	FailStatus int     `mapstructure:"fail_status"`

	// CORS enables permissive CORS headers. Builtin toggles the built-in
	// httpbin-style endpoints, and Prefix mounts them under a path prefix.
	CORS    bool   `mapstructure:"cors"`
	Builtin bool   `mapstructure:"builtin"`
	Prefix  string `mapstructure:"prefix"`
}

// ValidateServeTLS checks serve options that are coupled to TLS and the
// HTTP→HTTPS redirect listener. It is shared between the serve command's
// runtime checks and the offline `radix validate` path so that a misconfigured
// file is rejected before it ever reaches the server.
//
// Rules enforced:
//   - serve.hsts requires tls.enabled
//   - serve.http_redirect requires tls.enabled
//   - serve.http_port must differ from port when http_redirect is set
//   - serve.hsts_max_age must not be negative (0 is valid: it clears the policy)
func ValidateServeTLS(cfg *Config) error {
	// A negative HSTS max-age is never valid (max-age=0 clears the policy).
	if cfg.Serve.HSTSMaxAge < 0 {
		return fmt.Errorf("--hsts-max-age (%d) must not be negative", cfg.Serve.HSTSMaxAge)
	}

	// HSTS and HTTP→HTTPS redirect are only meaningful with TLS enabled.
	if cfg.Serve.HSTS && !cfg.TLS.Enabled {
		return fmt.Errorf("--hsts requires --tls")
	}
	if cfg.Serve.HTTPRedirect {
		if !cfg.TLS.Enabled {
			return fmt.Errorf("--http-redirect requires --tls")
		}
		if cfg.Serve.HTTPPort == cfg.Port {
			return fmt.Errorf("--http-port (%d) must differ from --port (%d)", cfg.Serve.HTTPPort, cfg.Port)
		}
	}
	return nil
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

	// Metrics defaults
	v.SetDefault("metrics.enabled", true)
	v.SetDefault("metrics.path", "/_metrics")
	v.SetDefault("metrics.format", "json")

	// Serve defaults
	v.SetDefault("serve.dir", ".")
	v.SetDefault("serve.index", "index.html")
	v.SetDefault("serve.spa", false)
	v.SetDefault("serve.cors", false)
	v.SetDefault("serve.gzip", false)
	v.SetDefault("serve.http_redirect", false)
	v.SetDefault("serve.http_port", 8080)
	v.SetDefault("serve.hsts", false)
	v.SetDefault("serve.hsts_max_age", 31536000)

	// Proxy defaults
	v.SetDefault("proxy.timeout", "30s")
	v.SetDefault("proxy.websocket", false)
	v.SetDefault("proxy.tls_skip_verify", false)
	// Immediate flush by default: best for a dev proxy serving agent/chat SSE.
	v.SetDefault("proxy.flush_interval", -1*time.Nanosecond)

	// Echo defaults
	v.SetDefault("echo.delay", "0")
	v.SetDefault("echo.delay_jitter", "0")
	v.SetDefault("echo.status", 200)
	v.SetDefault("echo.content_type", "application/json")
	v.SetDefault("echo.echo_body", true)
	v.SetDefault("echo.echo_headers", true)
	v.SetDefault("echo.echo_query", true)
	v.SetDefault("echo.body_limit", 1048576)
	v.SetDefault("echo.pretty", true)
	v.SetDefault("echo.status_from_path", false)
	v.SetDefault("echo.delay_from_path", false)
	v.SetDefault("echo.cors", false)

	// Mock defaults
	v.SetDefault("mock.watch", false)
	v.SetDefault("mock.latency", "0")
	v.SetDefault("mock.latency_jitter", "0")
	v.SetDefault("mock.fail_rate", 0.0)
	v.SetDefault("mock.fail_status", 500)
	v.SetDefault("mock.cors", false)
	v.SetDefault("mock.builtin", true)
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
	// #nosec G304 - config file path is user-provided and validated
	file, err := os.Open(absPath)
	if err != nil {
		return fmt.Errorf("config file is not readable: %w", err)
	}
	defer func() { _ = file.Close() }()

	return nil
}
