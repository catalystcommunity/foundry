package host

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/catalystcommunity/foundry/v1/internal/config"
	"github.com/catalystcommunity/foundry/v1/internal/host"
	"github.com/catalystcommunity/foundry/v1/internal/ssh"
	"github.com/urfave/cli/v3"
)

// MigrateKeysCommand migrates SSH keys from filesystem to OpenBAO
var MigrateKeysCommand = &cli.Command{
	Name:  "migrate-keys",
	Usage: "Migrate SSH keys from filesystem storage to OpenBAO",
	Description: `Migrates SSH keys from local filesystem storage (~/.foundry/keys/) to OpenBAO.

This command:
- Reads keys from filesystem storage
- Stores them in OpenBAO at foundry-core/ssh-keys/<hostname>
- Optionally removes filesystem keys after successful migration
- Updates host registry to indicate keys are in OpenBAO

OpenBAO must be installed and initialized before running this command.`,
	Flags: []cli.Flag{
		&cli.BoolFlag{
			Name:  "keep-local",
			Usage: "Keep local filesystem keys after migration (default: remove)",
		},
		&cli.StringFlag{
			Name:  "hostname",
			Usage: "Migrate keys for specific hostname only (default: all hosts)",
		},
	},
	Action: runMigrateKeys,
}

func runMigrateKeys(ctx context.Context, cmd *cli.Command) error {
	// Find config file
	configPath, err := config.FindConfig("")
	if err != nil {
		return fmt.Errorf("failed to find config file: %w (run 'foundry config init' first)", err)
	}

	// Load stack config to get cluster name
	cfg, err := config.Load(configPath)
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	clusterName := cfg.Cluster.Name
	if clusterName == "" {
		return fmt.Errorf("cluster name not set in config")
	}

	// Get config directory
	configDir, err := getConfigDir()
	if err != nil {
		return fmt.Errorf("failed to get config directory: %w", err)
	}

	// Check if OpenBAO is available
	if !ssh.IsOpenBAOAvailable(configDir, clusterName) {
		return fmt.Errorf("OpenBAO is not available - ensure it is installed and initialized first")
	}

	// Create OpenBAO client
	openBAOStorage, err := ssh.GetOpenBAOClient(configDir, clusterName)
	if err != nil {
		return fmt.Errorf("failed to create OpenBAO client: %w", err)
	}

	// Create filesystem storage
	keysDir := filepath.Join(configDir, "keys")
	fsStorage, err := ssh.NewFilesystemKeyStorage(keysDir)
	if err != nil {
		return fmt.Errorf("failed to create filesystem storage: %w", err)
	}

	// Get list of hosts to migrate
	specificHost := cmd.String("hostname")
	var hostsToMigrate []string

	if specificHost != "" {
		// Migrate specific host
		hostsToMigrate = []string{specificHost}
	} else {
		// Migrate all hosts
		loader := config.NewHostConfigLoader(configPath)
		registry := host.NewConfigRegistry(configPath, loader)
		hosts, err := registry.List()
		if err != nil {
			return fmt.Errorf("failed to list hosts: %w", err)
		}
		for _, h := range hosts {
			hostsToMigrate = append(hostsToMigrate, h.Hostname)
		}
	}

	if len(hostsToMigrate) == 0 {
		fmt.Println("No hosts found to migrate")
		return nil
	}

	fmt.Printf("Migrating SSH keys to OpenBAO for %d host(s)...\n\n", len(hostsToMigrate))

	keepLocal := cmd.Bool("keep-local")
	successCount := 0
	failCount := 0

	for _, hostname := range hostsToMigrate {
		fmt.Printf("[%s] Migrating keys...\n", hostname)

		// Check if keys exist in filesystem
		exists, err := fsStorage.Exists(hostname)
		if err != nil {
			fmt.Printf("  ✗ Failed to check filesystem: %v\n", err)
			failCount++
			continue
		}
		if !exists {
			fmt.Printf("  ⚠ No keys found in filesystem (may already be migrated)\n")
			continue
		}

		// Load keys from filesystem
		keyPair, err := fsStorage.Load(hostname)
		if err != nil {
			fmt.Printf("  ✗ Failed to load keys: %v\n", err)
			failCount++
			continue
		}

		// Store in OpenBAO
		if err := openBAOStorage.Store(hostname, keyPair)  ; err != nil {
			fmt.Printf("  ✗ Failed to store in OpenBAO: %v\n", err)
			failCount++
			continue
		}
		fmt.Printf("  ✓ Keys stored in OpenBAO\n")

		// Verify keys can be loaded from OpenBAO
		_, err = openBAOStorage.Load(hostname)
		if err != nil {
			fmt.Printf("  ✗ Failed to verify keys in OpenBAO: %v\n", err)
			failCount++
			continue
		}
		fmt.Printf("  ✓ Keys verified in OpenBAO\n")

		// Remove filesystem keys if requested
		if !keepLocal {
			if err := fsStorage.Delete(hostname); err != nil {
				fmt.Printf("  ⚠ Failed to remove filesystem keys: %v\n", err)
				// Don't increment fail count - migration succeeded
			} else {
				fmt.Printf("  ✓ Filesystem keys removed\n")
			}
		} else {
			fmt.Printf("  ℹ Filesystem keys kept (--keep-local)\n")
		}

		successCount++
		fmt.Println()
	}

	fmt.Printf("Migration complete: %d succeeded, %d failed\n", successCount, failCount)

	if successCount > 0 {
		fmt.Println("\n✓ SSH keys are now stored in OpenBAO")
		fmt.Println("  Future SSH operations will use keys from OpenBAO")
	}

	if failCount > 0 {
		return fmt.Errorf("some migrations failed")
	}

	return nil
}

func getConfigDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".foundry"), nil
}
