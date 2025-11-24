package cluster

import (
	"context"
	"fmt"

	"github.com/catalystcommunity/foundry/v1/internal/component/k3s"
	"github.com/catalystcommunity/foundry/v1/internal/component/openbao"
	"github.com/catalystcommunity/foundry/v1/internal/config"
	"github.com/catalystcommunity/foundry/v1/internal/host"
	"github.com/urfave/cli/v3"
)

// NewNodeAddCommand creates the 'cluster node add' command
func NewNodeAddCommand() *cli.Command {
	return &cli.Command{
		Name:      "add",
		Usage:     "Add a node to the K3s cluster",
		ArgsUsage: "<hostname>",
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:  "role",
				Usage: "Node role: control-plane, worker, or both (default: auto)",
			},
			&cli.StringFlag{
				Name:    "config",
				Aliases: []string{"c"},
				Usage:   "Path to configuration file",
			},
			&cli.BoolFlag{
				Name:  "dry-run",
				Usage: "Show what would be done without making changes",
			},
		},
		Action: runNodeAdd,
	}
}

func runNodeAdd(ctx context.Context, cmd *cli.Command) error {
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

	// Determine node role
	explicitRole := cmd.String("role")
	nodeRole, err := determineNodeRole(cfg, explicitRole)
	if err != nil {
		return fmt.Errorf("failed to determine node role: %w", err)
	}

	if cmd.Bool("dry-run") {
		printNodeAddPlan(hostname, nodeRole, cfg)
		return nil
	}

	// Add node to cluster
	fmt.Printf("Adding node %s to cluster %s...\n", hostname, cfg.Cluster.Name)
	if err := addNodeToCluster(ctx, hostname, nodeRole, cfg); err != nil {
		return fmt.Errorf("failed to add node: %w", err)
	}

	fmt.Printf("✓ Node %s successfully added to cluster\n", hostname)
	return nil
}

// determineNodeRole determines what role the new node should have
func determineNodeRole(cfg *config.Config, explicitRole string) (*k3s.DeterminedRole, error) {
	// If user specified explicit role, use it
	if explicitRole != "" {
		var role k3s.NodeRole
		var isCP, isWorker bool

		switch explicitRole {
		case "control-plane":
			role = k3s.RoleControlPlane
			isCP = true
			isWorker = false
		case "worker":
			role = k3s.RoleWorker
			isCP = false
			isWorker = true
		case "both":
			role = k3s.RoleBoth
			isCP = true
			isWorker = true
		default:
			return nil, fmt.Errorf("invalid role %q: must be control-plane, worker, or both", explicitRole)
		}

		return &k3s.DeterminedRole{
			Role:           role,
			IsControlPlane: isCP,
			IsWorker:       isWorker,
			ExplicitlySet:  true,
		}, nil
	}

	// Auto-determine based on existing cluster
	// Count existing control plane nodes
	controlPlaneCount := 0
	clusterHosts := cfg.GetClusterHosts()
	for _, h := range clusterHosts {
		// Check if host has cluster-control-plane role
		for _, role := range h.Roles {
			if role == "cluster-control-plane" {
				controlPlaneCount++
				break
			}
		}
	}

	// Default: if we have < 3 control planes, add as both, otherwise worker only
	if controlPlaneCount < 3 {
		return &k3s.DeterminedRole{
			Role:           k3s.RoleBoth,
			IsControlPlane: true,
			IsWorker:       true,
			ExplicitlySet:  false,
		}, nil
	}

	return &k3s.DeterminedRole{
		Role:           k3s.RoleWorker,
		IsControlPlane: false,
		IsWorker:       true,
		ExplicitlySet:  false,
	}, nil
}

func printNodeAddPlan(hostname string, role *k3s.DeterminedRole, cfg *config.Config) {
	fmt.Printf("\nNode addition plan:\n")
	fmt.Printf("  Node: %s\n", hostname)

	roleStr := string(role.Role)
	if role.Role == k3s.RoleBoth {
		roleStr = "both (control-plane + worker)"
	}
	fmt.Printf("  Role: %s\n", roleStr)
	fmt.Printf("  Cluster: %s\n", cfg.Cluster.Name)
	fmt.Printf("  VIP: %s\n", cfg.Cluster.VIP)

	fmt.Printf("\nSteps:\n")
	fmt.Printf("  1. Connect to %s via SSH\n", hostname)
	fmt.Printf("  2. Load K3s tokens from OpenBAO\n")
	if role.IsControlPlane {
		fmt.Printf("  3. Join as control plane (server mode)\n")
	} else {
		fmt.Printf("  3. Join as worker (agent mode)\n")
	}
	fmt.Printf("  4. Verify node appears in cluster\n")
}

// addNodeToCluster performs the actual node addition
func addNodeToCluster(ctx context.Context, hostname string, nodeRole *k3s.DeterminedRole, cfg *config.Config) error {
	// Step 1: Get OpenBAO client
	fmt.Println("Connecting to OpenBAO...")
	openbaoIP, err := cfg.GetPrimaryOpenBAOAddress()
	if err != nil {
		return fmt.Errorf("failed to get OpenBAO address: %w", err)
	}
	openbaoAddr := fmt.Sprintf("http://%s:8200", openbaoIP)
	// TODO: Get token from auth management
	openbaoClient := openbao.NewClient(openbaoAddr, "")

	// Step 2: Load tokens from OpenBAO
	fmt.Println("Loading K3s tokens from OpenBAO...")
	tokens, err := k3s.LoadTokens(ctx, openbaoClient)
	if err != nil {
		return fmt.Errorf("failed to load tokens: %w", err)
	}

	// Step 3: Connect to host via SSH
	fmt.Printf("Connecting to %s via SSH...\n", hostname)
	conn, err := connectToHost(hostname)
	if err != nil {
		return fmt.Errorf("failed to connect to host: %w", err)
	}
	defer conn.Close()

	// Step 4: Build K3s config
	serverURL := fmt.Sprintf("https://%s:6443", cfg.Cluster.VIP)
	k3sConfig := &k3s.Config{
		ClusterInit:  false,
		ServerURL:    serverURL,
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

	// Step 5: Join node based on role
	if nodeRole.IsControlPlane {
		fmt.Printf("Joining %s as control plane node...\n", hostname)
		if err := k3s.JoinControlPlane(ctx, conn, serverURL, tokens, k3sConfig); err != nil {
			return fmt.Errorf("failed to join control plane: %w", err)
		}
	} else {
		fmt.Printf("Joining %s as worker node...\n", hostname)
		if err := k3s.JoinWorker(ctx, conn, serverURL, tokens, k3sConfig); err != nil {
			return fmt.Errorf("failed to join worker: %w", err)
		}
	}

	return nil
}
