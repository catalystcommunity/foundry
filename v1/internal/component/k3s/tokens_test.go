package k3s

import (
	"context"
	"encoding/base64"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGenerateToken(t *testing.T) {
	t.Run("generates valid token", func(t *testing.T) {
		token, err := GenerateToken()
		require.NoError(t, err)
		assert.NotEmpty(t, token)

		// Verify it's valid base64
		decoded, err := base64.RawURLEncoding.DecodeString(token)
		require.NoError(t, err)
		assert.Len(t, decoded, TokenLength, "decoded token should be exactly TokenLength bytes")
	})

	t.Run("generates unique tokens", func(t *testing.T) {
		token1, err := GenerateToken()
		require.NoError(t, err)

		token2, err := GenerateToken()
		require.NoError(t, err)

		assert.NotEqual(t, token1, token2, "consecutive tokens should be different")
	})

	t.Run("generates tokens of consistent length", func(t *testing.T) {
		for i := 0; i < 10; i++ {
			token, err := GenerateToken()
			require.NoError(t, err)

			decoded, err := base64.RawURLEncoding.DecodeString(token)
			require.NoError(t, err)
			assert.Len(t, decoded, TokenLength)
		}
	})
}

func TestGenerateTokens(t *testing.T) {
	t.Run("generates both tokens", func(t *testing.T) {
		tokens, err := GenerateTokens()
		require.NoError(t, err)
		require.NotNil(t, tokens)

		assert.NotEmpty(t, tokens.ClusterToken)
		assert.NotEmpty(t, tokens.AgentToken)
	})

	t.Run("generates different tokens for cluster and agent", func(t *testing.T) {
		tokens, err := GenerateTokens()
		require.NoError(t, err)

		assert.NotEqual(t, tokens.ClusterToken, tokens.AgentToken,
			"cluster and agent tokens should be different")
	})

	t.Run("generates valid base64 tokens", func(t *testing.T) {
		tokens, err := GenerateTokens()
		require.NoError(t, err)

		// Verify cluster token is valid base64
		decoded, err := base64.RawURLEncoding.DecodeString(tokens.ClusterToken)
		require.NoError(t, err)
		assert.Len(t, decoded, TokenLength)

		// Verify agent token is valid base64
		decoded, err = base64.RawURLEncoding.DecodeString(tokens.AgentToken)
		require.NoError(t, err)
		assert.Len(t, decoded, TokenLength)
	})
}

func TestStoreTokens(t *testing.T) {
	// Create a mock OpenBAO client
	mockClient := &mockOpenBAOClient{
		secrets: make(map[string]map[string]interface{}),
	}

	ctx := context.Background()

	t.Run("stores both tokens successfully", func(t *testing.T) {
		tokens := &Tokens{
			ClusterToken: "test-cluster-token",
			AgentToken:   "test-agent-token",
		}

		err := StoreTokens(ctx, mockClient, tokens)
		require.NoError(t, err)

		// Verify cluster token was stored
		clusterKey := SecretMount + "/data/" + ClusterTokenPath
		assert.Contains(t, mockClient.secrets, clusterKey)
		assert.Equal(t, "test-cluster-token", mockClient.secrets[clusterKey]["token"])

		// Verify agent token was stored
		agentKey := SecretMount + "/data/" + AgentTokenPath
		assert.Contains(t, mockClient.secrets, agentKey)
		assert.Equal(t, "test-agent-token", mockClient.secrets[agentKey]["token"])
	})

	t.Run("returns error for nil tokens", func(t *testing.T) {
		err := StoreTokens(ctx, mockClient, nil)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "tokens cannot be nil")
	})

	t.Run("returns error for empty cluster token", func(t *testing.T) {
		tokens := &Tokens{
			ClusterToken: "",
			AgentToken:   "test-agent-token",
		}

		err := StoreTokens(ctx, mockClient, tokens)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "cluster token cannot be empty")
	})

	t.Run("returns error for empty agent token", func(t *testing.T) {
		tokens := &Tokens{
			ClusterToken: "test-cluster-token",
			AgentToken:   "",
		}

		err := StoreTokens(ctx, mockClient, tokens)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "agent token cannot be empty")
	})

	t.Run("returns error when client fails to write cluster token", func(t *testing.T) {
		failingClient := &mockOpenBAOClient{
			secrets:   make(map[string]map[string]interface{}),
			writeErr:  assert.AnError,
			failPaths: []string{SecretMount + "/data/" + ClusterTokenPath},
		}

		tokens := &Tokens{
			ClusterToken: "test-cluster-token",
			AgentToken:   "test-agent-token",
		}

		err := StoreTokens(ctx, failingClient, tokens)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to store cluster token")
	})

	t.Run("returns error when client fails to write agent token", func(t *testing.T) {
		// Use a fresh mock that doesn't have writeErr set globally
		// but only fails for the specific agent token path
		failingClient := &mockOpenBAOClient{
			secrets:   make(map[string]map[string]interface{}),
			failPaths: []string{SecretMount + "/data/" + AgentTokenPath},
			// Set writeErr for use only with failPaths
			writeErr: assert.AnError,
		}

		tokens := &Tokens{
			ClusterToken: "test-cluster-token",
			AgentToken:   "test-agent-token",
		}

		err := StoreTokens(ctx, failingClient, tokens)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to store agent token")
	})
}

