package secrets

import (
	"fmt"
	"os"
)

// Resolver is the interface for secret resolution implementations
type Resolver interface {
	Resolve(ctx *ResolutionContext, ref SecretRef) (string, error)
}

// EnvResolver resolves secrets from environment variables
type EnvResolver struct{}

// NewEnvResolver creates a new environment variable resolver
func NewEnvResolver() *EnvResolver {
	return &EnvResolver{}
}

// Resolve resolves a secret from environment variables
// Uses the pattern: FOUNDRY_SECRET_<instance>_<path>_<key>
func (e *EnvResolver) Resolve(ctx *ResolutionContext, ref SecretRef) (string, error) {
	if ctx == nil {
		return "", fmt.Errorf("resolution context is required")
	}

	envVarName := ctx.EnvVarName(ref)

	value := os.Getenv(envVarName)
	if value == "" {
		return "", fmt.Errorf("environment variable %s not set", envVarName)
	}

	return value, nil
}
