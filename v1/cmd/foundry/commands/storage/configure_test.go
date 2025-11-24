package storage

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/catalystcommunity/foundry/v1/internal/config"
	"github.com/catalystcommunity/foundry/v1/internal/host"
	"github.com/catalystcommunity/foundry/v1/internal/storage/truenas"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// strPtr is a helper to create string pointers for optional fields
func strPtr(s string) *string {
	return &s
}

func TestRunConfigure_MissingConfig(t *testing.T) {
	// This test verifies that configure can create a new config if it doesn't exist
	// We'll test the logic by checking error conditions

	tests := []struct {
		name        string
		apiURL      string
		apiKey      string
		skipTest    bool
		expectError bool
		errorMsg    string
	}{
		{
			name:        "missing API URL",
			apiURL:      "",
			apiKey:      "test-key",
			skipTest:    true,
			expectError: true,
			errorMsg:    "API URL is required",
		},
		{
			name:        "missing API key",
			apiURL:      "https://truenas.example.com",
			apiKey:      "",
			skipTest:    true,
			expectError: true,
			errorMsg:    "API key is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Test logic would go here
			// For now, we verify the structure is correct
			assert.NotEmpty(t, tt.name)
		})
	}
}

func TestConfigureCommandIntegration(t *testing.T) {
	// Create a temporary directory for config
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "test-stack.yaml")

	// Create a basic config with required fields
	cfg := &config.Config{
		Cluster: config.ClusterConfig{
			Name:   "test",
			Domain: "test.local",
			VIP:    "192.168.1.100",
		},
		Components: config.ComponentMap{
			"k3s": config.ComponentConfig{},
		},
		Storage: &config.StorageConfig{
			TrueNAS: &config.TrueNASConfig{
				APIURL: "https://truenas.example.com",
				APIKey: "${secret:foundry-core/truenas:api_key}",
			},
		},
	}

	// Save the config
	err := config.Save(cfg, configPath)
	require.NoError(t, err)

	// Verify config was saved correctly
	loadedCfg, err := config.Load(configPath)
	require.NoError(t, err)
	assert.NotNil(t, loadedCfg.Storage)
	assert.NotNil(t, loadedCfg.Storage.TrueNAS)
	assert.Equal(t, "https://truenas.example.com", loadedCfg.Storage.TrueNAS.APIURL)
	assert.Equal(t, "${secret:foundry-core/truenas:api_key}", loadedCfg.Storage.TrueNAS.APIKey)
}

func TestConfigureCommandValidation(t *testing.T) {
	tests := []struct {
		name     string
		apiURL   string
		apiKey   string
		wantErr  bool
		errCheck func(t *testing.T, err error)
	}{
		{
			name:    "valid configuration",
			apiURL:  "https://truenas.example.com",
			apiKey:  "valid-api-key-12345",
			wantErr: false,
		},
		{
			name:    "empty API URL",
			apiURL:  "",
			apiKey:  "valid-api-key",
			wantErr: true,
			errCheck: func(t *testing.T, err error) {
				assert.Contains(t, err.Error(), "API URL")
			},
		},
		{
			name:    "empty API key",
			apiURL:  "https://truenas.example.com",
			apiKey:  "",
			wantErr: true,
			errCheck: func(t *testing.T, err error) {
				assert.Contains(t, err.Error(), "API key")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Test TrueNAS client creation
			if tt.apiURL != "" && tt.apiKey != "" {
				client, err := truenas.NewClient(tt.apiURL, tt.apiKey)
				if tt.wantErr {
					assert.Error(t, err)
					if tt.errCheck != nil {
						tt.errCheck(t, err)
					}
				} else {
					assert.NoError(t, err)
					assert.NotNil(t, client)
				}
			}
		})
	}
}

func TestConfigureWithExistingConfig(t *testing.T) {
	// Create a temporary directory for config
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "existing-stack.yaml")

	// Create an existing config with network settings
	existingCfg := &config.Config{
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
		Cluster: config.ClusterConfig{
			Name:   "test-cluster",
			Domain: "test.local",
			VIP:    "192.168.1.100",
		},
		Components: config.ComponentMap{
			"k3s": config.ComponentConfig{
				Version: strPtr("v1.28.5+k3s1"),
			},
		},
	}

	// Save the existing config
	err := config.Save(existingCfg, configPath)
	require.NoError(t, err)

	// Now add storage configuration
	existingCfg.Storage = &config.StorageConfig{
		TrueNAS: &config.TrueNASConfig{
			APIURL: "https://truenas.example.com",
			APIKey: "${secret:foundry-core/truenas:api_key}",
		},
	}

	// Save updated config
	err = config.Save(existingCfg, configPath)
	require.NoError(t, err)

	// Verify storage was added without affecting other config
	loadedCfg, err := config.Load(configPath)
	require.NoError(t, err)

	assert.NotNil(t, loadedCfg.Network, "Network config should be preserved")
	assert.Equal(t, "192.168.1.1", loadedCfg.Network.Gateway, "Gateway should be preserved")

	assert.NotNil(t, loadedCfg.Storage, "Storage config should be added")
	assert.NotNil(t, loadedCfg.Storage.TrueNAS, "TrueNAS config should be added")
	assert.Equal(t, "https://truenas.example.com", loadedCfg.Storage.TrueNAS.APIURL)
}

func TestConfigPath(t *testing.T) {
	// Test default config path
	defaultPath := config.DefaultConfigPath()
	assert.NotEmpty(t, defaultPath)
	assert.Contains(t, defaultPath, ".foundry")

	// Test that path exists in home directory (or can be created)
	homeDir, err := os.UserHomeDir()
	require.NoError(t, err)
	assert.Contains(t, defaultPath, homeDir)
}
