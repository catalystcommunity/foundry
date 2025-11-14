package ssh

import (
	"fmt"
	"os"
	"path/filepath"

	"encoding/json"
)

// OpenBAOConfig represents the minimal config needed to connect to OpenBAO
type OpenBAOConfig struct {
	Address string
	Token   string
}

// GetOpenBAOClient creates an OpenBAO key storage client using config and saved keys
// It reads the OpenBAO address from network config and token from keys file
func GetOpenBAOClient(configDir, clusterName string) (*OpenBAOKeyStorage, error) {
	config, err := loadOpenBAOConfig(configDir, clusterName)
	if err != nil {
		return nil, fmt.Errorf("failed to load OpenBAO config: %w", err)
	}

	return NewOpenBAOKeyStorage(config.Address, config.Token)
}

// loadOpenBAOConfig reads OpenBAO address and token from Foundry config files
func loadOpenBAOConfig(configDir, clusterName string) (*OpenBAOConfig, error) {
	// Read token from OpenBAO keys file
	keysPath := filepath.Join(configDir, "openbao-keys", clusterName, "keys.json")
	keysData, err := os.ReadFile(keysPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read OpenBAO keys file: %w", err)
	}

	var keys struct {
		RootToken string `json:"root_token"`
	}
	if err := json.Unmarshal(keysData, &keys); err != nil {
		return nil, fmt.Errorf("failed to parse OpenBAO keys file: %w", err)
	}

	if keys.RootToken == "" {
		return nil, fmt.Errorf("root token not found in keys file")
	}

	// Read OpenBAO address from stack config
	stackPath := filepath.Join(configDir, "stack.yaml")
	stackData, err := os.ReadFile(stackPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read stack config: %w", err)
	}

	// Simple YAML parsing to extract openbao_hosts
	// This is basic but avoids adding yaml dependency here
	// Format expected: "    openbao_hosts:\n        - 10.16.0.42"
	address := ""
	lines := string(stackData)
	if idx := findYAMLValue(lines, "openbao_hosts"); idx != "" {
		address = "http://" + idx + ":8200"
	}

	if address == "" {
		return nil, fmt.Errorf("OpenBAO address not found in stack config")
	}

	return &OpenBAOConfig{
		Address: address,
		Token:   keys.RootToken,
	}, nil
}

// findYAMLValue is a simple helper to extract first list item under a key
// This is intentionally simple to avoid yaml library dependency
func findYAMLValue(content, key string) string {
	lines := splitLines(content)
	for i, line := range lines {
		if containsKey(line, key+":") {
			// Found the key, next line should have the value
			if i+1 < len(lines) {
				return extractListValue(lines[i+1])
			}
		}
	}
	return ""
}

func splitLines(s string) []string {
	result := []string{}
	current := ""
	for _, ch := range s {
		if ch == '\n' {
			result = append(result, current)
			current = ""
		} else {
			current += string(ch)
		}
	}
	if current != "" {
		result = append(result, current)
	}
	return result
}

func containsKey(line, key string) bool {
	for i := 0; i < len(line); i++ {
		if i+len(key) <= len(line) && line[i:i+len(key)] == key {
			return true
		}
	}
	return false
}

func extractListValue(line string) string {
	// Line format: "        - 10.16.0.42"
	trimmed := ""
	started := false
	for _, ch := range line {
		if ch == '-' {
			started = true
			continue
		}
		if started && ch != ' ' {
			trimmed += string(ch)
		} else if started && trimmed != "" {
			break
		}
	}
	return trimmed
}

// IsOpenBAOAvailable checks if OpenBAO is installed and initialized
func IsOpenBAOAvailable(configDir, clusterName string) bool {
	_, err := loadOpenBAOConfig(configDir, clusterName)
	return err == nil
}

// GetKeyStorage returns the appropriate key storage backend
// Uses hybrid storage when OpenBAO is available (auto-migrates keys)
// Falls back to filesystem-only when OpenBAO is not available
func GetKeyStorage(configDir, clusterName string) (KeyStorage, error) {
	// Try hybrid storage (OpenBAO + filesystem with auto-migration)
	if IsOpenBAOAvailable(configDir, clusterName) {
		openbao, err := GetOpenBAOClient(configDir, clusterName)
		if err == nil {
			// Use hybrid storage for automatic migration
			hybrid, err := NewHybridKeyStorage(openbao, configDir)
			if err == nil {
				return hybrid, nil
			}
			// If hybrid setup fails, fall through to filesystem
		}
		// If OpenBAO client fails, fall through to filesystem
	}

	// Fall back to filesystem-only storage
	keysDir := filepath.Join(configDir, "keys")
	return NewFilesystemKeyStorage(keysDir)
}
