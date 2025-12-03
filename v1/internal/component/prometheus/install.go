package prometheus

import (
	"context"
	"fmt"
	"time"

	"github.com/catalystcommunity/foundry/v1/internal/helm"
)

const (
	prometheusRepoName = "prometheus-community"
	prometheusRepoURL  = "https://prometheus-community.github.io/helm-charts"
	prometheusChart    = "prometheus-community/kube-prometheus-stack"
	releaseName        = "kube-prometheus-stack"
)

// Install installs the kube-prometheus-stack using Helm
func Install(ctx context.Context, helmClient HelmClient, k8sClient K8sClient, cfg *Config) error {
	if helmClient == nil {
		return fmt.Errorf("helm client cannot be nil")
	}
	if cfg == nil {
		cfg = DefaultConfig()
	}

	fmt.Println("  Installing Prometheus stack...")

	// Add Helm repository
	if err := helmClient.AddRepo(ctx, helm.RepoAddOptions{
		Name:        prometheusRepoName,
		URL:         prometheusRepoURL,
		ForceUpdate: true,
	}); err != nil {
		return fmt.Errorf("failed to add helm repository: %w", err)
	}

	// Build Helm values
	values := buildHelmValues(cfg)

	// Check if release already exists
	releases, err := helmClient.List(ctx, cfg.Namespace)
	if err == nil {
		for _, rel := range releases {
			if rel.Name == releaseName {
				if rel.Status == "deployed" {
					fmt.Println("  Prometheus stack already installed")
					return verifyInstallation(ctx, k8sClient, cfg.Namespace)
				}
				// Uninstall failed release
				fmt.Printf("  Removing failed release (status: %s)...\n", rel.Status)
				if err := helmClient.Uninstall(ctx, helm.UninstallOptions{
					ReleaseName: releaseName,
					Namespace:   cfg.Namespace,
				}); err != nil {
					return fmt.Errorf("failed to uninstall existing release: %w", err)
				}
				break
			}
		}
	}

	// Install kube-prometheus-stack via Helm
	if err := helmClient.Install(ctx, helm.InstallOptions{
		ReleaseName:     releaseName,
		Namespace:       cfg.Namespace,
		Chart:           prometheusChart,
		Version:         cfg.Version,
		Values:          values,
		CreateNamespace: true,
		Wait:            true,
		Timeout:         15 * time.Minute, // Prometheus stack can take a while
	}); err != nil {
		return fmt.Errorf("failed to install prometheus stack: %w", err)
	}

	// Verify installation
	if k8sClient != nil {
		if err := verifyInstallation(ctx, k8sClient, cfg.Namespace); err != nil {
			return fmt.Errorf("installation verification failed: %w", err)
		}
	}

	fmt.Println("  Prometheus stack installed successfully")
	fmt.Printf("  Prometheus endpoint: %s\n", cfg.GetPrometheusEndpoint())
	if cfg.AlertmanagerEnabled {
		fmt.Printf("  Alertmanager endpoint: %s\n", cfg.GetAlertmanagerEndpoint())
	}
	return nil
}

