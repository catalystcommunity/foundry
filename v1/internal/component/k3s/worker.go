package k3s

import (
	"context"
	"fmt"
	"strings"
)

// JoinWorker joins a worker node to an existing K3s cluster
// Workers use the agent token and join via the K3s agent installation
func JoinWorker(ctx context.Context, executor SSHExecutor, serverURL string, tokens *Tokens, cfg *Config) error {
	return reconcileWorker(ctx, executor, serverURL, tokens, cfg, false)
}

// UpgradeWorker reconciles and upgrades an installed K3s agent.
func UpgradeWorker(ctx context.Context, executor SSHExecutor, serverURL string, tokens *Tokens, cfg *Config) error {
	return reconcileWorker(ctx, executor, serverURL, tokens, cfg, true)
}

func reconcileWorker(ctx context.Context, executor SSHExecutor, serverURL string, tokens *Tokens, cfg *Config, upgrade bool) error {
	// Validate that we have the required tokens
	if tokens == nil || tokens.AgentToken == "" {
		return fmt.Errorf("agent token is required for joining worker nodes")
	}

	// Ensure server URL is provided
	if serverURL == "" {
		return fmt.Errorf("server URL is required for joining worker nodes")
	}

	// Validate VIP if provided (worker nodes may not need full config validation)
	if cfg.VIP != "" {
		// Dereference AllowCGNATVIP pointer (defaults to false if nil)
		allowCGNAT := cfg.AllowCGNATVIP != nil && *cfg.AllowCGNATVIP
		if err := ValidateVIP(cfg.VIP, allowCGNAT); err != nil {
			return fmt.Errorf("VIP validation failed: %w", err)
		}
	}

	// Fail fast if the memory cgroup is unavailable (common on Raspberry Pi OS):
	// without it the k3s-agent service crash-loops and never becomes ready
	if err := EnsureMemoryCgroup(executor); err != nil {
		return err
	}

	// Check if K3s agent is already installed (idempotency)
	isInstalled, err := IsK3sAgentInstalled(executor)
	if err != nil {
		return fmt.Errorf("failed to check if K3s agent is installed: %w", err)
	}

	if isInstalled {
		// K3s agent is already installed - apply updates idempotently
		fmt.Println("   K3s agent already installed, applying updates...")
		if upgrade {
			currentVersion, err := GetInstalledVersion(executor)
			if err != nil {
				return err
			}
			selectors, err := K3sUpgradeSelectors(currentVersion, cfg.Version)
			if err != nil {
				return fmt.Errorf("unsafe K3s agent upgrade target: %w", err)
			}
			for step, selector := range selectors {
				fmt.Printf("   Applying K3s agent upgrade step %d/%d...\n", step+1, len(selectors))
				result, err := executor.Exec(generateK3sAgentCommand(serverURL, tokens.AgentToken, selector))
				if err != nil {
					return fmt.Errorf("failed to execute K3s agent upgrade step %d: %w", step+1, err)
				}
				if result.ExitCode != 0 {
					return fmt.Errorf("K3s agent upgrade failed at step %d with exit code %d: %s", step+1, result.ExitCode, result.Stderr)
				}
				if err := waitForK3sAgentReady(executor, DefaultRetryConfig()); err != nil {
					return fmt.Errorf("k3s-agent failed to become ready after upgrade step %d: %w", step+1, err)
				}
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
				// Restart k3s-agent to pick up registry changes
				if _, err := executor.Exec("sudo systemctl restart k3s-agent"); err != nil {
					return fmt.Errorf("failed to restart k3s-agent: %w", err)
				}
				// Wait for k3s-agent to be ready after restart
				if err := waitForK3sAgentReady(executor, DefaultRetryConfig()); err != nil {
					return fmt.Errorf("k3s-agent failed to become ready after restart: %w", err)
				}
			} else {
				fmt.Println("   ✓ registries.yaml unchanged, skipping restart")
			}
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

	// Step 3: Install K3s in agent mode
	installCmd := generateK3sAgentInstallCommandForVersion(serverURL, tokens.AgentToken, cfg.Version)
	result, err := executor.Exec(installCmd)
	if err != nil {
		return fmt.Errorf("failed to execute K3s agent install command: %w", err)
	}

	if result.ExitCode != 0 {
		return fmt.Errorf("K3s agent installation failed with exit code %d: %s", result.ExitCode, result.Stderr)
	}

	// Step 4: Wait for K3s agent to be ready
	if err := waitForK3sAgentReady(executor, DefaultRetryConfig()); err != nil {
		return fmt.Errorf("K3s agent failed to become ready: %w", err)
	}

	// Step 5: Verify node joined the cluster (worker nodes can query via server)
	if err := verifyWorkerNodeJoined(executor); err != nil {
		return fmt.Errorf("worker node failed to join cluster: %w", err)
	}

	return nil
}

// generateK3sAgentInstallCommand generates the K3s agent installation command
func generateK3sAgentInstallCommand(serverURL string, agentToken string) string {
	return generateK3sAgentInstallCommandForVersion(serverURL, agentToken, "")
}

func generateK3sAgentInstallCommandForVersion(serverURL string, agentToken string, version string) string {
	selector := ""
	if version != "" && version != "latest" {
		selector = fmt.Sprintf("INSTALL_K3S_VERSION=%s ", version)
	}
	return generateK3sAgentCommand(serverURL, agentToken, selector)
}

func generateK3sAgentCommand(serverURL, agentToken, selector string) string {
	return fmt.Sprintf("curl -sfL https://get.k3s.io | %sK3S_URL=%s K3S_TOKEN=%s sh -", selector, serverURL, agentToken)
}

// waitForK3sAgentReady waits for K3s agent to be ready
// Agent nodes don't have kubectl, so we check the service status instead
func waitForK3sAgentReady(executor SSHExecutor, retryCfg RetryConfig) error {
	return waitForServiceActive(executor, "k3s-agent", retryCfg)
}

// verifyWorkerNodeJoined verifies that the worker node successfully joined the cluster
// Since worker nodes don't have kubectl access, we verify by checking the kubelet status
func verifyWorkerNodeJoined(executor SSHExecutor) error {
	// Check if k3s-agent service is running
	result, err := executor.Exec("sudo systemctl is-active k3s-agent")
	if err != nil {
		return fmt.Errorf("failed to check k3s-agent status: %w", err)
	}
	if result.ExitCode != 0 || strings.TrimSpace(result.Stdout) != "active" {
		return fmt.Errorf("k3s-agent service is not active")
	}

	// Check kubelet logs for successful registration
	result, err = executor.Exec("sudo journalctl -u k3s-agent -n 100 --no-pager | grep -i 'successfully registered'")
	if err == nil && result.ExitCode == 0 && strings.Contains(result.Stdout, "successfully registered") {
		return nil
	}

	// If we can't find the success message, at least verify the service is healthy
	result, err = executor.Exec("sudo systemctl status k3s-agent")
	if err != nil {
		return fmt.Errorf("failed to get k3s-agent status: %w", err)
	}
	if result.ExitCode != 0 {
		return fmt.Errorf("k3s-agent service is not healthy")
	}

	return nil
}
