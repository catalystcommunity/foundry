package velero

import (
	"context"
	"fmt"
	"time"

	"github.com/catalystcommunity/foundry/v1/internal/helm"
)

const (
	veleroRepoName = "vmware-tanzu"
	veleroRepoURL  = "https://vmware-tanzu.github.io/helm-charts"
	veleroChart    = "vmware-tanzu/velero"
	releaseName    = "velero"
)

// Install installs Velero using Helm
func Install(ctx context.Context, helmClient HelmClient, k8sClient K8sClient, cfg *Config) error {
	if helmClient == nil {
		return fmt.Errorf("helm client cannot be nil")
	}
	if cfg == nil {
		cfg = DefaultConfig()
	}

	fmt.Println("  Installing Velero...")

	// Add Helm repository
	if err := helmClient.AddRepo(ctx, helm.RepoAddOptions{
		Name:        veleroRepoName,
		URL:         veleroRepoURL,
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
		fmt.Printf("  Upgrading Velero (current status: %s)...\n", releaseStatus)
		if err := helmClient.Upgrade(ctx, helm.UpgradeOptions{
			ReleaseName: releaseName,
			Namespace:   cfg.Namespace,
			Chart:       veleroChart,
			Version:     cfg.Version,
			Values:      values,
			Wait:        true,
			Timeout:     10 * time.Minute,
		}); err != nil {
			if releaseStatus != "deployed" {
				// Upgrade of failed release didn't work - warn and skip
				fmt.Printf("  ⚠ Warning: Failed to upgrade release (status: %s): %v\n", releaseStatus, err)
				fmt.Println("  ⚠ Manual intervention required. You may need to:")
				fmt.Println("    1. Check pod status: kubectl get pods -n", cfg.Namespace, "-l app.kubernetes.io/name=velero")
				fmt.Println("    2. If data loss is acceptable, uninstall manually: helm uninstall velero -n", cfg.Namespace)
				return fmt.Errorf("failed to upgrade velero (manual intervention required): %w", err)
			}
			return fmt.Errorf("failed to upgrade velero: %w", err)
		}
	} else {
		// Install Velero via Helm
		if err := helmClient.Install(ctx, helm.InstallOptions{
			ReleaseName:     releaseName,
			Namespace:       cfg.Namespace,
			Chart:           veleroChart,
			Version:         cfg.Version,
			Values:          values,
			CreateNamespace: true,
			Wait:            true,
			Timeout:         10 * time.Minute,
		}); err != nil {
			return fmt.Errorf("failed to install velero: %w", err)
		}
	}

	// Verify installation
	if k8sClient != nil {
		if err := verifyInstallation(ctx, k8sClient, cfg.Namespace); err != nil {
			return fmt.Errorf("installation verification failed: %w", err)
		}
	}

	fmt.Println("  Velero installed successfully")
	fmt.Printf("  Velero endpoint: %s\n", cfg.GetVeleroEndpoint())
	if cfg.ScheduleCron != "" {
		fmt.Printf("  Backup schedule: %s (%s)\n", cfg.ScheduleName, cfg.ScheduleCron)
	}
	return nil
}

// buildHelmValues constructs Helm values for Velero installation
func buildHelmValues(cfg *Config) map[string]interface{} {
	values := make(map[string]interface{})

	// Start with user-provided values
	for k, v := range cfg.Values {
		values[k] = v
	}

	// Initialize provider plugins based on provider type
	// The AWS plugin is used for all S3-compatible storage (including Garage)
	values["initContainers"] = []map[string]interface{}{
		{
			"name":            "velero-plugin-for-aws",
			"image":           "velero/velero-plugin-for-aws:v1.10.0",
			"imagePullPolicy": "IfNotPresent",
			"volumeMounts": []map[string]interface{}{
				{
					"name":      "plugins",
					"mountPath": "/target",
				},
			},
		},
	}

	// Build credentials secret data
	credentialsData := buildCredentialsData(cfg)

	// Configuration settings
	configuration := map[string]interface{}{
		"backupStorageLocation": []map[string]interface{}{
			buildBackupStorageLocation(cfg),
		},
		"defaultBackupStorageLocation": cfg.GetBackupStorageLocationName(),
	}

	// Add volume snapshot location if snapshots are enabled
	if cfg.SnapshotsEnabled {
		configuration["volumeSnapshotLocation"] = []map[string]interface{}{
			buildVolumeSnapshotLocation(cfg),
		}
		configuration["defaultVolumeSnapshotLocations"] = map[string]string{
			"aws": cfg.GetBackupStorageLocationName(),
		}
	} else {
		// Explicitly disable volume snapshot locations when snapshots are not enabled
		configuration["volumeSnapshotLocation"] = []map[string]interface{}{}
	}

	values["configuration"] = configuration

	// Credentials secret
	values["credentials"] = map[string]interface{}{
		"useSecret":      true,
		"secretContents": credentialsData,
	}

	// Resource configuration
	if len(cfg.ResourceRequests) > 0 || len(cfg.ResourceLimits) > 0 {
		resources := map[string]interface{}{}
		if len(cfg.ResourceRequests) > 0 {
			resources["requests"] = cfg.ResourceRequests
		}
		if len(cfg.ResourceLimits) > 0 {
			resources["limits"] = cfg.ResourceLimits
		}
		values["resources"] = resources
	}

	// Metrics configuration for Prometheus
	values["metrics"] = map[string]interface{}{
		"enabled": true,
		"serviceMonitor": map[string]interface{}{
			"enabled": true,
		},
	}

	// Deployment configuration
	values["deployNodeAgent"] = false // Disabled by default, enable for file-level backups

	// Override kubectl image for upgrade jobs (chart default uses version that may not exist)
	values["kubectl"] = map[string]interface{}{
		"image": map[string]interface{}{
			"repository": "docker.io/bitnami/kubectl",
			"tag":        "1.31",
		},
	}
	// Disable upgrade jobs since we don't need them for fresh installs
	values["upgradeCRDs"] = false

	// Snapshot configuration
	if cfg.SnapshotsEnabled {
		values["snapshotsEnabled"] = true
		values["features"] = "EnableCSI"
	}

	// Add schedule if configured
	if cfg.ScheduleCron != "" {
		values["schedules"] = buildSchedules(cfg)
	}

	return values
}

// buildBackupStorageLocation builds the backup storage location configuration
func buildBackupStorageLocation(cfg *Config) map[string]interface{} {
	bsl := map[string]interface{}{
		"name":     cfg.GetBackupStorageLocationName(),
		"provider": "aws", // AWS provider is used for all S3-compatible storage
		"bucket":   cfg.S3Bucket,
		"default":  cfg.DefaultBackupStorageLocation,
	}

	// Build S3 config
	s3Config := map[string]interface{}{
		"region": cfg.S3Region,
	}

	// S3-compatible storage configuration (Garage, MinIO, etc.)
	if cfg.Provider == ProviderS3 {
		s3Config["s3ForcePathStyle"] = cfg.S3ForcePathStyle
		s3Config["s3Url"] = cfg.S3Endpoint
		if cfg.S3InsecureSkipTLSVerify {
			s3Config["insecureSkipTLSVerify"] = "true"
		}
	}

	bsl["config"] = s3Config

	return bsl
}

// buildVolumeSnapshotLocation builds the volume snapshot location configuration
func buildVolumeSnapshotLocation(cfg *Config) map[string]interface{} {
	return map[string]interface{}{
		"name":     cfg.GetBackupStorageLocationName(),
		"provider": "aws",
		"config": map[string]interface{}{
			"region": cfg.S3Region,
		},
	}
}

// buildCredentialsData builds the credentials secret data for S3 access
func buildCredentialsData(cfg *Config) map[string]interface{} {
	// AWS credentials file format
	credentialsContent := fmt.Sprintf(`[default]
aws_access_key_id=%s
aws_secret_access_key=%s
`, cfg.S3AccessKey, cfg.S3SecretKey)

	return map[string]interface{}{
		"cloud": credentialsContent,
	}
}

// buildSchedules builds the backup schedules configuration
func buildSchedules(cfg *Config) map[string]interface{} {
	schedule := map[string]interface{}{
		"schedule": cfg.ScheduleCron,
	}

	// Template for the backup
	template := map[string]interface{}{}

	// TTL for backups
	if cfg.BackupRetentionDays > 0 {
		template["ttl"] = fmt.Sprintf("%dh", cfg.BackupRetentionDays*24)
	}

	// Included namespaces (empty means all)
	if len(cfg.ScheduleIncludedNamespaces) > 0 {
		template["includedNamespaces"] = cfg.ScheduleIncludedNamespaces
	}

	// Excluded namespaces
	if len(cfg.ScheduleExcludedNamespaces) > 0 {
		template["excludedNamespaces"] = cfg.ScheduleExcludedNamespaces
	}

	// Storage location
	template["storageLocation"] = cfg.GetBackupStorageLocationName()

	// Include cluster resources
	template["includeClusterResources"] = true

	schedule["template"] = template

	return map[string]interface{}{
		cfg.ScheduleName: schedule,
	}
}

// verifyInstallation verifies that Velero pods are running
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
			return fmt.Errorf("timeout waiting for velero pods to be ready")
		case <-ticker.C:
			pods, err := k8sClient.GetPods(ctx, namespace)
			if err != nil {
				continue // Retry on error
			}

			if len(pods) == 0 {
				continue // Wait for pods to appear
			}

			// Check if velero pod is running
			veleroFound := false
			for _, pod := range pods {
				if pod.Name == "" {
					continue
				}
				// Look for velero pod
				if containsSubstring(pod.Name, "velero") {
					veleroFound = true
					if pod.Status != "Running" {
						break
					}
				}
			}

			if veleroFound {
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
