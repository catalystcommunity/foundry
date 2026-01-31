package k3s

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/catalystcommunity/foundry/v1/internal/network"
)

// RetryConfig holds configuration for retry operations
type RetryConfig struct {
	MaxRetries int
	RetryDelay time.Duration
}

// DefaultRetryConfig returns the default retry configuration
func DefaultRetryConfig() RetryConfig {
	return RetryConfig{
		MaxRetries: 30,
		RetryDelay: 10 * time.Second,
	}
}

// IsK3sInstalled checks if K3s is already installed and running on a node
func IsK3sInstalled(executor SSHExecutor) (bool, error) {
	result, err := executor.Exec("systemctl is-active k3s 2>/dev/null")
	if err != nil {
		return false, fmt.Errorf("failed to check K3s status: %w", err)
	}

	// If exit code is 0 and output is "active", K3s is installed and running
	return result.ExitCode == 0 && strings.TrimSpace(result.Stdout) == "active", nil
}

// IsK3sAgentInstalled checks if K3s agent is already installed and running on a node
func IsK3sAgentInstalled(executor SSHExecutor) (bool, error) {
	result, err := executor.Exec("systemctl is-active k3s-agent 2>/dev/null")
	if err != nil {
		return false, fmt.Errorf("failed to check K3s agent status: %w", err)
	}

	// If exit code is 0 and output is "active", K3s agent is installed and running
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
		// K3s is already installed - apply updates idempotently
		fmt.Println("   K3s already installed, applying updates...")

		// Track if we need to restart K3s
		needsRestart := false

		// Update etcd config if configured (for virtualized environments)
		if len(cfg.EtcdArgs) > 0 {
			changed, err := updateEtcdConfig(executor, cfg.EtcdArgs)
			if err != nil {
				return fmt.Errorf("failed to update etcd config: %w", err)
			}
			if changed {
				fmt.Println("   etcd config updated")
				needsRestart = true
			} else {
				fmt.Println("   ✓ etcd config unchanged")
			}
		}

		// Update registries.yaml if configured (idempotent - only restart if changed)
		if cfg.RegistryConfig != "" {
			fmt.Println("   Updating registries.yaml...")
			// Check if config actually changed before restarting
			existingResult, _ := executor.Exec("cat /etc/rancher/k3s/registries.yaml 2>/dev/null")
			existingConfig := ""
			if existingResult != nil {
				existingConfig = existingResult.Stdout
			}
			if strings.TrimSpace(existingConfig) != strings.TrimSpace(cfg.RegistryConfig) {
				if err := createRegistriesConfig(executor, cfg.RegistryConfig); err != nil {
					return fmt.Errorf("failed to update registries config: %w", err)
				}
				fmt.Println("   registries.yaml updated")
				needsRestart = true
			} else {
				fmt.Println("   ✓ registries.yaml unchanged")
			}
		}

		// Restart K3s if any config changed
		if needsRestart {
			fmt.Println("   Restarting k3s to apply config changes...")
			if _, err := executor.Exec("sudo systemctl restart k3s"); err != nil {
				return fmt.Errorf("failed to restart k3s: %w", err)
			}
			// Wait for k3s to be ready after restart
			if err := waitForK3sReady(executor, DefaultRetryConfig()); err != nil {
				return fmt.Errorf("k3s failed to become ready after restart: %w", err)
			}
		} else {
			fmt.Println("   ✓ No config changes, skipping restart")
		}

		// Detect network interface for kube-vip if not provided
		if cfg.Interface == "" {
			iface, err := network.DetectPrimaryInterface(executor)
			if err != nil {
				return fmt.Errorf("failed to detect network interface: %w", err)
			}
			cfg.Interface = iface
		}

		// Apply kube-vip manifests (idempotent via kubectl apply)
		// This updates RBAC, ConfigMaps, and other resources
		if err := setupKubeVIP(ctx, executor, cfg); err != nil {
			return fmt.Errorf("failed to update kube-vip: %w", err)
		}

		// Note: kube-vip-cloud-provider restart is no longer done automatically
		// as it causes unnecessary VIP disruption. The deployment is idempotent
		// via kubectl apply, and config changes are picked up on pod restart.

		fmt.Println("   ✓ Updates applied successfully")
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
	if err := waitForK3sReady(executor, DefaultRetryConfig()); err != nil {
		return fmt.Errorf("K3s failed to become ready: %w", err)
	}

	// Step 5: Set up kube-vip
	if err := setupKubeVIP(ctx, executor, cfg); err != nil {
		return fmt.Errorf("failed to setup kube-vip: %w", err)
	}

	// Step 6: Wait for kube-vip to be ready
	if err := waitForKubeVIPReady(executor, cfg.VIP, DefaultRetryConfig()); err != nil {
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
func waitForK3sReady(executor SSHExecutor, retryCfg RetryConfig) error {
	for i := 0; i < retryCfg.MaxRetries; i++ {
		result, err := executor.Exec("sudo k3s kubectl get nodes")
		if err == nil && result.ExitCode == 0 {
			return nil
		}

		time.Sleep(retryCfg.RetryDelay)
	}

	return fmt.Errorf("K3s did not become ready after %d retries", retryCfg.MaxRetries)
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
func waitForKubeVIPReady(executor SSHExecutor, vip string, retryCfg RetryConfig) error {
	for i := 0; i < retryCfg.MaxRetries; i++ {
		// Check if VIP is assigned to the interface
		result, err := executor.Exec(fmt.Sprintf("ip addr show | grep %s", vip))
		if err == nil && result.ExitCode == 0 && strings.Contains(result.Stdout, vip) {
			return nil
		}

		time.Sleep(retryCfg.RetryDelay)
	}

	return fmt.Errorf("kube-vip did not configure VIP after %d retries", retryCfg.MaxRetries)
}

// updateEtcdConfig creates or updates the etcd configuration file for K3s
// Returns true if the config was changed and K3s needs a restart
func updateEtcdConfig(executor SSHExecutor, etcdArgs []string) (bool, error) {
	if len(etcdArgs) == 0 {
		return false, nil
	}

	// Generate the config content
	// K3s uses config.yaml.d/ for drop-in configs
	var configLines []string
	configLines = append(configLines, "# etcd tuning for virtualized environments")
	configLines = append(configLines, "# Generated by foundry")
	configLines = append(configLines, "etcd-arg:")
	for _, arg := range etcdArgs {
		configLines = append(configLines, fmt.Sprintf("  - \"%s\"", arg))
	}
	newConfig := strings.Join(configLines, "\n") + "\n"

	// Ensure the config directory exists
	configPath := "/etc/rancher/k3s/config.yaml.d/etcd-tuning.yaml"
	result, err := executor.Exec("sudo mkdir -p /etc/rancher/k3s/config.yaml.d")
	if err != nil {
		return false, fmt.Errorf("failed to create config directory: %w", err)
	}
	if result.ExitCode != 0 {
		return false, fmt.Errorf("failed to create config directory: exit code %d", result.ExitCode)
	}

	// Check existing config
	existingResult, _ := executor.Exec(fmt.Sprintf("cat %s 2>/dev/null", configPath))
	existingConfig := ""
	if existingResult != nil {
		existingConfig = existingResult.Stdout
	}

	// Compare configs
	if strings.TrimSpace(existingConfig) == strings.TrimSpace(newConfig) {
		return false, nil
	}

	// Write new config
	escapedConfig := strings.ReplaceAll(newConfig, "'", "'\"'\"'")
	writeCmd := fmt.Sprintf("echo '%s' | sudo tee %s", escapedConfig, configPath)
	result, err = executor.Exec(writeCmd)
	if err != nil {
		return false, fmt.Errorf("failed to write etcd config: %w", err)
	}
	if result.ExitCode != 0 {
		return false, fmt.Errorf("failed to write etcd config: exit code %d, stderr: %s", result.ExitCode, result.Stderr)
	}

	return true, nil
}
