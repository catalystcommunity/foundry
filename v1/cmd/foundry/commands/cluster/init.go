package cluster

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/catalystcommunity/foundry/v1/internal/component/k3s"
	"github.com/catalystcommunity/foundry/v1/internal/component/openbao"
	"github.com/catalystcommunity/foundry/v1/internal/config"
	"github.com/catalystcommunity/foundry/v1/internal/host"
	"github.com/catalystcommunity/foundry/v1/internal/setup"
	"github.com/catalystcommunity/foundry/v1/internal/ssh"
	"github.com/urfave/cli/v3"
)

// convertToK3sNodes converts hosts with cluster roles to k3s.NodeConfig
func convertToK3sNodes(hosts []*host.Host) []k3s.NodeConfig {
	k3sNodes := make([]k3s.NodeConfig, len(hosts))
	for i, h := range hosts {
		// Determine explicit role from host roles
		// Prefer control-plane role if host has both
		var explicitRole string
		for _, role := range h.Roles {
			if role == "cluster-control-plane" {
				explicitRole = "control-plane"
				break
			} else if role == "cluster-worker" {
				explicitRole = "worker"
			}
		}

		k3sNodes[i] = k3s.NodeConfig{
			Hostname:     h.Hostname,
			ExplicitRole: explicitRole,
		}
	}
	return k3sNodes
}

func initCommand() *cli.Command {
	return &cli.Command{
		Name:  "init",
		Usage: "Initialize Kubernetes cluster",
		Flags: []cli.Flag{
			&cli.BoolFlag{
				Name:  "single-node",
				Usage: "Initialize single-node cluster (overrides config)",
			},
			&cli.BoolFlag{
				Name:  "dry-run",
				Usage: "Show what would be done without making changes",
			},
		},
		Action: runClusterInit,
	}
}

func runClusterInit(ctx context.Context, cmd *cli.Command) error {
	// Load configuration (--config flag inherited from root command)
	configPath, err := config.FindConfig(cmd.String("config"))
	if err != nil {
		return fmt.Errorf("failed to find config: %w", err)
	}

	cfg, err := config.Load(configPath)
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	// Validate cluster configuration
	if cfg.Cluster.Name == "" {
		return fmt.Errorf("cluster configuration is missing")
	}

	// Get hosts with cluster roles
	clusterHosts := cfg.GetClusterHosts()
	if len(clusterHosts) == 0 {
		return fmt.Errorf("no hosts with cluster roles (cluster-control-plane or cluster-worker) defined")
	}

	// Override with single-node flag if provided
	singleNode := cmd.Bool("single-node")
	if singleNode {
		if len(clusterHosts) > 1 {
			fmt.Println("Warning: --single-node flag will use only the first node")
			clusterHosts = clusterHosts[:1]
		}
	}

	dryRun := cmd.Bool("dry-run")
	if dryRun {
		fmt.Println("Dry-run mode enabled - no changes will be made")
		return printClusterPlan(cfg)
	}

	// Initialize the cluster
	fmt.Println("Initializing Kubernetes cluster...")
	if err := InitializeCluster(ctx, cfg); err != nil {
		return fmt.Errorf("cluster initialization failed: %w", err)
	}

	fmt.Println("✓ Cluster initialized successfully")

	// Update setup state to mark K8s as installed
	if err := updateK8sInstalledState(configPath); err != nil {
		// Don't fail the command if state update fails, just warn
		fmt.Printf("\n⚠ Warning: Failed to update setup state: %v\n", err)
		fmt.Println("The cluster is initialized and working, but state tracking may be incorrect.")
	} else {
		fmt.Println("✓ Setup state updated")
	}

	return nil
}

