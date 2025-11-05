package certmanager

import (
	"context"
	"testing"

	"github.com/catalystcommunity/foundry/v1/internal/component"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewComponent(t *testing.T) {
	tests := []struct {
		name     string
		config   *Config
		expected *Config
	}{
		{
			name:   "nil config uses defaults",
			config: nil,
			expected: &Config{
				Namespace:         "cert-manager",
				Version:           "v1.14.2",
				DefaultIssuerType: "self-signed",
				ACMEServer:        "https://acme-v02.api.letsencrypt.org/directory",
				InstallCRDs:       true,
			},
		},
		{
			name: "empty config uses defaults",
			config: &Config{},
			expected: &Config{
				Namespace:         "cert-manager",
				Version:           "v1.14.2",
				DefaultIssuerType: "self-signed",
				ACMEServer:        "https://acme-v02.api.letsencrypt.org/directory",
				InstallCRDs:       true,
			},
		},
		{
			name: "custom config preserved",
			config: &Config{
				Namespace:           "custom-ns",
				Version:             "v1.13.0",
				CreateDefaultIssuer: true,
				DefaultIssuerType:   "acme",
				ACMEEmail:           "test@example.com",
				ACMEServer:          "https://acme-staging-v02.api.letsencrypt.org/directory",
			},
			expected: &Config{
				Namespace:           "custom-ns",
				Version:             "v1.13.0",
				CreateDefaultIssuer: true,
				DefaultIssuerType:   "acme",
				ACMEEmail:           "test@example.com",
				ACMEServer:          "https://acme-staging-v02.api.letsencrypt.org/directory",
				InstallCRDs:         true,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			comp := NewComponent(tt.config)
			require.NotNil(t, comp)
			assert.Equal(t, tt.expected, comp.config)
		})
	}
}

func TestComponent_Name(t *testing.T) {
	comp := NewComponent(nil)
	assert.Equal(t, "cert-manager", comp.Name())
}

func TestComponent_Dependencies(t *testing.T) {
	comp := NewComponent(nil)
	deps := comp.Dependencies()
	assert.Equal(t, []string{"k3s"}, deps)
}

func TestComponent_Config(t *testing.T) {
	cfg := &Config{
		Namespace: "test-ns",
		Version:   "v1.14.0",
	}
	comp := NewComponent(cfg)

	result := comp.Config()
	require.NotNil(t, result)

	resultCfg, ok := result.(*Config)
	require.True(t, ok)
	assert.Equal(t, "test-ns", resultCfg.Namespace)
	assert.Equal(t, "v1.14.0", resultCfg.Version)
}

func TestParseConfig(t *testing.T) {
	tests := []struct {
		name     string
		input    map[string]interface{}
		expected *Config
	}{
		{
			name:  "empty map",
			input: map[string]interface{}{},
			expected: &Config{},
		},
		{
			name: "full config",
			input: map[string]interface{}{
				"namespace":             "custom-ns",
				"version":               "v1.14.0",
				"create_default_issuer": true,
				"default_issuer_type":   "acme",
				"acme_email":            "test@example.com",
				"acme_server":           "https://acme-staging.example.com/directory",
				"install_crds":          false,
			},
			expected: &Config{
				Namespace:           "custom-ns",
				Version:             "v1.14.0",
				CreateDefaultIssuer: true,
				DefaultIssuerType:   "acme",
				ACMEEmail:           "test@example.com",
				ACMEServer:          "https://acme-staging.example.com/directory",
				InstallCRDs:         false,
			},
		},
		{
			name: "partial config",
			input: map[string]interface{}{
				"namespace": "test-ns",
				"version":   "v1.13.0",
			},
			expected: &Config{
				Namespace: "test-ns",
				Version:   "v1.13.0",
			},
		},
		{
			name: "wrong types ignored",
			input: map[string]interface{}{
				"namespace":             123, // Should be string
				"create_default_issuer": "true", // Should be bool
			},
			expected: &Config{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg, err := ParseConfig(tt.input)
			require.NoError(t, err)
			assert.Equal(t, tt.expected, cfg)
		})
	}
}

func TestComponent_Install(t *testing.T) {
	comp := NewComponent(&Config{
		Namespace: "cert-manager",
		Version:   "v1.14.2",
	})

	// Test that Install is callable (actual implementation tested in install_test.go)
	ctx := context.Background()
	componentCfg := component.ComponentConfig{}

	// This will fail because we don't provide clients, but verifies the method exists
	err := comp.Install(ctx, componentCfg)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "helm_client not provided")
}

func TestComponent_Upgrade(t *testing.T) {
	comp := NewComponent(&Config{
		Namespace: "cert-manager",
		Version:   "v1.14.2",
	})

	ctx := context.Background()
	componentCfg := component.ComponentConfig{}

	err := comp.Upgrade(ctx, componentCfg)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "helm_client not provided")
}

func TestComponent_Status(t *testing.T) {
	comp := NewComponent(&Config{
		Namespace: "cert-manager",
		Version:   "v1.14.2",
	})

	ctx := context.Background()
	status, err := comp.Status(ctx)
	require.NoError(t, err)
	require.NotNil(t, status)
	assert.True(t, status.Installed)
	assert.Equal(t, "v1.14.2", status.Version)
}

func TestComponent_Uninstall(t *testing.T) {
	comp := NewComponent(&Config{
		Namespace: "cert-manager",
		Version:   "v1.14.2",
	})

	ctx := context.Background()
	err := comp.Uninstall(ctx)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not yet implemented")
}
