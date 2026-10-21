package cluster

import (
	"context"
	"fmt"

	"github.com/catalystcommunity/foundry/v1/internal/component/k3s"
	"github.com/catalystcommunity/foundry/v1/internal/config"
	"github.com/urfave/cli/v3"
)

func networkCommand() *cli.Command {
	return &cli.Command{
		Name: "network", Usage: "Convert control planes to persistent VIP interface isolation",
		Flags: []cli.Flag{&cli.BoolFlag{Name: "dry-run", Usage: "Inspect all nodes and show the conversion plan"}},
		Action: func(ctx context.Context, cmd *cli.Command) error {
			path, err := config.FindConfig(cmd.String("config"))
			if err != nil {
				return err
			}
			cfg, err := config.Load(path)
			if err != nil {
				return err
			}
			return reconcileClusterNetwork(ctx, cfg, cmd.Bool("dry-run"))
		},
	}
}

func reconcileClusterNetwork(ctx context.Context, cfg *config.Config, dryRun bool) error {
	allowCGNAT := cfg.Cluster.AllowCGNATVIP != nil && *cfg.Cluster.AllowCGNATVIP
	if err := k3s.ValidateVIP(cfg.Cluster.VIP, allowCGNAT); err != nil {
		return err
	}
	hosts := cfg.GetClusterHosts()
	roles, err := k3s.DetermineNodeRoles(convertToK3sNodes(hosts))
	if err != nil {
		return err
	}
	targets := []k3s.NetworkHost{}
	for i, h := range hosts {
		if !roles[i].IsControlPlane {
			continue
		}
		conn, err := connectToHost(h.Hostname)
		if err != nil {
			return err
		}
		defer conn.Close()
		targets = append(targets, k3s.NetworkHost{Name: h.Hostname, Address: h.Address, Executor: conn})
	}
	fmt.Println("Reconciling VIP and Flannel interfaces...")
	if err := k3s.ReconcileVIPIsolation(ctx, targets, cfg.Cluster.VIP, dryRun); err != nil {
		return err
	}
	if dryRun {
		fmt.Println("Network preflight passed; no changes made")
	} else {
		fmt.Println("VIP interface isolation reconciled")
	}
	return nil
}
