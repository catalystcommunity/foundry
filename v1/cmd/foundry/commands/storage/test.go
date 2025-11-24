package storage

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/catalystcommunity/foundry/v1/internal/config"
	"github.com/catalystcommunity/foundry/v1/internal/storage/truenas"
	"github.com/urfave/cli/v3"
)

// TestCommand tests storage backend connectivity
var TestCommand = &cli.Command{
	Name:  "test",
	Usage: "Test storage backend connectivity",
	Description: `Test connectivity and permissions for configured storage backend.

This command will:
  1. Test API connection to TrueNAS
  2. List available pools
  3. Create a test dataset (if --full-test)
  4. Delete the test dataset (cleanup)

The test dataset is created as: <pool>/foundry-test-<timestamp>`,
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:    "config",
			Aliases: []string{"c"},
			Usage:   "Path to config file",
			Sources: cli.EnvVars("FOUNDRY_CONFIG"),
		},
		&cli.BoolFlag{
			Name:  "full-test",
			Usage: "Run full test including dataset creation/deletion",
		},
		&cli.StringFlag{
			Name:  "pool",
			Usage: "Pool to use for full test (defaults to first available pool)",
		},
	},
	Action: runTest,
}

func runTest(ctx context.Context, cmd *cli.Command) error {
	// Load config
	configPath := cmd.String("config")
	if configPath == "" {
		configPath = config.DefaultConfigPath()
	}

	cfg, err := config.Load(configPath)
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	// Check if TrueNAS is configured
	if cfg.Storage == nil || cfg.Storage.TrueNAS == nil || cfg.Storage.TrueNAS.APIURL == "" {
		return fmt.Errorf("TrueNAS not configured. Run 'foundry storage configure' first")
	}

	fmt.Println("Testing TrueNAS storage backend...")
	fmt.Printf("API URL: %s\n", cfg.Storage.TrueNAS.APIURL)
	fmt.Println()

	// Resolve API key
	apiKey, err := resolveAPIKey(cfg)
	if err != nil {
		return fmt.Errorf("failed to resolve API key: %w", err)
	}

	// Create client
	fmt.Println("1. Creating TrueNAS client...")
	client, err := truenas.NewClient(cfg.Storage.TrueNAS.APIURL, apiKey)
	if err != nil {
		return fmt.Errorf("✗ Failed to create client: %w", err)
	}
	fmt.Println("   ✓ Client created")

	// Test connection
	fmt.Println("\n2. Testing API connection...")
	if err := client.Ping(); err != nil {
		return fmt.Errorf("✗ Connection failed: %w", err)
	}
	fmt.Println("   ✓ Connection successful")

	// List pools
	fmt.Println("\n3. Listing storage pools...")
	pools, err := client.ListPools()
	if err != nil {
		return fmt.Errorf("✗ Failed to list pools: %w", err)
	}
	if len(pools) == 0 {
		fmt.Println("   ✗ No pools found")
		return fmt.Errorf("no storage pools available")
	}
	fmt.Printf("   ✓ Found %d pool(s)\n", len(pools))

	for _, pool := range pools {
		healthStatus := "healthy"
		if !pool.Healthy {
			healthStatus = "UNHEALTHY"
		}
		freeGB := float64(pool.Free) / (1024 * 1024 * 1024)
		fmt.Printf("     - %s: %s, %.2f GB free\n", pool.Name, healthStatus, freeGB)
	}

	// Full test if requested
	if cmd.Bool("full-test") {
		// Determine which pool to use
		poolName := cmd.String("pool")
		if poolName == "" {
			poolName = pools[0].Name
			fmt.Printf("\n4. Running full test on pool '%s'...\n", poolName)
		} else {
			fmt.Printf("\n4. Running full test on pool '%s'...\n", poolName)
			// Verify pool exists
			found := false
			for _, p := range pools {
				if p.Name == poolName {
					found = true
					break
				}
			}
			if !found {
				return fmt.Errorf("pool '%s' not found", poolName)
			}
		}

		// Create test dataset
		testDatasetName := fmt.Sprintf("%s/foundry-test-%d", poolName, getCurrentTimestamp())
		fmt.Printf("   Creating test dataset: %s\n", testDatasetName)

		dataset, err := client.CreateDataset(truenas.DatasetConfig{
			Name:     testDatasetName,
			Type:     "FILESYSTEM",
			Comments: "Foundry connectivity test - safe to delete",
		})
		if err != nil {
			return fmt.Errorf("✗ Failed to create test dataset: %w", err)
		}
		fmt.Printf("   ✓ Dataset created: %s\n", dataset.Name)

		// Verify dataset exists
		fmt.Println("   Verifying dataset...")
		_, err = client.GetDataset(testDatasetName)
		if err != nil {
			return fmt.Errorf("✗ Failed to verify dataset: %w", err)
		}
		fmt.Println("   ✓ Dataset verified")

		// Cleanup: Delete test dataset
		fmt.Println("   Cleaning up test dataset...")
		if err := client.DeleteDataset(testDatasetName); err != nil {
			fmt.Printf("   ✗ Warning: Failed to delete test dataset: %v\n", err)
			fmt.Printf("   You may need to manually delete: %s\n", testDatasetName)
		} else {
			fmt.Println("   ✓ Test dataset deleted")
		}
	} else {
		fmt.Println("\n4. Skipping dataset creation test")
		fmt.Println("   (use --full-test to test dataset creation/deletion)")
	}

	// List datasets (just count them)
	fmt.Println("\n5. Listing datasets...")
	datasets, err := client.ListDatasets()
	if err != nil {
		fmt.Printf("   ✗ Warning: Failed to list datasets: %v\n", err)
	} else {
		fmt.Printf("   ✓ Found %d dataset(s)\n", len(datasets))
	}

	fmt.Println("\n" + strings.Repeat("=", 50))
	fmt.Println("✓ All tests passed")
	fmt.Println(strings.Repeat("=", 50))

	return nil
}

// getCurrentTimestamp returns current unix timestamp
func getCurrentTimestamp() int64 {
	return time.Now().Unix()
}
