package config

import "github.com/urfave/cli/v3"

// Command is the top-level config command
var Command = &cli.Command{
	Name:  "config",
	Usage: "Manage configuration files",
	Commands: []*cli.Command{
		InitCommand,
		ValidateCommand,
		ShowCommand,
		ListCommand,
	},
}
