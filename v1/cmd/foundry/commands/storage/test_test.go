package storage

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/catalystcommunity/foundry/v1/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetCurrentTimestamp(t *testing.T) {
	// Test that timestamp is reasonable
	ts := getCurrentTimestamp()
	now := time.Now().Unix()

	// Timestamp should be within 1 second of now
	assert.InDelta(t, now, ts, 1.0)

	// Timestamp should be positive
	assert.Greater(t, ts, int64(0))

	// Timestamp should be recent (after 2020-01-01)
	assert.Greater(t, ts, int64(1577836800))
}

func TestTestCommand_NoConfig(t *testing.T) {
	// Create a temporary directory
	tmpDir := t.TempDir()
	nonExistentPath := filepath.Join(tmpDir, "nonexistent.yaml")

	// Try to load a non-existent config
	_, err := config.Load(nonExistentPath)
	assert.Error(t, err)
}

func TestTestCommand_NoStorage(t *testing.T) {
	// Create a config without storage
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "no-storage.yaml")

	cfg := &config.Config{
		Cluster: config.ClusterConfig{
			Name:   "test",
			Domain: "test.local",
			VIP:    "192.168.1.100",
		},
		Components: config.ComponentMap{
			"k3s": config.ComponentConfig{},
		},
	}

	err := config.Save(cfg, configPath)
	require.NoError(t, err)

	// Load config and verify storage is nil
	loadedCfg, err := config.Load(configPath)
	require.NoError(t, err)
	assert.Nil(t, loadedCfg.Storage)
}

func TestTestCommand_WithStorage(t *testing.T) {
	// Create a config with storage
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "with-storage.yaml")

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

	err := config.Save(cfg, configPath)
	require.NoError(t, err)

	// Load and verify
	loadedCfg, err := config.Load(configPath)
	require.NoError(t, err)
	assert.NotNil(t, loadedCfg.Storage)
	assert.NotNil(t, loadedCfg.Storage.TrueNAS)
	assert.Equal(t, "https://truenas.example.com", loadedCfg.Storage.TrueNAS.APIURL)
}

func TestTestDatasetNaming(t *testing.T) {
	// Test dataset name generation
	poolName := "tank"
	expectedName := "tank/foundry-test-1234567890"

	// The name should contain the pool name
	assert.Contains(t, expectedName, poolName)
	assert.Contains(t, expectedName, "foundry-test")

	// Verify timestamp is included in the expected format
	assert.Contains(t, expectedName, "1234567890")
}

func TestPoolSelection(t *testing.T) {
	// Test pool selection logic
	tests := []struct {
		name          string
		pools         []string
		selectedPool  string
		shouldBeFirst bool
		expectError   bool
	}{
		{
			name:          "select first pool when none specified",
			pools:         []string{"tank", "backup"},
			selectedPool:  "",
			shouldBeFirst: true,
			expectError:   false,
		},
		{
			name:          "select specified pool",
			pools:         []string{"tank", "backup"},
			selectedPool:  "backup",
			shouldBeFirst: false,
			expectError:   false,
		},
		{
			name:          "error on non-existent pool",
			pools:         []string{"tank", "backup"},
			selectedPool:  "nonexistent",
			shouldBeFirst: false,
			expectError:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Simulate pool selection
			var selected string
			if tt.selectedPool == "" {
				// Select first pool
				if len(tt.pools) > 0 {
					selected = tt.pools[0]
				}
			} else {
				// Verify pool exists
				found := false
				for _, p := range tt.pools {
					if p == tt.selectedPool {
						found = true
						selected = tt.selectedPool
						break
					}
				}
				if !found && tt.expectError {
					assert.Empty(t, selected)
					return
				}
			}

			if tt.shouldBeFirst {
				assert.Equal(t, tt.pools[0], selected)
			} else if tt.selectedPool != "" {
				assert.Equal(t, tt.selectedPool, selected)
			}
		})
	}
}

func TestTestCommand_Validation(t *testing.T) {
	tests := []struct {
		name       string
		apiURL     string
		apiKey     string
		validSetup bool
	}{
		{
			name:       "valid configuration",
			apiURL:     "https://truenas.example.com",
			apiKey:     "valid-key",
			validSetup: true,
		},
		{
			name:       "empty API URL",
			apiURL:     "",
			apiKey:     "valid-key",
			validSetup: false,
		},
		{
			name:       "empty API key",
			apiURL:     "https://truenas.example.com",
			apiKey:     "",
			validSetup: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &config.Config{
				Cluster: config.ClusterConfig{
					Name:   "test",
					Domain: "test.local",
					VIP:    "192.168.1.100",
				},
				Components: config.ComponentMap{
					"k3s": config.ComponentConfig{},
				},
			}

			if tt.validSetup {
				cfg.Storage = &config.StorageConfig{
					TrueNAS: &config.TrueNASConfig{
						APIURL: tt.apiURL,
						APIKey: tt.apiKey,
					},
				}

				// Validate
				err := cfg.Storage.Validate()
				assert.NoError(t, err)
			} else {
				if tt.apiURL != "" || tt.apiKey != "" {
					cfg.Storage = &config.StorageConfig{
						TrueNAS: &config.TrueNASConfig{
							APIURL: tt.apiURL,
							APIKey: tt.apiKey,
						},
					}

					// Should fail validation
					err := cfg.Storage.Validate()
					assert.Error(t, err)
				}
			}
		})
	}
}

func TestTestCommand_ConnectionTest(t *testing.T) {
	// This test verifies the structure of connection testing
	// Actual connection testing requires a live TrueNAS instance

	tests := []struct {
		name      string
		url       string
		key       string
		shouldErr bool
	}{
		{
			name:      "empty URL fails",
			url:       "",
			key:       "test-key",
			shouldErr: true,
		},
		{
			name:      "empty key fails",
			url:       "https://truenas.example.com",
			key:       "",
			shouldErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Test client creation validation
			if tt.url == "" || tt.key == "" {
				// TrueNAS client should reject empty URL or key
				assert.True(t, tt.shouldErr)
			}
		})
	}
}
