//go:build integration
// +build integration

package integration

import (
	"strings"
)

// containsSubstringP3 checks if s contains substr (for Phase 3 tests)
// Named differently to avoid conflict with any existing containsSubstring
func containsSubstringP3(s, substr string) bool {
	return strings.Contains(s, substr)
}
