package storage

import (
	"context"
	"fmt"

	"github.com/catalystcommunity/foundry/v1/internal/config"
	"github.com/urfave/cli/v3"
)

// ListCommand lists configured storage backends
var ListCommand = &cli.Command{
	Name:  "list",
	Usage: "List configured storage backends",
	Description: `Show all configured storage backends.

This command displays:
  - Storage backend type (local-path, nfs, longhorn)
  - Configuration status`,
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:    "config",
			Aliases: []string{"c"},
			Usage:   "Path to config file",
			Sources: cli.EnvVars("FOUNDRY_CONFIG"),
		},
	},
	Action: runList,
}

func runList(ctx context.Context, cmd *cli.Command) error {
	// Load config
	configPath := cmd.String("config")
	if configPath == "" {
		configPath = config.DefaultConfigPath()
	}

	cfg, err := config.Load(configPath)
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	// Check if storage is configured
	if cfg.Storage == nil || cfg.Storage.Backend == "" {
		fmt.Println("No storage backend explicitly configured")
		fmt.Println("\nDefault backend: local-path (K3s bundled)")
		fmt.Println("\nTo configure a different storage backend, set storage.backend in your config:")
		fmt.Println("  storage:")
		fmt.Println("    backend: longhorn  # or: local-path, nfs")
		return nil
	}

	fmt.Println("Storage Backend Configuration:")
	fmt.Println()
	fmt.Printf("  Backend: %s\n", cfg.Storage.Backend)

	switch cfg.Storage.Backend {
	case "longhorn":
		fmt.Println("  Type: Distributed block storage")
		fmt.Println("  Storage Class: longhorn")
	case "local-path":
		fmt.Println("  Type: Local path provisioner")
		fmt.Println("  Storage Class: local-path")
	case "nfs":
		fmt.Println("  Type: NFS subdir provisioner")
		fmt.Println("  Storage Class: nfs-client")
	default:
		fmt.Printf("  Type: Custom (%s)\n", cfg.Storage.Backend)
	}

	fmt.Println()
	return nil
}
