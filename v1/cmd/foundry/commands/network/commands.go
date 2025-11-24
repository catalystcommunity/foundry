package network

import "github.com/urfave/cli/v3"

// Command is the top-level network command
var Command = &cli.Command{
	Name:  "network",
	Usage: "Manage network configuration",
	Description: `Network configuration and planning commands.

These commands help you plan, configure, and validate your network setup
before installing infrastructure components.

Typical workflow:
  1. foundry network plan          - Interactive network planning wizard
  2. Configure static IPs or DHCP reservations in your router/DHCP server
     (This is YOUR responsibility - Foundry doesn't manage host IPs)
  3. foundry network validate      - Verify network configuration`,
	Commands: []*cli.Command{
		PlanCommand,
		ValidateCommand,
	},
}
