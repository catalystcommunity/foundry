package openbao

import "github.com/urfave/cli/v3"

// Command is the top-level openbao command
var Command = &cli.Command{
	Name:  "openbao",
	Usage: "Manage OpenBAO secret management system",
	Description: `OpenBAO management commands.

These commands allow you to manage OpenBAO, including unsealing after a restart.

Commands:
  foundry openbao unseal        - Unseal OpenBAO after a restart
  foundry component status openbao - Check OpenBAO status`,
	Commands: []*cli.Command{
		UnsealCommand,
	},
}
