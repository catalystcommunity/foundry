package k3s

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"fmt"
)

const (
	// TokenLength is the number of random bytes to generate for tokens (32 bytes = 256 bits)
	TokenLength = 32

	// OpenBAO paths for storing K3s tokens (relative to mount point)
	ClusterTokenPath = "k3s/cluster-token"
	AgentTokenPath   = "k3s/agent-token"

	// Mount point for KV v2 secrets engine
	SecretMount = "foundry-core"
)

// SecretClient defines the interface for storing and retrieving secrets
type SecretClient interface {
	ReadSecretV2(ctx context.Context, mount, path string) (map[string]interface{}, error)
	WriteSecretV2(ctx context.Context, mount, path string, data map[string]interface{}) error
}

// Tokens holds the cluster and agent tokens for K3s
type Tokens struct {
	ClusterToken string
	AgentToken   string
}

// GenerateToken generates a cryptographically secure random token
// Returns a base64-encoded string suitable for use as a K3s token
func GenerateToken() (string, error) {
	bytes := make([]byte, TokenLength)
	if _, err := rand.Read(bytes); err != nil {
		return "", fmt.Errorf("failed to generate random bytes: %w", err)
	}

	// Use URL-safe base64 encoding without padding
	token := base64.RawURLEncoding.EncodeToString(bytes)
	return token, nil
}

// GenerateTokens generates both cluster and agent tokens
func GenerateTokens() (*Tokens, error) {
	clusterToken, err := GenerateToken()
	if err != nil {
		return nil, fmt.Errorf("failed to generate cluster token: %w", err)
	}

	agentToken, err := GenerateToken()
	if err != nil {
		return nil, fmt.Errorf("failed to generate agent token: %w", err)
	}

	return &Tokens{
		ClusterToken: clusterToken,
		AgentToken:   agentToken,
	}, nil
}

// StoreTokens stores the cluster and agent tokens in OpenBAO
// Tokens are stored at:
// - foundry-core/k3s/cluster-token
// - foundry-core/k3s/agent-token
func StoreTokens(ctx context.Context, client SecretClient, tokens *Tokens) error {
	if tokens == nil {
		return fmt.Errorf("tokens cannot be nil")
	}

	if tokens.ClusterToken == "" {
		return fmt.Errorf("cluster token cannot be empty")
	}

	if tokens.AgentToken == "" {
		return fmt.Errorf("agent token cannot be empty")
	}

	// Store cluster token using generated K3sClusterToken type
	clusterStorage := &K3sClusterToken{
		Token: tokens.ClusterToken,
	}
	clusterData := map[string]interface{}{
		"token": clusterStorage.Token,
	}
	if err := client.WriteSecretV2(ctx, SecretMount, ClusterTokenPath, clusterData); err != nil {
		return fmt.Errorf("failed to store cluster token: %w", err)
	}

	// Store agent token using generated K3sAgentToken type
	agentStorage := &K3sAgentToken{
		Token: tokens.AgentToken,
	}
	agentData := map[string]interface{}{
		"token": agentStorage.Token,
	}
	if err := client.WriteSecretV2(ctx, SecretMount, AgentTokenPath, agentData); err != nil {
		return fmt.Errorf("failed to store agent token: %w", err)
	}

	return nil
}

// LoadTokens retrieves the cluster and agent tokens from OpenBAO
func LoadTokens(ctx context.Context, client SecretClient) (*Tokens, error) {
	// Load cluster token using generated K3sClusterToken type
	clusterData, err := client.ReadSecretV2(ctx, SecretMount, ClusterTokenPath)
	if err != nil {
		return nil, fmt.Errorf("failed to load cluster token: %w", err)
	}

	clusterTokenStr, ok := clusterData["token"].(string)
	if !ok {
		return nil, fmt.Errorf("cluster token is not a string")
	}

	clusterStorage := &K3sClusterToken{
		Token: clusterTokenStr,
	}

	if clusterStorage.Token == "" {
		return nil, fmt.Errorf("cluster token is empty")
	}

	// Load agent token using generated K3sAgentToken type
	agentData, err := client.ReadSecretV2(ctx, SecretMount, AgentTokenPath)
	if err != nil {
		return nil, fmt.Errorf("failed to load agent token: %w", err)
	}

	agentTokenStr, ok := agentData["token"].(string)
	if !ok {
		return nil, fmt.Errorf("agent token is not a string")
	}

	agentStorage := &K3sAgentToken{
		Token: agentTokenStr,
	}

	if agentStorage.Token == "" {
		return nil, fmt.Errorf("agent token is empty")
	}

	return &Tokens{
		ClusterToken: clusterStorage.Token,
		AgentToken:   agentStorage.Token,
	}, nil
}

// GenerateAndStoreTokens is a convenience function that generates tokens and stores them in OpenBAO
// This function is idempotent - if tokens already exist in OpenBAO, it returns those instead of generating new ones
func GenerateAndStoreTokens(ctx context.Context, client SecretClient) (*Tokens, error) {
	// First, try to load existing tokens (idempotency check)
	existingTokens, err := LoadTokens(ctx, client)
	if err == nil && existingTokens != nil {
		// Tokens already exist, return them
		fmt.Println("   K3s tokens already exist in OpenBAO, reusing them")
		return existingTokens, nil
	}

	// Tokens don't exist, generate new ones
	tokens, err := GenerateTokens()
	if err != nil {
		return nil, fmt.Errorf("failed to generate tokens: %w", err)
	}

	if err := StoreTokens(ctx, client, tokens); err != nil {
		return nil, fmt.Errorf("failed to store tokens: %w", err)
	}

	return tokens, nil
}
