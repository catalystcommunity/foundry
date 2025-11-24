package config

import (
	"testing"

	"github.com/catalystcommunity/foundry/v1/internal/host"
)

func TestGetHostsByRole(t *testing.T) {
	cfg := &Config{
		Hosts: []*host.Host{
			{Hostname: "host1", Address: "10.0.0.1", Roles: []string{host.RoleOpenBAO, host.RoleDNS}},
			{Hostname: "host2", Address: "10.0.0.2", Roles: []string{host.RoleZot}},
			{Hostname: "host3", Address: "10.0.0.3", Roles: []string{host.RoleClusterControlPlane}},
		},
	}

	tests := []struct {
		name     string
		role     string
		expected int
	}{
		{"OpenBAO hosts", host.RoleOpenBAO, 1},
		{"DNS hosts", host.RoleDNS, 1},
		{"Zot hosts", host.RoleZot, 1},
		{"Cluster control plane hosts", host.RoleClusterControlPlane, 1},
		{"Non-existent role", "invalid-role", 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			hosts := cfg.GetHostsByRole(tt.role)
			if len(hosts) != tt.expected {
				t.Errorf("Expected %d hosts, got %d", tt.expected, len(hosts))
			}
		})
	}
}

func TestGetHostByHostname(t *testing.T) {
	cfg := &Config{
		Hosts: []*host.Host{
			{Hostname: "host1", Address: "10.0.0.1"},
			{Hostname: "host2", Address: "10.0.0.2"},
		},
	}

	tests := []struct {
		name        string
		hostname    string
		shouldExist bool
	}{
		{"Existing host", "host1", true},
		{"Non-existent host", "host3", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h, err := cfg.GetHostByHostname(tt.hostname)
			if tt.shouldExist {
				if err != nil {
					t.Errorf("Expected host to exist, got error: %v", err)
				}
				if h.Hostname != tt.hostname {
					t.Errorf("Expected hostname %s, got %s", tt.hostname, h.Hostname)
				}
			} else {
				if err == nil {
					t.Errorf("Expected error for non-existent host, got nil")
				}
			}
		})
	}
}

func TestGetOpenBAOHosts(t *testing.T) {
	cfg := &Config{
		Hosts: []*host.Host{
			{Hostname: "host1", Roles: []string{host.RoleOpenBAO}},
			{Hostname: "host2", Roles: []string{host.RoleDNS}},
		},
	}

	hosts := cfg.GetOpenBAOHosts()
	if len(hosts) != 1 {
		t.Errorf("Expected 1 OpenBAO host, got %d", len(hosts))
	}
	if hosts[0].Hostname != "host1" {
		t.Errorf("Expected hostname host1, got %s", hosts[0].Hostname)
	}
}

func TestGetClusterHosts(t *testing.T) {
	cfg := &Config{
		Hosts: []*host.Host{
			{Hostname: "host1", Roles: []string{host.RoleClusterControlPlane}},
			{Hostname: "host2", Roles: []string{host.RoleClusterWorker}},
			{Hostname: "host3", Roles: []string{host.RoleOpenBAO}},
			{Hostname: "host4", Roles: []string{host.RoleClusterControlPlane, host.RoleClusterWorker}},
		},
	}

	hosts := cfg.GetClusterHosts()

	// Should return host1, host2, and host4 (3 hosts total)
	if len(hosts) != 3 {
		t.Errorf("Expected 3 cluster hosts, got %d", len(hosts))
	}

	// Verify no duplicates (host4 has both roles but should only appear once)
	seen := make(map[string]bool)
	for _, h := range hosts {
		if seen[h.Hostname] {
			t.Errorf("Duplicate host in results: %s", h.Hostname)
		}
		seen[h.Hostname] = true
	}
}

func TestGetHostAddresses(t *testing.T) {
	cfg := &Config{
		Hosts: []*host.Host{
			{Hostname: "host1", Address: "10.0.0.1", Roles: []string{host.RoleOpenBAO}},
			{Hostname: "host2", Address: "10.0.0.2", Roles: []string{host.RoleOpenBAO}},
			{Hostname: "host3", Address: "10.0.0.3", Roles: []string{host.RoleDNS}},
		},
	}

	addresses := cfg.GetHostAddresses(host.RoleOpenBAO)
	if len(addresses) != 2 {
		t.Errorf("Expected 2 addresses, got %d", len(addresses))
	}

	expected := map[string]bool{"10.0.0.1": true, "10.0.0.2": true}
	for _, addr := range addresses {
		if !expected[addr] {
			t.Errorf("Unexpected address: %s", addr)
		}
	}
}
