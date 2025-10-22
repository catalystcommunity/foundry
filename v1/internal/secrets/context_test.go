package secrets

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNewResolutionContext(t *testing.T) {
	ctx := NewResolutionContext("myapp-prod")
	assert.NotNil(t, ctx)
	assert.Equal(t, "myapp-prod", ctx.Instance)
	assert.Equal(t, "", ctx.Namespace)
}

func TestNewResolutionContextWithNamespace(t *testing.T) {
	ctx := NewResolutionContextWithNamespace("myapp-prod", "production")
	assert.NotNil(t, ctx)
	assert.Equal(t, "myapp-prod", ctx.Instance)
	assert.Equal(t, "production", ctx.Namespace)
}

func TestResolutionContext_NamespacedPath(t *testing.T) {
	tests := []struct {
		name     string
		instance string
		ref      SecretRef
		want     string
	}{
		{
			name:     "simple path with instance",
			instance: "myapp-prod",
			ref:      SecretRef{Path: "database/main"},
			want:     "myapp-prod/database/main",
		},
		{
			name:     "nested path with instance",
			instance: "foundry-core",
			ref:      SecretRef{Path: "openbao/tokens"},
			want:     "foundry-core/openbao/tokens",
		},
		{
			name:     "empty instance",
			instance: "",
			ref:      SecretRef{Path: "database/main"},
			want:     "database/main",
		},
		{
			name:     "single level path",
			instance: "myapp-stable",
			ref:      SecretRef{Path: "secret"},
			want:     "myapp-stable/secret",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := NewResolutionContext(tt.instance)
			got := ctx.NamespacedPath(tt.ref)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestResolutionContext_FullKey(t *testing.T) {
	tests := []struct {
		name     string
		instance string
		ref      SecretRef
		want     string
	}{
		{
			name:     "full key with instance",
			instance: "myapp-prod",
			ref:      SecretRef{Path: "database/main", Key: "password"},
			want:     "myapp-prod/database/main:password",
		},
		{
			name:     "foundry-core example",
			instance: "foundry-core",
			ref:      SecretRef{Path: "truenas", Key: "api_key"},
			want:     "foundry-core/truenas:api_key",
		},
		{
			name:     "empty instance",
			instance: "",
			ref:      SecretRef{Path: "database/main", Key: "password"},
			want:     "database/main:password",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := NewResolutionContext(tt.instance)
			got := ctx.FullKey(tt.ref)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestResolutionContext_EnvVarName(t *testing.T) {
	tests := []struct {
		name     string
		instance string
		ref      SecretRef
		want     string
	}{
		{
			name:     "simple env var",
			instance: "myapp-prod",
			ref:      SecretRef{Path: "database/main", Key: "password"},
			want:     "FOUNDRY_SECRET_MYAPP_PROD_DATABASE_MAIN_PASSWORD",
		},
		{
			name:     "with underscores in key",
			instance: "myapp-prod",
			ref:      SecretRef{Path: "database/main", Key: "api_key"},
			want:     "FOUNDRY_SECRET_MYAPP_PROD_DATABASE_MAIN_API_KEY",
		},
		{
			name:     "foundry-core example",
			instance: "foundry-core",
			ref:      SecretRef{Path: "truenas", Key: "api_key"},
			want:     "FOUNDRY_SECRET_FOUNDRY_CORE_TRUENAS_API_KEY",
		},
		{
			name:     "nested path",
			instance: "myservice",
			ref:      SecretRef{Path: "deep/nested/secret", Key: "value"},
			want:     "FOUNDRY_SECRET_MYSERVICE_DEEP_NESTED_SECRET_VALUE",
		},
		{
			name:     "empty instance",
			instance: "",
			ref:      SecretRef{Path: "database/main", Key: "password"},
			want:     "FOUNDRY_SECRET_DATABASE_MAIN_PASSWORD",
		},
		{
			name:     "with many hyphens",
			instance: "my-app-prod",
			ref:      SecretRef{Path: "my-database/my-secret", Key: "my-key"},
			want:     "FOUNDRY_SECRET_MY_APP_PROD_MY_DATABASE_MY_SECRET_MY_KEY",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := NewResolutionContext(tt.instance)
			got := ctx.EnvVarName(tt.ref)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestResolutionContext_RealWorldScenarios(t *testing.T) {
	// Test scenarios from the design document

	t.Run("myapp-prod database secret", func(t *testing.T) {
		ctx := NewResolutionContext("myapp-prod")
		ref := SecretRef{Path: "database/prod", Key: "password"}

		assert.Equal(t, "myapp-prod/database/prod", ctx.NamespacedPath(ref))
		assert.Equal(t, "myapp-prod/database/prod:password", ctx.FullKey(ref))
		assert.Equal(t, "FOUNDRY_SECRET_MYAPP_PROD_DATABASE_PROD_PASSWORD", ctx.EnvVarName(ref))
	})

	t.Run("foundry-core truenas secret", func(t *testing.T) {
		ctx := NewResolutionContext("foundry-core")
		ref := SecretRef{Path: "truenas", Key: "api_key"}

		assert.Equal(t, "foundry-core/truenas", ctx.NamespacedPath(ref))
		assert.Equal(t, "foundry-core/truenas:api_key", ctx.FullKey(ref))
		assert.Equal(t, "FOUNDRY_SECRET_FOUNDRY_CORE_TRUENAS_API_KEY", ctx.EnvVarName(ref))
	})

	t.Run("myapp-stable different from myapp-prod", func(t *testing.T) {
		ctxProd := NewResolutionContext("myapp-prod")
		ctxStable := NewResolutionContext("myapp-stable")
		ref := SecretRef{Path: "database/main", Key: "password"}

		prodPath := ctxProd.NamespacedPath(ref)
		stablePath := ctxStable.NamespacedPath(ref)

		assert.NotEqual(t, prodPath, stablePath)
		assert.Equal(t, "myapp-prod/database/main", prodPath)
		assert.Equal(t, "myapp-stable/database/main", stablePath)
	})
}
