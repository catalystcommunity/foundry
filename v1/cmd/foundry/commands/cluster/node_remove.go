package cluster

import (
	"context"
	"fmt"
	"time"

	"github.com/catalystcommunity/foundry/v1/internal/component/openbao"
	"github.com/catalystcommunity/foundry/v1/internal/config"
	"github.com/catalystcommunity/foundry/v1/internal/host"
	"github.com/catalystcommunity/foundry/v1/internal/k8s"
	"github.com/catalystcommunity/foundry/v1/internal/ssh"
	"github.com/urfave/cli/v3"
)

// NewNodeRemoveCommand creates the 'cluster node remove' command
func NewNodeRemoveCommand() *cli.Command {
	return &cli.Command{
		Name:      "remove",
		Usage:     "Remove a node from the K3s cluster",
		ArgsUsage: "<hostname>",
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:    "config",
				Aliases: []string{"c"},
				Usage:   "Path to configuration file",
				Sources: cli.EnvVars("FOUNDRY_CONFIG"),
			},
			&cli.BoolFlag{
				Name:  "dry-run",
				Usage: "Show what would be done without making changes",
			},
			&cli.BoolFlag{
				Name:  "force",
				Usage: "Skip confirmation prompts",
			},
		},
		Action: runNodeRemove,
	}
}

func runNodeRemove(ctx context.Context, cmd *cli.Command) error {
	// Get hostname argument
	if cmd.Args().Len() != 1 {
		return fmt.Errorf("hostname argument required")
	}
	hostname := cmd.Args().Get(0)

	// Load configuration
	configPath := cmd.String("config")
	if configPath == "" {
		configPath = config.DefaultConfigPath()
	}

	cfg, err := config.Load(configPath)
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	// Validate cluster config
	if cfg.Cluster.Name == "" {
		return fmt.Errorf("cluster configuration not found")
	}
	if cfg.Network == nil || cfg.Cluster.VIP == "" {
		return fmt.Errorf("network configuration with k8s_vip required")
	}

	// Get host from registry
	_, err = host.Get(hostname)
	if err != nil {
		return fmt.Errorf("host %s not found in registry: %w", hostname, err)
	}

	if cmd.Bool("dry-run") {
		printNodeRemovePlan(hostname, cfg)
		return nil
	}

	// Confirm removal unless forced
	if !cmd.Bool("force") {
		fmt.Printf("\nWARNING: This will remove node %s from cluster %s\n", hostname, cfg.Cluster.Name)
		fmt.Printf("The node will be drained and removed from the cluster.\n")
		fmt.Printf("\nAre you sure you want to continue? (yes/no): ")

		var response string
		fmt.Scanln(&response)
		if response != "yes" {
			fmt.Println("Operation cancelled")
			return nil
		}
	}

	// Remove node from cluster
	fmt.Printf("Removing node %s from cluster %s...\n", hostname, cfg.Cluster.Name)
	if err := removeNodeFromCluster(ctx, hostname, cfg); err != nil {
		return fmt.Errorf("failed to remove node: %w", err)
	}

	fmt.Printf("✓ Node %s successfully removed from cluster\n", hostname)
	return nil
}

func printNodeRemovePlan(hostname string, cfg *config.Config) {
	fmt.Printf("\nNode removal plan:\n")
	fmt.Printf("  Node: %s\n", hostname)
	fmt.Printf("  Cluster: %s\n", cfg.Cluster.Name)

	fmt.Printf("\nSteps:\n")
	fmt.Printf("  1. Get kubeconfig from OpenBAO\n")
	fmt.Printf("  2. Connect to Kubernetes API\n")
	fmt.Printf("  3. Cordon node %s (mark unschedulable)\n", hostname)
	fmt.Printf("  4. Drain node %s (evict all pods)\n", hostname)
	fmt.Printf("  5. Delete node from cluster\n")
	fmt.Printf("  6. Connect to %s via SSH\n", hostname)
	fmt.Printf("  7. Uninstall K3s from node\n")
}

