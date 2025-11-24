package config

import (
	"testing"

	"github.com/catalystcommunity/foundry/v1/internal/host"
	"github.com/stretchr/testify/assert"
)

// Helper function for tests
func strPtr(s string) *string {
	return &s
}

func TestDNSConfig_Validate(t *testing.T) {
	tests := []struct {
		name    string
		config  DNSConfig
		wantErr bool
		errMsg  string
	}{
		{
			name: "valid DNS config",
			config: DNSConfig{
				InfrastructureZones: []DNSZone{
					{Name: "infra.example.com", Public: true, PublicCNAME: strPtr("home.example.com")},
				},
				KubernetesZones: []DNSZone{
					{Name: "k8s.example.com", Public: true, PublicCNAME: strPtr("home.example.com")},
				},
				Forwarders: []string{"8.8.8.8", "1.1.1.1"},
				Backend:    "sqlite",
				APIKey:     "${secret:foundry-core/dns:api_key}",
			},
			wantErr: false,
		},
		{
			name: "valid with .local zone",
			config: DNSConfig{
				InfrastructureZones: []DNSZone{
					{Name: "infra.local", Public: false},
				},
				KubernetesZones: []DNSZone{
					{Name: "k8s.local", Public: false},
				},
				Forwarders: []string{"8.8.8.8"},
				Backend:    "sqlite",
				APIKey:     "test-key",
			},
			wantErr: false,
		},
		{
			name: "zones are optional - only backend and api_key required",
			config: DNSConfig{
				Backend: "sqlite",
				APIKey:  "test-key",
			},
			wantErr: false,
		},
		{
			name: "duplicate zone names",
			config: DNSConfig{
				InfrastructureZones: []DNSZone{
					{Name: "example.com", Public: true, PublicCNAME: strPtr("home.example.com")},
				},
				KubernetesZones: []DNSZone{
					{Name: "example.com", Public: true, PublicCNAME: strPtr("home.example.com")},
				},
				Backend: "sqlite",
				APIKey:  "test-key",
			},
			wantErr: true,
			errMsg:  "duplicate zone name",
		},
		{
			name: "missing backend",
			config: DNSConfig{
				InfrastructureZones: []DNSZone{
					{Name: "infra.example.com", Public: true, PublicCNAME: strPtr("home.example.com")},
				},
				KubernetesZones: []DNSZone{
					{Name: "k8s.example.com", Public: true, PublicCNAME: strPtr("home.example.com")},
				},
				APIKey: "test-key",
			},
			wantErr: true,
			errMsg:  "backend is required",
		},
		{
			name: "missing api_key",
			config: DNSConfig{
				InfrastructureZones: []DNSZone{
					{Name: "infra.example.com", Public: true, PublicCNAME: strPtr("home.example.com")},
				},
				KubernetesZones: []DNSZone{
					{Name: "k8s.example.com", Public: true, PublicCNAME: strPtr("home.example.com")},
				},
				Backend: "sqlite",
			},
			wantErr: true,
			errMsg:  "api_key is required",
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

func TestDNSZone_Validate(t *testing.T) {
	tests := []struct {
		name    string
		zone    DNSZone
		wantErr bool
		errMsg  string
	}{
		{
			name: "valid public zone",
			zone: DNSZone{
				Name:        "example.com",
				Public:      true,
				PublicCNAME: strPtr("home.example.com"),
			},
			wantErr: false,
		},
		{
			name: "valid private zone",
			zone: DNSZone{
				Name:   "example.com",
				Public: false,
			},
			wantErr: false,
		},
		{
			name: "valid .local zone (private)",
			zone: DNSZone{
				Name:   "infra.local",
				Public: false,
			},
			wantErr: false,
		},
		{
			name:    "missing zone name",
			zone:    DNSZone{Public: false},
			wantErr: true,
			errMsg:  "zone name is required",
		},
		{
			name: ".local zone marked as public",
			zone: DNSZone{
				Name:        "infra.local",
				Public:      true,
				PublicCNAME: strPtr("home.example.com"),
			},
			wantErr: true,
			errMsg:  "ends with .local but is marked as public",
		},
		{
			name: "public zone missing public_cname",
			zone: DNSZone{
				Name:   "example.com",
				Public: true,
			},
			wantErr: true,
			errMsg:  "marked as public but missing public_cname",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.zone.Validate()
			if tt.wantErr {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.errMsg)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestConfig_WithNetworkAndDNS(t *testing.T) {
	config := Config{
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
		DNS: &DNSConfig{
			InfrastructureZones: []DNSZone{
				{Name: "infra.example.com", Public: true, PublicCNAME: strPtr("home.example.com")},
			},
			KubernetesZones: []DNSZone{
				{Name: "k8s.example.com", Public: true, PublicCNAME: strPtr("home.example.com")},
			},
			Backend: "sqlite",
			APIKey:  "${secret:foundry-core/dns:api_key}",
		},
		Cluster: ClusterConfig{
			Name:   "test",
			Domain: "example.com",
			VIP:    "192.168.1.100",
		},
		Components: ComponentMap{
			"k3s": ComponentConfig{},
		},
	}

	err := config.Validate()
	assert.NoError(t, err)
}
