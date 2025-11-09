package openbao

import (
	"context"
	"fmt"

	"github.com/catalystcommunity/foundry/v1/internal/component"
	"github.com/catalystcommunity/foundry/v1/internal/container"
)

// Component implements the component.Component interface for OpenBAO
type Component struct {
	conn container.SSHExecutor
}

// NewComponent creates a new OpenBAO component instance
func NewComponent(conn container.SSHExecutor) *Component {
	return &Component{
		conn: conn,
	}
}

// Dependencies returns the list of component dependencies
func (c *Component) Dependencies() []string {
	return []string{} // OpenBAO has no dependencies
}

// Name returns the component name
func (c *Component) Name() string {
	return "openbao"
}

// Upgrade upgrades the OpenBAO component
func (c *Component) Upgrade(ctx context.Context, cfg component.ComponentConfig) error {
	return fmt.Errorf("upgrade not yet implemented")
}

// Status returns the status of the OpenBAO component
func (c *Component) Status(ctx context.Context) (*component.ComponentStatus, error) {
	// For now, return a basic status
	// TODO: Implement actual status checking via systemd service status
	return &component.ComponentStatus{
		Installed: false,
		Version:   "",
		Healthy:   false,
		Message:   "status check not implemented",
	}, nil
}

// Uninstall removes the OpenBAO component
func (c *Component) Uninstall(ctx context.Context) error {
	return fmt.Errorf("uninstall not yet implemented")
}

// Config type is generated from CSIL in types.gen.go
// This file extends the generated type with methods

// DefaultConfig returns a Config with sensible defaults
func DefaultConfig() *Config {
	return &Config{
		Version:          "2.0.0",
		DataPath:         "/var/lib/openbao",
		ConfigPath:       "/etc/openbao",
		Address:          "0.0.0.0:8200",
		ContainerRuntime: "docker",
	}
}

// Validate checks if the configuration is valid
func (c *Config) Validate() error {
	if c.Version == "" {
		return fmt.Errorf("version is required")
	}
	if c.DataPath == "" {
		return fmt.Errorf("data_path is required")
	}
	if c.ConfigPath == "" {
		return fmt.Errorf("config_path is required")
	}
	if c.Address == "" {
		return fmt.Errorf("address is required")
	}
	if c.ContainerRuntime != "docker" && c.ContainerRuntime != "podman" {
		return fmt.Errorf("container_runtime must be 'docker' or 'podman', got: %s", c.ContainerRuntime)
	}
	return nil
}
