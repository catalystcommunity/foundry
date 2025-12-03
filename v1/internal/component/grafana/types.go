package grafana

import (
	"context"
	"fmt"

	"github.com/catalystcommunity/foundry/v1/internal/component"
	"github.com/catalystcommunity/foundry/v1/internal/helm"
	"github.com/catalystcommunity/foundry/v1/internal/k8s"
)

// Config holds Grafana component configuration
type Config struct {
	// Version is the Helm chart version to install
	Version string `json:"version" yaml:"version"`

	// Namespace for Grafana deployment
	Namespace string `json:"namespace" yaml:"namespace"`

	// AdminUser is the admin username (default: admin)
	AdminUser string `json:"admin_user" yaml:"admin_user"`

	// AdminPassword is the admin password (or leave empty to generate)
	AdminPassword string `json:"admin_password" yaml:"admin_password"`

	// StorageClass is the StorageClass to use for Grafana PVC
	StorageClass string `json:"storage_class" yaml:"storage_class"`

	// StorageSize is the size of storage for Grafana
	StorageSize string `json:"storage_size" yaml:"storage_size"`

	// PrometheusURL is the Prometheus data source URL
	PrometheusURL string `json:"prometheus_url" yaml:"prometheus_url"`

	// LokiURL is the Loki data source URL
	LokiURL string `json:"loki_url" yaml:"loki_url"`

	// IngressEnabled enables Ingress for Grafana
	IngressEnabled bool `json:"ingress_enabled" yaml:"ingress_enabled"`

	// IngressHost is the hostname for Grafana Ingress
	IngressHost string `json:"ingress_host" yaml:"ingress_host"`

	// DefaultDashboardsEnabled enables default dashboards
	DefaultDashboardsEnabled bool `json:"default_dashboards_enabled" yaml:"default_dashboards_enabled"`

	// SidecarEnabled enables the sidecar for dashboard/datasource discovery
	SidecarEnabled bool `json:"sidecar_enabled" yaml:"sidecar_enabled"`

	// Values allows passing additional Helm values
	Values map[string]interface{} `json:"values" yaml:",inline"`
}

// HelmClient defines the Helm operations needed for Grafana component
type HelmClient interface {
	AddRepo(ctx context.Context, opts helm.RepoAddOptions) error
	Install(ctx context.Context, opts helm.InstallOptions) error
	Upgrade(ctx context.Context, opts helm.UpgradeOptions) error
	Uninstall(ctx context.Context, opts helm.UninstallOptions) error
	List(ctx context.Context, namespace string) ([]helm.Release, error)
}

// K8sClient defines the Kubernetes operations needed for Grafana component
type K8sClient interface {
	GetPods(ctx context.Context, namespace string) ([]*k8s.Pod, error)
}

// Component implements the component.Component interface for Grafana
type Component struct {
	helmClient HelmClient
	k8sClient  K8sClient
}

// NewComponent creates a new Grafana component instance
func NewComponent(helmClient HelmClient, k8sClient K8sClient) *Component {
	return &Component{
		helmClient: helmClient,
		k8sClient:  k8sClient,
	}
}

// Name returns the component name
func (c *Component) Name() string {
	return "grafana"
}

// Install installs Grafana
func (c *Component) Install(ctx context.Context, cfg component.ComponentConfig) error {
	config, err := ParseConfig(cfg)
	if err != nil {
		return fmt.Errorf("parse config: %w", err)
	}

	return Install(ctx, c.helmClient, c.k8sClient, config)
}

// Upgrade upgrades Grafana
func (c *Component) Upgrade(ctx context.Context, cfg component.ComponentConfig) error {
	return fmt.Errorf("upgrade not yet implemented")
}

// Status returns the current status of Grafana
func (c *Component) Status(ctx context.Context) (*component.ComponentStatus, error) {
	if c.helmClient == nil {
		return &component.ComponentStatus{
			Installed: false,
			Healthy:   false,
			Message:   "helm client not initialized",
		}, nil
	}

	// Check for grafana release
	releases, err := c.helmClient.List(ctx, "grafana")
	if err != nil {
		return &component.ComponentStatus{
			Installed: false,
			Healthy:   false,
			Message:   fmt.Sprintf("failed to list releases: %v", err),
		}, nil
	}

	// Look for grafana release
	for _, rel := range releases {
		if rel.Name == "grafana" {
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
		Message:   "grafana release not found",
	}, nil
}

// Uninstall removes Grafana
func (c *Component) Uninstall(ctx context.Context) error {
	return fmt.Errorf("uninstall not yet implemented")
}

// Dependencies returns the list of components that Grafana depends on
func (c *Component) Dependencies() []string {
	return []string{"prometheus", "loki"} // Grafana needs data sources
}

// DefaultConfig returns a Config with sensible defaults
func DefaultConfig() *Config {
	return &Config{
		Version:                  "8.8.2", // grafana Helm chart version
		Namespace:                "grafana",
		AdminUser:                "admin",
		AdminPassword:            "", // Will be auto-generated if empty
		StorageClass:             "", // Use cluster default
		StorageSize:              "5Gi",
		PrometheusURL:            "http://kube-prometheus-stack-prometheus.monitoring.svc.cluster.local:9090",
		LokiURL:                  "http://loki-gateway.loki.svc.cluster.local:80",
		IngressEnabled:           false,
		DefaultDashboardsEnabled: true,
		SidecarEnabled:           true,
		Values:                   make(map[string]interface{}),
	}
}

// ParseConfig parses a ComponentConfig into a Grafana Config
func ParseConfig(cfg component.ComponentConfig) (*Config, error) {
	config := DefaultConfig()

	if version, ok := cfg.GetString("version"); ok {
		config.Version = version
	}

	if namespace, ok := cfg.GetString("namespace"); ok {
		config.Namespace = namespace
	}

	if adminUser, ok := cfg.GetString("admin_user"); ok {
		config.AdminUser = adminUser
	}

	if adminPassword, ok := cfg.GetString("admin_password"); ok {
		config.AdminPassword = adminPassword
	}

	if storageClass, ok := cfg.GetString("storage_class"); ok {
		config.StorageClass = storageClass
	}

	if storageSize, ok := cfg.GetString("storage_size"); ok {
		config.StorageSize = storageSize
	}

	if prometheusURL, ok := cfg.GetString("prometheus_url"); ok {
		config.PrometheusURL = prometheusURL
	}

	if lokiURL, ok := cfg.GetString("loki_url"); ok {
		config.LokiURL = lokiURL
	}

	if ingressEnabled, ok := cfg.GetBool("ingress_enabled"); ok {
		config.IngressEnabled = ingressEnabled
	}

	if ingressHost, ok := cfg.GetString("ingress_host"); ok {
		config.IngressHost = ingressHost
	}

	if defaultDashboardsEnabled, ok := cfg.GetBool("default_dashboards_enabled"); ok {
		config.DefaultDashboardsEnabled = defaultDashboardsEnabled
	}

	if sidecarEnabled, ok := cfg.GetBool("sidecar_enabled"); ok {
		config.SidecarEnabled = sidecarEnabled
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

// Validate validates the Grafana configuration
func (c *Config) Validate() error {
	if c.IngressEnabled && c.IngressHost == "" {
		return fmt.Errorf("ingress_host is required when ingress is enabled")
	}

	return nil
}

// GetGrafanaEndpoint returns the Grafana endpoint URL for internal cluster access
func (c *Config) GetGrafanaEndpoint() string {
	return fmt.Sprintf("http://grafana.%s.svc.cluster.local:80", c.Namespace)
}
