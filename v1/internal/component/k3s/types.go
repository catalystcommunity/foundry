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
	return nil, fmt.Errorf("not implemented")
}

// Uninstall removes the K3s component
func (c *Component) Uninstall(ctx context.Context) error {
	return fmt.Errorf("not implemented")
}

// Dependencies returns the components that K3s depends on
func (c *Component) Dependencies() []string {
	return []string{"openbao", "dns", "zot"}
}

// Config holds the K3s installation configuration
type Config struct {
	// Version is the K3s version to install (e.g., "v1.28.5+k3s1")
	Version string

	// VIP is the virtual IP for the cluster
	VIP string

	// Interface is the network interface for kube-vip
	Interface string

	// ClusterToken is the token for joining control plane nodes
	ClusterToken string

	// AgentToken is the token for joining worker nodes
	AgentToken string

	// TLSSANs are additional TLS SANs for the API server certificate
	TLSSANs []string

	// DisableComponents are K3s components to disable (e.g., traefik, servicelb)
	DisableComponents []string

	// RegistryConfig is the path to the registries.yaml file (optional)
	RegistryConfig string

	// ClusterInit indicates if this is the first control plane node (for HA)
	ClusterInit bool

	// ServerURL is the URL of an existing control plane (for joining additional nodes)
	ServerURL string

	// DNSServers are the DNS servers to configure for K3s and the node
	DNSServers []string
}

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
