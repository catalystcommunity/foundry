package host

import "github.com/urfave/cli/v3"

// Command is the top-level host command
var Command = &cli.Command{
	Name:  "host",
	Usage: "Manage infrastructure hosts",
	Commands: []*cli.Command{
		AddCommand,
		ListCommand,
		ConfigureCommand,
		SyncKeysCommand,
		MigrateKeysCommand,
	},
}
