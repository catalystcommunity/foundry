package kyverno

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestManifestOverrides(t *testing.T) {
	catalog, err := LoadCatalog()
	require.NoError(t, err)
	policy, _ := catalog.Policy("restrict-image-registries")
	custom := strings.ReplaceAll(strings.ReplaceAll(policy.YAML, "eu.foo.io/", "registry.example.com/"), "bar.io/", "ghcr.io/")
	for _, tc := range []struct {
		name, manifest string
		state          State
		errorText      string
	}{
		{"missing customization", "", StateAudit, "requires a custom manifest"},
		{"unchanged example", policy.YAML, StateEnforce, "replace the example"},
		{"custom registry", custom, StateEnforce, ""},
		{"disabled example", policy.YAML, StateDisabled, ""},
		{"different name", strings.Replace(custom, "name: restrict-image-registries", "name: unrelated", 1), StateAudit, "keep its catalog"},
		{"different kind", strings.Replace(custom, "kind: ValidatingPolicy", "kind: Namespace", 1), StateAudit, "keep its catalog"},
		{"multiple documents", custom + "\n---\nkind: Namespace\n", StateAudit, "exactly one"},
		{"invalid yaml", "{", StateAudit, "manifest"},
		{"missing spec", "apiVersion: policies.kyverno.io/v1\nkind: ValidatingPolicy\nmetadata:\n  name: restrict-image-registries\n", StateAudit, "non-empty spec"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			resolved, err := catalog.ResolveSelections([]Selection{{Name: policy.Name, State: tc.state, Manifest: tc.manifest}})
			if tc.errorText != "" {
				require.ErrorContains(t, err, tc.errorText)
				return
			}
			require.NoError(t, err)
			for _, selection := range resolved {
				if selection.Name != policy.Name {
					continue
				}
				assert.Equal(t, tc.manifest, selection.Manifest)
				overridden, err := selectedPolicy(policy, selection)
				require.NoError(t, err)
				assert.Equal(t, policy.Name, overridden.Name)
			}
		})
	}
}

func TestImageVerificationRequiresCustomTrust(t *testing.T) {
	catalog, err := LoadCatalog()
	require.NoError(t, err)
	policy, _ := catalog.Policy("verify-image")
	manifest := strings.ReplaceAll(policy.YAML, "ghcr.io/kyverno/test-verify-image", "registry.example.com/team/")
	_, err = catalog.ResolveSelections([]Selection{{Name: policy.Name, State: StateEnforce, Manifest: manifest}})
	require.ErrorContains(t, err, "custom image references and trust settings")
}
