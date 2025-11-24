package k3s

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/catalystcommunity/foundry/v1/internal/network"
)

const (
	// MaxRetries is the maximum number of retries for waiting operations
	MaxRetries = 30

	// RetryDelay is the delay between retries
	RetryDelay = 10 * time.Second
)

// IsK3sInstalled checks if K3s is already installed and running on a node
func IsK3sInstalled(executor SSHExecutor) (bool, error) {
	result, err := executor.Exec("systemctl is-active k3s 2>/dev/null")
	if err != nil {
		return false, fmt.Errorf("failed to check K3s status: %w", err)
	}

	// If exit code is 0 and output is "active", K3s is installed and running
	return result.ExitCode == 0 && strings.TrimSpace(result.Stdout) == "active", nil
}

// IsKubeVIPInstalled checks if kube-vip is already deployed in the cluster
func IsKubeVIPInstalled(executor SSHExecutor) (bool, error) {
	// Check if kube-vip daemonset exists
	result, err := executor.Exec("sudo kubectl get daemonset -n kube-system kube-vip 2>/dev/null")
	if err != nil {
		return false, fmt.Errorf("failed to check kube-vip status: %w", err)
	}

	// If exit code is 0, kube-vip daemonset exists
	return result.ExitCode == 0, nil
}

// InstallControlPlane installs K3s control plane on a node
func InstallControlPlane(ctx context.Context, executor SSHExecutor, cfg *Config) error {
	// Validate configuration
	if err := cfg.Validate(); err != nil {
		return fmt.Errorf("config validation failed: %w", err)
	}

	// Check if K3s is already installed (idempotency)
	isInstalled, err := IsK3sInstalled(executor)
	if err != nil {
		return fmt.Errorf("failed to check if K3s is installed: %w", err)
	}

	if isInstalled {
		// K3s is already installed, but we need to check kube-vip separately
		fmt.Println("   K3s already installed, skipping K3s installation")

		// Check if kube-vip is also installed
		kubeVIPInstalled, err := IsKubeVIPInstalled(executor)
		if err != nil {
			return fmt.Errorf("failed to check if kube-vip is installed: %w", err)
		}

		if kubeVIPInstalled {
			// Both K3s and kube-vip are installed, nothing to do
			fmt.Println("   kube-vip already installed, skipping setup")
			return nil
		}

		// K3s is installed but kube-vip is not, set it up
		fmt.Println("   kube-vip not found, setting it up...")

		// Detect network interface for kube-vip
		if cfg.Interface == "" {
			iface, err := network.DetectPrimaryInterface(executor)
			if err != nil {
				return fmt.Errorf("failed to detect network interface: %w", err)
			}
			cfg.Interface = iface
		}

		if err := setupKubeVIP(ctx, executor, cfg); err != nil {
			return fmt.Errorf("failed to setup kube-vip: %w", err)
		}

		if err := waitForKubeVIPReady(executor, cfg.VIP); err != nil {
			return fmt.Errorf("kube-vip failed to become ready: %w", err)
		}

		return nil
	}

	// Detect network interface if not provided
	if cfg.Interface == "" {
		iface, err := network.DetectPrimaryInterface(executor)
		if err != nil {
			return fmt.Errorf("failed to detect network interface: %w", err)
		}
		cfg.Interface = iface
	}

	// Step 1: Configure DNS (if DNS servers provided)
	if len(cfg.DNSServers) > 0 {
		if err := configureDNS(executor, cfg.DNSServers); err != nil {
			return fmt.Errorf("failed to configure DNS: %w", err)
		}
	}

	// Step 2: Create registries.yaml (if Zot is configured)
	if cfg.RegistryConfig != "" {
		if err := createRegistriesConfig(executor, cfg.RegistryConfig); err != nil {
			return fmt.Errorf("failed to create registries config: %w", err)
		}
	}

	// Step 3: Install K3s
	installCmd := GenerateK3sInstallCommand(cfg)
	result, err := executor.Exec(installCmd)
	if err != nil {
		return fmt.Errorf("failed to execute K3s install command: %w", err)
	}

	if result.ExitCode != 0 {
		return fmt.Errorf("K3s installation failed with exit code %d: %s", result.ExitCode, result.Stderr)
	}

	// Step 4: Wait for K3s to be ready
	if err := waitForK3sReady(executor); err != nil {
		return fmt.Errorf("K3s failed to become ready: %w", err)
	}

	// Step 5: Set up kube-vip
	if err := setupKubeVIP(ctx, executor, cfg); err != nil {
		return fmt.Errorf("failed to setup kube-vip: %w", err)
	}

	// Step 6: Wait for kube-vip to be ready
	if err := waitForKubeVIPReady(executor, cfg.VIP); err != nil {
		return fmt.Errorf("kube-vip failed to become ready: %w", err)
	}

	return nil
}

