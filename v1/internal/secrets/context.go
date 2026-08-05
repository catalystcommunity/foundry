package secrets

import (
	"fmt"
	"path"
	"strings"
)

// ResolutionContext provides instance context for secret resolution
type ResolutionContext struct {
	Instance string // e.g., "myapp-prod", "foundry-core"
}

// NewResolutionContext creates a new resolution context with the given instance
func NewResolutionContext(instance string) *ResolutionContext {
	return &ResolutionContext{
		Instance: instance,
	}
}

// NamespacedPath prefixes a secret path with the instance name.
func (rc *ResolutionContext) NamespacedPath(ref SecretRef) string {
	if rc.Instance == "" {
		return ref.Path
	}
	return path.Join(rc.Instance, ref.Path)
}

// FullKey returns the path and key in "path:key" format.
func (rc *ResolutionContext) FullKey(ref SecretRef) string {
	namespacedPath := rc.NamespacedPath(ref)
	return fmt.Sprintf("%s:%s", namespacedPath, ref.Key)
}

// EnvVarName returns the normalized FOUNDRY_SECRET environment variable name.
func (rc *ResolutionContext) EnvVarName(ref SecretRef) string {
	namespacedPath := rc.NamespacedPath(ref)

	fullPath := fmt.Sprintf("%s_%s", namespacedPath, ref.Key)
	fullPath = strings.ReplaceAll(fullPath, "/", "_")
	fullPath = strings.ReplaceAll(fullPath, "-", "_")
	fullPath = strings.ReplaceAll(fullPath, ":", "_")

	fullPath = strings.ToUpper(fullPath)

	return fmt.Sprintf("FOUNDRY_SECRET_%s", fullPath)
}