func TestLoadTokens(t *testing.T) {
	ctx := context.Background()

	t.Run("loads both tokens successfully", func(t *testing.T) {
		mockClient := &mockOpenBAOClient{
			secrets: map[string]map[string]interface{}{
				SecretMount + "/data/" + ClusterTokenPath: {
					"token": "test-cluster-token",
				},
				SecretMount + "/data/" + AgentTokenPath: {
					"token": "test-agent-token",
				},
			},
		}

		tokens, err := LoadTokens(ctx, mockClient)
		require.NoError(t, err)
		require.NotNil(t, tokens)

		assert.Equal(t, "test-cluster-token", tokens.ClusterToken)
		assert.Equal(t, "test-agent-token", tokens.AgentToken)
	})

	t.Run("returns error when cluster token not found", func(t *testing.T) {
		mockClient := &mockOpenBAOClient{
			secrets: make(map[string]map[string]interface{}),
			readErr: assert.AnError,
		}

		tokens, err := LoadTokens(ctx, mockClient)
		assert.Error(t, err)
		assert.Nil(t, tokens)
		assert.Contains(t, err.Error(), "failed to load cluster token")
	})

	t.Run("returns error when agent token not found", func(t *testing.T) {
		mockClient := &mockOpenBAOClient{
			secrets: map[string]map[string]interface{}{
				SecretMount + "/data/" + ClusterTokenPath: {
					"token": "test-cluster-token",
				},
			},
			readErr:   assert.AnError,
			failPaths: []string{SecretMount + "/data/" + AgentTokenPath},
		}

		tokens, err := LoadTokens(ctx, mockClient)
		assert.Error(t, err)
		assert.Nil(t, tokens)
		assert.Contains(t, err.Error(), "failed to load agent token")
	})

	t.Run("returns error when cluster token is not a string", func(t *testing.T) {
		mockClient := &mockOpenBAOClient{
			secrets: map[string]map[string]interface{}{
				SecretMount + "/data/" + ClusterTokenPath: {
					"token": 12345, // Not a string
				},
				SecretMount + "/data/" + AgentTokenPath: {
					"token": "test-agent-token",
				},
			},
		}

		tokens, err := LoadTokens(ctx, mockClient)
		assert.Error(t, err)
		assert.Nil(t, tokens)
		assert.Contains(t, err.Error(), "cluster token is not a string")
	})

	t.Run("returns error when agent token is not a string", func(t *testing.T) {
		mockClient := &mockOpenBAOClient{
			secrets: map[string]map[string]interface{}{
				SecretMount + "/data/" + ClusterTokenPath: {
					"token": "test-cluster-token",
				},
				SecretMount + "/data/" + AgentTokenPath: {
					"token": 12345, // Not a string
				},
			},
		}

		tokens, err := LoadTokens(ctx, mockClient)
		assert.Error(t, err)
		assert.Nil(t, tokens)
		assert.Contains(t, err.Error(), "agent token is not a string")
	})

	t.Run("returns error when cluster token is empty", func(t *testing.T) {
		mockClient := &mockOpenBAOClient{
			secrets: map[string]map[string]interface{}{
				SecretMount + "/data/" + ClusterTokenPath: {
					"token": "",
				},
				SecretMount + "/data/" + AgentTokenPath: {
					"token": "test-agent-token",
				},
			},
		}

		tokens, err := LoadTokens(ctx, mockClient)
		assert.Error(t, err)
		assert.Nil(t, tokens)
		assert.Contains(t, err.Error(), "cluster token is empty")
	})

	t.Run("returns error when agent token is empty", func(t *testing.T) {
		mockClient := &mockOpenBAOClient{
			secrets: map[string]map[string]interface{}{
				SecretMount + "/data/" + ClusterTokenPath: {
					"token": "test-cluster-token",
				},
				SecretMount + "/data/" + AgentTokenPath: {
					"token": "",
				},
			},
		}

		tokens, err := LoadTokens(ctx, mockClient)
		assert.Error(t, err)
		assert.Nil(t, tokens)
		assert.Contains(t, err.Error(), "agent token is empty")
	})
}

