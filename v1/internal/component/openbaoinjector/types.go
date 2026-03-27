package openbaoinjector

import (
	"context"
	"fmt"

	"github.com/catalystcommunity/foundry/v1/internal/component"
	"github.com/catalystcommunity/foundry/v1/internal/helm"
)

// HelmClient defines the Helm operations needed for the OpenBao injector
type HelmClient interface {
	AddRepo(ctx context.Context, opts helm.RepoAddOptions) error
	Install(ctx context.Context, opts helm.InstallOptions) error
	Upgrade(ctx context.Context, opts helm.UpgradeOptions) error
	Uninstall(ctx context.Context, opts helm.UninstallOptions) error
	List(ctx context.Context, namespace string) ([]helm.Release, error)
}

// Component implements the component.Component interface for the OpenBao agent injector
type Component struct {
	helmClient HelmClient
}

// NewComponent creates a new OpenBao injector component instance
func NewComponent(helmClient HelmClient) *Component {
	return &Component{helmClient: helmClient}
}

// Name returns the component name
func (c *Component) Name() string {
	return "openbao-injector"
}

// Dependencies returns the components this depends on
func (c *Component) Dependencies() []string {
	return []string{"openbao", "k3s"}
}

// Install installs the OpenBao agent injector via Helm
func (c *Component) Install(ctx context.Context, cfg component.ComponentConfig) error {
	config, err := ParseConfig(cfg)
	if err != nil {
		return fmt.Errorf("parse config: %w", err)
	}
	return Install(ctx, c.helmClient, config)
}

// Upgrade upgrades the OpenBao agent injector
func (c *Component) Upgrade(ctx context.Context, cfg component.ComponentConfig) error {
	return fmt.Errorf("upgrade not yet implemented")
}

// Status returns the current status of the OpenBao agent injector
func (c *Component) Status(ctx context.Context) (*component.ComponentStatus, error) {
	if c.helmClient == nil {
		return &component.ComponentStatus{
			Installed: false,
			Healthy:   false,
			Message:   "helm client not initialized",
		}, nil
	}

	releases, err := c.helmClient.List(ctx, DefaultNamespace)
	if err != nil {
		return &component.ComponentStatus{
			Installed: false,
			Healthy:   false,
			Message:   fmt.Sprintf("failed to list releases: %v", err),
		}, nil
	}

	for _, rel := range releases {
		if rel.Name == ReleaseName {
			healthy := rel.Status == "deployed"
			msg := fmt.Sprintf("release status: %s", rel.Status)
			if healthy {
				msg = "injector webhook running"
			}
			return &component.ComponentStatus{
				Installed: true,
				Version:   rel.AppVersion,
				Healthy:   healthy,
				Message:   msg,
			}, nil
		}
	}

	return &component.ComponentStatus{
		Installed: false,
		Healthy:   false,
		Message:   "release not found",
	}, nil
}

// Uninstall removes the OpenBao agent injector
func (c *Component) Uninstall(ctx context.Context) error {
	return fmt.Errorf("uninstall not yet implemented")
}

// DefaultConfig returns a Config with sensible defaults
func DefaultConfig() *Config {
	return &Config{
		Version:           "0.26.2",
		Namespace:         DefaultNamespace,
		ExternalVaultAddr: "",
	}
}

// ParseConfig parses a ComponentConfig into an openbaoinjector Config
func ParseConfig(cfg component.ComponentConfig) (*Config, error) {
	config := DefaultConfig()

	if version, ok := cfg.GetString("version"); ok {
		config.Version = version
	}
	if namespace, ok := cfg.GetString("namespace"); ok {
		config.Namespace = namespace
	}
	if addr, ok := cfg.GetString("external_vault_addr"); ok {
		config.ExternalVaultAddr = addr
	}

	if config.ExternalVaultAddr == "" {
		return nil, fmt.Errorf("external_vault_addr is required — set it to your OpenBao address (e.g. http://100.81.89.62:8200)")
	}

	return config, nil
}
