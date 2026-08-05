package secrets

import (
	"fmt"
	"regexp"
	"strings"
)

// SecretRef represents a parsed secret reference
type SecretRef struct {
	Path string // path/to/secret
	Key  string // key_name
	Raw  string // original ${secret:path:key}
}

var (
	// secretRefPattern matches ${secret:path/to/secret:key}
	// Group 1: full path (can contain slashes, alphanumeric, dash, underscore)
	// Group 2: key name (alphanumeric, dash, underscore)
	secretRefPattern = regexp.MustCompile(`^\$\{secret:([a-zA-Z0-9/_-]+):([a-zA-Z0-9_-]+)\}$`)
)

// ParseSecretRef parses ${secret:path:key} syntax.
func ParseSecretRef(s string) (*SecretRef, error) {
	s = strings.TrimSpace(s)

	if !IsSecretRef(s) {
		return nil, nil
	}

	matches := secretRefPattern.FindStringSubmatch(s)
	if matches == nil {
		return nil, fmt.Errorf("invalid secret reference format: %s (expected: ${secret:path:key})", s)
	}

	if len(matches) != 3 {
		return nil, fmt.Errorf("malformed secret reference: %s", s)
	}

	path := matches[1]
	key := matches[2]

	if path == "" {
		return nil, fmt.Errorf("secret path cannot be empty in: %s", s)
	}

	if key == "" {
		return nil, fmt.Errorf("secret key cannot be empty in: %s", s)
	}

	return &SecretRef{
		Path: path,
		Key:  key,
		Raw:  s,
	}, nil
}

// IsSecretRef reports whether a string has secret-reference delimiters.
func IsSecretRef(s string) bool {
	s = strings.TrimSpace(s)
	return strings.HasPrefix(s, "${secret:") && strings.HasSuffix(s, "}")
}

// String returns the original raw reference string
func (sr *SecretRef) String() string {
	if sr.Raw != "" {
		return sr.Raw
	}
	return fmt.Sprintf("${secret:%s:%s}", sr.Path, sr.Key)
}
