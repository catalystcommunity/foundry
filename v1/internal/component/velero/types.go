package velero

import (
	"context"
	"fmt"

	"github.com/catalystcommunity/foundry/v1/internal/component"
	"github.com/catalystcommunity/foundry/v1/internal/helm"
	"github.com/catalystcommunity/foundry/v1/internal/k8s"
)

// BackupStorageProvider represents the storage provider for Velero backups
type BackupStorageProvider string

const (
	// ProviderAWS uses AWS S3 for backup storage
	ProviderAWS BackupStorageProvider = "aws"

	// ProviderS3 uses S3-compatible storage (Garage, MinIO, etc.) for backup storage
	ProviderS3 BackupStorageProvider = "s3"
)

// Config holds Velero component configuration
type Config struct {
	// Version is the Helm chart version to install
	Version string `json:"version" yaml:"version"`

	// Namespace for Velero deployment
	Namespace string `json:"namespace" yaml:"namespace"`

	// Provider specifies the backup storage provider (aws for AWS S3, s3 for S3-compatible like Garage)
	Provider BackupStorageProvider `json:"provider" yaml:"provider"`

	// S3Endpoint is the S3-compatible endpoint URL (required for s3 provider)
	S3Endpoint string `json:"s3_endpoint" yaml:"s3_endpoint"`

	// S3Bucket is the bucket name for backups
	S3Bucket string `json:"s3_bucket" yaml:"s3_bucket"`

	// S3Region is the S3 region (default: garage for Garage)
	S3Region string `json:"s3_region" yaml:"s3_region"`

	// S3AccessKey is the S3 access key ID
	S3AccessKey string `json:"s3_access_key" yaml:"s3_access_key"`

	// S3SecretKey is the S3 secret access key
	S3SecretKey string `json:"s3_secret_key" yaml:"s3_secret_key"`

	// S3InsecureSkipTLSVerify skips TLS verification for S3 endpoint
	S3InsecureSkipTLSVerify bool `json:"s3_insecure_skip_tls_verify" yaml:"s3_insecure_skip_tls_verify"`

	// S3ForcePathStyle uses path-style S3 URLs (required for Garage and other S3-compatible storage)
	S3ForcePathStyle bool `json:"s3_force_path_style" yaml:"s3_force_path_style"`

	// DefaultBackupStorageLocation is whether this is the default BSL
	DefaultBackupStorageLocation bool `json:"default_backup_storage_location" yaml:"default_backup_storage_location"`

	// DefaultVolumeSnapshotLocations is whether this is the default VSL
	DefaultVolumeSnapshotLocations bool `json:"default_volume_snapshot_locations" yaml:"default_volume_snapshot_locations"`

	// BackupRetentionDays is how long to retain backups (0 = forever)
	BackupRetentionDays int `json:"backup_retention_days" yaml:"backup_retention_days"`

	// ScheduleName is the name for the default backup schedule
	ScheduleName string `json:"schedule_name" yaml:"schedule_name"`

	// ScheduleCron is the cron expression for scheduled backups (empty = no schedule)
	ScheduleCron string `json:"schedule_cron" yaml:"schedule_cron"`

	// ScheduleIncludedNamespaces is the list of namespaces to include in scheduled backups
	ScheduleIncludedNamespaces []string `json:"schedule_included_namespaces" yaml:"schedule_included_namespaces"`

	// ScheduleExcludedNamespaces is the list of namespaces to exclude from scheduled backups
	ScheduleExcludedNamespaces []string `json:"schedule_excluded_namespaces" yaml:"schedule_excluded_namespaces"`

	// SnapshotsEnabled enables volume snapshots (requires CSI driver support)
	SnapshotsEnabled bool `json:"snapshots_enabled" yaml:"snapshots_enabled"`

	// CSISnapshotTimeout is the timeout for CSI volume snapshots
	CSISnapshotTimeout string `json:"csi_snapshot_timeout" yaml:"csi_snapshot_timeout"`

	// ResourceRequests specifies resource requests for Velero server
	ResourceRequests map[string]string `json:"resource_requests" yaml:"resource_requests"`

	// ResourceLimits specifies resource limits for Velero server
	ResourceLimits map[string]string `json:"resource_limits" yaml:"resource_limits"`

	// Values allows passing additional Helm values
	Values map[string]interface{} `json:"values" yaml:",inline"`
}

