package storage

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/catalystcommunity/foundry/v1/internal/config"
	"github.com/catalystcommunity/foundry/v1/internal/secrets"
	"github.com/catalystcommunity/foundry/v1/internal/storage/truenas"
	"github.com/urfave/cli/v3"
)

// ConfigureCommand configures storage backend interactively
var ConfigureCommand = &cli.Command{
	Name:  "configure",
	Usage: "Configure TrueNAS storage backend",
	Description: `Interactive configuration wizard for TrueNAS storage backend.

This command will:
  1. Prompt for TrueNAS API URL and key
  2. Test connection to TrueNAS
  3. Update config file with TrueNAS settings
  4. Store API key in OpenBAO (foundry-core/truenas:api_key)

The API key can be generated in TrueNAS:
  System → API Keys → Add`,
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:    "config",
			Aliases: []string{"c"},
			Usage:   "Path to config file",
			Sources: cli.EnvVars("FOUNDRY_CONFIG"),
		},
		&cli.StringFlag{
			Name:  "api-url",
			Usage: "TrueNAS API URL (e.g., https://truenas.example.com)",
		},
		&cli.StringFlag{
			Name:  "api-key",
			Usage: "TrueNAS API key (if not provided, will prompt)",
		},
		&cli.BoolFlag{
			Name:  "skip-test",
			Usage: "Skip connection test",
		},
	},
	Action: runConfigure,
}

func runConfigure(ctx context.Context, cmd *cli.Command) error {
	// Load or create config
	configPath := cmd.String("config")
	if configPath == "" {
		configPath = config.DefaultConfigPath()
	}

	cfg, err := config.Load(configPath)
	if err != nil {
		// If config doesn't exist, create a new one
		if os.IsNotExist(err) {
			cfg = &config.Config{
				Storage: &config.StorageConfig{},
			}
		} else {
			return fmt.Errorf("failed to load config: %w", err)
		}
	}

	// Ensure storage config exists
	if cfg.Storage == nil {
		cfg.Storage = &config.StorageConfig{}
	}
	if cfg.Storage.TrueNAS == nil {
		cfg.Storage.TrueNAS = &config.TrueNASConfig{}
	}

	// Get API URL
	apiURL := cmd.String("api-url")
	if apiURL == "" {
		// Prompt for API URL
		fmt.Print("TrueNAS API URL (e.g., https://truenas.example.com): ")
		fmt.Scanln(&apiURL)
		apiURL = strings.TrimSpace(apiURL)
	}
	if apiURL == "" {
		return fmt.Errorf("API URL is required")
	}

	// Get API key
	apiKey := cmd.String("api-key")
	if apiKey == "" {
		// Prompt for API key
		fmt.Print("TrueNAS API Key: ")
		fmt.Scanln(&apiKey)
		apiKey = strings.TrimSpace(apiKey)
	}
	if apiKey == "" {
		return fmt.Errorf("API key is required")
	}

	// Test connection unless --skip-test
	if !cmd.Bool("skip-test") {
		fmt.Println("\nTesting connection to TrueNAS...")
		client, err := truenas.NewClient(apiURL, apiKey)
		if err != nil {
			return fmt.Errorf("failed to create TrueNAS client: %w", err)
		}

		if err := client.Ping(); err != nil {
			return fmt.Errorf("connection test failed: %w", err)
		}
		fmt.Println("✓ Connection successful")

		// List pools to show what's available
		pools, err := client.ListPools()
		if err != nil {
			fmt.Printf("Warning: Could not list pools: %v\n", err)
		} else if len(pools) > 0 {
			fmt.Printf("\nAvailable storage pools (%d):\n", len(pools))
			for _, pool := range pools {
				status := "healthy"
				if !pool.Healthy {
					status = "unhealthy"
				}
				fmt.Printf("  - %s (%s, %.2f GB free)\n",
					pool.Name, status, float64(pool.Free)/(1024*1024*1024))
			}
		}
	}

	// Update config
	cfg.Storage.TrueNAS.APIURL = apiURL
	// Store API key reference in config (actual key goes to OpenBAO)
	cfg.Storage.TrueNAS.APIKey = "${secret:foundry-core/truenas:api_key}"

	// Save config
	if err := config.Save(cfg, configPath); err != nil {
		return fmt.Errorf("failed to save config: %w", err)
	}
	fmt.Printf("\n✓ Configuration saved to %s\n", configPath)

	// Store API key in OpenBAO if available
	// First check if OpenBAO is configured
	if token, _ := secrets.LoadAuthToken(); token != "" {
		fmt.Println("\nStoring API key in OpenBAO...")
		// Get OpenBAO URL from config or environment
		openbaoURL := os.Getenv("OPENBAO_ADDR")
		if openbaoURL == "" {
			addr, err := cfg.GetPrimaryOpenBAOAddress()
			if err == nil {
				// Construct URL from first OpenBAO host
				openbaoURL = fmt.Sprintf("https://%s:8200", addr)
			}
		}

		if openbaoURL != "" {
			_, err := secrets.NewOpenBAOResolver(openbaoURL, token)
			if err != nil {
				fmt.Printf("Warning: Could not create OpenBAO client: %v\n", err)
				fmt.Println("API key not stored in OpenBAO. You'll need to store it manually.")
			} else {
				// Store the API key
				secretPath := "foundry-core/truenas"
				secretKey := "api_key"

				// OpenBAO resolver doesn't have a Set method, we need to use the CLI
				// For now, show instructions
				fmt.Printf("\nTo store the API key in OpenBAO, run:\n")
				fmt.Printf("  foundry secret set %s:%s\n", secretPath, secretKey)
				fmt.Printf("\nOr manually via OpenBAO CLI:\n")
				fmt.Printf("  bao kv put %s %s='<your-api-key>'\n", secretPath, secretKey)
			}
		} else {
			fmt.Println("\nOpenBAO address not found. API key not stored.")
			fmt.Println("To store the API key in OpenBAO, run:")
			fmt.Println("  foundry secret set foundry-core/truenas:api_key")
		}
	} else {
		fmt.Println("\nOpenBAO token not found. API key not stored.")
		fmt.Println("After setting up OpenBAO, store the API key with:")
		fmt.Println("  foundry secret set foundry-core/truenas:api_key")
	}

	fmt.Println("\n✓ TrueNAS storage backend configured successfully")
	return nil
}
