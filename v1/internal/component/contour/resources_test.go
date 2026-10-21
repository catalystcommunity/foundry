package contour

import (
	"testing"

	"github.com/catalystcommunity/foundry/v1/internal/component"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestResourcesDefaultsAndOverrides(t *testing.T) {
	for _, name := range []string{"contour", "envoy"} {
		t.Run(name, func(t *testing.T) {
			cfg, err := ParseConfig(component.ComponentConfig{})
			require.NoError(t, err)
			resources := buildHelmValues(cfg)[name].(map[string]interface{})["resources"].(map[string]interface{})
			assert.Equal(t, "512Mi", resources["requests"].(map[string]interface{})["memory"])
			assert.Equal(t, "512Mi", resources["limits"].(map[string]interface{})["memory"])
			assert.Equal(t, "500m", resources["limits"].(map[string]interface{})["cpu"])
			requests := map[string]interface{}{"memory": "1Gi"}
			cfg, err = ParseConfig(component.ComponentConfig{"values": map[string]interface{}{
				name: map[string]interface{}{"resources": map[string]interface{}{
					"requests": requests,
					"limits":   map[string]interface{}{"memory": "1Gi", "cpu": 2},
				}},
			}})
			require.NoError(t, err)
			resources = buildHelmValues(cfg)[name].(map[string]interface{})["resources"].(map[string]interface{})
			assert.Equal(t, "1Gi", resources["requests"].(map[string]interface{})["memory"])
			assert.Equal(t, "1Gi", resources["limits"].(map[string]interface{})["memory"])
			assert.Equal(t, 2, resources["limits"].(map[string]interface{})["cpu"])
			assert.Equal(t, "100m", resources["requests"].(map[string]interface{})["cpu"])
			assert.Equal(t, map[string]interface{}{"memory": "1Gi"}, requests, "must not mutate the saved configuration")
		})
	}
}

func TestInvalidResources(t *testing.T) {
	for _, name := range []string{"contour", "envoy"} {
		for label, section := range map[string]interface{}{
			"section":               "bad",
			"resources":             map[string]interface{}{"resources": "bad"},
			"requests":              map[string]interface{}{"resources": map[string]interface{}{"requests": "bad"}},
			"unknown field":         map[string]interface{}{"resources": map[string]interface{}{"request": map[string]interface{}{}}},
			"quantity":              map[string]interface{}{"resources": map[string]interface{}{"limits": map[string]interface{}{"memory": "512MiB"}}},
			"negative":              map[string]interface{}{"resources": map[string]interface{}{"limits": map[string]interface{}{"cpu": "-1"}}},
			"request exceeds limit": map[string]interface{}{"resources": map[string]interface{}{"requests": map[string]interface{}{"memory": "1Gi"}}},
		} {
			t.Run(name+"/"+label, func(t *testing.T) {
				_, err := ParseConfig(component.ComponentConfig{"values": map[string]interface{}{name: section}})
				require.Error(t, err)
				assert.Contains(t, err.Error(), "values."+name)
			})
		}
	}
	_, err := ParseConfig(component.ComponentConfig{"values": "bad"})
	require.ErrorContains(t, err, "values must be a map")
}
