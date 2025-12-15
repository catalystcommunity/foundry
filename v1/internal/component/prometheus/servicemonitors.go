package prometheus

import (
	"fmt"
)

// ServiceMonitorConfig holds configuration for creating a ServiceMonitor
type ServiceMonitorConfig struct {
	Name          string            // Name of the ServiceMonitor
	Namespace     string            // Namespace for the ServiceMonitor
	Selector      map[string]string // Label selector for target services
	Port          string            // Port name to scrape (e.g., "metrics", "http")
	Path          string            // Metrics path (default: /metrics)
	Interval      string            // Scrape interval (default: 30s)
	TargetPort    int               // Target port number (if port name not used)
	ScrapeTimeout string            // Scrape timeout (optional)
}

// GetServiceMonitorManifest returns the YAML manifest for a ServiceMonitor
func GetServiceMonitorManifest(cfg ServiceMonitorConfig) string {
	if cfg.Path == "" {
		cfg.Path = "/metrics"
	}
	if cfg.Interval == "" {
		cfg.Interval = "30s"
	}

	selectorLabels := ""
	for k, v := range cfg.Selector {
		selectorLabels += fmt.Sprintf("      %s: %s\n", k, v)
	}

	endpoint := ""
	if cfg.Port != "" {
		endpoint = fmt.Sprintf("port: %s", cfg.Port)
	} else if cfg.TargetPort > 0 {
		endpoint = fmt.Sprintf("targetPort: %d", cfg.TargetPort)
	}

	scrapeTimeout := ""
	if cfg.ScrapeTimeout != "" {
		scrapeTimeout = fmt.Sprintf("\n      scrapeTimeout: %s", cfg.ScrapeTimeout)
	}

	return fmt.Sprintf(`apiVersion: monitoring.coreos.com/v1
kind: ServiceMonitor
metadata:
  name: %s
  namespace: %s
  labels:
    app.kubernetes.io/part-of: foundry
spec:
  selector:
    matchLabels:
%s  endpoints:
    - %s
      path: %s
      interval: %s%s
`, cfg.Name, cfg.Namespace, selectorLabels, endpoint, cfg.Path, cfg.Interval, scrapeTimeout)
}

// NOTE: Core component ServiceMonitors are now created by their respective Helm charts.
// The following components enable ServiceMonitors via Helm values:
// - Contour: metrics.serviceMonitor.enabled
// - SeaweedFS: global.monitoring.enabled
// - Loki: monitoring.serviceMonitor.enabled
// - Longhorn: metrics.serviceMonitor.enabled
// - Cert-Manager: prometheus.servicemonitor.enabled
// - External-DNS: serviceMonitor.enabled
// - Grafana: serviceMonitor.enabled
// - Velero: metrics.serviceMonitor.enabled
//
// The GetServiceMonitorManifest function above can still be used to create
// custom ServiceMonitors for components that don't have Helm chart support.
