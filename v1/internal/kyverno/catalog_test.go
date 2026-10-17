package kyverno

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func TestCatalogContainsCurrentCELPolicies(t *testing.T) {
	catalog, err := LoadCatalog()
	require.NoError(t, err)
	require.Len(t, catalog.Policies, 46)

	policy, ok := catalog.Policy("disallow-default-namespace")
	require.True(t, ok)
	assert.Equal(t, StateEnforce, policy.DefaultState)
	assert.Equal(t, "policies.kyverno.io/v1", manifestAPIVersion(t, policy))
	assert.Contains(t, policy.SourceURL, catalog.Source.Commit)

	imagePolicy, ok := catalog.Policy("verify-image")
	require.True(t, ok)
	assert.Equal(t, "ImageValidatingPolicy", imagePolicy.Kind)
}

func TestResolveSelectionsUsesDefaultsAndCanonicalizesOverrides(t *testing.T) {
	catalog, err := LoadCatalog()
	require.NoError(t, err)

	resolved, err := catalog.ResolveSelections([]Selection{
		{Name: "disallow-default-namespace", State: "audit"},
		{Name: "require-labels", State: "enabled"},
	})
	require.NoError(t, err)
	require.Len(t, resolved, len(catalog.Policies))

	states := make(map[string]State, len(resolved))
	for _, policy := range resolved {
		states[policy.Name] = policy.State
	}
	assert.Equal(t, StateAudit, states["disallow-default-namespace"])
	assert.Equal(t, StateEnforce, states["require-labels"])
	assert.Equal(t, StateAudit, states["disallow-host-path"])
}

func TestResolveSelectionsRejectsUnknownAndDuplicatePolicies(t *testing.T) {
	catalog, err := LoadCatalog()
	require.NoError(t, err)

	_, err = catalog.ResolveSelections([]Selection{{Name: "unknown", State: StateAudit}})
	assert.ErrorContains(t, err, "not in the Foundry catalog")

	_, err = catalog.ResolveSelections([]Selection{
		{Name: "disallow-host-path", State: StateAudit},
		{Name: "disallow-host-path", State: StateEnforce},
	})
	assert.ErrorContains(t, err, "appears more than once")
}

func TestRenderPolicySetsStateAndOwnership(t *testing.T) {
	catalog, err := LoadCatalog()
	require.NoError(t, err)
	policy, ok := catalog.Policy("disallow-default-namespace")
	require.True(t, ok)

	object, err := renderPolicy(policy, StateEnforce)
	require.NoError(t, err)
	assert.Equal(t, ManagedByValue, object.GetLabels()[ManagedByLabel])
	actions, found, err := unstructured.NestedStringSlice(object.Object, "spec", "validationActions")
	require.NoError(t, err)
	require.True(t, found)
	assert.Equal(t, []string{"Deny"}, actions)

	object, err = renderPolicy(policy, StateAudit)
	require.NoError(t, err)
	actions, _, err = unstructured.NestedStringSlice(object.Object, "spec", "validationActions")
	require.NoError(t, err)
	assert.Equal(t, []string{"Audit"}, actions)
}

func TestSensitiveResourcePoliciesDoNotRequireBackgroundReadAccess(t *testing.T) {
	catalog, err := LoadCatalog()
	require.NoError(t, err)

	names := []string{
		"deny-secret-service-account-token-type",
		"restrict-binding-clusteradmin",
		"restrict-binding-system-groups",
		"restrict-clusterrole-csr",
		"restrict-escalation-verbs-roles",
		"restrict-image-registries",
		"restrict-secret-role-verbs",
		"restrict-wildcard-resources",
		"restrict-wildcard-verbs",
		"restrict-seccomp-strict",
		"disallow-capabilities-strict",
		"disallow-latest-tag",
		"require-image-checksum",
	}
	for _, name := range names {
		t.Run(name, func(t *testing.T) {
			policy, ok := catalog.Policy(name)
			require.True(t, ok)
			object, err := renderPolicy(policy, StateAudit)
			require.NoError(t, err)
			enabled, found, err := unstructured.NestedBool(object.Object, "spec", "evaluation", "background", "enabled")
			require.NoError(t, err)
			require.True(t, found)
			assert.False(t, enabled)
		})
	}
}

func manifestAPIVersion(t *testing.T, policy Policy) string {
	t.Helper()
	object, err := renderPolicy(policy, StateAudit)
	require.NoError(t, err)
	return object.GetAPIVersion()
}
