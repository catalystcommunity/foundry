package k3s

import (
	"context"
	"testing"

	"github.com/catalystcommunity/foundry/v1/internal/component"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestComponent_Name(t *testing.T) {
	c := &Component{}
	assert.Equal(t, "k3s", c.Name())
}

func TestComponent_Dependencies(t *testing.T) {
	c := &Component{}
	deps := c.Dependencies()

	assert.Len(t, deps, 3)
	assert.Contains(t, deps, "openbao")
	assert.Contains(t, deps, "dns")
	assert.Contains(t, deps, "zot")
}

func TestComponent_NotImplemented(t *testing.T) {
	c := &Component{}
	ctx := context.Background()
	cfg := component.ComponentConfig{}

	// These should return not implemented errors for now
	err := c.Install(ctx, cfg)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not implemented")

	err = c.Upgrade(ctx, cfg)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not implemented")

	_, err = c.Status(ctx)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not implemented")

	err = c.Uninstall(ctx)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not implemented")
}

func TestParseConfig(t *testing.T) {
	tests := []struct {
		name    string
		input   component.ComponentConfig
		want    *Config
		wantErr bool
	}{
		{
			name: "minimal config",
			input: component.ComponentConfig{
				"vip": "192.168.1.100",
			},
			want: &Config{
				VIP:               "192.168.1.100",
				DisableComponents: []string{"traefik", "servicelb"},
			},
			wantErr: false,
		},
		{
			name: "full config",
			input: component.ComponentConfig{
				"version":        "v1.28.5+k3s1",
				"vip":            "192.168.1.100",
				"interface":      "eth0",
				"cluster_token":  "test-cluster-token",
				"agent_token":    "test-agent-token",
				"tls_sans":       []interface{}{"example.com", "api.example.com"},
				"disable":        []interface{}{"traefik", "servicelb", "local-storage"},
				"registry_config": "mirrors:\n  docker.io:\n    endpoint:\n      - http://zot:5000",
				"cluster_init":   true,
				"dns_servers":    []interface{}{"192.168.1.10", "8.8.8.8"},
			},
			want: &Config{
				Version:           "v1.28.5+k3s1",
				VIP:               "192.168.1.100",
				Interface:         "eth0",
				ClusterToken:      "test-cluster-token",
				AgentToken:        "test-agent-token",
				TLSSANs:           []string{"example.com", "api.example.com"},
				DisableComponents: []string{"traefik", "servicelb", "local-storage"},
				RegistryConfig:    "mirrors:\n  docker.io:\n    endpoint:\n      - http://zot:5000",
				ClusterInit:       true,
				DNSServers:        []string{"192.168.1.10", "8.8.8.8"},
			},
			wantErr: false,
		},
		{
			name: "joining node config",
			input: component.ComponentConfig{
				"vip":         "192.168.1.100",
				"server_url":  "https://192.168.1.100:6443",
				"agent_token": "test-agent-token",
			},
			want: &Config{
				VIP:               "192.168.1.100",
				ServerURL:         "https://192.168.1.100:6443",
				AgentToken:        "test-agent-token",
				DisableComponents: []string{"traefik", "servicelb"},
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseConfig(tt.input)
			if tt.wantErr {
				assert.Error(t, err)
				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.want.Version, got.Version)
			assert.Equal(t, tt.want.VIP, got.VIP)
			assert.Equal(t, tt.want.Interface, got.Interface)
			assert.Equal(t, tt.want.ClusterToken, got.ClusterToken)
			assert.Equal(t, tt.want.AgentToken, got.AgentToken)
			assert.Equal(t, tt.want.TLSSANs, got.TLSSANs)
			assert.Equal(t, tt.want.DisableComponents, got.DisableComponents)
			assert.Equal(t, tt.want.RegistryConfig, got.RegistryConfig)
			assert.Equal(t, tt.want.ClusterInit, got.ClusterInit)
			assert.Equal(t, tt.want.ServerURL, got.ServerURL)
			assert.Equal(t, tt.want.DNSServers, got.DNSServers)
		})
	}
}

func TestConfig_Validate(t *testing.T) {
	tests := []struct {
		name    string
		config  *Config
		wantErr bool
		errMsg  string
	}{
		{
			name: "valid first control plane",
			config: &Config{
				VIP:         "192.168.1.100",
				ClusterInit: true,
			},
			wantErr: false,
		},
		{
			name: "valid joining node with cluster token",
			config: &Config{
				VIP:          "192.168.1.100",
				ServerURL:    "https://192.168.1.100:6443",
				ClusterToken: "test-token",
				ClusterInit:  false,
			},
			wantErr: false,
		},
		{
			name: "valid joining node with agent token",
			config: &Config{
				VIP:        "192.168.1.100",
				ServerURL:  "https://192.168.1.100:6443",
				AgentToken: "test-token",
				ClusterInit: false,
			},
			wantErr: false,
		},
		{
			name: "missing VIP",
			config: &Config{
				ClusterInit: true,
			},
			wantErr: true,
			errMsg:  "VIP is required",
		},
		{
			name: "invalid VIP format",
			config: &Config{
				VIP:         "not-an-ip",
				ClusterInit: true,
			},
			wantErr: true,
			errMsg:  "VIP validation failed",
		},
		{
			name: "missing server URL when joining",
			config: &Config{
				VIP:          "192.168.1.100",
				ClusterToken: "test-token",
				ClusterInit:  false,
			},
			wantErr: true,
			errMsg:  "server_url is required",
		},
		{
			name: "missing tokens when joining",
			config: &Config{
				VIP:         "192.168.1.100",
				ServerURL:   "https://192.168.1.100:6443",
				ClusterInit: false,
			},
			wantErr: true,
			errMsg:  "either cluster_token or agent_token is required",
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
