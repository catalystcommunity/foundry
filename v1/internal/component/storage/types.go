package storage

import (
	"context"
	"fmt"

	"github.com/catalystcommunity/foundry/v1/internal/component"
	"github.com/catalystcommunity/foundry/v1/internal/helm"
	"github.com/catalystcommunity/foundry/v1/internal/k8s"
)

// StorageBackend represents the type of storage backend to use
type StorageBackend string

const (
	// BackendLocalPath uses Rancher's local-path-provisioner for simple local storage
	BackendLocalPath StorageBackend = "local-path"

	// BackendNFS uses nfs-subdir-external-provisioner for any NFS server
	BackendNFS StorageBackend = "nfs"

	// BackendTrueNASNFS uses democratic-csi with freenas-nfs driver for TrueNAS NFS
	BackendTrueNASNFS StorageBackend = "truenas-nfs"

	// BackendTrueNASiSCSI uses democratic-csi with freenas-iscsi driver for TrueNAS iSCSI
	BackendTrueNASiSCSI StorageBackend = "truenas-iscsi"
)

// Config holds storage component configuration
type Config struct {
	// Backend specifies which storage provisioner to use
	Backend StorageBackend `json:"backend" yaml:"backend"`

	// Version is the Helm chart version to install
	Version string `json:"version" yaml:"version"`

	// Namespace for the storage provisioner
	Namespace string `json:"namespace" yaml:"namespace"`

	// StorageClassName is the name of the StorageClass to create
	StorageClassName string `json:"storage_class_name" yaml:"storage_class_name"`

	// SetDefault marks this StorageClass as the cluster default
	SetDefault bool `json:"set_default" yaml:"set_default"`

	// LocalPath configuration (for BackendLocalPath)
	LocalPath *LocalPathConfig `json:"local_path,omitempty" yaml:"local_path,omitempty"`

	// NFS configuration (for BackendNFS)
	NFS *NFSConfig `json:"nfs,omitempty" yaml:"nfs,omitempty"`

	// TrueNAS configuration (for BackendTrueNASNFS or BackendTrueNASiSCSI)
	TrueNAS *TrueNASCSIConfig `json:"truenas,omitempty" yaml:"truenas,omitempty"`

	// Values allows passing additional Helm values
	Values map[string]interface{} `json:"values" yaml:",inline"`
}

// LocalPathConfig holds configuration for local-path-provisioner
type LocalPathConfig struct {
	// Path is the base path on nodes for local storage (default: /opt/local-path-provisioner)
	Path string `json:"path" yaml:"path"`

	// ReclaimPolicy is the reclaim policy for PVs (default: Delete)
	ReclaimPolicy string `json:"reclaim_policy" yaml:"reclaim_policy"`
}

// NFSConfig holds configuration for nfs-subdir-external-provisioner
type NFSConfig struct {
	// Server is the NFS server hostname or IP
	Server string `json:"server" yaml:"server"`

	// Path is the base path on the NFS server
	Path string `json:"path" yaml:"path"`

	// ReclaimPolicy is the reclaim policy for PVs (default: Delete)
	ReclaimPolicy string `json:"reclaim_policy" yaml:"reclaim_policy"`

	// ArchiveOnDelete keeps data in archived-<pv-name> directory when PVC is deleted
	ArchiveOnDelete bool `json:"archive_on_delete" yaml:"archive_on_delete"`
}

// TrueNASCSIConfig holds configuration for democratic-csi with TrueNAS
type TrueNASCSIConfig struct {
	// APIURL is the TrueNAS API URL (e.g., https://truenas.example.com)
	APIURL string `json:"api_url" yaml:"api_url"`

	// APIKey is the TrueNAS API key (or secret reference)
	APIKey string `json:"api_key" yaml:"api_key"`

	// Pool is the ZFS pool to use for storage
	Pool string `json:"pool" yaml:"pool"`

	// DatasetParent is the parent dataset for PVC datasets (e.g., tank/k8s/volumes)
	DatasetParent string `json:"dataset_parent" yaml:"dataset_parent"`

	// ShareHost is the hostname/IP that K8s nodes will use to mount NFS shares
	// For NFS backend only
	ShareHost string `json:"share_host,omitempty" yaml:"share_host,omitempty"`

	// ShareNetworks are the networks allowed to access NFS shares (CIDR notation)
	// For NFS backend only
	ShareNetworks []string `json:"share_networks,omitempty" yaml:"share_networks,omitempty"`

	// PortalID is the iSCSI portal ID for iSCSI backend
	PortalID int `json:"portal_id,omitempty" yaml:"portal_id,omitempty"`

	// InitiatorGroupID is the initiator group ID for iSCSI backend
	InitiatorGroupID int `json:"initiator_group_id,omitempty" yaml:"initiator_group_id,omitempty"`
}

