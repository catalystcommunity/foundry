package config

import (
	"testing"

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
				Gateway:      "192.168.1.1",
				Netmask:      "255.255.255.0",
				K8sVIP:       "192.168.1.100",
				OpenBAOHosts: []string{"192.168.1.10"},
				DNSHosts:     []string{"192.168.1.10"},
				ZotHosts:     []string{"192.168.1.10"},
			},
			wantErr: false,
		},
		{
			name: "valid with DHCP range and TrueNAS",
			config: NetworkConfig{
				Gateway:      "192.168.1.1",
				Netmask:      "255.255.255.0",
				DHCPRange: &DHCPRange{
					Start: "192.168.1.50",
					End:   "192.168.1.200",
				},
				K8sVIP:       "192.168.1.100",
				OpenBAOHosts: []string{"192.168.1.10"},
				DNSHosts:     []string{"192.168.1.10"},
				ZotHosts:     []string{"192.168.1.10"},
				TrueNASHosts: []string{"192.168.1.15"},
			},
			wantErr: false,
		},
		{
			name: "missing gateway",
			config: NetworkConfig{
				Netmask:      "255.255.255.0",
				K8sVIP:       "192.168.1.100",
				OpenBAOHosts: []string{"192.168.1.10"},
				DNSHosts:     []string{"192.168.1.10"},
				ZotHosts:     []string{"192.168.1.10"},
			},
			wantErr: true,
			errMsg:  "gateway is required",
		},
		{
			name: "invalid gateway IP",
			config: NetworkConfig{
				Gateway:      "not-an-ip",
				Netmask:      "255.255.255.0",
				K8sVIP:       "192.168.1.100",
				OpenBAOHosts: []string{"192.168.1.10"},
				DNSHosts:     []string{"192.168.1.10"},
				ZotHosts:     []string{"192.168.1.10"},
			},
			wantErr: true,
			errMsg:  "not a valid IP address",
		},
		{
			name: "missing netmask",
			config: NetworkConfig{
				Gateway:      "192.168.1.1",
				K8sVIP:       "192.168.1.100",
				OpenBAOHosts: []string{"192.168.1.10"},
				DNSHosts:     []string{"192.168.1.10"},
				ZotHosts:     []string{"192.168.1.10"},
			},
			wantErr: true,
			errMsg:  "netmask is required",
		},
		{
			name: "missing k8s_vip",
			config: NetworkConfig{
				Gateway:      "192.168.1.1",
				Netmask:      "255.255.255.0",
				OpenBAOHosts: []string{"192.168.1.10"},
				DNSHosts:     []string{"192.168.1.10"},
				ZotHosts:     []string{"192.168.1.10"},
			},
			wantErr: true,
			errMsg:  "k8s_vip is required",
		},
		{
			name: "invalid k8s_vip",
			config: NetworkConfig{
				Gateway:      "192.168.1.1",
				Netmask:      "255.255.255.0",
				K8sVIP:       "invalid",
				OpenBAOHosts: []string{"192.168.1.10"},
				DNSHosts:     []string{"192.168.1.10"},
				ZotHosts:     []string{"192.168.1.10"},
			},
			wantErr: true,
			errMsg:  "not a valid IP address",
		},
		{
			name: "missing openbao_hosts",
			config: NetworkConfig{
				Gateway: "192.168.1.1",
				Netmask: "255.255.255.0",
				K8sVIP:  "192.168.1.100",
				DNSHosts: []string{"192.168.1.10"},
				ZotHosts: []string{"192.168.1.10"},
			},
			wantErr: true,
			errMsg:  "at least one openbao_hosts entry is required",
		},
		{
			name: "invalid openbao host IP",
			config: NetworkConfig{
				Gateway:      "192.168.1.1",
				Netmask:      "255.255.255.0",
				K8sVIP:       "192.168.1.100",
				OpenBAOHosts: []string{"invalid"},
				DNSHosts:     []string{"192.168.1.10"},
				ZotHosts:     []string{"192.168.1.10"},
			},
			wantErr: true,
			errMsg:  "not a valid IP address",
		},
		{
			name: "missing dns_hosts",
			config: NetworkConfig{
				Gateway:      "192.168.1.1",
				Netmask:      "255.255.255.0",
				K8sVIP:       "192.168.1.100",
				OpenBAOHosts: []string{"192.168.1.10"},
				ZotHosts:     []string{"192.168.1.10"},
			},
			wantErr: true,
			errMsg:  "at least one dns_hosts entry is required",
		},
		{
			name: "missing zot_hosts",
			config: NetworkConfig{
				Gateway:      "192.168.1.1",
				Netmask:      "255.255.255.0",
				K8sVIP:       "192.168.1.100",
				OpenBAOHosts: []string{"192.168.1.10"},
				DNSHosts:     []string{"192.168.1.10"},
			},
			wantErr: true,
			errMsg:  "at least one zot_hosts entry is required",
		},
		{
			name: "invalid truenas host IP",
			config: NetworkConfig{
				Gateway:      "192.168.1.1",
				Netmask:      "255.255.255.0",
				K8sVIP:       "192.168.1.100",
				OpenBAOHosts: []string{"192.168.1.10"},
				DNSHosts:     []string{"192.168.1.10"},
				ZotHosts:     []string{"192.168.1.10"},
				TrueNASHosts: []string{"invalid"},
			},
			wantErr: true,
			errMsg:  "not a valid IP address",
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

func TestConfig_ValidateK8sVIPUniqueness(t *testing.T) {
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
					Gateway:      "192.168.1.1",
					Netmask:      "255.255.255.0",
					K8sVIP:       "192.168.1.100",
					OpenBAOHosts: []string{"192.168.1.10"},
					DNSHosts:     []string{"192.168.1.11"},
					ZotHosts:     []string{"192.168.1.12"},
				},
				Cluster: ClusterConfig{
					Name:   "test",
					Domain: "example.com",
					Nodes:  []NodeConfig{{Hostname: "node1", Role: NodeRoleControlPlane}},
				},
				Components: ComponentMap{"k3s": ComponentConfig{}},
			},
			wantErr: false,
		},
		{
			name: "VIP conflicts with OpenBAO host",
			config: Config{
				Network: &NetworkConfig{
					Gateway:      "192.168.1.1",
					Netmask:      "255.255.255.0",
					K8sVIP:       "192.168.1.10",
					OpenBAOHosts: []string{"192.168.1.10"},
					DNSHosts:     []string{"192.168.1.11"},
					ZotHosts:     []string{"192.168.1.12"},
				},
				Cluster: ClusterConfig{
					Name:   "test",
					Domain: "example.com",
					Nodes:  []NodeConfig{{Hostname: "node1", Role: NodeRoleControlPlane}},
				},
				Components: ComponentMap{"k3s": ComponentConfig{}},
			},
			wantErr: true,
			errMsg:  "conflicts with infrastructure host IP",
		},
		{
			name: "VIP conflicts with DNS host",
			config: Config{
				Network: &NetworkConfig{
					Gateway:      "192.168.1.1",
					Netmask:      "255.255.255.0",
					K8sVIP:       "192.168.1.11",
					OpenBAOHosts: []string{"192.168.1.10"},
					DNSHosts:     []string{"192.168.1.11"},
					ZotHosts:     []string{"192.168.1.12"},
				},
				Cluster: ClusterConfig{
					Name:   "test",
					Domain: "example.com",
					Nodes:  []NodeConfig{{Hostname: "node1", Role: NodeRoleControlPlane}},
				},
				Components: ComponentMap{"k3s": ComponentConfig{}},
			},
			wantErr: true,
			errMsg:  "conflicts with infrastructure host IP",
		},
		{
			name: "VIP conflicts with TrueNAS host",
			config: Config{
				Network: &NetworkConfig{
					Gateway:      "192.168.1.1",
					Netmask:      "255.255.255.0",
					K8sVIP:       "192.168.1.15",
					OpenBAOHosts: []string{"192.168.1.10"},
					DNSHosts:     []string{"192.168.1.11"},
					ZotHosts:     []string{"192.168.1.12"},
					TrueNASHosts: []string{"192.168.1.15"},
				},
				Cluster: ClusterConfig{
					Name:   "test",
					Domain: "example.com",
					Nodes:  []NodeConfig{{Hostname: "node1", Role: NodeRoleControlPlane}},
				},
				Components: ComponentMap{"k3s": ComponentConfig{}},
			},
			wantErr: true,
			errMsg:  "conflicts with infrastructure host IP",
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
