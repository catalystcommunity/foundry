package setup

import (
	"context"
	"fmt"

	"github.com/urfave/cli/v3"
)

// Commands returns the setup command tree
func Commands() *cli.Command {
	return &cli.Command{
		Name:  "setup",
		Usage: "Interactive setup wizard for complete stack installation",
		Description: `The setup wizard guides you through the entire Foundry stack installation:
  • Network planning and validation
  • OpenBAO secrets management
  • PowerDNS for infrastructure and Kubernetes DNS
  • Zot container registry
  • Kubernetes cluster with K3s
  • Basic networking (Contour, cert-manager)

Progress is tracked and saved at each step. You can safely interrupt
and resume the wizard at any time.`,
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:    "config",
				Aliases: []string{"c"},
				Usage:   "Path to stack config file",
				Value:   "~/.foundry/stack.yaml",
			},
			&cli.BoolFlag{
				Name:  "reset",
				Usage: "Reset setup state and start from beginning",
			},
			&cli.BoolFlag{
				Name:  "resume",
				Usage: "Resume from last checkpoint (default behavior)",
			},
		},
		Action: runSetup,
	}
}

func runSetup(ctx context.Context, cmd *cli.Command) error {
	configPath := cmd.String("config")
	reset := cmd.Bool("reset")

	// Create wizard
	wizard, err := NewWizard(configPath)
	if err != nil {
		return fmt.Errorf("failed to create wizard: %w", err)
	}

	// Handle reset flag
	if reset {
		if err := wizard.Reset(); err != nil {
			return err
		}
		fmt.Println("Restarting setup from the beginning...")
	}

	// Run wizard
	return wizard.Run(ctx)
}
