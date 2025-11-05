package component

import (
	"fmt"
	"sync"
)

// Registry manages all available components
type Registry struct {
	mu         sync.RWMutex
	components map[string]Component
}

// NewRegistry creates a new component registry
func NewRegistry() *Registry {
	return &Registry{
		components: make(map[string]Component),
	}
}

// Register adds a component to the registry
// Returns an error if a component with the same name already exists
func (r *Registry) Register(component Component) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	name := component.Name()
	if _, exists := r.components[name]; exists {
		return fmt.Errorf("component %q is already registered", name)
	}

	r.components[name] = component
	return nil
}

// Unregister removes a component from the registry
func (r *Registry) Unregister(name string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	delete(r.components, name)
}

// Get retrieves a component by name
// Returns nil if the component is not found
func (r *Registry) Get(name string) Component {
	r.mu.RLock()
	defer r.mu.RUnlock()

	return r.components[name]
}

// Has checks if a component is registered
func (r *Registry) Has(name string) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()

	_, exists := r.components[name]
	return exists
}

// List returns all registered component names
func (r *Registry) List() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	names := make([]string, 0, len(r.components))
	for name := range r.components {
		names = append(names, name)
	}
	return names
}

// GetAll returns all registered components
func (r *Registry) GetAll() []Component {
	r.mu.RLock()
	defer r.mu.RUnlock()

	components := make([]Component, 0, len(r.components))
	for _, component := range r.components {
		components = append(components, component)
	}
	return components
}

// DefaultRegistry is the global component registry
var DefaultRegistry = NewRegistry()

// Register adds a component to the default registry
func Register(component Component) error {
	return DefaultRegistry.Register(component)
}

// Get retrieves a component from the default registry
func Get(name string) Component {
	return DefaultRegistry.Get(name)
}

// Has checks if a component exists in the default registry
func Has(name string) bool {
	return DefaultRegistry.Has(name)
}

// List returns all registered component names from the default registry
func List() []string {
	return DefaultRegistry.List()
}

// GetAll returns all registered components from the default registry
func GetAll() []Component {
	return DefaultRegistry.GetAll()
}
