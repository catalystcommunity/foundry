package component

import (
	"context"
	"fmt"
)

// Component represents a deployable infrastructure component (OpenBAO, Zot, K3s, etc.)
type Component interface {
	// Name returns the unique identifier for this component
	Name() string

	// Install installs the component with the given configuration
	Install(ctx context.Context, cfg ComponentConfig) error

	// Upgrade upgrades the component to the version in the configuration
	Upgrade(ctx context.Context, cfg ComponentConfig) error

	// Status queries the current status of the component
	Status(ctx context.Context) (*ComponentStatus, error)

	// Uninstall removes the component
	Uninstall(ctx context.Context) error

	// Dependencies returns the list of component names this component depends on
	Dependencies() []string
}

// ComponentStatus represents the runtime status of a component
type ComponentStatus struct {
	// Installed indicates whether the component is installed
	Installed bool

	// Version is the currently installed version (empty if not installed)
	Version string

	// Healthy indicates whether the component is functioning correctly
	Healthy bool

	// Message provides additional status information (errors, warnings, etc.)
	Message string
}

// ComponentConfig is a flexible configuration map for component installation
// Keys and values vary by component type
type ComponentConfig map[string]interface{}

// Get retrieves a value from the config by key
func (c ComponentConfig) Get(key string) (interface{}, bool) {
	val, ok := c[key]
	return val, ok
}

// GetString retrieves a string value from the config
func (c ComponentConfig) GetString(key string) (string, bool) {
	val, ok := c[key]
	if !ok {
		return "", false
	}
	str, ok := val.(string)
	return str, ok
}

// GetInt retrieves an int value from the config
func (c ComponentConfig) GetInt(key string) (int, bool) {
	val, ok := c[key]
	if !ok {
		return 0, false
	}
	// Handle both int and float64 (JSON unmarshaling produces float64)
	switch v := val.(type) {
	case int:
		return v, true
	case float64:
		return int(v), true
	default:
		return 0, false
	}
}

// GetBool retrieves a bool value from the config
func (c ComponentConfig) GetBool(key string) (bool, bool) {
	val, ok := c[key]
	if !ok {
		return false, false
	}
	b, ok := val.(bool)
	return b, ok
}

// GetMap retrieves a nested map from the config
func (c ComponentConfig) GetMap(key string) (map[string]interface{}, bool) {
	val, ok := c[key]
	if !ok {
		return nil, false
	}
	m, ok := val.(map[string]interface{})
	return m, ok
}

// GetStringSlice retrieves a string slice from the config
func (c ComponentConfig) GetStringSlice(key string) ([]string, bool) {
	val, ok := c[key]
	if !ok {
		return nil, false
	}
	// Handle []interface{} from JSON unmarshaling
	if slice, ok := val.([]interface{}); ok {
		result := make([]string, 0, len(slice))
		for _, item := range slice {
			if str, ok := item.(string); ok {
				result = append(result, str)
			} else {
				return nil, false
			}
		}
		return result, true
	}
	// Handle direct []string
	if slice, ok := val.([]string); ok {
		return slice, true
	}
	return nil, false
}

// ErrComponentNotFound returns an error indicating that a component was not found
func ErrComponentNotFound(name string) error {
	return fmt.Errorf("component %q not found in registry", name)
}
