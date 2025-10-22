package secrets

import (
	"bufio"
	"fmt"
	"os"
	"strings"
)

// FoundryVarsResolver resolves secrets from ~/.foundryvars file
type FoundryVarsResolver struct {
	vars map[string]string // fullKey -> value
}

// NewFoundryVarsResolver creates a resolver from a .foundryvars file
// File format: instance/path/to/secret:key=value
func NewFoundryVarsResolver(path string) (*FoundryVarsResolver, error) {
	file, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			// File doesn't exist is not an error - just return empty resolver
			return &FoundryVarsResolver{vars: make(map[string]string)}, nil
		}
		return nil, fmt.Errorf("failed to open foundryvars file %s: %w", path, err)
	}
	defer file.Close()

	vars := make(map[string]string)
	scanner := bufio.NewScanner(file)
	lineNum := 0

	for scanner.Scan() {
		lineNum++
		line := strings.TrimSpace(scanner.Text())

		// Skip empty lines and comments
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		// Parse key=value format
		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			return nil, fmt.Errorf("invalid format at line %d: expected key=value, got: %s", lineNum, line)
		}

		key := strings.TrimSpace(parts[0])
		value := parts[1] // Don't trim value - whitespace might be significant

		if key == "" {
			return nil, fmt.Errorf("empty key at line %d", lineNum)
		}

		vars[key] = value
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("error reading foundryvars file: %w", err)
	}

	return &FoundryVarsResolver{vars: vars}, nil
}

// NewFoundryVarsResolverDefault creates a resolver from the default ~/.foundryvars location
func NewFoundryVarsResolverDefault() (*FoundryVarsResolver, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("failed to get user home directory: %w", err)
	}

	path := fmt.Sprintf("%s/.foundryvars", homeDir)
	return NewFoundryVarsResolver(path)
}

// Resolve resolves a secret from the loaded variables
// Uses the full key format: instance/path:key
func (f *FoundryVarsResolver) Resolve(ctx *ResolutionContext, ref SecretRef) (string, error) {
	if ctx == nil {
		return "", fmt.Errorf("resolution context is required")
	}

	fullKey := ctx.FullKey(ref)

	value, exists := f.vars[fullKey]
	if !exists {
		return "", fmt.Errorf("secret not found in foundryvars: %s", fullKey)
	}

	return value, nil
}
