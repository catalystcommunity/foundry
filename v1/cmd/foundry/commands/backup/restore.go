package backup

import (
	"context"
	"fmt"
	"time"

	"github.com/urfave/cli/v3"
)

// RestoreCommand restores from a backup
var RestoreCommand = &cli.Command{
	Name:      "restore",
	Usage:     "Restore from a cluster backup",
	ArgsUsage: "<backup-name>",
	Description: `Restores cluster resources from a Velero backup.

The backup name is required. You can see available backups with 'foundry backup list'.

Examples:
  foundry backup restore my-backup                    # Restore from backup
  foundry backup restore my-backup --namespace app   # Restore specific namespace
  foundry backup restore my-backup --wait            # Wait for restore to complete`,
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:  "name",
			Usage: "Name for the restore operation (auto-generated if not specified)",
		},
		&cli.StringSliceFlag{
			Name:    "namespace",
			Aliases: []string{"n"},
			Usage:   "Namespaces to restore (can be specified multiple times)",
		},
		&cli.StringSliceFlag{
			Name:  "exclude-namespace",
			Usage: "Namespaces to exclude from restore (can be specified multiple times)",
		},
		&cli.BoolFlag{
			Name:  "restore-pvs",
			Usage: "Restore PersistentVolumes from snapshots",
			Value: true,
		},
		&cli.BoolFlag{
			Name:  "wait",
			Usage: "Wait for restore to complete",
			Value: false,
		},
		&cli.DurationFlag{
			Name:  "timeout",
			Usage: "Timeout when waiting for restore (used with --wait)",
			Value: 30 * time.Minute,
		},
	},
	Action: runRestore,
}

func runRestore(ctx context.Context, cmd *cli.Command) error {
	// Get backup name
	backupName := cmd.Args().Get(0)
	if backupName == "" {
		return fmt.Errorf("backup name is required\n\nUsage: foundry backup restore <backup-name>\n\nTo see available backups, run:\n  foundry backup list")
	}

	// Create Velero client
	client, err := NewVeleroClient()
	if err != nil {
		return err
	}

	// Verify backup exists
	backup, err := client.GetBackup(ctx, backupName)
	if err != nil {
		return fmt.Errorf("backup %q not found: %w\n\nTo see available backups, run:\n  foundry backup list", backupName, err)
	}

	if backup.Status != "Completed" {
		return fmt.Errorf("backup %q is not in Completed state (current: %s)", backupName, backup.Status)
	}

	// Get or generate restore name
	restoreName := cmd.String("name")
	if restoreName == "" {
		restoreName = fmt.Sprintf("restore-%s-%s", backupName, time.Now().Format("20060102-150405"))
	}

	// Build restore options
	restorePVs := cmd.Bool("restore-pvs")
	opts := RestoreOptions{
		IncludedNamespaces: cmd.StringSlice("namespace"),
		ExcludedNamespaces: cmd.StringSlice("exclude-namespace"),
		RestorePVs:         &restorePVs,
	}

	// Create restore
	fmt.Printf("Creating restore %q from backup %q...\n", restoreName, backupName)
	if err := client.CreateRestore(ctx, restoreName, backupName, opts); err != nil {
		return fmt.Errorf("failed to create restore: %w", err)
	}

	fmt.Printf("Restore %q created\n", restoreName)

	// Wait for restore if requested
	if cmd.Bool("wait") {
		fmt.Println("Waiting for restore to complete...")
		timeout := cmd.Duration("timeout")
		if err := waitForRestore(ctx, client, restoreName, timeout); err != nil {
			return err
		}
	} else {
		fmt.Println("\nTo check restore status, run:")
		fmt.Printf("  foundry backup list --restores\n")
	}

	return nil
}

// waitForRestore waits for a restore to complete
func waitForRestore(ctx context.Context, client *VeleroClient, name string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			if time.Now().After(deadline) {
				return fmt.Errorf("timeout waiting for restore to complete")
			}

			restore, err := client.GetRestore(ctx, name)
			if err != nil {
				return fmt.Errorf("failed to get restore status: %w", err)
			}

			switch restore.Status {
			case "Completed":
				fmt.Printf("\nRestore %q completed successfully\n", name)
				if restore.ItemsRestored > 0 {
					fmt.Printf("  Items restored: %d\n", restore.ItemsRestored)
				}
				if restore.Warnings > 0 {
					fmt.Printf("  Warnings: %d\n", restore.Warnings)
				}
				return nil
			case "Failed", "PartiallyFailed":
				fmt.Printf("\nRestore %q %s\n", name, restore.Status)
				if restore.Errors > 0 {
					fmt.Printf("  Errors: %d\n", restore.Errors)
				}
				return fmt.Errorf("restore failed with status: %s", restore.Status)
			case "InProgress", "New":
				// Still running, continue waiting
				if restore.ItemsRestored > 0 {
					fmt.Printf("\r  Items restored: %d", restore.ItemsRestored)
				} else {
					fmt.Printf("\r  Status: %s", restore.Status)
				}
			default:
				fmt.Printf("\r  Status: %s", restore.Status)
			}
		}
	}
}
