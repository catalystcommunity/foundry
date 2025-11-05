package secrets

import (
	"context"
	"fmt"

	"github.com/catalystcommunity/foundry/v1/internal/component/openbao"
)

// OpenBAOResolver resolves secrets from OpenBAO using KV v2 API
type OpenBAOResolver struct {
	client *openbao.Client
	mount  string // KV v2 mount point (default: "secret")
}

// NewOpenBAOResolver creates a new OpenBAO resolver
func NewOpenBAOResolver(addr, token string) (*OpenBAOResolver, error) {
	if addr == "" {
		return nil, fmt.Errorf("OpenBAO address is required")
	}
	if token == "" {
		return nil, fmt.Errorf("OpenBAO token is required")
	}

	client := openbao.NewClient(addr, token)

	return &OpenBAOResolver{
		client: client,
		mount:  "secret", // Default KV v2 mount
	}, nil
}

// NewOpenBAOResolverWithMount creates a new OpenBAO resolver with a custom mount point
func NewOpenBAOResolverWithMount(addr, token, mount string) (*OpenBAOResolver, error) {
	if addr == "" {
		return nil, fmt.Errorf("OpenBAO address is required")
	}
	if token == "" {
		return nil, fmt.Errorf("OpenBAO token is required")
	}
	if mount == "" {
		return nil, fmt.Errorf("mount point is required")
	}

	client := openbao.NewClient(addr, token)

	return &OpenBAOResolver{
		client: client,
		mount:  mount,
	}, nil
}

// Resolve attempts to resolve a secret from OpenBAO
// Uses instance scoping: reads from <instance>/<path> in OpenBAO
func (o *OpenBAOResolver) Resolve(ctx *ResolutionContext, ref SecretRef) (string, error) {
	if ctx == nil {
		return "", fmt.Errorf("resolution context is required")
	}

	// Build the full path with instance scoping
	namespacedPath := ctx.NamespacedPath(ref)

	// Read secret from KV v2
	data, err := o.client.ReadSecretV2(context.Background(), o.mount, namespacedPath)
	if err != nil {
		return "", fmt.Errorf("failed to read secret from OpenBAO at %s: %w", namespacedPath, err)
	}

	// Extract the specific key
	value, ok := data[ref.Key]
	if !ok {
		return "", fmt.Errorf("key %s not found in secret at path %s", ref.Key, namespacedPath)
	}

	// Convert to string
	strValue, ok := value.(string)
	if !ok {
		return "", fmt.Errorf("value for key %s at path %s is not a string", ref.Key, namespacedPath)
	}

	return strValue, nil
}

// ResolveSecret resolves a secret directly from OpenBAO without instance scoping
// This is used by components that need direct access to secrets (e.g., K8s client)
func (o *OpenBAOResolver) ResolveSecret(ctx context.Context, path, key string) (string, error) {
	// Read secret from KV v2
	data, err := o.client.ReadSecretV2(ctx, o.mount, path)
	if err != nil {
		return "", fmt.Errorf("failed to read secret from OpenBAO at %s: %w", path, err)
	}

	// Extract the specific key
	value, ok := data[key]
	if !ok {
		return "", fmt.Errorf("key %s not found in secret at path %s", key, path)
	}

	// Convert to string
	strValue, ok := value.(string)
	if !ok {
		return "", fmt.Errorf("value for key %s at path %s is not a string", key, path)
	}

	return strValue, nil
}
