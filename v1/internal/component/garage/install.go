package garage

import (
	"context"
	"fmt"
	"time"

	"github.com/catalystcommunity/foundry/v1/internal/helm"
)

const (
	garageRepoName = "garage"
	garageRepoURL  = "https://git.deuxfleurs.fr/Deuxfleurs/garage/raw/branch/main/script/helm/garage"
	garageChart    = "garage/garage"
)

// Install installs Garage using Helm
func Install(ctx context.Context, helmClient HelmClient, k8sClient K8sClient, cfg *Config) error {
	if helmClient == nil {
		return fmt.Errorf("helm client cannot be nil")
	}
	if cfg == nil {
		cfg = DefaultConfig()
	}

	fmt.Println("  Installing Garage...")

	// Add Helm repository
	if err := helmClient.AddRepo(ctx, helm.RepoAddOptions{
		Name:        garageRepoName,
		URL:         garageRepoURL,
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
			if rel.Name == "garage" {
				if rel.Status == "deployed" {
					fmt.Println("  Garage already installed")
					return verifyInstallation(ctx, k8sClient, cfg.Namespace)
				}
				// Uninstall failed release
				fmt.Printf("  Removing failed release (status: %s)...\n", rel.Status)
				if err := helmClient.Uninstall(ctx, helm.UninstallOptions{
					ReleaseName: "garage",
					Namespace:   cfg.Namespace,
				}); err != nil {
					return fmt.Errorf("failed to uninstall existing release: %w", err)
				}
				break
			}
		}
	}

	// Install Garage via Helm
	if err := helmClient.Install(ctx, helm.InstallOptions{
		ReleaseName:     "garage",
		Namespace:       cfg.Namespace,
		Chart:           garageChart,
		Version:         cfg.Version,
		Values:          values,
		CreateNamespace: true,
		Wait:            true,
		Timeout:         10 * time.Minute,
	}); err != nil {
		return fmt.Errorf("failed to install garage: %w", err)
	}

	// Verify installation
	if k8sClient != nil {
		if err := verifyInstallation(ctx, k8sClient, cfg.Namespace); err != nil {
			return fmt.Errorf("installation verification failed: %w", err)
		}
	}

	fmt.Println("  Garage installed successfully")
	fmt.Printf("  S3 Endpoint: %s\n", cfg.GetEndpoint())
	fmt.Printf("  Admin Endpoint: %s\n", cfg.GetAdminEndpoint())
	return nil
}

// buildHelmValues constructs Helm values for Garage installation
func buildHelmValues(cfg *Config) map[string]interface{} {
	values := make(map[string]interface{})

	// Start with user-provided values
	for k, v := range cfg.Values {
		values[k] = v
	}

	// Garage configuration
	garageConfig := map[string]interface{}{
		"replication_factor": cfg.ReplicationFactor,
		"rpc_bind_addr":      "[::]:3901",
		"rpc_public_addr":    "[::]:3901",
		"db_engine":          "lmdb",
		"block_size":         "1048576", // 1MB blocks
		"metadata_dir":       "/mnt/meta",
		"data_dir":           "/mnt/data",
		"s3_api": map[string]interface{}{
			"s3_region":   cfg.S3Region,
			"api_bind_addr": "[::]:3900",
			"root_domain": ".s3.garage.svc.cluster.local",
		},
		"admin": map[string]interface{}{
			"api_bind_addr": "[::]:3903",
			"admin_token":   cfg.AdminKey,
		},
	}
	values["garage"] = garageConfig

	// Persistence configuration
	persistence := map[string]interface{}{
		"enabled": true,
		"meta": map[string]interface{}{
			"size": "1Gi", // Metadata is small
		},
		"data": map[string]interface{}{
			"size": cfg.StorageSize,
		},
	}
	if cfg.StorageClass != "" {
		persistence["meta"].(map[string]interface{})["storageClass"] = cfg.StorageClass
		persistence["data"].(map[string]interface{})["storageClass"] = cfg.StorageClass
	}
	values["persistence"] = persistence

	// Deployment configuration
	values["replicaCount"] = cfg.Replicas

	// Resource configuration (reasonable defaults for homelab)
	values["resources"] = map[string]interface{}{
		"requests": map[string]interface{}{
			"memory": "256Mi",
			"cpu":    "100m",
		},
		"limits": map[string]interface{}{
			"memory": "1Gi",
		},
	}

	// Service configuration
	values["service"] = map[string]interface{}{
		"type": "ClusterIP",
		"s3": map[string]interface{}{
			"port": 3900,
		},
		"rpc": map[string]interface{}{
			"port": 3901,
		},
		"admin": map[string]interface{}{
			"port": 3903,
		},
	}

	// Environment variables for admin credentials
	values["env"] = []map[string]interface{}{
		{
			"name":  "GARAGE_ADMIN_TOKEN",
			"value": cfg.AdminKey,
		},
	}

	return values
}

// verifyInstallation verifies that Garage pods are running
func verifyInstallation(ctx context.Context, k8sClient K8sClient, namespace string) error {
	if k8sClient == nil {
		return nil // Skip verification if no k8s client
	}

	// Wait for pods to be ready (up to 2 minutes)
	timeout := time.After(2 * time.Minute)
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-timeout:
			return fmt.Errorf("timeout waiting for garage pods to be ready")
		case <-ticker.C:
			pods, err := k8sClient.GetPods(ctx, namespace)
			if err != nil {
				continue // Retry on error
			}

			if len(pods) == 0 {
				continue // Wait for pods to appear
			}

			// Check if all pods are running
			allRunning := true
			garagePodFound := false
			for _, pod := range pods {
				if pod.Name == "" {
					continue
				}
				// Check for garage pods specifically
				if containsSubstring(pod.Name, "garage") {
					garagePodFound = true
					if pod.Status != "Running" {
						allRunning = false
						break
					}
				}
			}

			if garagePodFound && allRunning {
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

// ConnectionInfo holds connection details for other components
type ConnectionInfo struct {
	Endpoint    string
	AccessKey   string
	SecretKey   string
	UseSSL      bool
	Region      string
}

// GetConnectionInfo returns Garage connection details for other components
func GetConnectionInfo(cfg *Config) *ConnectionInfo {
	return &ConnectionInfo{
		Endpoint:  fmt.Sprintf("garage.%s.svc.cluster.local:3900", cfg.Namespace),
		AccessKey: cfg.AdminKey,
		SecretKey: cfg.AdminSecret,
		UseSSL:    false,
		Region:    cfg.S3Region,
	}
}
