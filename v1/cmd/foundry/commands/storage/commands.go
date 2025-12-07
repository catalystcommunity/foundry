package storage

import "github.com/urfave/cli/v3"

// Command is the top-level storage command
var Command = &cli.Command{
	Name:  "storage",
	Usage: "Manage storage backend and PVC operations",
	Description: `Storage backend configuration and PVC management commands.

These commands help you manage storage backends and Persistent Volume Claims
in your Kubernetes cluster.

Storage Management:
  foundry storage list        - Show configured storage backends

PVC Management:
  foundry storage provision   - Create a new PVC
  foundry storage pvc list    - List PVCs
  foundry storage pvc delete  - Delete a PVC`,
	Commands: []*cli.Command{
		ListCommand,
		ProvisionCommand,
		PVCCommand,
	},
}
