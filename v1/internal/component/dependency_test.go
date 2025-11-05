package component

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestResolveInstallationOrder_NoDependencies(t *testing.T) {
	registry := NewRegistry()

	comp1 := &mockComponent{name: "comp1", dependencies: []string{}}
	comp2 := &mockComponent{name: "comp2", dependencies: []string{}}
	comp3 := &mockComponent{name: "comp3", dependencies: []string{}}

	require.NoError(t, registry.Register(comp1))
	require.NoError(t, registry.Register(comp2))
	require.NoError(t, registry.Register(comp3))

	order, err := ResolveInstallationOrder(registry, []string{"comp1", "comp2", "comp3"})
	require.NoError(t, err)
	assert.Len(t, order, 3)
	assert.Contains(t, order, "comp1")
	assert.Contains(t, order, "comp2")
	assert.Contains(t, order, "comp3")
}

func TestResolveInstallationOrder_LinearDependencies(t *testing.T) {
	registry := NewRegistry()

	// comp3 depends on comp2, comp2 depends on comp1
	comp1 := &mockComponent{name: "comp1", dependencies: []string{}}
	comp2 := &mockComponent{name: "comp2", dependencies: []string{"comp1"}}
	comp3 := &mockComponent{name: "comp3", dependencies: []string{"comp2"}}

	require.NoError(t, registry.Register(comp1))
	require.NoError(t, registry.Register(comp2))
	require.NoError(t, registry.Register(comp3))

	order, err := ResolveInstallationOrder(registry, []string{"comp3"})
	require.NoError(t, err)
	assert.Equal(t, []string{"comp1", "comp2", "comp3"}, order)
}

func TestResolveInstallationOrder_MultipleDependencies(t *testing.T) {
	registry := NewRegistry()

	// comp4 depends on comp2 and comp3
	// comp2 depends on comp1
	// comp3 depends on comp1
	comp1 := &mockComponent{name: "comp1", dependencies: []string{}}
	comp2 := &mockComponent{name: "comp2", dependencies: []string{"comp1"}}
	comp3 := &mockComponent{name: "comp3", dependencies: []string{"comp1"}}
	comp4 := &mockComponent{name: "comp4", dependencies: []string{"comp2", "comp3"}}

	require.NoError(t, registry.Register(comp1))
	require.NoError(t, registry.Register(comp2))
	require.NoError(t, registry.Register(comp3))
	require.NoError(t, registry.Register(comp4))

	order, err := ResolveInstallationOrder(registry, []string{"comp4"})
	require.NoError(t, err)
	assert.Len(t, order, 4)

	// comp1 must come first
	assert.Equal(t, "comp1", order[0])

	// comp2 and comp3 must come before comp4
	comp2Idx := -1
	comp3Idx := -1
	comp4Idx := -1
	for i, name := range order {
		switch name {
		case "comp2":
			comp2Idx = i
		case "comp3":
			comp3Idx = i
		case "comp4":
			comp4Idx = i
		}
	}

	assert.True(t, comp2Idx < comp4Idx, "comp2 must come before comp4")
	assert.True(t, comp3Idx < comp4Idx, "comp3 must come before comp4")
}

func TestResolveInstallationOrder_CircularDependency(t *testing.T) {
	registry := NewRegistry()

	// comp1 → comp2 → comp3 → comp1 (circular)
	comp1 := &mockComponent{name: "comp1", dependencies: []string{"comp3"}}
	comp2 := &mockComponent{name: "comp2", dependencies: []string{"comp1"}}
	comp3 := &mockComponent{name: "comp3", dependencies: []string{"comp2"}}

	require.NoError(t, registry.Register(comp1))
	require.NoError(t, registry.Register(comp2))
	require.NoError(t, registry.Register(comp3))

	_, err := ResolveInstallationOrder(registry, []string{"comp1"})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "circular dependency")
}

func TestResolveInstallationOrder_SelfDependency(t *testing.T) {
	registry := NewRegistry()

	// comp1 depends on itself
	comp1 := &mockComponent{name: "comp1", dependencies: []string{"comp1"}}

	require.NoError(t, registry.Register(comp1))

	_, err := ResolveInstallationOrder(registry, []string{"comp1"})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "circular dependency")
}

func TestResolveInstallationOrder_ComponentNotFound(t *testing.T) {
	registry := NewRegistry()

	comp1 := &mockComponent{name: "comp1", dependencies: []string{}}
	require.NoError(t, registry.Register(comp1))

	_, err := ResolveInstallationOrder(registry, []string{"nonexistent"})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not found in registry")
}

func TestResolveInstallationOrder_DependencyNotFound(t *testing.T) {
	registry := NewRegistry()

	// comp1 depends on "missing" which doesn't exist
	comp1 := &mockComponent{name: "comp1", dependencies: []string{"missing"}}
	require.NoError(t, registry.Register(comp1))

	_, err := ResolveInstallationOrder(registry, []string{"comp1"})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not found in registry")
}

