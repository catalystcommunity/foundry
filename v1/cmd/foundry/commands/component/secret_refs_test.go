package component

import (
	"os"
	"testing"

	"github.com/catalystcommunity/foundry/v1/internal/secrets"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestResolveSecretRefs(t *testing.T) {
	os.Setenv("FOUNDRY_SECRET_GRAFANA_ALERTING_OPSGENIE_API_KEY", "topsecret")
	defer os.Unsetenv("FOUNDRY_SECRET_GRAFANA_ALERTING_OPSGENIE_API_KEY")

	resolver := secrets.NewChainResolver(secrets.NewEnvResolver())
	resCtx := &secrets.ResolutionContext{}

	// Grafana's native provisioning structure (passed through verbatim), with a
	// ${secret:...} ref nested in a receiver's secureSettings.
	alerting := map[string]interface{}{
		"contactpoints.yaml": map[string]interface{}{
			"apiVersion": 1,
			"contactPoints": []interface{}{
				map[string]interface{}{
					"name": "ops",
					"receivers": []interface{}{
						map[string]interface{}{
							"uid":            "ops-ntfy",
							"type":           "webhook",
							"settings":       map[string]interface{}{"url": "https://ntfy.example.com/alerts"},
							"secureSettings": map[string]interface{}{"authorization_credentials": "${secret:grafana/alerting/opsgenie:api_key}"},
						},
					},
				},
			},
		},
	}

	require.NoError(t, resolveSecretRefs(alerting, resolver, resCtx))

	recv := alerting["contactpoints.yaml"].(map[string]interface{})["contactPoints"].([]interface{})[0].(map[string]interface{})["receivers"].([]interface{})[0].(map[string]interface{})
	assert.Equal(t, "topsecret", recv["secureSettings"].(map[string]interface{})["authorization_credentials"], "secret ref resolved")
	assert.Equal(t, "https://ntfy.example.com/alerts", recv["settings"].(map[string]interface{})["url"], "non-secret string untouched")
}

func TestResolveSecretRefs_MissingErrors(t *testing.T) {
	resolver := secrets.NewChainResolver(secrets.NewEnvResolver())
	resCtx := &secrets.ResolutionContext{}
	m := map[string]interface{}{"k": "${secret:missing/path:key}"}
	require.Error(t, resolveSecretRefs(m, resolver, resCtx), "unresolvable secret ref must error")
}
