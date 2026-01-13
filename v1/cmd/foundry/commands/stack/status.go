package stack

import (
	"context"
	"fmt"

	"github.com/catalystcommunity/foundry/v1/cmd/foundry/commands/component"
	internalComponent "github.com/catalystcommunity/foundry/v1/internal/component"
	"github.com/catalystcommunity/foundry/v1/internal/config"
	"github.com/urfave/cli/v3"
)

// StatusCommand handles the 'foundry stack status' command
var StatusCommand = &cli.Command{
	Name:   "status",
	Usage:  "Show status of all stack components",
	Action: runStackStatus,
}

func runStackStatus(ctx context.Context, cmd *cli.Command) error {
	// Load configuration (--config flag inherited from root command)
	configPath, err := config.FindConfig(cmd.String("config"))
	if err != nil {
		return fmt.Errorf("failed to find config: %w", err)
	}

	_, err = config.Load(configPath)
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	// Get installation order
	installOrder, err := determineInstallationOrder()
	if err != nil {
		return fmt.Errorf("failed to determine installation order: %w", err)
	}

	// Query status of all components
	statuses := make(map[string]*internalComponent.ComponentStatus)
	for _, compName := range installOrder {
		var status *internalComponent.ComponentStatus
		var err error

		// Use the same checking logic as component status command
		switch compName {
		case "openbao":
			status, err = component.CheckOpenBAOStatus(ctx)
		case "dns":
			status, err = component.CheckDNSStatus(ctx)
		case "zot":
			status, err = component.CheckZotStatus(ctx)
		case "k3s":
			status, err = component.CheckK3sStatus(ctx)
		case "contour", "cert-manager":
			// These components don't have dedicated status checkers yet
			comp := internalComponent.Get(compName)
			if comp == nil {
				status = &internalComponent.ComponentStatus{
					Installed: false,
					Healthy:   false,
					Message:   "not found in registry",
				}
			} else {
				status, err = comp.Status(ctx)
			}
		default:
			status = &internalComponent.ComponentStatus{
				Installed: false,
				Healthy:   false,
				Message:   "status checking not implemented",
			}
		}

		if err != nil {
			statuses[compName] = &internalComponent.ComponentStatus{
				Installed: false,
				Healthy:   false,
				Message:   fmt.Sprintf("error querying status: %v", err),
			}
			continue
		}
		statuses[compName] = status
	}

	// Display status table
	displayStatusTable(installOrder, statuses)

	// Show overall health
	health := calculateOverallHealth(statuses)
	fmt.Println()
	displayOverallHealth(health)

	return nil
}

// displayStatusTable shows a formatted table of component statuses
func displayStatusTable(order []string, statuses map[string]*internalComponent.ComponentStatus) {
	fmt.Println("Stack Component Status:")
	fmt.Println()
	fmt.Printf("  %-20s %-12s %-10s %-15s %s\n", "COMPONENT", "INSTALLED", "HEALTHY", "VERSION", "MESSAGE")
	fmt.Printf("  %-20s %-12s %-10s %-15s %s\n", "─────────", "─────────", "───────", "───────", "───────")

	for _, name := range order {
		status := statuses[name]
		if status == nil {
			continue
		}

		// Status symbol
		symbol := "✓"
		if !status.Installed {
			symbol = "✗"
		} else if !status.Healthy {
			symbol = "⚠"
		}

		// Installed and Healthy indicators
		installedStr := "no"
		if status.Installed {
			installedStr = "yes"
		}
		healthyStr := "no"
		if status.Healthy {
			healthyStr = "yes"
		}

		// Version (use "-" if not available)
		version := status.Version
		if version == "" {
			version = "-"
		}

		// Message (truncate if too long)
		message := status.Message
		if len(message) > 50 {
			message = message[:47] + "..."
		}

		fmt.Printf("  %s %-18s %-12s %-10s %-15s %s\n",
			symbol, name, installedStr, healthyStr, version, message)
	}
}

// StackHealth represents the overall health of the stack
type StackHealth struct {
	TotalComponents     int
	InstalledComponents int
	HealthyComponents   int
	UnhealthyComponents int
	NotInstalledCount   int
	OverallHealthy      bool
}

// calculateOverallHealth determines the overall health of the stack
func calculateOverallHealth(statuses map[string]*internalComponent.ComponentStatus) StackHealth {
	health := StackHealth{
		TotalComponents: len(statuses),
	}

	for _, status := range statuses {
		if status.Installed {
			health.InstalledComponents++
			if status.Healthy {
				health.HealthyComponents++
			} else {
				health.UnhealthyComponents++
			}
		} else {
			health.NotInstalledCount++
		}
	}

	// Stack is healthy if all installed components are healthy
	health.OverallHealthy = health.InstalledComponents > 0 &&
		health.UnhealthyComponents == 0

	return health
}

// displayOverallHealth shows the overall health summary
func displayOverallHealth(health StackHealth) {
	fmt.Println("Overall Stack Health:")
	fmt.Printf("  Total components:       %d\n", health.TotalComponents)
	fmt.Printf("  Installed:              %d\n", health.InstalledComponents)
	fmt.Printf("  Healthy:                %d\n", health.HealthyComponents)
	fmt.Printf("  Unhealthy:              %d\n", health.UnhealthyComponents)
	fmt.Printf("  Not installed:          %d\n", health.NotInstalledCount)
	fmt.Println()

	if health.OverallHealthy {
		fmt.Println("  Status: ✓ All installed components are healthy")
	} else if health.InstalledComponents == 0 {
		fmt.Println("  Status: ⚠ No components are installed")
	} else if health.UnhealthyComponents > 0 {
		fmt.Printf("  Status: ✗ %d component(s) are unhealthy\n", health.UnhealthyComponents)
	}
}
