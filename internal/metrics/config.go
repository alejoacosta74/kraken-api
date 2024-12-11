package metrics

import (
	"path/filepath"
)

// PrometheusConfig holds paths to Prometheus configuration files
type PrometheusConfig struct {
	ConfigPath string // Path to prometheus.yml
	AlertsPath string // Path to alerts.yml
}

// DefaultPrometheusConfig returns the default configuration paths
func DefaultPrometheusConfig() PrometheusConfig {
	return PrometheusConfig{
		ConfigPath: filepath.Join("configs", "prometheus", "prometheus.yml"),
		AlertsPath: filepath.Join("configs", "prometheus", "alerts.yml"),
	}
}
