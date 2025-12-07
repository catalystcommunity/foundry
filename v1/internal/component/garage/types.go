package garage

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"

	"github.com/catalystcommunity/foundry/v1/internal/component"
	"github.com/catalystcommunity/foundry/v1/internal/helm"
	"github.com/catalystcommunity/foundry/v1/internal/k8s"
)

// Config holds Garage component configuration
type Config struct {
	// Version is the Helm chart version to install
	Version string `json:"version" yaml:"version"`

	// Namespace for Garage deployment
	Namespace string `json:"namespace" yaml:"namespace"`

	// ReplicationFactor is the number of copies of each object (default: 3, or node count if < 3)
	ReplicationFactor int `json:"replication_factor" yaml:"replication_factor"`

	// Replicas is the number of Garage instances to run
	Replicas int `json:"replicas" yaml:"replicas"`

	// StorageClass is the StorageClass to use for Garage PVCs
	StorageClass string `json:"storage_class" yaml:"storage_class"`

	// StorageSize is the size of storage per replica
	StorageSize string `json:"storage_size" yaml:"storage_size"`

	// S3Region is the S3 region to report (default: "garage")
	S3Region string `json:"s3_region" yaml:"s3_region"`

	// AdminKey is the admin API key (auto-generated if empty)
	AdminKey string `json:"admin_key" yaml:"admin_key"`

	// AdminSecret is the admin API secret (auto-generated if empty)
	AdminSecret string `json:"admin_secret" yaml:"admin_secret"`

	// Buckets is a list of buckets to create on startup
	Buckets []string `json:"buckets" yaml:"buckets"`

	// Values allows passing additional Helm values
	Values map[string]interface{} `json:"values" yaml:",inline"`
}

// HelmClient defines the Helm operations needed for Garage component
type HelmClient interface {
	AddRepo(ctx context.Context, opts helm.RepoAddOptions) error
	Install(ctx context.Context, opts helm.InstallOptions) error
	Upgrade(ctx context.Context, opts helm.UpgradeOptions) error
	Uninstall(ctx context.Context, opts helm.UninstallOptions) error
	List(ctx context.Context, namespace string) ([]helm.Release, error)
}

// K8sClient defines the Kubernetes operations needed for Garage component
type K8sClient interface {
	GetPods(ctx context.Context, namespace string) ([]*k8s.Pod, error)
}

// Component implements the component.Component interface for Garage
type Component struct {
	helmClient HelmClient
	k8sClient  K8sClient
}

// NewComponent creates a new Garage component instance
func NewComponent(helmClient HelmClient, k8sClient K8sClient) *Component {
	return &Component{
		helmClient: helmClient,
		k8sClient:  k8sClient,
	}
}

// Name returns the component name
func (c *Component) Name() string {
	return "garage"
}

// Install installs Garage
func (c *Component) Install(ctx context.Context, cfg component.ComponentConfig) error {
	config, err := ParseConfig(cfg)
	if err != nil {
		return fmt.Errorf("parse config: %w", err)
	}

	return Install(ctx, c.helmClient, c.k8sClient, config)
}

// Upgrade upgrades Garage
func (c *Component) Upgrade(ctx context.Context, cfg component.ComponentConfig) error {
	return fmt.Errorf("upgrade not yet implemented")
}

