package secrets

import "fmt"

// OpenBAOResolver resolves secrets from OpenBAO
// This is a stub implementation - full implementation will be in Phase 2
type OpenBAOResolver struct {
	addr  string
	token string
}

// NewOpenBAOResolver creates a new OpenBAO resolver
func NewOpenBAOResolver(addr, token string) (*OpenBAOResolver, error) {
	return &OpenBAOResolver{
		addr:  addr,
		token: token,
	}, nil
}

// Resolve attempts to resolve a secret from OpenBAO
// Currently returns not-implemented error as this is a stub
func (o *OpenBAOResolver) Resolve(ctx *ResolutionContext, ref SecretRef) (string, error) {
	return "", fmt.Errorf("OpenBAO integration not yet implemented (will be available in Phase 2)")
}
