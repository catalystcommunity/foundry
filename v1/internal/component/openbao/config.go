package openbao

import (
	"fmt"
	"text/template"
)

// ConfigTemplate is the OpenBAO server configuration template
const ConfigTemplate = `
ui = true

storage "file" {
  path = "/vault/data"
}

listener "tcp" {
  address     = "{{ .Address }}"
  tls_disable = 1
  # Enable unauthenticated metrics access for Prometheus scraping
  telemetry {
    unauthenticated_metrics_access = true
  }
}

api_addr = "http://{{ .Address }}"

# Telemetry configuration for Prometheus metrics
# Metrics are available at /v1/sys/metrics?format=prometheus
telemetry {
  disable_hostname = true
  prometheus_retention_time = "60s"
}
`

// GenerateConfig creates an OpenBAO configuration file from the template
func GenerateConfig(cfg *Config) (string, error) {
	if err := cfg.Validate(); err != nil {
		return "", fmt.Errorf("invalid config: %w", err)
	}

	tmpl, err := template.New("config").Parse(ConfigTemplate)
	if err != nil {
		return "", fmt.Errorf("failed to parse template: %w", err)
	}

	var buf []byte
	writer := &writeBuffer{buf: buf}
	if err := tmpl.Execute(writer, cfg); err != nil {
		return "", fmt.Errorf("failed to execute template: %w", err)
	}

	return string(writer.buf), nil
}

// writeBuffer implements io.Writer for template execution
type writeBuffer struct {
	buf []byte
}

func (w *writeBuffer) Write(p []byte) (n int, err error) {
	w.buf = append(w.buf, p...)
	return len(p), nil
}
