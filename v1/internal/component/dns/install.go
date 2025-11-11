package dns

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"path/filepath"

	"github.com/catalystcommunity/foundry/v1/internal/component"
	"github.com/catalystcommunity/foundry/v1/internal/container"
	"github.com/catalystcommunity/foundry/v1/internal/ssh"
	"github.com/catalystcommunity/foundry/v1/internal/systemd"
)

// sshExecutorAdapter adapts ssh.Connection to container.SSHExecutor interface
type sshExecutorAdapter struct {
	conn *ssh.Connection
}

func (a *sshExecutorAdapter) Execute(cmd string) (string, error) {
	result, err := a.conn.Exec(cmd)
	if err != nil {
		return "", err
	}
	if result.ExitCode != 0 {
		return result.Stdout, fmt.Errorf("command failed with exit code %d: %s", result.ExitCode, result.Stderr)
	}
	return result.Stdout, nil
}

const (
	defaultImageRegistry = "docker.io/powerdns"
	defaultAuthImage     = "pdns-auth-49" // PowerDNS 4.9.x
	defaultRecursorImage = "pdns-recursor-49"
)

// Install implements the component.Component interface.
func (c *Component) Install(ctx context.Context, cfg component.ComponentConfig) error {
	// Extract DNS-specific config from ComponentConfig
	dnsConfig, host, err := configFromComponentConfig(cfg)
	if err != nil {
		return fmt.Errorf("invalid DNS config: %w", err)
	}

	// Validate required fields
	if host == nil {
		return fmt.Errorf("host connection is required")
	}
	if dnsConfig.APIKey == "" {
		return fmt.Errorf("API key is required")
	}

	// Generate API key if not provided
	if dnsConfig.APIKey == "" {
		apiKey, err := generateAPIKey()
		if err != nil {
			return fmt.Errorf("failed to generate API key: %w", err)
		}
		dnsConfig.APIKey = apiKey
	}

	// Create container runtime with adapter
	adapter := &sshExecutorAdapter{conn: host}
	runtime := container.NewDockerRuntime(adapter)

	// Create data and config directories
	if err := createDirectories(host, dnsConfig); err != nil {
		return fmt.Errorf("failed to create directories: %w", err)
	}

	// Generate and write config files
	if err := writeConfigFiles(host, dnsConfig); err != nil {
		return fmt.Errorf("failed to write config files: %w", err)
	}

	// Pull container images
	authImage := fmt.Sprintf("%s/%s:%s", defaultImageRegistry, "pdns-auth", dnsConfig.ImageTag)
	recursorImage := fmt.Sprintf("%s/%s:%s", defaultImageRegistry, "pdns-recursor", dnsConfig.ImageTag)

	if err := runtime.Pull(authImage); err != nil {
		return fmt.Errorf("failed to pull auth image: %w", err)
	}
	if err := runtime.Pull(recursorImage); err != nil {
		return fmt.Errorf("failed to pull recursor image: %w", err)
	}

	// Create systemd services
	if err := createSystemdServices(host, dnsConfig, authImage, recursorImage); err != nil {
		return fmt.Errorf("failed to create systemd services: %w", err)
	}

	// Enable and start services
	if err := enableAndStartServices(host); err != nil {
		return fmt.Errorf("failed to start services: %w", err)
	}

	return nil
}

// configFromComponentConfig extracts DNS-specific config from generic ComponentConfig.
func configFromComponentConfig(cfg component.ComponentConfig) (*Config, *ssh.Connection, error) {
	dnsConfig := DefaultConfig()

	// Extract Host (runtime-only, not part of persisted config)
	var host *ssh.Connection
	if h, ok := cfg["host"].(*ssh.Connection); ok {
		host = h
	}

	// Extract ImageTag
	if tag, ok := cfg["image_tag"].(string); ok && tag != "" {
		dnsConfig.ImageTag = tag
	}

	// Extract APIKey
	if key, ok := cfg["api_key"].(string); ok {
		dnsConfig.APIKey = key
	}

	// Extract Forwarders
	if fwds, ok := cfg["forwarders"].([]string); ok && len(fwds) > 0 {
		dnsConfig.Forwarders = fwds
	}

	// Extract Backend
	if backend, ok := cfg["backend"].(string); ok && backend != "" {
		dnsConfig.Backend = backend
	}

	// Extract DataDir
	if dataDir, ok := cfg["data_dir"].(string); ok && dataDir != "" {
		dnsConfig.DataDir = dataDir
	}

	// Extract ConfigDir
	if configDir, ok := cfg["config_dir"].(string); ok && configDir != "" {
		dnsConfig.ConfigDir = configDir
	}

	return dnsConfig, host, nil
}

// generateAPIKey generates a secure random API key.
func generateAPIKey() (string, error) {
	bytes := make([]byte, 32) // 256 bits
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return hex.EncodeToString(bytes), nil
}

// createDirectories creates the data and config directories on the remote host.
func createDirectories(conn *ssh.Connection, cfg *Config) error {
	dirs := []string{
		cfg.DataDir,
		cfg.ConfigDir,
		filepath.Join(cfg.ConfigDir, "auth"),
		filepath.Join(cfg.ConfigDir, "recursor"),
	}

	adapter := &sshExecutorAdapter{conn: conn}
	for _, dir := range dirs {
		cmd := fmt.Sprintf("sudo mkdir -p %s && sudo chmod 755 %s", dir, dir)
		if _, err := adapter.Execute(cmd); err != nil {
			return fmt.Errorf("failed to create directory %s: %w", dir, err)
		}
	}

	return nil
}

