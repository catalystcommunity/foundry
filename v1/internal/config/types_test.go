package config

import (
	"testing"

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
					Nodes: []NodeConfig{
						{Hostname: "node1", Role: NodeRoleControlPlane},
					},
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
					Nodes: []NodeConfig{
						{Hostname: "node1", Role: NodeRoleControlPlane},
					},
				},
				Components: ComponentMap{},
			},
			wantErr: true,
			errMsg:  "at least one component must be defined",
		},
		{
			name: "invalid cluster config",
			config: Config{
				Cluster: ClusterConfig{
					Name: "", // Missing name
				},
				Components: ComponentMap{
					"k3s": ComponentConfig{},
				},
			},
			wantErr: true,
			errMsg:  "cluster validation failed",
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

func TestClusterConfig_Validate(t *testing.T) {
	tests := []struct {
		name    string
		cluster ClusterConfig
		wantErr bool
		errMsg  string
	}{
		{
			name: "valid cluster",
			cluster: ClusterConfig{
				Name:   "production",
				Domain: "example.com",
				Nodes: []NodeConfig{
					{Hostname: "node1", Role: NodeRoleControlPlane},
					{Hostname: "node2", Role: NodeRoleWorker},
				},
			},
			wantErr: false,
		},
		{
			name: "missing name",
			cluster: ClusterConfig{
				Domain: "example.com",
				Nodes: []NodeConfig{
					{Hostname: "node1", Role: NodeRoleControlPlane},
				},
			},
			wantErr: true,
			errMsg:  "cluster name is required",
		},
		{
			name: "missing domain",
			cluster: ClusterConfig{
				Name: "production",
				Nodes: []NodeConfig{
					{Hostname: "node1", Role: NodeRoleControlPlane},
				},
			},
			wantErr: true,
			errMsg:  "cluster domain is required",
		},
		{
			name: "no nodes",
			cluster: ClusterConfig{
				Name:   "production",
				Domain: "example.com",
				Nodes:  []NodeConfig{},
			},
			wantErr: true,
			errMsg:  "at least one node must be defined",
		},
		{
			name: "no control-plane node",
			cluster: ClusterConfig{
				Name:   "production",
				Domain: "example.com",
				Nodes: []NodeConfig{
					{Hostname: "node1", Role: NodeRoleWorker},
					{Hostname: "node2", Role: NodeRoleWorker},
				},
			},
			wantErr: true,
			errMsg:  "at least one control-plane node is required",
		},
		{
			name: "invalid node",
			cluster: ClusterConfig{
				Name:   "production",
				Domain: "example.com",
				Nodes: []NodeConfig{
					{Hostname: "", Role: NodeRoleControlPlane},
				},
			},
			wantErr: true,
			errMsg:  "node 0 validation failed",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.cluster.Validate()
			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errMsg)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestNodeConfig_Validate(t *testing.T) {
	tests := []struct {
		name    string
		node    NodeConfig
		wantErr bool
		errMsg  string
	}{
		{
			name: "valid control-plane node",
			node: NodeConfig{
				Hostname: "node1.example.com",
				Role:     NodeRoleControlPlane,
			},
			wantErr: false,
		},
		{
			name: "valid worker node",
			node: NodeConfig{
				Hostname: "node2.example.com",
				Role:     NodeRoleWorker,
			},
			wantErr: false,
		},
		{
			name: "missing hostname",
			node: NodeConfig{
				Role: NodeRoleControlPlane,
			},
			wantErr: true,
			errMsg:  "node hostname is required",
		},
		{
			name: "missing role",
			node: NodeConfig{
				Hostname: "node1.example.com",
			},
			wantErr: true,
			errMsg:  "node role is required",
		},
		{
			name: "invalid role",
			node: NodeConfig{
				Hostname: "node1.example.com",
				Role:     "invalid-role",
			},
			wantErr: true,
			errMsg:  "invalid node role",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.node.Validate()
			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errMsg)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

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
  nodes:
    - hostname: node1
      role: control-plane
    - hostname: node2
      role: worker

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
	assert.Len(t, config.Cluster.Nodes, 2)
	assert.Equal(t, "node1", config.Cluster.Nodes[0].Hostname)
	assert.Equal(t, NodeRoleControlPlane, config.Cluster.Nodes[0].Role)
	assert.Equal(t, "node2", config.Cluster.Nodes[1].Hostname)
	assert.Equal(t, NodeRoleWorker, config.Cluster.Nodes[1].Role)

	assert.Len(t, config.Components, 2)
	assert.Contains(t, config.Components, "k3s")
	assert.Contains(t, config.Components, "zot")
	assert.Equal(t, "v1.28.5+k3s1", config.Components["k3s"].Version)

	require.NotNil(t, config.Observability)
	require.NotNil(t, config.Observability.Prometheus)
	assert.Equal(t, "30d", config.Observability.Prometheus.Retention)

	require.NotNil(t, config.Storage)
	require.NotNil(t, config.Storage.TrueNAS)
	assert.Equal(t, "https://truenas.example.com", config.Storage.TrueNAS.APIURL)
	assert.Equal(t, "secret-key", config.Storage.TrueNAS.APIKey)

	// Validate the unmarshaled config
	err = config.Validate()
	require.NoError(t, err)
}
