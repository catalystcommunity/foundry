package storage

import (
	"context"
	"fmt"
	"time"

	"github.com/catalystcommunity/foundry/v1/internal/helm"
)

const (
	// Local path provisioner
	localPathRepoName = "local-path-provisioner"
	localPathRepoURL  = "https://charts.k8s.home/local-path-provisioner"
	localPathChart    = "local-path-provisioner/local-path-provisioner"

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

	// Check if release already exists
	releases, err := helmClient.List(ctx, cfg.Namespace)
	if err == nil {
		for _, rel := range releases {
			if rel.Name == "local-path-provisioner" {
				if rel.Status == "deployed" {
					fmt.Println("  local-path-provisioner already installed")
					return nil
				}
				// Uninstall failed release
				fmt.Printf("  Removing failed release (status: %s)...\n", rel.Status)
				if err := helmClient.Uninstall(ctx, helm.UninstallOptions{
					ReleaseName: "local-path-provisioner",
					Namespace:   cfg.Namespace,
				}); err != nil {
					return fmt.Errorf("failed to uninstall existing release: %w", err)
				}
				break
			}
		}
	}

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
	releases, err := helmClient.List(ctx, cfg.Namespace)
	if err == nil {
		for _, rel := range releases {
			if rel.Name == "nfs-subdir-external-provisioner" {
				if rel.Status == "deployed" {
					fmt.Println("  nfs-subdir-external-provisioner already installed")
					return nil
				}
				// Uninstall failed release
				fmt.Printf("  Removing failed release (status: %s)...\n", rel.Status)
				if err := helmClient.Uninstall(ctx, helm.UninstallOptions{
					ReleaseName: "nfs-subdir-external-provisioner",
					Namespace:   cfg.Namespace,
				}); err != nil {
					return fmt.Errorf("failed to uninstall existing release: %w", err)
				}
				break
			}
		}
	}

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
	releases, err := helmClient.List(ctx, namespace)
	if err == nil {
		for _, rel := range releases {
			if rel.Name == "longhorn" {
				if rel.Status == "deployed" {
					fmt.Println("  Longhorn already installed")
					return nil
				}
				// Uninstall failed release
				fmt.Printf("  Removing failed release (status: %s)...\n", rel.Status)
				if err := helmClient.Uninstall(ctx, helm.UninstallOptions{
					ReleaseName: "longhorn",
					Namespace:   namespace,
				}); err != nil {
					return fmt.Errorf("failed to uninstall existing release: %w", err)
				}
				break
			}
		}
	}

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

	// Ingress disabled by default
	values["ingress"] = map[string]interface{}{
		"enabled": false,
	}

	return values
}
