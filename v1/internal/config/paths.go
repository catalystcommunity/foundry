package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const (
	// DefaultConfigDir is the default directory name for foundry configs
	DefaultConfigDir = ".foundry"
	// DefaultConfigName is the default config file name
	DefaultConfigName = "stack.yaml"
)

// GetConfigDir returns the foundry configuration directory path
// Defaults to ~/.foundry/ unless overridden by environment
func GetConfigDir() (string, error) {
	// Check if there's an override via environment variable
	if dir := os.Getenv("FOUNDRY_CONFIG_DIR"); dir != "" {
		return dir, nil
	}

	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("failed to get user home directory: %w", err)
	}

	return filepath.Join(homeDir, DefaultConfigDir), nil
}

// FindConfig finds a configuration file by name
// If name is an absolute path, returns it as-is
// If name is a filename, looks in the config directory
// If name is empty, looks for the default config
func FindConfig(name string) (string, error) {
	// If it's an absolute path, use it directly
	if filepath.IsAbs(name) {
		if _, err := os.Stat(name); err != nil {
			if os.IsNotExist(err) {
				return "", fmt.Errorf("config file not found: %s", name)
			}
			return "", fmt.Errorf("failed to stat config file %s: %w", name, err)
		}
		return name, nil
	}

	configDir, err := GetConfigDir()
	if err != nil {
		return "", err
	}

	// If no name provided, use default
	if name == "" {
		name = DefaultConfigName
	}

	// Add .yaml extension if not present
	if !strings.HasSuffix(name, ".yaml") && !strings.HasSuffix(name, ".yml") {
		name += ".yaml"
	}

	configPath := filepath.Join(configDir, name)

	// Check if file exists
	if _, err := os.Stat(configPath); err != nil {
		if os.IsNotExist(err) {
			return "", fmt.Errorf("config file not found: %s", configPath)
		}
		return "", fmt.Errorf("failed to stat config file %s: %w", configPath, err)
	}

	return configPath, nil
}

// ListConfigs returns a list of all available configuration files
func ListConfigs() ([]string, error) {
	configDir, err := GetConfigDir()
	if err != nil {
		return nil, err
	}

	// Check if config directory exists
	if _, err := os.Stat(configDir); err != nil {
		if os.IsNotExist(err) {
			// Config directory doesn't exist yet, return empty list
			return []string{}, nil
		}
		return nil, fmt.Errorf("failed to access config directory %s: %w", configDir, err)
	}

	// Read directory contents
	entries, err := os.ReadDir(configDir)
	if err != nil {
		return nil, fmt.Errorf("failed to read config directory %s: %w", configDir, err)
	}

	var configs []string
	for _, entry := range entries {
		// Skip directories and non-YAML files
		if entry.IsDir() {
			continue
		}

		name := entry.Name()
		if strings.HasSuffix(name, ".yaml") || strings.HasSuffix(name, ".yml") {
			configs = append(configs, filepath.Join(configDir, name))
		}
	}

	return configs, nil
}

// EnsureConfigDir creates the config directory if it doesn't exist
func EnsureConfigDir() (string, error) {
	configDir, err := GetConfigDir()
	if err != nil {
		return "", err
	}

	if err := os.MkdirAll(configDir, 0755); err != nil {
		return "", fmt.Errorf("failed to create config directory %s: %w", configDir, err)
	}

	return configDir, nil
}
