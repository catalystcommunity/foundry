package openbao

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/catalystcommunity/foundry/v1/internal/component/openbao"
	"github.com/catalystcommunity/foundry/v1/internal/config"
	"github.com/urfave/cli/v3"
)

// UnsealCommand unseals OpenBAO using stored keys
var UnsealCommand = &cli.Command{
	Name:  "unseal",
	Usage: "Unseal OpenBAO using stored keys",
	Description: `Unseals OpenBAO after a restart using the keys stored during initialization.

Keys are stored in ~/.foundry/openbao-keys/<cluster-name>/keys.json

Example:
  foundry openbao unseal`,
	Action: runUnseal,
}

func runUnseal(ctx context.Context, cmd *cli.Command) error {
	// Load stack configuration
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("failed to get home directory: %w", err)
	}

	configPath, err := config.FindConfig(cmd.String("config"))
	if err != nil {
		return fmt.Errorf("failed to find config: %w", err)
	}
	stackConfig, err := config.Load(configPath)
	if err != nil {
		return fmt.Errorf("failed to load stack config: %w", err)
	}

	// Get OpenBAO API URL from config
	apiURL, err := stackConfig.GetPrimaryOpenBAOAddress()
	if err != nil {
		return fmt.Errorf("failed to get OpenBAO address: %w", err)
	}
	apiURL = fmt.Sprintf("http://%s:8200", apiURL)

	// Get cluster name for keys
	clusterName := stackConfig.Cluster.Name
	if clusterName == "" {
		clusterName = "default"
	}

	// Get keys directory
	keysDir := filepath.Join(homeDir, ".foundry", "openbao-keys")

	// Check if keys exist
	if !openbao.KeyMaterialExists(keysDir, clusterName) {
		return fmt.Errorf("no keys found for cluster %s - OpenBAO may not have been initialized", clusterName)
	}

	// Load keys
	material, err := openbao.LoadKeyMaterial(keysDir, clusterName)
	if err != nil {
		return fmt.Errorf("failed to load keys: %w", err)
	}

	// Create client
	client := openbao.NewClient(apiURL, "")

	// Check if already unsealed
	sealed, err := client.VerifySealed(ctx)
	if err != nil {
		return fmt.Errorf("failed to check seal status: %w", err)
	}

	if !sealed {
		fmt.Println("✓ OpenBAO is already unsealed")
		return nil
	}

	// Unseal
	fmt.Printf("Unsealing OpenBAO at %s...\n", apiURL)
	if err := client.UnsealWithKeys(ctx, material.UnsealKeys); err != nil {
		return fmt.Errorf("failed to unseal: %w", err)
	}

	fmt.Println("✓ OpenBAO unsealed successfully")
	return nil
}
