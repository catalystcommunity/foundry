package minio

import (
	"context"
	"fmt"

	"github.com/catalystcommunity/foundry/v1/internal/component"
	"github.com/catalystcommunity/foundry/v1/internal/helm"
	"github.com/catalystcommunity/foundry/v1/internal/k8s"
)

// DeploymentMode represents how MinIO is deployed
type DeploymentMode string

const (
	// ModeStandalone runs a single MinIO instance (suitable for dev/test)
	ModeStandalone DeploymentMode = "standalone"

	// ModeDistributed runs MinIO in distributed mode (production)
	ModeDistributed DeploymentMode = "distributed"
)

// Config holds MinIO component configuration
type Config struct {
	// Version is the Helm chart version to install
	Version string `json:"version" yaml:"version"`

	// Namespace for MinIO deployment
	Namespace string `json:"namespace" yaml:"namespace"`

	// Mode specifies standalone or distributed deployment
	Mode DeploymentMode `json:"mode" yaml:"mode"`

	// Replicas is the number of MinIO replicas (for distributed mode)
	Replicas int `json:"replicas" yaml:"replicas"`

	// RootUser is the root user for MinIO (default: minioadmin)
	RootUser string `json:"root_user" yaml:"root_user"`

	// RootPassword is the root password for MinIO (or secret reference)
	RootPassword string `json:"root_password" yaml:"root_password"`

	// StorageClass is the StorageClass to use for MinIO PVCs
	StorageClass string `json:"storage_class" yaml:"storage_class"`

	// StorageSize is the size of storage per replica
	StorageSize string `json:"storage_size" yaml:"storage_size"`

	// Buckets is a list of buckets to create on startup
	Buckets []string `json:"buckets" yaml:"buckets"`

	// IngressEnabled enables Ingress for MinIO console
	IngressEnabled bool `json:"ingress_enabled" yaml:"ingress_enabled"`

	// IngressHost is the hostname for MinIO console Ingress
	IngressHost string `json:"ingress_host" yaml:"ingress_host"`

	// TLSEnabled enables TLS for MinIO
	TLSEnabled bool `json:"tls_enabled" yaml:"tls_enabled"`

	// Values allows passing additional Helm values
	Values map[string]interface{} `json:"values" yaml:",inline"`
}

// HelmClient defines the Helm operations needed for MinIO component
type HelmClient interface {
	AddRepo(ctx context.Context, opts helm.RepoAddOptions) error
	Install(ctx context.Context, opts helm.InstallOptions) error
	Upgrade(ctx context.Context, opts helm.UpgradeOptions) error
	Uninstall(ctx context.Context, opts helm.UninstallOptions) error
	List(ctx context.Context, namespace string) ([]helm.Release, error)
}

// K8sClient defines the Kubernetes operations needed for MinIO component
type K8sClient interface {
	GetPods(ctx context.Context, namespace string) ([]*k8s.Pod, error)
}

// Component implements the component.Component interface for MinIO
type Component struct {
	helmClient HelmClient
	k8sClient  K8sClient
}

// NewComponent creates a new MinIO component instance
func NewComponent(helmClient HelmClient, k8sClient K8sClient) *Component {
	return &Component{
		helmClient: helmClient,
		k8sClient:  k8sClient,
	}
}

// Name returns the component name
func (c *Component) Name() string {
	return "minio"
}

// Install installs MinIO
func (c *Component) Install(ctx context.Context, cfg component.ComponentConfig) error {
	config, err := ParseConfig(cfg)
	if err != nil {
		return fmt.Errorf("parse config: %w", err)
	}

	return Install(ctx, c.helmClient, c.k8sClient, config)
}

// Upgrade upgrades MinIO
func (c *Component) Upgrade(ctx context.Context, cfg component.ComponentConfig) error {
	return fmt.Errorf("upgrade not yet implemented")
}

