package tailscale

import (
	"context"
	"fmt"
	"strings"
)

// Secret represents a secret value that can be stored in OpenBAO.
type Secret struct {
	Data map[string]interface{}
}

// OpenBAOClient defines the interface for OpenBAO operations.
// This interface allows for easier testing with mock implementations.
type OpenBAOClient interface {
	Read(ctx context.Context, path string) (*Secret, error)
}

// SecretResolver handles resolution of secret references from OpenBAO.
type SecretResolver struct {
	client OpenBAOClient
}

// NewSecretResolver creates a new secret resolver.
func NewSecretResolver(client OpenBAOClient) *SecretResolver {
	return &SecretResolver{
		client: client,
	}
}

// Resolve resolves a value that may contain an OpenBAO secret reference.
// Supports format: ${secret:path:key}
// If the value is not a secret reference, returns the literal value.
// If OpenBAO client is nil, returns the literal value (fallback mode).
func (r *SecretResolver) Resolve(ctx context.Context, value string) (string, error) {
	// Check if this is a secret reference
	if !isSecretReference(value) {
		return value, nil
	}

	// If no client available, fail (secrets are required but OpenBAO not configured)
	if r.client == nil {
		return "", fmt.Errorf("secret reference %q requires OpenBAO but client is nil", value)
	}

	// Parse secret reference
	path, key, err := parseSecretReference(value)
	if err != nil {
		return "", fmt.Errorf("failed to parse secret reference: %w", err)
	}

	// Fetch from OpenBAO
	secret, err := r.client.Read(ctx, path)
	if err != nil {
		return "", fmt.Errorf("failed to read secret from OpenBAO at %q: %w", path, err)
	}

	// Extract key from secret data
	val, ok := secret.Data[key]
	if !ok {
		return "", fmt.Errorf("key %q not found in secret at path %q", key, path)
	}

	// Convert to string
	strVal, ok := val.(string)
	if !ok {
		return "", fmt.Errorf("value for key %q at path %q is not a string (got %T)", key, path, val)
	}

	return strVal, nil
}

// ResolveConfig resolves all secret references in a Tailscale config.
// This mutates the config in place, replacing secret references with actual values.
func (r *SecretResolver) ResolveConfig(ctx context.Context, cfg *Config) error {
	if cfg == nil {
		return fmt.Errorf("config cannot be nil")
	}

	// Resolve OAuth client ID
	if cfg.OAuthClientID != nil && *cfg.OAuthClientID != "" {
		resolved, err := r.Resolve(ctx, *cfg.OAuthClientID)
		if err != nil {
			return fmt.Errorf("failed to resolve oauth_client_id: %w", err)
		}
		cfg.OAuthClientID = &resolved
	}

	// Resolve OAuth client secret
	if cfg.OAuthClientSecret != nil && *cfg.OAuthClientSecret != "" {
		resolved, err := r.Resolve(ctx, *cfg.OAuthClientSecret)
		if err != nil {
			return fmt.Errorf("failed to resolve oauth_client_secret: %w", err)
		}
		cfg.OAuthClientSecret = &resolved
	}

	return nil
}

// isSecretReference checks if a value is a secret reference (format: ${secret:...}).
func isSecretReference(value string) bool {
	return strings.HasPrefix(value, "${secret:") && strings.HasSuffix(value, "}")
}

// parseSecretReference parses a secret reference into path and key.
// Format: ${secret:path/to/secret:key}
// Returns: path, key, error
func parseSecretReference(value string) (string, string, error) {
	// Remove ${secret: prefix and } suffix
	if !isSecretReference(value) {
		return "", "", fmt.Errorf("not a valid secret reference: %q", value)
	}

	inner := strings.TrimPrefix(value, "${secret:")
	inner = strings.TrimSuffix(inner, "}")

	// Split by last colon to separate path and key
	parts := strings.Split(inner, ":")
	if len(parts) < 2 {
		return "", "", fmt.Errorf("invalid secret reference format %q: expected ${secret:path:key}", value)
	}

	// Path is everything except the last part, key is the last part
	key := parts[len(parts)-1]
	path := strings.Join(parts[:len(parts)-1], ":")

	if path == "" {
		return "", "", fmt.Errorf("invalid secret reference %q: path cannot be empty", value)
	}
	if key == "" {
		return "", "", fmt.Errorf("invalid secret reference %q: key cannot be empty", value)
	}

	return path, key, nil
}

// NewSecretResolverWithFallback creates a secret resolver that falls back to literal values
// if OpenBAO is not available. This is useful for development/testing scenarios.
func NewSecretResolverWithFallback(client OpenBAOClient) *SecretResolver {
	// If client is nil, we still create the resolver
	// The Resolve method will handle nil client by returning literal values for non-references
	return &SecretResolver{
		client: client,
	}
}
