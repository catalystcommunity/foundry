package backup

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/urfave/cli/v3"
)

// ListCommand lists all backups
var ListCommand = &cli.Command{
	Name:  "list",
	Usage: "List all cluster backups",
	Description: `Lists all Velero backups in the cluster.

Shows backup name, status, start time, and item counts.

Examples:
  foundry backup list              # List all backups
  foundry backup list --restores   # Also show restore operations`,
	Flags: []cli.Flag{
		&cli.BoolFlag{
			Name:  "restores",
			Usage: "Also list restore operations",
		},
		&cli.BoolFlag{
			Name:  "schedules",
			Usage: "Also list backup schedules",
		},
		&cli.BoolFlag{
			Name:  "all",
			Usage: "List backups, restores, and schedules",
		},
	},
	Action: runList,
}

func runList(ctx context.Context, cmd *cli.Command) error {
	// Create Velero client
	client, err := NewVeleroClient()
	if err != nil {
		return err
	}

	showRestores := cmd.Bool("restores") || cmd.Bool("all")
	showSchedules := cmd.Bool("schedules") || cmd.Bool("all")

	// List backups
	backups, err := client.ListBackups(ctx)
	if err != nil {
		return fmt.Errorf("failed to list backups: %w", err)
	}

	// Sort backups by start time (most recent first)
	sort.Slice(backups, func(i, j int) bool {
		if backups[i].StartTimestamp == nil {
			return false
		}
		if backups[j].StartTimestamp == nil {
			return true
		}
		return backups[i].StartTimestamp.After(*backups[j].StartTimestamp)
	})

	fmt.Println("BACKUPS")
	fmt.Println(strings.Repeat("-", 90))
	if len(backups) == 0 {
		fmt.Println("No backups found")
	} else {
		fmt.Printf("%-30s %-15s %-20s %-10s %-10s\n", "NAME", "STATUS", "STARTED", "ITEMS", "ERRORS")
		for _, b := range backups {
			startTime := "N/A"
			if b.StartTimestamp != nil {
				startTime = formatTime(*b.StartTimestamp)
			}
			items := fmt.Sprintf("%d", b.ItemsBackedUp)
			if b.TotalItems > 0 && b.Status == "InProgress" {
				items = fmt.Sprintf("%d/%d", b.ItemsBackedUp, b.TotalItems)
			}
			errors := "-"
			if b.Errors > 0 {
				errors = fmt.Sprintf("%d", b.Errors)
			}
			fmt.Printf("%-30s %-15s %-20s %-10s %-10s\n", truncate(b.Name, 30), b.Status, startTime, items, errors)
		}
	}
	fmt.Println()

	// List restores if requested
	if showRestores {
		restores, err := client.ListRestores(ctx)
		if err != nil {
			return fmt.Errorf("failed to list restores: %w", err)
		}

		// Sort restores by start time (most recent first)
		sort.Slice(restores, func(i, j int) bool {
			if restores[i].StartTimestamp == nil {
				return false
			}
			if restores[j].StartTimestamp == nil {
				return true
			}
			return restores[i].StartTimestamp.After(*restores[j].StartTimestamp)
		})

		fmt.Println("RESTORES")
		fmt.Println(strings.Repeat("-", 90))
		if len(restores) == 0 {
			fmt.Println("No restores found")
		} else {
			fmt.Printf("%-30s %-30s %-15s %-10s\n", "NAME", "BACKUP", "STATUS", "ERRORS")
			for _, r := range restores {
				errors := "-"
				if r.Errors > 0 {
					errors = fmt.Sprintf("%d", r.Errors)
				}
				fmt.Printf("%-30s %-30s %-15s %-10s\n", truncate(r.Name, 30), truncate(r.BackupName, 30), r.Status, errors)
			}
		}
		fmt.Println()
	}

	// List schedules if requested
	if showSchedules {
		schedules, err := client.ListSchedules(ctx)
		if err != nil {
			return fmt.Errorf("failed to list schedules: %w", err)
		}

		fmt.Println("SCHEDULES")
		fmt.Println(strings.Repeat("-", 90))
		if len(schedules) == 0 {
			fmt.Println("No schedules found")
		} else {
			fmt.Printf("%-25s %-20s %-15s %-25s\n", "NAME", "SCHEDULE", "STATUS", "LAST BACKUP")
			for _, s := range schedules {
				lastBackup := "Never"
				if s.LastBackup != nil {
					lastBackup = formatTime(*s.LastBackup)
				}
				fmt.Printf("%-25s %-20s %-15s %-25s\n", truncate(s.Name, 25), s.Schedule, s.Phase, lastBackup)
			}
		}
		fmt.Println()
	}

	return nil
}

// formatTime formats a time for display
func formatTime(t time.Time) string {
	now := time.Now()
	diff := now.Sub(t)

	if diff < time.Minute {
		return "just now"
	} else if diff < time.Hour {
		return fmt.Sprintf("%dm ago", int(diff.Minutes()))
	} else if diff < 24*time.Hour {
		return fmt.Sprintf("%dh ago", int(diff.Hours()))
	} else if diff < 7*24*time.Hour {
		return fmt.Sprintf("%dd ago", int(diff.Hours()/24))
	}
	return t.Format("2006-01-02 15:04")
}

// truncate truncates a string to max length with ellipsis
func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max-3] + "..."
}
