package stack

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/catalystcommunity/foundry/v1/internal/config"
	"github.com/catalystcommunity/foundry/v1/internal/k8s"
	"github.com/catalystcommunity/foundry/v1/internal/kyverno"
	"github.com/urfave/cli/v3"
)

// PoliciesCommand applies the policy engine configuration and saved selections.
var PoliciesCommand = &cli.Command{
	Name:  "policies",
	Usage: "Manage cluster admission policies",
	Commands: []*cli.Command{{
		Name:  "apply",
		Usage: "Apply Kyverno configuration and saved policy selections",
		Action: func(ctx context.Context, cmd *cli.Command) error {
			path := cmd.String("config")
			if path == "" {
				path = config.DefaultConfigPath()
			}
			cfg, err := config.Load(path)
			if err != nil {
				return err
			}
			if !componentEnabled(cfg, "kyverno") {
				return fmt.Errorf("Kyverno is disabled; enable it before applying policies")
			}
			if !componentUpgradesAllowed(cfg, "kyverno") {
				return fmt.Errorf("Kyverno chart updates are disabled by components.kyverno.allow_upgrades")
			}
			return installK8sComponent(ctx, cfg, "kyverno", nil)
		},
	}},
}

// ApplyPolicies reconciles policies without installing or upgrading any component.
// Kyverno must already be installed by stack install.
func ApplyPolicies(ctx context.Context, configPath string) error {
	cfg, err := config.Load(configPath)
	if err != nil {
		return err
	}
	if !componentEnabled(cfg, "kyverno") {
		return fmt.Errorf("Kyverno is disabled; enable it and run stack install before applying policies")
	}
	return reconcileConfiguredPolicies(ctx, cfg)
}

func reconcileConfiguredPolicies(ctx context.Context, cfg *config.Config) error {
	if !componentEnabled(cfg, "kyverno") {
		return nil
	}
	configDir, err := config.GetConfigDir()
	if err != nil {
		return err
	}
	kubeconfig, err := os.ReadFile(filepath.Join(configDir, "kubeconfig"))
	if err != nil {
		return fmt.Errorf("read kubeconfig for policy reconciliation: %w", err)
	}
	client, err := k8s.NewClientFromKubeconfig(kubeconfig)
	if err != nil {
		return err
	}
	catalog, err := kyverno.LoadCatalog()
	if err != nil {
		return err
	}
	reconciler, err := kyverno.NewReconciler(client.DynamicClient(), catalog)
	if err != nil {
		return err
	}
	result, err := reconciler.Reconcile(ctx, config.PolicySelections(cfg.Policies))
	if err != nil {
		return fmt.Errorf("reconcile Kyverno policies (run stack install if Kyverno is missing): %w", err)
	}
	fmt.Printf("Kyverno policies: %d applied, %d removed, %d unchanged\n", result.Applied, result.Deleted, result.Skipped)
	return nil
}
