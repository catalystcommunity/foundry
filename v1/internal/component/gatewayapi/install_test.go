package gatewayapi

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/catalystcommunity/foundry/v1/internal/component"
)

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()

	assert.Equal(t, "v1.3.0", cfg.Version)
}

func TestParseConfig(t *testing.T) {
	tests := []struct {
		name     string
		cfg      component.ComponentConfig
		expected *Config
	}{
		{
			name: "default values",
			cfg:  component.ComponentConfig{},
			expected: &Config{
				Version: "v1.3.0",
			},
		},
		{
			name: "custom version",
			cfg: component.ComponentConfig{
				"version": "v1.2.0",
			},
			expected: &Config{
				Version: "v1.2.0",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config, err := ParseConfig(tt.cfg)
			require.NoError(t, err)
			assert.Equal(t, tt.expected.Version, config.Version)
		})
	}
}

func TestComponent_Name(t *testing.T) {
	comp := NewComponent(nil)
	assert.Equal(t, "gateway-api", comp.Name())
}

func TestComponent_Dependencies(t *testing.T) {
	comp := NewComponent(nil)
	deps := comp.Dependencies()

	assert.Contains(t, deps, "k3s")
}

func TestComponent_Status_NilClient(t *testing.T) {
	comp := NewComponent(nil)

	status, err := comp.Status(nil)
	require.NoError(t, err)
	assert.False(t, status.Installed)
	assert.False(t, status.Healthy)
	assert.Contains(t, status.Message, "not initialized")
}

func TestPluralizeKind(t *testing.T) {
	tests := []struct {
		kind     string
		expected string
	}{
		{"CustomResourceDefinition", "customresourcedefinitions"},
		{"GatewayClass", "gatewayclasses"},
		{"Gateway", "gateways"},
		{"HTTPRoute", "httproutes"},
		{"ReferenceGrant", "referencegrants"},
		{"Ingress", "ingresses"},
		{"Endpoints", "endpoints"},
		{"Pod", "pods"},
		{"Service", "services"},
	}

	for _, tt := range tests {
		t.Run(tt.kind, func(t *testing.T) {
			result := pluralizeKind(tt.kind)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestGatewayAPIReleaseURL(t *testing.T) {
	cfg := DefaultConfig()
	expectedURL := "https://github.com/kubernetes-sigs/gateway-api/releases/download/v1.3.0/experimental-install.yaml"
	actualURL := GatewayAPIReleaseURL + "/" + cfg.Version + "/" + ExperimentalInstallFile

	assert.Equal(t, expectedURL, actualURL)
}
