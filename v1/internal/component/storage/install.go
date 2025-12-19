package storage

import (
	"context"
	"fmt"
	"time"

	"github.com/catalystcommunity/foundry/v1/internal/helm"
)

const (
	// Local path provisioner (cowboysysop helm chart)
	localPathRepoName = "cowboysysop"
	localPathRepoURL  = "https://cowboysysop.github.io/charts"
	localPathChart    = "cowboysysop/local-path-provisioner"

	// NFS subdir external provisioner
	nfsRepoName = "nfs-subdir-external-provisioner"
	nfsRepoURL  = "https://kubernetes-sigs.github.io/nfs-subdir-external-provisioner"
	nfsChart    = "nfs-subdir-external-provisioner/nfs-subdir-external-provisioner"

	// Longhorn distributed block storage
	longhornRepoName = "longhorn"
	longhornRepoURL  = "https://charts.longhorn.io"
	longhornChart    = "longhorn/longhorn"
)

// Install installs the storage provisioner based on the configured backend
func Install(ctx context.Context, helmClient HelmClient, k8sClient K8sClient, cfg *Config) error {
	if helmClient == nil {
		return fmt.Errorf("helm client cannot be nil")
	}
	if cfg == nil {
		cfg = DefaultConfig()
	}

	switch cfg.Backend {
	case BackendLocalPath:
		return installLocalPath(ctx, helmClient, k8sClient, cfg)
	case BackendNFS:
		return installNFS(ctx, helmClient, k8sClient, cfg)
	case BackendLonghorn:
		return installLonghorn(ctx, helmClient, k8sClient, cfg)
	default:
		return fmt.Errorf("unsupported storage backend: %s", cfg.Backend)
	}
}

// installLocalPath installs Rancher's local-path-provisioner
func installLocalPath(ctx context.Context, helmClient HelmClient, k8sClient K8sClient, cfg *Config) error {
	fmt.Println("  Installing local-path-provisioner...")

	// Check if local-path provisioner is already installed via K3s (built-in)
	// K3s includes local-path-provisioner by default, so check for existing release
	releases, err := helmClient.List(ctx, "kube-system")
	if err == nil {
		// If no helm release exists, K3s's built-in is likely being used
		foundHelmRelease := false
		for _, rel := range releases {
			if rel.Name == "local-path-provisioner" {
				foundHelmRelease = true
				break
			}
		}
		if !foundHelmRelease {
			// Also check the target namespace
			releases, _ = helmClient.List(ctx, cfg.Namespace)
			for _, rel := range releases {
				if rel.Name == "local-path-provisioner" {
					foundHelmRelease = true
					break
				}
			}
		}
		if !foundHelmRelease {
			// K3s likely has built-in local-path-provisioner
			fmt.Println("  local-path-provisioner already available (K3s built-in)")
			return nil
		}
	}

	// Add Helm repository
	if err := helmClient.AddRepo(ctx, helm.RepoAddOptions{
		Name:        localPathRepoName,
		URL:         localPathRepoURL,
		ForceUpdate: true,
	}); err != nil {
		return fmt.Errorf("failed to add helm repository: %w", err)
	}

	// Build values
	values := buildLocalPathValues(cfg)

	// Check if helm release already exists (for upgrades when we installed via helm)
	var releaseExists bool
	var releaseStatus string
	releases, err = helmClient.List(ctx, cfg.Namespace)
	if err == nil {
		for _, rel := range releases {
			if rel.Name == "local-path-provisioner" {
				releaseExists = true
				releaseStatus = rel.Status
				break
			}
		}
	}

	if releaseExists {
		// Try to upgrade existing release (even if failed - avoid data loss)
		fmt.Printf("  Upgrading local-path-provisioner (current status: %s)...\n", releaseStatus)
		if err := helmClient.Upgrade(ctx, helm.UpgradeOptions{
			ReleaseName: "local-path-provisioner",
			Namespace:   cfg.Namespace,
			Chart:       localPathChart,
			Version:     cfg.Version,
			Values:      values,
			Wait:        true,
			Timeout:     5 * time.Minute,
		}); err != nil {
			if releaseStatus != "deployed" {
				// Upgrade of failed release didn't work - warn and skip
				fmt.Printf("  ⚠ Warning: Failed to upgrade release (status: %s): %v\n", releaseStatus, err)
				fmt.Println("  ⚠ Manual intervention required. You may need to:")
				fmt.Println("    1. Check pod status: kubectl get pods -n", cfg.Namespace, "-l app.kubernetes.io/name=local-path-provisioner")
				fmt.Println("    2. If data loss is acceptable, uninstall manually: helm uninstall local-path-provisioner -n", cfg.Namespace)
				return fmt.Errorf("failed to upgrade local-path-provisioner (manual intervention required): %w", err)
			}
			return fmt.Errorf("failed to upgrade local-path-provisioner: %w", err)
		}
	} else {
		// Install via Helm
		if err := helmClient.Install(ctx, helm.InstallOptions{
			ReleaseName:     "local-path-provisioner",
			Namespace:       cfg.Namespace,
			Chart:           localPathChart,
			Version:         cfg.Version,
			Values:          values,
			CreateNamespace: true,
			Wait:            true,
			Timeout:         5 * time.Minute,
		}); err != nil {
			return fmt.Errorf("failed to install local-path-provisioner: %w", err)
		}
	}

	fmt.Println("  local-path-provisioner installed successfully")
	return nil
}

