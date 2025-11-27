package gatewayapi

import (
	"context"
	"fmt"

	"github.com/catalystcommunity/foundry/v1/internal/component"
	"github.com/catalystcommunity/foundry/v1/internal/k8s"
)

// K8sClient defines the Kubernetes operations needed for Gateway API
type K8sClient interface {
	// Clientset returns the underlying clientset for CRD checks
	Clientset() interface{}
}

// Component implements the component.Component interface for Gateway API CRDs
type Component struct {
	k8sClient *k8s.Client
}

// NewComponent creates a new Gateway API component instance
func NewComponent(k8sClient *k8s.Client) *Component {
	return &Component{
		k8sClient: k8sClient,
	}
}

// Name returns the component name
func (c *Component) Name() string {
	return "gateway-api"
}

// Install installs the Gateway API CRDs
func (c *Component) Install(ctx context.Context, cfg component.ComponentConfig) error {
	config, err := ParseConfig(cfg)
	if err != nil {
		return fmt.Errorf("parse config: %w", err)
	}

	return Install(ctx, c.k8sClient, config)
}

// Upgrade upgrades the Gateway API CRDs to a new version
func (c *Component) Upgrade(ctx context.Context, cfg component.ComponentConfig) error {
	// Upgrading CRDs is the same as installing - kubectl apply is idempotent
	return c.Install(ctx, cfg)
}

// Status returns the current status of the Gateway API CRDs
func (c *Component) Status(ctx context.Context) (*component.ComponentStatus, error) {
	if c.k8sClient == nil {
		return &component.ComponentStatus{
			Installed: false,
			Healthy:   false,
			Message:   "k8s client not initialized",
		}, nil
	}

	// Check if Gateway API CRDs are installed
	installed, version, err := CheckCRDsInstalled(ctx, c.k8sClient)
	if err != nil {
		return &component.ComponentStatus{
			Installed: false,
			Healthy:   false,
			Message:   fmt.Sprintf("failed to check CRDs: %v", err),
		}, nil
	}

	if !installed {
		return &component.ComponentStatus{
			Installed: false,
			Healthy:   false,
			Message:   "Gateway API CRDs not installed",
		}, nil
	}

	return &component.ComponentStatus{
		Installed: true,
		Version:   version,
		Healthy:   true,
		Message:   "Gateway API CRDs installed",
	}, nil
}

// Uninstall removes the Gateway API CRDs
func (c *Component) Uninstall(ctx context.Context) error {
	return fmt.Errorf("uninstall not yet implemented")
}

// Dependencies returns the list of components that Gateway API depends on
func (c *Component) Dependencies() []string {
	return []string{"k3s"} // Gateway API CRDs require a running Kubernetes cluster
}

// Config holds configuration for Gateway API installation
type Config struct {
	// Version is the Gateway API version to install (e.g., "v1.3.0")
	Version string `json:"version" yaml:"version"`
}

// DefaultConfig returns a Config with sensible defaults
func DefaultConfig() *Config {
	return &Config{
		Version: "v1.3.0",
	}
}

// ParseConfig parses a ComponentConfig into a Gateway API Config
func ParseConfig(cfg component.ComponentConfig) (*Config, error) {
	config := DefaultConfig()

	if version, ok := cfg.GetString("version"); ok {
		config.Version = version
	}

	return config, nil
}
