package k3s

import (
	"context"
	"fmt"
	"strings"
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
	if err := waitForK3sReady(executor); err != nil {
		return fmt.Errorf("K3s failed to become ready: %w", err)
	}

	// Step 5: Verify node joined the cluster
	if err := verifyNodeJoined(executor); err != nil {
		return fmt.Errorf("node failed to join cluster: %w", err)
	}

	return nil
}

// verifyNodeJoined verifies that the node successfully joined the cluster
func verifyNodeJoined(executor SSHExecutor) error {
	// Get the node name
	result, err := executor.Exec("hostname")
	if err != nil {
		return fmt.Errorf("failed to get hostname: %w", err)
	}
	if result.ExitCode != 0 {
		return fmt.Errorf("failed to get hostname: exit code %d", result.ExitCode)
	}
	hostname := strings.TrimSpace(result.Stdout)

	// Check if node appears in cluster
	result, err = executor.Exec(fmt.Sprintf("sudo k3s kubectl get node %s", hostname))
	if err != nil {
		return fmt.Errorf("failed to query node status: %w", err)
	}
	if result.ExitCode != 0 {
		return fmt.Errorf("node %s not found in cluster", hostname)
	}

	// Verify node has control-plane role
	result, err = executor.Exec(fmt.Sprintf("sudo k3s kubectl get node %s -o jsonpath='{.metadata.labels.node-role\\.kubernetes\\.io/control-plane}'", hostname))
	if err != nil {
		return fmt.Errorf("failed to check node role: %w", err)
	}
	if result.ExitCode != 0 || strings.TrimSpace(result.Stdout) != "true" {
		return fmt.Errorf("node %s does not have control-plane role", hostname)
	}

	return nil
}
