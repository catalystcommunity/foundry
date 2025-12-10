package cluster

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/catalystcommunity/foundry/v1/internal/component/k3s"
	"github.com/catalystcommunity/foundry/v1/internal/component/openbao"
	"github.com/catalystcommunity/foundry/v1/internal/config"
	"github.com/catalystcommunity/foundry/v1/internal/host"
	"github.com/catalystcommunity/foundry/v1/internal/k8s"
	"github.com/catalystcommunity/foundry/v1/internal/secrets"
	"github.com/urfave/cli/v3"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
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
			&cli.StringSliceFlag{
				Name:  "labels",
				Usage: "Node labels in key=value format (can be specified multiple times)",
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

	// Parse labels from --labels flag
	labelArgs := cmd.StringSlice("labels")
	labels, err := parseNodeAddLabels(labelArgs)
	if err != nil {
		return fmt.Errorf("invalid labels: %w", err)
	}

	if cmd.Bool("dry-run") {
		printNodeAddPlan(hostname, nodeRole, cfg, labels)
		return nil
	}

	// Add node to cluster
	fmt.Printf("Adding node %s to cluster %s...\n", hostname, cfg.Cluster.Name)
	if err := addNodeToCluster(ctx, hostname, nodeRole, cfg); err != nil {
		return fmt.Errorf("failed to add node: %w", err)
	}

	// Apply labels if specified
	if len(labels) > 0 {
		fmt.Printf("Applying labels to node %s...\n", hostname)
		if err := applyNodeLabelsAfterJoin(ctx, hostname, labels); err != nil {
			// Don't fail the whole operation, just warn
			fmt.Printf("Warning: failed to apply labels: %v\n", err)
			fmt.Println("You can apply labels manually with: foundry cluster node label", hostname)
		} else {
			fmt.Printf("Labels applied: %v\n", labels)
		}
	}

	// Update Longhorn replica count if needed (after node successfully joins)
	if err := maybeUpdateLonghornReplicaCount(ctx, cfg); err != nil {
		// Don't fail the whole operation, just warn
		fmt.Printf("Warning: failed to update Longhorn replica count: %v\n", err)
	}

	fmt.Printf("Node %s successfully added to cluster\n", hostname)
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

func printNodeAddPlan(hostname string, role *k3s.DeterminedRole, cfg *config.Config, labels map[string]string) {
	fmt.Printf("\nNode addition plan:\n")
	fmt.Printf("  Node: %s\n", hostname)

	roleStr := string(role.Role)
	if role.Role == k3s.RoleBoth {
		roleStr = "both (control-plane + worker)"
	}
	fmt.Printf("  Role: %s\n", roleStr)
	fmt.Printf("  Cluster: %s\n", cfg.Cluster.Name)
	fmt.Printf("  VIP: %s\n", cfg.Cluster.VIP)

	if len(labels) > 0 {
		fmt.Printf("  Labels:\n")
		for k, v := range labels {
			fmt.Printf("    %s=%s\n", k, v)
		}
	}

	fmt.Printf("\nSteps:\n")
	fmt.Printf("  1. Connect to %s via SSH\n", hostname)
	fmt.Printf("  2. Load K3s tokens from OpenBAO\n")
	if role.IsControlPlane {
		fmt.Printf("  3. Join as control plane (server mode)\n")
	} else {
		fmt.Printf("  3. Join as worker (agent mode)\n")
	}
	fmt.Printf("  4. Verify node appears in cluster\n")
	if len(labels) > 0 {
		fmt.Printf("  5. Apply node labels\n")
	}
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

	// Load OpenBAO token from keys.json file
	configDir, err := config.GetConfigDir()
	if err != nil {
		return fmt.Errorf("failed to get config directory: %w", err)
	}
	openbaoKeysDir := filepath.Join(configDir, "openbao-keys")
	keyMaterial, err := openbao.LoadKeyMaterial(openbaoKeysDir, cfg.Cluster.Name)
	if err != nil {
		return fmt.Errorf("failed to load OpenBAO keys: %w (ensure OpenBAO was initialized)", err)
	}
	openbaoClient := openbao.NewClient(openbaoAddr, keyMaterial.RootToken)

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

// parseNodeAddLabels parses label arguments from the --labels flag
func parseNodeAddLabels(labelArgs []string) (map[string]string, error) {
	labels := make(map[string]string)

	for _, arg := range labelArgs {
		parts := strings.SplitN(arg, "=", 2)
		if len(parts) != 2 {
			return nil, fmt.Errorf("invalid label format: %s (use key=value)", arg)
		}

		key := parts[0]
		value := parts[1]

		if err := host.ValidateLabelKey(key); err != nil {
			return nil, fmt.Errorf("invalid label key %q: %w", key, err)
		}
		if err := host.ValidateLabelValue(value); err != nil {
			return nil, fmt.Errorf("invalid label value for key %q: %w", key, err)
		}

		// Check for system label
		if k8s.IsSystemLabel(key) {
			return nil, fmt.Errorf("cannot set system label: %s", key)
		}

		labels[key] = value
	}

	return labels, nil
}

// applyNodeLabelsAfterJoin applies labels to a node after it joins the cluster
// It waits briefly for the node to appear in the API before applying labels
func applyNodeLabelsAfterJoin(ctx context.Context, nodeName string, labels map[string]string) error {
	// Create OpenBAO resolver to get kubeconfig
	resolver, err := secrets.NewOpenBAOResolver("", "")
	if err != nil {
		return fmt.Errorf("failed to create OpenBAO resolver: %w", err)
	}

	// Create K8s client from kubeconfig in OpenBAO
	client, err := k8s.NewClientFromOpenBAO(ctx, resolver, "foundry-core/k3s/kubeconfig", "value")
	if err != nil {
		return fmt.Errorf("failed to create Kubernetes client: %w", err)
	}

	// Wait for node to appear in cluster (with timeout)
	deadline := time.Now().Add(60 * time.Second)
	for time.Now().Before(deadline) {
		_, err := client.GetNodeLabels(ctx, nodeName)
		if err == nil {
			break // Node found
		}
		time.Sleep(2 * time.Second)
	}

	// Apply labels
	return client.SetNodeLabels(ctx, nodeName, labels)
}

// maybeUpdateLonghornReplicaCount checks if Longhorn is installed and updates
// the default replica count based on current node count (max 3)
func maybeUpdateLonghornReplicaCount(ctx context.Context, cfg *config.Config) error {
	// Create K8s client
	resolver, err := secrets.NewOpenBAOResolver("", "")
	if err != nil {
		return nil // No OpenBAO, likely Longhorn not installed yet
	}

	client, err := k8s.NewClientFromOpenBAO(ctx, resolver, "foundry-core/k3s/kubeconfig", "value")
	if err != nil {
		return nil // Can't connect, skip silently
	}

	// Check if Longhorn namespace exists
	_, err = client.GetNamespace(ctx, "longhorn-system")
	if err != nil {
		return nil // Longhorn not installed, nothing to update
	}

	// Get current node count
	nodes, err := client.GetNodes(ctx)
	if err != nil {
		return fmt.Errorf("failed to get nodes: %w", err)
	}

	nodeCount := len(nodes)
	optimalReplicas := nodeCount
	if optimalReplicas > 3 {
		optimalReplicas = 3
	}
	if optimalReplicas < 1 {
		optimalReplicas = 1
	}

	// Update Longhorn setting via Setting CRD
	return updateLonghornReplicaSetting(ctx, client, optimalReplicas)
}

// updateLonghornReplicaSetting updates the default-replica-count Setting in Longhorn
func updateLonghornReplicaSetting(ctx context.Context, client *k8s.Client, replicaCount int) error {
	// Use dynamic client to patch the Setting resource
	dynamicClient := client.DynamicClient()

	settingGVR := schema.GroupVersionResource{
		Group:    "longhorn.io",
		Version:  "v1beta2",
		Resource: "settings",
	}

	// Get current setting
	setting, err := dynamicClient.Resource(settingGVR).Namespace("longhorn-system").Get(ctx, "default-replica-count", metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("failed to get Longhorn setting: %w", err)
	}

	// Get current value
	currentValue, found, err := unstructured.NestedString(setting.Object, "value")
	if err != nil || !found {
		currentValue = "3" // Default
	}

	// Parse current value
	currentReplicas := 3
	if _, err := fmt.Sscanf(currentValue, "%d", &currentReplicas); err != nil {
		currentReplicas = 3
	}

	// Only update if we can increase replicas (don't decrease)
	if replicaCount <= currentReplicas {
		return nil // Already at or above optimal
	}

	// Update the setting
	if err := unstructured.SetNestedField(setting.Object, fmt.Sprintf("%d", replicaCount), "value"); err != nil {
		return fmt.Errorf("failed to set replica count value: %w", err)
	}

	_, err = dynamicClient.Resource(settingGVR).Namespace("longhorn-system").Update(ctx, setting, metav1.UpdateOptions{})
	if err != nil {
		return fmt.Errorf("failed to update Longhorn setting: %w", err)
	}

	fmt.Printf("Updated Longhorn default replica count from %d to %d\n", currentReplicas, replicaCount)
	return nil
}