// removeNodeFromCluster performs the actual node removal
func removeNodeFromCluster(ctx context.Context, hostname string, cfg *config.Config) error {
	// Step 1: Get OpenBAO client and load kubeconfig
	fmt.Println("Loading kubeconfig from OpenBAO...")
	openbaoIP, err := cfg.GetPrimaryOpenBAOAddress()
	if err != nil {
		return fmt.Errorf("failed to get OpenBAO address: %w", err)
	}
	openbaoAddr := fmt.Sprintf("http://%s:8200", openbaoIP)
	// TODO: Get token from auth management
	openbaoClient := openbao.NewClient(openbaoAddr, "")

	// Get kubeconfig from OpenBAO
	kubeconfig, err := openbaoClient.ReadSecretV2(ctx, "secret", "foundry-core/k3s")
	if err != nil {
		return fmt.Errorf("failed to get kubeconfig from OpenBAO: %w", err)
	}

	kubeconfigBytes, ok := kubeconfig["kubeconfig"].(string)
	if !ok {
		return fmt.Errorf("kubeconfig not found in OpenBAO secret")
	}

	// Step 2: Create Kubernetes client
	fmt.Println("Connecting to Kubernetes API...")
	k8sClient, err := k8s.NewClientFromKubeconfig([]byte(kubeconfigBytes))
	if err != nil {
		return fmt.Errorf("failed to create Kubernetes client: %w", err)
	}

	// Step 3: Get the node name (may be different from hostname)
	// For simplicity, we'll use hostname as node name
	// In production, we might want to query the node first
	nodeName := hostname

	// Step 4: Cordon the node
	fmt.Printf("Cordoning node %s...\n", nodeName)
	if err := k8sClient.CordonNode(ctx, nodeName); err != nil {
		return fmt.Errorf("failed to cordon node: %w", err)
	}

	// Step 5: Drain the node
	fmt.Printf("Draining node %s (this may take a few minutes)...\n", nodeName)
	if err := k8sClient.DrainNode(ctx, nodeName, 300*time.Second); err != nil {
		return fmt.Errorf("failed to drain node: %w", err)
	}

	// Step 6: Delete node from cluster
	fmt.Printf("Deleting node %s from cluster...\n", nodeName)
	if err := k8sClient.DeleteNode(ctx, nodeName); err != nil {
		return fmt.Errorf("failed to delete node: %w", err)
	}

	// Step 7: Connect to host and uninstall K3s
	fmt.Printf("Connecting to %s to uninstall K3s...\n", hostname)
	conn, err := connectToHost(hostname)
	if err != nil {
		return fmt.Errorf("failed to connect to host: %w", err)
	}
	defer conn.Close()

	// Step 8: Uninstall K3s
	fmt.Println("Uninstalling K3s...")
	if err := uninstallK3s(conn); err != nil {
		return fmt.Errorf("failed to uninstall K3s: %w", err)
	}

	return nil
}

// SSHExecutor defines the interface for executing commands via SSH
type SSHExecutor interface {
	Exec(cmd string) (*ssh.ExecResult, error)
}

// uninstallK3s removes K3s from a host
func uninstallK3s(conn SSHExecutor) error {
	// K3s provides uninstall scripts
	// Try both server and agent uninstall scripts
	commands := []string{
		"/usr/local/bin/k3s-uninstall.sh",
		"/usr/local/bin/k3s-agent-uninstall.sh",
	}

	var lastErr error
	for _, cmd := range commands {
		// Check if script exists
		result, err := conn.Exec(fmt.Sprintf("test -f %s", cmd))
		if err == nil && result.ExitCode == 0 {
			// Script exists, run it
			result, err = conn.Exec(cmd)
			if err != nil {
				lastErr = fmt.Errorf("failed to execute %s: %w", cmd, err)
				continue
			}
			if result.ExitCode != 0 {
				lastErr = fmt.Errorf("%s failed with exit code %d: %s", cmd, result.ExitCode, result.Stderr)
				continue
			}
			return nil // Success
		}
	}

	if lastErr != nil {
		return lastErr
	}

	return fmt.Errorf("no K3s uninstall script found on host")
}
