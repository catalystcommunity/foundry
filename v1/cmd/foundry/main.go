package main

import (
	"context"
	"fmt"
	"os"

	backupcmd "github.com/catalystcommunity/foundry/v1/cmd/foundry/commands/backup"
	clustercmd "github.com/catalystcommunity/foundry/v1/cmd/foundry/commands/cluster"
	componentcmd "github.com/catalystcommunity/foundry/v1/cmd/foundry/commands/component"
	configcmd "github.com/catalystcommunity/foundry/v1/cmd/foundry/commands/config"
	dashboardcmd "github.com/catalystcommunity/foundry/v1/cmd/foundry/commands/dashboard"
	dnscmd "github.com/catalystcommunity/foundry/v1/cmd/foundry/commands/dns"
	gatewaycmd "github.com/catalystcommunity/foundry/v1/cmd/foundry/commands/gateway"
	grafanacmd "github.com/catalystcommunity/foundry/v1/cmd/foundry/commands/grafana"
	guicmd "github.com/catalystcommunity/foundry/v1/cmd/foundry/commands/gui"
	hostcmd "github.com/catalystcommunity/foundry/v1/cmd/foundry/commands/host"
	logscmd "github.com/catalystcommunity/foundry/v1/cmd/foundry/commands/logs"
	metricscmd "github.com/catalystcommunity/foundry/v1/cmd/foundry/commands/metrics"
	networkcmd "github.com/catalystcommunity/foundry/v1/cmd/foundry/commands/network"
	openbaocmd "github.com/catalystcommunity/foundry/v1/cmd/foundry/commands/openbao"
	stackcmd "github.com/catalystcommunity/foundry/v1/cmd/foundry/commands/stack"
	storagecmd "github.com/catalystcommunity/foundry/v1/cmd/foundry/commands/storage"
	"github.com/catalystcommunity/foundry/v1/cmd/foundry/registry"
	"github.com/urfave/cli/v3"
)

var (
	// Release builds set these values with linker flags.
	Version   = "dev"
	GitCommit = "unknown"
	BuildDate = "unknown"
)

func main() {
	if err := registry.InitComponents(); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to initialize component registry: %v\n", err)
		os.Exit(1)
	}

	if err := registry.InitHostRegistry(); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to initialize host registry: %v\n", err)
		os.Exit(1)
	}

	cmd := &cli.Command{
		Name:    "foundry",
		Usage:   "Manage Catalyst Community tech stacks",
		Version: Version,
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:    "config",
				Aliases: []string{"c"},
				Usage:   "Path to the configuration file",
				Sources: cli.EnvVars("FOUNDRY_CONFIG"),
			},
		},
		Before: func(ctx context.Context, cmd *cli.Command) (context.Context, error) {
			configFlag := cmd.String("config")
			if configFlag != "" {
				if err := registry.InitHostRegistryWithConfig(configFlag); err != nil {
					return ctx, fmt.Errorf("failed to initialize host registry with config %s: %w", configFlag, err)
				}
			}
			return ctx, nil
		},
		Commands: []*cli.Command{
			backupcmd.Command,
			clustercmd.Commands(),
			componentcmd.Command,
			configcmd.Command,
			dashboardcmd.Command,
			dnscmd.Command,
			gatewaycmd.Command,
			grafanacmd.Command,
			guicmd.Command,
			hostcmd.Command,
			logscmd.Command,
			metricscmd.Command,
			networkcmd.Command,
			openbaocmd.Command,
			stackcmd.Command,
			storagecmd.Command,
			guicmd.ServeCommand,
		},
	}

	if err := cmd.Run(context.Background(), os.Args); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}
