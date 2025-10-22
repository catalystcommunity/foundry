package secrets

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewOpenBAOResolver(t *testing.T) {
	resolver, err := NewOpenBAOResolver("https://openbao.example.com", "test-token")
	require.NoError(t, err)
	require.NotNil(t, resolver)
	assert.Equal(t, "https://openbao.example.com", resolver.addr)
	assert.Equal(t, "test-token", resolver.token)
}

func TestOpenBAOResolver_Resolve(t *testing.T) {
	resolver, err := NewOpenBAOResolver("https://openbao.example.com", "test-token")
	require.NoError(t, err)

	ctx := NewResolutionContext("myapp-prod")
	ref := SecretRef{Path: "database/main", Key: "password"}

	// Should return not-implemented error
	_, err = resolver.Resolve(ctx, ref)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not yet implemented")
}
