package config

import (
	"fmt"
	"os"

	"github.com/catalystcommunity/foundry/v1/internal/host"
	"gopkg.in/yaml.v3"
)

// HostConfigLoader implements host.ConfigLoader for loading/saving hosts to config
type HostConfigLoader struct {
	configPath string
}

// NewHostConfigLoader creates a new host config loader
func NewHostConfigLoader(configPath string) *HostConfigLoader {
	return &HostConfigLoader{
		configPath: configPath,
	}
}

// LoadHosts loads hosts from the config file
func (l *HostConfigLoader) LoadHosts() ([]*host.Host, error) {
	// Read config file
	data, err := os.ReadFile(l.configPath)
	if err != nil {
		if os.IsNotExist(err) {
			// File doesn't exist, return empty list
			return []*host.Host{}, nil
		}
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("failed to parse config file: %w", err)
	}

	// Return hosts (may be nil/empty)
	if cfg.Hosts == nil {
		return []*host.Host{}, nil
	}

	return cfg.Hosts, nil
}

// SaveHosts saves hosts to the config file
func (l *HostConfigLoader) SaveHosts(hosts []*host.Host) error {
	// Read existing config
	data, err := os.ReadFile(l.configPath)
	if err != nil {
		return fmt.Errorf("failed to read config file: %w", err)
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return fmt.Errorf("failed to parse config file: %w", err)
	}

	// Update hosts
	cfg.Hosts = hosts

	// Write back to file
	output, err := yaml.Marshal(&cfg)
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	if err := os.WriteFile(l.configPath, output, 0644); err != nil {
		return fmt.Errorf("failed to write config file: %w", err)
	}

	return nil
}
