package stack

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"github.com/catalystcommunity/foundry/v1/internal/component/velero"
	"github.com/catalystcommunity/foundry/v1/internal/config"
	"github.com/catalystcommunity/foundry/v1/internal/helm"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

// Capture the third-party Helm boundary while using the real stack builder,
// component parser, and component installer.
type veleroHelmRecorder struct {
	velero.HelmClient
	existing bool
	values   map[string]interface{}
	err      error
}

func (h *veleroHelmRecorder) AddRepo(context.Context, helm.RepoAddOptions) error {
	return nil
}

func (h *veleroHelmRecorder) List(context.Context, string) ([]helm.Release, error) {
	if h.existing {
		return []helm.Release{{Name: "velero", Status: "deployed"}}, nil
	}
	return nil, nil
}

func (h *veleroHelmRecorder) Install(_ context.Context, opts helm.InstallOptions) error {
	h.values = opts.Values
	return h.err
}

func (h *veleroHelmRecorder) Upgrade(_ context.Context, opts helm.UpgradeOptions) error {
	h.values = opts.Values
	return h.err
}

func TestStackVeleroNodeAgent(t *testing.T) {
	for _, tc := range []struct {
		name    string
		field   string
		raw     bool
		enabled bool
	}{
		{"enabled", "deploy_node_agent: true", true, true},
		{"enabled overrides Helm value", "deploy_node_agent: true", false, true},
		{"disabled", "deploy_node_agent: false", false, false},
		{"disabled overrides Helm value", "deploy_node_agent: false", true, false},
		{"omitted defaults to disabled", "", false, false},
		{"omitted retains typed default", "", true, false},
	} {
		for _, existing := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/existing=%t", tc.name, existing), func(t *testing.T) {
				cfg, err := config.LoadFromReader(strings.NewReader(fmt.Sprintf(`
cluster:
  name: test
  primary_domain: example.test
components:
  velero:
    %s
    values:
      deployNodeAgent: %t
      nodeAgent:
        tolerations:
          - key: foundry.catalystcommunity.org/arch
            operator: Equal
            value: arm64
            effect: NoSchedule
        resources:
          requests: {cpu: 100m}
          limits: {memory: 1Gi}
`, tc.field, tc.raw)))
				require.NoError(t, err)
				path := filepath.Join(t.TempDir(), "stack.yaml")
				for round := 0; round < 2; round++ {
					before, err := yaml.Marshal(cfg)
					require.NoError(t, err)
					input := buildVeleroConfig(cfg)
					client := &veleroHelmRecorder{existing: existing}
					require.NoError(t, velero.NewComponent(client, nil).Install(context.Background(), input))
					assert.Equal(t, tc.enabled, client.values["deployNodeAgent"])
					nodeAgent := client.values["nodeAgent"].(map[string]interface{})
					assert.Equal(t, []interface{}{map[string]interface{}{
						"key": "foundry.catalystcommunity.org/arch", "operator": "Equal",
						"value": "arm64", "effect": "NoSchedule",
					}}, nodeAgent["tolerations"])
					requests := map[string]interface{}{"cpu": "100m"}
					if tc.enabled {
						requests["memory"] = "256Mi"
					}
					assert.Equal(t, map[string]interface{}{
						"requests": requests, "limits": map[string]interface{}{"memory": "1Gi"},
					}, nodeAgent["resources"])
					after, err := yaml.Marshal(cfg)
					require.NoError(t, err)
					assert.Equal(t, string(before), string(after), "installation must not mutate input settings")

					// The stack saves component settings after each successful install.
					saveComponentConfig(cfg, "velero", input)
					require.NoError(t, config.Save(cfg, path))
					cfg, err = config.Load(path)
					require.NoError(t, err)
				}
			})
		}
	}
}

func TestStackVeleroNodeAgentHelmFailure(t *testing.T) {
	for _, existing := range []bool{false, true} {
		t.Run(fmt.Sprintf("existing=%t", existing), func(t *testing.T) {
			cfg, err := config.LoadFromReader(strings.NewReader(`
cluster:
  name: test
  primary_domain: example.test
components:
  velero:
    deploy_node_agent: true
`))
			require.NoError(t, err)
			failure := errors.New("Helm failed")
			client := &veleroHelmRecorder{existing: existing, err: failure}
			err = velero.NewComponent(client, nil).Install(context.Background(), buildVeleroConfig(cfg))
			require.ErrorIs(t, err, failure)
			assert.Equal(t, true, client.values["deployNodeAgent"])
		})
	}
}
