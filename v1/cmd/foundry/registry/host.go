package registry

import (
	"os"

	"github.com/catalystcommunity/foundry/v1/internal/config"
	"github.com/catalystcommunity/foundry/v1/internal/host"
)

// InitHostRegistry initializes the global host registry with the ConfigRegistry.
// This loads hosts from the stack configuration file and makes them available
// to all commands that use host.Get(), host.List(), etc.
func InitHostRegistry() error {
	return InitHostRegistryWithConfig("")
}

// InitHostRegistryWithConfig initializes the global host registry with a specific config path.
// If configPath is empty, it uses FindConfig to resolve the path (respecting --config flag).
func InitHostRegistryWithConfig(configPath string) error {
	// Resolve config path if not provided
	if configPath == "" {
		var err error
		configPath, err = config.FindConfig("")
		if err != nil {
			// Config doesn't exist yet (first run), create empty in-memory registry
			// Commands like 'config init' need to work before config exists
			registry := host.NewMemoryRegistry()
			host.SetDefaultRegistry(registry)
			return nil
		}
	}

	// Check if config exists
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		// If config doesn't exist yet (first run), create empty in-memory registry
		registry := host.NewMemoryRegistry()
		host.SetDefaultRegistry(registry)
		return nil
	}

	// Create ConfigLoader for hosts
	loader := config.NewHostConfigLoader(configPath)

	// Create ConfigRegistry
	registry := host.NewConfigRegistry(configPath, loader)

	// Set as default registry
	host.SetDefaultRegistry(registry)

	return nil
}
