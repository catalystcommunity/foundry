package gatewaycontroller

import (
	"testing"

	"github.com/catalystcommunity/foundry/v1/internal/component"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()
	assert.Equal(t, "foundry-system", cfg.Namespace)
	assert.Equal(t, "containers.catalystsquad.com/public/catalystcommunity/foundry", cfg.ImageRepository)
	assert.Equal(t, "", cfg.ImageTag)
	assert.Equal(t, "contour", cfg.GatewayName)
	assert.Equal(t, "projectcontour", cfg.GatewayNamespace)
	assert.Equal(t, "contour-envoy", cfg.EnvoyService)
	assert.Equal(t, "contour-envoy", cfg.NetworkPolicy)
	assert.Equal(t, "15s", cfg.Interval)
	assert.Equal(t, uint64(1), cfg.ReplicaCount)
}

func TestParseConfig_Overrides(t *testing.T) {
	cfg := component.ComponentConfig{
		"namespace":         "ingress-system",
		"image_tag":         "0.2.0",
		"gateway_namespace": "projectcontour",
		"network_policy":    "",
		"interval":          "30s",
		"replica_count":     2,
	}

	config, err := ParseConfig(cfg)
	require.NoError(t, err)

	assert.Equal(t, "ingress-system", config.Namespace)
	assert.Equal(t, "0.2.0", config.ImageTag)
	assert.Equal(t, "30s", config.Interval)
	assert.Equal(t, uint64(2), config.ReplicaCount)
	// network_policy explicitly set to empty -> skip netpol reconciliation
	assert.Equal(t, "", config.NetworkPolicy)
	// untouched fields keep defaults
	assert.Equal(t, "contour", config.GatewayName)
}

func TestComponent_NameAndDependencies(t *testing.T) {
	c := NewComponent(nil, nil)
	assert.Equal(t, "gateway-controller", c.Name())
	assert.Equal(t, []string{"contour"}, c.Dependencies())
}
