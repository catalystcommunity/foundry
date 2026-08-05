package zot

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/catalystcommunity/foundry/v1/internal/container"
	"github.com/catalystcommunity/foundry/v1/internal/systemd"
)

// Install installs Zot registry as a containerized systemd service
func Install(conn container.SSHExecutor, runtime container.Runtime, cfg *ParsedConfig) error {
	if err := createDirectories(conn, cfg); err != nil {
		return fmt.Errorf("create directories: %w", err)
	}

	if err := writeConfigFile(conn, cfg); err != nil {
		return fmt.Errorf("write config file: %w", err)
	}

	imageName := fmt.Sprintf("ghcr.io/project-zot/zot:%s", cfg.Version)
	if err := runtime.Pull(imageName); err != nil {
		return fmt.Errorf("pull container image: %w", err)
	}

	if err := createSystemdService(conn, runtime, cfg); err != nil {
		return fmt.Errorf("create systemd service: %w", err)
	}

	if err := systemd.EnableService(conn, "foundry-zot"); err != nil {
		return fmt.Errorf("enable service: %w", err)
	}

	if err := systemd.StartService(conn, "foundry-zot"); err != nil {
		return fmt.Errorf("start service: %w", err)
	}

	status, err := systemd.GetServiceStatus(conn, "foundry-zot")
	if err != nil {
		return fmt.Errorf("get service status: %w", err)
	}

	if !status.Active || !status.Running {
		return fmt.Errorf("service failed to start: %s", status.SubState)
	}

	return nil
}

// createDirectories creates the necessary directories for Zot
func createDirectories(conn container.SSHExecutor, cfg *ParsedConfig) error {
	dirs := []string{
		cfg.DataDir,
		cfg.ConfigDir,
	}

	// If using external storage (e.g., NFS mount), add that path
	if cfg.StorageBackend != nil && cfg.StorageBackend.MountPath != "" {
		dirs = append(dirs, cfg.StorageBackend.MountPath)
	}

	for _, dir := range dirs {
		cmd := fmt.Sprintf("sudo mkdir -p %s && sudo chown -R $(id -u):$(id -g) %s", dir, dir)
		if _, err := conn.Execute(cmd); err != nil {
			return fmt.Errorf("create directory %s: %w", dir, err)
		}
	}

	return nil
}

// writeConfigFile generates and writes the Zot config.json file
func writeConfigFile(conn container.SSHExecutor, cfg *ParsedConfig) error {
	configContent, err := GenerateConfig(cfg.Config, cfg.UpstreamCreds)
	if err != nil {
		return fmt.Errorf("generate config: %w", err)
	}

	// A quoted heredoc preserves the JSON without shell expansion.
	configPath := filepath.Join(cfg.ConfigDir, "config.json")
	cmd := fmt.Sprintf("sudo tee %s > /dev/null <<'EOF'\n%s\nEOF", configPath, configContent)

	if _, err := conn.Execute(cmd); err != nil {
		return fmt.Errorf("write config file: %w", err)
	}

	return nil
}

// createSystemdService creates the systemd service unit for Zot
func createSystemdService(conn container.SSHExecutor, runtime container.Runtime, cfg *ParsedConfig) error {
	runtimePath, err := detectRuntimePath(conn, runtime.Name())
	if err != nil {
		return fmt.Errorf("detect runtime path: %w", err)
	}

	dataDir := cfg.DataDir
	if cfg.StorageBackend != nil && cfg.StorageBackend.MountPath != "" {
		dataDir = cfg.StorageBackend.MountPath
	}

	imageName := fmt.Sprintf("ghcr.io/project-zot/zot:%s", cfg.Version)
	configPath := filepath.Join(cfg.ConfigDir, "config.json")

	execStart := buildExecStart(runtimePath, imageName, int(cfg.Port), dataDir, configPath)

	unit := systemd.ContainerUnitFile(
		"foundry-zot",
		"Foundry Zot Registry",
		execStart,
	)
	// Cleanup makes service restarts idempotent.
	unit.ExecStartPre = fmt.Sprintf("-%s rm -f foundry-zot", runtimePath)
	// systemd sends SIGTERM through the runtime. An explicit stop can fail with
	// AppArmor on Ubuntu and Debian.
	unit.ExecStopPost = fmt.Sprintf("-%s rm -f foundry-zot", runtimePath)
	unit.TimeoutStopSec = 30

	if err := systemd.CreateService(conn, "foundry-zot", unit); err != nil {
		return fmt.Errorf("create systemd service: %w", err)
	}

	return nil
}

// detectRuntimePath finds the actual path to the container runtime executable
func detectRuntimePath(conn container.SSHExecutor, runtimeType string) (string, error) {
	cmd := fmt.Sprintf("which %s", runtimeType)
	output, err := conn.Execute(cmd)
	if err != nil {
		return "", fmt.Errorf("failed to find %s: %w", runtimeType, err)
	}
	return strings.TrimSpace(output), nil
}

// buildExecStart builds the ExecStart command for the systemd service
func buildExecStart(runtimePath, image string, port int, dataDir, configPath string) string {
	// Note: No --rm flag - systemd manages the container lifecycle via ExecStartPre/ExecStopPost
	// --security-opt apparmor=unconfined: nerdctl-default profile blocks runc signal operations
	return fmt.Sprintf("%s run --name foundry-zot --security-opt apparmor=unconfined -p %d:%d -v %s:/var/lib/zot -v %s:/etc/zot/config.json %s",
		runtimePath, port, port, dataDir, configPath, image)
}
