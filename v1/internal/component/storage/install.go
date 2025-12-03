package storage

import (
	"context"
	"fmt"
	"time"

	"github.com/catalystcommunity/foundry/v1/internal/helm"
	"github.com/catalystcommunity/foundry/v1/internal/storage/truenas"
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

	// Democratic CSI (TrueNAS)
	democraticCSIRepoName = "democratic-csi"
	democraticCSIRepoURL  = "https://democratic-csi.github.io/charts"
	democraticCSIChart    = "democratic-csi/democratic-csi"
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
	case BackendTrueNASNFS:
		return installTrueNASNFS(ctx, helmClient, k8sClient, cfg)
	case BackendTrueNASiSCSI:
		return installTrueNASiSCSI(ctx, helmClient, k8sClient, cfg)
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

// installTrueNASNFS installs democratic-csi with freenas-nfs driver
func installTrueNASNFS(ctx context.Context, helmClient HelmClient, k8sClient K8sClient, cfg *Config) error {
	fmt.Println("  Installing democratic-csi (TrueNAS NFS)...")

	// Add Helm repository
	if err := helmClient.AddRepo(ctx, helm.RepoAddOptions{
		Name:        democraticCSIRepoName,
		URL:         democraticCSIRepoURL,
		ForceUpdate: true,
	}); err != nil {
		return fmt.Errorf("failed to add helm repository: %w", err)
	}

	// Build values
	values := buildTrueNASNFSValues(cfg)

	// Set default version for democratic-csi
	version := cfg.Version
	if version == "" || version == "0.0.28" { // default local-path version
		version = "0.14.6" // democratic-csi chart version
	}

	// Use democratic-csi namespace
	namespace := cfg.Namespace
	if namespace == "kube-system" {
		namespace = "democratic-csi"
	}

	// Check if release already exists
	releases, err := helmClient.List(ctx, namespace)
	if err == nil {
		for _, rel := range releases {
			if rel.Name == "democratic-csi-nfs" {
				if rel.Status == "deployed" {
					fmt.Println("  democratic-csi (NFS) already installed")
					return nil
				}
				// Uninstall failed release
				fmt.Printf("  Removing failed release (status: %s)...\n", rel.Status)
				if err := helmClient.Uninstall(ctx, helm.UninstallOptions{
					ReleaseName: "democratic-csi-nfs",
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
		ReleaseName:     "democratic-csi-nfs",
		Namespace:       namespace,
		Chart:           democraticCSIChart,
		Version:         version,
		Values:          values,
		CreateNamespace: true,
		Wait:            true,
		Timeout:         10 * time.Minute,
	}); err != nil {
		return fmt.Errorf("failed to install democratic-csi: %w", err)
	}

	fmt.Println("  democratic-csi (TrueNAS NFS) installed successfully")
	return nil
}

// buildTrueNASNFSValues constructs Helm values for democratic-csi with freenas-nfs
func buildTrueNASNFSValues(cfg *Config) map[string]interface{} {
	values := make(map[string]interface{})

	// Start with user-provided values
	for k, v := range cfg.Values {
		values[k] = v
	}

	// CSI driver configuration
	values["csiDriver"] = map[string]interface{}{
		"name": "org.democratic-csi.nfs",
	}

	// Storage classes
	storageClasses := []map[string]interface{}{
		{
			"name":          cfg.StorageClassName,
			"defaultClass":  cfg.SetDefault,
			"reclaimPolicy": "Delete",
			"volumeBindingMode": "Immediate",
			"allowVolumeExpansion": true,
			"parameters": map[string]interface{}{
				"fsType": "nfs",
			},
		},
	}
	values["storageClasses"] = storageClasses

	// Volume snapshot classes (optional but useful)
	values["volumeSnapshotClasses"] = []map[string]interface{}{
		{
			"name": cfg.StorageClassName + "-snapclass",
			"parameters": map[string]interface{}{
				"detachedSnapshots": "true",
			},
		},
	}

	// Build share networks for NFS
	shareNetworks := cfg.TrueNAS.ShareNetworks
	if len(shareNetworks) == 0 {
		shareNetworks = []string{"0.0.0.0/0"} // Allow all by default (should be restricted in production)
	}

	// Driver configuration
	driverConfig := map[string]interface{}{
		"driver": "freenas-nfs",
		"httpConnection": map[string]interface{}{
			"protocol": "https",
			"host":     cfg.TrueNAS.APIURL,
			"port":     443,
			"apiKey":   cfg.TrueNAS.APIKey,
			"allowInsecure": true, // For self-signed certs
		},
		"zfs": map[string]interface{}{
			"datasetParentName":       cfg.TrueNAS.DatasetParent,
			"detachedSnapshotsDatasetParentName": cfg.TrueNAS.DatasetParent + "/snaps",
			"datasetEnableQuotas":     true,
			"datasetEnableReservation": false,
			"datasetPermissionsMode":  "0777",
			"datasetPermissionsUser":  0,
			"datasetPermissionsGroup": 0,
		},
		"nfs": map[string]interface{}{
			"shareHost":               cfg.TrueNAS.ShareHost,
			"shareAlldirs":            false,
			"shareAllowedHosts":       []string{},
			"shareAllowedNetworks":    shareNetworks,
			"shareMaprootUser":        "root",
			"shareMaprootGroup":       "wheel",
		},
	}
	values["driver"] = map[string]interface{}{
		"config": driverConfig,
	}

	return values
}

// installTrueNASiSCSI installs democratic-csi with freenas-iscsi driver
func installTrueNASiSCSI(ctx context.Context, helmClient HelmClient, k8sClient K8sClient, cfg *Config) error {
	fmt.Println("  Installing democratic-csi (TrueNAS iSCSI)...")

	// Add Helm repository
	if err := helmClient.AddRepo(ctx, helm.RepoAddOptions{
		Name:        democraticCSIRepoName,
		URL:         democraticCSIRepoURL,
		ForceUpdate: true,
	}); err != nil {
		return fmt.Errorf("failed to add helm repository: %w", err)
	}

	// Build values
	values := buildTrueNASiSCSIValues(cfg)

	// Set default version for democratic-csi
	version := cfg.Version
	if version == "" || version == "0.0.28" { // default local-path version
		version = "0.14.6" // democratic-csi chart version
	}

	// Use democratic-csi namespace
	namespace := cfg.Namespace
	if namespace == "kube-system" {
		namespace = "democratic-csi"
	}

	// Check if release already exists
	releases, err := helmClient.List(ctx, namespace)
	if err == nil {
		for _, rel := range releases {
			if rel.Name == "democratic-csi-iscsi" {
				if rel.Status == "deployed" {
					fmt.Println("  democratic-csi (iSCSI) already installed")
					return nil
				}
				// Uninstall failed release
				fmt.Printf("  Removing failed release (status: %s)...\n", rel.Status)
				if err := helmClient.Uninstall(ctx, helm.UninstallOptions{
					ReleaseName: "democratic-csi-iscsi",
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
		ReleaseName:     "democratic-csi-iscsi",
		Namespace:       namespace,
		Chart:           democraticCSIChart,
		Version:         version,
		Values:          values,
		CreateNamespace: true,
		Wait:            true,
		Timeout:         10 * time.Minute,
	}); err != nil {
		return fmt.Errorf("failed to install democratic-csi: %w", err)
	}

	fmt.Println("  democratic-csi (TrueNAS iSCSI) installed successfully")
	return nil
}

// buildTrueNASiSCSIValues constructs Helm values for democratic-csi with freenas-iscsi
func buildTrueNASiSCSIValues(cfg *Config) map[string]interface{} {
	values := make(map[string]interface{})

	// Start with user-provided values
	for k, v := range cfg.Values {
		values[k] = v
	}

	// CSI driver configuration
	values["csiDriver"] = map[string]interface{}{
		"name": "org.democratic-csi.iscsi",
	}

	// Storage classes
	storageClasses := []map[string]interface{}{
		{
			"name":          cfg.StorageClassName,
			"defaultClass":  cfg.SetDefault,
			"reclaimPolicy": "Delete",
			"volumeBindingMode": "Immediate",
			"allowVolumeExpansion": true,
			"parameters": map[string]interface{}{
				"fsType": "ext4",
			},
		},
	}
	values["storageClasses"] = storageClasses

	// Volume snapshot classes
	values["volumeSnapshotClasses"] = []map[string]interface{}{
		{
			"name": cfg.StorageClassName + "-snapclass",
			"parameters": map[string]interface{}{
				"detachedSnapshots": "true",
			},
		},
	}

	// Driver configuration
	driverConfig := map[string]interface{}{
		"driver": "freenas-iscsi",
		"httpConnection": map[string]interface{}{
			"protocol": "https",
			"host":     cfg.TrueNAS.APIURL,
			"port":     443,
			"apiKey":   cfg.TrueNAS.APIKey,
			"allowInsecure": true,
		},
		"zfs": map[string]interface{}{
			"datasetParentName":       cfg.TrueNAS.DatasetParent,
			"detachedSnapshotsDatasetParentName": cfg.TrueNAS.DatasetParent + "/snaps",
			"zvolCompression":         "",
			"zvolDedup":               "",
			"zvolEnableReservation":   false,
			"zvolBlocksize":           "",
		},
		"iscsi": map[string]interface{}{
			"targetPortal":       cfg.TrueNAS.APIURL + ":3260",
			"targetPortals":      []string{},
			"interface":          "",
			"namePrefix":         "csi-",
			"nameSuffix":         "",
			"targetGroups": []map[string]interface{}{
				{
					"targetGroupPortalGroup": cfg.TrueNAS.PortalID,
					"targetGroupInitiatorGroup": cfg.TrueNAS.InitiatorGroupID,
					"targetGroupAuthType":    "None",
					"targetGroupAuthGroup":   nil,
				},
			},
			"extentInsecureTpc":       true,
			"extentXenCompat":         false,
			"extentDisablePhysicalBlocksize": true,
			"extentBlocksize":         512,
			"extentRpm":               "SSD",
			"extentAvailThreshold":    0,
		},
	}
	values["driver"] = map[string]interface{}{
		"config": driverConfig,
	}

	// Node configuration for iSCSI
	values["node"] = map[string]interface{}{
		"hostPID": true,
		"driver": map[string]interface{}{
			"extraEnv": []map[string]interface{}{
				{
					"name":  "ISCSIADM_HOST_STRATEGY",
					"value": "nsenter",
				},
			},
		},
	}

	return values
}

// OpenBAOClient defines the interface for storing secrets in OpenBAO
type OpenBAOClient interface {
	WriteSecretV2(ctx context.Context, mount, path string, data map[string]interface{}) error
	ReadSecretV2(ctx context.Context, mount, path string) (map[string]interface{}, error)
}

// TrueNASSetupOptions contains options for TrueNAS setup integration
type TrueNASSetupOptions struct {
	// APIURL is the TrueNAS API URL (if not in config)
	APIURL string
	// APIKey is the TrueNAS API key (if not in config)
	APIKey string
	// Interactive enables prompting for missing config
	Interactive bool
	// SkipSetup skips TrueNAS setup (assumes already configured)
	SkipSetup bool
	// OpenBAOClient for storing API key
	OpenBAOClient OpenBAOClient
}

// PrepareTrueNASInstall prepares TrueNAS for CSI installation
// It validates config, runs setup if needed, and optionally stores API key in OpenBAO
// Returns an updated Config with TrueNAS settings populated
func PrepareTrueNASInstall(ctx context.Context, cfg *Config, opts *TrueNASSetupOptions) (*Config, error) {
	if opts == nil {
		opts = &TrueNASSetupOptions{}
	}

	// Ensure TrueNAS config exists
	if cfg.TrueNAS == nil {
		cfg.TrueNAS = &TrueNASCSIConfig{}
	}

	// Use provided values or fall back to config
	apiURL := opts.APIURL
	if apiURL == "" {
		apiURL = cfg.TrueNAS.APIURL
	}

	apiKey := opts.APIKey
	if apiKey == "" {
		apiKey = cfg.TrueNAS.APIKey
	}

	// Build install config for the truenas integration module
	installCfg := &truenas.InstallConfig{
		APIURL:      apiURL,
		APIKey:      apiKey,
		Interactive: opts.Interactive,
		SkipSetup:   opts.SkipSetup,
		SetupConfig: &truenas.SetupConfig{
			PoolName:    cfg.TrueNAS.Pool,
			DatasetName: "k8s", // Default dataset name for CSI
			EnableNFS:   cfg.Backend == BackendTrueNASNFS,
			EnableISCSI: cfg.Backend == BackendTrueNASiSCSI,
		},
	}

	// Use defaults if pool not specified
	if installCfg.SetupConfig.PoolName == "" {
		installCfg.SetupConfig.PoolName = "tank"
	}

	// Run TrueNAS preparation
	result, err := truenas.PrepareInstall(ctx, installCfg, opts.OpenBAOClient)
	if err != nil {
		return nil, fmt.Errorf("TrueNAS preparation failed: %w", err)
	}

	// Update config with CSI configuration from setup result
	if result.CSIConfig != nil {
		cfg.TrueNAS.APIURL = result.CSIConfig.HTTPURL
		cfg.TrueNAS.APIKey = result.CSIConfig.APIKey
		cfg.TrueNAS.Pool = result.CSIConfig.PoolName
		cfg.TrueNAS.DatasetParent = result.CSIConfig.DatasetParent
		cfg.TrueNAS.ShareHost = result.CSIConfig.NFSShareHost

		// Update iSCSI-specific config
		if cfg.Backend == BackendTrueNASiSCSI && result.CSIConfig.ISCSIPortal != "" {
			cfg.TrueNAS.PortalID = result.CSIConfig.ISCSITargetPortalGroup
			cfg.TrueNAS.InitiatorGroupID = result.CSIConfig.ISCSIInitiatorGroup
		}
	}

	// If API key was stored in OpenBAO, update config to use secret reference
	if result.APIKeyStored {
		cfg.TrueNAS.APIKey = "${secret:foundry-core/truenas:api_key}"
	}

	return cfg, nil
}

// EnsureTrueNASAPIKey ensures TrueNAS API key is available (from OpenBAO or config)
// This is a convenience function for retrieving or storing the API key
func EnsureTrueNASAPIKey(ctx context.Context, client OpenBAOClient, apiKey string) (string, error) {
	return truenas.EnsureTrueNASAPIKey(ctx, client, apiKey)
}

// GetTrueNASAPIKey retrieves the TrueNAS API key from OpenBAO
func GetTrueNASAPIKey(ctx context.Context, client OpenBAOClient) (string, error) {
	return truenas.GetTrueNASAPIKey(ctx, client)
}
