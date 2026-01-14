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
					Name:   "${secret:foundry-core/cluster:name}",
					PrimaryDomain: "example.com",
				},
				Components: ComponentMap{
					"k3s": ComponentConfig{},
				},
			},
			wantErr: false,
		},
		{
			name: "invalid secret ref format",
			config: &Config{
				Cluster: ClusterConfig{
					Name:   "${secret:invalid}",
					PrimaryDomain: "example.com",
				},
				Components: ComponentMap{
					"k3s": ComponentConfig{},
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
					PrimaryDomain: "example.com",
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
					Name:   "${secret:cluster:name}",
					PrimaryDomain: "example.com",
				},
				Components: ComponentMap{
					"k3s": ComponentConfig{},
				},
			},
			ctx: secrets.NewResolutionContext("foundry-core"),
			resolver: &mockSecretResolver{
				values: map[string]string{
					"foundry-core/cluster:name": "resolved-cluster-name",
				},
			},
			wantErr: false,
			check: func(t *testing.T, cfg *Config) {
				assert.Equal(t, "resolved-cluster-name", cfg.Cluster.Name)
			},
		},
		{
			name: "multiple secrets",
			config: &Config{
				Cluster: ClusterConfig{
					Name:   "${secret:cluster:name}",
					PrimaryDomain: "${secret:cluster:primary_domain}",
				},
				Components: ComponentMap{
					"k3s": ComponentConfig{},
				},
			},
			ctx: secrets.NewResolutionContext("foundry-core"),
			resolver: &mockSecretResolver{
				values: map[string]string{
					"foundry-core/cluster:name":   "resolved-name",
					"foundry-core/cluster:primary_domain": "resolved.example.com",
				},
			},
			wantErr: false,
			check: func(t *testing.T, cfg *Config) {
				assert.Equal(t, "resolved-name", cfg.Cluster.Name)
				assert.Equal(t, "resolved.example.com", cfg.Cluster.PrimaryDomain)
			},
		},
		{
			name: "resolution failure",
			config: &Config{
				Cluster: ClusterConfig{
					Name:   "${secret:cluster:name}",
					PrimaryDomain: "example.com",
				},
				Components: ComponentMap{
					"k3s": ComponentConfig{},
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
					PrimaryDomain: "example.com",
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
			PrimaryDomain: "example.com",
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