// HelmClient defines the Helm operations needed for Velero component
type HelmClient interface {
	AddRepo(ctx context.Context, opts helm.RepoAddOptions) error
	Install(ctx context.Context, opts helm.InstallOptions) error
	Upgrade(ctx context.Context, opts helm.UpgradeOptions) error
	Uninstall(ctx context.Context, opts helm.UninstallOptions) error
	List(ctx context.Context, namespace string) ([]helm.Release, error)
}

// K8sClient defines the Kubernetes operations needed for Velero component
type K8sClient interface {
	GetPods(ctx context.Context, namespace string) ([]*k8s.Pod, error)
}

// Component implements the component.Component interface for Velero
type Component struct {
	helmClient HelmClient
	k8sClient  K8sClient
}

// NewComponent creates a new Velero component instance
func NewComponent(helmClient HelmClient, k8sClient K8sClient) *Component {
	return &Component{
		helmClient: helmClient,
		k8sClient:  k8sClient,
	}
}

// Name returns the component name
func (c *Component) Name() string {
	return "velero"
}

// Install installs Velero
func (c *Component) Install(ctx context.Context, cfg component.ComponentConfig) error {
	config, err := ParseConfig(cfg)
	if err != nil {
		return fmt.Errorf("parse config: %w", err)
	}

	return Install(ctx, c.helmClient, c.k8sClient, config)
}

// Upgrade upgrades Velero
func (c *Component) Upgrade(ctx context.Context, cfg component.ComponentConfig) error {
	return fmt.Errorf("upgrade not yet implemented")
}

// Status returns the current status of Velero
func (c *Component) Status(ctx context.Context) (*component.ComponentStatus, error) {
	if c.helmClient == nil {
		return &component.ComponentStatus{
			Installed: false,
			Healthy:   false,
			Message:   "helm client not initialized",
		}, nil
	}

	// Check for velero release
	releases, err := c.helmClient.List(ctx, "velero")
	if err != nil {
		return &component.ComponentStatus{
			Installed: false,
			Healthy:   false,
			Message:   fmt.Sprintf("failed to list releases: %v", err),
		}, nil
	}

	// Look for velero release
	for _, rel := range releases {
		if rel.Name == "velero" {
			healthy := rel.Status == "deployed"
			return &component.ComponentStatus{
				Installed: true,
				Version:   rel.AppVersion,
				Healthy:   healthy,
				Message:   fmt.Sprintf("release status: %s", rel.Status),
			}, nil
		}
	}

	return &component.ComponentStatus{
		Installed: false,
		Healthy:   false,
		Message:   "velero release not found",
	}, nil
}

// Uninstall removes Velero
func (c *Component) Uninstall(ctx context.Context) error {
	return fmt.Errorf("uninstall not yet implemented")
}

// Dependencies returns the list of components that Velero depends on
func (c *Component) Dependencies() []string {
	return []string{"garage"} // Velero needs Garage for S3-compatible backup storage
}

// DefaultConfig returns a Config with sensible defaults
func DefaultConfig() *Config {
	return &Config{
		Version:                        "8.0.0", // Velero Helm chart version
		Namespace:                      "velero",
		Provider:                       ProviderS3,
		S3Endpoint:                     "http://garage.garage.svc.cluster.local:3900",
		S3Bucket:                       "velero",
		S3Region:                       "garage",
		S3InsecureSkipTLSVerify:        true, // Garage typically runs without TLS internally
		S3ForcePathStyle:               true, // Required for Garage
		DefaultBackupStorageLocation:   true,
		DefaultVolumeSnapshotLocations: false,
		BackupRetentionDays:            30,
		ScheduleName:                   "daily-backup",
		ScheduleCron:                   "", // Empty = no schedule by default
		ScheduleIncludedNamespaces:     []string{},
		ScheduleExcludedNamespaces:     []string{"kube-system", "velero"},
		SnapshotsEnabled:               false, // Disabled by default, requires CSI support
		CSISnapshotTimeout:             "10m",
		ResourceRequests: map[string]string{
			"memory": "128Mi", // No CPU request for homelab environments with limited resources
		},
		ResourceLimits: map[string]string{},
		Values:         make(map[string]interface{}),
	}
}

