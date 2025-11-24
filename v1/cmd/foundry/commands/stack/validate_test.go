package stack

import (
	"testing"

	"github.com/catalystcommunity/foundry/v1/internal/component"
	"github.com/catalystcommunity/foundry/v1/internal/config"
	"github.com/catalystcommunity/foundry/v1/internal/host"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// setupValidateTestRegistry ensures the default registry has components for testing
func setupValidateTestRegistry(t *testing.T) {
	// Reset the default registry
	component.DefaultRegistry = component.NewRegistry()

	// Register mock components matching the stack installation order
	// We can use the same mock component from install_test.go
	component.DefaultRegistry.Register(&mockComponent{name: "openbao", dependencies: []string{}})
	component.DefaultRegistry.Register(&mockComponent{name: "dns", dependencies: []string{"openbao"}})
	component.DefaultRegistry.Register(&mockComponent{name: "zot", dependencies: []string{"dns", "openbao"}})
	component.DefaultRegistry.Register(&mockComponent{name: "k3s", dependencies: []string{"openbao", "dns", "zot"}})
	component.DefaultRegistry.Register(&mockComponent{name: "contour", dependencies: []string{"k3s"}})
	component.DefaultRegistry.Register(&mockComponent{name: "certmanager", dependencies: []string{"k3s"}})
}

// TestValidateConfigStructure tests basic config structure validation
func TestValidateConfigStructure(t *testing.T) {
	tests := []struct {
		name    string
		cfg     *config.Config
		wantErr bool
		errMsg  string
	}{
		{
			name: "valid config",
			cfg: &config.Config{
				Network: &config.NetworkConfig{
					Gateway:      "192.168.1.1",
					Netmask:      "255.255.255.0",
				},
				DNS: &config.DNSConfig{
					InfrastructureZones: []config.DNSZone{{Name: "infra.example.com"}},
					KubernetesZones:     []config.DNSZone{{Name: "k8s.example.com"}},
					Backend:             "sqlite",
					APIKey:              "test-key",
				},
				Cluster: config.ClusterConfig{
					Name:   "test-cluster",
					Domain: "example.com",
				},
				Components: config.ComponentMap{
					"openbao": {},
				},
			},
			wantErr: false,
		},
		{
			name: "invalid gateway IP",
			cfg: &config.Config{
				Network: &config.NetworkConfig{
					Gateway:      "not-an-ip",
					Netmask:      "255.255.255.0",
				},
				DNS: &config.DNSConfig{
					InfrastructureZones: []config.DNSZone{{Name: "infra.example.com"}},
					KubernetesZones:     []config.DNSZone{{Name: "k8s.example.com"}},
					Backend:             "sqlite",
					APIKey:              "test-key",
				},
				Cluster: config.ClusterConfig{
					Name:   "test-cluster",
					Domain: "example.com",
				},
				Components: config.ComponentMap{
					"openbao": {},
				},
			},
			wantErr: true,
			errMsg:  "not a valid IP",
		},
		{
			name: "invalid VIP (same as infrastructure host)",
			cfg: &config.Config{
				Network: &config.NetworkConfig{
					Gateway: "192.168.1.1",
					Netmask: "255.255.255.0",
				},
				DNS: &config.DNSConfig{
					InfrastructureZones: []config.DNSZone{{Name: "infra.example.com"}},
					KubernetesZones:     []config.DNSZone{{Name: "k8s.example.com"}},
					Backend:             "sqlite",
					APIKey:              "test-key",
				},
				Cluster: config.ClusterConfig{
					Name:   "test-cluster",
					Domain: "example.com",
					VIP:    "192.168.1.10", // Same as host below
				},
				Hosts: []*host.Host{
					{
						Hostname: "infra1.example.com",
						Address:  "192.168.1.10", // Conflicts with VIP
						Roles:    []string{host.RoleOpenBAO},
					},
				},
				Components: config.ComponentMap{
					"openbao": {},
					"k3s":     {},
				},
			},
			wantErr: true,
			errMsg:  "conflicts with host",
		},
		{
			name: "missing components",
			cfg: &config.Config{
				Network: &config.NetworkConfig{
					Gateway:      "192.168.1.1",
					Netmask:      "255.255.255.0",
				},
				DNS: &config.DNSConfig{
					InfrastructureZones: []config.DNSZone{{Name: "infra.example.com"}},
					KubernetesZones:     []config.DNSZone{{Name: "k8s.example.com"}},
					Backend:             "sqlite",
					APIKey:              "test-key",
				},
				Cluster: config.ClusterConfig{
					Name:   "test-cluster",
					Domain: "example.com",
					},
				Components: config.ComponentMap{}, // Empty!
			},
			wantErr: true,
			errMsg:  "at least one component must be defined",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateConfigStructure(tt.cfg)
			if tt.wantErr {
				assert.Error(t, err)
				if tt.errMsg != "" {
					assert.Contains(t, err.Error(), tt.errMsg)
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

// TestValidateSecretReferences tests secret reference syntax validation
func TestValidateSecretReferences(t *testing.T) {
	tests := []struct {
		name    string
		cfg     *config.Config
		wantErr bool
		errMsg  string
	}{
		{
			name: "valid secret references",
			cfg: &config.Config{
				Network: &config.NetworkConfig{
					Gateway:      "192.168.1.1",
					Netmask:      "255.255.255.0",
				},
				DNS: &config.DNSConfig{
					InfrastructureZones: []config.DNSZone{{Name: "infra.example.com"}},
					KubernetesZones:     []config.DNSZone{{Name: "k8s.example.com"}},
					Backend:             "sqlite",
					APIKey:              "${secret:foundry-core/dns:api_key}",
				},
				Cluster: config.ClusterConfig{
					Name:   "test-cluster",
					Domain: "example.com",
				},
				Components: config.ComponentMap{
					"openbao": {},
				},
			},
			wantErr: false,
		},
		{
			name: "invalid secret reference (missing colon)",
			cfg: &config.Config{
				Network: &config.NetworkConfig{
					Gateway:      "192.168.1.1",
					Netmask:      "255.255.255.0",
				},
				DNS: &config.DNSConfig{
					InfrastructureZones: []config.DNSZone{{Name: "infra.example.com"}},
					KubernetesZones:     []config.DNSZone{{Name: "k8s.example.com"}},
					Backend:             "sqlite",
					APIKey:              "${secret:invalid-format}", // Missing :key part
				},
				Cluster: config.ClusterConfig{
					Name:   "test-cluster",
					Domain: "example.com",
				},
				Components: config.ComponentMap{
					"openbao": {},
				},
			},
			wantErr: true,
			errMsg:  "invalid secret reference",
		},
		{
			name: "no secret references is valid",
			cfg: &config.Config{
				Network: &config.NetworkConfig{
					Gateway:      "192.168.1.1",
					Netmask:      "255.255.255.0",
				},
				DNS: &config.DNSConfig{
					InfrastructureZones: []config.DNSZone{{Name: "infra.example.com"}},
					KubernetesZones:     []config.DNSZone{{Name: "k8s.example.com"}},
					Backend:             "sqlite",
					APIKey:              "plain-text-key",
				},
				Cluster: config.ClusterConfig{
					Name:   "test-cluster",
					Domain: "example.com",
				},
				Components: config.ComponentMap{
					"openbao": {},
				},
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateSecretReferences(tt.cfg)
			if tt.wantErr {
				assert.Error(t, err)
				if tt.errMsg != "" {
					assert.Contains(t, err.Error(), tt.errMsg)
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

// TestValidateNetworkConfig tests network configuration validation
func TestValidateNetworkConfig(t *testing.T) {
	tests := []struct {
		name    string
		cfg     *config.Config
		wantErr bool
		errMsg  string
	}{
		{
			name: "valid network config",
			cfg: &config.Config{
				Network: &config.NetworkConfig{
					Gateway: "192.168.1.1",
					Netmask: "255.255.255.0",
				},
				Hosts: []*host.Host{
					{
						Hostname: "host1.example.com",
						Address:  "192.168.1.10",
						Roles:    []string{host.RoleOpenBAO},
					},
				},
			},
			wantErr: false,
		},
		{
			name: "IP not on same network",
			cfg: &config.Config{
				Network: &config.NetworkConfig{
					Gateway: "192.168.1.1",
					Netmask: "255.255.255.0",
				},
				Hosts: []*host.Host{
					{
						Hostname: "host1.example.com",
						Address:  "10.0.0.10", // Different network
						Roles:    []string{host.RoleOpenBAO},
					},
				},
			},
			wantErr: true,
			errMsg:  "not in network",
		},
		{
			name: "DHCP conflict",
			cfg: &config.Config{
				Network: &config.NetworkConfig{
					Gateway: "192.168.1.1",
					Netmask: "255.255.255.0",
					DHCPRange: &config.DHCPRange{
						Start: "192.168.1.50",
						End:   "192.168.1.200",
					},
				},
				Hosts: []*host.Host{
					{
						Hostname: "host1.example.com",
						Address:  "192.168.1.100", // In DHCP range
						Roles:    []string{host.RoleOpenBAO},
					},
				},
			},
			wantErr: true,
			errMsg:  "within DHCP range",
		},
		{
			name: "nil network config",
			cfg: &config.Config{
				Network: nil,
			},
			wantErr: true,
			errMsg:  "network configuration is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateNetworkConfig(tt.cfg)
			if tt.wantErr {
				assert.Error(t, err)
				if tt.errMsg != "" {
					assert.Contains(t, err.Error(), tt.errMsg)
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

// TestValidateDNSConfig tests DNS configuration validation
func TestValidateDNSConfig(t *testing.T) {
	tests := []struct {
		name    string
		cfg     *config.Config
		wantErr bool
		errMsg  string
	}{
		{
			name: "valid DNS config",
			cfg: &config.Config{
				DNS: &config.DNSConfig{
					InfrastructureZones: []config.DNSZone{{Name: "infra.example.com"}},
					KubernetesZones:     []config.DNSZone{{Name: "k8s.example.com"}},
					Backend:             "sqlite",
					APIKey:              "test-key",
				},
			},
			wantErr: false,
		},
		{
			name: "valid DNS config with public zones (same CNAME)",
			cfg: &config.Config{
				DNS: &config.DNSConfig{
					InfrastructureZones: []config.DNSZone{
						{Name: "infra.example.com", Public: true, PublicCNAME: strPtr("home.ddns.net")},
					},
					KubernetesZones: []config.DNSZone{
						{Name: "k8s.example.com", Public: true, PublicCNAME: strPtr("home.ddns.net")},
					},
					Backend: "sqlite",
					APIKey:  "test-key",
				},
			},
			wantErr: false,
		},
		{
			name: "different public CNAMEs",
			cfg: &config.Config{
				DNS: &config.DNSConfig{
					InfrastructureZones: []config.DNSZone{
						{Name: "infra.example.com", Public: true, PublicCNAME: strPtr("home1.ddns.net")},
					},
					KubernetesZones: []config.DNSZone{
						{Name: "k8s.example.com", Public: true, PublicCNAME: strPtr("home2.ddns.net")}, // Different!
					},
					Backend: "sqlite",
					APIKey:  "test-key",
				},
			},
			wantErr: true,
			errMsg:  "same public_cname",
		},
		{
			name: "missing infrastructure zones",
			cfg: &config.Config{
				DNS: &config.DNSConfig{
					InfrastructureZones: []config.DNSZone{}, // Empty!
					KubernetesZones:     []config.DNSZone{{Name: "k8s.example.com"}},
					Backend:             "sqlite",
					APIKey:              "test-key",
				},
			},
			wantErr: true,
			errMsg:  "at least one infrastructure zone is required",
		},
		{
			name: "missing kubernetes zones",
			cfg: &config.Config{
				DNS: &config.DNSConfig{
					InfrastructureZones: []config.DNSZone{{Name: "infra.example.com"}},
					KubernetesZones:     []config.DNSZone{}, // Empty!
					Backend:             "sqlite",
					APIKey:              "test-key",
				},
			},
			wantErr: true,
			errMsg:  "at least one kubernetes zone is required",
		},
		{
			name: "nil DNS config",
			cfg: &config.Config{
				DNS: nil,
			},
			wantErr: true,
			errMsg:  "dns configuration is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateDNSConfig(tt.cfg)
			if tt.wantErr {
				assert.Error(t, err)
				if tt.errMsg != "" {
					assert.Contains(t, err.Error(), tt.errMsg)
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

// TestValidateVIPConfig tests VIP configuration validation
func TestValidateVIPConfig(t *testing.T) {
	tests := []struct {
		name    string
		cfg     *config.Config
		wantErr bool
		errMsg  string
	}{
		{
			name: "valid VIP config",
			cfg: &config.Config{
				Network: &config.NetworkConfig{
				},
				Cluster: config.ClusterConfig{
					VIP: "192.168.1.100",
				},
			},
			wantErr: false,
		},
		{
			name: "missing VIP",
			cfg: &config.Config{
				Network: &config.NetworkConfig{
				},
			},
			wantErr: true,
			errMsg:  "vip is required",
		},
		{
			name: "nil network config",
			cfg: &config.Config{
				Network: nil,
			},
			wantErr: true,
			errMsg:  "network configuration is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateVIPConfig(tt.cfg)
			if tt.wantErr {
				assert.Error(t, err)
				if tt.errMsg != "" {
					assert.Contains(t, err.Error(), tt.errMsg)
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

// TestValidateClusterConfig tests cluster configuration validation
func TestValidateClusterConfig(t *testing.T) {
	tests := []struct {
		name    string
		cfg     *config.Config
		wantErr bool
		errMsg  string
	}{
		{
			name: "valid cluster config",
			cfg: &config.Config{
				Cluster: config.ClusterConfig{
					Name:   "test-cluster",
					Domain: "example.com",
				},
				Hosts: []*host.Host{
					{
						Hostname: "node1.example.com",
						Address:  "192.168.1.10",
						Roles:    []string{host.RoleClusterControlPlane},
					},
				},
			},
			wantErr: false,
		},
		{
			name: "no nodes",
			cfg: &config.Config{
				Cluster: config.ClusterConfig{
					Name:   "test-cluster",
					Domain: "example.com",
				},
			},
			wantErr: true,
			errMsg:  "at least one host with cluster role",
		},
		{
			name: "multiple nodes",
			cfg: &config.Config{
				Cluster: config.ClusterConfig{
					Name:   "test-cluster",
					Domain: "example.com",
				},
				Hosts: []*host.Host{
					{
						Hostname: "node1.example.com",
						Address:  "192.168.1.10",
						Roles:    []string{host.RoleClusterControlPlane},
					},
					{
						Hostname: "node2.example.com",
						Address:  "192.168.1.11",
						Roles:    []string{host.RoleClusterWorker},
					},
				},
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateClusterConfig(tt.cfg)
			if tt.wantErr {
				assert.Error(t, err)
				if tt.errMsg != "" {
					assert.Contains(t, err.Error(), tt.errMsg)
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

// TestValidateComponentDependencies tests component dependency validation
func TestValidateComponentDependencies(t *testing.T) {
	// Setup the component registry for testing
	setupValidateTestRegistry(t)

	tests := []struct {
		name    string
		cfg     *config.Config
		wantErr bool
		errMsg  string
	}{
		{
			name: "dependencies can be resolved",
			cfg: &config.Config{
				Components: config.ComponentMap{
					"openbao": {},
				},
			},
			wantErr: false,
		},
	}

	// Also test with an empty registry to trigger errors
	t.Run("empty registry causes error", func(t *testing.T) {
		// Temporarily clear the registry
		originalRegistry := component.DefaultRegistry
		component.DefaultRegistry = component.NewRegistry()
		defer func() {
			component.DefaultRegistry = originalRegistry
		}()

		cfg := &config.Config{
			Components: config.ComponentMap{
				"openbao": {},
			},
		}

		err := validateComponentDependencies(cfg)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "not found")
	})

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateComponentDependencies(tt.cfg)
			if tt.wantErr {
				assert.Error(t, err)
				if tt.errMsg != "" {
					assert.Contains(t, err.Error(), tt.errMsg)
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

// TestRunStackValidate_Integration tests the full validation flow
func TestRunStackValidate_Integration(t *testing.T) {
	// Setup the component registry for testing
	setupValidateTestRegistry(t)

	// Create a valid minimal config
	validCfg := &config.Config{
		Network: &config.NetworkConfig{
			Gateway: "192.168.1.1",
			Netmask: "255.255.255.0",
		},
		DNS: &config.DNSConfig{
			InfrastructureZones: []config.DNSZone{{Name: "infra.example.com"}},
			KubernetesZones:     []config.DNSZone{{Name: "k8s.example.com"}},
			Backend:             "sqlite",
			APIKey:              "test-key",
		},
		Cluster: config.ClusterConfig{
			Name:   "test-cluster",
			Domain: "example.com",
			VIP:    "192.168.1.100",
		},
		Hosts: []*host.Host{
			{
				Hostname: "infra1.example.com",
				Address:  "192.168.1.10",
				Roles:    []string{host.RoleOpenBAO, host.RoleDNS},
			},
			{
				Hostname: "node1.example.com",
				Address:  "192.168.1.20",
				Roles:    []string{host.RoleClusterControlPlane},
			},
		},
		Components: config.ComponentMap{
			"openbao": {},
		},
	}

	// Test each validation function individually
	t.Run("all validations pass", func(t *testing.T) {
		require.NoError(t, validateConfigStructure(validCfg))
		require.NoError(t, validateSecretReferences(validCfg))
		require.NoError(t, validateNetworkConfig(validCfg))
		require.NoError(t, validateDNSConfig(validCfg))
		require.NoError(t, validateVIPConfig(validCfg))
		require.NoError(t, validateClusterConfig(validCfg))
		require.NoError(t, validateComponentDependencies(validCfg))
	})

	// Test a config with multiple errors
	t.Run("config with multiple errors", func(t *testing.T) {
		invalidCfg := &config.Config{
			Network: &config.NetworkConfig{
				Gateway:      "192.168.1.1",
				Netmask:      "255.255.255.0",
			},
			DNS: &config.DNSConfig{
				InfrastructureZones: []config.DNSZone{}, // Empty!
				KubernetesZones:     []config.DNSZone{{Name: "k8s.example.com"}},
				Backend:             "sqlite",
				APIKey:              "test-key",
			},
			Cluster: config.ClusterConfig{
				Name:   "test-cluster",
				Domain: "example.com",
				VIP:    "10.0.0.100", // IP not on same network as 192.168.1.0/24
			},
			Hosts: []*host.Host{
				{Hostname: "test-host", Address: "192.168.1.10", Roles: []string{host.RoleOpenBAO}},
			},
			Components: config.ComponentMap{
				"openbao": {},
			},
		}

		// Should fail on network validation (VIP not on same network)
		err := validateNetworkConfig(invalidCfg)
		assert.Error(t, err)

		// Should fail on DNS validation (no infrastructure zones)
		err = validateDNSConfig(invalidCfg)
		assert.Error(t, err)

		// Should fail on cluster validation (no nodes)
		err = validateClusterConfig(invalidCfg)
		assert.Error(t, err)
	})
}
