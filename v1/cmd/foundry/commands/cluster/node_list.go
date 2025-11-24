package cluster

import (
	"context"
	"fmt"
	"strings"

	"github.com/catalystcommunity/foundry/v1/internal/k8s"
	"github.com/catalystcommunity/foundry/v1/internal/secrets"
	"github.com/urfave/cli/v3"
)

// NewNodeListCommand creates the cluster node list command
func NewNodeListCommand() *cli.Command {
	return &cli.Command{
		Name:  "list",
		Usage: "List all cluster nodes",
		Action: func(ctx context.Context, cmd *cli.Command) error {
			return runNodeList(ctx)
		},
	}
}

// runNodeList lists all nodes in the cluster
func runNodeList(ctx context.Context) error {
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

	if len(nodes) == 0 {
		fmt.Println("No nodes found in cluster")
		return nil
	}

	// Print table header
	fmt.Printf("%-30s %-20s %-15s %-15s\n", "NAME", "ROLES", "STATUS", "VERSION")
	fmt.Println(strings.Repeat("-", 80))

	// Print node information
	for _, node := range nodes {
		roles := strings.Join(node.Roles, ",")
		fmt.Printf("%-30s %-20s %-15s %-15s\n",
			node.Name,
			roles,
			node.Status,
			node.Version,
		)
	}

	return nil
}
