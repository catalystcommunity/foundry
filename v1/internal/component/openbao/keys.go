package openbao

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// KeyMaterial contains the OpenBAO initialization secrets
type KeyMaterial struct {
	RootToken  string   `json:"root_token"`
	UnsealKeys []string `json:"unseal_keys"`
	Shares     int      `json:"shares"`
	Threshold  int      `json:"threshold"`
}

// SaveKeyMaterial saves OpenBAO keys to the filesystem
// Keys are saved in ~/.foundry/openbao-keys/<cluster-name>/keys.json
func SaveKeyMaterial(keysDir string, clusterName string, material *KeyMaterial) error {
	// Create keys directory for this cluster
	clusterKeysDir := filepath.Join(keysDir, clusterName)
	if err := os.MkdirAll(clusterKeysDir, 0700); err != nil {
		return fmt.Errorf("failed to create keys directory: %w", err)
	}

	// Save as JSON
	keysPath := filepath.Join(clusterKeysDir, "keys.json")
	data, err := json.MarshalIndent(material, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal keys: %w", err)
	}

	// Write with restricted permissions (owner read/write only)
	if err := os.WriteFile(keysPath, data, 0600); err != nil {
		return fmt.Errorf("failed to write keys file: %w", err)
	}

	return nil
}

// LoadKeyMaterial loads OpenBAO keys from the filesystem
func LoadKeyMaterial(keysDir string, clusterName string) (*KeyMaterial, error) {
	keysPath := filepath.Join(keysDir, clusterName, "keys.json")

	data, err := os.ReadFile(keysPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read keys file: %w", err)
	}

	var material KeyMaterial
	if err := json.Unmarshal(data, &material); err != nil {
		return nil, fmt.Errorf("failed to unmarshal keys: %w", err)
	}

	return &material, nil
}

// KeyMaterialExists checks if key material exists for a cluster
func KeyMaterialExists(keysDir string, clusterName string) bool {
	keysPath := filepath.Join(keysDir, clusterName, "keys.json")
	_, err := os.Stat(keysPath)
	return err == nil
}