// buildHelmValues constructs Helm values for kube-prometheus-stack installation
func buildHelmValues(cfg *Config) map[string]interface{} {
	values := make(map[string]interface{})

	// Start with user-provided values
	for k, v := range cfg.Values {
		values[k] = v
	}

	// Prometheus configuration
	prometheusSpec := map[string]interface{}{
		"retention":     fmt.Sprintf("%dd", cfg.RetentionDays),
		"retentionSize": cfg.RetentionSize,
		"scrapeInterval": cfg.ScrapeInterval,
	}

	// Storage configuration
	if cfg.StorageSize != "" {
		storageSpec := map[string]interface{}{
			"volumeClaimTemplate": map[string]interface{}{
				"spec": map[string]interface{}{
					"accessModes": []string{"ReadWriteOnce"},
					"resources": map[string]interface{}{
						"requests": map[string]interface{}{
							"storage": cfg.StorageSize,
						},
					},
				},
			},
		}
		if cfg.StorageClass != "" {
			storageSpec["volumeClaimTemplate"].(map[string]interface{})["spec"].(map[string]interface{})["storageClassName"] = cfg.StorageClass
		}
		prometheusSpec["storageSpec"] = storageSpec
	}

	// ServiceMonitor selector - discover all ServiceMonitors
	prometheusSpec["serviceMonitorSelectorNilUsesHelmValues"] = false
	prometheusSpec["serviceMonitorSelector"] = map[string]interface{}{}
	prometheusSpec["serviceMonitorNamespaceSelector"] = map[string]interface{}{}

	// PodMonitor selector - discover all PodMonitors
	prometheusSpec["podMonitorSelectorNilUsesHelmValues"] = false
	prometheusSpec["podMonitorSelector"] = map[string]interface{}{}
	prometheusSpec["podMonitorNamespaceSelector"] = map[string]interface{}{}

	// ProbeSelector - discover all Probes
	prometheusSpec["probeSelectorNilUsesHelmValues"] = false
	prometheusSpec["probeSelector"] = map[string]interface{}{}
	prometheusSpec["probeNamespaceSelector"] = map[string]interface{}{}

	// Rule selector - discover all PrometheusRules
	prometheusSpec["ruleSelectorNilUsesHelmValues"] = false
	prometheusSpec["ruleSelector"] = map[string]interface{}{}
	prometheusSpec["ruleNamespaceSelector"] = map[string]interface{}{}

	values["prometheus"] = map[string]interface{}{
		"prometheusSpec": prometheusSpec,
	}

	// Ingress configuration
	if cfg.IngressEnabled {
		values["prometheus"].(map[string]interface{})["ingress"] = map[string]interface{}{
			"enabled":          true,
			"ingressClassName": "contour",
			"hosts":            []string{cfg.IngressHost},
			"tls": []map[string]interface{}{
				{
					"hosts":      []string{cfg.IngressHost},
					"secretName": "prometheus-tls",
				},
			},
		}
	}

	// Alertmanager configuration
	if cfg.AlertmanagerEnabled {
		values["alertmanager"] = map[string]interface{}{
			"enabled": true,
			"alertmanagerSpec": map[string]interface{}{
				"storage": map[string]interface{}{
					"volumeClaimTemplate": map[string]interface{}{
						"spec": map[string]interface{}{
							"accessModes": []string{"ReadWriteOnce"},
							"resources": map[string]interface{}{
								"requests": map[string]interface{}{
									"storage": "5Gi",
								},
							},
						},
					},
				},
			},
		}
		if cfg.StorageClass != "" {
			values["alertmanager"].(map[string]interface{})["alertmanagerSpec"].(map[string]interface{})["storage"].(map[string]interface{})["volumeClaimTemplate"].(map[string]interface{})["spec"].(map[string]interface{})["storageClassName"] = cfg.StorageClass
		}
	} else {
		values["alertmanager"] = map[string]interface{}{
			"enabled": false,
		}
	}

	// Grafana - disabled as we deploy it separately
	values["grafana"] = map[string]interface{}{
		"enabled": cfg.GrafanaEnabled,
	}

	// Node Exporter configuration
	values["nodeExporter"] = map[string]interface{}{
		"enabled": cfg.NodeExporterEnabled,
	}
	// Prometheus-node-exporter subchart configuration
	values["prometheus-node-exporter"] = map[string]interface{}{
		"hostRootFsMount": map[string]interface{}{
			"enabled": true,
		},
	}

	// Kube State Metrics configuration
	values["kubeStateMetrics"] = map[string]interface{}{
		"enabled": cfg.KubeStateMetricsEnabled,
	}

	// Default resource requests (reasonable for homelab)
	values["prometheusOperator"] = map[string]interface{}{
		"resources": map[string]interface{}{
			"requests": map[string]interface{}{
				"cpu":    "100m",
				"memory": "128Mi",
			},
		},
	}

	return values
}

// verifyInstallation verifies that Prometheus pods are running
func verifyInstallation(ctx context.Context, k8sClient K8sClient, namespace string) error {
	if k8sClient == nil {
		return nil // Skip verification if no k8s client
	}

	// Wait for pods to be ready (up to 3 minutes for prometheus stack)
	timeout := time.After(3 * time.Minute)
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-timeout:
			return fmt.Errorf("timeout waiting for prometheus pods to be ready")
		case <-ticker.C:
			pods, err := k8sClient.GetPods(ctx, namespace)
			if err != nil {
				continue // Retry on error
			}

			if len(pods) == 0 {
				continue // Wait for pods to appear
			}

			// Check if prometheus pod is running
			prometheusFound := false
			for _, pod := range pods {
				if pod.Name == "" {
					continue
				}
				// Look for prometheus-prometheus pod
				if containsSubstring(pod.Name, "prometheus-kube-prometheus-stack-prometheus") ||
					containsSubstring(pod.Name, "prometheus-prometheus") {
					prometheusFound = true
					if pod.Status != "Running" {
						break
					}
				}
			}

			if prometheusFound {
				return nil
			}
		}
	}
}

// containsSubstring checks if s contains substr
func containsSubstring(s, substr string) bool {
	if len(substr) > len(s) {
		return false
	}
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

// CreateServiceMonitor creates a ServiceMonitor for a service
// This is a helper for other components to expose metrics to Prometheus
type ServiceMonitorSpec struct {
	Name       string            // Name of the ServiceMonitor
	Namespace  string            // Namespace for the ServiceMonitor
	Selector   map[string]string // Label selector for target services
	Port       string            // Port name to scrape (e.g., "metrics")
	Path       string            // Metrics path (default: /metrics)
	Interval   string            // Scrape interval (default: 30s)
	TargetPort int               // Target port number (if port name not used)
}

// GetServiceMonitorYAML returns the YAML for a ServiceMonitor
func GetServiceMonitorYAML(spec ServiceMonitorSpec) string {
	if spec.Path == "" {
		spec.Path = "/metrics"
	}
	if spec.Interval == "" {
		spec.Interval = "30s"
	}

	selectorLabels := ""
	for k, v := range spec.Selector {
		selectorLabels += fmt.Sprintf("        %s: %s\n", k, v)
	}

	endpoint := ""
	if spec.Port != "" {
		endpoint = fmt.Sprintf("port: %s", spec.Port)
	} else if spec.TargetPort > 0 {
		endpoint = fmt.Sprintf("targetPort: %d", spec.TargetPort)
	}

	return fmt.Sprintf(`apiVersion: monitoring.coreos.com/v1
kind: ServiceMonitor
metadata:
  name: %s
  namespace: %s
spec:
  selector:
    matchLabels:
%s  endpoints:
    - %s
      path: %s
      interval: %s
`, spec.Name, spec.Namespace, selectorLabels, endpoint, spec.Path, spec.Interval)
}
