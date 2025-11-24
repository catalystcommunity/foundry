package storage

import "github.com/urfave/cli/v3"

// Command is the top-level storage command
var Command = &cli.Command{
	Name:  "storage",
	Usage: "Manage storage backend configuration",
	Description: `Storage backend configuration and management commands.

These commands help you configure and manage storage backends like TrueNAS
for use with Foundry components (Zot registry, K8s persistent volumes, etc.).

Typical workflow:
  1. foundry storage configure   - Interactive TrueNAS setup
  2. foundry storage test         - Verify connectivity
  3. foundry storage list         - Show configured storage backends`,
	Commands: []*cli.Command{
		ConfigureCommand,
		ListCommand,
		TestCommand,
	},
}
