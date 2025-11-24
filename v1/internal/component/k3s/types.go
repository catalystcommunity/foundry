package k3s

import (
	"context"
	"fmt"

	"github.com/catalystcommunity/foundry/v1/internal/component"
)

// Component implements the component.Component interface for K3s
type Component struct {
	// Connection dependencies will be injected during installation
}

// Name returns the component name
func (c *Component) Name() string {
	return "k3s"
}

// Install installs the K3s component
func (c *Component) Install(ctx context.Context, cfg component.ComponentConfig) error {
	// Implementation will be in install.go
	return fmt.Errorf("not implemented")
}

// Upgrade upgrades the K3s component
func (c *Component) Upgrade(ctx context.Context, cfg component.ComponentConfig) error {
	return fmt.Errorf("not implemented")
}

// Status returns the status of the K3s component
func (c *Component) Status(ctx context.Context) (*component.ComponentStatus, error) {
	// Status checking is implemented in cmd/foundry/commands/component/status.go
	// to avoid import cycles with config/ssh/secrets packages
	return nil, fmt.Errorf("not implemented - use 'foundry component status k3s' command")
}

// Uninstall removes the K3s component
func (c *Component) Uninstall(ctx context.Context) error {
	return fmt.Errorf("not implemented")
}

// Dependencies returns the components that K3s depends on
func (c *Component) Dependencies() []string {
	return []string{"openbao", "dns", "zot"}
}

// Config and RegistryConfig types are generated from CSIL in types.gen.go
// This file extends the generated types with parsing methods

// ParseConfig parses a component.ComponentConfig into a K3s Config
func ParseConfig(cfg component.ComponentConfig) (*Config, error) {
	config := &Config{
		DisableComponents: []string{"traefik", "servicelb"}, // Default disabled components
	}

	// Version
	if version, ok := cfg.GetString("version"); ok {
		config.Version = version
	}

	// VIP
	if vip, ok := cfg.GetString("vip"); ok {
		config.VIP = vip
	}

	// Interface
	if iface, ok := cfg.GetString("interface"); ok {
		config.Interface = iface
	}

	// Cluster token
	if token, ok := cfg.GetString("cluster_token"); ok {
		config.ClusterToken = token
	}

	// Agent token
	if token, ok := cfg.GetString("agent_token"); ok {
		config.AgentToken = token
	}

	// TLS SANs
	if sans, ok := cfg.GetStringSlice("tls_sans"); ok {
		config.TLSSANs = sans
	}

	// Disable components
	if disable, ok := cfg.GetStringSlice("disable"); ok {
		config.DisableComponents = disable
	}

	// Registry config
	if registryConfig, ok := cfg.GetString("registry_config"); ok {
		config.RegistryConfig = registryConfig
	}

	// Cluster init
	if clusterInit, ok := cfg.GetBool("cluster_init"); ok {
		config.ClusterInit = clusterInit
	}

	// Server URL
	if serverURL, ok := cfg.GetString("server_url"); ok {
		config.ServerURL = serverURL
	}

	// DNS servers
	if dnsServers, ok := cfg.GetStringSlice("dns_servers"); ok {
		config.DNSServers = dnsServers
	}

	return config, nil
}

// Validate validates the K3s configuration
func (c *Config) Validate() error {
	if c.VIP == "" {
		return fmt.Errorf("VIP is required")
	}

	if err := ValidateVIP(c.VIP); err != nil {
		return fmt.Errorf("VIP validation failed: %w", err)
	}

	// If joining an existing cluster, server URL is required
	if !c.ClusterInit && c.ServerURL == "" {
		return fmt.Errorf("server_url is required when cluster_init is false")
	}

	// Tokens are required for multi-node clusters
	if !c.ClusterInit {
		if c.ClusterToken == "" && c.AgentToken == "" {
			return fmt.Errorf("either cluster_token or agent_token is required when joining a cluster")
		}
	}

	return nil
}