// buildLocalPathValues constructs Helm values for local-path-provisioner
func buildLocalPathValues(cfg *Config) map[string]interface{} {
	values := make(map[string]interface{})

	// Start with user-provided values
	for k, v := range cfg.Values {
		values[k] = v
	}

	// Storage class configuration
	storageClass := map[string]interface{}{
		"create":       true,
		"name":         cfg.StorageClassName,
		"defaultClass": cfg.SetDefault,
	}

	if cfg.LocalPath != nil && cfg.LocalPath.ReclaimPolicy != "" {
		storageClass["reclaimPolicy"] = cfg.LocalPath.ReclaimPolicy
	}

	values["storageClass"] = storageClass

	// Node path configuration
	if cfg.LocalPath != nil && cfg.LocalPath.Path != "" {
		values["nodePathMap"] = []map[string]interface{}{
			{
				"node":  "DEFAULT_PATH_FOR_NON_LISTED_NODES",
				"paths": []string{cfg.LocalPath.Path},
			},
		}
	}

	return values
}

// installNFS installs nfs-subdir-external-provisioner
func installNFS(ctx context.Context, helmClient HelmClient, k8sClient K8sClient, cfg *Config) error {
	fmt.Println("  Installing nfs-subdir-external-provisioner...")

	// Add Helm repository
	if err := helmClient.AddRepo(ctx, helm.RepoAddOptions{
		Name:        nfsRepoName,
		URL:         nfsRepoURL,
		ForceUpdate: true,
	}); err != nil {
		return fmt.Errorf("failed to add helm repository: %w", err)
	}

	// Build values
	values := buildNFSValues(cfg)

	// Set default version for NFS provisioner
	version := cfg.Version
	if version == "" || version == "0.0.28" { // default local-path version
		version = "4.0.18" // nfs-subdir-external-provisioner chart version
	}

	// Check if release already exists
	var releaseExists bool
	var releaseStatus string
	releases, err := helmClient.List(ctx, cfg.Namespace)
	if err == nil {
		for _, rel := range releases {
			if rel.Name == "nfs-subdir-external-provisioner" {
				releaseExists = true
				releaseStatus = rel.Status
				break
			}
		}
	}

	if releaseExists {
		// Try to upgrade existing release (even if failed - avoid data loss)
		fmt.Printf("  Upgrading nfs-subdir-external-provisioner (current status: %s)...\n", releaseStatus)
		if err := helmClient.Upgrade(ctx, helm.UpgradeOptions{
			ReleaseName: "nfs-subdir-external-provisioner",
			Namespace:   cfg.Namespace,
			Chart:       nfsChart,
			Version:     version,
			Values:      values,
			Wait:        true,
			Timeout:     5 * time.Minute,
		}); err != nil {
			if releaseStatus != "deployed" {
				// Upgrade of failed release didn't work - warn and skip
				fmt.Printf("  ⚠ Warning: Failed to upgrade release (status: %s): %v\n", releaseStatus, err)
				fmt.Println("  ⚠ Manual intervention required. You may need to:")
				fmt.Println("    1. Check pod status: kubectl get pods -n", cfg.Namespace, "-l app.kubernetes.io/name=nfs-subdir-external-provisioner")
				fmt.Println("    2. If data loss is acceptable, uninstall manually: helm uninstall nfs-subdir-external-provisioner -n", cfg.Namespace)
				return fmt.Errorf("failed to upgrade nfs-subdir-external-provisioner (manual intervention required): %w", err)
			}
			return fmt.Errorf("failed to upgrade nfs-subdir-external-provisioner: %w", err)
		}
	} else {
		// Install via Helm
		if err := helmClient.Install(ctx, helm.InstallOptions{
			ReleaseName:     "nfs-subdir-external-provisioner",
			Namespace:       cfg.Namespace,
			Chart:           nfsChart,
			Version:         version,
			Values:          values,
			CreateNamespace: true,
			Wait:            true,
			Timeout:         5 * time.Minute,
		}); err != nil {
			return fmt.Errorf("failed to install nfs-subdir-external-provisioner: %w", err)
		}
	}

	fmt.Println("  nfs-subdir-external-provisioner installed successfully")
	return nil
}

