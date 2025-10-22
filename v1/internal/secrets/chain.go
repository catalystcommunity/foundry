package secrets

import (
	"fmt"
	"strings"
)

// ChainResolver tries multiple resolvers in order until one succeeds
type ChainResolver struct {
	resolvers []Resolver
}

// NewChainResolver creates a new chain resolver with the given resolvers
// Resolvers are tried in the order they are provided
func NewChainResolver(resolvers ...Resolver) *ChainResolver {
	return &ChainResolver{
		resolvers: resolvers,
	}
}

// Resolve tries each resolver in order until one succeeds
// If all fail, returns an aggregate error with details from all attempts
func (c *ChainResolver) Resolve(ctx *ResolutionContext, ref SecretRef) (string, error) {
	if len(c.resolvers) == 0 {
		return "", fmt.Errorf("no resolvers configured")
	}

	var errors []string

	for i, resolver := range c.resolvers {
		value, err := resolver.Resolve(ctx, ref)
		if err == nil {
			// Success!
			return value, nil
		}

		// Record error and continue to next resolver
		errors = append(errors, fmt.Sprintf("resolver %d: %s", i+1, err.Error()))
	}

	// All resolvers failed
	fullKey := ctx.FullKey(ref)
	return "", fmt.Errorf("failed to resolve secret %s after trying %d resolver(s):\n  %s",
		fullKey, len(c.resolvers), strings.Join(errors, "\n  "))
}
