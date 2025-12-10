package prometheus

import (
	"context"
	"fmt"
)

// K8sApplyClient defines the interface for applying Kubernetes manifests
type K8sApplyClient interface {
	ApplyManifest(ctx context.Context, manifest string) error
}

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

// CoreServiceMonitors returns ServiceMonitor configurations for Foundry core components
func CoreServiceMonitors() []ServiceMonitorConfig {
	return []ServiceMonitorConfig{
		// Contour - Envoy proxy metrics
		{
			Name:      "contour-envoy",
			Namespace: "monitoring",
			Selector: map[string]string{
				"app.kubernetes.io/component": "envoy",
				"app.kubernetes.io/name":      "contour",
			},
			TargetPort: 8002,
			Path:       "/stats/prometheus",
			Interval:   "30s",
		},
		// Contour - Contour controller metrics
		{
			Name:      "contour-controller",
			Namespace: "monitoring",
			Selector: map[string]string{
				"app.kubernetes.io/component": "contour",
				"app.kubernetes.io/name":      "contour",
			},
			Port:     "metrics",
			Path:     "/metrics",
			Interval: "30s",
		},
		// SeaweedFS - Master metrics
		{
			Name:      "seaweedfs-master",
			Namespace: "monitoring",
			Selector: map[string]string{
				"app.kubernetes.io/component": "master",
				"app.kubernetes.io/name":      "seaweedfs",
			},
			TargetPort: 9333,
			Path:       "/metrics",
			Interval:   "30s",
		},
		// SeaweedFS - Volume metrics
		{
			Name:      "seaweedfs-volume",
			Namespace: "monitoring",
			Selector: map[string]string{
				"app.kubernetes.io/component": "volume",
				"app.kubernetes.io/name":      "seaweedfs",
			},
			TargetPort: 8080,
			Path:       "/metrics",
			Interval:   "30s",
		},
		// SeaweedFS - Filer metrics
		{
			Name:      "seaweedfs-filer",
			Namespace: "monitoring",
			Selector: map[string]string{
				"app.kubernetes.io/component": "filer",
				"app.kubernetes.io/name":      "seaweedfs",
			},
			TargetPort: 8888,
			Path:       "/metrics",
			Interval:   "30s",
		},
		// Loki metrics
		{
			Name:      "loki",
			Namespace: "monitoring",
			Selector: map[string]string{
				"app.kubernetes.io/name": "loki",
			},
			Port:     "http-metrics",
			Path:     "/metrics",
			Interval: "30s",
		},
		// Longhorn - Manager metrics
		{
			Name:      "longhorn-manager",
			Namespace: "monitoring",
			Selector: map[string]string{
				"app": "longhorn-manager",
			},
			Port:     "manager",
			Path:     "/metrics",
			Interval: "30s",
		},
		// Cert-Manager metrics
		{
			Name:      "cert-manager",
			Namespace: "monitoring",
			Selector: map[string]string{
				"app.kubernetes.io/name":      "cert-manager",
				"app.kubernetes.io/component": "controller",
			},
			Port:     "http-metrics",
			Path:     "/metrics",
			Interval: "60s",
		},
		// External-DNS metrics
		{
			Name:      "external-dns",
			Namespace: "monitoring",
			Selector: map[string]string{
				"app.kubernetes.io/name": "external-dns",
			},
			Port:     "http",
			Path:     "/metrics",
			Interval: "60s",
		},
	}
}

// InstallCoreServiceMonitors creates ServiceMonitors for Foundry core components
func InstallCoreServiceMonitors(ctx context.Context, k8sClient K8sApplyClient) error {
	if k8sClient == nil {
		return fmt.Errorf("k8s client cannot be nil")
	}

	monitors := CoreServiceMonitors()
	for _, cfg := range monitors {
		manifest := GetServiceMonitorManifest(cfg)
		if err := k8sClient.ApplyManifest(ctx, manifest); err != nil {
			// Log warning but continue - some services may not exist yet
			fmt.Printf("  Warning: failed to create ServiceMonitor %s: %v\n", cfg.Name, err)
			continue
		}
		fmt.Printf("  Created ServiceMonitor: %s\n", cfg.Name)
	}

	return nil
}

// GetServiceMonitorNames returns the names of all core ServiceMonitors
func GetServiceMonitorNames() []string {
	monitors := CoreServiceMonitors()
	names := make([]string, len(monitors))
	for i, m := range monitors {
		names[i] = m.Name
	}
	return names
}
