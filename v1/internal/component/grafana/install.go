package grafana

import (
	"context"
	"fmt"
	"time"

	"github.com/catalystcommunity/foundry/v1/internal/helm"
)

const (
	grafanaRepoName = "grafana"
	grafanaRepoURL  = "https://grafana.github.io/helm-charts"
	grafanaChart    = "grafana/grafana"
	releaseName     = "grafana"
)

// Install installs Grafana using Helm
func Install(ctx context.Context, helmClient HelmClient, k8sClient K8sClient, cfg *Config) error {
	if helmClient == nil {
		return fmt.Errorf("helm client cannot be nil")
	}
	if cfg == nil {
		cfg = DefaultConfig()
	}

	fmt.Println("  Installing Grafana...")

	// Add Helm repository
	if err := helmClient.AddRepo(ctx, helm.RepoAddOptions{
		Name:        grafanaRepoName,
		URL:         grafanaRepoURL,
		ForceUpdate: true,
	}); err != nil {
		return fmt.Errorf("failed to add helm repository: %w", err)
	}

	// Build Helm values
	values := buildHelmValues(cfg)

	// Check if release already exists
	var releaseExists bool
	var releaseStatus string
	releases, err := helmClient.List(ctx, cfg.Namespace)
	if err == nil {
		for _, rel := range releases {
			if rel.Name == releaseName {
				releaseExists = true
				releaseStatus = rel.Status
				break
			}
		}
	}

	if releaseExists {
		// Try to upgrade existing release (even if failed - avoid data loss)
		fmt.Printf("  Upgrading Grafana (current status: %s)...\n", releaseStatus)
		if err := helmClient.Upgrade(ctx, helm.UpgradeOptions{
			ReleaseName: releaseName,
			Namespace:   cfg.Namespace,
			Chart:       grafanaChart,
			Version:     cfg.Version,
			Values:      values,
			Wait:        true,
			Timeout:     15 * time.Minute,
		}); err != nil {
			if releaseStatus != "deployed" {
				// Upgrade of failed release didn't work - warn and skip
				fmt.Printf("  ⚠ Warning: Failed to upgrade release (status: %s): %v\n", releaseStatus, err)
				fmt.Println("  ⚠ Manual intervention required. You may need to:")
				fmt.Println("    1. Check pod status: kubectl get pods -n", cfg.Namespace, "-l app.kubernetes.io/name=grafana")
				fmt.Println("    2. Check PVC status: kubectl get pvc -n", cfg.Namespace)
				fmt.Println("    3. If data loss is acceptable, uninstall manually: helm uninstall grafana -n", cfg.Namespace)
				return fmt.Errorf("failed to upgrade grafana (manual intervention required): %w", err)
			}
			return fmt.Errorf("failed to upgrade grafana: %w", err)
		}
	} else {
		// Install Grafana via Helm
		if err := helmClient.Install(ctx, helm.InstallOptions{
			ReleaseName:     releaseName,
			Namespace:       cfg.Namespace,
			Chart:           grafanaChart,
			Version:         cfg.Version,
			Values:          values,
			CreateNamespace: true,
			Wait:            true,
			Timeout:         10 * time.Minute,
		}); err != nil {
			return fmt.Errorf("failed to install grafana: %w", err)
		}
	}

	// Verify installation
	if k8sClient != nil {
		if err := verifyInstallation(ctx, k8sClient, cfg.Namespace); err != nil {
			return fmt.Errorf("installation verification failed: %w", err)
		}
	}

	fmt.Println("  Grafana installed successfully")
	fmt.Printf("  Grafana endpoint: %s\n", cfg.GetGrafanaEndpoint())
	if cfg.IngressEnabled {
		fmt.Printf("  Grafana URL: https://%s\n", cfg.IngressHost)
	}
	return nil
}

