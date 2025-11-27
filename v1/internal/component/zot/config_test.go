package zot

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGenerateConfig_Defaults(t *testing.T) {
	cfg := DefaultConfig()

	configStr, err := GenerateConfig(cfg)
	require.NoError(t, err)
	require.NotEmpty(t, configStr)

	// Parse the JSON to validate structure
	var zotConfig ZotConfig
	err = json.Unmarshal([]byte(configStr), &zotConfig)
	require.NoError(t, err)

	// Verify basic structure
	assert.Equal(t, "1.1.0", zotConfig.DistSpecVersion)
	assert.Equal(t, "/var/lib/foundry-zot", zotConfig.Storage.RootDirectory)
	assert.True(t, zotConfig.Storage.GC)
	assert.True(t, zotConfig.Storage.Dedupe)
	assert.Equal(t, "0.0.0.0", zotConfig.HTTP.Address)
	assert.Equal(t, "5000", zotConfig.HTTP.Port)
	assert.Equal(t, "info", zotConfig.Log.Level)
	assert.Equal(t, "/dev/stdout", zotConfig.Log.Output)
}

func TestGenerateConfig_WithPullThroughCache(t *testing.T) {
	cfg := DefaultConfig()
	cfg.PullThroughCache = true

	configStr, err := GenerateConfig(cfg)
	require.NoError(t, err)

	var zotConfig ZotConfig
	err = json.Unmarshal([]byte(configStr), &zotConfig)
	require.NoError(t, err)

	// Verify extensions are present
	require.NotNil(t, zotConfig.Extensions)
	require.NotNil(t, zotConfig.Extensions.Sync)
	assert.True(t, zotConfig.Extensions.Sync.Enable)

	// Verify registries (Docker Hub and GHCR)
	require.Len(t, zotConfig.Extensions.Sync.Registries, 2)

	// Docker Hub registry
	dockerRegistry := zotConfig.Extensions.Sync.Registries[0]
	assert.Equal(t, []string{"https://registry-1.docker.io"}, dockerRegistry.URLs)
	assert.True(t, dockerRegistry.TLSVerify)
	assert.True(t, dockerRegistry.OnDemand)

	// GHCR registry
	ghcrRegistry := zotConfig.Extensions.Sync.Registries[1]
	assert.Equal(t, []string{"https://ghcr.io"}, ghcrRegistry.URLs)
	assert.True(t, ghcrRegistry.TLSVerify)
	assert.True(t, ghcrRegistry.OnDemand)

	// Verify search and UI are enabled
	require.NotNil(t, zotConfig.Extensions.Search)
	assert.True(t, zotConfig.Extensions.Search.Enable)
	require.NotNil(t, zotConfig.Extensions.UI)
	assert.True(t, zotConfig.Extensions.UI.Enable)
}

func TestGenerateConfig_WithoutPullThroughCache(t *testing.T) {
	cfg := DefaultConfig()
	cfg.PullThroughCache = false

	configStr, err := GenerateConfig(cfg)
	require.NoError(t, err)

	var zotConfig ZotConfig
	err = json.Unmarshal([]byte(configStr), &zotConfig)
	require.NoError(t, err)

	// Extensions should be nil when pull-through cache is disabled
	assert.Nil(t, zotConfig.Extensions)
}

func TestGenerateConfig_CustomPort(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Port = 5050

	configStr, err := GenerateConfig(cfg)
	require.NoError(t, err)

	var zotConfig ZotConfig
	err = json.Unmarshal([]byte(configStr), &zotConfig)
	require.NoError(t, err)

	assert.Equal(t, "5050", zotConfig.HTTP.Port)
}

func TestGenerateConfig_CustomDataDir(t *testing.T) {
	cfg := DefaultConfig()
	cfg.DataDir = "/custom/data/path"

	configStr, err := GenerateConfig(cfg)
	require.NoError(t, err)

	var zotConfig ZotConfig
	err = json.Unmarshal([]byte(configStr), &zotConfig)
	require.NoError(t, err)

	assert.Equal(t, "/custom/data/path", zotConfig.Storage.RootDirectory)
}

func TestGenerateConfig_WithBasicAuth(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Auth = &AuthConfig{
		Type: "basic",
		Config: map[string]interface{}{
			"htpasswd_path": "/etc/zot/htpasswd",
		},
	}

	configStr, err := GenerateConfig(cfg)
	require.NoError(t, err)

	var zotConfig ZotConfig
	err = json.Unmarshal([]byte(configStr), &zotConfig)
	require.NoError(t, err)

	// Verify auth is configured
	require.NotNil(t, zotConfig.HTTP.Auth)
	assert.Equal(t, "/etc/zot/htpasswd", zotConfig.HTTP.Auth.HTPasswd.Path)
}

func TestGenerateConfig_WithUnknownAuth(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Auth = &AuthConfig{
		Type: "unknown",
		Config: map[string]interface{}{
			"some_key": "some_value",
		},
	}

	// Should not error, just ignore unknown auth type
	configStr, err := GenerateConfig(cfg)
	require.NoError(t, err)

	var zotConfig ZotConfig
	err = json.Unmarshal([]byte(configStr), &zotConfig)
	require.NoError(t, err)

	// Auth should be nil for unknown type
	assert.Nil(t, zotConfig.HTTP.Auth)
}

func TestGenerateConfig_ValidJSON(t *testing.T) {
	cfg := &Config{
		Version:          "v2.0.0",
		DataDir:          "/custom/data",
		ConfigDir:        "/custom/config",
		Port:             5050,
		PullThroughCache: true,
		Auth: &AuthConfig{
			Type: "basic",
			Config: map[string]interface{}{
				"htpasswd_path": "/etc/zot/htpasswd",
			},
		},
	}

	configStr, err := GenerateConfig(cfg)
	require.NoError(t, err)

	// Should be valid JSON
	var result map[string]interface{}
	err = json.Unmarshal([]byte(configStr), &result)
	require.NoError(t, err)

	// Should be pretty-printed (contains newlines and indentation)
	assert.Contains(t, configStr, "\n")
	assert.Contains(t, configStr, "  ")
}

func TestGenerateConfig_CompleteConfiguration(t *testing.T) {
	cfg := &Config{
		Version:          "v2.0.0",
		DataDir:          "/var/lib/zot",
		ConfigDir:        "/etc/zot",
		Port:             5000,
		PullThroughCache: true,
		StorageBackend: &StorageConfig{
			Type:      "truenas",
			MountPath: "/mnt/truenas/zot",
		},
		Auth: &AuthConfig{
			Type: "basic",
			Config: map[string]interface{}{
				"htpasswd_path": "/etc/zot/htpasswd",
			},
		},
	}

	configStr, err := GenerateConfig(cfg)
	require.NoError(t, err)

	var zotConfig ZotConfig
	err = json.Unmarshal([]byte(configStr), &zotConfig)
	require.NoError(t, err)

	// Verify all major sections
	assert.Equal(t, "1.1.0", zotConfig.DistSpecVersion)
	assert.Equal(t, "/var/lib/zot", zotConfig.Storage.RootDirectory)
	assert.Equal(t, "5000", zotConfig.HTTP.Port)
	assert.NotNil(t, zotConfig.Extensions)
	assert.NotNil(t, zotConfig.Extensions.Sync)
	assert.NotNil(t, zotConfig.HTTP.Auth)
}
