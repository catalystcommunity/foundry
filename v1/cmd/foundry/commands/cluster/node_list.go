package cluster

import (
	"context"
	"fmt"
	"sort"
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
		Flags: []cli.Flag{
			&cli.BoolFlag{
				Name:  "show-labels",
				Usage: "Show node labels in output",
			},
			&cli.BoolFlag{
				Name:  "user-labels-only",
				Usage: "When showing labels, exclude system labels (kubernetes.io/*, etc.)",
			},
		},
		Action: func(ctx context.Context, cmd *cli.Command) error {
			return runNodeList(ctx, cmd.Bool("show-labels"), cmd.Bool("user-labels-only"))
		},
	}
}

// runNodeList lists all nodes in the cluster
func runNodeList(ctx context.Context, showLabels, userLabelsOnly bool) error {
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
	if showLabels {
		fmt.Printf("%-30s %-20s %-10s %-12s %s\n", "NAME", "ROLES", "STATUS", "VERSION", "LABELS")
		fmt.Println(strings.Repeat("-", 100))
	} else {
		fmt.Printf("%-30s %-20s %-15s %-15s\n", "NAME", "ROLES", "STATUS", "VERSION")
		fmt.Println(strings.Repeat("-", 80))
	}

	// Print node information
	for _, node := range nodes {
		roles := strings.Join(node.Roles, ",")

		if showLabels {
			labels := node.Labels
			if userLabelsOnly {
				labels = k8s.FilterUserLabels(labels)
			}
			labelStr := formatLabels(labels)
			fmt.Printf("%-30s %-20s %-10s %-12s %s\n",
				node.Name,
				roles,
				node.Status,
				node.Version,
				labelStr,
			)
		} else {
			fmt.Printf("%-30s %-20s %-15s %-15s\n",
				node.Name,
				roles,
				node.Status,
				node.Version,
			)
		}
	}

	return nil
}

// formatLabels formats a label map as a comma-separated string
func formatLabels(labels map[string]string) string {
	if len(labels) == 0 {
		return "<none>"
	}

	// Sort keys for consistent output
	keys := make([]string, 0, len(labels))
	for k := range labels {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	parts := make([]string, 0, len(labels))
	for _, k := range keys {
		v := labels[k]
		if v == "" {
			parts = append(parts, k)
		} else {
			parts = append(parts, fmt.Sprintf("%s=%s", k, v))
		}
	}

	return strings.Join(parts, ",")
}
