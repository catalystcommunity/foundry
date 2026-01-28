package container

import (
	"fmt"
	"strings"

	"github.com/catalystcommunity/foundry/v1/internal/systemd"
)

// CommandExecutor is an interface for executing remote commands.
// This avoids importing the ssh package which would create an import cycle.
type CommandExecutor interface {
	Exec(cmd string) (*ExecResult, error)
}

// ExecResult represents the result of a command execution
type ExecResult struct {
	Stdout   string
	Stderr   string
	ExitCode int
}

// RuntimeType represents the type of container runtime installed
type RuntimeType int

const (
	RuntimeNone RuntimeType = iota
	RuntimeDocker
	RuntimeNerdctl
	RuntimeNerdctlIncomplete
)

// CNI configuration paths
const (
	CNIConfigPath = "/etc/cni/net.d/10-containerd-net.conflist"
	CNIConfigDir  = "/etc/cni/net.d"
)

// BridgeNetworkingServices lists services that use bridge networking with port mapping
// These services need to be restarted when CNI configuration is added
var BridgeNetworkingServices = []string{"openbao", "foundry-zot"}

// CNIConfigContent is the standard CNI configuration for containerd/nerdctl
// Uses bridge networking with portmap, firewall, and tuning plugins
const CNIConfigContent = `{
  "cniVersion": "1.0.0",
  "name": "containerd-net",
  "plugins": [
    {
      "type": "bridge",
      "bridge": "cni0",
      "isGateway": true,
      "ipMasq": true,
      "promiscMode": true,
      "ipam": {
        "type": "host-local",
        "ranges": [[{"subnet": "10.88.0.0/16"}]],
        "routes": [{"dst": "0.0.0.0/0"}]
      }
    },
    {"type": "portmap", "capabilities": {"portMappings": true}},
    {"type": "firewall"},
    {"type": "tuning"}
  ]
}`

// InstallRuntime installs a container runtime if not already present.
// Strategy:
//   - If real Docker is installed, use it (do nothing)
//   - If nerdctl is partially installed, complete the installation
//   - If nothing is installed, install containerd + nerdctl + CNI
func InstallRuntime(executor CommandExecutor, user string) error {
	runtimeType := DetectRuntimeInstallation(executor)

	switch runtimeType {
	case RuntimeDocker:
		// Real Docker is installed, nothing to do
		return nil
	case RuntimeNerdctl:
		// nerdctl is fully installed, nothing to do
		return nil
	case RuntimeNerdctlIncomplete:
		// nerdctl exists but CNI is missing, complete the installation
		return installCNIPlugins(executor)
	case RuntimeNone:
		// Nothing installed, do full installation
		return installContainerdAndNerdctl(executor, user)
	default:
		return fmt.Errorf("unknown runtime type")
	}
}

// DetectRuntimeInstallation determines what container runtime is installed
func DetectRuntimeInstallation(executor CommandExecutor) RuntimeType {
	// Check if docker command exists
	result, err := executor.Exec("which docker")
	if err != nil || result.ExitCode != 0 {
		return RuntimeNone
	}

	// Check what 'docker' actually is by looking at version output
	result, err = executor.Exec("docker version 2>&1")
	if err != nil || result.ExitCode != 0 {
		// Command exists but doesn't work
		return RuntimeNone
	}

	output := strings.ToLower(result.Stdout + result.Stderr)

	// Check if it's real Docker
	if strings.Contains(output, "docker engine") || strings.Contains(output, "docker.com") {
		return RuntimeDocker
	}

	// Check if it's nerdctl by multiple methods:
	// 1. Version output explicitly contains "nerdctl"
	// 2. The nerdctl binary exists at /usr/local/bin/nerdctl
	// 3. The docker symlink points to nerdctl
	isNerdctl := strings.Contains(output, "nerdctl")

	if !isNerdctl {
		// Check if nerdctl binary exists directly
		nerdctlResult, err := executor.Exec("test -x /usr/local/bin/nerdctl && echo 'ok'")
		if err == nil && nerdctlResult.ExitCode == 0 && strings.TrimSpace(nerdctlResult.Stdout) == "ok" {
			isNerdctl = true
		}
	}

	if !isNerdctl {
		// Check if docker is a symlink to nerdctl
		symlinkResult, err := executor.Exec("readlink -f $(which docker) 2>/dev/null")
		if err == nil && symlinkResult.ExitCode == 0 {
			if strings.Contains(symlinkResult.Stdout, "nerdctl") {
				isNerdctl = true
			}
		}
	}

	if isNerdctl {
		// It's nerdctl, but is it complete?
		// Check if CNI plugins are installed
		cniResult, err := executor.Exec("test -f /opt/cni/bin/bridge && echo 'ok'")
		if err != nil || cniResult.ExitCode != 0 || strings.TrimSpace(cniResult.Stdout) != "ok" {
			return RuntimeNerdctlIncomplete
		}

		// Also check if CNI config exists
		if !isCNIConfigValid(executor) {
			return RuntimeNerdctlIncomplete
		}

		return RuntimeNerdctl
	}

	// docker command exists but we can't identify it
	return RuntimeNone
}

