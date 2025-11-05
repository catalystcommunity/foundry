package k3s

import (
	"context"
	"fmt"
	"strings"
	"time"
)

// JoinWorker joins a worker node to an existing K3s cluster
// Workers use the agent token and join via the K3s agent installation
func JoinWorker(ctx context.Context, executor SSHExecutor, serverURL string, tokens *Tokens, cfg *Config) error {
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
		if err := ValidateVIP(cfg.VIP); err != nil {
			return fmt.Errorf("VIP validation failed: %w", err)
		}
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
	installCmd := generateK3sAgentInstallCommand(serverURL, tokens.AgentToken)
	result, err := executor.Exec(installCmd)
	if err != nil {
		return fmt.Errorf("failed to execute K3s agent install command: %w", err)
	}

	if result.ExitCode != 0 {
		return fmt.Errorf("K3s agent installation failed with exit code %d: %s", result.ExitCode, result.Stderr)
	}

	// Step 4: Wait for K3s agent to be ready
	if err := waitForK3sAgentReady(executor); err != nil {
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
	// K3s agent installation uses different syntax than server
	return fmt.Sprintf("curl -sfL https://get.k3s.io | K3S_URL=%s K3S_TOKEN=%s sh -", serverURL, agentToken)
}

// waitForK3sAgentReady waits for K3s agent to be ready
// Agent nodes don't have kubectl, so we check the service status instead
func waitForK3sAgentReady(executor SSHExecutor) error {
	for i := 0; i < MaxRetries; i++ {
		result, err := executor.Exec("sudo systemctl is-active k3s-agent")
		if err == nil && result.ExitCode == 0 && strings.TrimSpace(result.Stdout) == "active" {
			return nil
		}

		// Also check if service exists
		result, err = executor.Exec("sudo systemctl status k3s-agent")
		if err == nil && result.ExitCode == 0 {
			// Service exists and running
			return nil
		}

		time.Sleep(RetryDelay)
	}

	return fmt.Errorf("K3s agent did not become ready after %d retries", MaxRetries)
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