// ParseConfig parses a ComponentConfig into a Velero Config
func ParseConfig(cfg component.ComponentConfig) (*Config, error) {
	config := DefaultConfig()

	if version, ok := cfg.GetString("version"); ok {
		config.Version = version
	}

	if namespace, ok := cfg.GetString("namespace"); ok {
		config.Namespace = namespace
	}

	if provider, ok := cfg.GetString("provider"); ok {
		config.Provider = BackupStorageProvider(provider)
	}

	if s3Endpoint, ok := cfg.GetString("s3_endpoint"); ok {
		config.S3Endpoint = s3Endpoint
	}

	if s3Bucket, ok := cfg.GetString("s3_bucket"); ok {
		config.S3Bucket = s3Bucket
	}

	if s3Region, ok := cfg.GetString("s3_region"); ok {
		config.S3Region = s3Region
	}

	if s3AccessKey, ok := cfg.GetString("s3_access_key"); ok {
		config.S3AccessKey = s3AccessKey
	}

	if s3SecretKey, ok := cfg.GetString("s3_secret_key"); ok {
		config.S3SecretKey = s3SecretKey
	}

	if s3InsecureSkipTLSVerify, ok := cfg.GetBool("s3_insecure_skip_tls_verify"); ok {
		config.S3InsecureSkipTLSVerify = s3InsecureSkipTLSVerify
	}

	if s3ForcePathStyle, ok := cfg.GetBool("s3_force_path_style"); ok {
		config.S3ForcePathStyle = s3ForcePathStyle
	}

	if defaultBSL, ok := cfg.GetBool("default_backup_storage_location"); ok {
		config.DefaultBackupStorageLocation = defaultBSL
	}

	if defaultVSL, ok := cfg.GetBool("default_volume_snapshot_locations"); ok {
		config.DefaultVolumeSnapshotLocations = defaultVSL
	}

	if backupRetentionDays, ok := cfg.GetInt("backup_retention_days"); ok {
		config.BackupRetentionDays = backupRetentionDays
	}

	if scheduleName, ok := cfg.GetString("schedule_name"); ok {
		config.ScheduleName = scheduleName
	}

	if scheduleCron, ok := cfg.GetString("schedule_cron"); ok {
		config.ScheduleCron = scheduleCron
	}

	if scheduleIncludedNamespaces, ok := cfg.GetStringSlice("schedule_included_namespaces"); ok {
		config.ScheduleIncludedNamespaces = scheduleIncludedNamespaces
	}

	if scheduleExcludedNamespaces, ok := cfg.GetStringSlice("schedule_excluded_namespaces"); ok {
		config.ScheduleExcludedNamespaces = scheduleExcludedNamespaces
	}

	if snapshotsEnabled, ok := cfg.GetBool("snapshots_enabled"); ok {
		config.SnapshotsEnabled = snapshotsEnabled
	}

	if csiSnapshotTimeout, ok := cfg.GetString("csi_snapshot_timeout"); ok {
		config.CSISnapshotTimeout = csiSnapshotTimeout
	}

	if resourceRequests, ok := cfg.GetMap("resource_requests"); ok {
		config.ResourceRequests = make(map[string]string)
		for k, v := range resourceRequests {
			if str, ok := v.(string); ok {
				config.ResourceRequests[k] = str
			}
		}
	}

	if resourceLimits, ok := cfg.GetMap("resource_limits"); ok {
		config.ResourceLimits = make(map[string]string)
		for k, v := range resourceLimits {
			if str, ok := v.(string); ok {
				config.ResourceLimits[k] = str
			}
		}
	}

	if values, ok := cfg.GetMap("values"); ok {
		config.Values = values
	}

	// Validate configuration
	if err := config.Validate(); err != nil {
		return nil, err
	}

	return config, nil
}

// Validate validates the Velero configuration
func (c *Config) Validate() error {
	switch c.Provider {
	case ProviderAWS, ProviderS3:
		// Valid providers
	default:
		return fmt.Errorf("unsupported provider: %s (supported: aws, s3)", c.Provider)
	}

	if c.S3Bucket == "" {
		return fmt.Errorf("s3_bucket is required")
	}

	// S3-compatible storage requires endpoint
	if c.Provider == ProviderS3 && c.S3Endpoint == "" {
		return fmt.Errorf("s3_endpoint is required when using S3-compatible provider")
	}

	if c.BackupRetentionDays < 0 {
		return fmt.Errorf("backup_retention_days cannot be negative")
	}

	return nil
}

// GetVeleroEndpoint returns the Velero server endpoint URL for internal cluster access
func (c *Config) GetVeleroEndpoint() string {
	return fmt.Sprintf("http://velero.%s.svc.cluster.local:8085", c.Namespace)
}

// GetBackupStorageLocationName returns the name of the default backup storage location
func (c *Config) GetBackupStorageLocationName() string {
	return "default"
}
