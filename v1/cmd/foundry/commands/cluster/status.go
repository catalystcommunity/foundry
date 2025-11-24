package cluster

import (
	"context"
	"fmt"

	"github.com/catalystcommunity/foundry/v1/internal/k8s"
	"github.com/catalystcommunity/foundry/v1/internal/secrets"
	"github.com/urfave/cli/v3"
)

// NewStatusCommand creates the cluster status command
func NewStatusCommand() *cli.Command {
	return &cli.Command{
		Name:  "status",
		Usage: "Show overall cluster status and health",
		Action: func(ctx context.Context, cmd *cli.Command) error {
			return runClusterStatus(ctx)
		},
	}
}

// ClusterHealth represents the overall health of the cluster
type ClusterHealth struct {
	TotalNodes         int
	ControlPlaneNodes  int
	WorkerNodes        int
	ReadyNodes         int
	NotReadyNodes      int
	Version            string
	OverallHealthy     bool
	HealthMessage      string
}

// runClusterStatus shows overall cluster health and status
func runClusterStatus(ctx context.Context) error {
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

	// Get all nodes
	nodes, err := client.GetNodes(ctx)
	if err != nil {
		return fmt.Errorf("failed to get nodes: %w", err)
	}

	// Calculate cluster health
	health := calculateClusterHealth(nodes)

	// Display cluster status
	displayClusterStatus(health)

	return nil
}

// calculateClusterHealth analyzes nodes and determines cluster health
func calculateClusterHealth(nodes []*k8s.Node) *ClusterHealth {
	health := &ClusterHealth{
		TotalNodes: len(nodes),
	}

	if len(nodes) == 0 {
		health.OverallHealthy = false
		health.HealthMessage = "No nodes found in cluster"
		return health
	}

	// Analyze each node
	for _, node := range nodes {
		// Count node roles
		for _, role := range node.Roles {
			if role == "control-plane" {
				health.ControlPlaneNodes++
			}
			if role == "worker" {
				health.WorkerNodes++
			}
		}

		// Count ready vs not ready
		if node.Status == "Ready" {
			health.ReadyNodes++
		} else {
			health.NotReadyNodes++
		}

		// Capture version from first node (should be same across cluster)
		if health.Version == "" {
			health.Version = node.Version
		}
	}

	// Determine overall health
	if health.NotReadyNodes > 0 {
		health.OverallHealthy = false
		health.HealthMessage = fmt.Sprintf("%d node(s) not ready", health.NotReadyNodes)
	} else if health.ControlPlaneNodes == 0 {
		health.OverallHealthy = false
		health.HealthMessage = "No control plane nodes found"
	} else {
		health.OverallHealthy = true
		health.HealthMessage = "All nodes ready"
	}

	return health
}

// displayClusterStatus prints the cluster status information
func displayClusterStatus(health *ClusterHealth) {
	fmt.Println("Cluster Status")
	fmt.Println("==============")
	fmt.Println()

	// Overall health indicator
	healthIndicator := "✓ Healthy"
	if !health.OverallHealthy {
		healthIndicator = "✗ Unhealthy"
	}
	fmt.Printf("Overall Health:       %s\n", healthIndicator)
	fmt.Printf("Health Message:       %s\n", health.HealthMessage)
	fmt.Println()

	// Node statistics
	fmt.Printf("Total Nodes:          %d\n", health.TotalNodes)
	fmt.Printf("Control Plane Nodes:  %d\n", health.ControlPlaneNodes)
	fmt.Printf("Worker Nodes:         %d\n", health.WorkerNodes)
	fmt.Printf("Ready Nodes:          %d\n", health.ReadyNodes)
	fmt.Printf("Not Ready Nodes:      %d\n", health.NotReadyNodes)
	fmt.Println()

	// Version info
	fmt.Printf("Kubernetes Version:   %s\n", health.Version)
}