// buildNFSValues constructs Helm values for nfs-subdir-external-provisioner
func buildNFSValues(cfg *Config) map[string]interface{} {
	values := make(map[string]interface{})

	// Start with user-provided values
	for k, v := range cfg.Values {
		values[k] = v
	}

	// NFS server configuration
	values["nfs"] = map[string]interface{}{
		"server": cfg.NFS.Server,
		"path":   cfg.NFS.Path,
	}

	// Storage class configuration
	storageClass := map[string]interface{}{
		"create":           true,
		"name":             cfg.StorageClassName,
		"defaultClass":     cfg.SetDefault,
		"archiveOnDelete":  cfg.NFS.ArchiveOnDelete,
		"accessModes":      "ReadWriteOnce",
		"allowVolumeExpansion": true,
	}

	if cfg.NFS.ReclaimPolicy != "" {
		storageClass["reclaimPolicy"] = cfg.NFS.ReclaimPolicy
	} else {
		storageClass["reclaimPolicy"] = "Delete"
	}

	values["storageClass"] = storageClass

	return values
}

// installLonghorn installs Longhorn distributed block storage
func installLonghorn(ctx context.Context, helmClient HelmClient, k8sClient K8sClient, cfg *Config) error {
	fmt.Println("  Installing Longhorn...")

	// Check for ServiceMonitor CRD if ServiceMonitor is enabled
	// If CRD doesn't exist, automatically disable ServiceMonitor for initial install
	// It will be enabled when storage is upgraded after Prometheus is installed
	serviceMonitorAvailable := false
	if cfg.Longhorn != nil && cfg.Longhorn.ServiceMonitorEnabled {
		if k8sClient != nil {
			crdExists, err := k8sClient.ServiceMonitorCRDExists(ctx)
			if err != nil {
				fmt.Printf("  ⚠ Could not check for ServiceMonitor CRD: %v\n", err)
			} else if !crdExists {
				fmt.Println("  ⚠ ServiceMonitor CRD not available - installing without metrics integration")
				fmt.Println("    (ServiceMonitor will be enabled when storage is upgraded after Prometheus)")
				// Temporarily disable for this install
				cfg.Longhorn.ServiceMonitorEnabled = false
			} else {
				serviceMonitorAvailable = true
			}
		}
	}
	_ = serviceMonitorAvailable // Will be used for logging

	// Add Helm repository
	if err := helmClient.AddRepo(ctx, helm.RepoAddOptions{
		Name:        longhornRepoName,
		URL:         longhornRepoURL,
		ForceUpdate: true,
	}); err != nil {
		return fmt.Errorf("failed to add helm repository: %w", err)
	}

	// Build values
	values := buildLonghornValues(cfg)

	// Set default version for Longhorn
	version := cfg.Version
	if version == "" || version == "0.0.28" { // default local-path version
		version = "1.7.2" // Longhorn chart version
	}

	// Use longhorn-system namespace
	namespace := cfg.Namespace
	if namespace == "kube-system" {
		namespace = "longhorn-system"
	}

	// Check if release already exists
	var releaseExists bool
	var releaseStatus string
	releases, err := helmClient.List(ctx, namespace)
	if err == nil {
		for _, rel := range releases {
			if rel.Name == "longhorn" {
				releaseExists = true
				releaseStatus = rel.Status
				break
			}
		}
	}

	if releaseExists {
		// Try to upgrade existing release (even if failed - avoid data loss)
		fmt.Printf("  Upgrading Longhorn (current status: %s)...\n", releaseStatus)
		if err := helmClient.Upgrade(ctx, helm.UpgradeOptions{
			ReleaseName: "longhorn",
			Namespace:   namespace,
			Chart:       longhornChart,
			Version:     version,
			Values:      values,
			Wait:        true,
			Timeout:     10 * time.Minute,
		}); err != nil {
			if releaseStatus != "deployed" {
				// Upgrade of failed release didn't work - warn and skip
				fmt.Printf("  ⚠ Warning: Failed to upgrade release (status: %s): %v\n", releaseStatus, err)
				fmt.Println("  ⚠ Manual intervention required. You may need to:")
				fmt.Println("    1. Check pod status: kubectl get pods -n", namespace, "-l app.kubernetes.io/name=longhorn")
				fmt.Println("    2. Check PVC status: kubectl get pvc -n", namespace)
				fmt.Println("    3. If data loss is acceptable, uninstall manually: helm uninstall longhorn -n", namespace)
				return fmt.Errorf("failed to upgrade longhorn (manual intervention required): %w", err)
			}
			return fmt.Errorf("failed to upgrade longhorn: %w", err)
		}
	} else {
		// Install via Helm
		if err := helmClient.Install(ctx, helm.InstallOptions{
			ReleaseName:     "longhorn",
			Namespace:       namespace,
			Chart:           longhornChart,
			Version:         version,
			Values:          values,
			CreateNamespace: true,
			Wait:            true,
			Timeout:         10 * time.Minute,
		}); err != nil {
			return fmt.Errorf("failed to install longhorn: %w", err)
		}
	}

	fmt.Println("  Longhorn installed successfully")
	return nil
}

