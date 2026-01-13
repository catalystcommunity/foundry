package host

import (
	"context"
	"fmt"
	"text/tabwriter"

	"github.com/catalystcommunity/foundry/v1/internal/config"
	"github.com/catalystcommunity/foundry/v1/internal/host"
	"github.com/urfave/cli/v3"
)

// ListCommand lists all registered hosts
var ListCommand = &cli.Command{
	Name:    "list",
	Aliases: []string{"ls"},
	Usage:   "List all registered hosts",
	Flags: []cli.Flag{
		&cli.BoolFlag{
			Name:    "verbose",
			Aliases: []string{"v"},
			Usage:   "show detailed information",
		},
	},
	Action: runList,
}

func runList(ctx context.Context, cmd *cli.Command) error {
	// Initialize config-based host registry (--config flag inherited from root command)
	configPath, err := config.FindConfig(cmd.String("config"))
	if err != nil {
		return fmt.Errorf("failed to find config file: %w (run 'foundry config init' first)", err)
	}

	loader := config.NewHostConfigLoader(configPath)
	registry := host.NewConfigRegistry(configPath, loader)
	host.SetDefaultRegistry(registry)

	// Get all hosts from registry
	hosts, err := host.List()
	if err != nil {
		return fmt.Errorf("failed to list hosts: %w", err)
	}

	// Handle empty registry
	if len(hosts) == 0 {
		fmt.Fprintln(cmd.Root().Writer, "No hosts registered.")
		fmt.Fprintln(cmd.Root().Writer, "\nUse 'foundry host add' to add a new host.")
		return nil
	}

	// Display hosts in table format
	w := tabwriter.NewWriter(cmd.Root().Writer, 0, 0, 3, ' ', 0)
	defer w.Flush()

	if cmd.Bool("verbose") {
		fmt.Fprintln(w, "HOSTNAME\tADDRESS\tPORT\tUSER\tSSH KEY")
		fmt.Fprintln(w, "--------\t-------\t----\t----\t-------")
		for _, h := range hosts {
			keyStatus := "not set"
			if h.SSHKeySet {
				keyStatus = "configured"
			}
			fmt.Fprintf(w, "%s\t%s\t%d\t%s\t%s\n",
				h.Hostname, h.Address, h.Port, h.User, keyStatus)
		}
	} else {
		fmt.Fprintln(w, "HOSTNAME\tADDRESS\tUSER")
		fmt.Fprintln(w, "--------\t-------\t----")
		for _, h := range hosts {
			fmt.Fprintf(w, "%s\t%s\t%s\n",
				h.Hostname, h.Address, h.User)
		}
	}

	return nil
}
