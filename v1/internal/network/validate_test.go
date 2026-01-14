package network

import (
	"fmt"
	"net"
	"testing"

	"github.com/catalystcommunity/foundry/v1/internal/config"
	"github.com/catalystcommunity/foundry/v1/internal/host"
	"github.com/catalystcommunity/foundry/v1/internal/ssh"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValidateIPs(t *testing.T) {
	tests := []struct {
		name    string
		cfg     *config.Config
		wantErr bool
		errMsg  string
	}{
		{
			name:    "nil config",
			cfg:     nil,
			wantErr: true,
			errMsg:  "network config is nil",
		},
		{
			name: "all IPs on same network",
			cfg: &config.Config{
				Network: &config.NetworkConfig{
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
				Cluster: config.ClusterConfig{
					Name:          "test",
					PrimaryDomain: "example.com",
					VIP:    "192.168.1.100",
				},
				Components: config.ComponentMap{
					"k3s": config.ComponentConfig{},
				},
			},
			wantErr: false,
		},
		{
			name: "host IP outside network range",
			cfg: &config.Config{
				Network: &config.NetworkConfig{
					Gateway: "192.168.1.1",
					Netmask: "255.255.255.0",
				},
				Hosts: []*host.Host{
					{
						Hostname: "host1",
						Address:  "192.168.2.10", // Different subnet
						Roles:    []string{host.RoleOpenBAO},
					},
				},
				Cluster: config.ClusterConfig{
					Name:          "test",
					PrimaryDomain: "example.com",
					VIP:    "192.168.1.100",
				},
				Components: config.ComponentMap{
					"k3s": config.ComponentConfig{},
				},
			},
			wantErr: true,
			errMsg:  "is not in network",
		},
		{
			name: "VIP outside network range",
			cfg: &config.Config{
				Network: &config.NetworkConfig{
					Gateway: "192.168.1.1",
					Netmask: "255.255.255.0",
				},
				Hosts: []*host.Host{
					{
						Hostname: "host1",
						Address:  "192.168.1.10",
						Roles:    []string{host.RoleOpenBAO},
					},
				},
				Cluster: config.ClusterConfig{
					Name:          "test",
					PrimaryDomain: "example.com",
					VIP:    "10.0.0.100", // Completely different network
				},
				Components: config.ComponentMap{
					"k3s": config.ComponentConfig{},
				},
			},
			wantErr: true,
			errMsg:  "is not in network",
		},
		{
			name: "multiple hosts all on same network",
			cfg: &config.Config{
				Network: &config.NetworkConfig{
					Gateway: "192.168.1.1",
					Netmask: "255.255.255.0",
				},
				Hosts: []*host.Host{
					{
						Hostname: "host1",
						Address:  "192.168.1.10",
						Roles:    []string{host.RoleOpenBAO},
					},
					{
						Hostname: "host2",
						Address:  "192.168.1.11",
						Roles:    []string{host.RoleDNS},
					},
					{
						Hostname: "host3",
						Address:  "192.168.1.12",
						Roles:    []string{host.RoleZot},
					},
				},
				Cluster: config.ClusterConfig{
					Name:          "test",
					PrimaryDomain: "example.com",
					VIP:    "192.168.1.100",
				},
				Components: config.ComponentMap{
					"k3s": config.ComponentConfig{},
				},
			},
			wantErr: false,
		},
		{
			name: "one of multiple hosts outside network",
			cfg: &config.Config{
				Network: &config.NetworkConfig{
					Gateway: "192.168.1.1",
					Netmask: "255.255.255.0",
				},
				Hosts: []*host.Host{
					{
						Hostname: "host1",
						Address:  "192.168.1.10",
						Roles:    []string{host.RoleOpenBAO},
					},
					{
						Hostname: "host2",
						Address:  "192.168.2.15", // Different subnet
						Roles:    []string{host.RoleDNS},
					},
				},
				Cluster: config.ClusterConfig{
					Name:          "test",
					PrimaryDomain: "example.com",
					VIP:    "192.168.1.100",
				},
				Components: config.ComponentMap{
					"k3s": config.ComponentConfig{},
				},
			},
			wantErr: true,
			errMsg:  "is not in network",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateIPs(tt.cfg)
			if tt.wantErr {
				require.Error(t, err)
				if tt.errMsg != "" {
					assert.Contains(t, err.Error(), tt.errMsg)
				}
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestCheckReachability(t *testing.T) {
	tests := []struct {
		name       string
		ips        []string
		mockResult map[string]*ssh.ExecResult
		wantErr    bool
		errMsg     string
	}{
		{
			name:    "empty IP list",
			ips:     []string{},
			wantErr: false,
		},
		{
			name: "all IPs reachable",
			ips:  []string{"192.168.1.10", "192.168.1.11"},
			mockResult: map[string]*ssh.ExecResult{
				"ping -c 1 -W 2 192.168.1.10 > /dev/null 2>&1": {ExitCode: 0},
				"ping -c 1 -W 2 192.168.1.11 > /dev/null 2>&1": {ExitCode: 0},
			},
			wantErr: false,
		},
		{
			name: "one IP unreachable",
			ips:  []string{"192.168.1.10", "192.168.1.11"},
			mockResult: map[string]*ssh.ExecResult{
				"ping -c 1 -W 2 192.168.1.10 > /dev/null 2>&1": {ExitCode: 0},
				"ping -c 1 -W 2 192.168.1.11 > /dev/null 2>&1": {ExitCode: 1},
			},
			wantErr: true,
			errMsg:  "unreachable IPs: 192.168.1.11",
		},
		{
			name: "multiple IPs unreachable",
			ips:  []string{"192.168.1.10", "192.168.1.11", "192.168.1.12"},
			mockResult: map[string]*ssh.ExecResult{
				"ping -c 1 -W 2 192.168.1.10 > /dev/null 2>&1": {ExitCode: 1},
				"ping -c 1 -W 2 192.168.1.11 > /dev/null 2>&1": {ExitCode: 0},
				"ping -c 1 -W 2 192.168.1.12 > /dev/null 2>&1": {ExitCode: 1},
			},
			wantErr: true,
			errMsg:  "unreachable IPs",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mock := &mockSSHExecutor{results: tt.mockResult}
			err := CheckReachability(mock, tt.ips)
			if tt.wantErr {
				require.Error(t, err)
				if tt.errMsg != "" {
					assert.Contains(t, err.Error(), tt.errMsg)
				}
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestCheckDHCPConflicts(t *testing.T) {
	tests := []struct {
		name    string
		cfg     *config.Config
		wantErr bool
		errMsg  string
	}{
		{
			name:    "nil config",
			cfg:     nil,
			wantErr: true,
			errMsg:  "network config is nil",
		},
		{
			name: "no DHCP range configured",
			cfg: &config.Config{
				Network: &config.NetworkConfig{
					Gateway:   "192.168.1.1",
					Netmask:   "255.255.255.0",
					DHCPRange: nil,
				},
				Hosts: []*host.Host{
					{
						Hostname: "host1",
						Address:  "192.168.1.10",
						Roles:    []string{host.RoleOpenBAO},
					},
				},
				Cluster: config.ClusterConfig{
					Name:          "test",
					PrimaryDomain: "example.com",
					VIP:    "192.168.1.100",
				},
				Components: config.ComponentMap{
					"k3s": config.ComponentConfig{},
				},
			},
			wantErr: false,
		},
		{
			name: "no conflicts - IPs outside DHCP range",
			cfg: &config.Config{
				Network: &config.NetworkConfig{
					Gateway: "192.168.1.1",
					Netmask: "255.255.255.0",
					DHCPRange: &config.DHCPRange{
						Start: "192.168.1.50",
						End:   "192.168.1.99",
					},
				},
				Hosts: []*host.Host{
					{
						Hostname: "host1",
						Address:  "192.168.1.10",
						Roles:    []string{host.RoleOpenBAO},
					},
					{
						Hostname: "host2",
						Address:  "192.168.1.11",
						Roles:    []string{host.RoleDNS},
					},
					{
						Hostname: "host3",
						Address:  "192.168.1.12",
						Roles:    []string{host.RoleZot},
					},
				},
				Cluster: config.ClusterConfig{
					Name:          "test",
					PrimaryDomain: "example.com",
					VIP:    "192.168.1.100",
				},
				Components: config.ComponentMap{
					"k3s": config.ComponentConfig{},
				},
			},
			wantErr: false,
		},
		{
			name: "VIP conflicts with DHCP range",
			cfg: &config.Config{
				Network: &config.NetworkConfig{
					Gateway: "192.168.1.1",
					Netmask: "255.255.255.0",
					DHCPRange: &config.DHCPRange{
						Start: "192.168.1.50",
						End:   "192.168.1.99",
					},
				},
				Hosts: []*host.Host{
					{
						Hostname: "host1",
						Address:  "192.168.1.10",
						Roles:    []string{host.RoleOpenBAO},
					},
				},
				Cluster: config.ClusterConfig{
					Name:          "test",
					PrimaryDomain: "example.com",
					VIP:    "192.168.1.75", // Inside DHCP range
				},
				Components: config.ComponentMap{
					"k3s": config.ComponentConfig{},
				},
			},
			wantErr: true,
			errMsg:  "infrastructure IPs within DHCP range",
		},
		{
			name: "host conflicts with DHCP range",
			cfg: &config.Config{
				Network: &config.NetworkConfig{
					Gateway: "192.168.1.1",
					Netmask: "255.255.255.0",
					DHCPRange: &config.DHCPRange{
						Start: "192.168.1.50",
						End:   "192.168.1.99",
					},
				},
				Hosts: []*host.Host{
					{
						Hostname: "host1",
						Address:  "192.168.1.50", // Inside DHCP range
						Roles:    []string{host.RoleOpenBAO},
					},
				},
				Cluster: config.ClusterConfig{
					Name:          "test",
					PrimaryDomain: "example.com",
					VIP:    "192.168.1.100",
				},
				Components: config.ComponentMap{
					"k3s": config.ComponentConfig{},
				},
			},
			wantErr: true,
			errMsg:  "192.168.1.50",
		},
		{
			name: "multiple conflicts",
			cfg: &config.Config{
				Network: &config.NetworkConfig{
					Gateway: "192.168.1.1",
					Netmask: "255.255.255.0",
					DHCPRange: &config.DHCPRange{
						Start: "192.168.1.50",
						End:   "192.168.1.99",
					},
				},
				Hosts: []*host.Host{
					{
						Hostname: "host1",
						Address:  "192.168.1.60",
						Roles:    []string{host.RoleOpenBAO},
					},
					{
						Hostname: "host2",
						Address:  "192.168.1.70",
						Roles:    []string{host.RoleDNS},
					},
					{
						Hostname: "host3",
						Address:  "192.168.1.80",
						Roles:    []string{host.RoleZot},
					},
				},
				Cluster: config.ClusterConfig{
					Name:          "test",
					PrimaryDomain: "example.com",
					VIP:    "192.168.1.100",
				},
				Components: config.ComponentMap{
					"k3s": config.ComponentConfig{},
				},
			},
			wantErr: true,
			errMsg:  "infrastructure IPs within DHCP range",
		},
		{
			name: "boundary case - IP at start of DHCP range",
			cfg: &config.Config{
				Network: &config.NetworkConfig{
					Gateway: "192.168.1.1",
					Netmask: "255.255.255.0",
					DHCPRange: &config.DHCPRange{
						Start: "192.168.1.50",
						End:   "192.168.1.99",
					},
				},
				Hosts: []*host.Host{
					{
						Hostname: "host1",
						Address:  "192.168.1.50", // Exactly at start
						Roles:    []string{host.RoleOpenBAO},
					},
				},
				Cluster: config.ClusterConfig{
					Name:          "test",
					PrimaryDomain: "example.com",
					VIP:    "192.168.1.100",
				},
				Components: config.ComponentMap{
					"k3s": config.ComponentConfig{},
				},
			},
			wantErr: true,
			errMsg:  "192.168.1.50",
		},
		{
			name: "boundary case - VIP at end of DHCP range",
			cfg: &config.Config{
				Network: &config.NetworkConfig{
					Gateway: "192.168.1.1",
					Netmask: "255.255.255.0",
					DHCPRange: &config.DHCPRange{
						Start: "192.168.1.50",
						End:   "192.168.1.99",
					},
				},
				Hosts: []*host.Host{
					{
						Hostname: "host1",
						Address:  "192.168.1.10",
						Roles:    []string{host.RoleOpenBAO},
					},
				},
				Cluster: config.ClusterConfig{
					Name:          "test",
					PrimaryDomain: "example.com",
					VIP:    "192.168.1.99", // Exactly at end
				},
				Components: config.ComponentMap{
					"k3s": config.ComponentConfig{},
				},
			},
			wantErr: true,
			errMsg:  "192.168.1.99",
		},
		{
			name: "boundary case - IP just before DHCP range",
			cfg: &config.Config{
				Network: &config.NetworkConfig{
					Gateway: "192.168.1.1",
					Netmask: "255.255.255.0",
					DHCPRange: &config.DHCPRange{
						Start: "192.168.1.50",
						End:   "192.168.1.99",
					},
				},
				Hosts: []*host.Host{
					{
						Hostname: "host1",
						Address:  "192.168.1.49", // Just before start
						Roles:    []string{host.RoleOpenBAO},
					},
				},
				Cluster: config.ClusterConfig{
					Name:          "test",
					PrimaryDomain: "example.com",
					VIP:    "192.168.1.100",
				},
				Components: config.ComponentMap{
					"k3s": config.ComponentConfig{},
				},
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := CheckDHCPConflicts(tt.cfg)
			if tt.wantErr {
				require.Error(t, err)
				if tt.errMsg != "" {
					assert.Contains(t, err.Error(), tt.errMsg)
				}
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestValidateDNSResolution(t *testing.T) {
	tests := []struct {
		name       string
		hostname   string
		expectedIP string
		mockResult *ssh.ExecResult
		wantErr    bool
		errMsg     string
	}{
		{
			name:       "empty hostname",
			hostname:   "",
			expectedIP: "192.168.1.10",
			wantErr:    true,
			errMsg:     "hostname is required",
		},
		{
			name:       "empty expected IP",
			hostname:   "openbao.example.com",
			expectedIP: "",
			wantErr:    true,
			errMsg:     "expected IP is required",
		},
		{
			name:       "hostname resolves to expected IP",
			hostname:   "openbao.example.com",
			expectedIP: "192.168.1.10",
			mockResult: &ssh.ExecResult{
				ExitCode: 0,
				Stdout:   "192.168.1.10\n",
			},
			wantErr: false,
		},
		{
			name:       "hostname resolves to different IP",
			hostname:   "openbao.example.com",
			expectedIP: "192.168.1.10",
			mockResult: &ssh.ExecResult{
				ExitCode: 0,
				Stdout:   "192.168.1.99\n",
			},
			wantErr: true,
			errMsg:  "resolved to 192.168.1.99, expected 192.168.1.10",
		},
		{
			name:       "hostname does not resolve",
			hostname:   "nonexistent.example.com",
			expectedIP: "192.168.1.10",
			mockResult: &ssh.ExecResult{
				ExitCode: 0,
				Stdout:   "",
			},
			wantErr: true,
			errMsg:  "did not resolve to any IP",
		},
		{
			name:       "DNS query fails",
			hostname:   "openbao.example.com",
			expectedIP: "192.168.1.10",
			mockResult: &ssh.ExecResult{
				ExitCode: 1,
				Stderr:   "DNS query failed",
			},
			wantErr: true,
			errMsg:  "failed to resolve",
		},
		{
			name:       "hostname resolves to multiple IPs (takes first)",
			hostname:   "openbao.example.com",
			expectedIP: "192.168.1.10",
			mockResult: &ssh.ExecResult{
				ExitCode: 0,
				Stdout:   "192.168.1.10\n192.168.1.11\n",
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var mock *mockSSHExecutor
			if tt.mockResult != nil {
				mock = &mockSSHExecutor{
					defaultResult: tt.mockResult,
				}
			}

			err := ValidateDNSResolution(mock, tt.hostname, tt.expectedIP)
			if tt.wantErr {
				require.Error(t, err)
				if tt.errMsg != "" {
					assert.Contains(t, err.Error(), tt.errMsg)
				}
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestGetNetworkCIDR(t *testing.T) {
	tests := []struct {
		name     string
		gateway  string
		netmask  string
		wantCIDR string
		wantErr  bool
		errMsg   string
	}{
		{
			name:     "valid /24 network",
			gateway:  "192.168.1.1",
			netmask:  "255.255.255.0",
			wantCIDR: "192.168.1.0/24",
			wantErr:  false,
		},
		{
			name:     "valid /16 network",
			gateway:  "10.0.5.1",
			netmask:  "255.255.0.0",
			wantCIDR: "10.0.0.0/16",
			wantErr:  false,
		},
		{
			name:     "valid /8 network",
			gateway:  "10.5.5.1",
			netmask:  "255.0.0.0",
			wantCIDR: "10.0.0.0/8",
			wantErr:  false,
		},
		{
			name:    "invalid gateway",
			gateway: "invalid",
			netmask: "255.255.255.0",
			wantErr: true,
			errMsg:  "invalid gateway IP",
		},
		{
			name:    "invalid netmask",
			gateway: "192.168.1.1",
			netmask: "invalid",
			wantErr: true,
			errMsg:  "invalid netmask",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cidr, err := GetNetworkCIDR(tt.gateway, tt.netmask)
			if tt.wantErr {
				require.Error(t, err)
				if tt.errMsg != "" {
					assert.Contains(t, err.Error(), tt.errMsg)
				}
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.wantCIDR, cidr.String())
			}
		})
	}
}

func TestIsIPInRange(t *testing.T) {
	tests := []struct {
		name     string
		ip       string
		start    string
		end      string
		expected bool
	}{
		{
			name:     "IP in range",
			ip:       "192.168.1.75",
			start:    "192.168.1.50",
			end:      "192.168.1.99",
			expected: true,
		},
		{
			name:     "IP at start of range",
			ip:       "192.168.1.50",
			start:    "192.168.1.50",
			end:      "192.168.1.99",
			expected: true,
		},
		{
			name:     "IP at end of range",
			ip:       "192.168.1.99",
			start:    "192.168.1.50",
			end:      "192.168.1.99",
			expected: true,
		},
		{
			name:     "IP before range",
			ip:       "192.168.1.49",
			start:    "192.168.1.50",
			end:      "192.168.1.99",
			expected: false,
		},
		{
			name:     "IP after range",
			ip:       "192.168.1.100",
			start:    "192.168.1.50",
			end:      "192.168.1.99",
			expected: false,
		},
		{
			name:     "IP in different subnet",
			ip:       "192.168.2.75",
			start:    "192.168.1.50",
			end:      "192.168.1.99",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ip := net.ParseIP(tt.ip)
			start := net.ParseIP(tt.start)
			end := net.ParseIP(tt.end)
			result := isIPInRange(ip, start, end)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestIpToUint32(t *testing.T) {
	tests := []struct {
		name     string
		ip       string
		expected uint32
	}{
		{
			name:     "192.168.1.1",
			ip:       "192.168.1.1",
			expected: 3232235777, // 192<<24 | 168<<16 | 1<<8 | 1
		},
		{
			name:     "0.0.0.0",
			ip:       "0.0.0.0",
			expected: 0,
		},
		{
			name:     "255.255.255.255",
			ip:       "255.255.255.255",
			expected: 4294967295,
		},
		{
			name:     "10.0.0.1",
			ip:       "10.0.0.1",
			expected: 167772161,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ip := net.ParseIP(tt.ip)
			result := ipToUint32(ip)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// mockSSHExecutor is a mock implementation of SSHExecutor for testing
type mockSSHExecutor struct {
	results       map[string]*ssh.ExecResult
	defaultResult *ssh.ExecResult
	execCalls     []string
}

func (m *mockSSHExecutor) Exec(command string) (*ssh.ExecResult, error) {
	if m.execCalls != nil {
		m.execCalls = append(m.execCalls, command)
	}

	if m.results != nil {
		if result, ok := m.results[command]; ok {
			return result, nil
		}
	}

	if m.defaultResult != nil {
		return m.defaultResult, nil
	}

	return nil, fmt.Errorf("no mock result configured for command: %s", command)
}