func TestResolveInstallationOrder_DiamondDependency(t *testing.T) {
	registry := NewRegistry()

	// Diamond dependency:
	//     comp1
	//    /     \
	// comp2   comp3
	//    \     /
	//     comp4
	comp1 := &mockComponent{name: "comp1", dependencies: []string{}}
	comp2 := &mockComponent{name: "comp2", dependencies: []string{"comp1"}}
	comp3 := &mockComponent{name: "comp3", dependencies: []string{"comp1"}}
	comp4 := &mockComponent{name: "comp4", dependencies: []string{"comp2", "comp3"}}

	require.NoError(t, registry.Register(comp1))
	require.NoError(t, registry.Register(comp2))
	require.NoError(t, registry.Register(comp3))
	require.NoError(t, registry.Register(comp4))

	order, err := ResolveInstallationOrder(registry, []string{"comp4"})
	require.NoError(t, err)
	assert.Len(t, order, 4)

	// comp1 must be first
	assert.Equal(t, "comp1", order[0])

	// comp4 must be last
	assert.Equal(t, "comp4", order[3])

	// comp2 and comp3 must be before comp4 (order between them doesn't matter)
	assert.True(t, order[1] == "comp2" || order[1] == "comp3")
	assert.True(t, order[2] == "comp2" || order[2] == "comp3")
}

func TestValidateDependencies_AllPresent(t *testing.T) {
	registry := NewRegistry()

	comp1 := &mockComponent{name: "comp1", dependencies: []string{}}
	comp2 := &mockComponent{name: "comp2", dependencies: []string{"comp1"}}
	comp3 := &mockComponent{name: "comp3", dependencies: []string{"comp2"}}

	require.NoError(t, registry.Register(comp1))
	require.NoError(t, registry.Register(comp2))
	require.NoError(t, registry.Register(comp3))

	err := ValidateDependencies(registry, []string{"comp3"})
	assert.NoError(t, err)
}

func TestValidateDependencies_MissingDependencies(t *testing.T) {
	registry := NewRegistry()

	// comp2 depends on "comp1" which is not registered
	comp2 := &mockComponent{name: "comp2", dependencies: []string{"comp1"}}
	require.NoError(t, registry.Register(comp2))

	err := ValidateDependencies(registry, []string{"comp2"})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "missing dependencies")
	assert.Contains(t, err.Error(), "comp2")
	assert.Contains(t, err.Error(), "comp1")
}

func TestValidateDependencies_ComponentNotFound(t *testing.T) {
	registry := NewRegistry()

	err := ValidateDependencies(registry, []string{"nonexistent"})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

func TestValidateDependencies_TransitiveMissing(t *testing.T) {
	registry := NewRegistry()

	// comp2 depends on comp1 (missing)
	// comp3 depends on comp2 (present)
	comp2 := &mockComponent{name: "comp2", dependencies: []string{"comp1"}}
	comp3 := &mockComponent{name: "comp3", dependencies: []string{"comp2"}}

	require.NoError(t, registry.Register(comp2))
	require.NoError(t, registry.Register(comp3))

	err := ValidateDependencies(registry, []string{"comp3"})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "missing dependencies")
}

func TestHasCircularDependencies_NoCircular(t *testing.T) {
	registry := NewRegistry()

	comp1 := &mockComponent{name: "comp1", dependencies: []string{}}
	comp2 := &mockComponent{name: "comp2", dependencies: []string{"comp1"}}

	require.NoError(t, registry.Register(comp1))
	require.NoError(t, registry.Register(comp2))

	hasCircular, err := HasCircularDependencies(registry)
	assert.NoError(t, err)
	assert.False(t, hasCircular)
}

func TestHasCircularDependencies_WithCircular(t *testing.T) {
	registry := NewRegistry()

	// comp1 → comp2 → comp1
	comp1 := &mockComponent{name: "comp1", dependencies: []string{"comp2"}}
	comp2 := &mockComponent{name: "comp2", dependencies: []string{"comp1"}}

	require.NoError(t, registry.Register(comp1))
	require.NoError(t, registry.Register(comp2))

	hasCircular, err := HasCircularDependencies(registry)
	assert.True(t, hasCircular)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "circular dependency")
}

func TestHasCircularDependencies_MissingComponent(t *testing.T) {
	registry := NewRegistry()

	// comp1 depends on missing "comp2"
	comp1 := &mockComponent{name: "comp1", dependencies: []string{"comp2"}}
	require.NoError(t, registry.Register(comp1))

	hasCircular, err := HasCircularDependencies(registry)
	assert.False(t, hasCircular)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}
