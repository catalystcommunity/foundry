package config

import (
	"fmt"
	"strings"
)

// Config represents the main stack configuration
type Config struct {
	Version       string         `yaml:"version"`
	Cluster       ClusterConfig  `yaml:"cluster"`
	Components    ComponentMap   `yaml:"components"`
	Observability *ObsConfig     `yaml:"observability,omitempty"`
	Storage       *StorageConfig `yaml:"storage,omitempty"`
}

// ClusterConfig defines the cluster settings
type ClusterConfig struct {
	Name   string       `yaml:"name"`
	Domain string       `yaml:"domain"`
	Nodes  []NodeConfig `yaml:"nodes"`
}

// NodeConfig defines a single node in the cluster
type NodeConfig struct {
	Hostname string `yaml:"hostname"`
	Role     string `yaml:"role"` // control-plane, worker
}

// ComponentMap is a map of component names to their configurations
type ComponentMap map[string]ComponentConfig

// ComponentConfig defines configuration for a single component
type ComponentConfig struct {
	Version string                 `yaml:"version,omitempty"`
	Hosts   []string               `yaml:"hosts,omitempty"`
	Config  map[string]interface{} `yaml:",inline"`
}

// ObsConfig defines observability settings
type ObsConfig struct {
	Prometheus *PrometheusConfig `yaml:"prometheus,omitempty"`
	Loki       *LokiConfig       `yaml:"loki,omitempty"`
	Grafana    *GrafanaConfig    `yaml:"grafana,omitempty"`
}

// PrometheusConfig defines Prometheus-specific settings
type PrometheusConfig struct {
	Retention string `yaml:"retention,omitempty"`
}

// LokiConfig defines Loki-specific settings
type LokiConfig struct {
	Retention string `yaml:"retention,omitempty"`
}

// GrafanaConfig defines Grafana-specific settings
type GrafanaConfig struct {
	AdminPassword string `yaml:"admin_password,omitempty"`
}

// StorageConfig defines storage backend configuration
type StorageConfig struct {
	TrueNAS *TrueNASConfig `yaml:"truenas,omitempty"`
}

// TrueNASConfig defines TrueNAS-specific settings
type TrueNASConfig struct {
	APIURL string `yaml:"api_url"`
	APIKey string `yaml:"api_key"`
}

// Valid node roles
const (
	NodeRoleControlPlane = "control-plane"
	NodeRoleWorker       = "worker"
)

// Validate performs validation on the Config struct
func (c *Config) Validate() error {
	if c.Version == "" {
		return fmt.Errorf("version is required")
	}

	if err := c.Cluster.Validate(); err != nil {
		return fmt.Errorf("cluster validation failed: %w", err)
	}

	if len(c.Components) == 0 {
		return fmt.Errorf("at least one component must be defined")
	}

	for name, comp := range c.Components {
		if err := comp.Validate(); err != nil {
			return fmt.Errorf("component %s validation failed: %w", name, err)
		}
	}

	if c.Observability != nil {
		if err := c.Observability.Validate(); err != nil {
			return fmt.Errorf("observability validation failed: %w", err)
		}
	}

	if c.Storage != nil {
		if err := c.Storage.Validate(); err != nil {
			return fmt.Errorf("storage validation failed: %w", err)
		}
	}

	return nil
}

// Validate performs validation on ClusterConfig
func (c *ClusterConfig) Validate() error {
	if c.Name == "" {
		return fmt.Errorf("cluster name is required")
	}

	if c.Domain == "" {
		return fmt.Errorf("cluster domain is required")
	}

	if len(c.Nodes) == 0 {
		return fmt.Errorf("at least one node must be defined")
	}

	for i, node := range c.Nodes {
		if err := node.Validate(); err != nil {
			return fmt.Errorf("node %d validation failed: %w", i, err)
		}
	}

	// At least one control-plane node is required
	hasControlPlane := false
	for _, node := range c.Nodes {
		if node.Role == NodeRoleControlPlane {
			hasControlPlane = true
			break
		}
	}
	if !hasControlPlane {
		return fmt.Errorf("at least one control-plane node is required")
	}

	return nil
}

// Validate performs validation on NodeConfig
func (n *NodeConfig) Validate() error {
	if n.Hostname == "" {
		return fmt.Errorf("node hostname is required")
	}

	if n.Role == "" {
		return fmt.Errorf("node role is required")
	}

	// Validate role is one of the allowed values
	validRole := n.Role == NodeRoleControlPlane || n.Role == NodeRoleWorker
	if !validRole {
		return fmt.Errorf("invalid node role %q: must be %s or %s",
			n.Role, NodeRoleControlPlane, NodeRoleWorker)
	}

	return nil
}

// Validate performs validation on ComponentConfig
func (c *ComponentConfig) Validate() error {
	// Version format validation could be added here if needed
	// For now, we just accept any non-empty string or omitted version

	// Hosts validation - if specified, must not be empty
	if c.Hosts != nil && len(c.Hosts) == 0 {
		return fmt.Errorf("if hosts is specified, it must not be empty")
	}

	// Validate each host is not empty
	for i, host := range c.Hosts {
		if strings.TrimSpace(host) == "" {
			return fmt.Errorf("host %d is empty or whitespace", i)
		}
	}

	return nil
}

// Validate performs validation on ObsConfig
func (o *ObsConfig) Validate() error {
	// Observability config is optional, but if present, validate its fields
	// Currently no specific validation needed for nested configs
	return nil
}

// Validate performs validation on StorageConfig
func (s *StorageConfig) Validate() error {
	if s.TrueNAS != nil {
		if err := s.TrueNAS.Validate(); err != nil {
			return fmt.Errorf("truenas validation failed: %w", err)
		}
	}
	return nil
}

// Validate performs validation on TrueNASConfig
func (t *TrueNASConfig) Validate() error {
	if t.APIURL == "" {
		return fmt.Errorf("truenas api_url is required")
	}

	if t.APIKey == "" {
		return fmt.Errorf("truenas api_key is required")
	}

	return nil
}