// configureDNS configures DNS on the node
func configureDNS(executor SSHExecutor, dnsServers []string) error {
	// Check if systemd-resolved is running
	result, err := executor.Exec("systemctl is-active systemd-resolved")
	if err != nil {
		return fmt.Errorf("failed to check systemd-resolved status: %w", err)
	}

	useSystemdResolved := result.ExitCode == 0 && strings.TrimSpace(result.Stdout) == "active"

	if useSystemdResolved {
		// Use systemd-resolved configuration
		config := GenerateSystemdResolvdConfig(dnsServers, nil)

		// Write to /etc/systemd/resolved.conf.d/foundry.conf
		writeCmd := fmt.Sprintf("sudo mkdir -p /etc/systemd/resolved.conf.d && echo '%s' | sudo tee /etc/systemd/resolved.conf.d/foundry.conf", config)
		result, err := executor.Exec(writeCmd)
		if err != nil {
			return fmt.Errorf("failed to write systemd-resolved config: %w", err)
		}
		if result.ExitCode != 0 {
			return fmt.Errorf("failed to write systemd-resolved config: exit code %d, stderr: %s", result.ExitCode, result.Stderr)
		}

		// Restart systemd-resolved
		result, err = executor.Exec("sudo systemctl restart systemd-resolved")
		if err != nil {
			return fmt.Errorf("failed to restart systemd-resolved: %w", err)
		}
		if result.ExitCode != 0 {
			return fmt.Errorf("failed to restart systemd-resolved: exit code %d, stderr: %s", result.ExitCode, result.Stderr)
		}
	} else {
		// Use traditional /etc/resolv.conf
		config := GenerateResolvConfContent(dnsServers, nil)

		// Make resolv.conf immutable to prevent automatic changes
		immutableCmd := fmt.Sprintf("echo '%s' | sudo tee /etc/resolv.conf && sudo chattr +i /etc/resolv.conf", config)
		result, err := executor.Exec(immutableCmd)
		if err != nil {
			return fmt.Errorf("failed to write resolv.conf: %w", err)
		}
		if result.ExitCode != 0 {
			return fmt.Errorf("failed to write resolv.conf: exit code %d, stderr: %s", result.ExitCode, result.Stderr)
		}
	}

	return nil
}

// createRegistriesConfig creates the /etc/rancher/k3s/registries.yaml file
func createRegistriesConfig(executor SSHExecutor, registryConfigContent string) error {
	// Create directory
	result, err := executor.Exec("sudo mkdir -p /etc/rancher/k3s")
	if err != nil {
		return fmt.Errorf("failed to create registries config directory: %w", err)
	}
	if result.ExitCode != 0 {
		return fmt.Errorf("failed to create registries config directory: exit code %d, stderr: %s", result.ExitCode, result.Stderr)
	}

	// Write registries.yaml
	// Escape single quotes in the content
	escapedContent := strings.ReplaceAll(registryConfigContent, "'", "'\"'\"'")
	writeCmd := fmt.Sprintf("echo '%s' | sudo tee %s", escapedContent, RegistriesConfigPath)
	result, err = executor.Exec(writeCmd)
	if err != nil {
		return fmt.Errorf("failed to write registries config: %w", err)
	}
	if result.ExitCode != 0 {
		return fmt.Errorf("failed to write registries config: exit code %d, stderr: %s", result.ExitCode, result.Stderr)
	}

	return nil
}

// waitForK3sReady waits for K3s to be ready
func waitForK3sReady(executor SSHExecutor) error {
	for i := 0; i < MaxRetries; i++ {
		result, err := executor.Exec("sudo k3s kubectl get nodes")
		if err == nil && result.ExitCode == 0 {
			return nil
		}

		time.Sleep(RetryDelay)
	}

	return fmt.Errorf("K3s did not become ready after %d retries", MaxRetries)
}

// setupKubeVIP sets up kube-vip on the control plane node
func setupKubeVIP(ctx context.Context, executor SSHExecutor, cfg *Config) error {
	// Determine VIP config
	vipConfig := &VIPConfig{
		VIP:       cfg.VIP,
		Interface: cfg.Interface,
	}

	// Generate kube-vip manifests
	rbacManifest := GenerateKubeVIPRBACManifest()
	cloudProviderManifest := GenerateKubeVIPCloudProviderManifest()
	configMapManifest, err := GenerateKubeVIPConfigMap(fmt.Sprintf("%s/32", cfg.VIP))
	if err != nil {
		return fmt.Errorf("failed to generate kube-vip configmap: %w", err)
	}
	daemonsetManifest, err := GenerateKubeVIPManifest(vipConfig)
	if err != nil {
		return fmt.Errorf("failed to generate kube-vip manifest: %w", err)
	}

	// Combine all manifests
	allManifests := FormatManifests(rbacManifest, cloudProviderManifest, configMapManifest, daemonsetManifest)

	// Apply manifests using kubectl
	escapedManifests := strings.ReplaceAll(allManifests, "'", "'\"'\"'")
	applyCmd := fmt.Sprintf("echo '%s' | sudo k3s kubectl apply -f -", escapedManifests)
	result, err := executor.Exec(applyCmd)
	if err != nil {
		return fmt.Errorf("failed to apply kube-vip manifests: %w", err)
	}
	if result.ExitCode != 0 {
		return fmt.Errorf("failed to apply kube-vip manifests: exit code %d, stderr: %s", result.ExitCode, result.Stderr)
	}

	return nil
}

// waitForKubeVIPReady waits for kube-vip to configure the VIP
func waitForKubeVIPReady(executor SSHExecutor, vip string) error {
	for i := 0; i < MaxRetries; i++ {
		// Check if VIP is assigned to the interface
		result, err := executor.Exec(fmt.Sprintf("ip addr show | grep %s", vip))
		if err == nil && result.ExitCode == 0 && strings.Contains(result.Stdout, vip) {
			return nil
		}

		time.Sleep(RetryDelay)
	}

	return fmt.Errorf("kube-vip did not configure VIP after %d retries", MaxRetries)
}
