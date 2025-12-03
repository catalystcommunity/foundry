package backup

import (
	"context"
	"fmt"

	"github.com/urfave/cli/v3"
)

// ScheduleCommand manages backup schedules
var ScheduleCommand = &cli.Command{
	Name:      "schedule",
	Usage:     "Manage backup schedules",
	ArgsUsage: "<name>",
	Description: `Create or manage Velero backup schedules.

A schedule creates backups automatically at the specified interval using cron syntax.

Common cron expressions:
  "0 2 * * *"      - Daily at 2 AM
  "0 0 * * 0"      - Weekly on Sunday at midnight
  "0 0 1 * *"      - Monthly on the 1st at midnight
  "0 */6 * * *"    - Every 6 hours

Examples:
  foundry backup schedule daily --cron "0 2 * * *"
  foundry backup schedule weekly --cron "0 0 * * 0" --ttl 720h
  foundry backup schedule daily --delete    # Delete a schedule`,
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:     "cron",
			Usage:    "Cron expression for schedule (required unless --delete)",
			Required: false,
		},
		&cli.StringSliceFlag{
			Name:    "namespace",
			Aliases: []string{"n"},
			Usage:   "Namespaces to include in scheduled backups (can be specified multiple times)",
		},
		&cli.StringSliceFlag{
			Name:  "exclude-namespace",
			Usage: "Namespaces to exclude from scheduled backups (can be specified multiple times)",
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
		&cli.BoolFlag{
			Name:  "delete",
			Usage: "Delete the schedule instead of creating it",
		},
	},
	Action: runSchedule,
}

func runSchedule(ctx context.Context, cmd *cli.Command) error {
	// Get schedule name
	name := cmd.Args().Get(0)
	if name == "" {
		return fmt.Errorf("schedule name is required\n\nUsage: foundry backup schedule <name> --cron \"<expression>\"")
	}

	// Create Velero client
	client, err := NewVeleroClient()
	if err != nil {
		return err
	}

	// Handle delete
	if cmd.Bool("delete") {
		fmt.Printf("Deleting schedule %q...\n", name)
		if err := client.DeleteSchedule(ctx, name); err != nil {
			return fmt.Errorf("failed to delete schedule: %w", err)
		}
		fmt.Printf("Schedule %q deleted\n", name)
		return nil
	}

	// Cron is required for create
	cron := cmd.String("cron")
	if cron == "" {
		return fmt.Errorf("--cron is required when creating a schedule\n\nExamples:\n  --cron \"0 2 * * *\"    Daily at 2 AM\n  --cron \"0 0 * * 0\"    Weekly on Sunday")
	}

	// Build schedule options
	snapshotVolumes := cmd.Bool("snapshot-volumes")
	opts := ScheduleOptions{
		Cron:               cron,
		IncludedNamespaces: cmd.StringSlice("namespace"),
		ExcludedNamespaces: cmd.StringSlice("exclude-namespace"),
		TTL:                cmd.String("ttl"),
		SnapshotVolumes:    &snapshotVolumes,
	}

	// Add default excluded namespaces if none specified
	if len(opts.IncludedNamespaces) == 0 && len(opts.ExcludedNamespaces) == 0 {
		// By default, exclude system namespaces
		opts.ExcludedNamespaces = []string{"kube-system", "kube-public", "kube-node-lease", "velero"}
	}

	// Create schedule
	fmt.Printf("Creating schedule %q with cron %q...\n", name, cron)
	if err := client.CreateSchedule(ctx, name, opts); err != nil {
		return fmt.Errorf("failed to create schedule: %w", err)
	}

	fmt.Printf("Schedule %q created\n", name)
	fmt.Println("\nTo view schedules, run:")
	fmt.Println("  foundry backup list --schedules")

	return nil
}