// writeConfigFiles writes the PowerDNS config files to the remote host.
func writeConfigFiles(conn *ssh.Connection, cfg *Config) error {
	// Generate auth config
	authConfig, err := GenerateAuthConfig(cfg)
	if err != nil {
		return fmt.Errorf("failed to generate auth config: %w", err)
	}

	// Generate recursor config
	recursorConfig, err := GenerateRecursorConfig(cfg)
	if err != nil {
		return fmt.Errorf("failed to generate recursor config: %w", err)
	}

	// Write auth config
	authConfigPath := filepath.Join(cfg.ConfigDir, "auth", "pdns.conf")
	if err := writeRemoteFile(conn, authConfigPath, authConfig); err != nil {
		return fmt.Errorf("failed to write auth config: %w", err)
	}

	// Write recursor config
	recursorConfigPath := filepath.Join(cfg.ConfigDir, "recursor", "recursor.conf")
	if err := writeRemoteFile(conn, recursorConfigPath, recursorConfig); err != nil {
		return fmt.Errorf("failed to write recursor config: %w", err)
	}

	return nil
}

// writeRemoteFile writes content to a file on the remote host.
func writeRemoteFile(conn *ssh.Connection, path, content string) error {
	adapter := &sshExecutorAdapter{conn: conn}

	// Use a heredoc to write the file
	cmd := fmt.Sprintf("sudo tee %s > /dev/null <<'FOUNDRY_EOF'\n%s\nFOUNDRY_EOF", path, content)
	if _, err := adapter.Execute(cmd); err != nil {
		return fmt.Errorf("failed to write file %s: %w", path, err)
	}

	// Set permissions
	chmodCmd := fmt.Sprintf("sudo chmod 644 %s", path)
	if _, err := adapter.Execute(chmodCmd); err != nil {
		return fmt.Errorf("failed to set permissions on %s: %w", path, err)
	}

	return nil
}

// createSystemdServices creates systemd service files for PowerDNS.
func createSystemdServices(conn *ssh.Connection, cfg *Config, authImage, recursorImage string) error {
	adapter := &sshExecutorAdapter{conn: conn}

	// Create auth service
	authUnit := systemd.UnitFile{
		Description: "PowerDNS Authoritative Server (Foundry)",
		After:       []string{"network.target"},
		Wants:       []string{"network.target"},
		Type:        "simple",
		Restart:     "always",
		ExecStart:   buildAuthExecStart(authImage, cfg),
		ExecStop:    buildExecStop("powerdns-auth"),
		WantedBy:    []string{"multi-user.target"},
	}

	if err := systemd.CreateService(adapter, "powerdns-auth", &authUnit); err != nil {
		return fmt.Errorf("failed to create auth service: %w", err)
	}

	// Create recursor service
	recursorUnit := systemd.UnitFile{
		Description: "PowerDNS Recursor (Foundry)",
		After:       []string{"network.target"},
		Wants:       []string{"network.target"},
		Type:        "simple",
		Restart:     "always",
		ExecStart:   buildRecursorExecStart(recursorImage, cfg),
		ExecStop:    buildExecStop("powerdns-recursor"),
		WantedBy:    []string{"multi-user.target"},
	}

	if err := systemd.CreateService(adapter, "powerdns-recursor", &recursorUnit); err != nil {
		return fmt.Errorf("failed to create recursor service: %w", err)
	}

	return nil
}

// buildAuthExecStart builds the ExecStart command for the auth service.
func buildAuthExecStart(image string, cfg *Config) string {
	return fmt.Sprintf(
		"docker run --rm --name powerdns-auth "+
			"-p 8081:8081 "+
			"-v %s/auth:/etc/powerdns "+
			"-v %s:/var/lib/powerdns "+
			"%s "+
			"--config-dir=/etc/powerdns",
		cfg.ConfigDir,
		cfg.DataDir,
		image,
	)
}

// buildRecursorExecStart builds the ExecStart command for the recursor service.
func buildRecursorExecStart(image string, cfg *Config) string {
	return fmt.Sprintf(
		"docker run --rm --name powerdns-recursor "+
			"-p 53:53/udp "+
			"-p 53:53/tcp "+
			"-p 8082:8082 "+
			"-v %s/recursor:/etc/powerdns-recursor "+
			"%s "+
			"--config-dir=/etc/powerdns-recursor",
		cfg.ConfigDir,
		image,
	)
}

// buildExecStop builds the ExecStop command for a service.
func buildExecStop(containerName string) string {
	return fmt.Sprintf("docker stop %s", containerName)
}

// enableAndStartServices enables and starts the PowerDNS systemd services.
func enableAndStartServices(conn *ssh.Connection) error {
	adapter := &sshExecutorAdapter{conn: conn}
	services := []string{"powerdns-auth", "powerdns-recursor"}

	for _, svc := range services {
		// Enable service
		if err := systemd.EnableService(adapter, svc); err != nil {
			return fmt.Errorf("failed to enable %s: %w", svc, err)
		}

		// Start service
		if err := systemd.StartService(adapter, svc); err != nil {
			return fmt.Errorf("failed to start %s: %w", svc, err)
		}
	}

	return nil
}
