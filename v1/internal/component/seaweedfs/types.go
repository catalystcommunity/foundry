package seaweedfs

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/catalystcommunity/foundry/v1/internal/component"
	"github.com/catalystcommunity/foundry/v1/internal/helm"
	"github.com/catalystcommunity/foundry/v1/internal/k8s"
)

// Config holds SeaweedFS component configuration
type Config struct {
	// Version is the Helm chart version to install
	Version string `json:"version" yaml:"version"`

	// Namespace for SeaweedFS deployment
	Namespace string `json:"namespace" yaml:"namespace"`

	// MasterReplicas is the number of master server instances
	MasterReplicas int `json:"master_replicas" yaml:"master_replicas"`

	// VolumeReplicas is the number of volume server instances
	VolumeReplicas int `json:"volume_replicas" yaml:"volume_replicas"`

	// FilerReplicas is the number of filer server instances
	FilerReplicas int `json:"filer_replicas" yaml:"filer_replicas"`

	// StorageClass is the StorageClass to use for SeaweedFS PVCs
	StorageClass string `json:"storage_class" yaml:"storage_class"`

	// StorageSize is the size of storage per volume server
	StorageSize string `json:"storage_size" yaml:"storage_size"`

	// S3Enabled enables the S3 API gateway
	S3Enabled bool `json:"s3_enabled" yaml:"s3_enabled"`

	// S3Port is the port for the S3 API gateway
	S3Port int `json:"s3_port" yaml:"s3_port"`

	// AccessKey is the S3 access key (auto-generated if empty)
	AccessKey string `json:"access_key" yaml:"access_key"`

	// SecretKey is the S3 secret key (auto-generated if empty)
	SecretKey string `json:"secret_key" yaml:"secret_key"`

	// Buckets is a list of buckets to create on startup
	Buckets []string `json:"buckets" yaml:"buckets"`

	// IngressEnabled enables Ingress for SeaweedFS UIs
	IngressEnabled bool `json:"ingress_enabled" yaml:"ingress_enabled"`

	// IngressHostFiler is the hostname for Filer UI Ingress
	IngressHostFiler string `json:"ingress_host_filer" yaml:"ingress_host_filer"`

	// IngressHostS3 is the hostname for S3 API Ingress
	IngressHostS3 string `json:"ingress_host_s3" yaml:"ingress_host_s3"`

	// Values allows passing additional Helm values
	Values map[string]interface{} `json:"values" yaml:",inline"`
}

// HelmClient defines the Helm operations needed for SeaweedFS component
type HelmClient interface {
	AddRepo(ctx context.Context, opts helm.RepoAddOptions) error
	Install(ctx context.Context, opts helm.InstallOptions) error
	Upgrade(ctx context.Context, opts helm.UpgradeOptions) error
	Uninstall(ctx context.Context, opts helm.UninstallOptions) error
	List(ctx context.Context, namespace string) ([]helm.Release, error)
}

// K8sClient defines the Kubernetes operations needed for SeaweedFS component
type K8sClient interface {
	GetPods(ctx context.Context, namespace string) ([]*k8s.Pod, error)
	ApplyManifest(ctx context.Context, manifest string) error
	DeleteJob(ctx context.Context, namespace, name string) error
	WaitForJobComplete(ctx context.Context, namespace, name string, timeout time.Duration) error
}

// Component implements the component.Component interface for SeaweedFS
type Component struct {
	helmClient HelmClient
	k8sClient  K8sClient
}

// NewComponent creates a new SeaweedFS component instance
func NewComponent(helmClient HelmClient, k8sClient K8sClient) *Component {
	return &Component{
		helmClient: helmClient,
		k8sClient:  k8sClient,
	}
}

// Name returns the component name
func (c *Component) Name() string {
	return "seaweedfs"
}

// Install installs SeaweedFS
func (c *Component) Install(ctx context.Context, cfg component.ComponentConfig) error {
	config, err := ParseConfig(cfg)
	if err != nil {
		return fmt.Errorf("parse config: %w", err)
	}

	return Install(ctx, c.helmClient, c.k8sClient, config)
}

// Upgrade upgrades SeaweedFS
func (c *Component) Upgrade(ctx context.Context, cfg component.ComponentConfig) error {
	return fmt.Errorf("upgrade not yet implemented")
}

