package secrets

import (
	"fmt"
	"path"
	"strings"
)

// ResolutionContext provides instance and namespace context for secret resolution
type ResolutionContext struct {
	Instance  string // e.g., "myapp-prod", "foundry-core"
	Namespace string // Optional: for namespace-scoped secrets
}

// NewResolutionContext creates a new resolution context with the given instance
func NewResolutionContext(instance string) *ResolutionContext {
	return &ResolutionContext{
		Instance: instance,
	}
}

// NewResolutionContextWithNamespace creates a new resolution context with instance and namespace
func NewResolutionContextWithNamespace(instance, namespace string) *ResolutionContext {
	return &ResolutionContext{
		Instance:  instance,
		Namespace: namespace,
	}
}

// NamespacedPath returns the full namespaced path for a secret reference
// Combines the instance with the secret path
// Example: instance="myapp-prod", ref.Path="database/main" → "myapp-prod/database/main"
func (rc *ResolutionContext) NamespacedPath(ref SecretRef) string {
	if rc.Instance == "" {
		// If no instance, just return the path as-is
		return ref.Path
	}

	// Join instance and path
	return path.Join(rc.Instance, ref.Path)
}

// FullKey returns the complete key identifier for lookups
// Format: "namespace-path:key"
func (rc *ResolutionContext) FullKey(ref SecretRef) string {
	namespacedPath := rc.NamespacedPath(ref)
	return fmt.Sprintf("%s:%s", namespacedPath, ref.Key)
}

// EnvVarName generates the environment variable name for a secret
// Pattern: FOUNDRY_SECRET_<instance>_<path>_<key>
// All slashes, hyphens, and colons are replaced with underscores
// All letters are uppercased
func (rc *ResolutionContext) EnvVarName(ref SecretRef) string {
	namespacedPath := rc.NamespacedPath(ref)

	// Combine path and key
	fullPath := fmt.Sprintf("%s_%s", namespacedPath, ref.Key)

	// Replace separators with underscores
	fullPath = strings.ReplaceAll(fullPath, "/", "_")
	fullPath = strings.ReplaceAll(fullPath, "-", "_")
	fullPath = strings.ReplaceAll(fullPath, ":", "_")

	// Convert to uppercase
	fullPath = strings.ToUpper(fullPath)

	return fmt.Sprintf("FOUNDRY_SECRET_%s", fullPath)
}
