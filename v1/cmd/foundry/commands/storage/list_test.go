package storage

import (
	"path/filepath"
	"testing"

	"github.com/catalystcommunity/foundry/v1/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestListCommand_NoConfig(t *testing.T) {
	// Create a temporary directory
	tmpDir := t.TempDir()
	nonExistentPath := filepath.Join(tmpDir, "nonexistent.yaml")

	// Try to load a non-existent config
	_, err := config.Load(nonExistentPath)
	assert.Error(t, err)
	// The error might be wrapped, so just check that it's an error
	assert.NotNil(t, err)
}

func TestListCommand_EmptyStorage(t *testing.T) {
	// Create a config without storage
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "empty-storage.yaml")

	cfg := &config.Config{
		Cluster: config.ClusterConfig{
			Name:          "test",
			PrimaryDomain: "test.local",
			VIP:           "192.168.1.100",
		},
		Components: config.ComponentMap{
			"k3s": config.ComponentConfig{},
		},
	}

	err := config.Save(cfg, configPath)
	require.NoError(t, err)

	// Load and verify
	loadedCfg, err := config.Load(configPath)
	require.NoError(t, err)
	assert.Nil(t, loadedCfg.Storage, "Storage should be nil")
}

func TestListCommand_WithLonghorn(t *testing.T) {
	// Create a config with Longhorn storage
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "with-longhorn.yaml")

	cfg := &config.Config{
		Cluster: config.ClusterConfig{
			Name:          "test",
			PrimaryDomain: "test.local",
			VIP:           "192.168.1.100",
		},
		Components: config.ComponentMap{
			"k3s": config.ComponentConfig{},
		},
		Storage: &config.StorageConfig{
			Backend: "longhorn",
		},
	}

	err := config.Save(cfg, configPath)
	require.NoError(t, err)

	// Load and verify
	loadedCfg, err := config.Load(configPath)
	require.NoError(t, err)
	assert.NotNil(t, loadedCfg.Storage)
	assert.Equal(t, "longhorn", loadedCfg.Storage.Backend)
}

func TestListCommand_WithLocalPath(t *testing.T) {
	// Create a config with local-path storage
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "with-local-path.yaml")

	cfg := &config.Config{
		Cluster: config.ClusterConfig{
			Name:          "test",
			PrimaryDomain: "test.local",
			VIP:           "192.168.1.100",
		},
		Components: config.ComponentMap{
			"k3s": config.ComponentConfig{},
		},
		Storage: &config.StorageConfig{
			Backend: "local-path",
		},
	}

	err := config.Save(cfg, configPath)
	require.NoError(t, err)

	// Load and verify
	loadedCfg, err := config.Load(configPath)
	require.NoError(t, err)
	assert.NotNil(t, loadedCfg.Storage)
	assert.Equal(t, "local-path", loadedCfg.Storage.Backend)
}

func TestListCommand_WithNFS(t *testing.T) {
	// Create a config with NFS storage
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "with-nfs.yaml")

	cfg := &config.Config{
		Cluster: config.ClusterConfig{
			Name:          "test",
			PrimaryDomain: "test.local",
			VIP:           "192.168.1.100",
		},
		Components: config.ComponentMap{
			"k3s": config.ComponentConfig{},
		},
		Storage: &config.StorageConfig{
			Backend: "nfs",
		},
	}

	err := config.Save(cfg, configPath)
	require.NoError(t, err)

	// Load and verify
	loadedCfg, err := config.Load(configPath)
	require.NoError(t, err)
	assert.NotNil(t, loadedCfg.Storage)
	assert.Equal(t, "nfs", loadedCfg.Storage.Backend)
}

func TestStorageBackendTypes(t *testing.T) {
	tests := []struct {
		name    string
		backend string
	}{
		{name: "longhorn", backend: "longhorn"},
		{name: "local-path", backend: "local-path"},
		{name: "nfs", backend: "nfs"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir := t.TempDir()
			configPath := filepath.Join(tmpDir, tt.name+".yaml")

			cfg := &config.Config{
				Cluster: config.ClusterConfig{
					Name:          "test",
					PrimaryDomain: "test.local",
				},
				Components: config.ComponentMap{
					"k3s": config.ComponentConfig{},
				},
				Storage: &config.StorageConfig{
					Backend: tt.backend,
				},
			}

			err := config.Save(cfg, configPath)
			require.NoError(t, err)

			loadedCfg, err := config.Load(configPath)
			require.NoError(t, err)
			assert.Equal(t, tt.backend, loadedCfg.Storage.Backend)
		})
	}
}