// buildLonghornValues constructs Helm values for Longhorn
func buildLonghornValues(cfg *Config) map[string]interface{} {
	values := make(map[string]interface{})

	// Start with user-provided values
	for k, v := range cfg.Values {
		values[k] = v
	}

	// Default settings
	defaultSettings := map[string]interface{}{
		"defaultReplicaCount": cfg.Longhorn.ReplicaCount,
	}

	if cfg.Longhorn.DataPath != "" {
		defaultSettings["defaultDataPath"] = cfg.Longhorn.DataPath
	}

	if cfg.Longhorn.GuaranteedInstanceManagerCPU > 0 {
		defaultSettings["guaranteedInstanceManagerCPU"] = cfg.Longhorn.GuaranteedInstanceManagerCPU
	}

	if cfg.Longhorn.DefaultDataLocality != "" {
		defaultSettings["defaultDataLocality"] = cfg.Longhorn.DefaultDataLocality
	}

	values["defaultSettings"] = defaultSettings

	// Persistence settings for StorageClass
	persistence := map[string]interface{}{
		"defaultClass":             cfg.SetDefault,
		"defaultClassReplicaCount": cfg.Longhorn.ReplicaCount,
		"reclaimPolicy":            "Delete",
	}
	values["persistence"] = persistence

	// Ingress configuration
	if cfg.Longhorn != nil && cfg.Longhorn.IngressEnabled && cfg.Longhorn.IngressHost != "" {
		values["ingress"] = map[string]interface{}{
			"enabled":          true,
			"ingressClassName": "contour",
			"host":             cfg.Longhorn.IngressHost,
			"tls":              true,
			"tlsSecret":        "longhorn-tls",
			"annotations": map[string]interface{}{
				"cert-manager.io/cluster-issuer": "foundry-ca-issuer",
			},
		}
	} else {
		values["ingress"] = map[string]interface{}{
			"enabled": false,
		}
	}

	// Enable metrics, and ServiceMonitor if configured (requires CRD from Prometheus Operator)
	metricsConfig := map[string]interface{}{}
	if cfg.Longhorn != nil && cfg.Longhorn.ServiceMonitorEnabled {
		metricsConfig["serviceMonitor"] = map[string]interface{}{
			"enabled": true,
		}
	}
	values["metrics"] = metricsConfig

	return values
}
