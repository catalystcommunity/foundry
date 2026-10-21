package contour

import (
	"fmt"

	"k8s.io/apimachinery/pkg/api/resource"
)

// containerResources merges individual overrides with the production defaults.
// Both the controller and proxy need more memory than the chart's nano preset.
func containerResources(values map[string]interface{}, name string) map[string]interface{} {
	result := map[string]interface{}{
		"requests": map[string]interface{}{"cpu": "100m", "memory": "512Mi", "ephemeral-storage": "50Mi"},
		"limits":   map[string]interface{}{"cpu": "500m", "memory": "512Mi", "ephemeral-storage": "2Gi"},
	}
	section, _ := values[name].(map[string]interface{})
	overrides, _ := section["resources"].(map[string]interface{})
	for _, kind := range []string{"requests", "limits"} {
		entries, _ := overrides[kind].(map[string]interface{})
		for key, value := range entries {
			result[kind].(map[string]interface{})[key] = value
		}
	}
	return result
}

// validateResources rejects malformed values before any cluster changes.
func (c *Config) validateResources() error {
	for _, name := range []string{"contour", "envoy"} {
		if raw, exists := c.Values[name]; exists {
			section, ok := raw.(map[string]interface{})
			if !ok {
				return fmt.Errorf("values.%s must be a map", name)
			}
			if raw, exists := section["resources"]; exists {
				resources, ok := raw.(map[string]interface{})
				if !ok {
					return fmt.Errorf("values.%s.resources must be a map", name)
				}
				for kind, raw := range resources {
					if kind != "requests" && kind != "limits" {
						return fmt.Errorf("values.%s.resources: unknown field %q", name, kind)
					}
					if _, ok := raw.(map[string]interface{}); !ok {
						return fmt.Errorf("values.%s.resources.%s must be a map", name, kind)
					}
				}
			}
		}
		resources := containerResources(c.Values, name)
		parsed := map[string]map[string]resource.Quantity{}
		for _, kind := range []string{"requests", "limits"} {
			parsed[kind] = map[string]resource.Quantity{}
			for key, raw := range resources[kind].(map[string]interface{}) {
				quantity, err := resource.ParseQuantity(fmt.Sprint(raw))
				if err != nil || quantity.Sign() < 0 {
					return fmt.Errorf("values.%s.resources.%s.%s must be a non-negative Kubernetes quantity", name, kind, key)
				}
				parsed[kind][key] = quantity
			}
		}
		for key, request := range parsed["requests"] {
			if limit, exists := parsed["limits"][key]; exists && request.Cmp(limit) > 0 {
				return fmt.Errorf("values.%s.resources: %s request exceeds limit", name, key)
			}
		}
	}
	return nil
}