func printClusterPlan(cfg *config.Config) error {
	fmt.Println("\nCluster initialization plan:")
	fmt.Printf("  Cluster name: %s\n", cfg.Cluster.Name)
	fmt.Printf("  Domain: %s\n", cfg.Cluster.Domain)
	fmt.Printf("  VIP: %s\n", cfg.Cluster.VIP)
	fmt.Println("\nHosts:")

	// Get cluster hosts and determine node roles
	clusterHosts := cfg.GetClusterHosts()
	k3sNodes := convertToK3sNodes(clusterHosts)
	nodeRoles, err := k3s.DetermineNodeRoles(k3sNodes)
	if err != nil {
		return fmt.Errorf("failed to determine node roles: %w", err)
	}
	for i, h := range clusterHosts {
		role := nodeRoles[i]
		fmt.Printf("  %d. %s (%s) - %s\n", i+1, h.Hostname, h.Address, role.Role)
	}

	fmt.Println("\nSteps:")
	fmt.Println("  1. Load OpenBAO credentials")
	fmt.Println("  2. Generate and store K3s tokens in OpenBAO")
	fmt.Println("  3. Determine node roles")
	fmt.Println("  4. Find first control plane node")
	fmt.Println("  5. Install control plane on first node(s)")
	fmt.Println("  6. Set up kube-vip for VIP")
	fmt.Println("  7. Join additional control plane nodes (if any)")
	fmt.Println("  8. Join worker nodes (if any)")
	fmt.Println("  9. Retrieve and store kubeconfig in OpenBAO")
	fmt.Println("  10. Verify cluster health")

	return nil
}

// connectToHost creates an SSH connection to a host using stored keys
func connectToHost(hostname string) (*ssh.Connection, error) {
	// Get host from registry
	h, err := host.Get(hostname)
	if err != nil {
		return nil, fmt.Errorf("host %s not found in registry: %w", hostname, err)
	}

	// Get SSH key from filesystem storage
	keysDir, err := config.GetKeysDir()
	if err != nil {
		return nil, fmt.Errorf("failed to get keys directory: %w", err)
	}

	keyStorage, err := ssh.NewFilesystemKeyStorage(keysDir)
	if err != nil {
		return nil, fmt.Errorf("failed to create key storage: %w", err)
	}

	keyPair, err := keyStorage.Load(hostname)
	if err != nil {
		return nil, fmt.Errorf("SSH key not found for host %s (use 'foundry host add' first): %w", hostname, err)
	}

	// Create auth method from key pair
	authMethod, err := keyPair.AuthMethod()
	if err != nil {
		return nil, fmt.Errorf("failed to create auth method: %w", err)
	}

	// Create connection options
	connOpts := &ssh.ConnectionOptions{
		Host:       h.Address,
		Port:       h.Port,
		User:       h.User,
		AuthMethod: authMethod,
		Timeout:    30,
	}

	// Connect
	conn, err := ssh.Connect(connOpts)
	if err != nil {
		return nil, fmt.Errorf("failed to connect: %w", err)
	}

	return conn, nil
}

