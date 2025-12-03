package backup

import (
	"context"
	"fmt"
	"time"

	"github.com/urfave/cli/v3"
)

// CreateCommand creates a new cluster backup
var CreateCommand = &cli.Command{
	Name:      "create",
	Usage:     "Create a new cluster backup",
	ArgsUsage: "[name]",
	Description: `Creates a new Velero backup of the cluster.

If no name is provided, a name will be generated based on the current timestamp.

Examples:
  foundry backup create                        # Auto-generated name
  foundry backup create my-backup              # Named backup
  foundry backup create --namespace default    # Backup specific namespace
  foundry backup create --ttl 168h             # Backup with 7-day retention`,
	Flags: []cli.Flag{
		&cli.StringSliceFlag{
			Name:    "namespace",
			Aliases: []string{"n"},
			Usage:   "Namespaces to include in backup (can be specified multiple times)",
		},
		&cli.StringSliceFlag{
			Name:  "exclude-namespace",
			Usage: "Namespaces to exclude from backup (can be specified multiple times)",
		},
		&cli.StringFlag{
			Name:  "ttl",
			Usage: "Backup retention period (e.g., 720h for 30 days)",
			Value: "720h",
		},
		&cli.BoolFlag{
			Name:  "snapshot-volumes",
			Usage: "Take snapshots of PersistentVolumes (requires CSI support)",
			Value: false,
		},
		&cli.StringFlag{
			Name:  "storage-location",
			Usage: "Backup storage location to use",
			Value: "default",
		},
		&cli.BoolFlag{
			Name:  "wait",
			Usage: "Wait for backup to complete",
			Value: false,
		},
		&cli.DurationFlag{
			Name:  "timeout",
			Usage: "Timeout when waiting for backup (used with --wait)",
			Value: 30 * time.Minute,
		},
	},
	Action: runCreate,
}

func runCreate(ctx context.Context, cmd *cli.Command) error {
	// Get backup name
	name := cmd.Args().Get(0)
	if name == "" {
		// Generate name based on timestamp
		name = fmt.Sprintf("backup-%s", time.Now().Format("20060102-150405"))
	}

	// Create Velero client
	client, err := NewVeleroClient()
	if err != nil {
		return err
	}

	// Build backup options
	snapshotVolumes := cmd.Bool("snapshot-volumes")
	opts := BackupOptions{
		IncludedNamespaces: cmd.StringSlice("namespace"),
		ExcludedNamespaces: cmd.StringSlice("exclude-namespace"),
		TTL:                cmd.String("ttl"),
		SnapshotVolumes:    &snapshotVolumes,
		StorageLocation:    cmd.String("storage-location"),
	}

	// Create backup
	fmt.Printf("Creating backup %q...\n", name)
	if err := client.CreateBackup(ctx, name, opts); err != nil {
		return fmt.Errorf("failed to create backup: %w", err)
	}

	fmt.Printf("Backup %q created\n", name)

	// Wait for backup if requested
	if cmd.Bool("wait") {
		fmt.Println("Waiting for backup to complete...")
		timeout := cmd.Duration("timeout")
		if err := waitForBackup(ctx, client, name, timeout); err != nil {
			return err
		}
	} else {
		fmt.Println("\nTo check backup status, run:")
		fmt.Printf("  foundry backup list\n")
	}

	return nil
}

// waitForBackup waits for a backup to complete
func waitForBackup(ctx context.Context, client *VeleroClient, name string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			if time.Now().After(deadline) {
				return fmt.Errorf("timeout waiting for backup to complete")
			}

			backup, err := client.GetBackup(ctx, name)
			if err != nil {
				return fmt.Errorf("failed to get backup status: %w", err)
			}

			switch backup.Status {
			case "Completed":
				fmt.Printf("\nBackup %q completed successfully\n", name)
				if backup.ItemsBackedUp > 0 {
					fmt.Printf("  Items backed up: %d\n", backup.ItemsBackedUp)
				}
				if backup.Warnings > 0 {
					fmt.Printf("  Warnings: %d\n", backup.Warnings)
				}
				return nil
			case "Failed", "PartiallyFailed":
				fmt.Printf("\nBackup %q %s\n", name, backup.Status)
				if backup.Errors > 0 {
					fmt.Printf("  Errors: %d\n", backup.Errors)
				}
				return fmt.Errorf("backup failed with status: %s", backup.Status)
			case "InProgress", "New":
				// Still running, continue waiting
				if backup.TotalItems > 0 {
					fmt.Printf("\r  Progress: %d/%d items", backup.ItemsBackedUp, backup.TotalItems)
				} else {
					fmt.Printf("\r  Status: %s", backup.Status)
				}
			default:
				fmt.Printf("\r  Status: %s", backup.Status)
			}
		}
	}
}