// Status returns the current status of MinIO
func (c *Component) Status(ctx context.Context) (*component.ComponentStatus, error) {
	if c.helmClient == nil {
		return &component.ComponentStatus{
			Installed: false,
			Healthy:   false,
			Message:   "helm client not initialized",
		}, nil
	}

	// Check for MinIO release
	releases, err := c.helmClient.List(ctx, "minio")
	if err != nil {
		return &component.ComponentStatus{
			Installed: false,
			Healthy:   false,
			Message:   fmt.Sprintf("failed to list releases: %v", err),
		}, nil
	}

	// Look for minio release
	for _, rel := range releases {
		if rel.Name == "minio" {
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
		Message:   "minio release not found",
	}, nil
}

// Uninstall removes MinIO
func (c *Component) Uninstall(ctx context.Context) error {
	return fmt.Errorf("uninstall not yet implemented")
}

// Dependencies returns the list of components that MinIO depends on
func (c *Component) Dependencies() []string {
	return []string{"storage"} // MinIO needs storage for PVCs
}

// DefaultConfig returns a Config with sensible defaults
func DefaultConfig() *Config {
	return &Config{
		Version:        "5.2.0", // MinIO Helm chart version
		Namespace:      "minio",
		Mode:           ModeStandalone,
		Replicas:       1,
		RootUser:       "minioadmin",
		RootPassword:   "", // Should be set by user or generated
		StorageClass:   "",  // Use cluster default
		StorageSize:    "10Gi",
		Buckets:        []string{},
		IngressEnabled: false,
		TLSEnabled:     false,
		Values:         make(map[string]interface{}),
	}
}

// ParseConfig parses a ComponentConfig into a MinIO Config
func ParseConfig(cfg component.ComponentConfig) (*Config, error) {
	config := DefaultConfig()

	if version, ok := cfg.GetString("version"); ok {
		config.Version = version
	}

	if namespace, ok := cfg.GetString("namespace"); ok {
		config.Namespace = namespace
	}

	if mode, ok := cfg.GetString("mode"); ok {
		config.Mode = DeploymentMode(mode)
	}

	if replicas, ok := cfg.GetInt("replicas"); ok {
		config.Replicas = replicas
	}

	if rootUser, ok := cfg.GetString("root_user"); ok {
		config.RootUser = rootUser
	}

	if rootPassword, ok := cfg.GetString("root_password"); ok {
		config.RootPassword = rootPassword
	}

	if storageClass, ok := cfg.GetString("storage_class"); ok {
		config.StorageClass = storageClass
	}

	if storageSize, ok := cfg.GetString("storage_size"); ok {
		config.StorageSize = storageSize
	}

	if buckets, ok := cfg.GetStringSlice("buckets"); ok {
		config.Buckets = buckets
	}

	if ingressEnabled, ok := cfg.GetBool("ingress_enabled"); ok {
		config.IngressEnabled = ingressEnabled
	}

	if ingressHost, ok := cfg.GetString("ingress_host"); ok {
		config.IngressHost = ingressHost
	}

	if tlsEnabled, ok := cfg.GetBool("tls_enabled"); ok {
		config.TLSEnabled = tlsEnabled
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

// Validate validates the MinIO configuration
func (c *Config) Validate() error {
	switch c.Mode {
	case ModeStandalone:
		// Standalone mode - single replica is fine
	case ModeDistributed:
		if c.Replicas < 4 {
			return fmt.Errorf("distributed mode requires at least 4 replicas")
		}
	default:
		return fmt.Errorf("unsupported deployment mode: %s", c.Mode)
	}

	if c.IngressEnabled && c.IngressHost == "" {
		return fmt.Errorf("ingress_host is required when ingress is enabled")
	}

	return nil
}

// GetEndpoint returns the MinIO endpoint URL for internal cluster access
func (c *Config) GetEndpoint() string {
	return fmt.Sprintf("http://minio.%s.svc.cluster.local:9000", c.Namespace)
}

// GetConsoleEndpoint returns the MinIO console URL for internal cluster access
func (c *Config) GetConsoleEndpoint() string {
	return fmt.Sprintf("http://minio-console.%s.svc.cluster.local:9001", c.Namespace)
}
