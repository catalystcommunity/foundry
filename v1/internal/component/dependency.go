package component

import (
	"fmt"
)

// ResolveInstallationOrder takes a list of component names and returns them
// in the order they should be installed based on dependencies.
// Returns an error if:
// - A component is not found in the registry
// - A circular dependency is detected
// - A dependency is not found
func ResolveInstallationOrder(registry *Registry, componentNames []string) ([]string, error) {
	// Build a set of requested components for quick lookup
	requested := make(map[string]bool)
	for _, name := range componentNames {
		requested[name] = true
	}

	// Collect all components (requested + their dependencies)
	allComponents := make(map[string]Component)
	var collectDeps func(string) error
	collectDeps = func(name string) error {
		// Skip if already collected
		if _, seen := allComponents[name]; seen {
			return nil
		}

		// Get component from registry
		comp := registry.Get(name)
		if comp == nil {
			return fmt.Errorf("component %q not found in registry", name)
		}

		allComponents[name] = comp

		// Recursively collect dependencies
		for _, dep := range comp.Dependencies() {
			if err := collectDeps(dep); err != nil {
				return err
			}
		}

		return nil
	}

	// Collect all dependencies for requested components
	for _, name := range componentNames {
		if err := collectDeps(name); err != nil {
			return nil, err
		}
	}

	// Topological sort using DFS
	order := make([]string, 0, len(allComponents))
	visited := make(map[string]bool)
	visiting := make(map[string]bool) // For cycle detection

	var visit func(string) error
	visit = func(name string) error {
		// Cycle detection
		if visiting[name] {
			return fmt.Errorf("circular dependency detected involving component %q", name)
		}

		// Already visited
		if visited[name] {
			return nil
		}

		visiting[name] = true

		comp := allComponents[name]
		for _, dep := range comp.Dependencies() {
			if err := visit(dep); err != nil {
				return err
			}
		}

		visiting[name] = false
		visited[name] = true
		order = append(order, name)

		return nil
	}

	// Visit all components
	for name := range allComponents {
		if err := visit(name); err != nil {
			return nil, err
		}
	}

	return order, nil
}

// ValidateDependencies checks if all dependencies for the given components
// are registered in the registry. Returns an error listing any missing dependencies.
func ValidateDependencies(registry *Registry, componentNames []string) error {
	missing := make(map[string][]string) // component -> list of missing deps

	var checkDeps func(string, map[string]bool) error
	checkDeps = func(name string, seen map[string]bool) error {
		// Prevent infinite recursion
		if seen[name] {
			return nil
		}
		seen[name] = true

		comp := registry.Get(name)
		if comp == nil {
			return fmt.Errorf("component %q not found", name)
		}

		for _, dep := range comp.Dependencies() {
			if !registry.Has(dep) {
				if missing[name] == nil {
					missing[name] = []string{}
				}
				missing[name] = append(missing[name], dep)
			} else {
				// Recursively check dependencies
				if err := checkDeps(dep, seen); err != nil {
					return err
				}
			}
		}

		return nil
	}

	for _, name := range componentNames {
		if err := checkDeps(name, make(map[string]bool)); err != nil {
			return err
		}
	}

	if len(missing) > 0 {
		errMsg := "missing dependencies:"
		for comp, deps := range missing {
			errMsg += fmt.Sprintf("\n  %s requires: %v", comp, deps)
		}
		return fmt.Errorf("%s", errMsg)
	}

	return nil
}

// HasCircularDependencies detects if there are any circular dependencies
// in the registered components
func HasCircularDependencies(registry *Registry) (bool, error) {
	allComponents := registry.List()
	_, err := ResolveInstallationOrder(registry, allComponents)
	if err != nil {
		// Check if it's a circular dependency error
		if isCircularDependencyError(err) {
			return true, err
		}
		// Some other error
		return false, err
	}
	return false, nil
}

// isCircularDependencyError checks if an error is a circular dependency error
func isCircularDependencyError(err error) bool {
	if err == nil {
		return false
	}
	// Simple string check - could be improved with custom error types
	return len(err.Error()) > 0 && err.Error()[:8] == "circular"
}