// Status returns the current status of Garage
func (c *Component) Status(ctx context.Context) (*component.ComponentStatus, error) {
	if c.helmClient == nil {
		return &component.ComponentStatus{
			Installed: false,
			Healthy:   false,
			Message:   "helm client not initialized",
		}, nil
	}

	// Check for Garage release
	releases, err := c.helmClient.List(ctx, "garage")
	if err != nil {
		return &component.ComponentStatus{
			Installed: false,
			Healthy:   false,
			Message:   fmt.Sprintf("failed to list releases: %v", err),
		}, nil
	}

	// Look for garage release
	for _, rel := range releases {
		if rel.Name == "garage" {
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
		Message:   "garage release not found",
	}, nil
}

// Uninstall removes Garage
func (c *Component) Uninstall(ctx context.Context) error {
	return fmt.Errorf("uninstall not yet implemented")
}

// Dependencies returns the list of components that Garage depends on
func (c *Component) Dependencies() []string {
	return []string{"storage"} // Garage needs storage for PVCs
}

// DefaultConfig returns a Config with sensible defaults
func DefaultConfig() *Config {
	return &Config{
		Version:           "1.0.1", // Garage Helm chart version
		Namespace:         "garage",
		ReplicationFactor: 1,       // Safe default for single-node; set to 3 for multi-node
		Replicas:          1,       // Match replication factor for homelab
		StorageClass:      "",      // Use cluster default
		StorageSize:       "50Gi",
		S3Region:          "garage",
		AdminKey:          "", // Will be auto-generated
		AdminSecret:       "", // Will be auto-generated
		Buckets:           []string{},
		Values:            make(map[string]interface{}),
	}
}

// ParseConfig parses a ComponentConfig into a Garage Config
func ParseConfig(cfg component.ComponentConfig) (*Config, error) {
	config := DefaultConfig()

	if version, ok := cfg.GetString("version"); ok {
		config.Version = version
	}

	if namespace, ok := cfg.GetString("namespace"); ok {
		config.Namespace = namespace
	}

	if replicationFactor, ok := cfg.GetInt("replication_factor"); ok {
		config.ReplicationFactor = replicationFactor
	}

	if replicas, ok := cfg.GetInt("replicas"); ok {
		config.Replicas = replicas
	}

	if storageClass, ok := cfg.GetString("storage_class"); ok {
		config.StorageClass = storageClass
	}

	if storageSize, ok := cfg.GetString("storage_size"); ok {
		config.StorageSize = storageSize
	}

	if s3Region, ok := cfg.GetString("s3_region"); ok {
		config.S3Region = s3Region
	}

	if adminKey, ok := cfg.GetString("admin_key"); ok {
		config.AdminKey = adminKey
	}

	if adminSecret, ok := cfg.GetString("admin_secret"); ok {
		config.AdminSecret = adminSecret
	}

	if buckets, ok := cfg.GetStringSlice("buckets"); ok {
		config.Buckets = buckets
	}

	if values, ok := cfg.GetMap("values"); ok {
		config.Values = values
	}

	// Generate admin credentials if not provided
	if config.AdminKey == "" {
		config.AdminKey = generateRandomKey(32)
	}
	if config.AdminSecret == "" {
		config.AdminSecret = generateRandomKey(32)
	}

	// Validate configuration
	if err := config.Validate(); err != nil {
		return nil, err
	}

	return config, nil
}

// Validate validates the Garage configuration
func (c *Config) Validate() error {
	if c.ReplicationFactor < 1 {
		return fmt.Errorf("replication_factor must be at least 1")
	}

	if c.Replicas < 1 {
		return fmt.Errorf("replicas must be at least 1")
	}

	if c.ReplicationFactor > c.Replicas {
		return fmt.Errorf("replication_factor (%d) cannot be greater than replicas (%d)", c.ReplicationFactor, c.Replicas)
	}

	return nil
}

// GetEndpoint returns the Garage S3 endpoint URL for internal cluster access
func (c *Config) GetEndpoint() string {
	return fmt.Sprintf("http://garage.%s.svc.cluster.local:3900", c.Namespace)
}

// GetAdminEndpoint returns the Garage admin API endpoint URL for internal cluster access
func (c *Config) GetAdminEndpoint() string {
	return fmt.Sprintf("http://garage.%s.svc.cluster.local:3903", c.Namespace)
}

// GetRPCEndpoint returns the Garage RPC endpoint URL for internal cluster access
func (c *Config) GetRPCEndpoint() string {
	return fmt.Sprintf("http://garage.%s.svc.cluster.local:3901", c.Namespace)
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