// buildHelmValues constructs Helm values for Grafana installation
func buildHelmValues(cfg *Config) map[string]interface{} {
	values := make(map[string]interface{})

	// Start with user-provided values
	for k, v := range cfg.Values {
		values[k] = v
	}

	// Admin credentials
	values["adminUser"] = cfg.AdminUser
	if cfg.AdminPassword != "" {
		values["adminPassword"] = cfg.AdminPassword
	}

	// Persistence configuration
	persistence := map[string]interface{}{
		"enabled": true,
		"size":    cfg.StorageSize,
	}
	if cfg.StorageClass != "" {
		persistence["storageClassName"] = cfg.StorageClass
	}
	values["persistence"] = persistence

	// Service configuration
	values["service"] = map[string]interface{}{
		"type": "ClusterIP",
		"port": 80,
	}

	// Resource configuration (reasonable defaults for homelab)
	values["resources"] = map[string]interface{}{
		"requests": map[string]interface{}{
			"cpu":    "100m",
			"memory": "128Mi",
		},
	}

	// Ingress configuration
	if cfg.IngressEnabled {
		values["ingress"] = map[string]interface{}{
			"enabled":          true,
			"ingressClassName": "contour",
			"hosts":            []string{cfg.IngressHost},
			"annotations": map[string]interface{}{
				"cert-manager.io/cluster-issuer": "foundry-ca-issuer",
			},
			"tls": []map[string]interface{}{
				{
					"hosts":      []string{cfg.IngressHost},
					"secretName": "grafana-tls",
				},
			},
		}
	}

	// Data sources configuration
	datasources := buildDatasources(cfg)
	values["datasources"] = datasources

	// Sidecar configuration for dynamic dashboard/datasource discovery
	if cfg.SidecarEnabled {
		values["sidecar"] = map[string]interface{}{
			"dashboards": map[string]interface{}{
				"enabled":         true,
				"searchNamespace": "ALL",
				"label":           "grafana_dashboard",
				"labelValue":      "1",
				"folderAnnotation": "grafana_folder",
			},
			"datasources": map[string]interface{}{
				"enabled":         true,
				"searchNamespace": "ALL",
				"label":           "grafana_datasource",
				"labelValue":      "1",
			},
		}
	}

	// Default dashboards
	if cfg.DefaultDashboardsEnabled {
		values["dashboardProviders"] = map[string]interface{}{
			"dashboardproviders.yaml": map[string]interface{}{
				"apiVersion": 1,
				"providers": []map[string]interface{}{
					{
						"name":            "default",
						"orgId":           1,
						"folder":          "",
						"type":            "file",
						"disableDeletion": false,
						"editable":        true,
						"options": map[string]interface{}{
							"path": "/var/lib/grafana/dashboards/default",
						},
					},
				},
			},
		}

		// Include some default dashboards
		values["dashboards"] = map[string]interface{}{
			"default": map[string]interface{}{
				"kubernetes-cluster": map[string]interface{}{
					"gnetId":     15520,
					"revision":   1,
					"datasource": "Prometheus",
				},
				"node-exporter": map[string]interface{}{
					"gnetId":     1860,
					"revision":   37,
					"datasource": "Prometheus",
				},
			},
		}
	}

	// Grafana.ini configuration
	values["grafana.ini"] = map[string]interface{}{
		"server": map[string]interface{}{
			"root_url": func() string {
				if cfg.IngressEnabled && cfg.IngressHost != "" {
					return fmt.Sprintf("https://%s", cfg.IngressHost)
				}
				return "%(protocol)s://%(domain)s:%(http_port)s/"
			}(),
		},
		"analytics": map[string]interface{}{
			"check_for_updates": false,
			"reporting_enabled": false,
		},
		"security": map[string]interface{}{
			"disable_gravatar": true,
		},
		"users": map[string]interface{}{
			"allow_sign_up": false,
		},
	}

	// ServiceMonitor for Prometheus
	values["serviceMonitor"] = map[string]interface{}{
		"enabled": true,
	}

	return values
}

// buildDatasources creates the datasources configuration
func buildDatasources(cfg *Config) map[string]interface{} {
	datasources := []map[string]interface{}{}

	// Prometheus data source
	if cfg.PrometheusURL != "" {
		datasources = append(datasources, map[string]interface{}{
			"name":      "Prometheus",
			"type":      "prometheus",
			"url":       cfg.PrometheusURL,
			"access":    "proxy",
			"isDefault": true,
			"editable":  true,
		})
	}

	// Loki data source
	if cfg.LokiURL != "" {
		datasources = append(datasources, map[string]interface{}{
			"name":     "Loki",
			"type":     "loki",
			"url":      cfg.LokiURL,
			"access":   "proxy",
			"editable": true,
			"jsonData": map[string]interface{}{
				"maxLines": 1000,
			},
		})
	}

	return map[string]interface{}{
		"datasources.yaml": map[string]interface{}{
			"apiVersion":  1,
			"datasources": datasources,
		},
	}
}

// verifyInstallation verifies that Grafana pods are running
func verifyInstallation(ctx context.Context, k8sClient K8sClient, namespace string) error {
	if k8sClient == nil {
		return nil // Skip verification if no k8s client
	}

	// Wait for pods to be ready (up to 3 minutes)
	timeout := time.After(3 * time.Minute)
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-timeout:
			return fmt.Errorf("timeout waiting for grafana pods to be ready")
		case <-ticker.C:
			pods, err := k8sClient.GetPods(ctx, namespace)
			if err != nil {
				continue // Retry on error
			}

			if len(pods) == 0 {
				continue // Wait for pods to appear
			}

			// Check if grafana pod is running
			grafanaFound := false
			for _, pod := range pods {
				if pod.Name == "" {
					continue
				}
				// Look for grafana pod
				if containsSubstring(pod.Name, "grafana") {
					grafanaFound = true
					if pod.Status != "Running" {
						break
					}
				}
			}

			if grafanaFound {
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
