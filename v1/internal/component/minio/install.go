package minio

import (
	"context"
	"fmt"
	"time"

	"github.com/catalystcommunity/foundry/v1/internal/helm"
)

const (
	minioRepoName = "minio"
	minioRepoURL  = "https://charts.min.io/"
	minioChart    = "minio/minio"
)

// Install installs MinIO using Helm
func Install(ctx context.Context, helmClient HelmClient, k8sClient K8sClient, cfg *Config) error {
	if helmClient == nil {
		return fmt.Errorf("helm client cannot be nil")
	}
	if cfg == nil {
		cfg = DefaultConfig()
	}

	fmt.Println("  Installing MinIO...")

	// Add Helm repository
	if err := helmClient.AddRepo(ctx, helm.RepoAddOptions{
		Name:        minioRepoName,
		URL:         minioRepoURL,
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
			if rel.Name == "minio" {
				if rel.Status == "deployed" {
					fmt.Println("  MinIO already installed")
					return verifyInstallation(ctx, k8sClient, cfg.Namespace)
				}
				// Uninstall failed release
				fmt.Printf("  Removing failed release (status: %s)...\n", rel.Status)
				if err := helmClient.Uninstall(ctx, helm.UninstallOptions{
					ReleaseName: "minio",
					Namespace:   cfg.Namespace,
				}); err != nil {
					return fmt.Errorf("failed to uninstall existing release: %w", err)
				}
				break
			}
		}
	}

	// Install MinIO via Helm
	if err := helmClient.Install(ctx, helm.InstallOptions{
		ReleaseName:     "minio",
		Namespace:       cfg.Namespace,
		Chart:           minioChart,
		Version:         cfg.Version,
		Values:          values,
		CreateNamespace: true,
		Wait:            true,
		Timeout:         10 * time.Minute,
	}); err != nil {
		return fmt.Errorf("failed to install minio: %w", err)
	}

	// Verify installation
	if k8sClient != nil {
		if err := verifyInstallation(ctx, k8sClient, cfg.Namespace); err != nil {
			return fmt.Errorf("installation verification failed: %w", err)
		}
	}

	fmt.Println("  MinIO installed successfully")
	fmt.Printf("  Endpoint: %s\n", cfg.GetEndpoint())
	return nil
}

// buildHelmValues constructs Helm values for MinIO installation
func buildHelmValues(cfg *Config) map[string]interface{} {
	values := make(map[string]interface{})

	// Start with user-provided values
	for k, v := range cfg.Values {
		values[k] = v
	}

	// Deployment mode
	if cfg.Mode == ModeStandalone {
		values["mode"] = "standalone"
	} else {
		values["mode"] = "distributed"
		values["replicas"] = cfg.Replicas
	}

	// Root credentials
	values["rootUser"] = cfg.RootUser
	if cfg.RootPassword != "" {
		values["rootPassword"] = cfg.RootPassword
	}

	// Persistence configuration
	persistence := map[string]interface{}{
		"enabled": true,
		"size":    cfg.StorageSize,
	}
	if cfg.StorageClass != "" {
		persistence["storageClass"] = cfg.StorageClass
	}
	values["persistence"] = persistence

	// Resource configuration (reasonable defaults)
	values["resources"] = map[string]interface{}{
		"requests": map[string]interface{}{
			"memory": "512Mi",
		},
	}

	// Service configuration
	values["service"] = map[string]interface{}{
		"type": "ClusterIP",
	}

	// Console service configuration
	values["consoleService"] = map[string]interface{}{
		"type": "ClusterIP",
	}

	// Ingress configuration
	if cfg.IngressEnabled {
		values["ingress"] = map[string]interface{}{
			"enabled":   true,
			"ingressClassName": "contour",
			"hosts": []string{cfg.IngressHost},
			"tls": []map[string]interface{}{
				{
					"hosts":      []string{cfg.IngressHost},
					"secretName": "minio-tls",
				},
			},
		}
		values["consoleIngress"] = map[string]interface{}{
			"enabled":   true,
			"ingressClassName": "contour",
			"hosts": []string{"console." + cfg.IngressHost},
			"tls": []map[string]interface{}{
				{
					"hosts":      []string{"console." + cfg.IngressHost},
					"secretName": "minio-console-tls",
				},
			},
		}
	}

	// Bucket configuration
	if len(cfg.Buckets) > 0 {
		buckets := make([]map[string]interface{}, 0, len(cfg.Buckets))
		for _, bucket := range cfg.Buckets {
			buckets = append(buckets, map[string]interface{}{
				"name":   bucket,
				"policy": "none",
				"purge":  false,
			})
		}
		values["buckets"] = buckets
	}

	// TLS configuration
	if cfg.TLSEnabled {
		values["tls"] = map[string]interface{}{
			"enabled": true,
		}
	}

	return values
}

// verifyInstallation verifies that MinIO pods are running
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
			return fmt.Errorf("timeout waiting for minio pods to be ready")
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
			minioPodFound := false
			for _, pod := range pods {
				if pod.Name == "" {
					continue
				}
				// Check for minio pods specifically
				if len(pod.Name) >= 5 && pod.Name[:5] == "minio" {
					minioPodFound = true
					if pod.Status != "Running" {
						allRunning = false
						break
					}
				}
			}

			if minioPodFound && allRunning {
				return nil
			}
		}
	}
}

// CreateBucket creates a bucket in MinIO (for use after installation)
func CreateBucket(ctx context.Context, k8sClient K8sClient, namespace, bucketName string) error {
	// This would typically use the MinIO client SDK or kubectl exec
	// For now, we rely on the Helm chart's bucket creation feature
	return fmt.Errorf("CreateBucket not implemented - use buckets config in Helm values")
}

// GetConnectionInfo returns the connection information for MinIO
type ConnectionInfo struct {
	Endpoint    string
	AccessKey   string
	SecretKey   string
	UseSSL      bool
	Region      string
}

// GetConnectionInfo returns MinIO connection details for other components
func GetConnectionInfo(cfg *Config) *ConnectionInfo {
	return &ConnectionInfo{
		Endpoint:  fmt.Sprintf("minio.%s.svc.cluster.local:9000", cfg.Namespace),
		AccessKey: cfg.RootUser,
		SecretKey: cfg.RootPassword,
		UseSSL:    cfg.TLSEnabled,
		Region:    "us-east-1", // Default region
	}
}
