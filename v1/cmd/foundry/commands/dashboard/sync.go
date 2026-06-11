package dashboard

import (
	"context"
	"fmt"
	"path/filepath"

	"github.com/urfave/cli/v3"

	"github.com/catalystcommunity/foundry/v1/internal/config"
	"github.com/catalystcommunity/foundry/v1/internal/dashboards"
)

// SyncCommand renders dashboard JSON files into Grafana-discoverable ConfigMaps.
var SyncCommand = &cli.Command{
	Name:  "sync",
	Usage: "Install dashboards from the stack dashboards directory into Grafana",
	Description: `Seeds the bundled default dashboards into ~/.foundry/<stack>_dashboards/defaults/,
then renders every dashboard JSON file (the managed defaults plus any user dashboards
you drop directly in ~/.foundry/<stack>_dashboards/) into ConfigMaps labeled
` + "`grafana_dashboard=1`" + `. The Grafana dashboard sidecar discovers and loads them
automatically — no Grafana restart required.

This also runs automatically during 'foundry component install grafana', so default
dashboards are installed/refreshed on every stack update. Use this command to push
changes without reinstalling Grafana.

User dashboards are any *.json files placed at the top level of the directory.
The defaults/ subdirectory is managed by foundry and refreshed on every sync.

Examples:
  foundry dashboard sync                 # seed defaults + install all dashboards
  foundry dashboard sync --prune         # also remove dashboards no longer present
  foundry dashboard sync --dry-run       # show what would change`,
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:    "namespace",
			Aliases: []string{"n"},
			Usage:   "Namespace where Grafana (and its dashboard sidecar) runs",
			Value:   "monitoring",
		},
		&cli.StringFlag{
			Name:  "dir",
			Usage: "Override the dashboards directory (default ~/.foundry/<stack>_dashboards)",
		},
		&cli.BoolFlag{
			Name:  "prune",
			Usage: "Delete foundry-managed dashboard ConfigMaps that are no longer present locally",
		},
		&cli.BoolFlag{
			Name:    "dry-run",
			Aliases: []string{"d"},
			Usage:   "Show what would change without modifying the cluster",
		},
		&cli.BoolFlag{
			Name:  "no-seed",
			Usage: "Do not refresh the bundled default dashboards before syncing",
		},
	},
	Action: runSync,
}

// ListCommand lists dashboards in the directory.
var ListCommand = &cli.Command{
	Name:  "list",
	Usage: "List available dashboards in the stack dashboards directory",
	Flags: []cli.Flag{
		&cli.StringFlag{Name: "dir", Usage: "Override the dashboards directory"},
	},
	Action: runList,
}

func runSync(ctx context.Context, cmd *cli.Command) error {
	dir, err := resolveDashboardsDir(cmd)
	if err != nil {
		return err
	}
	namespace := cmd.String("namespace")

	if !cmd.Bool("no-seed") {
		n, err := dashboards.SeedDefaults(dir)
		if err != nil {
			return fmt.Errorf("failed to seed default dashboards: %w", err)
		}
		fmt.Printf("Seeded %d default dashboard(s) into %s\n", n, filepath.Join(dir, dashboards.DefaultsSubdir))
	}

	files, err := dashboards.Collect(dir)
	if err != nil {
		return err
	}
	if len(files) == 0 {
		fmt.Printf("No dashboards found in %s\n", dir)
		return nil
	}

	var valid []dashboards.File
	for _, f := range files {
		if !f.Valid {
			fmt.Printf("  ⚠ skipping %s (%s): %s\n", f.FileName, f.Source, f.Err)
			continue
		}
		valid = append(valid, f)
	}

	if cmd.Bool("dry-run") {
		fmt.Printf("\nDry-run: would install %d dashboard(s) into namespace %q:\n", len(valid), namespace)
		for _, f := range valid {
			fmt.Printf("  %s  ->  configmap/%s  [%s]\n", f.FileName, dashboards.ConfigMapName(f.Base), f.Source)
		}
		if cmd.Bool("prune") {
			fmt.Println("  (prune enabled: stale foundry-managed dashboards would be removed)")
		}
		return nil
	}

	clientset, err := getK8sClient()
	if err != nil {
		return err
	}

	result, err := dashboards.Apply(ctx, clientset, namespace, valid, cmd.Bool("prune"))
	if err != nil {
		return err
	}

	fmt.Printf("\n✓ Dashboards synced to namespace %q: %d created, %d updated", namespace, result.Created, result.Updated)
	if cmd.Bool("prune") {
		fmt.Printf(", %d pruned", result.Pruned)
	}
	fmt.Println()
	fmt.Println("The Grafana sidecar will load them within ~1 minute. Open with: foundry dashboard open")
	return nil
}

func runList(ctx context.Context, cmd *cli.Command) error {
	dir, err := resolveDashboardsDir(cmd)
	if err != nil {
		return err
	}
	if _, err := dashboards.SeedDefaults(dir); err != nil {
		return fmt.Errorf("failed to seed default dashboards: %w", err)
	}
	files, err := dashboards.Collect(dir)
	if err != nil {
		return err
	}
	fmt.Printf("Dashboards directory: %s\n\n", dir)
	if len(files) == 0 {
		fmt.Println("(none)")
		return nil
	}
	fmt.Printf("%-32s %-8s %-7s %s\n", "FILE", "SOURCE", "VALID", "TITLE")
	for _, f := range files {
		valid := "yes"
		title := f.Title
		if !f.Valid {
			valid = "NO"
			title = f.Err
		}
		fmt.Printf("%-32s %-8s %-7s %s\n", f.FileName, f.Source, valid, title)
	}
	return nil
}

// resolveDashboardsDir determines the dashboards directory, honoring --dir, else
// derives it from the configured stack name: ~/.foundry/<stack>_dashboards.
func resolveDashboardsDir(cmd *cli.Command) (string, error) {
	if override := cmd.String("dir"); override != "" {
		return override, nil
	}
	configDir, err := config.GetConfigDir()
	if err != nil {
		return "", err
	}
	return dashboards.Dir(configDir, resolveStackName(cmd)), nil
}

// resolveStackName reads the cluster name from the stack config, falling back to
// "foundry" when no config is available.
func resolveStackName(cmd *cli.Command) string {
	configPath, err := config.FindConfig(cmd.String("config"))
	if err != nil {
		return "foundry"
	}
	cfg, err := config.Load(configPath)
	if err != nil || cfg.Cluster.Name == "" {
		return "foundry"
	}
	return cfg.Cluster.Name
}
