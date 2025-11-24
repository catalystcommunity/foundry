package component

import "github.com/urfave/cli/v3"

// Command is the top-level component command
var Command = &cli.Command{
	Name:  "component",
	Usage: "Manage infrastructure components (OpenBAO, DNS, Zot, K3s, etc.)",
	Commands: []*cli.Command{
		ListCommand,
		InstallCommand,
		StatusCommand,
	},
}
