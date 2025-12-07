package prometheus

import (
	"context"
	"fmt"

	"github.com/catalystcommunity/foundry/v1/internal/component"
	"github.com/catalystcommunity/foundry/v1/internal/helm"
	"github.com/catalystcommunity/foundry/v1/internal/k8s"
)

// Config holds Prometheus stack component configuration
type Config struct {
	// Version is the Helm chart version to install
	Version string `json:"version" yaml:"version"`

	// Namespace for Prometheus deployment
	Namespace string `json:"namespace" yaml:"namespace"`

	// RetentionDays is how many days to retain metrics data
	RetentionDays int `json:"retention_days" yaml:"retention_days"`

	// RetentionSize is the maximum size of the TSDB (e.g., "10Gi")
	RetentionSize string `json:"retention_size" yaml:"retention_size"`

	// StorageClass is the StorageClass to use for Prometheus PVCs
	StorageClass string `json:"storage_class" yaml:"storage_class"`

	// StorageSize is the size of storage for Prometheus TSDB
	StorageSize string `json:"storage_size" yaml:"storage_size"`

	// AlertmanagerEnabled enables Alertmanager deployment
	AlertmanagerEnabled bool `json:"alertmanager_enabled" yaml:"alertmanager_enabled"`

	// GrafanaEnabled enables Grafana deployment (we disable since we deploy separately)
	GrafanaEnabled bool `json:"grafana_enabled" yaml:"grafana_enabled"`

	// NodeExporterEnabled enables node-exporter for host metrics
	NodeExporterEnabled bool `json:"node_exporter_enabled" yaml:"node_exporter_enabled"`

	// KubeStateMetricsEnabled enables kube-state-metrics
	KubeStateMetricsEnabled bool `json:"kube_state_metrics_enabled" yaml:"kube_state_metrics_enabled"`

	// ScrapeInterval is the default scrape interval (e.g., "30s")
	ScrapeInterval string `json:"scrape_interval" yaml:"scrape_interval"`

	// IngressEnabled enables Ingress for Prometheus
	IngressEnabled bool `json:"ingress_enabled" yaml:"ingress_enabled"`

	// IngressHost is the hostname for Prometheus Ingress
	IngressHost string `json:"ingress_host" yaml:"ingress_host"`

	// Values allows passing additional Helm values
	Values map[string]interface{} `json:"values" yaml:",inline"`
}

// HelmClient defines the Helm operations needed for Prometheus component
type HelmClient interface {
	AddRepo(ctx context.Context, opts helm.RepoAddOptions) error
	Install(ctx context.Context, opts helm.InstallOptions) error
	Upgrade(ctx context.Context, opts helm.UpgradeOptions) error
	Uninstall(ctx context.Context, opts helm.UninstallOptions) error
	List(ctx context.Context, namespace string) ([]helm.Release, error)
}

// K8sClient defines the Kubernetes operations needed for Prometheus component
type K8sClient interface {
	GetPods(ctx context.Context, namespace string) ([]*k8s.Pod, error)
}

// Component implements the component.Component interface for Prometheus
type Component struct {
	helmClient HelmClient
	k8sClient  K8sClient
}

// NewComponent creates a new Prometheus component instance
func NewComponent(helmClient HelmClient, k8sClient K8sClient) *Component {
	return &Component{
		helmClient: helmClient,
		k8sClient:  k8sClient,
	}
}

// Name returns the component name
func (c *Component) Name() string {
	return "prometheus"
}

// Install installs the Prometheus stack
func (c *Component) Install(ctx context.Context, cfg component.ComponentConfig) error {
	config, err := ParseConfig(cfg)
	if err != nil {
		return fmt.Errorf("parse config: %w", err)
	}

	return Install(ctx, c.helmClient, c.k8sClient, config)
}

// Upgrade upgrades the Prometheus stack
func (c *Component) Upgrade(ctx context.Context, cfg component.ComponentConfig) error {
	return fmt.Errorf("upgrade not yet implemented")
}