// Status returns the current status of SeaweedFS
func (c *Component) Status(ctx context.Context) (*component.ComponentStatus, error) {
	if c.helmClient == nil {
		return &component.ComponentStatus{
			Installed: false,
			Healthy:   false,
			Message:   "helm client not initialized",
		}, nil
	}

	// Check for SeaweedFS release
	releases, err := c.helmClient.List(ctx, "seaweedfs")
	if err != nil {
		return &component.ComponentStatus{
			Installed: false,
			Healthy:   false,
			Message:   fmt.Sprintf("failed to list releases: %v", err),
		}, nil
	}

	// Look for seaweedfs release
	for _, rel := range releases {
		if rel.Name == "seaweedfs" {
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
		Message:   "seaweedfs release not found",
	}, nil
}

// Uninstall removes SeaweedFS
func (c *Component) Uninstall(ctx context.Context) error {
	return fmt.Errorf("uninstall not yet implemented")
}

// Dependencies returns the list of components that SeaweedFS depends on
func (c *Component) Dependencies() []string {
	return []string{"storage"} // SeaweedFS needs storage for PVCs
}

// DefaultConfig returns a Config with sensible defaults
func DefaultConfig() *Config {
	return &Config{
		Version:          "4.0.401", // SeaweedFS Helm chart version (app version 4.01)
		Namespace:        "seaweedfs",
		MasterReplicas:   1,
		VolumeReplicas:   1,
		FilerReplicas:    1,
		StorageClass:     "", // Use cluster default
		StorageSize:      "50Gi",
		S3Enabled:        true,
		S3Port:           8333,
		AccessKey:        "", // Will be auto-generated
		SecretKey:        "", // Will be auto-generated
		Buckets:          []string{},
		IngressEnabled:   false,
		IngressHostFiler: "",
		IngressHostS3:    "",
		Values:           make(map[string]interface{}),
	}
}

// ParseConfig parses a ComponentConfig into a SeaweedFS Config
func ParseConfig(cfg component.ComponentConfig) (*Config, error) {
	config := DefaultConfig()

	if version, ok := cfg.GetString("version"); ok {
		config.Version = version
	}

	if namespace, ok := cfg.GetString("namespace"); ok {
		config.Namespace = namespace
	}

	if masterReplicas, ok := cfg.GetInt("master_replicas"); ok {
		config.MasterReplicas = masterReplicas
	}

	if volumeReplicas, ok := cfg.GetInt("volume_replicas"); ok {
		config.VolumeReplicas = volumeReplicas
	}

	if filerReplicas, ok := cfg.GetInt("filer_replicas"); ok {
		config.FilerReplicas = filerReplicas
	}

	if storageClass, ok := cfg.GetString("storage_class"); ok {
		config.StorageClass = storageClass
	}

	if storageSize, ok := cfg.GetString("storage_size"); ok {
		config.StorageSize = storageSize
	}

	if s3Enabled, ok := cfg.GetBool("s3_enabled"); ok {
		config.S3Enabled = s3Enabled
	}

	if s3Port, ok := cfg.GetInt("s3_port"); ok {
		config.S3Port = s3Port
	}

	if accessKey, ok := cfg.GetString("access_key"); ok {
		config.AccessKey = accessKey
	}

	if secretKey, ok := cfg.GetString("secret_key"); ok {
		config.SecretKey = secretKey
	}

	if buckets, ok := cfg.GetStringSlice("buckets"); ok {
		config.Buckets = buckets
	}

	if ingressEnabled, ok := cfg.GetBool("ingress_enabled"); ok {
		config.IngressEnabled = ingressEnabled
	}

	if ingressHostFiler, ok := cfg.GetString("ingress_host_filer"); ok {
		config.IngressHostFiler = ingressHostFiler
	}

	if ingressHostS3, ok := cfg.GetString("ingress_host_s3"); ok {
		config.IngressHostS3 = ingressHostS3
	}

	if values, ok := cfg.GetMap("values"); ok {
		config.Values = values
	}

	// Generate credentials if not provided
	if config.AccessKey == "" {
		config.AccessKey = generateRandomKey(16)
	}
	if config.SecretKey == "" {
		config.SecretKey = generateRandomKey(32)
	}

	// Validate configuration
	if err := config.Validate(); err != nil {
		return nil, err
	}

	return config, nil
}

// Validate validates the SeaweedFS configuration
func (c *Config) Validate() error {
	if c.MasterReplicas < 1 {
		return fmt.Errorf("master_replicas must be at least 1")
	}

	if c.VolumeReplicas < 1 {
		return fmt.Errorf("volume_replicas must be at least 1")
	}

	if c.FilerReplicas < 1 {
		return fmt.Errorf("filer_replicas must be at least 1")
	}

	if c.S3Enabled && (c.S3Port < 1 || c.S3Port > 65535) {
		return fmt.Errorf("s3_port must be between 1 and 65535")
	}

	if c.IngressEnabled && c.IngressHostFiler == "" {
		return fmt.Errorf("ingress_host_filer is required when ingress is enabled")
	}

	return nil
}

// GetS3Endpoint returns the SeaweedFS S3 endpoint URL for internal cluster access
func (c *Config) GetS3Endpoint() string {
	return fmt.Sprintf("http://seaweedfs-s3.%s.svc.cluster.local:%d", c.Namespace, c.S3Port)
}

// GetMasterEndpoint returns the SeaweedFS master endpoint URL for internal cluster access
func (c *Config) GetMasterEndpoint() string {
	return fmt.Sprintf("http://seaweedfs-master.%s.svc.cluster.local:9333", c.Namespace)
}

// GetFilerEndpoint returns the SeaweedFS filer endpoint URL for internal cluster access
func (c *Config) GetFilerEndpoint() string {
	return fmt.Sprintf("http://seaweedfs-filer.%s.svc.cluster.local:8888", c.Namespace)
}

// generateRandomKey generates a random hex-encoded key of the specified byte length
func generateRandomKey(length int) string {
	bytes := make([]byte, length)
	if _, err := rand.Read(bytes); err != nil {
		// Fallback to a fixed string if random generation fails (shouldn't happen)
		return "default-key-please-change"
	}
	return hex.EncodeToString(bytes)
}

// ConnectionInfo holds connection details for other components
type ConnectionInfo struct {
	Endpoint  string
	AccessKey string
	SecretKey string
	UseSSL    bool
	Region    string
}

// GetConnectionInfo returns SeaweedFS connection details for other components
func GetConnectionInfo(cfg *Config) *ConnectionInfo {
	return &ConnectionInfo{
		Endpoint:  fmt.Sprintf("seaweedfs-s3.%s.svc.cluster.local:%d", cfg.Namespace, cfg.S3Port),
		AccessKey: cfg.AccessKey,
		SecretKey: cfg.SecretKey,
		UseSSL:    false,
		Region:    "us-east-1", // SeaweedFS default region
	}
}
