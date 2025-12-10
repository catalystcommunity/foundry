package k8s

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestIsSystemLabel(t *testing.T) {
	tests := []struct {
		name     string
		key      string
		isSystem bool
	}{
		// System labels
		{"kubernetes.io prefix", "kubernetes.io/hostname", true},
		{"kubernetes.io arch", "kubernetes.io/arch", true},
		{"k8s.io prefix", "k8s.io/something", true},
		{"node-role prefix", "node-role.kubernetes.io/control-plane", true},
		{"node-role worker", "node-role.kubernetes.io/worker", true},
		{"node.kubernetes.io prefix", "node.kubernetes.io/instance-type", true},

		// User labels
		{"simple label", "environment", false},
		{"custom prefix", "mycompany.com/tier", false},
		{"dotted label", "app.kubernetes.io/name", false}, // Not in protected list
		{"another custom", "team", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := IsSystemLabel(tt.key)
			assert.Equal(t, tt.isSystem, result)
		})
	}
}

func TestFilterUserLabels(t *testing.T) {
	t.Run("filters out system labels", func(t *testing.T) {
		labels := map[string]string{
			"kubernetes.io/hostname":                  "node1",
			"node-role.kubernetes.io/control-plane":   "",
			"environment":                             "production",
			"zone":                                    "us-east-1a",
		}

		result := FilterUserLabels(labels)

		assert.Len(t, result, 2)
		assert.Equal(t, "production", result["environment"])
		assert.Equal(t, "us-east-1a", result["zone"])
		assert.NotContains(t, result, "kubernetes.io/hostname")
		assert.NotContains(t, result, "node-role.kubernetes.io/control-plane")
	})

	t.Run("empty input", func(t *testing.T) {
		result := FilterUserLabels(nil)
		assert.Empty(t, result)
	})

	t.Run("all user labels", func(t *testing.T) {
		labels := map[string]string{
			"environment": "production",
			"zone":        "us-east-1a",
		}

		result := FilterUserLabels(labels)
		assert.Equal(t, labels, result)
	})

	t.Run("all system labels", func(t *testing.T) {
		labels := map[string]string{
			"kubernetes.io/hostname": "node1",
			"kubernetes.io/arch":     "amd64",
		}

		result := FilterUserLabels(labels)
		assert.Empty(t, result)
	})
}

func TestFilterSystemLabels(t *testing.T) {
	t.Run("keeps only system labels", func(t *testing.T) {
		labels := map[string]string{
			"kubernetes.io/hostname":                  "node1",
			"node-role.kubernetes.io/control-plane":   "",
			"environment":                             "production",
			"zone":                                    "us-east-1a",
		}

		result := FilterSystemLabels(labels)

		assert.Len(t, result, 2)
		assert.Equal(t, "node1", result["kubernetes.io/hostname"])
		assert.Contains(t, result, "node-role.kubernetes.io/control-plane")
		assert.NotContains(t, result, "environment")
		assert.NotContains(t, result, "zone")
	})

	t.Run("empty input", func(t *testing.T) {
		result := FilterSystemLabels(nil)
		assert.Empty(t, result)
	})
}