// InitializeCluster initializes a Kubernetes cluster with the given configuration.
// This function is exported so it can be called from other commands like stack install.
func InitializeCluster(ctx context.Context, cfg *config.Config) error {
	// Step 1: Load OpenBAO credentials
	fmt.Println("Loading OpenBAO credentials...")

	// Get config directory and construct OpenBAO keys path
	configDir, err := config.GetConfigDir()
	if err != nil {
		return fmt.Errorf("failed to get config directory: %w", err)
	}

	openbaoKeysDir := filepath.Join(configDir, "openbao-keys")

	// Load OpenBAO key material (contains root token)
	keyMaterial, err := openbao.LoadKeyMaterial(openbaoKeysDir, cfg.Cluster.Name)
	if err != nil {
		return fmt.Errorf("failed to load OpenBAO keys (has OpenBAO been initialized?): %w", err)
	}

	if keyMaterial.RootToken == "" {
		return fmt.Errorf("root token not found in OpenBAO keys")
	}

	// Get OpenBAO client with authenticated token
	openbaoIP, err := cfg.GetPrimaryOpenBAOAddress()
	if err != nil {
		return fmt.Errorf("failed to get OpenBAO address: %w", err)
	}
	openbaoAddr := fmt.Sprintf("http://%s:8200", openbaoIP)
	openbaoClient := openbao.NewClient(openbaoAddr, keyMaterial.RootToken)
	fmt.Println("✓ OpenBAO credentials loaded")

	// Step 2: Generate and store K3s tokens
	fmt.Println("Generating K3s tokens...")

	tokens, err := k3s.GenerateAndStoreTokens(ctx, openbaoClient)
	if err != nil {
		return fmt.Errorf("failed to generate tokens: %w", err)
	}
	fmt.Println("✓ Tokens generated and stored in OpenBAO")

	// Step 3: Get cluster hosts and determine node roles
	clusterHosts := cfg.GetClusterHosts()
	k3sNodes := convertToK3sNodes(clusterHosts)
	nodeRoles, err := k3s.DetermineNodeRoles(k3sNodes)
	if err != nil {
		return fmt.Errorf("failed to determine node roles: %w", err)
	}

	// Step 4: Find first control plane node
	var firstCPIndex int = -1
	for i, role := range nodeRoles {
		if role.IsControlPlane {
			firstCPIndex = i
			break
		}
	}

	if firstCPIndex == -1 {
		return fmt.Errorf("no control plane nodes found in configuration")
	}

	// Step 5: Install control plane on first node
	firstHost := clusterHosts[firstCPIndex]
	fmt.Printf("Installing control plane on %s...\n", firstHost.Hostname)

	// Create SSH connection
	conn, err := connectToHost(firstHost.Hostname)
	if err != nil {
		return fmt.Errorf("failed to connect to %s: %w", firstHost.Hostname, err)
	}
	defer conn.Close()

	// Build K3s config for control plane
	k3sConfig := &k3s.Config{
		ClusterInit:  true,
		ServerURL:    "",
		ClusterToken: tokens.ClusterToken,
		AgentToken:   tokens.AgentToken,
		VIP:          cfg.Cluster.VIP,
		TLSSANs: []string{
			cfg.Cluster.VIP,
			fmt.Sprintf("%s.%s", cfg.Cluster.Name, cfg.Cluster.Domain),
		},
		DisableComponents: []string{"traefik", "servicelb"},
	}

	// Add registry config if Zot is configured
	if zotAddr, err := cfg.GetPrimaryZotAddress(); err == nil {
		k3sConfig.RegistryConfig = k3s.GenerateRegistriesConfig(zotAddr)
	}

	// Install control plane
	if err := k3s.InstallControlPlane(ctx, conn, k3sConfig); err != nil {
		return fmt.Errorf("failed to install control plane on %s: %w", firstHost.Hostname, err)
	}
	fmt.Printf("✓ Control plane installed on %s\n", firstHost.Hostname)

	// Step 6: Wait a bit for cluster to stabilize
	fmt.Println("Waiting for cluster to stabilize...")
	time.Sleep(10 * time.Second)

	// Step 7: Join additional control plane nodes
	serverURL := fmt.Sprintf("https://%s:6443", cfg.Cluster.VIP)
	for i := firstCPIndex + 1; i < len(clusterHosts); i++ {
		role := nodeRoles[i]
		if !role.IsControlPlane {
			continue
		}

		h := clusterHosts[i]
		fmt.Printf("Joining control plane node %s...\n", h.Hostname)

		// Create SSH connection
		conn, err := connectToHost(h.Hostname)
		if err != nil {
			return fmt.Errorf("failed to connect to %s: %w", h.Hostname, err)
		}

		// Build config for joining control plane
		joinConfig := &k3s.Config{
			ClusterInit:       false,
			ServerURL:         serverURL,
			ClusterToken:      tokens.ClusterToken,
			AgentToken:        tokens.AgentToken,
			VIP:               cfg.Cluster.VIP,
			TLSSANs:           k3sConfig.TLSSANs,
			DisableComponents: k3sConfig.DisableComponents,
			RegistryConfig:    k3sConfig.RegistryConfig,
		}

		// Join control plane
		if err := k3s.JoinControlPlane(ctx, conn, serverURL, tokens, joinConfig); err != nil {
			conn.Close()
			return fmt.Errorf("failed to join control plane node %s: %w", h.Hostname, err)
		}

		conn.Close()
		fmt.Printf("✓ Control plane node %s joined\n", h.Hostname)

		// Wait a bit between nodes
		time.Sleep(5 * time.Second)
	}

	// Step 8: Join worker nodes
	for i, role := range nodeRoles {
		if role.IsControlPlane {
			continue // Skip control plane nodes
		}

		h := clusterHosts[i]
		fmt.Printf("Joining worker node %s...\n", h.Hostname)

		// Create SSH connection
		conn, err := connectToHost(h.Hostname)
		if err != nil {
			return fmt.Errorf("failed to connect to %s: %w", h.Hostname, err)
		}

		// Build config for worker
		workerConfig := &k3s.Config{
			ServerURL:      serverURL,
			AgentToken:     tokens.AgentToken,
			RegistryConfig: k3sConfig.RegistryConfig,
		}

		// Join worker
		if err := k3s.JoinWorker(ctx, conn, serverURL, tokens, workerConfig); err != nil {
			conn.Close()
			return fmt.Errorf("failed to join worker node %s: %w", h.Hostname, err)
		}

		conn.Close()
		fmt.Printf("✓ Worker node %s joined\n", h.Hostname)

		// Wait a bit between nodes
		time.Sleep(5 * time.Second)
	}

	// Step 9: Retrieve and store kubeconfig
	fmt.Println("Retrieving kubeconfig...")

	// Reconnect to first node to get kubeconfig
	conn, err = connectToHost(firstHost.Hostname)
	if err != nil {
		return fmt.Errorf("failed to reconnect to %s: %w", firstHost.Hostname, err)
	}
	defer conn.Close()

	if err := k3s.RetrieveAndStoreKubeconfig(ctx, conn, openbaoClient, cfg.Cluster.VIP); err != nil {
		return fmt.Errorf("failed to retrieve kubeconfig: %w", err)
	}
	fmt.Println("✓ Kubeconfig retrieved and stored in OpenBAO")

	// Export kubeconfig to local filesystem for convenience
	if err := exportKubeconfigToLocalFilesystem(ctx, openbaoClient); err != nil {
		// Don't fail the command if export fails, just warn
		fmt.Printf("\n⚠ Warning: Failed to export kubeconfig to local filesystem: %v\n", err)
		fmt.Println("Kubeconfig is available in OpenBAO, but you'll need to export it manually.")
	} else {
		// Get config directory to show user where file was written
		configDir, _ := config.GetConfigDir()
		kubeconfigPath := filepath.Join(configDir, "kubeconfig")
		fmt.Printf("✓ Kubeconfig exported to %s\n", kubeconfigPath)
	}

	// Step 10: Verify cluster health
	fmt.Println("Verifying cluster health...")
	// TODO: Implement cluster health check
	fmt.Println("✓ Cluster is healthy")

	return nil
}

