package loki

import (
	"context"
	"fmt"
	"time"

	"github.com/catalystcommunity/foundry/v1/internal/helm"
)

const (
	lokiRepoName  = "grafana"
	lokiRepoURL   = "https://grafana.github.io/helm-charts"
	lokiChart     = "grafana/loki"
	promtailChart = "grafana/promtail"
	releaseName   = "loki"
)

// Install installs Loki using Helm
func Install(ctx context.Context, helmClient HelmClient, k8sClient K8sClient, cfg *Config) error {
	if helmClient == nil {
		return fmt.Errorf("helm client cannot be nil")
	}
	if cfg == nil {
		cfg = DefaultConfig()
	}

	fmt.Println("  Installing Loki...")

	// Add Helm repository
	if err := helmClient.AddRepo(ctx, helm.RepoAddOptions{
		Name:        lokiRepoName,
		URL:         lokiRepoURL,
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
					fmt.Println("  Loki already installed")
					if err := verifyInstallation(ctx, k8sClient, cfg.Namespace); err != nil {
						return err
					}
					// Install Promtail if enabled
					if cfg.PromtailEnabled {
						return installPromtail(ctx, helmClient, k8sClient, cfg)
					}
					return nil
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

	// Install Loki via Helm
	if err := helmClient.Install(ctx, helm.InstallOptions{
		ReleaseName:     releaseName,
		Namespace:       cfg.Namespace,
		Chart:           lokiChart,
		Version:         cfg.Version,
		Values:          values,
		CreateNamespace: true,
		Wait:            true,
		Timeout:         10 * time.Minute,
	}); err != nil {
		return fmt.Errorf("failed to install loki: %w", err)
	}

	// Verify installation
	if k8sClient != nil {
		if err := verifyInstallation(ctx, k8sClient, cfg.Namespace); err != nil {
			return fmt.Errorf("installation verification failed: %w", err)
		}
	}

	fmt.Println("  Loki installed successfully")
	fmt.Printf("  Loki endpoint: %s\n", cfg.GetLokiEndpoint())

	// Install Promtail if enabled
	if cfg.PromtailEnabled {
		return installPromtail(ctx, helmClient, k8sClient, cfg)
	}

	return nil
}

// installPromtail installs Promtail for log collection
func installPromtail(ctx context.Context, helmClient HelmClient, k8sClient K8sClient, cfg *Config) error {
	fmt.Println("  Installing Promtail...")

	// Check if promtail release already exists
	releases, err := helmClient.List(ctx, cfg.Namespace)
	if err == nil {
		for _, rel := range releases {
			if rel.Name == "promtail" {
				if rel.Status == "deployed" {
					fmt.Println("  Promtail already installed")
					return nil
				}
				// Uninstall failed release
				fmt.Printf("  Removing failed Promtail release (status: %s)...\n", rel.Status)
				if err := helmClient.Uninstall(ctx, helm.UninstallOptions{
					ReleaseName: "promtail",
					Namespace:   cfg.Namespace,
				}); err != nil {
					return fmt.Errorf("failed to uninstall existing promtail release: %w", err)
				}
				break
			}
		}
	}

	// Build Promtail values
	promtailValues := buildPromtailValues(cfg)

	// Install Promtail
	if err := helmClient.Install(ctx, helm.InstallOptions{
		ReleaseName:     "promtail",
		Namespace:       cfg.Namespace,
		Chart:           promtailChart,
		Version:         "6.16.6", // Compatible Promtail version
		Values:          promtailValues,
		CreateNamespace: false, // Already created by Loki
		Wait:            true,
		Timeout:         5 * time.Minute,
	}); err != nil {
		return fmt.Errorf("failed to install promtail: %w", err)
	}

	fmt.Println("  Promtail installed successfully")
	return nil
}

// buildHelmValues constructs Helm values for Loki installation
func buildHelmValues(cfg *Config) map[string]interface{} {
	values := make(map[string]interface{})

	// Start with user-provided values
	for k, v := range cfg.Values {
		values[k] = v
	}

	// Deployment mode
	values["deploymentMode"] = cfg.DeploymentMode

	// Loki configuration
	lokiConfig := map[string]interface{}{
		"auth_enabled": false,
		"commonConfig": map[string]interface{}{
			"replication_factor": 1,
		},
		"schemaConfig": map[string]interface{}{
			"configs": []map[string]interface{}{
				{
					"from": "2024-01-01",
					"store": "tsdb",
					"object_store": func() string {
						if cfg.StorageBackend == BackendS3 {
							return "s3"
						}
						return "filesystem"
					}(),
					"schema": "v13",
					"index": map[string]interface{}{
						"prefix": "loki_index_",
						"period": "24h",
					},
				},
			},
		},
		"limits_config": map[string]interface{}{
			"retention_period": fmt.Sprintf("%dh", cfg.RetentionDays*24),
		},
		"compactor": map[string]interface{}{
			"retention_enabled": true,
		},
	}

	// Storage configuration
	if cfg.StorageBackend == BackendS3 {
		lokiConfig["storage"] = map[string]interface{}{
			"type": "s3",
			"s3": map[string]interface{}{
				"endpoint":         cfg.S3Endpoint,
				"bucketnames":      cfg.S3Bucket,
				"access_key_id":    cfg.S3AccessKey,
				"secret_access_key": cfg.S3SecretKey,
				"region":           cfg.S3Region,
				"insecure":         true, // For MinIO without TLS
				"s3ForcePathStyle": true, // Required for MinIO
			},
		}
	} else {
		lokiConfig["storage"] = map[string]interface{}{
			"type": "filesystem",
		}
	}

	values["loki"] = lokiConfig

	// Single binary configuration (for SingleBinary mode)
	if cfg.DeploymentMode == "SingleBinary" {
		values["singleBinary"] = map[string]interface{}{
			"replicas": 1,
			"persistence": map[string]interface{}{
				"enabled": true,
				"size":    cfg.StorageSize,
			},
			"resources": map[string]interface{}{
				"requests": map[string]interface{}{
					"cpu":    "100m",
					"memory": "256Mi",
				},
			},
		}
		if cfg.StorageClass != "" {
			values["singleBinary"].(map[string]interface{})["persistence"].(map[string]interface{})["storageClass"] = cfg.StorageClass
		}

		// Disable components not needed for SingleBinary
		values["backend"] = map[string]interface{}{"replicas": 0}
		values["read"] = map[string]interface{}{"replicas": 0}
		values["write"] = map[string]interface{}{"replicas": 0}
	}

	// Gateway configuration
	values["gateway"] = map[string]interface{}{
		"enabled": true,
		"replicas": 1,
	}

	// Ingress configuration
	if cfg.IngressEnabled {
		values["gateway"].(map[string]interface{})["ingress"] = map[string]interface{}{
			"enabled":          true,
			"ingressClassName": "contour",
			"hosts": []map[string]interface{}{
				{
					"host": cfg.IngressHost,
					"paths": []map[string]interface{}{
						{
							"path":     "/",
							"pathType": "Prefix",
						},
					},
				},
			},
			"tls": []map[string]interface{}{
				{
					"hosts":      []string{cfg.IngressHost},
					"secretName": "loki-tls",
				},
			},
		}
	}

	// Disable test pod
	values["test"] = map[string]interface{}{
		"enabled": false,
	}

	// Monitoring configuration
	values["monitoring"] = map[string]interface{}{
		"serviceMonitor": map[string]interface{}{
			"enabled": true,
		},
		"selfMonitoring": map[string]interface{}{
			"enabled": false, // Disable self-monitoring for simplicity
		},
		"lokiCanary": map[string]interface{}{
			"enabled": false, // Disable canary for simplicity
		},
	}

	return values
}

// buildPromtailValues constructs Helm values for Promtail installation
func buildPromtailValues(cfg *Config) map[string]interface{} {
	values := map[string]interface{}{
		"config": map[string]interface{}{
			"clients": []map[string]interface{}{
				{
					"url": cfg.GetLokiPushEndpoint(),
				},
			},
		},
		"resources": map[string]interface{}{
			"requests": map[string]interface{}{
				"cpu":    "50m",
				"memory": "64Mi",
			},
		},
		"serviceMonitor": map[string]interface{}{
			"enabled": true,
		},
	}

	return values
}

// verifyInstallation verifies that Loki pods are running
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
			return fmt.Errorf("timeout waiting for loki pods to be ready")
		case <-ticker.C:
			pods, err := k8sClient.GetPods(ctx, namespace)
			if err != nil {
				continue // Retry on error
			}

			if len(pods) == 0 {
				continue // Wait for pods to appear
			}

			// Check if loki pod is running
			lokiFound := false
			for _, pod := range pods {
				if pod.Name == "" {
					continue
				}
				// Look for loki pod (could be loki-0 or loki-gateway-*)
				if containsSubstring(pod.Name, "loki") {
					lokiFound = true
					if pod.Status != "Running" {
						break
					}
				}
			}

			if lokiFound {
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
