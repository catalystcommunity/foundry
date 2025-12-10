package cluster

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/catalystcommunity/foundry/v1/internal/config"
	"github.com/catalystcommunity/foundry/v1/internal/host"
	"github.com/catalystcommunity/foundry/v1/internal/k8s"
	"github.com/catalystcommunity/foundry/v1/internal/secrets"
	"github.com/urfave/cli/v3"
)

// NewNodeLabelCommand creates the cluster node label command
func NewNodeLabelCommand() *cli.Command {
	return &cli.Command{
		Name:      "label",
		Usage:     "Manage node labels",
		ArgsUsage: "<node> [key=value | key-]...",
		Description: `Manage Kubernetes node labels.

Examples:
  # Set labels on a node
  foundry cluster node label node1 environment=production zone=us-east-1a

  # List labels on a node
  foundry cluster node label node1 --list

  # Remove labels from a node (use key- syntax)
  foundry cluster node label node1 zone-

  # Set labels and save to config
  foundry cluster node label node1 environment=production --save`,
		Flags: []cli.Flag{
			&cli.BoolFlag{
				Name:  "list",
				Usage: "List all labels on the node",
			},
			&cli.BoolFlag{
				Name:  "save",
				Usage: "Save labels to host configuration",
			},
			&cli.BoolFlag{
				Name:  "user-only",
				Usage: "When listing, show only user-manageable labels (exclude system labels)",
			},
		},
		Action: func(ctx context.Context, cmd *cli.Command) error {
			return runNodeLabel(ctx, cmd)
		},
	}
}

// runNodeLabel executes the node label command
func runNodeLabel(ctx context.Context, cmd *cli.Command) error {
	args := cmd.Args().Slice()
	if len(args) < 1 {
		return fmt.Errorf("node name is required")
	}

	nodeName := args[0]
	labelArgs := args[1:]

	listMode := cmd.Bool("list")
	saveToConfig := cmd.Bool("save")
	userOnly := cmd.Bool("user-only")

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

	// List mode
	if listMode || len(labelArgs) == 0 {
		return listNodeLabels(ctx, client, nodeName, userOnly)
	}

	// Parse and apply labels
	setLabels, removeLabels, err := parseLabelArgs(labelArgs)
	if err != nil {
		return err
	}

	// Check for system label modifications
	for key := range setLabels {
		if k8s.IsSystemLabel(key) {
			return fmt.Errorf("cannot modify system label: %s", key)
		}
	}
	for _, key := range removeLabels {
		if k8s.IsSystemLabel(key) {
			return fmt.Errorf("cannot remove system label: %s", key)
		}
	}

	// Apply labels to set
	if len(setLabels) > 0 {
		if err := client.SetNodeLabels(ctx, nodeName, setLabels); err != nil {
			return fmt.Errorf("failed to set labels: %w", err)
		}
		for k, v := range setLabels {
			fmt.Printf("Label %s=%s set on node %s\n", k, v, nodeName)
		}
	}

	// Remove labels
	for _, key := range removeLabels {
		if err := client.RemoveNodeLabel(ctx, nodeName, key); err != nil {
			return fmt.Errorf("failed to remove label %s: %w", key, err)
		}
		fmt.Printf("Label %s removed from node %s\n", key, nodeName)
	}

	// Save to config if requested
	if saveToConfig {
		if err := saveLabelsToConfig(nodeName, setLabels, removeLabels); err != nil {
			return fmt.Errorf("failed to save labels to config: %w", err)
		}
		fmt.Println("Labels saved to configuration")
	}

	return nil
}

// listNodeLabels lists all labels on a node
func listNodeLabels(ctx context.Context, client *k8s.Client, nodeName string, userOnly bool) error {
	labels, err := client.GetNodeLabels(ctx, nodeName)
	if err != nil {
		return fmt.Errorf("failed to get labels: %w", err)
	}

	if userOnly {
		labels = k8s.FilterUserLabels(labels)
	}

	if len(labels) == 0 {
		if userOnly {
			fmt.Printf("No user labels found on node %s\n", nodeName)
		} else {
			fmt.Printf("No labels found on node %s\n", nodeName)
		}
		return nil
	}

	// Sort keys for consistent output
	keys := make([]string, 0, len(labels))
	for k := range labels {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	fmt.Printf("Labels on node %s:\n", nodeName)
	for _, k := range keys {
		v := labels[k]
		if v == "" {
			fmt.Printf("  %s\n", k)
		} else {
			fmt.Printf("  %s=%s\n", k, v)
		}
	}

	return nil
}

// parseLabelArgs parses label arguments into set and remove operations
// Format: key=value to set, key- to remove
func parseLabelArgs(args []string) (set map[string]string, remove []string, err error) {
	set = make(map[string]string)
	remove = []string{}

	for _, arg := range args {
		// Check for removal syntax (key-)
		if strings.HasSuffix(arg, "-") && !strings.Contains(arg, "=") {
			key := strings.TrimSuffix(arg, "-")
			if key == "" {
				return nil, nil, fmt.Errorf("invalid label removal syntax: %s", arg)
			}
			if err := host.ValidateLabelKey(key); err != nil {
				return nil, nil, fmt.Errorf("invalid label key %q: %w", key, err)
			}
			remove = append(remove, key)
			continue
		}

		// Parse key=value syntax
		parts := strings.SplitN(arg, "=", 2)
		if len(parts) != 2 {
			return nil, nil, fmt.Errorf("invalid label format: %s (use key=value or key-)", arg)
		}

		key := parts[0]
		value := parts[1]

		if err := host.ValidateLabelKey(key); err != nil {
			return nil, nil, fmt.Errorf("invalid label key %q: %w", key, err)
		}
		if err := host.ValidateLabelValue(value); err != nil {
			return nil, nil, fmt.Errorf("invalid label value for key %q: %w", key, err)
		}

		set[key] = value
	}

	return set, remove, nil
}

// saveLabelsToConfig saves label changes to the host configuration
func saveLabelsToConfig(nodeName string, setLabels map[string]string, removeLabels []string) error {
	// Load config from default path
	configPath := config.DefaultConfigPath()
	cfg, err := config.Load(configPath)
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	// Find the host
	var targetHost *host.Host
	for _, h := range cfg.Hosts {
		if h.Hostname == nodeName {
			targetHost = h
			break
		}
	}

	if targetHost == nil {
		return fmt.Errorf("host %s not found in configuration", nodeName)
	}

	// Apply label changes
	for k, v := range setLabels {
		targetHost.SetLabel(k, v)
	}
	for _, k := range removeLabels {
		targetHost.RemoveLabel(k)
	}

	// Save config
	if err := config.Save(cfg, configPath); err != nil {
		return fmt.Errorf("failed to save config: %w", err)
	}

	return nil
}
