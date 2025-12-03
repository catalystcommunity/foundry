package loki

import (
	"context"
	"fmt"

	"github.com/catalystcommunity/foundry/v1/internal/component"
	"github.com/catalystcommunity/foundry/v1/internal/helm"
	"github.com/catalystcommunity/foundry/v1/internal/k8s"
)

// StorageBackend represents the storage backend for Loki
type StorageBackend string

const (
	// BackendFilesystem uses local filesystem storage (simple, for dev/test)
	BackendFilesystem StorageBackend = "filesystem"

	// BackendS3 uses S3-compatible storage (MinIO or other)
	BackendS3 StorageBackend = "s3"
)

// Config holds Loki component configuration
type Config struct {
	// Version is the Helm chart version to install
	Version string `json:"version" yaml:"version"`

	// Namespace for Loki deployment
	Namespace string `json:"namespace" yaml:"namespace"`

	// DeploymentMode specifies how Loki is deployed (SingleBinary, SimpleScalable, Distributed)
	DeploymentMode string `json:"deployment_mode" yaml:"deployment_mode"`

	// StorageBackend specifies the storage backend (filesystem or s3)
	StorageBackend StorageBackend `json:"storage_backend" yaml:"storage_backend"`

	// RetentionDays is how many days to retain log data
	RetentionDays int `json:"retention_days" yaml:"retention_days"`

	// StorageClass is the StorageClass to use for Loki PVCs
	StorageClass string `json:"storage_class" yaml:"storage_class"`

	// StorageSize is the size of storage for Loki
	StorageSize string `json:"storage_size" yaml:"storage_size"`

	// S3 configuration (required when StorageBackend is s3)
	S3Endpoint  string `json:"s3_endpoint" yaml:"s3_endpoint"`
	S3Bucket    string `json:"s3_bucket" yaml:"s3_bucket"`
	S3AccessKey string `json:"s3_access_key" yaml:"s3_access_key"`
	S3SecretKey string `json:"s3_secret_key" yaml:"s3_secret_key"`
	S3Region    string `json:"s3_region" yaml:"s3_region"`

	// PromtailEnabled enables Promtail for log collection
	PromtailEnabled bool `json:"promtail_enabled" yaml:"promtail_enabled"`

	// GrafanaAgentEnabled uses Grafana Agent instead of Promtail
	GrafanaAgentEnabled bool `json:"grafana_agent_enabled" yaml:"grafana_agent_enabled"`

	// IngressEnabled enables Ingress for Loki
	IngressEnabled bool `json:"ingress_enabled" yaml:"ingress_enabled"`

	// IngressHost is the hostname for Loki Ingress
	IngressHost string `json:"ingress_host" yaml:"ingress_host"`

	// Values allows passing additional Helm values
	Values map[string]interface{} `json:"values" yaml:",inline"`
}

// HelmClient defines the Helm operations needed for Loki component
type HelmClient interface {
	AddRepo(ctx context.Context, opts helm.RepoAddOptions) error
	Install(ctx context.Context, opts helm.InstallOptions) error
	Upgrade(ctx context.Context, opts helm.UpgradeOptions) error
	Uninstall(ctx context.Context, opts helm.UninstallOptions) error
	List(ctx context.Context, namespace string) ([]helm.Release, error)
}

// K8sClient defines the Kubernetes operations needed for Loki component
type K8sClient interface {
	GetPods(ctx context.Context, namespace string) ([]*k8s.Pod, error)
}

// Component implements the component.Component interface for Loki
type Component struct {
	helmClient HelmClient
	k8sClient  K8sClient
}

// NewComponent creates a new Loki component instance
func NewComponent(helmClient HelmClient, k8sClient K8sClient) *Component {
	return &Component{
		helmClient: helmClient,
		k8sClient:  k8sClient,
	}
}

// Name returns the component name
func (c *Component) Name() string {
	return "loki"
}

// Install installs Loki
func (c *Component) Install(ctx context.Context, cfg component.ComponentConfig) error {
	config, err := ParseConfig(cfg)
	if err != nil {
		return fmt.Errorf("parse config: %w", err)
	}

	return Install(ctx, c.helmClient, c.k8sClient, config)
}

// Upgrade upgrades Loki
func (c *Component) Upgrade(ctx context.Context, cfg component.ComponentConfig) error {
	return fmt.Errorf("upgrade not yet implemented")
}

