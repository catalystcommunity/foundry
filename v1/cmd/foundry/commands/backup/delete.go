package backup

import (
	"context"
	"fmt"

	"github.com/urfave/cli/v3"
)

// DeleteCommand deletes a backup
var DeleteCommand = &cli.Command{
	Name:      "delete",
	Usage:     "Delete a cluster backup",
	ArgsUsage: "<backup-name>",
	Description: `Deletes a Velero backup.

This will remove the backup and its associated data from the storage location.

Examples:
  foundry backup delete my-backup`,
	Flags: []cli.Flag{
		&cli.BoolFlag{
			Name:    "force",
			Aliases: []string{"f"},
			Usage:   "Skip confirmation prompt",
		},
	},
	Action: runDelete,
}

func runDelete(ctx context.Context, cmd *cli.Command) error {
	// Get backup name
	name := cmd.Args().Get(0)
	if name == "" {
		return fmt.Errorf("backup name is required\n\nUsage: foundry backup delete <backup-name>\n\nTo see available backups, run:\n  foundry backup list")
	}

	// Create Velero client
	client, err := NewVeleroClient()
	if err != nil {
		return err
	}

	// Verify backup exists
	_, err = client.GetBackup(ctx, name)
	if err != nil {
		return fmt.Errorf("backup %q not found: %w", name, err)
	}

	// Confirm unless --force
	if !cmd.Bool("force") {
		fmt.Printf("Are you sure you want to delete backup %q? This cannot be undone.\n", name)
		fmt.Print("Type 'yes' to confirm: ")
		var response string
		fmt.Scanln(&response)
		if response != "yes" {
			fmt.Println("Aborted")
			return nil
		}
	}

	// Delete backup
	fmt.Printf("Deleting backup %q...\n", name)
	if err := client.DeleteBackup(ctx, name); err != nil {
		return fmt.Errorf("failed to delete backup: %w", err)
	}

	fmt.Printf("Backup %q deleted\n", name)
	return nil
}
