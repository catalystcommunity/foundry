package secrets

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Mock resolver for testing
type mockResolver struct {
	returnValue string
	returnError error
}

func (m *mockResolver) Resolve(ctx *ResolutionContext, ref SecretRef) (string, error) {
	if m.returnError != nil {
		return "", m.returnError
	}
	return m.returnValue, nil
}

func TestNewChainResolver(t *testing.T) {
	r1 := &mockResolver{}
	r2 := &mockResolver{}

	chain := NewChainResolver(r1, r2)
	require.NotNil(t, chain)
	assert.Len(t, chain.resolvers, 2)
}

func TestChainResolver_Resolve(t *testing.T) {
	ctx := NewResolutionContext("myapp-prod")
	ref := SecretRef{Path: "database/main", Key: "password"}

	t.Run("first resolver succeeds", func(t *testing.T) {
		r1 := &mockResolver{returnValue: "value1"}
		r2 := &mockResolver{returnValue: "value2"}

		chain := NewChainResolver(r1, r2)
		value, err := chain.Resolve(ctx, ref)

		require.NoError(t, err)
		assert.Equal(t, "value1", value)
	})

	t.Run("second resolver succeeds after first fails", func(t *testing.T) {
		r1 := &mockResolver{returnError: fmt.Errorf("not found")}
		r2 := &mockResolver{returnValue: "value2"}

		chain := NewChainResolver(r1, r2)
		value, err := chain.Resolve(ctx, ref)

		require.NoError(t, err)
		assert.Equal(t, "value2", value)
	})

	t.Run("all resolvers fail", func(t *testing.T) {
		r1 := &mockResolver{returnError: fmt.Errorf("error1")}
		r2 := &mockResolver{returnError: fmt.Errorf("error2")}
		r3 := &mockResolver{returnError: fmt.Errorf("error3")}

		chain := NewChainResolver(r1, r2, r3)
		_, err := chain.Resolve(ctx, ref)

		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to resolve secret")
		assert.Contains(t, err.Error(), "error1")
		assert.Contains(t, err.Error(), "error2")
		assert.Contains(t, err.Error(), "error3")
	})

	t.Run("no resolvers configured", func(t *testing.T) {
		chain := NewChainResolver()
		_, err := chain.Resolve(ctx, ref)

		require.Error(t, err)
		assert.Contains(t, err.Error(), "no resolvers configured")
	})

	t.Run("third resolver succeeds", func(t *testing.T) {
		r1 := &mockResolver{returnError: fmt.Errorf("not found in env")}
		r2 := &mockResolver{returnError: fmt.Errorf("not found in foundryvars")}
		r3 := &mockResolver{returnValue: "from-openbao"}

		chain := NewChainResolver(r1, r2, r3)
		value, err := chain.Resolve(ctx, ref)

		require.NoError(t, err)
		assert.Equal(t, "from-openbao", value)
	})
}

func TestChainResolver_RealResolvers(t *testing.T) {
	// Test with real resolver implementations
	ctx := NewResolutionContext("test-app")
	ref := SecretRef{Path: "test/secret", Key: "value"}

	t.Run("env resolver in chain", func(t *testing.T) {
		// Set up environment variable
		envVarName := "FOUNDRY_SECRET_TEST_APP_TEST_SECRET_VALUE"
		t.Setenv(envVarName, "env-value")

		envResolver := NewEnvResolver()
		chain := NewChainResolver(envResolver)

		value, err := chain.Resolve(ctx, ref)
		require.NoError(t, err)
		assert.Equal(t, "env-value", value)
	})

	t.Run("fallback through chain", func(t *testing.T) {
		// Create a chain: env (will fail) -> openbao stub (will fail) -> should fail overall
		envResolver := NewEnvResolver()
		openbaoResolver, _ := NewOpenBAOResolver("https://test", "token")

		chain := NewChainResolver(envResolver, openbaoResolver)

		_, err := chain.Resolve(ctx, ref)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to resolve secret")
	})
}
