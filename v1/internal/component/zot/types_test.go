package zot

import (
	"testing"

	"github.com/catalystcommunity/foundry/v1/internal/component"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDefaultConfig(t *testing.T) {
	config := DefaultConfig()

	assert.Equal(t, "latest", config.Version)
	assert.Equal(t, "/var/lib/foundry-zot", config.DataDir)
	assert.Equal(t, "/etc/foundry-zot", config.ConfigDir)
	assert.Equal(t, 5000, config.Port)
	assert.True(t, config.PullThroughCache)
	assert.Nil(t, config.Auth)
	assert.Nil(t, config.StorageBackend)
}

func TestParseConfig_Defaults(t *testing.T) {
	cfg := component.ComponentConfig{}

	config, err := ParseConfig(cfg)
	require.NoError(t, err)

	assert.Equal(t, "latest", config.Version)
	assert.Equal(t, "/var/lib/foundry-zot", config.DataDir)
	assert.Equal(t, "/etc/foundry-zot", config.ConfigDir)
	assert.Equal(t, 5000, config.Port)
	assert.True(t, config.PullThroughCache)
}

func TestParseConfig_CustomValues(t *testing.T) {
	cfg := component.ComponentConfig{
		"version":            "v2.0.0",
		"data_dir":           "/custom/data",
		"config_dir":         "/custom/config",
		"port":               5050,
		"pull_through_cache": false,
	}

	config, err := ParseConfig(cfg)
	require.NoError(t, err)

	assert.Equal(t, "v2.0.0", config.Version)
	assert.Equal(t, "/custom/data", config.DataDir)
	assert.Equal(t, "/custom/config", config.ConfigDir)
	assert.Equal(t, 5050, config.Port)
	assert.False(t, config.PullThroughCache)
}

func TestParseConfig_PortAsFloat(t *testing.T) {
	// YAML unmarshaling sometimes produces float64 for numbers
	cfg := component.ComponentConfig{
		"port": float64(5050),
	}

	config, err := ParseConfig(cfg)
	require.NoError(t, err)

	assert.Equal(t, 5050, config.Port)
}

func TestParseConfig_WithStorage(t *testing.T) {
	cfg := component.ComponentConfig{
		"storage": map[string]interface{}{
			"type":       "truenas",
			"mount_path": "/mnt/truenas/zot",
		},
	}

	config, err := ParseConfig(cfg)
	require.NoError(t, err)

	require.NotNil(t, config.StorageBackend)
	assert.Equal(t, "truenas", config.StorageBackend.Type)
	assert.Equal(t, "/mnt/truenas/zot", config.StorageBackend.MountPath)
}

func TestParseConfig_WithAuth(t *testing.T) {
	cfg := component.ComponentConfig{
		"auth": map[string]interface{}{
			"type": "basic",
			"config": map[string]interface{}{
				"htpasswd_path": "/etc/zot/htpasswd",
			},
		},
	}

	config, err := ParseConfig(cfg)
	require.NoError(t, err)

	require.NotNil(t, config.Auth)
	assert.Equal(t, "basic", config.Auth.Type)
	require.NotNil(t, config.Auth.Config)
	assert.Equal(t, "/etc/zot/htpasswd", config.Auth.Config["htpasswd_path"])
}

func TestParseConfig_Complete(t *testing.T) {
	cfg := component.ComponentConfig{
		"version":            "v2.0.0",
		"data_dir":           "/custom/data",
		"config_dir":         "/custom/config",
		"port":               5050,
		"pull_through_cache": false,
		"storage": map[string]interface{}{
			"type":       "truenas",
			"mount_path": "/mnt/truenas/zot",
		},
		"auth": map[string]interface{}{
			"type": "basic",
			"config": map[string]interface{}{
				"htpasswd_path": "/etc/zot/htpasswd",
			},
		},
	}

	config, err := ParseConfig(cfg)
	require.NoError(t, err)

	assert.Equal(t, "v2.0.0", config.Version)
	assert.Equal(t, "/custom/data", config.DataDir)
	assert.Equal(t, "/custom/config", config.ConfigDir)
	assert.Equal(t, 5050, config.Port)
	assert.False(t, config.PullThroughCache)

	require.NotNil(t, config.StorageBackend)
	assert.Equal(t, "truenas", config.StorageBackend.Type)
	assert.Equal(t, "/mnt/truenas/zot", config.StorageBackend.MountPath)

	require.NotNil(t, config.Auth)
	assert.Equal(t, "basic", config.Auth.Type)
	require.NotNil(t, config.Auth.Config)
	assert.Equal(t, "/etc/zot/htpasswd", config.Auth.Config["htpasswd_path"])
}

func TestComponent_Name(t *testing.T) {
	comp := NewComponent(nil)
	assert.Equal(t, "zot", comp.Name())
}
