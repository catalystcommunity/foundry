package backup

import "github.com/urfave/cli/v3"

// Command is the top-level backup command
var Command = &cli.Command{
	Name:  "backup",
	Usage: "Manage cluster backups with Velero",
	Description: `Backup and restore commands for Kubernetes cluster data.

These commands use Velero to create, manage, and restore cluster backups.
Velero must be installed before using these commands.

Typical workflow:
  1. foundry backup create               - Create a backup
  2. foundry backup list                 - View available backups
  3. foundry backup restore <name>       - Restore from a backup
  4. foundry backup schedule             - Configure scheduled backups`,
	Commands: []*cli.Command{
		CreateCommand,
		ListCommand,
		RestoreCommand,
		ScheduleCommand,
		DeleteCommand,
	},
}
