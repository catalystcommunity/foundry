package config

import (
	"testing"

	"github.com/catalystcommunity/foundry/v1/internal/host"
	"github.com/stretchr/testify/assert"
)

func TestNetworkConfig_Validate(t *testing.T) {
	tests := []struct {
		name    string
		config  NetworkConfig
		wantErr bool
		errMsg  string
	}{
		{
			name: "valid network config",
			config: NetworkConfig{
				Gateway: "192.168.1.1",
				Netmask: "255.255.255.0",
			},
			wantErr: false,
		},
		{
			name: "valid with DHCP range",
			config: NetworkConfig{
				Gateway: "192.168.1.1",
				Netmask: "255.255.255.0",
				DHCPRange: &DHCPRange{
					Start: "192.168.1.50",
					End:   "192.168.1.200",
				},
			},
			wantErr: false,
		},
		{
			name: "missing gateway",
			config: NetworkConfig{
				Netmask: "255.255.255.0",
			},
			wantErr: true,
			errMsg:  "gateway is required",
		},
		{
			name: "invalid gateway IP",
			config: NetworkConfig{
				Gateway: "not-an-ip",
				Netmask: "255.255.255.0",
			},
			wantErr: true,
			errMsg:  "not a valid IP address",
		},
		{
			name: "missing netmask",
			config: NetworkConfig{
				Gateway: "192.168.1.1",
			},
			wantErr: true,
			errMsg:  "netmask is required",
		},
		{
			name: "invalid netmask",
			config: NetworkConfig{
				Gateway: "192.168.1.1",
				Netmask: "invalid",
			},
			wantErr: true,
			errMsg:  "not a valid IP address",
		},
		{
			name: "invalid DHCP range",
			config: NetworkConfig{
				Gateway: "192.168.1.1",
				Netmask: "255.255.255.0",
				DHCPRange: &DHCPRange{
					Start: "invalid",
					End:   "192.168.1.200",
				},
			},
			wantErr: true,
			errMsg:  "dhcp_range validation failed",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.Validate()
			if tt.wantErr {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.errMsg)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestDHCPRange_Validate(t *testing.T) {
	tests := []struct {
		name    string
		dhcp    DHCPRange
		wantErr bool
		errMsg  string
	}{
		{
			name: "valid DHCP range",
			dhcp: DHCPRange{
				Start: "192.168.1.50",
				End:   "192.168.1.200",
			},
			wantErr: false,
		},
		{
			name: "missing start",
			dhcp: DHCPRange{
				End: "192.168.1.200",
			},
			wantErr: true,
			errMsg:  "start is required",
		},
		{
			name: "invalid start IP",
			dhcp: DHCPRange{
				Start: "invalid",
				End:   "192.168.1.200",
			},
			wantErr: true,
			errMsg:  "not a valid IP address",
		},
		{
			name: "missing end",
			dhcp: DHCPRange{
				Start: "192.168.1.50",
			},
			wantErr: true,
			errMsg:  "end is required",
		},
		{
			name: "invalid end IP",
			dhcp: DHCPRange{
				Start: "192.168.1.50",
				End:   "invalid",
			},
			wantErr: true,
			errMsg:  "not a valid IP address",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.dhcp.Validate()
			if tt.wantErr {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.errMsg)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestConfig_ValidateVIPUniqueness(t *testing.T) {
	tests := []struct {
		name    string
		config  Config
		wantErr bool
		errMsg  string
	}{
		{
			name: "VIP is unique",
			config: Config{
				Network: &NetworkConfig{
					Gateway: "192.168.1.1",
					Netmask: "255.255.255.0",
				},
				Hosts: []*host.Host{
					{
						Hostname: "host1",
						Address:  "192.168.1.10",
						Roles:    []string{host.RoleOpenBAO, host.RoleDNS, host.RoleZot},
					},
					{
						Hostname: "node1",
						Address:  "192.168.1.20",
						Roles:    []string{host.RoleClusterControlPlane},
					},
				},
				Cluster: ClusterConfig{
					Name:   "test",
					Domain: "example.com",
					VIP:    "192.168.1.100",
				},
				Components: ComponentMap{"k3s": ComponentConfig{}},
			},
			wantErr: false,
		},
		{
			name: "VIP conflicts with infrastructure host",
			config: Config{
				Network: &NetworkConfig{
					Gateway: "192.168.1.1",
					Netmask: "255.255.255.0",
				},
				Hosts: []*host.Host{
					{
						Hostname: "host1",
						Address:  "192.168.1.10",
						Roles:    []string{host.RoleOpenBAO, host.RoleDNS, host.RoleZot},
					},
				},
				Cluster: ClusterConfig{
					Name:   "test",
					Domain: "example.com",
					VIP:    "192.168.1.10", // Conflicts with host1
				},
				Components: ComponentMap{"k3s": ComponentConfig{}},
			},
			wantErr: true,
			errMsg:  "conflicts with host",
		},
		{
			name: "VIP conflicts with cluster node",
			config: Config{
				Network: &NetworkConfig{
					Gateway: "192.168.1.1",
					Netmask: "255.255.255.0",
				},
				Hosts: []*host.Host{
					{
						Hostname: "node1",
						Address:  "192.168.1.20",
						Roles:    []string{host.RoleClusterControlPlane},
					},
				},
				Cluster: ClusterConfig{
					Name:   "test",
					Domain: "example.com",
					VIP:    "192.168.1.20", // Conflicts with node1
				},
				Components: ComponentMap{"k3s": ComponentConfig{}},
			},
			wantErr: true,
			errMsg:  "conflicts with host",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.Validate()
			if tt.wantErr {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.errMsg)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}
