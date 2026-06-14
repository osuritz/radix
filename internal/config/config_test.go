package config

import (
	"strings"
	"testing"
)

func TestValidateMetrics(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		cfg     *Config
		wantErr string // substring; "" means no error expected
	}{
		{
			name:    "valid distinct ports",
			cfg:     &Config{Port: 8080, Metrics: MetricsConfig{Enabled: true, Port: 9090, Path: "/_metrics"}},
			wantErr: "",
		},
		{
			name:    "valid custom path",
			cfg:     &Config{Port: 8080, Metrics: MetricsConfig{Enabled: true, Port: 9090, Path: "/stats"}},
			wantErr: "",
		},
		{
			name:    "disabled ignores port collision",
			cfg:     &Config{Port: 8080, Metrics: MetricsConfig{Enabled: false, Port: 8080}},
			wantErr: "",
		},
		{
			name:    "disabled ignores out-of-range port",
			cfg:     &Config{Port: 8080, Metrics: MetricsConfig{Enabled: false, Port: 0}},
			wantErr: "",
		},
		{
			name:    "disabled ignores empty path",
			cfg:     &Config{Port: 8080, Metrics: MetricsConfig{Enabled: false, Path: ""}},
			wantErr: "",
		},
		{
			name:    "port collision with app port",
			cfg:     &Config{Port: 8080, Metrics: MetricsConfig{Enabled: true, Port: 8080, Path: "/_metrics"}},
			wantErr: "must differ from the app port",
		},
		{
			name:    "port too low",
			cfg:     &Config{Port: 8080, Metrics: MetricsConfig{Enabled: true, Port: 0, Path: "/_metrics"}},
			wantErr: "between 1 and 65535",
		},
		{
			name:    "port too high",
			cfg:     &Config{Port: 8080, Metrics: MetricsConfig{Enabled: true, Port: 70000, Path: "/_metrics"}},
			wantErr: "between 1 and 65535",
		},
		{
			name:    "port negative",
			cfg:     &Config{Port: 8080, Metrics: MetricsConfig{Enabled: true, Port: -1, Path: "/_metrics"}},
			wantErr: "between 1 and 65535",
		},
		{
			name:    "empty path rejected",
			cfg:     &Config{Port: 8080, Metrics: MetricsConfig{Enabled: true, Port: 9090, Path: ""}},
			wantErr: "metrics.path must not be empty",
		},
		{
			name:    "relative path rejected",
			cfg:     &Config{Port: 8080, Metrics: MetricsConfig{Enabled: true, Port: 9090, Path: "metrics"}},
			wantErr: "must start with",
		},
		{
			name:    "path collides with healthz",
			cfg:     &Config{Port: 8080, Metrics: MetricsConfig{Enabled: true, Port: 9090, Path: HealthzPath}},
			wantErr: "reserved",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := ValidateMetrics(tt.cfg)
			if tt.wantErr == "" {
				if err != nil {
					t.Errorf("ValidateMetrics() = %v, want nil", err)
				}
				return
			}
			if err == nil {
				t.Fatalf("ValidateMetrics() = nil, want error containing %q", tt.wantErr)
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("ValidateMetrics() error = %q, want substring %q", err.Error(), tt.wantErr)
			}
		})
	}
}

func TestLoad_DefaultMetricsPort(t *testing.T) {
	t.Parallel()

	// No config file: defaults apply. Load tolerates a missing file.
	cfg, err := Load("")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Metrics.Port != 9090 {
		t.Errorf("default metrics.port = %d, want 9090", cfg.Metrics.Port)
	}
}
