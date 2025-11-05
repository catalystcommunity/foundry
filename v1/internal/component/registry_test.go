package component

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockComponent is a simple test component
type mockComponent struct {
	name         string
	dependencies []string
}

func (m *mockComponent) Name() string {
	return m.name
}

func (m *mockComponent) Install(ctx context.Context, cfg ComponentConfig) error {
	return nil
}

func (m *mockComponent) Upgrade(ctx context.Context, cfg ComponentConfig) error {
	return nil
}

func (m *mockComponent) Status(ctx context.Context) (*ComponentStatus, error) {
	return &ComponentStatus{
		Installed: true,
		Version:   "1.0.0",
		Healthy:   true,
		Message:   "OK",
	}, nil
}

func (m *mockComponent) Uninstall(ctx context.Context) error {
	return nil
}

func (m *mockComponent) Dependencies() []string {
	return m.dependencies
}

func TestRegistry_Register(t *testing.T) {
	registry := NewRegistry()
	comp := &mockComponent{name: "test"}

	err := registry.Register(comp)
	assert.NoError(t, err)

	// Verify component is registered
	assert.True(t, registry.Has("test"))
	retrieved := registry.Get("test")
	assert.Equal(t, comp, retrieved)
}

func TestRegistry_RegisterDuplicate(t *testing.T) {
	registry := NewRegistry()
	comp1 := &mockComponent{name: "test"}
	comp2 := &mockComponent{name: "test"}

	err := registry.Register(comp1)
	require.NoError(t, err)

	// Registering duplicate should fail
	err = registry.Register(comp2)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "already registered")
}

func TestRegistry_Get(t *testing.T) {
	registry := NewRegistry()
	comp := &mockComponent{name: "test"}

	err := registry.Register(comp)
	require.NoError(t, err)

	// Get existing component
	retrieved := registry.Get("test")
	assert.Equal(t, comp, retrieved)

	// Get non-existent component
	notFound := registry.Get("nonexistent")
	assert.Nil(t, notFound)
}

func TestRegistry_Has(t *testing.T) {
	registry := NewRegistry()
	comp := &mockComponent{name: "test"}

	assert.False(t, registry.Has("test"))

	err := registry.Register(comp)
	require.NoError(t, err)

	assert.True(t, registry.Has("test"))
	assert.False(t, registry.Has("nonexistent"))
}

func TestRegistry_Unregister(t *testing.T) {
	registry := NewRegistry()
	comp := &mockComponent{name: "test"}

	err := registry.Register(comp)
	require.NoError(t, err)
	assert.True(t, registry.Has("test"))

	registry.Unregister("test")
	assert.False(t, registry.Has("test"))

	// Unregistering non-existent component should not panic
	registry.Unregister("nonexistent")
}

func TestRegistry_List(t *testing.T) {
	registry := NewRegistry()

	// Empty registry
	list := registry.List()
	assert.Empty(t, list)

	// Add components
	comp1 := &mockComponent{name: "comp1"}
	comp2 := &mockComponent{name: "comp2"}
	comp3 := &mockComponent{name: "comp3"}

	require.NoError(t, registry.Register(comp1))
	require.NoError(t, registry.Register(comp2))
	require.NoError(t, registry.Register(comp3))

	list = registry.List()
	assert.Len(t, list, 3)
	assert.Contains(t, list, "comp1")
	assert.Contains(t, list, "comp2")
	assert.Contains(t, list, "comp3")
}

func TestRegistry_GetAll(t *testing.T) {
	registry := NewRegistry()

	// Empty registry
	all := registry.GetAll()
	assert.Empty(t, all)

	// Add components
	comp1 := &mockComponent{name: "comp1"}
	comp2 := &mockComponent{name: "comp2"}

	require.NoError(t, registry.Register(comp1))
	require.NoError(t, registry.Register(comp2))

	all = registry.GetAll()
	assert.Len(t, all, 2)

	// Check that all components are present (order doesn't matter)
	names := make(map[string]bool)
	for _, comp := range all {
		names[comp.Name()] = true
	}
	assert.True(t, names["comp1"])
	assert.True(t, names["comp2"])
}

func TestDefaultRegistry(t *testing.T) {
	// Note: This test modifies the global DefaultRegistry
	// In a real scenario, we'd want to reset it after the test
	// For now, we'll use unique names to avoid conflicts

	comp := &mockComponent{name: "default-test"}

	err := Register(comp)
	assert.NoError(t, err)

	assert.True(t, Has("default-test"))
	retrieved := Get("default-test")
	assert.Equal(t, comp, retrieved)

	list := List()
	assert.Contains(t, list, "default-test")

	all := GetAll()
	found := false
	for _, c := range all {
		if c.Name() == "default-test" {
			found = true
			break
		}
	}
	assert.True(t, found)

	// Clean up
	DefaultRegistry.Unregister("default-test")
}

func TestRegistry_ThreadSafety(t *testing.T) {
	registry := NewRegistry()

	// Launch multiple goroutines registering components
	done := make(chan bool, 10)
	for i := 0; i < 10; i++ {
		go func(id int) {
			comp := &mockComponent{name: string(rune('a' + id))}
			registry.Register(comp)
			done <- true
		}(i)
	}

	// Wait for all goroutines to finish
	for i := 0; i < 10; i++ {
		<-done
	}

	// Verify all components are registered
	list := registry.List()
	assert.Len(t, list, 10)
}