// IsDockerAvailable checks if a working Docker runtime is available
// (either real Docker or fully installed nerdctl)
func IsDockerAvailable(executor CommandExecutor) bool {
	runtimeType := DetectRuntimeInstallation(executor)
	return runtimeType == RuntimeDocker || runtimeType == RuntimeNerdctl
}

// installCNIPlugins installs only the CNI plugins and config (for incomplete nerdctl installations)
func installCNIPlugins(executor CommandExecutor) error {
	commands := []string{
		// Download and install CNI plugins
		"curl -fsSL https://github.com/containernetworking/plugins/releases/download/v1.4.0/cni-plugins-linux-amd64-v1.4.0.tgz -o /tmp/cni-plugins.tgz",
		"sudo mkdir -p /opt/cni/bin",
		"sudo tar Cxzf /opt/cni/bin /tmp/cni-plugins.tgz",
		"rm /tmp/cni-plugins.tgz",
	}

	for i, cmd := range commands {
		result, err := executor.Exec(cmd)
		if err != nil {
			return fmt.Errorf("CNI plugin installation command %d failed: %w", i+1, err)
		}
		if result.ExitCode != 0 {
			errMsg := result.Stderr
			if errMsg == "" {
				errMsg = result.Stdout
			}
			return fmt.Errorf("CNI plugin installation command %d exited with code %d: %s", i+1, result.ExitCode, strings.TrimSpace(errMsg))
		}
	}

	// Also install CNI config if missing
	if err := installCNIConfig(executor); err != nil {
		return fmt.Errorf("failed to install CNI config: %w", err)
	}

	return nil
}

// installContainerdAndNerdctl installs containerd and nerdctl on Debian/Ubuntu
func installContainerdAndNerdctl(executor CommandExecutor, user string) error {
	// Install containerd from official repos
	commands := []string{
		// Update package index
		"sudo apt-get update -qq",

		// Install containerd and iptables
		// Note: On Debian 11/12, 'iptables' package provides iptables-nft which uses
		// nftables backend. This gives us:
		// - Compatibility with CNI plugins and K3s (both expect iptables commands)
		// - Modern nftables backend under the hood
		// - Easy migration path to pure nftables mode later (K3s --kube-proxy-arg=proxy-mode=nftables)
		"sudo apt-get install -y -qq containerd iptables",

		// Create containerd group (GID 996 matches docker group convention) for socket access
		// This allows non-root users in the containerd group to access the socket
		"sudo groupadd -g 996 containerd 2>/dev/null || true",

		// Add the current user to the containerd group for CLI access
		fmt.Sprintf("sudo usermod -aG containerd %s 2>/dev/null || true", user),

		// Create containerd config directory
		"sudo mkdir -p /etc/containerd",

		// Configure containerd socket permissions via config.toml
		// Setting gid=996 allows containerd group members to access the socket
		`sudo tee /etc/containerd/config.toml > /dev/null << 'CONTAINERD_CONFIG'
version = 2

[grpc]
address = "/run/containerd/containerd.sock"
uid = 0
gid = 996
CONTAINERD_CONFIG`,

		// Enable and start containerd (will pick up the new config)
		"sudo systemctl enable containerd",
		"sudo systemctl restart containerd",

		// Download and install CNI plugins (required by nerdctl for networking)
		"curl -fsSL https://github.com/containernetworking/plugins/releases/download/v1.4.0/cni-plugins-linux-amd64-v1.4.0.tgz -o /tmp/cni-plugins.tgz",
		"sudo mkdir -p /opt/cni/bin",
		"sudo tar Cxzf /opt/cni/bin /tmp/cni-plugins.tgz",
		"rm /tmp/cni-plugins.tgz",

		// Download and install nerdctl
		"curl -fsSL https://github.com/containerd/nerdctl/releases/download/v1.7.2/nerdctl-1.7.2-linux-amd64.tar.gz -o /tmp/nerdctl.tar.gz",
		"sudo tar Cxzf /usr/local/bin /tmp/nerdctl.tar.gz",
		"rm /tmp/nerdctl.tar.gz",

		// Create symlink: docker -> nerdctl (for CLI compatibility)
		"sudo ln -sf /usr/local/bin/nerdctl /usr/local/bin/docker",
	}

	for i, cmd := range commands {
		result, err := executor.Exec(cmd)
		if err != nil {
			return fmt.Errorf("command %d failed: %w", i+1, err)
		}
		if result.ExitCode != 0 {
			// Provide context on failure
			errMsg := result.Stderr
			if errMsg == "" {
				errMsg = result.Stdout
			}
			return fmt.Errorf("command %d exited with code %d: %s", i+1, result.ExitCode, strings.TrimSpace(errMsg))
		}
	}

	// Install CNI configuration for bridge networking with port mapping
	if err := installCNIConfig(executor); err != nil {
		return fmt.Errorf("failed to install CNI config: %w", err)
	}

	return nil
}

