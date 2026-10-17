package config

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func TestApplyPolicyDefaultsAddsCatalogAndKeepsOverrides(t *testing.T) {
	config := &Config{Policies: []PolicyConfig{{Name: "disallow-host-path", State: "Enforce"}}}
	require.NoError(t, ApplyPolicyDefaults(config))
	require.Len(t, config.Policies, 46)

	states := make(map[string]string, len(config.Policies))
	for _, policy := range config.Policies {
		states[policy.Name] = policy.State
	}
	assert.Equal(t, "Enforce", states["disallow-host-path"])
	assert.Equal(t, "Enforce", states["disallow-default-namespace"])
	assert.Equal(t, "Disabled", states["verify-image"])
}

func TestPolicyManifestSurvivesDefaultsAndYAMLRoundTrip(t *testing.T) {
	manifest := "apiVersion: policies.kyverno.io/v1\nkind: ValidatingPolicy\nmetadata:\n  name: require-labels\nspec:\n  validations:\n    - expression: 'true'\n"
	cfg := &Config{Policies: []PolicyConfig{{Name: "require-labels", State: "audit", Manifest: &manifest}}}
	require.NoError(t, ApplyPolicyDefaults(cfg))
	data, err := yaml.Marshal(cfg)
	require.NoError(t, err)
	var decoded Config
	require.NoError(t, yaml.NewDecoder(bytes.NewReader(data)).Decode(&decoded))
	require.NoError(t, ApplyPolicyDefaults(&decoded))
	for _, policy := range decoded.Policies {
		if policy.Name == "require-labels" {
			require.NotNil(t, policy.Manifest)
			assert.Equal(t, manifest, *policy.Manifest)
			assert.Equal(t, "Audit", policy.State)
			return
		}
	}
	t.Fatal("custom policy is missing")
}

func TestPolicyValidationRejectsUnknownNamesAndStates(t *testing.T) {
	err := validatePolicies([]PolicyConfig{{Name: "unknown", State: "Audit"}})
	assert.ErrorContains(t, err, "not in the Foundry catalog")

	err = validatePolicies([]PolicyConfig{{Name: "disallow-host-path", State: "Maybe"}})
	assert.ErrorContains(t, err, "must be Enforce, Audit, or Disabled")
}
