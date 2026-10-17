package velero

import (
	"context"
	"testing"

	"github.com/catalystcommunity/foundry/v1/internal/component"
	"github.com/catalystcommunity/foundry/v1/internal/helm"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func TestBuildHelmValues_NodeAgent(t *testing.T) {
	for _, tc := range []struct {
		name     string
		enabled  bool
		custom   string
		expected string
	}{
		{"defaults", true, "{}", "resources: {requests: {memory: 256Mi}}"},
		{"custom settings", true, `nodeAgent:
  tolerations: [{key: foundry.catalystcommunity.org/arch, operator: Equal, value: arm64, effect: NoSchedule}]
  nodeSelector: {backup: enabled}
  podVolumePath: /custom/kubelet/pods
  containerSecurityContext: {privileged: true}
  resources:
    requests: {cpu: 100m}
    limits: {memory: 1Gi}
`, `tolerations: [{key: foundry.catalystcommunity.org/arch, operator: Equal, value: arm64, effect: NoSchedule}]
nodeSelector: {backup: enabled}
podVolumePath: /custom/kubelet/pods
containerSecurityContext: {privileged: true}
resources:
  requests: {cpu: 100m, memory: 256Mi}
  limits: {memory: 1Gi}
`},
		{"custom memory", true, "nodeAgent: {resources: {requests: {memory: 512Mi}}}", "resources: {requests: {memory: 512Mi}}"},
		{"empty requests", true, "nodeAgent: {resources: {requests: {}}}", "resources: {requests: {memory: 256Mi}}"},
		{"null memory", true, "nodeAgent: {resources: {requests: {memory: null}}}", "resources: {requests: {memory: null}}"},
		{"null requests", true, "nodeAgent: {resources: {requests: null}}", "resources: {requests: null}"},
		{"null resources", true, "nodeAgent: {resources: null}", "resources: null"},
		{"null node agent", true, "nodeAgent: null", "null"},
		// Invalid types must reach Helm validation, not be silently replaced.
		{"invalid resources", true, "nodeAgent: {resources: invalid}", "resources: invalid"},
		{"disabled defaults", false, "{}", ""},
		{"disabled custom settings", false, "nodeAgent: {resources: {requests: {cpu: 100m}}, tolerations: []}", "resources: {requests: {cpu: 100m}}\ntolerations: []"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cfg := DefaultConfig()
			cfg.DeployNodeAgent = tc.enabled
			require.NoError(t, yaml.Unmarshal([]byte(tc.custom), &cfg.Values))
			// The typed switch must still override a conflicting raw Helm value.
			cfg.Values["deployNodeAgent"] = !tc.enabled
			before, err := yaml.Marshal(cfg.Values)
			require.NoError(t, err)

			values := buildHelmValues(cfg)
			assert.Equal(t, tc.enabled, values["deployNodeAgent"])
			if tc.expected == "" {
				assert.NotContains(t, values, "nodeAgent")
			} else {
				var expected interface{}
				require.NoError(t, yaml.Unmarshal([]byte(tc.expected), &expected))
				assert.Contains(t, values, "nodeAgent")
				assert.Equal(t, expected, values["nodeAgent"])
			}
			after, err := yaml.Marshal(cfg.Values)
			require.NoError(t, err)
			assert.Equal(t, string(before), string(after), "building Helm values must not change the config")
			assert.Equal(t, values, buildHelmValues(cfg), "repeated builds must give the same values")
		})
	}
}

func TestInstall_UpgradePreservesNodeAgentSettings(t *testing.T) {
	var input map[string]interface{}
	require.NoError(t, yaml.Unmarshal([]byte(`deploy_node_agent: true
values:
  deployNodeAgent: false
  nodeAgent:
    tolerations:
      - key: foundry.catalystcommunity.org/arch
        operator: Equal
        value: arm64
        effect: NoSchedule
    resources:
      requests: {cpu: 100m}
      limits: {memory: 1Gi}
`), &input))
	cfg, err := ParseConfig(component.ComponentConfig(input))
	require.NoError(t, err)
	client := &mockHelmClient{
		listReleases: []helm.Release{{Name: releaseName, Status: "deployed"}},
	}
	require.NoError(t, Install(context.Background(), client, nil, cfg))
	require.Len(t, client.upgradeCalls, 1)
	assert.Empty(t, client.chartsInstalled)
	values := client.upgradeCalls[0].Values
	assert.Equal(t, true, values["deployNodeAgent"])
	nodeAgent, ok := values["nodeAgent"].(map[string]interface{})
	require.True(t, ok)
	configured, ok := cfg.Values["nodeAgent"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, configured["tolerations"], nodeAgent["tolerations"])
	assert.Equal(t, map[string]interface{}{
		"requests": map[string]interface{}{"cpu": "100m", "memory": "256Mi"},
		"limits":   map[string]interface{}{"memory": "1Gi"},
	}, nodeAgent["resources"])
}
