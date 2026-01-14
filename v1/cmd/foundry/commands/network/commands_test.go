package network

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/catalystcommunity/foundry/v1/internal/config"
	"github.com/catalystcommunity/foundry/v1/internal/host"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNetworkCommandsIntegration(t *testing.T) {
	t.Run("network commands registered", func(t *testing.T) {
		// Test that all commands are registered
		assert.NotNil(t, Command)
		assert.Equal(t, "network", Command.Name)
		assert.Len(t, Command.Commands, 2)

		// Check each subcommand
		var planCmd, validateCmd bool
		for _, cmd := range Command.Commands {
			switch cmd.Name {
			case "plan":
				planCmd = true
			case "validate":
				validateCmd = true
			}
		}

		assert.True(t, planCmd, "plan command should be registered")
		assert.True(t, validateCmd, "validate command should be registered")
	})
}

func TestConfigSaveLoad(t *testing.T) {
	// Create a temporary directory for test configs
	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "test-config.yaml")

	t.Run("save and load config", func(t *testing.T) {
		// Create a test configuration
		cfg := &config.Config{
			Network: &config.NetworkConfig{
				Gateway: "192.168.1.1",
				Netmask: "255.255.255.0",
			},
			Hosts: []*host.Host{
				{
					Hostname: "host1",
					Address:  "192.168.1.10",
					Roles:    []string{host.RoleOpenBAO, host.RoleDNS, host.RoleZot},
					State:    host.StateConfigured,
				},
				{
					Hostname: "node1",
					Address:  "192.168.1.20",
					Roles:    []string{host.RoleClusterControlPlane},
					State:    host.StateConfigured,
				},
			},
			DNS: &config.DNSConfig{
				InfrastructureZones: []config.DNSZone{
					{Name: "infra.local", Public: false},
				},
				KubernetesZones: []config.DNSZone{
					{Name: "k8s.local", Public: false},
				},
				Backend: "sqlite",
				APIKey:  "${secret:foundry-core/dns:api_key}",
			},
			Cluster: config.ClusterConfig{
				Name:   "test-cluster",
				PrimaryDomain: "example.com",
				VIP:    "192.168.1.100",
			},
			Components: config.ComponentMap{
				"openbao": config.ComponentConfig{},
			},
		}

		// Save the configuration
		err := config.Save(cfg, configPath)
		require.NoError(t, err)

		// Verify file exists
		_, err = os.Stat(configPath)
		require.NoError(t, err)

		// Load the configuration
		loaded, err := config.Load(configPath)
		require.NoError(t, err)

		// Verify loaded config matches
		assert.Equal(t, cfg.Network.Gateway, loaded.Network.Gateway)
		assert.Equal(t, cfg.Cluster.VIP, loaded.Cluster.VIP)
		assert.Equal(t, cfg.Cluster.Name, loaded.Cluster.Name)
	})

	t.Run("save creates directory", func(t *testing.T) {
		nestedPath := filepath.Join(tempDir, "nested", "dir", "config.yaml")

		cfg := &config.Config{
			Network: &config.NetworkConfig{
				Gateway: "192.168.1.1",
				Netmask: "255.255.255.0",
			},
			Hosts: []*host.Host{
				{
					Hostname: "host1",
					Address:  "192.168.1.10",
					Roles:    []string{host.RoleOpenBAO, host.RoleDNS, host.RoleZot},
					State:    host.StateConfigured,
				},
			},
			DNS: &config.DNSConfig{
				InfrastructureZones: []config.DNSZone{
					{Name: "infra.local", Public: false},
				},
				KubernetesZones: []config.DNSZone{
					{Name: "k8s.local", Public: false},
				},
				Backend: "sqlite",
				APIKey:  "test-key",
			},
			Cluster: config.ClusterConfig{
				Name:   "test",
				PrimaryDomain: "example.com",
				VIP:    "192.168.1.100",
			},
			Components: config.ComponentMap{
				"test": config.ComponentConfig{},
			},
		}

		err := config.Save(cfg, nestedPath)
		require.NoError(t, err)

		// Verify directory and file exist
		_, err = os.Stat(filepath.Dir(nestedPath))
		require.NoError(t, err)

		_, err = os.Stat(nestedPath)
		require.NoError(t, err)
	})
}

func TestDefaultConfigPath(t *testing.T) {
	path := config.DefaultConfigPath()
	assert.NotEmpty(t, path)
	assert.Contains(t, path, ".foundry")
	assert.Contains(t, path, "stack.yaml")
}
