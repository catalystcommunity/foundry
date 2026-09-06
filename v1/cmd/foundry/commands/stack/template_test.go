package stack

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGenerateTemplateYAMLShowsUpgradeControlForAllComponents(t *testing.T) {
	template := generateTemplateYAML()
	components := []string{
		"openbao", "dns", "zot", "k3s", "gateway-api", "storage",
		"prometheus", "contour", "gateway-controller", "cert-manager",
		"seaweedfs", "external-dns", "loki", "grafana", "velero",
	}

	for _, name := range components {
		assert.Contains(t, template, "  "+name+":\n    allow_upgrades: true")
	}
	assert.Equal(t, len(components), strings.Count(template, "allow_upgrades: true"))
	assert.Contains(t, template, "  gateway-controller:\n    allow_upgrades: true\n    enabled: true")
	assert.NotContains(t, template, "version: latest")
}
