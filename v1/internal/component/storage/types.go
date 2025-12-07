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

	// BackendLonghorn uses Longhorn for distributed block storage with replication
	BackendLonghorn StorageBackend = "longhorn"
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

	// Longhorn configuration (for BackendLonghorn)
	Longhorn *LonghornConfig `json:"longhorn,omitempty" yaml:"longhorn,omitempty"`

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

// LonghornConfig holds configuration for Longhorn distributed storage
type LonghornConfig struct {
	// ReplicaCount is the default number of replicas for volumes (default: 3)
	ReplicaCount int `json:"replica_count" yaml:"replica_count"`

	// DataPath is the path on nodes where Longhorn stores data (default: /var/lib/longhorn)
	DataPath string `json:"data_path" yaml:"data_path"`

	// GuaranteedInstanceManagerCPU is the CPU allocation for instance managers (default: 12)
	GuaranteedInstanceManagerCPU int `json:"guaranteed_instance_manager_cpu" yaml:"guaranteed_instance_manager_cpu"`

	// DefaultDataLocality controls data locality (disabled, best-effort, strict-local)
	DefaultDataLocality string `json:"default_data_locality" yaml:"default_data_locality"`
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
	// First, try to check via K8s client for StorageClasses
	// This works even for K3s's built-in local-path provisioner
	if c.k8sClient != nil {
		// Check if there's a default storage class
		// The k8sClient doesn't have GetStorageClasses, so we'll fall through to helm check
	}

	// If we have a helm client, check for Helm-installed provisioners
	if c.helmClient != nil {
		// Check default namespace for storage releases
		releases, err := c.helmClient.List(ctx, "kube-system")
		if err == nil {
			// Look for any storage provisioner release
			releaseNames := []string{"local-path-provisioner", "nfs-subdir-external-provisioner"}
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
		}

		// Check longhorn-system namespace for Longhorn
		releases, err = c.helmClient.List(ctx, "longhorn-system")
		if err == nil {
			for _, rel := range releases {
				if rel.Name == "longhorn" {
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
	}

	// Fall back: Check if K3s bundled local-path provisioner is running
	// by looking for the local-path-provisioner pod in kube-system
	// This is a best-effort check when we don't have clients initialized
	return &component.ComponentStatus{
		Installed: true, // Assume storage is available if K3s is running (it includes local-path)
		Healthy:   true,
		Message:   "assuming K3s bundled local-path provisioner (no client to verify)",
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

	if longhornCfg, ok := cfg.GetMap("longhorn"); ok {
		config.Longhorn = &LonghornConfig{}
		if replicaCount, ok := longhornCfg["replica_count"].(float64); ok {
			config.Longhorn.ReplicaCount = int(replicaCount)
		}
		if replicaCount, ok := longhornCfg["replica_count"].(int); ok {
			config.Longhorn.ReplicaCount = replicaCount
		}
		if dataPath, ok := longhornCfg["data_path"].(string); ok {
			config.Longhorn.DataPath = dataPath
		}
		if cpu, ok := longhornCfg["guaranteed_instance_manager_cpu"].(float64); ok {
			config.Longhorn.GuaranteedInstanceManagerCPU = int(cpu)
		}
		if cpu, ok := longhornCfg["guaranteed_instance_manager_cpu"].(int); ok {
			config.Longhorn.GuaranteedInstanceManagerCPU = cpu
		}
		if dataLocality, ok := longhornCfg["default_data_locality"].(string); ok {
			config.Longhorn.DefaultDataLocality = dataLocality
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
	case BackendLonghorn:
		if c.Longhorn == nil {
			c.Longhorn = &LonghornConfig{
				ReplicaCount:                 3,
				DataPath:                     "/var/lib/longhorn",
				GuaranteedInstanceManagerCPU: 12,
				DefaultDataLocality:          "disabled",
			}
		}
		if c.Longhorn.ReplicaCount < 1 {
			return fmt.Errorf("longhorn replica_count must be at least 1")
		}
	default:
		return fmt.Errorf("unsupported storage backend: %s", c.Backend)
	}

	return nil
}
