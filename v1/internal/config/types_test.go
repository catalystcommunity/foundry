package config

import (
	"testing"

	"github.com/catalystcommunity/foundry/v1/internal/host"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func TestConfig_Validate(t *testing.T) {
	tests := []struct {
		name    string
		config  Config
		wantErr bool
		errMsg  string
	}{
		{
			name: "valid minimal config",
			config: Config{
				Cluster: ClusterConfig{
					Name:   "test",
					Domain: "example.com",
				},
				Components: ComponentMap{
					"k3s": ComponentConfig{},
				},
			},
			wantErr: false,
		},
		{
			name: "no components",
			config: Config{
				Cluster: ClusterConfig{
					Name:   "test",
					Domain: "example.com",
				},
				Components: ComponentMap{},
			},
			wantErr: true,
			errMsg:  "at least one component must be defined",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.Validate()
			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errMsg)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

// NOTE: TestClusterConfig_Validate removed - ClusterConfig.Validate() no longer exists
// Cluster validation moved to role-based host validation in host package

// NOTE: TestNodeConfig_Validate removed - NodeConfig type no longer exists
// Node configuration moved to host.Host with cluster-* roles

func TestComponentConfig_Validate(t *testing.T) {
	tests := []struct {
		name      string
		component ComponentConfig
		wantErr   bool
		errMsg    string
	}{
		{
			name: "valid component with version",
			component: ComponentConfig{
			},
			wantErr: false,
		},
		{
			name: "valid component without version",
			component: ComponentConfig{
				Hosts: []string{"host1.example.com"},
			},
			wantErr: false,
		},
		{
			name: "empty hosts array",
			component: ComponentConfig{
				Hosts: []string{},
			},
			wantErr: true,
			errMsg:  "if hosts is specified, it must not be empty",
		},
		{
			name: "whitespace host",
			component: ComponentConfig{
				Hosts: []string{"host1.example.com", "  "},
			},
			wantErr: true,
			errMsg:  "host 1 is empty or whitespace",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.component.Validate()
			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errMsg)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestTrueNASConfig_Validate(t *testing.T) {
	tests := []struct {
		name    string
		config  TrueNASConfig
		wantErr bool
		errMsg  string
	}{
		{
			name: "valid truenas config",
			config: TrueNASConfig{
				APIURL: "https://truenas.example.com",
				APIKey: "test-key",
			},
			wantErr: false,
		},
		{
			name: "missing api_url",
			config: TrueNASConfig{
				APIKey: "test-key",
			},
			wantErr: true,
			errMsg:  "truenas api_url is required",
		},
		{
			name: "missing api_key",
			config: TrueNASConfig{
				APIURL: "https://truenas.example.com",
			},
			wantErr: true,
			errMsg:  "truenas api_key is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.Validate()
			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errMsg)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestConfig_UnmarshalYAML(t *testing.T) {
	yamlData := `
version: "1.0"
cluster:
  name: test-cluster
  domain: test.com
  vip: 192.168.1.100

network:
  gateway: 192.168.1.1
  netmask: 255.255.255.0

hosts:
  - hostname: node1
    address: 192.168.1.20
    roles:
      - cluster-control-plane
      - openbao
      - dns
      - zot
    state: configured
  - hostname: node2
    address: 192.168.1.21
    roles:
      - cluster-worker
    state: configured

components:
  k3s:
    version: "v1.28.5+k3s1"
    ha: true
  zot:
    version: "2.0.0"
    storage:
      backend: truenas

observability:
  prometheus:
    retention: 30d
  loki:
    retention: 90d

storage:
  truenas:
    api_url: https://truenas.example.com
    api_key: secret-key
`

	var config Config
	err := yaml.Unmarshal([]byte(yamlData), &config)
	require.NoError(t, err)

	assert.Equal(t, "test-cluster", config.Cluster.Name)
	assert.Equal(t, "test.com", config.Cluster.Domain)
	assert.Equal(t, "192.168.1.100", config.Cluster.VIP)

	// Verify hosts
	assert.Len(t, config.Hosts, 2)
	assert.Equal(t, "node1", config.Hosts[0].Hostname)
	assert.Equal(t, "192.168.1.20", config.Hosts[0].Address)
	assert.Contains(t, config.Hosts[0].Roles, host.RoleClusterControlPlane)
	assert.Equal(t, "node2", config.Hosts[1].Hostname)
	assert.Contains(t, config.Hosts[1].Roles, host.RoleClusterWorker)

	assert.Len(t, config.Components, 2)
	assert.Contains(t, config.Components, "k3s")
	assert.Contains(t, config.Components, "zot")
	require.NotNil(t, config.Components["k3s"].Version)
	assert.Equal(t, "v1.28.5+k3s1", *config.Components["k3s"].Version)

	require.NotNil(t, config.Observability)
	require.NotNil(t, config.Observability.Prometheus)
	require.NotNil(t, config.Observability.Prometheus.Retention)
	assert.Equal(t, "30d", *config.Observability.Prometheus.Retention)

	require.NotNil(t, config.Storage)
	require.NotNil(t, config.Storage.TrueNAS)
	assert.Equal(t, "https://truenas.example.com", config.Storage.TrueNAS.APIURL)
	assert.Equal(t, "secret-key", config.Storage.TrueNAS.APIKey)

	// Validate the unmarshaled config
	err = config.Validate()
	require.NoError(t, err)
}