// Status returns the current status of Loki
func (c *Component) Status(ctx context.Context) (*component.ComponentStatus, error) {
	if c.helmClient == nil {
		return &component.ComponentStatus{
			Installed: false,
			Healthy:   false,
			Message:   "helm client not initialized",
		}, nil
	}

	// Check for loki release
	releases, err := c.helmClient.List(ctx, "loki")
	if err != nil {
		return &component.ComponentStatus{
			Installed: false,
			Healthy:   false,
			Message:   fmt.Sprintf("failed to list releases: %v", err),
		}, nil
	}

	// Look for loki release
	for _, rel := range releases {
		if rel.Name == "loki" {
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
		Message:   "loki release not found",
	}, nil
}

// Uninstall removes Loki
func (c *Component) Uninstall(ctx context.Context) error {
	return fmt.Errorf("uninstall not yet implemented")
}

// Dependencies returns the list of components that Loki depends on
func (c *Component) Dependencies() []string {
	return []string{"storage", "minio"} // Loki needs storage and MinIO for S3
}

// DefaultConfig returns a Config with sensible defaults
func DefaultConfig() *Config {
	return &Config{
		Version:             "6.23.0", // loki Helm chart version
		Namespace:           "loki",
		DeploymentMode:      "SingleBinary", // Simple for homelab
		StorageBackend:      BackendS3,      // Use MinIO by default
		RetentionDays:       30,
		StorageClass:        "", // Use cluster default
		StorageSize:         "10Gi",
		S3Endpoint:          "http://minio.minio.svc.cluster.local:9000",
		S3Bucket:            "loki",
		S3Region:            "us-east-1",
		PromtailEnabled:     true,
		GrafanaAgentEnabled: false,
		IngressEnabled:      false,
		Values:              make(map[string]interface{}),
	}
}

// ParseConfig parses a ComponentConfig into a Loki Config
func ParseConfig(cfg component.ComponentConfig) (*Config, error) {
	config := DefaultConfig()

	if version, ok := cfg.GetString("version"); ok {
		config.Version = version
	}

	if namespace, ok := cfg.GetString("namespace"); ok {
		config.Namespace = namespace
	}

	if deploymentMode, ok := cfg.GetString("deployment_mode"); ok {
		config.DeploymentMode = deploymentMode
	}

	if storageBackend, ok := cfg.GetString("storage_backend"); ok {
		config.StorageBackend = StorageBackend(storageBackend)
	}

	if retentionDays, ok := cfg.GetInt("retention_days"); ok {
		config.RetentionDays = retentionDays
	}

	if storageClass, ok := cfg.GetString("storage_class"); ok {
		config.StorageClass = storageClass
	}

	if storageSize, ok := cfg.GetString("storage_size"); ok {
		config.StorageSize = storageSize
	}

	if s3Endpoint, ok := cfg.GetString("s3_endpoint"); ok {
		config.S3Endpoint = s3Endpoint
	}

	if s3Bucket, ok := cfg.GetString("s3_bucket"); ok {
		config.S3Bucket = s3Bucket
	}

	if s3AccessKey, ok := cfg.GetString("s3_access_key"); ok {
		config.S3AccessKey = s3AccessKey
	}

	if s3SecretKey, ok := cfg.GetString("s3_secret_key"); ok {
		config.S3SecretKey = s3SecretKey
	}

	if s3Region, ok := cfg.GetString("s3_region"); ok {
		config.S3Region = s3Region
	}

	if promtailEnabled, ok := cfg.GetBool("promtail_enabled"); ok {
		config.PromtailEnabled = promtailEnabled
	}

	if grafanaAgentEnabled, ok := cfg.GetBool("grafana_agent_enabled"); ok {
		config.GrafanaAgentEnabled = grafanaAgentEnabled
	}

	if ingressEnabled, ok := cfg.GetBool("ingress_enabled"); ok {
		config.IngressEnabled = ingressEnabled
	}

	if ingressHost, ok := cfg.GetString("ingress_host"); ok {
		config.IngressHost = ingressHost
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

// Validate validates the Loki configuration
func (c *Config) Validate() error {
	if c.RetentionDays < 1 {
		return fmt.Errorf("retention_days must be at least 1")
	}

	if c.StorageBackend == BackendS3 {
		if c.S3Endpoint == "" {
			return fmt.Errorf("s3_endpoint is required when using S3 storage backend")
		}
		if c.S3Bucket == "" {
			return fmt.Errorf("s3_bucket is required when using S3 storage backend")
		}
	}

	if c.IngressEnabled && c.IngressHost == "" {
		return fmt.Errorf("ingress_host is required when ingress is enabled")
	}

	return nil
}

// GetLokiEndpoint returns the Loki endpoint URL for internal cluster access
func (c *Config) GetLokiEndpoint() string {
	return fmt.Sprintf("http://loki-gateway.%s.svc.cluster.local:80", c.Namespace)
}

// GetLokiPushEndpoint returns the Loki push API endpoint
func (c *Config) GetLokiPushEndpoint() string {
	return fmt.Sprintf("http://loki-gateway.%s.svc.cluster.local:80/loki/api/v1/push", c.Namespace)
}