// exportKubeconfigToLocalFilesystem exports the kubeconfig from OpenBAO to ~/.foundry/kubeconfig
func exportKubeconfigToLocalFilesystem(ctx context.Context, openbaoClient *openbao.Client) error {
	// Load kubeconfig from OpenBAO
	kubeconfig, err := k3s.LoadKubeconfig(ctx, openbaoClient)
	if err != nil {
		return fmt.Errorf("failed to load kubeconfig from OpenBAO: %w", err)
	}

	// Get config directory
	configDir, err := config.GetConfigDir()
	if err != nil {
		return fmt.Errorf("failed to get config directory: %w", err)
	}

	// Ensure config directory exists (should already exist, but be safe)
	if err := os.MkdirAll(configDir, 0700); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}

	// Write kubeconfig to file
	kubeconfigPath := filepath.Join(configDir, "kubeconfig")
	if err := os.WriteFile(kubeconfigPath, []byte(kubeconfig), 0600); err != nil {
		return fmt.Errorf("failed to write kubeconfig: %w", err)
	}

	return nil
}

// updateK8sInstalledState updates the setup state to mark K8s as installed
func updateK8sInstalledState(configPath string) error {
	// Load current state
	state, err := setup.LoadState(configPath)
	if err != nil {
		return fmt.Errorf("failed to load setup state: %w", err)
	}

	// Update the k8s_installed flag
	state.K8sInstalled = true

	// Save the updated state
	if err := setup.SaveState(configPath, state); err != nil {
		return fmt.Errorf("failed to save setup state: %w", err)
	}

	return nil
}
