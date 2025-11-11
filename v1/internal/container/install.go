package container

import (
	"fmt"
	"strings"
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

	// Check if it's nerdctl
	if strings.Contains(output, "nerdctl") {
		// It's nerdctl, but is it complete?
		// Check if CNI plugins are installed
		cniResult, err := executor.Exec("test -f /opt/cni/bin/bridge && echo 'ok'")
		if err == nil && cniResult.ExitCode == 0 && strings.TrimSpace(cniResult.Stdout) == "ok" {
			return RuntimeNerdctl
		}
		return RuntimeNerdctlIncomplete
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

// installCNIPlugins installs only the CNI plugins (for incomplete nerdctl installations)
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

		// Enable and start containerd
		"sudo systemctl enable containerd",
		"sudo systemctl start containerd",

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

		// Create containerd config directory for nerdctl
		"sudo mkdir -p /etc/containerd",
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
