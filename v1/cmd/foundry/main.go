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
	hostcmd "github.com/catalystcommunity/foundry/v1/cmd/foundry/commands/host"
	logscmd "github.com/catalystcommunity/foundry/v1/cmd/foundry/commands/logs"
	metricscmd "github.com/catalystcommunity/foundry/v1/cmd/foundry/commands/metrics"
	networkcmd "github.com/catalystcommunity/foundry/v1/cmd/foundry/commands/network"
	setupcmd "github.com/catalystcommunity/foundry/v1/cmd/foundry/commands/setup"
	stackcmd "github.com/catalystcommunity/foundry/v1/cmd/foundry/commands/stack"
	storagecmd "github.com/catalystcommunity/foundry/v1/cmd/foundry/commands/storage"
	"github.com/catalystcommunity/foundry/v1/cmd/foundry/registry"
	"github.com/urfave/cli/v3"
)

var (
	// Version information (will be set by build flags)
	Version   = "dev"
	GitCommit = "unknown"
	BuildDate = "unknown"
)

func main() {
	// Initialize component registry
	if err := registry.InitComponents(); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to initialize component registry: %v\n", err)
		os.Exit(1)
	}

	// Initialize host registry
	if err := registry.InitHostRegistry(); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to initialize host registry: %v\n", err)
		os.Exit(1)
	}

	cmd := &cli.Command{
		Name:    "foundry",
		Usage:   "A CLI for managing Catalyst Community tech stacks",
		Version: Version,
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:    "config",
				Aliases: []string{"c"},
				Usage:   "path to config file",
				Sources: cli.EnvVars("FOUNDRY_CONFIG"),
			},
		},
		Commands: []*cli.Command{
			backupcmd.Command,
			clustercmd.Commands(),
			componentcmd.Command,
			configcmd.Command,
			dashboardcmd.Command,
			dnscmd.Command,
			hostcmd.Command,
			logscmd.Command,
			metricscmd.Command,
			networkcmd.Command,
			setupcmd.Commands(),
			stackcmd.Command,
			storagecmd.Command,
		},
	}

	if err := cmd.Run(context.Background(), os.Args); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}
