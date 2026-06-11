package grafana

import (
	"testing"

	"github.com/catalystcommunity/foundry/v1/internal/component"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// nativeAlertingCfg is Grafana's own file-provisioning structure, verbatim — a map
// of provisioning file name to contents. foundry passes this through untouched.
func nativeAlertingCfg() component.ComponentConfig {
	return component.ComponentConfig{
		"alerting": map[string]interface{}{
			"contactpoints.yaml": map[string]interface{}{
				"apiVersion": 1,
				"contactPoints": []interface{}{
					map[string]interface{}{
						"orgId": 1,
						"name":  "ops",
						"receivers": []interface{}{
							map[string]interface{}{
								"uid":            "ops-email",
								"type":           "email",
								"settings":       map[string]interface{}{"addresses": "oncall@example.com"},
								"secureSettings": map[string]interface{}{"some_token": "already-resolved"},
							},
						},
					},
				},
			},
			"policies.yaml": map[string]interface{}{
				"apiVersion": 1,
				"policies":   []interface{}{map[string]interface{}{"orgId": 1, "receiver": "ops"}},
			},
		},
	}
}

func TestParseConfig_AlertingPassthrough(t *testing.T) {
	cfg, err := ParseConfig(nativeAlertingCfg())
	require.NoError(t, err)
	require.NotNil(t, cfg.Alerting)
	// Stored verbatim, both provisioning files present.
	assert.Contains(t, cfg.Alerting, "contactpoints.yaml")
	assert.Contains(t, cfg.Alerting, "policies.yaml")
}

func TestBuildHelmValues_AlertingPassthrough(t *testing.T) {
	cfg, err := ParseConfig(nativeAlertingCfg())
	require.NoError(t, err)

	values := buildHelmValues(cfg)
	alerting, ok := values["alerting"].(map[string]interface{})
	require.True(t, ok, "alerting should be present in helm values")

	// The exact native structure reaches the chart untouched.
	cps := alerting["contactpoints.yaml"].(map[string]interface{})
	assert.Equal(t, 1, cps["apiVersion"])
	list := cps["contactPoints"].([]interface{})
	require.Len(t, list, 1)
	recv := list[0].(map[string]interface{})["receivers"].([]interface{})[0].(map[string]interface{})
	assert.Equal(t, "email", recv["type"])
	assert.Equal(t, "oncall@example.com", recv["settings"].(map[string]interface{})["addresses"])
	assert.Contains(t, alerting, "policies.yaml")
}

func TestBuildHelmValues_NoAlerting(t *testing.T) {
	cfg, err := ParseConfig(component.ComponentConfig{})
	require.NoError(t, err)
	values := buildHelmValues(cfg)
	_, has := values["alerting"]
	assert.False(t, has, "no alerting config -> no alerting helm value")
}
