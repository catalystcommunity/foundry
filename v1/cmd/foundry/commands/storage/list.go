package storage

import (
	"context"
	"fmt"
	"os"

	"github.com/catalystcommunity/foundry/v1/internal/config"
	"github.com/catalystcommunity/foundry/v1/internal/secrets"
	"github.com/catalystcommunity/foundry/v1/internal/storage/truenas"
	"github.com/urfave/cli/v3"
)

// ListCommand lists configured storage backends
var ListCommand = &cli.Command{
	Name:  "list",
	Usage: "List configured storage backends",
	Description: `Show all configured storage backends and their status.

This command displays:
  - Storage backend type (TrueNAS)
  - API URL
  - Connection status
  - Available storage pools and space`,
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:    "config",
			Aliases: []string{"c"},
			Usage:   "Path to config file",
			Sources: cli.EnvVars("FOUNDRY_CONFIG"),
		},
		&cli.BoolFlag{
			Name:  "detailed",
			Usage: "Show detailed pool information",
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
	if cfg.Storage == nil {
		fmt.Println("No storage backends configured")
		fmt.Println("\nTo configure TrueNAS storage, run:")
		fmt.Println("  foundry storage configure")
		return nil
	}

	fmt.Println("Storage Backends:")
	fmt.Println()

	// Check TrueNAS
	if cfg.Storage.TrueNAS != nil && cfg.Storage.TrueNAS.APIURL != "" {
		fmt.Println("TrueNAS:")
		fmt.Printf("  API URL: %s\n", cfg.Storage.TrueNAS.APIURL)

		// Try to get status
		status := "unknown"
		var pools []truenas.Pool

		// Resolve API key and test connection
		apiKey, err := resolveAPIKey(cfg)
		if err != nil {
			status = fmt.Sprintf("error: %v", err)
		} else if apiKey != "" {
			client, err := truenas.NewClient(cfg.Storage.TrueNAS.APIURL, apiKey)
			if err != nil {
				status = fmt.Sprintf("error: %v", err)
			} else {
				if err := client.Ping(); err != nil {
					status = "unreachable"
				} else {
					status = "connected"
					// Get pools if connected
					pools, _ = client.ListPools()
				}
			}
		}

		fmt.Printf("  Status: %s\n", status)

		// Show pools
		if len(pools) > 0 {
			fmt.Printf("  Pools: %d\n", len(pools))

			if cmd.Bool("detailed") {
				fmt.Println()
				for _, pool := range pools {
					healthStatus := "healthy"
					if !pool.Healthy {
						healthStatus = "unhealthy"
					}

					sizeGB := float64(pool.Size) / (1024 * 1024 * 1024)
					freeGB := float64(pool.Free) / (1024 * 1024 * 1024)
					usedGB := float64(pool.Allocated) / (1024 * 1024 * 1024)
					usedPct := 0.0
					if pool.Size > 0 {
						usedPct = (float64(pool.Allocated) / float64(pool.Size)) * 100
					}

					fmt.Printf("    %s:\n", pool.Name)
					fmt.Printf("      Status: %s (%s)\n", pool.Status, healthStatus)
					fmt.Printf("      Size: %.2f GB\n", sizeGB)
					fmt.Printf("      Used: %.2f GB (%.1f%%)\n", usedGB, usedPct)
					fmt.Printf("      Free: %.2f GB\n", freeGB)
				}
			} else {
				// Summary view
				for _, pool := range pools {
					healthStatus := "✓"
					if !pool.Healthy {
						healthStatus = "✗"
					}
					freeGB := float64(pool.Free) / (1024 * 1024 * 1024)
					fmt.Printf("    - %s %s (%.2f GB free)\n", healthStatus, pool.Name, freeGB)
				}
			}
		}
		fmt.Println()
	} else {
		fmt.Println("No storage backends configured")
		fmt.Println("\nTo configure TrueNAS storage, run:")
		fmt.Println("  foundry storage configure")
	}

	return nil
}

// resolveAPIKey attempts to resolve the TrueNAS API key from config or OpenBAO
func resolveAPIKey(cfg *config.Config) (string, error) {
	if cfg.Storage == nil || cfg.Storage.TrueNAS == nil {
		return "", fmt.Errorf("TrueNAS not configured")
	}

	apiKey := cfg.Storage.TrueNAS.APIKey

	// If it's a secret reference, resolve it
	if apiKey != "" && apiKey[0] == '$' {
		// Parse secret reference
		ref, err := secrets.ParseSecretRef(apiKey)
		if err != nil {
			return "", fmt.Errorf("invalid secret reference: %w", err)
		}

		// Try to load OpenBAO token
		token, err := secrets.LoadAuthToken()
		if err != nil || token == "" {
			return "", fmt.Errorf("OpenBAO token not found (run 'foundry setup' or authenticate)")
		}

		// Get OpenBAO URL
		openbaoURL := os.Getenv("OPENBAO_ADDR")
		if openbaoURL == "" {
			addr, err := cfg.GetPrimaryOpenBAOAddress()
			if err == nil {
				openbaoURL = fmt.Sprintf("https://%s:8200", addr)
			}
		}
		if openbaoURL == "" {
			return "", fmt.Errorf("OpenBAO address not found")
		}

		// Create resolver
		resolver, err := secrets.NewOpenBAOResolver(openbaoURL, token)
		if err != nil {
			return "", fmt.Errorf("failed to create OpenBAO client: %w", err)
		}

		// Create resolution context
		resCtx := &secrets.ResolutionContext{
			Instance: "foundry-core",
		}

		// Resolve the secret
		resolvedValue, err := resolver.Resolve(resCtx, *ref)
		if err != nil {
			return "", fmt.Errorf("failed to resolve API key from OpenBAO: %w", err)
		}

		return resolvedValue, nil
	}

	return apiKey, nil
}
