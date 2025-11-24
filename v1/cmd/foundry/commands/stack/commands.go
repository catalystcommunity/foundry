package stack

import "github.com/urfave/cli/v3"

// Command is the top-level stack command
var Command = &cli.Command{
	Name:  "stack",
	Usage: "Manage the complete infrastructure stack",
	Commands: []*cli.Command{
		TemplateCommand,
		InstallCommand,
		StatusCommand,
		ValidateCommand,
	},
}
