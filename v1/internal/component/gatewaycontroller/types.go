package gatewaycontroller

import (
	"context"
	"fmt"

	"github.com/catalystcommunity/foundry/v1/internal/component"
	"github.com/catalystcommunity/foundry/v1/internal/helm"
)

const (
	releaseName = "gateway-controller"
)

// HelmClient defines the Helm operations needed for the gateway controller.
type HelmClient interface {
	Install(ctx context.Context, opts helm.InstallOptions) error
	Upgrade(ctx context.Context, opts helm.UpgradeOptions) error
	Uninstall(ctx context.Context, opts helm.UninstallOptions) error
	List(ctx context.Context, namespace string) ([]helm.Release, error)
}

// Component implements component.Component for the foundry gateway controller.
type Component struct {
	helmClient HelmClient
}

// NewComponent creates a new gateway controller component. The k8sClient
// parameter is accepted for signature parity with other components but is not
// currently used (readiness is handled by Helm --wait).
func NewComponent(helmClient HelmClient, _ interface{}) *Component {
	return &Component{helmClient: helmClient}
}

// Name returns the component name.
func (c *Component) Name() string {
	return "gateway-controller"
}

// Install installs the gateway controller via its embedded Helm chart.
func (c *Component) Install(ctx context.Context, cfg component.ComponentConfig) error {
	config, err := ParseConfig(cfg)
	if err != nil {
		return fmt.Errorf("parse config: %w", err)
	}
	return Install(ctx, c.helmClient, config)
}

// Upgrade re-runs Install, which upgrades the release if it already exists.
func (c *Component) Upgrade(ctx context.Context, cfg component.ComponentConfig) error {
	return c.Install(ctx, cfg)
}

// Status reports whether the gateway controller release is installed and healthy.
func (c *Component) Status(ctx context.Context) (*component.ComponentStatus, error) {
	if c.helmClient == nil {
		return &component.ComponentStatus{Message: "helm client not initialized"}, nil
	}

	cfg := DefaultConfig()
	releases, err := c.helmClient.List(ctx, cfg.Namespace)
	if err != nil {
		return &component.ComponentStatus{Message: fmt.Sprintf("failed to list releases: %v", err)}, nil
	}
	for i := range releases {
		if releases[i].Name == releaseName {
			healthy := releases[i].Status == "deployed"
			return &component.ComponentStatus{
				Installed: true,
				Version:   releases[i].AppVersion,
				Healthy:   healthy,
				Message:   fmt.Sprintf("release status: %s", releases[i].Status),
			}, nil
		}
	}
	return &component.ComponentStatus{Message: "gateway-controller release not found"}, nil
}

// Uninstall removes the gateway controller release.
func (c *Component) Uninstall(ctx context.Context) error {
	if c.helmClient == nil {
		return fmt.Errorf("helm client cannot be nil")
	}
	return c.helmClient.Uninstall(ctx, helm.UninstallOptions{
		ReleaseName: releaseName,
		Namespace:   DefaultConfig().Namespace,
	})
}

// Dependencies returns the components this one depends on.
func (c *Component) Dependencies() []string {
	return []string{"contour"} // needs the Contour Gateway + Envoy service to manage
}

// DefaultConfig returns a Config with sensible defaults.
func DefaultConfig() *Config {
	return &Config{
		Namespace:        "foundry-system",
		ImageRepository:  "containers.catalystsquad.com/public/catalystcommunity/foundry",
		ImageTag:         "", // empty -> chart appVersion
		GatewayName:      "contour",
		GatewayNamespace: "projectcontour",
		EnvoyService:     "contour-envoy",
		NetworkPolicy:    "contour-envoy",
		Interval:         "15s",
		ReplicaCount:     1,
		Values:           make(map[string]interface{}),
	}
}

// ParseConfig parses a ComponentConfig into a gateway controller Config.
func ParseConfig(cfg component.ComponentConfig) (*Config, error) {
	config := DefaultConfig()

	if v, ok := cfg.GetString("namespace"); ok {
		config.Namespace = v
	}
	if v, ok := cfg.GetString("image_repository"); ok {
		config.ImageRepository = v
	}
	if v, ok := cfg.GetString("image_tag"); ok {
		config.ImageTag = v
	}
	if v, ok := cfg.GetString("gateway_name"); ok {
		config.GatewayName = v
	}
	if v, ok := cfg.GetString("gateway_namespace"); ok {
		config.GatewayNamespace = v
	}
	if v, ok := cfg.GetString("envoy_service"); ok {
		config.EnvoyService = v
	}
	if v, ok := cfg.GetString("network_policy"); ok {
		config.NetworkPolicy = v
	}
	if v, ok := cfg.GetString("interval"); ok {
		config.Interval = v
	}
	if v, ok := cfg.GetInt("replica_count"); ok {
		config.ReplicaCount = uint64(v)
	}
	if v, ok := cfg.GetMap("values"); ok {
		config.Values = v
	}

	return config, nil
}
