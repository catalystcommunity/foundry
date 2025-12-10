package cluster

import (
	"github.com/urfave/cli/v3"
)

// Commands returns the cluster management commands
func Commands() *cli.Command {
	return &cli.Command{
		Name:  "cluster",
		Usage: "Manage Kubernetes cluster",
		Commands: []*cli.Command{
			initCommand(),
			nodeCommands(),
			NewStatusCommand(),
		},
	}
}

// nodeCommands returns the cluster node management commands
func nodeCommands() *cli.Command {
	return &cli.Command{
		Name:  "node",
		Usage: "Manage cluster nodes",
		Commands: []*cli.Command{
			NewNodeAddCommand(),
			NewNodeRemoveCommand(),
			NewNodeListCommand(),
			NewNodeLabelCommand(),
		},
	}
}
