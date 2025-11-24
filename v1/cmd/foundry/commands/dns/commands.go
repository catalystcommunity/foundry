package dns

import "github.com/urfave/cli/v3"

// Command is the top-level dns command
var Command = &cli.Command{
	Name:  "dns",
	Usage: "Manage DNS zones and records",
	Description: `DNS zone and record management commands for PowerDNS.

These commands allow you to manage DNS zones and records in your PowerDNS
installation. PowerDNS must be installed and configured before using these
commands.

Typical workflow:
  1. foundry dns zone create <zone>     - Create a new DNS zone
  2. foundry dns record add <zone> ...  - Add records to the zone
  3. foundry dns test <hostname>        - Test DNS resolution
  4. foundry dns zone list               - List all zones
  5. foundry dns record list <zone>     - List records in a zone`,
	Commands: []*cli.Command{
		ZoneCommand,
		RecordCommand,
		TestCommand,
	},
}
