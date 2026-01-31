package k3s

import (
	"context"
	"fmt"
	"strings"
	"time"
)

// JoinControlPlane joins an additional control plane node to an existing K3s cluster
// This is used to add additional control plane nodes for HA configurations
func JoinControlPlane(ctx context.Context, executor SSHExecutor, existingServerURL string, tokens *Tokens, cfg *Config) error {
	// Ensure we have the cluster token for control plane joins
	if tokens == nil || tokens.ClusterToken == "" {
		return fmt.Errorf("cluster token is required for joining control plane nodes")
	}

	// Ensure server URL is provided
	if existingServerURL == "" {
		return fmt.Errorf("existing server URL is required for joining control plane nodes")
	}

	// Update config for joining (not initializing) - do this before validation
	cfg.ClusterInit = false
	cfg.ServerURL = existingServerURL
	cfg.ClusterToken = tokens.ClusterToken
	cfg.AgentToken = tokens.AgentToken

	// Validate configuration after setting required fields
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

		fmt.Println("   ✓ Updates applied successfully")
		return nil
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

	// Step 3: Install K3s in server mode with --server flag
	installCmd := GenerateK3sInstallCommand(cfg)
	result, err := executor.Exec(installCmd)
	if err != nil {
		return fmt.Errorf("failed to execute K3s install command: %w", err)
	}

	if result.ExitCode != 0 {
		return fmt.Errorf("K3s installation failed with exit code %d: %s", result.ExitCode, result.Stderr)
	}

	// Step 4: Wait for K3s to be ready and join the cluster
	if err := waitForK3sReady(executor, DefaultRetryConfig()); err != nil {
		return fmt.Errorf("K3s failed to become ready: %w", err)
	}

	// Step 5: Verify node joined the cluster
	if err := verifyNodeJoined(executor, DefaultRetryConfig()); err != nil {
		return fmt.Errorf("node failed to join cluster: %w", err)
	}

	return nil
}

// verifyNodeJoined verifies that the node successfully joined the cluster
func verifyNodeJoined(executor SSHExecutor, retryCfg RetryConfig) error {
	// Get the node name
	result, err := executor.Exec("hostname")
	if err != nil {
		return fmt.Errorf("failed to get hostname: %w", err)
	}
	if result.ExitCode != 0 {
		return fmt.Errorf("failed to get hostname: exit code %d", result.ExitCode)
	}
	hostname := strings.TrimSpace(result.Stdout)

	// Check if node appears in cluster with retries (node registration can take time)
	var lastErr error
	for i := 0; i < retryCfg.MaxRetries; i++ {
		result, err = executor.Exec(fmt.Sprintf("sudo k3s kubectl get node %s", hostname))
		if err == nil && result.ExitCode == 0 {
			// Node found, now verify control-plane role
			result, err = executor.Exec(fmt.Sprintf("sudo k3s kubectl get node %s -o jsonpath='{.metadata.labels.node-role\\.kubernetes\\.io/control-plane}'", hostname))
			if err != nil {
				return fmt.Errorf("failed to check node role: %w", err)
			}
			if result.ExitCode != 0 || strings.TrimSpace(result.Stdout) != "true" {
				return fmt.Errorf("node %s does not have control-plane role", hostname)
			}
			return nil
		}
		lastErr = fmt.Errorf("node %s not found in cluster (attempt %d/%d)", hostname, i+1, retryCfg.MaxRetries)
		time.Sleep(retryCfg.RetryDelay)
	}

	return lastErr
}
