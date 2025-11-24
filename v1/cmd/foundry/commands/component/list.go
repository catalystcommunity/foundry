package component

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/catalystcommunity/foundry/v1/internal/component"
	"github.com/urfave/cli/v3"
)

// ListCommand lists all available components
var ListCommand = &cli.Command{
	Name:  "list",
	Usage: "List all available components",
	Description: `Lists all components that can be installed via Foundry.

Components are shown with their names and dependencies.`,
	Action: runList,
}

func runList(ctx context.Context, cmd *cli.Command) error {
	// Get all registered components
	components := component.GetAll()

	if len(components) == 0 {
		fmt.Println("No components registered")
		return nil
	}

	// Sort components by name for consistent output
	sort.Slice(components, func(i, j int) bool {
		return components[i].Name() < components[j].Name()
	})

	// Print header
	fmt.Println("Available Components:")
	fmt.Println()

	// Calculate maximum name length for formatting
	maxNameLen := 0
	for _, comp := range components {
		if len(comp.Name()) > maxNameLen {
			maxNameLen = len(comp.Name())
		}
	}

	// Print each component
	for _, comp := range components {
		deps := comp.Dependencies()
		depsStr := "none"
		if len(deps) > 0 {
			depsStr = strings.Join(deps, ", ")
		}

		// Format: name (padded) - dependencies
		fmt.Printf("  %-*s - Dependencies: %s\n", maxNameLen, comp.Name(), depsStr)
	}

	fmt.Println()
	fmt.Println("Use 'foundry component install <name>' to install a component")
	fmt.Println("Use 'foundry component status <name>' to check component status")

	return nil
}