// VerifyRuntimeInstalled checks if container runtime is installed and working
func VerifyRuntimeInstalled(executor CommandExecutor) error {
	// Check docker command exists
	result, err := executor.Exec("docker --version")
	if err != nil {
		return fmt.Errorf("failed to check docker version: %w", err)
	}

	if result.ExitCode != 0 {
		return fmt.Errorf("docker command not found")
	}

	// Try to run a simple container to verify runtime works
	// Using 'sudo' because containerd typically requires root
	result, err = executor.Exec("sudo docker run --rm hello-world")
	if err != nil {
		return fmt.Errorf("failed to run test container: %w", err)
	}

	if result.ExitCode != 0 {
		return fmt.Errorf("container test failed: %s", strings.TrimSpace(result.Stderr))
	}

	return nil
}

// isCNIConfigValid checks if the CNI config file exists and contains expected content
func isCNIConfigValid(executor CommandExecutor) bool {
	// Check if the config file exists
	result, err := executor.Exec(fmt.Sprintf("test -f %s && echo 'ok'", CNIConfigPath))
	if err != nil || result.ExitCode != 0 || strings.TrimSpace(result.Stdout) != "ok" {
		return false
	}

	// Check if it contains our expected content (bridge plugin with portmap)
	result, err = executor.Exec(fmt.Sprintf("grep -q 'containerd-net' %s && grep -q 'portmap' %s && echo 'ok'", CNIConfigPath, CNIConfigPath))
	if err != nil || result.ExitCode != 0 || strings.TrimSpace(result.Stdout) != "ok" {
		return false
	}

	return true
}

// installCNIConfig creates the CNI configuration file
func installCNIConfig(executor CommandExecutor) error {
	// Create the CNI config directory if it doesn't exist
	result, err := executor.Exec(fmt.Sprintf("sudo mkdir -p %s", CNIConfigDir))
	if err != nil {
		return fmt.Errorf("failed to create CNI config directory: %w", err)
	}
	if result.ExitCode != 0 {
		return fmt.Errorf("failed to create CNI config directory: %s", strings.TrimSpace(result.Stderr))
	}

	// Write the CNI config file using heredoc
	writeCmd := fmt.Sprintf("sudo tee %s > /dev/null << 'FOUNDRY_CNI_EOF'\n%s\nFOUNDRY_CNI_EOF", CNIConfigPath, CNIConfigContent)
	result, err = executor.Exec(writeCmd)
	if err != nil {
		return fmt.Errorf("failed to write CNI config: %w", err)
	}
	if result.ExitCode != 0 {
		return fmt.Errorf("failed to write CNI config: %s", strings.TrimSpace(result.Stderr))
	}

	return nil
}

// EnsureCNIConfig ensures CNI configuration exists, creating it if missing.
// Returns (created, error) where created indicates if the config was newly created.
// This function is idempotent - safe to call multiple times.
func EnsureCNIConfig(executor CommandExecutor) (bool, error) {
	// Check if valid CNI config already exists
	if isCNIConfigValid(executor) {
		return false, nil
	}

	// Create the CNI configuration
	if err := installCNIConfig(executor); err != nil {
		return false, fmt.Errorf("failed to install CNI config: %w", err)
	}

	return true, nil
}

// RestartBridgeNetworkingServices restarts services that use bridge networking.
// Only restarts services that are currently running.
// Errors are collected but don't stop processing of other services.
// The conn parameter should implement systemd.SSHExecutor (Execute(cmd) (string, error))
func RestartBridgeNetworkingServices(conn SSHExecutor) error {
	var errors []string

	for _, service := range BridgeNetworkingServices {
		// Check if service is running
		running, err := systemd.IsServiceRunning(conn, service)
		if err != nil {
			// Service may not exist yet, skip it
			continue
		}

		if !running {
			// Service exists but not running, skip it
			continue
		}

		// Restart the running service
		if err := systemd.RestartService(conn, service); err != nil {
			errors = append(errors, fmt.Sprintf("%s: %v", service, err))
		}
	}

	if len(errors) > 0 {
		return fmt.Errorf("failed to restart some services: %s", strings.Join(errors, "; "))
	}

	return nil
}
