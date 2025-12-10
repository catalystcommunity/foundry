package registry

import (
	"testing"

	"github.com/catalystcommunity/foundry/v1/internal/component"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestInitComponents(t *testing.T) {
	// Create a fresh registry for testing
	testRegistry := component.NewRegistry()

	// Temporarily replace the default registry
	oldRegistry := component.DefaultRegistry
	component.DefaultRegistry = testRegistry
	defer func() { component.DefaultRegistry = oldRegistry }()

	// Initialize the registry
	err := InitComponents()
	require.NoError(t, err)

	// Verify all expected components are registered
	expectedComponents := []string{
		"openbao",
		"dns",
		"zot",
		"k3s",
		"gateway-api",
		"contour",
		"cert-manager",
		"storage",
		"seaweedfs",
		"prometheus",
		"loki",
		"grafana",
		"external-dns",
		"velero",
	}
	for _, name := range expectedComponents {
		assert.True(t, testRegistry.Has(name), "component %s should be registered", name)
	}

	// Verify total count
	assert.Equal(t, len(expectedComponents), len(testRegistry.List()))
}

func TestInitComponents_ComponentNames(t *testing.T) {
	// Create a fresh registry for testing
	testRegistry := component.NewRegistry()

	// Temporarily replace the default registry
	oldRegistry := component.DefaultRegistry
	component.DefaultRegistry = testRegistry
	defer func() { component.DefaultRegistry = oldRegistry }()

	err := InitComponents()
	require.NoError(t, err)

	// Get each component and verify its name matches
	tests := []struct {
		name string
	}{
		{name: "openbao"},
		{name: "dns"},
		{name: "zot"},
		{name: "k3s"},
		{name: "gateway-api"},
		{name: "contour"},
		{name: "cert-manager"},
		{name: "storage"},
		{name: "seaweedfs"},
		{name: "prometheus"},
		{name: "loki"},
		{name: "grafana"},
		{name: "external-dns"},
		{name: "velero"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			comp := testRegistry.Get(tt.name)
			require.NotNil(t, comp, "component %s should exist", tt.name)
			assert.Equal(t, tt.name, comp.Name())
		})
	}
}

func TestInitComponents_Dependencies(t *testing.T) {
	// Create a fresh registry for testing
	testRegistry := component.NewRegistry()

	// Temporarily replace the default registry
	oldRegistry := component.DefaultRegistry
	component.DefaultRegistry = testRegistry
	defer func() { component.DefaultRegistry = oldRegistry }()

	err := InitComponents()
	require.NoError(t, err)

	tests := []struct {
		name         string
		dependencies []string
	}{
		{
			name:         "openbao",
			dependencies: []string{}, // No dependencies
		},
		{
			name:         "dns",
			dependencies: []string{"openbao"},
		},
		{
			name:         "zot",
			dependencies: []string{"openbao", "dns"},
		},
		{
			name:         "k3s",
			dependencies: []string{"openbao", "dns", "zot"},
		},
		{
			name:         "gateway-api",
			dependencies: []string{"k3s"},
		},
		{
			name:         "contour",
			dependencies: []string{"k3s", "gateway-api"},
		},
		{
			name:         "cert-manager",
			dependencies: []string{"k3s"},
		},
		{
			name:         "storage",
			dependencies: []string{"k3s"},
		},
		{
			name:         "seaweedfs",
			dependencies: []string{"storage"},
		},
		{
			name:         "prometheus",
			dependencies: []string{"storage"},
		},
		{
			name:         "loki",
			dependencies: []string{"storage", "seaweedfs"},
		},
		{
			name:         "grafana",
			dependencies: []string{"prometheus", "loki"},
		},
		{
			name:         "external-dns",
			dependencies: []string{}, // No strict dependencies
		},
		{
			name:         "velero",
			dependencies: []string{"seaweedfs"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			comp := testRegistry.Get(tt.name)
			require.NotNil(t, comp)

			deps := comp.Dependencies()
			assert.Equal(t, tt.dependencies, deps, "dependencies for %s should match", tt.name)
		})
	}
}

func TestInitComponents_Idempotent(t *testing.T) {
	// Create a fresh registry for testing
	testRegistry := component.NewRegistry()

	// Temporarily replace the default registry
	oldRegistry := component.DefaultRegistry
	component.DefaultRegistry = testRegistry
	defer func() { component.DefaultRegistry = oldRegistry }()

	// First init should succeed
	err := InitComponents()
	require.NoError(t, err)

	// Second init should fail (components already registered)
	err = InitComponents()
	assert.Error(t, err, "re-registering components should fail")
	assert.Contains(t, err.Error(), "already registered")
}