func TestGenerateAndStoreTokens(t *testing.T) {
	ctx := context.Background()

	t.Run("generates and stores tokens successfully", func(t *testing.T) {
		mockClient := &mockOpenBAOClient{
			secrets: make(map[string]map[string]interface{}),
		}

		tokens, err := GenerateAndStoreTokens(ctx, mockClient)
		require.NoError(t, err)
		require.NotNil(t, tokens)

		assert.NotEmpty(t, tokens.ClusterToken)
		assert.NotEmpty(t, tokens.AgentToken)

		// Verify tokens were stored
		clusterKey := SecretMount + "/data/" + ClusterTokenPath
		assert.Contains(t, mockClient.secrets, clusterKey)
		assert.Equal(t, tokens.ClusterToken, mockClient.secrets[clusterKey]["token"])

		agentKey := SecretMount + "/data/" + AgentTokenPath
		assert.Contains(t, mockClient.secrets, agentKey)
		assert.Equal(t, tokens.AgentToken, mockClient.secrets[agentKey]["token"])
	})

	t.Run("returns error when storage fails", func(t *testing.T) {
		failingClient := &mockOpenBAOClient{
			secrets:  make(map[string]map[string]interface{}),
			writeErr: assert.AnError,
		}

		tokens, err := GenerateAndStoreTokens(ctx, failingClient)
		assert.Error(t, err)
		assert.Nil(t, tokens)
		assert.Contains(t, err.Error(), "failed to store tokens")
	})
}

// mockOpenBAOClient is a mock implementation of the OpenBAO client for testing
type mockOpenBAOClient struct {
	secrets   map[string]map[string]interface{}
	readErr   error
	writeErr  error
	failPaths []string // Paths that should fail
}

func (m *mockOpenBAOClient) ReadSecretV2(ctx context.Context, mount, path string) (map[string]interface{}, error) {
	fullPath := mount + "/data/" + path

	// Check if this path should fail
	for _, fp := range m.failPaths {
		if fullPath == fp {
			return nil, m.readErr
		}
	}

	data, exists := m.secrets[fullPath]
	if !exists {
		if m.readErr != nil {
			return nil, m.readErr
		}
		return nil, assert.AnError
	}
	return data, nil
}

func (m *mockOpenBAOClient) WriteSecretV2(ctx context.Context, mount, path string, data map[string]interface{}) error {
	fullPath := mount + "/data/" + path

	// Check if this path should fail
	for _, fp := range m.failPaths {
		if fullPath == fp {
			if m.writeErr != nil {
				return m.writeErr
			}
			return assert.AnError
		}
	}

	// If writeErr is set but no failPaths match, only fail if failPaths is empty
	if m.writeErr != nil && len(m.failPaths) == 0 {
		return m.writeErr
	}

	m.secrets[fullPath] = data
	return nil
}

// Ensure mockOpenBAOClient implements the required interface
var _ interface {
	ReadSecretV2(ctx context.Context, mount, path string) (map[string]interface{}, error)
	WriteSecretV2(ctx context.Context, mount, path string, data map[string]interface{}) error
} = (*mockOpenBAOClient)(nil)

// Integration test helpers - these would use a real OpenBAO container in integration tests
func TestTokensIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	// This would be implemented with testcontainers in a real integration test
	t.Skip("integration test requires OpenBAO container - implement with testcontainers")

	// Example integration test structure:
	// 1. Start OpenBAO container
	// 2. Initialize and unseal
	// 3. Create client
	// 4. Generate and store tokens
	// 5. Load tokens back
	// 6. Verify tokens match
}