// HelmClient defines the Helm operations needed for storage component
type HelmClient interface {
	AddRepo(ctx context.Context, opts helm.RepoAddOptions) error
	Install(ctx context.Context, opts helm.InstallOptions) error
	Upgrade(ctx context.Context, opts helm.UpgradeOptions) error
	Uninstall(ctx context.Context, opts helm.UninstallOptions) error
	List(ctx context.Context, namespace string) ([]helm.Release, error)
}

// K8sClient defines the Kubernetes operations needed for storage component
type K8sClient interface {
	GetPods(ctx context.Context, namespace string) ([]*k8s.Pod, error)
	ApplyManifest(ctx context.Context, manifest string) error
}

// Component implements the component.Component interface for storage provisioning
type Component struct {
	helmClient HelmClient
	k8sClient  K8sClient
}

// NewComponent creates a new storage component instance
func NewComponent(helmClient HelmClient, k8sClient K8sClient) *Component {
	return &Component{
		helmClient: helmClient,
		k8sClient:  k8sClient,
	}
}

// Name returns the component name
func (c *Component) Name() string {
	return "storage"
}

// Install installs the storage provisioner based on the configured backend
func (c *Component) Install(ctx context.Context, cfg component.ComponentConfig) error {
	config, err := ParseConfig(cfg)
	if err != nil {
		return fmt.Errorf("parse config: %w", err)
	}

	return Install(ctx, c.helmClient, c.k8sClient, config)
}

// Upgrade upgrades the storage provisioner
func (c *Component) Upgrade(ctx context.Context, cfg component.ComponentConfig) error {
	return fmt.Errorf("upgrade not yet implemented")
}

// Status returns the current status of the storage provisioner
func (c *Component) Status(ctx context.Context) (*component.ComponentStatus, error) {
	if c.helmClient == nil {
		return &component.ComponentStatus{
			Installed: false,
			Healthy:   false,
			Message:   "helm client not initialized",
		}, nil
	}

	// Check default namespace for storage releases
	releases, err := c.helmClient.List(ctx, "kube-system")
	if err != nil {
		return &component.ComponentStatus{
			Installed: false,
			Healthy:   false,
			Message:   fmt.Sprintf("failed to list releases: %v", err),
		}, nil
	}

	// Look for any storage provisioner release
	releaseNames := []string{"local-path-provisioner", "nfs-subdir-external-provisioner", "democratic-csi"}
	for _, rel := range releases {
		for _, name := range releaseNames {
			if rel.Name == name {
				healthy := rel.Status == "deployed"
				return &component.ComponentStatus{
					Installed: true,
					Version:   rel.AppVersion,
					Healthy:   healthy,
					Message:   fmt.Sprintf("release: %s, status: %s", rel.Name, rel.Status),
				}, nil
			}
		}
	}

	// Also check democratic-csi namespace
	releases, err = c.helmClient.List(ctx, "democratic-csi")
	if err == nil {
		for _, rel := range releases {
			if rel.Name == "democratic-csi" {
				healthy := rel.Status == "deployed"
				return &component.ComponentStatus{
					Installed: true,
					Version:   rel.AppVersion,
					Healthy:   healthy,
					Message:   fmt.Sprintf("release: %s, status: %s", rel.Name, rel.Status),
				}, nil
			}
		}
	}

	return &component.ComponentStatus{
		Installed: false,
		Healthy:   false,
		Message:   "no storage provisioner found",
	}, nil
}

// Uninstall removes the storage provisioner
func (c *Component) Uninstall(ctx context.Context) error {
	return fmt.Errorf("uninstall not yet implemented")
}

// Dependencies returns the list of components that storage depends on
func (c *Component) Dependencies() []string {
	return []string{"k3s"} // Storage depends on Kubernetes being available
}

// DefaultConfig returns a Config with sensible defaults (local-path)
func DefaultConfig() *Config {
	return &Config{
		Backend:          BackendLocalPath,
		Version:          "0.0.28", // local-path-provisioner chart version
		Namespace:        "kube-system",
		StorageClassName: "local-path",
		SetDefault:       true,
		LocalPath: &LocalPathConfig{
			Path:          "/opt/local-path-provisioner",
			ReclaimPolicy: "Delete",
		},
	}
}

