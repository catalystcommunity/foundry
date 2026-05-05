package config

import (
	"fmt"
	"os"
	"strconv"

	"github.com/catalystcommunity/foundry/v1/internal/host"
)

const (
	OpenBAODefaultPort   = 8200
	OpenBAOAddrEnvVar    = "OPENBAO_ADDR"
	OpenBAOClientAddrEnv = "OPENBAO_CLIENT_ADDR"
)

// GetHostsByRole returns all hosts that have the specified role
func (c *Config) GetHostsByRole(role string) []*host.Host {
	var hosts []*host.Host
	for _, h := range c.Hosts {
		if h.HasRole(role) {
			hosts = append(hosts, h)
		}
	}
	return hosts
}

// GetHostByHostname returns a host by its hostname
func (c *Config) GetHostByHostname(hostname string) (*host.Host, error) {
	for _, h := range c.Hosts {
		if h.Hostname == hostname {
			return h, nil
		}
	}
	return nil, fmt.Errorf("host not found: %s", hostname)
}

// GetOpenBAOHosts returns all hosts with the openbao role
func (c *Config) GetOpenBAOHosts() []*host.Host {
	return c.GetHostsByRole(host.RoleOpenBAO)
}

// GetDNSHosts returns all hosts with the dns role
func (c *Config) GetDNSHosts() []*host.Host {
	return c.GetHostsByRole(host.RoleDNS)
}

// GetZotHosts returns all hosts with the zot role
func (c *Config) GetZotHosts() []*host.Host {
	return c.GetHostsByRole(host.RoleZot)
}

// GetClusterControlPlaneHosts returns all hosts with the cluster-control-plane role
func (c *Config) GetClusterControlPlaneHosts() []*host.Host {
	return c.GetHostsByRole(host.RoleClusterControlPlane)
}

// GetClusterWorkerHosts returns all hosts with the cluster-worker role
func (c *Config) GetClusterWorkerHosts() []*host.Host {
	return c.GetHostsByRole(host.RoleClusterWorker)
}

// GetClusterHosts returns all hosts with either cluster-control-plane or cluster-worker roles
func (c *Config) GetClusterHosts() []*host.Host {
	seen := make(map[string]bool)
	var hosts []*host.Host

	// Add control plane hosts
	for _, h := range c.GetClusterControlPlaneHosts() {
		if !seen[h.Hostname] {
			hosts = append(hosts, h)
			seen[h.Hostname] = true
		}
	}

	// Add worker hosts
	for _, h := range c.GetClusterWorkerHosts() {
		if !seen[h.Hostname] {
			hosts = append(hosts, h)
			seen[h.Hostname] = true
		}
	}

	return hosts
}

// GetHostAddresses returns IP addresses for all hosts with the specified role
func (c *Config) GetHostAddresses(role string) []string {
	hosts := c.GetHostsByRole(role)
	addresses := make([]string, len(hosts))
	for i, h := range hosts {
		addresses[i] = h.Address
	}
	return addresses
}

// GetPrimaryOpenBAOHost returns the first OpenBAO host (for single-host deployments)
func (c *Config) GetPrimaryOpenBAOHost() (*host.Host, error) {
	hosts := c.GetOpenBAOHosts()
	if len(hosts) == 0 {
		return nil, fmt.Errorf("no hosts with openbao role found")
	}
	return hosts[0], nil
}

// GetPrimaryOpenBAOAddress returns the IP address of the first OpenBAO host
func (c *Config) GetPrimaryOpenBAOAddress() (string, error) {
	h, err := c.GetPrimaryOpenBAOHost()
	if err != nil {
		return "", err
	}
	return h.Address, nil
}

// GetPrimaryOpenBAOAddr returns the full address with port for the first OpenBAO host
func (c *Config) GetPrimaryOpenBAOAddr() (string, error) {
	h, err := c.GetPrimaryOpenBAOHost()
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%s:%d", h.Address, OpenBAODefaultPort), nil
}

// GetPrimaryOpenBAOURL returns the full HTTP URL for the first OpenBAO host
// It first checks the OPENBAO_ADDR environment variable, then falls back to config
func (c *Config) GetPrimaryOpenBAOURL() (string, error) {
	if addr := os.Getenv(OpenBAOAddrEnvVar); addr != "" {
		return addr, nil
	}
	addr, err := c.GetPrimaryOpenBAOAddr()
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("http://%s", addr), nil
}

// GetOpenBAOClientAddr returns the client address for OpenBAO connections
// It first checks OPENBAO_CLIENT_ADDR env var, then falls back to config address
func (c *Config) GetOpenBAOClientAddr() (string, error) {
	if addr := os.Getenv(OpenBAOClientAddrEnv); addr != "" {
		return addr, nil
	}
	return c.GetPrimaryOpenBAOAddr()
}

// GetOpenBAOPort returns the OpenBAO port, first checking OPENBAO_PORT env var
func (c *Config) GetOpenBAOPort() int {
	if port := os.Getenv("OPENBAO_PORT"); port != "" {
		if p, err := strconv.Atoi(port); err == nil {
			return p
		}
	}
	return OpenBAODefaultPort
}

// GetPrimaryDNSHost returns the first DNS host
func (c *Config) GetPrimaryDNSHost() (*host.Host, error) {
	hosts := c.GetDNSHosts()
	if len(hosts) == 0 {
		return nil, fmt.Errorf("no hosts with dns role found")
	}
	return hosts[0], nil
}

// GetPrimaryDNSAddress returns the IP address of the first DNS host
func (c *Config) GetPrimaryDNSAddress() (string, error) {
	h, err := c.GetPrimaryDNSHost()
	if err != nil {
		return "", err
	}
	return h.Address, nil
}

// GetPrimaryZotHost returns the first Zot host
func (c *Config) GetPrimaryZotHost() (*host.Host, error) {
	hosts := c.GetZotHosts()
	if len(hosts) == 0 {
		return nil, fmt.Errorf("no hosts with zot role found")
	}
	return hosts[0], nil
}

// GetPrimaryZotAddress returns the IP address of the first Zot host
func (c *Config) GetPrimaryZotAddress() (string, error) {
	h, err := c.GetPrimaryZotHost()
	if err != nil {
		return "", err
	}
	return h.Address, nil
}
