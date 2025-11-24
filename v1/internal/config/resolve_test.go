package config

import (
	"fmt"
	"testing"

	"github.com/catalystcommunity/foundry/v1/internal/secrets"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Mock resolver for testing
type mockSecretResolver struct {
	values map[string]string
}

func (m *mockSecretResolver) Resolve(ctx *secrets.ResolutionContext, ref secrets.SecretRef) (string, error) {
	fullKey := ctx.FullKey(ref)
	value, exists := m.values[fullKey]
	if !exists {
		return "", fmt.Errorf("secret not found: %s", fullKey)
	}
	return value, nil
}

func TestValidateSecretRefs(t *testing.T) {
	tests := []struct {
		name    string
		config  *Config
		wantErr bool
		errMsg  string
	}{
		{
			name: "valid secret refs",
			config: &Config{
				Cluster: ClusterConfig{
					Name:   "test",
					Domain: "example.com",
				},
				Components: ComponentMap{
					"k3s": ComponentConfig{},
				},
				Storage: &StorageConfig{
					TrueNAS: &TrueNASConfig{
						APIURL: "https://truenas.example.com",
						APIKey: "${secret:foundry-core/truenas:api_key}",
					},
				},
			},
			wantErr: false,
		},
		{
			name: "invalid secret ref format",
			config: &Config{
				Cluster: ClusterConfig{
					Name:   "test",
					Domain: "example.com",
				},
				Components: ComponentMap{
					"k3s": ComponentConfig{},
				},
				Storage: &StorageConfig{
					TrueNAS: &TrueNASConfig{
						APIURL: "https://truenas.example.com",
						APIKey: "${secret:invalid}",
					},
				},
			},
			wantErr: true,
			errMsg:  "invalid secret reference",
		},
		{
			name: "no secret refs",
			config: &Config{
				Cluster: ClusterConfig{
					Name:   "test",
					Domain: "example.com",
				},
				Components: ComponentMap{
					"k3s": ComponentConfig{Version: strPtr("v1.28.5")},
				},
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateSecretRefs(tt.config)
			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errMsg)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestResolveSecrets(t *testing.T) {
	tests := []struct {
		name     string
		config   *Config
		ctx      *secrets.ResolutionContext
		resolver *mockSecretResolver
		wantErr  bool
		errMsg   string
		check    func(t *testing.T, cfg *Config)
	}{
		{
			name: "resolve single secret",
			config: &Config{
				Cluster: ClusterConfig{
					Name:   "test",
					Domain: "example.com",
				},
				Components: ComponentMap{
					"k3s": ComponentConfig{},
				},
				Storage: &StorageConfig{
					TrueNAS: &TrueNASConfig{
						APIURL: "https://truenas.example.com",
						APIKey: "${secret:truenas:api_key}",
					},
				},
			},
			ctx: secrets.NewResolutionContext("foundry-core"),
			resolver: &mockSecretResolver{
				values: map[string]string{
					"foundry-core/truenas:api_key": "resolved-key-123",
				},
			},
			wantErr: false,
			check: func(t *testing.T, cfg *Config) {
				require.NotNil(t, cfg.Storage)
				require.NotNil(t, cfg.Storage.TrueNAS)
				assert.Equal(t, "resolved-key-123", cfg.Storage.TrueNAS.APIKey)
			},
		},
		{
			name: "multiple secrets",
			config: &Config{
				Cluster: ClusterConfig{
					Name:   "test",
					Domain: "example.com",
				},
				Components: ComponentMap{
					"k3s": ComponentConfig{},
				},
				Storage: &StorageConfig{
					TrueNAS: &TrueNASConfig{
						APIURL: "${secret:truenas:api_url}",
						APIKey: "${secret:truenas:api_key}",
					},
				},
			},
			ctx: secrets.NewResolutionContext("foundry-core"),
			resolver: &mockSecretResolver{
				values: map[string]string{
					"foundry-core/truenas:api_url": "https://resolved.example.com",
					"foundry-core/truenas:api_key": "resolved-key-456",
				},
			},
			wantErr: false,
			check: func(t *testing.T, cfg *Config) {
				require.NotNil(t, cfg.Storage)
				require.NotNil(t, cfg.Storage.TrueNAS)
				assert.Equal(t, "https://resolved.example.com", cfg.Storage.TrueNAS.APIURL)
				assert.Equal(t, "resolved-key-456", cfg.Storage.TrueNAS.APIKey)
			},
		},
		{
			name: "resolution failure",
			config: &Config{
				Cluster: ClusterConfig{
					Name:   "test",
					Domain: "example.com",
				},
				Components: ComponentMap{
					"k3s": ComponentConfig{},
				},
				Storage: &StorageConfig{
					TrueNAS: &TrueNASConfig{
						APIURL: "https://truenas.example.com",
						APIKey: "${secret:truenas:api_key}",
					},
				},
			},
			ctx: secrets.NewResolutionContext("foundry-core"),
			resolver: &mockSecretResolver{
				values: map[string]string{},
			},
			wantErr: true,
			errMsg:  "failed to resolve secret",
		},
		{
			name: "non-secret values unchanged",
			config: &Config{
				Cluster: ClusterConfig{
					Name:   "test",
					Domain: "example.com",
				},
				Components: ComponentMap{
					"k3s": ComponentConfig{Version: strPtr("v1.28.5")},
				},
			},
			ctx:      secrets.NewResolutionContext("foundry-core"),
			resolver: &mockSecretResolver{values: map[string]string{}},
			wantErr:  false,
			check: func(t *testing.T, cfg *Config) {
				assert.Equal(t, "test", cfg.Cluster.Name)
				require.NotNil(t, cfg.Components["k3s"].Version)
				assert.Equal(t, "v1.28.5", *cfg.Components["k3s"].Version)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ResolveSecrets(tt.config, tt.ctx, tt.resolver)
			if tt.wantErr {
				require.Error(t, err)
				if tt.errMsg != "" {
					assert.Contains(t, err.Error(), tt.errMsg)
				}
			} else {
				require.NoError(t, err)
				if tt.check != nil {
					tt.check(t, tt.config)
				}
			}
		})
	}
}

func TestResolveSecrets_NilParameters(t *testing.T) {
	config := &Config{
		Cluster: ClusterConfig{
			Name:   "test",
			Domain: "example.com",
		},
		Components: ComponentMap{
			"k3s": ComponentConfig{},
		},
	}

	t.Run("nil context", func(t *testing.T) {
		resolver := &mockSecretResolver{values: map[string]string{}}
		err := ResolveSecrets(config, nil, resolver)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "resolution context is required")
	})

	t.Run("nil resolver", func(t *testing.T) {
		ctx := secrets.NewResolutionContext("foundry-core")
		err := ResolveSecrets(config, ctx, nil)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "resolver is required")
	})
}
