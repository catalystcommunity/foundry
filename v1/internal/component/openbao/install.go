package openbao

import (
	"context"
	"fmt"
	"strings"

	"github.com/catalystcommunity/foundry/v1/internal/component"
	"github.com/catalystcommunity/foundry/v1/internal/container"
	"github.com/catalystcommunity/foundry/v1/internal/systemd"
)

// Install installs OpenBAO as a containerized systemd service
func (c *Component) Install(ctx context.Context, cfg component.ComponentConfig) error {
	// Parse OpenBAO-specific config
	openbaoCfg, err := parseConfig(cfg)
	if err != nil {
		return fmt.Errorf("failed to parse config: %w", err)
	}

	// Validate config
	if err := openbaoCfg.Validate(); err != nil {
		return fmt.Errorf("invalid config: %w", err)
	}

	// Create container runtime client
	var runtime container.Runtime
	if openbaoCfg.ContainerRuntime == "podman" {
		runtime = container.NewPodmanRuntime(c.conn)
	} else {
		runtime = container.NewDockerRuntime(c.conn)
	}

	// Verify runtime is available
	if !runtime.IsAvailable() {
		return fmt.Errorf("%s runtime is not available on the host", runtime.Name())
	}

	// Create data and config directories
	if err := c.createDirectories(openbaoCfg); err != nil {
		return fmt.Errorf("failed to create directories: %w", err)
	}

	// Generate and write OpenBAO config file
	if err := c.writeConfigFile(openbaoCfg); err != nil {
		return fmt.Errorf("failed to write config file: %w", err)
	}

	// Pull OpenBAO container image
	image := fmt.Sprintf("quay.io/openbao/openbao:%s", openbaoCfg.Version)
	if err := runtime.Pull(image); err != nil {
		return fmt.Errorf("failed to pull image: %w", err)
	}

	// Create systemd service
	if err := c.createSystemdService(openbaoCfg, runtime.Name()); err != nil {
		return fmt.Errorf("failed to create systemd service: %w", err)
	}

	// Enable and start service
	if err := systemd.EnableService(c.conn, "openbao"); err != nil {
		return fmt.Errorf("failed to enable service: %w", err)
	}

	if err := systemd.StartService(c.conn, "openbao"); err != nil {
		return fmt.Errorf("failed to start service: %w", err)
	}

	// Wait for service to be ready
	if err := c.waitForReady(ctx); err != nil {
		return fmt.Errorf("service failed to become ready: %w", err)
	}

	return nil
}

// parseConfig converts generic component.ComponentConfig to OpenBAO-specific Config
func parseConfig(cfg component.ComponentConfig) (*Config, error) {
	openbaoCfg := DefaultConfig()

	// Override defaults with provided config
	if version, ok := cfg["version"].(string); ok {
		openbaoCfg.Version = version
	}
	if dataPath, ok := cfg["data_path"].(string); ok {
		openbaoCfg.DataPath = dataPath
	}
	if configPath, ok := cfg["config_path"].(string); ok {
		openbaoCfg.ConfigPath = configPath
	}
	if address, ok := cfg["address"].(string); ok {
		openbaoCfg.Address = address
	}
	if runtime, ok := cfg["container_runtime"].(string); ok {
		openbaoCfg.ContainerRuntime = runtime
	}

	return openbaoCfg, nil
}

// createDirectories creates the necessary directories on the remote host
func (c *Component) createDirectories(cfg *Config) error {
	dirs := []string{cfg.DataPath, cfg.ConfigPath}
	for _, dir := range dirs {
		cmd := fmt.Sprintf("mkdir -p %s && chmod 755 %s", dir, dir)
		if _, err := c.conn.Execute(cmd); err != nil {
			return fmt.Errorf("failed to create directory %s: %w", dir, err)
		}
	}
	return nil
}

// writeConfigFile writes the OpenBAO configuration to the remote host
func (c *Component) writeConfigFile(cfg *Config) error {
	configContent, err := GenerateConfig(cfg)
	if err != nil {
		return fmt.Errorf("failed to generate config: %w", err)
	}

	configPath := fmt.Sprintf("%s/config.hcl", cfg.ConfigPath)
	cmd := fmt.Sprintf("cat > %s << 'EOF'\n%s\nEOF", configPath, configContent)

	if _, err := c.conn.Execute(cmd); err != nil {
		return fmt.Errorf("failed to write config file: %w", err)
	}

	return nil
}

// createSystemdService creates and installs the systemd service unit
func (c *Component) createSystemdService(cfg *Config, runtimeType string) error {
	unit := systemd.ContainerUnitFile(
		"openbao",
		"OpenBAO Secret Management",
		c.buildExecStart(cfg, runtimeType),
	)
	unit.ExecStop = fmt.Sprintf("/usr/bin/%s stop openbao", runtimeType)

	if err := systemd.CreateService(c.conn, "openbao", unit); err != nil {
		return fmt.Errorf("failed to create service: %w", err)
	}

	return nil
}

// buildExecStart builds the ExecStart command for the systemd service
func (c *Component) buildExecStart(cfg *Config, runtimeType string) string {
	image := fmt.Sprintf("quay.io/openbao/openbao:%s", cfg.Version)

	parts := []string{
		fmt.Sprintf("/usr/bin/%s run", runtimeType),
		"--rm",
		"--name openbao",
		fmt.Sprintf("-p %s:8200", strings.Split(cfg.Address, ":")[1]),
		fmt.Sprintf("-v %s:/vault/data", cfg.DataPath),
		fmt.Sprintf("-v %s:/vault/config", cfg.ConfigPath),
		"--cap-add=IPC_LOCK",
		image,
		"server",
		"-config=/vault/config/config.hcl",
	}

	return strings.Join(parts, " ")
}

// waitForReady waits for OpenBAO to be ready to accept connections
func (c *Component) waitForReady(ctx context.Context) error {
	// Check if service is running
	status, err := systemd.GetServiceStatus(c.conn, "openbao")
	if err != nil {
		return fmt.Errorf("failed to get service status: %w", err)
	}

	if !status.Active {
		return fmt.Errorf("service is not active: %s", status.ActiveState)
	}

	// TODO: Add health check via API (requires client implementation)
	// For now, just verify the service is active

	return nil
}