// ParseConfig parses a ComponentConfig into a storage Config
func ParseConfig(cfg component.ComponentConfig) (*Config, error) {
	config := DefaultConfig()

	if backend, ok := cfg.GetString("backend"); ok {
		config.Backend = StorageBackend(backend)
	}

	if version, ok := cfg.GetString("version"); ok {
		config.Version = version
	}

	if namespace, ok := cfg.GetString("namespace"); ok {
		config.Namespace = namespace
	}

	if storageClassName, ok := cfg.GetString("storage_class_name"); ok {
		config.StorageClassName = storageClassName
	}

	if setDefault, ok := cfg.GetBool("set_default"); ok {
		config.SetDefault = setDefault
	}

	if values, ok := cfg.GetMap("values"); ok {
		config.Values = values
	}

	// Parse backend-specific config
	if localPathCfg, ok := cfg.GetMap("local_path"); ok {
		config.LocalPath = &LocalPathConfig{}
		if path, ok := localPathCfg["path"].(string); ok {
			config.LocalPath.Path = path
		}
		if reclaimPolicy, ok := localPathCfg["reclaim_policy"].(string); ok {
			config.LocalPath.ReclaimPolicy = reclaimPolicy
		}
	}

	if nfsCfg, ok := cfg.GetMap("nfs"); ok {
		config.NFS = &NFSConfig{}
		if server, ok := nfsCfg["server"].(string); ok {
			config.NFS.Server = server
		}
		if path, ok := nfsCfg["path"].(string); ok {
			config.NFS.Path = path
		}
		if reclaimPolicy, ok := nfsCfg["reclaim_policy"].(string); ok {
			config.NFS.ReclaimPolicy = reclaimPolicy
		}
		if archiveOnDelete, ok := nfsCfg["archive_on_delete"].(bool); ok {
			config.NFS.ArchiveOnDelete = archiveOnDelete
		}
	}

	if truenasCfg, ok := cfg.GetMap("truenas"); ok {
		config.TrueNAS = &TrueNASCSIConfig{}
		if apiURL, ok := truenasCfg["api_url"].(string); ok {
			config.TrueNAS.APIURL = apiURL
		}
		if apiKey, ok := truenasCfg["api_key"].(string); ok {
			config.TrueNAS.APIKey = apiKey
		}
		if pool, ok := truenasCfg["pool"].(string); ok {
			config.TrueNAS.Pool = pool
		}
		if datasetParent, ok := truenasCfg["dataset_parent"].(string); ok {
			config.TrueNAS.DatasetParent = datasetParent
		}
		if shareHost, ok := truenasCfg["share_host"].(string); ok {
			config.TrueNAS.ShareHost = shareHost
		}
		if shareNetworks, ok := truenasCfg["share_networks"].([]interface{}); ok {
			config.TrueNAS.ShareNetworks = make([]string, 0, len(shareNetworks))
			for _, n := range shareNetworks {
				if s, ok := n.(string); ok {
					config.TrueNAS.ShareNetworks = append(config.TrueNAS.ShareNetworks, s)
				}
			}
		}
		if portalID, ok := truenasCfg["portal_id"].(float64); ok {
			config.TrueNAS.PortalID = int(portalID)
		}
		if initiatorGroupID, ok := truenasCfg["initiator_group_id"].(float64); ok {
			config.TrueNAS.InitiatorGroupID = int(initiatorGroupID)
		}
	}

	// Validate configuration
	if err := config.Validate(); err != nil {
		return nil, err
	}

	return config, nil
}

// Validate validates the storage configuration
func (c *Config) Validate() error {
	switch c.Backend {
	case BackendLocalPath:
		if c.LocalPath == nil {
			c.LocalPath = &LocalPathConfig{
				Path:          "/opt/local-path-provisioner",
				ReclaimPolicy: "Delete",
			}
		}
	case BackendNFS:
		if c.NFS == nil {
			return fmt.Errorf("nfs configuration required for nfs backend")
		}
		if c.NFS.Server == "" {
			return fmt.Errorf("nfs server is required")
		}
		if c.NFS.Path == "" {
			return fmt.Errorf("nfs path is required")
		}
	case BackendTrueNASNFS:
		if c.TrueNAS == nil {
			return fmt.Errorf("truenas configuration required for truenas-nfs backend")
		}
		if c.TrueNAS.APIURL == "" {
			return fmt.Errorf("truenas api_url is required")
		}
		if c.TrueNAS.APIKey == "" {
			return fmt.Errorf("truenas api_key is required")
		}
		if c.TrueNAS.Pool == "" {
			return fmt.Errorf("truenas pool is required")
		}
		if c.TrueNAS.DatasetParent == "" {
			return fmt.Errorf("truenas dataset_parent is required")
		}
		if c.TrueNAS.ShareHost == "" {
			return fmt.Errorf("truenas share_host is required for NFS backend")
		}
	case BackendTrueNASiSCSI:
		if c.TrueNAS == nil {
			return fmt.Errorf("truenas configuration required for truenas-iscsi backend")
		}
		if c.TrueNAS.APIURL == "" {
			return fmt.Errorf("truenas api_url is required")
		}
		if c.TrueNAS.APIKey == "" {
			return fmt.Errorf("truenas api_key is required")
		}
		if c.TrueNAS.Pool == "" {
			return fmt.Errorf("truenas pool is required")
		}
		if c.TrueNAS.DatasetParent == "" {
			return fmt.Errorf("truenas dataset_parent is required")
		}
	default:
		return fmt.Errorf("unsupported storage backend: %s", c.Backend)
	}

	return nil
}