// Status returns the current status of Prometheus
func (c *Component) Status(ctx context.Context) (*component.ComponentStatus, error) {
	if c.helmClient == nil {
		return &component.ComponentStatus{
			Installed: false,
			Healthy:   false,
			Message:   "helm client not initialized",
		}, nil
	}

	// Check for prometheus-stack release
	releases, err := c.helmClient.List(ctx, "monitoring")
	if err != nil {
		return &component.ComponentStatus{
			Installed: false,
			Healthy:   false,
			Message:   fmt.Sprintf("failed to list releases: %v", err),
		}, nil
	}

	// Look for kube-prometheus-stack release
	for _, rel := range releases {
		if rel.Name == "kube-prometheus-stack" {
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
		Message:   "kube-prometheus-stack release not found",
	}, nil
}

// Uninstall removes the Prometheus stack
func (c *Component) Uninstall(ctx context.Context) error {
	return fmt.Errorf("uninstall not yet implemented")
}

// Dependencies returns the list of components that Prometheus depends on
func (c *Component) Dependencies() []string {
	return []string{"storage"} // Prometheus needs storage for PVCs
}

// DefaultConfig returns a Config with sensible defaults
func DefaultConfig() *Config {
	return &Config{
		Version:                 "67.4.0", // kube-prometheus-stack chart version
		Namespace:               "monitoring",
		RetentionDays:           15,
		RetentionSize:           "10GB", // Must end with 'B' per Prometheus CRD spec
		StorageClass:            "", // Use cluster default
		StorageSize:             "20Gi",
		AlertmanagerEnabled:     true,
		GrafanaEnabled:          false, // We deploy Grafana separately
		NodeExporterEnabled:     true,
		KubeStateMetricsEnabled: true,
		ScrapeInterval:          "30s",
		IngressEnabled:          false,
		Values:                  make(map[string]interface{}),
	}
}

// ParseConfig parses a ComponentConfig into a Prometheus Config
func ParseConfig(cfg component.ComponentConfig) (*Config, error) {
	config := DefaultConfig()

	if version, ok := cfg.GetString("version"); ok {
		config.Version = version
	}

	if namespace, ok := cfg.GetString("namespace"); ok {
		config.Namespace = namespace
	}

	if retentionDays, ok := cfg.GetInt("retention_days"); ok {
		config.RetentionDays = retentionDays
	}

	if retentionSize, ok := cfg.GetString("retention_size"); ok {
		config.RetentionSize = retentionSize
	}

	if storageClass, ok := cfg.GetString("storage_class"); ok {
		config.StorageClass = storageClass
	}

	if storageSize, ok := cfg.GetString("storage_size"); ok {
		config.StorageSize = storageSize
	}

	if alertmanagerEnabled, ok := cfg.GetBool("alertmanager_enabled"); ok {
		config.AlertmanagerEnabled = alertmanagerEnabled
	}

	if grafanaEnabled, ok := cfg.GetBool("grafana_enabled"); ok {
		config.GrafanaEnabled = grafanaEnabled
	}

	if nodeExporterEnabled, ok := cfg.GetBool("node_exporter_enabled"); ok {
		config.NodeExporterEnabled = nodeExporterEnabled
	}

	if kubeStateMetricsEnabled, ok := cfg.GetBool("kube_state_metrics_enabled"); ok {
		config.KubeStateMetricsEnabled = kubeStateMetricsEnabled
	}

	if scrapeInterval, ok := cfg.GetString("scrape_interval"); ok {
		config.ScrapeInterval = scrapeInterval
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

// Validate validates the Prometheus configuration
func (c *Config) Validate() error {
	if c.RetentionDays < 1 {
		return fmt.Errorf("retention_days must be at least 1")
	}

	if c.IngressEnabled && c.IngressHost == "" {
		return fmt.Errorf("ingress_host is required when ingress is enabled")
	}

	return nil
}

// GetPrometheusEndpoint returns the Prometheus endpoint URL for internal cluster access
func (c *Config) GetPrometheusEndpoint() string {
	return fmt.Sprintf("http://kube-prometheus-stack-prometheus.%s.svc.cluster.local:9090", c.Namespace)
}

// GetAlertmanagerEndpoint returns the Alertmanager endpoint URL for internal cluster access
func (c *Config) GetAlertmanagerEndpoint() string {
	return fmt.Sprintf("http://kube-prometheus-stack-alertmanager.%s.svc.cluster.local:9093", c.Namespace)
}
