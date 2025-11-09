package config

import (
	"fmt"
	"net"
	"strings"
)

// All type definitions (NetworkConfig, DHCPRange, DNSConfig, DNSZone, ClusterConfig, NodeConfig,
// ComponentMap, ComponentConfig, ObsConfig, PrometheusConfig, LokiConfig, GrafanaConfig,
// StorageConfig, TrueNASConfig, and Config) are generated from CSIL in types.gen.go
// Validation methods below extend the generated types

// Valid node roles
const (
	NodeRoleControlPlane = "control-plane"
	NodeRoleWorker       = "worker"
)

// Validate performs validation on the Config struct
func (c *Config) Validate() error {
	if c.Network != nil {
		if err := c.Network.Validate(); err != nil {
			return fmt.Errorf("network validation failed: %w", err)
		}
	}

	if c.DNS != nil {
		if err := c.DNS.Validate(); err != nil {
			return fmt.Errorf("dns validation failed: %w", err)
		}
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

	// Cross-validation: K8s VIP must be unique (not in any host list)
	if c.Network != nil && c.Network.K8sVIP != "" {
		if err := c.validateK8sVIPUniqueness(); err != nil {
			return err
		}
	}

	return nil
}

// validateK8sVIPUniqueness ensures the K8s VIP is not used by any infrastructure host
func (c *Config) validateK8sVIPUniqueness() error {
	if c.Network == nil {
		return nil
	}

	vip := c.Network.K8sVIP

	// Check against all infrastructure host lists
	allHosts := append([]string{},
		c.Network.OpenBAOHosts...,
	)
	allHosts = append(allHosts, c.Network.DNSHosts...)
	allHosts = append(allHosts, c.Network.ZotHosts...)
	allHosts = append(allHosts, c.Network.TrueNASHosts...)

	for _, host := range allHosts {
		if host == vip {
			return fmt.Errorf("k8s_vip %q conflicts with infrastructure host IP", vip)
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

// Validate performs validation on NetworkConfig
func (n *NetworkConfig) Validate() error {
	// Validate gateway
	if n.Gateway == "" {
		return fmt.Errorf("gateway is required")
	}
	if ip := net.ParseIP(n.Gateway); ip == nil {
		return fmt.Errorf("gateway %q is not a valid IP address", n.Gateway)
	}

	// Validate netmask
	if n.Netmask == "" {
		return fmt.Errorf("netmask is required")
	}
	if ip := net.ParseIP(n.Netmask); ip == nil {
		return fmt.Errorf("netmask %q is not a valid IP address", n.Netmask)
	}

	// Validate DHCP range if provided
	if n.DHCPRange != nil {
		if err := n.DHCPRange.Validate(); err != nil {
			return fmt.Errorf("dhcp_range validation failed: %w", err)
		}
	}

	// Validate K8s VIP
	if n.K8sVIP == "" {
		return fmt.Errorf("k8s_vip is required")
	}
	if ip := net.ParseIP(n.K8sVIP); ip == nil {
		return fmt.Errorf("k8s_vip %q is not a valid IP address", n.K8sVIP)
	}

	// Validate OpenBAO hosts
	if len(n.OpenBAOHosts) == 0 {
		return fmt.Errorf("at least one openbao_hosts entry is required")
	}
	for i, host := range n.OpenBAOHosts {
		if ip := net.ParseIP(host); ip == nil {
			return fmt.Errorf("openbao_hosts[%d] %q is not a valid IP address", i, host)
		}
	}

	// Validate DNS hosts
	if len(n.DNSHosts) == 0 {
		return fmt.Errorf("at least one dns_hosts entry is required")
	}
	for i, host := range n.DNSHosts {
		if ip := net.ParseIP(host); ip == nil {
			return fmt.Errorf("dns_hosts[%d] %q is not a valid IP address", i, host)
		}
	}

	// Validate Zot hosts
	if len(n.ZotHosts) == 0 {
		return fmt.Errorf("at least one zot_hosts entry is required")
	}
	for i, host := range n.ZotHosts {
		if ip := net.ParseIP(host); ip == nil {
			return fmt.Errorf("zot_hosts[%d] %q is not a valid IP address", i, host)
		}
	}

	// Validate TrueNAS hosts (optional)
	for i, host := range n.TrueNASHosts {
		if ip := net.ParseIP(host); ip == nil {
			return fmt.Errorf("truenas_hosts[%d] %q is not a valid IP address", i, host)
		}
	}

	return nil
}

// Validate performs validation on DHCPRange
func (d *DHCPRange) Validate() error {
	if d.Start == "" {
		return fmt.Errorf("start is required")
	}
	if ip := net.ParseIP(d.Start); ip == nil {
		return fmt.Errorf("start %q is not a valid IP address", d.Start)
	}

	if d.End == "" {
		return fmt.Errorf("end is required")
	}
	if ip := net.ParseIP(d.End); ip == nil {
		return fmt.Errorf("end %q is not a valid IP address", d.End)
	}

	return nil
}

// Validate performs validation on DNSConfig
func (d *DNSConfig) Validate() error {
	// At least one infrastructure zone required
	if len(d.InfrastructureZones) == 0 {
		return fmt.Errorf("at least one infrastructure zone is required")
	}

	// At least one kubernetes zone required
	if len(d.KubernetesZones) == 0 {
		return fmt.Errorf("at least one kubernetes zone is required")
	}

	// Validate all infrastructure zones
	for i, zone := range d.InfrastructureZones {
		if err := zone.Validate(); err != nil {
			return fmt.Errorf("infrastructure_zones[%d] validation failed: %w", i, err)
		}
	}

	// Validate all kubernetes zones
	for i, zone := range d.KubernetesZones {
		if err := zone.Validate(); err != nil {
			return fmt.Errorf("kubernetes_zones[%d] validation failed: %w", i, err)
		}
	}

	// Validate zone name uniqueness across both lists
	zoneNames := make(map[string]bool)
	for _, zone := range d.InfrastructureZones {
		if zoneNames[zone.Name] {
			return fmt.Errorf("duplicate zone name: %q", zone.Name)
		}
		zoneNames[zone.Name] = true
	}
	for _, zone := range d.KubernetesZones {
		if zoneNames[zone.Name] {
			return fmt.Errorf("duplicate zone name: %q", zone.Name)
		}
		zoneNames[zone.Name] = true
	}

	// Validate backend
	if d.Backend == "" {
		return fmt.Errorf("backend is required")
	}

	// Validate API key (can be secret reference)
	if d.APIKey == "" {
		return fmt.Errorf("api_key is required")
	}

	return nil
}

// Validate performs validation on DNSZone
func (z *DNSZone) Validate() error {
	if z.Name == "" {
		return fmt.Errorf("zone name is required")
	}

	// .local zones must not be public
	if strings.HasSuffix(z.Name, ".local") && z.Public {
		return fmt.Errorf("zone %q ends with .local but is marked as public (must be private)", z.Name)
	}

	// Public zones must have public_cname
	if z.Public && (z.PublicCNAME == nil || *z.PublicCNAME == "") {
		return fmt.Errorf("zone %q is marked as public but missing public_cname", z.Name)
	}

	return nil
}
