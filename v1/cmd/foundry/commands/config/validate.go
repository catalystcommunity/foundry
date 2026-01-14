package config

import (
	"context"
	"fmt"

	"github.com/catalystcommunity/foundry/v1/internal/config"
	"github.com/urfave/cli/v3"
)

// ValidateCommand validates a configuration file
var ValidateCommand = &cli.Command{
	Name:      "validate",
	Usage:     "Validate configuration file syntax and structure",
	ArgsUsage: "[config-file]",
	Action:    runValidate,
}

func runValidate(ctx context.Context, cmd *cli.Command) error {
	// Determine config path
	configPath := cmd.String("config")
	if cmd.Args().Len() > 0 {
		configPath = cmd.Args().First()
	}

	if configPath == "" {
		// Try to find default config
		path, err := config.FindConfig("stack")
		if err != nil {
			return fmt.Errorf("no config file specified and no default found: %w", err)
		}
		configPath = path
	}

	// Load the config
	cfg, err := config.Load(configPath)
	if err != nil {
		return fmt.Errorf("validation failed: %w", err)
	}

	// Validate secret references (syntax only, no resolution)
	if err := config.ValidateSecretRefs(cfg); err != nil {
		return fmt.Errorf("secret reference validation failed: %w", err)
	}

	fmt.Printf("✓ Configuration is valid: %s\n", configPath)
	fmt.Printf("  Cluster: %s (%s)\n", cfg.Cluster.Name, cfg.Cluster.PrimaryDomain)
	fmt.Printf("  Hosts: %d\n", len(cfg.Hosts))
	fmt.Printf("  Components: %d\n", len(cfg.Components))

	return nil
}
