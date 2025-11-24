package config

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/catalystcommunity/foundry/v1/internal/config"
	"github.com/urfave/cli/v3"
)

// ListCommand lists all available configuration files
var ListCommand = &cli.Command{
	Name:    "list",
	Aliases: []string{"ls"},
	Usage:   "List available configuration files",
	Action:  runList,
}

func runList(ctx context.Context, cmd *cli.Command) error {
	// Get config directory
	configDir, err := config.GetConfigDir()
	if err != nil {
		return fmt.Errorf("failed to get config directory: %w", err)
	}

	// Check if directory exists
	if _, err := os.Stat(configDir); os.IsNotExist(err) {
		fmt.Printf("No configuration directory found at: %s\n", configDir)
		fmt.Println("Run 'foundry config init' to create your first configuration.")
		return nil
	}

	// List all configs
	configs, err := config.ListConfigs()
	if err != nil {
		return fmt.Errorf("failed to list configs: %w", err)
	}

	if len(configs) == 0 {
		fmt.Printf("No configuration files found in: %s\n", configDir)
		fmt.Println("Run 'foundry config init' to create your first configuration.")
		return nil
	}

	// Determine which config would be used by default
	defaultConfig := ""
	if path, err := config.FindConfig("stack"); err == nil {
		defaultConfig = filepath.Base(path)
	}

	// Display configs
	fmt.Printf("Configuration files in %s:\n\n", configDir)

	for _, cfgPath := range configs {
		name := filepath.Base(cfgPath)
		marker := " "

		// Mark default config
		if name == defaultConfig {
			marker = "*"
		}

		// Try to load and get basic info
		cfg, err := config.Load(cfgPath)
		if err != nil {
			fmt.Printf("  %s %s (invalid: %v)\n", marker, name, err)
			continue
		}

		// Display config info
		fmt.Printf("  %s %s\n", marker, name)
		fmt.Printf("      Cluster: %s (%s)\n", cfg.Cluster.Name, cfg.Cluster.Domain)
		fmt.Printf("      Hosts: %d, Components: %d\n", len(cfg.Hosts), len(cfg.Components))
	}

	if defaultConfig != "" {
		fmt.Printf("\n* = default config\n")
	}

	return nil
}
