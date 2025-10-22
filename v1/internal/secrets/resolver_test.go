package secrets

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEnvResolver_Resolve(t *testing.T) {
	resolver := NewEnvResolver()

	tests := []struct {
		name       string
		setupEnv   func()
		cleanupEnv func()
		ctx        *ResolutionContext
		ref        SecretRef
		want       string
		wantErr    bool
		errMsg     string
	}{
		{
			name: "resolve from environment",
			setupEnv: func() {
				os.Setenv("FOUNDRY_SECRET_MYAPP_PROD_DATABASE_MAIN_PASSWORD", "secret123")
			},
			cleanupEnv: func() {
				os.Unsetenv("FOUNDRY_SECRET_MYAPP_PROD_DATABASE_MAIN_PASSWORD")
			},
			ctx:     NewResolutionContext("myapp-prod"),
			ref:     SecretRef{Path: "database/main", Key: "password"},
			want:    "secret123",
			wantErr: false,
		},
		{
			name: "environment variable not set",
			setupEnv: func() {
				// Don't set the env var
			},
			cleanupEnv: func() {},
			ctx:        NewResolutionContext("myapp-prod"),
			ref:        SecretRef{Path: "database/main", Key: "password"},
			wantErr:    true,
			errMsg:     "environment variable",
		},
		{
			name:       "nil context",
			setupEnv:   func() {},
			cleanupEnv: func() {},
			ctx:        nil,
			ref:        SecretRef{Path: "database/main", Key: "password"},
			wantErr:    true,
			errMsg:     "resolution context is required",
		},
		{
			name: "foundry-core example",
			setupEnv: func() {
				os.Setenv("FOUNDRY_SECRET_FOUNDRY_CORE_TRUENAS_API_KEY", "truenas-key-456")
			},
			cleanupEnv: func() {
				os.Unsetenv("FOUNDRY_SECRET_FOUNDRY_CORE_TRUENAS_API_KEY")
			},
			ctx:     NewResolutionContext("foundry-core"),
			ref:     SecretRef{Path: "truenas", Key: "api_key"},
			want:    "truenas-key-456",
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.setupEnv != nil {
				tt.setupEnv()
			}
			if tt.cleanupEnv != nil {
				defer tt.cleanupEnv()
			}

			got, err := resolver.Resolve(tt.ctx, tt.ref)

			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errMsg)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.want, got)
			}
		})
	}
}

func TestEnvResolver_MultipleInstances(t *testing.T) {
	resolver := NewEnvResolver()

	// Set up env vars for two different instances of the same app
	os.Setenv("FOUNDRY_SECRET_MYAPP_PROD_DATABASE_MAIN_PASSWORD", "prod-password")
	os.Setenv("FOUNDRY_SECRET_MYAPP_STABLE_DATABASE_MAIN_PASSWORD", "stable-password")
	defer func() {
		os.Unsetenv("FOUNDRY_SECRET_MYAPP_PROD_DATABASE_MAIN_PASSWORD")
		os.Unsetenv("FOUNDRY_SECRET_MYAPP_STABLE_DATABASE_MAIN_PASSWORD")
	}()

	ref := SecretRef{Path: "database/main", Key: "password"}

	// Resolve for prod instance
	prodCtx := NewResolutionContext("myapp-prod")
	prodValue, err := resolver.Resolve(prodCtx, ref)
	require.NoError(t, err)
	assert.Equal(t, "prod-password", prodValue)

	// Resolve for stable instance
	stableCtx := NewResolutionContext("myapp-stable")
	stableValue, err := resolver.Resolve(stableCtx, ref)
	require.NoError(t, err)
	assert.Equal(t, "stable-password", stableValue)

	// Verify they're different
	assert.NotEqual(t, prodValue, stableValue)
}
