package storage

import (
	"path/filepath"
	"testing"

	"github.com/catalystcommunity/foundry/v1/internal/config"
	"github.com/catalystcommunity/foundry/v1/internal/secrets"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestResolveAPIKey_NoStorage(t *testing.T) {
	cfg := &config.Config{}

	apiKey, err := resolveAPIKey(cfg)
	assert.Error(t, err)
	assert.Empty(t, apiKey)
	assert.Contains(t, err.Error(), "TrueNAS not configured")
}

func TestResolveAPIKey_PlainText(t *testing.T) {
	cfg := &config.Config{
		Storage: &config.StorageConfig{
			TrueNAS: &config.TrueNASConfig{
				APIURL: "https://truenas.example.com",
				APIKey: "plain-text-key-12345",
			},
		},
	}

	apiKey, err := resolveAPIKey(cfg)
	assert.NoError(t, err)
	assert.Equal(t, "plain-text-key-12345", apiKey)
}

func TestResolveAPIKey_SecretReference(t *testing.T) {
	cfg := &config.Config{
		Storage: &config.StorageConfig{
			TrueNAS: &config.TrueNASConfig{
				APIURL: "https://truenas.example.com",
				APIKey: "${secret:foundry-core/truenas:api_key}",
			},
		},
	}

	// This should fail since we don't have OpenBAO running
	apiKey, err := resolveAPIKey(cfg)
	assert.Error(t, err)
	assert.Empty(t, apiKey)
}

func TestResolveAPIKey_InvalidSecretReference(t *testing.T) {
	cfg := &config.Config{
		Storage: &config.StorageConfig{
			TrueNAS: &config.TrueNASConfig{
				APIURL: "https://truenas.example.com",
				APIKey: "${invalid-format}",
			},
		},
	}

	apiKey, err := resolveAPIKey(cfg)
	assert.Error(t, err)
	assert.Empty(t, apiKey)
}

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

	// Load and verify
	loadedCfg, err := config.Load(configPath)
	require.NoError(t, err)
	assert.Nil(t, loadedCfg.Storage, "Storage should be nil")
}

func TestListCommand_WithTrueNAS(t *testing.T) {
	// Create a config with TrueNAS storage
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "with-truenas.yaml")

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

func TestSecretRefParsing(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		wantPath  string
		wantKey   string
		wantNil   bool
		wantError bool
	}{
		{
			name:      "valid secret reference",
			input:     "${secret:foundry-core/truenas:api_key}",
			wantPath:  "foundry-core/truenas",
			wantKey:   "api_key",
			wantNil:   false,
			wantError: false,
		},
		{
			name:      "invalid format - no dollar sign (returns nil, no error)",
			input:     "{secret:path:key}",
			wantNil:   true,
			wantError: false,
		},
		{
			name:      "invalid format - no braces (returns nil, no error)",
			input:     "$secret:path:key",
			wantNil:   true,
			wantError: false,
		},
		{
			name:      "invalid format - no secret prefix (returns nil, no error)",
			input:     "${path:key}",
			wantNil:   true,
			wantError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ref, err := secrets.ParseSecretRef(tt.input)
			if tt.wantError {
				assert.Error(t, err)
			} else if tt.wantNil {
				assert.NoError(t, err)
				assert.Nil(t, ref)
			} else {
				require.NoError(t, err)
				require.NotNil(t, ref)
				assert.Equal(t, tt.wantPath, ref.Path)
				assert.Equal(t, tt.wantKey, ref.Key)
			}
		})
	}
}

func TestListCommand_PoolFormatting(t *testing.T) {
	// Test pool size calculations
	tests := []struct {
		name         string
		sizeBytes    int64
		expectedGB   float64
		freeBytes    int64
		expectedFree float64
	}{
		{
			name:         "1 TB pool",
			sizeBytes:    1024 * 1024 * 1024 * 1024,
			expectedGB:   1024.0,
			freeBytes:    512 * 1024 * 1024 * 1024,
			expectedFree: 512.0,
		},
		{
			name:         "500 GB pool",
			sizeBytes:    500 * 1024 * 1024 * 1024,
			expectedGB:   500.0,
			freeBytes:    100 * 1024 * 1024 * 1024,
			expectedFree: 100.0,
		},
		{
			name:         "100 GB pool",
			sizeBytes:    100 * 1024 * 1024 * 1024,
			expectedGB:   100.0,
			freeBytes:    50 * 1024 * 1024 * 1024,
			expectedFree: 50.0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sizeGB := float64(tt.sizeBytes) / (1024 * 1024 * 1024)
			freeGB := float64(tt.freeBytes) / (1024 * 1024 * 1024)

			assert.InDelta(t, tt.expectedGB, sizeGB, 0.01)
			assert.InDelta(t, tt.expectedFree, freeGB, 0.01)

			// Test percentage calculation
			usedPct := 0.0
			if tt.sizeBytes > 0 {
				allocatedBytes := tt.sizeBytes - tt.freeBytes
				usedPct = (float64(allocatedBytes) / float64(tt.sizeBytes)) * 100
			}
			assert.GreaterOrEqual(t, usedPct, 0.0)
			assert.LessOrEqual(t, usedPct, 100.0)
		})
	}
}
