package certmanager

import (
	"context"

	"github.com/catalystcommunity/foundry/v1/internal/component"
)

// Config type is generated from CSIL in types.gen.go
// This file extends the generated type with methods

// Component implements the component.Component interface for cert-manager
type Component struct {
	config *Config
}

// NewComponent creates a new cert-manager component with the given configuration
func NewComponent(cfg *Config) *Component {
	if cfg == nil {
		cfg = &Config{}
	}

	// Set defaults
	if cfg.Namespace == "" {
		cfg.Namespace = "cert-manager"
	}
	if cfg.Version == "" {
		cfg.Version = "v1.14.2" // Latest stable as of implementation
	}
	if cfg.DefaultIssuerType == "" {
		cfg.DefaultIssuerType = "self-signed"
	}
	if cfg.ACMEServer == "" {
		cfg.ACMEServer = "https://acme-v02.api.letsencrypt.org/directory"
	}
	cfg.InstallCRDs = true // Always install CRDs
	// ServiceMonitorEnabled defaults to true (set in ParseConfig or here if not set)
	// Note: bool zero value is false, so we need to handle this carefully
	// The CSIL default will be handled in ParseConfig

	return &Component{
		config: cfg,
	}
}

// Name returns the component name
func (c *Component) Name() string {
	return "cert-manager"
}

// Dependencies returns the list of components that cert-manager depends on
func (c *Component) Dependencies() []string {
	// cert-manager requires a running Kubernetes cluster
	return []string{"k3s"}
}

// Install installs the cert-manager component
func (c *Component) Install(ctx context.Context, cfg component.ComponentConfig) error {
	// Installation logic is in install.go
	return Install(ctx, c.config, cfg)
}

// Upgrade upgrades the cert-manager component
func (c *Component) Upgrade(ctx context.Context, cfg component.ComponentConfig) error {
	// For now, upgrade uses the same logic as install (Helm handles this)
	return Install(ctx, c.config, cfg)
}

// Status returns the current status of the cert-manager component
func (c *Component) Status(ctx context.Context) (*component.ComponentStatus, error) {
	// Status logic is in install.go
	return GetStatus(ctx, c.config)
}

// Uninstall removes the cert-manager component
func (c *Component) Uninstall(ctx context.Context) error {
	// Uninstall logic is in install.go
	return Uninstall(ctx, c.config)
}

// Config returns the component configuration
func (c *Component) Config() interface{} {
	return c.config
}

// ParseConfig parses component configuration from a map
func ParseConfig(data map[string]interface{}) (*Config, error) {
	cfg := &Config{
		ServiceMonitorEnabled: true, // Default to true
	}

	if namespace, ok := data["namespace"].(string); ok {
		cfg.Namespace = namespace
	}
	if version, ok := data["version"].(string); ok {
		cfg.Version = version
	}
	if createDefaultIssuer, ok := data["create_default_issuer"].(bool); ok {
		cfg.CreateDefaultIssuer = createDefaultIssuer
	}
	if defaultIssuerType, ok := data["default_issuer_type"].(string); ok {
		cfg.DefaultIssuerType = defaultIssuerType
	}
	if acmeEmail, ok := data["acme_email"].(string); ok {
		cfg.ACMEEmail = acmeEmail
	}
	if acmeServer, ok := data["acme_server"].(string); ok {
		cfg.ACMEServer = acmeServer
	}
	if installCRDs, ok := data["install_crds"].(bool); ok {
		cfg.InstallCRDs = installCRDs
	}
	if serviceMonitorEnabled, ok := data["service_monitor_enabled"].(bool); ok {
		cfg.ServiceMonitorEnabled = serviceMonitorEnabled
	}

	return cfg, nil
}
