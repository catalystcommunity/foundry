package stack

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/catalystcommunity/foundry/v1/internal/config"
	"github.com/stretchr/testify/require"
	"github.com/urfave/cli/v3"
)

func TestPoliciesApplyRejectsInvalidConfiguration(t *testing.T) {
	for _, tc := range []struct{ name, expected string }{
		{"missing", "config file not found"},
		{"disabled", "Kyverno is disabled"},
		{"upgrades disabled", "chart updates are disabled"},
		{"missing kubeconfig", "failed to read kubeconfig"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("FOUNDRY_CONFIG_DIR", t.TempDir())
			cfg := createTestConfig(t)
			if tc.name == "disabled" {
				cfg.Components["kyverno"] = config.ComponentConfig{Config: map[string]interface{}{"enabled": false}}
			}
			if tc.name == "upgrades disabled" {
				no := false
				cfg.Components["kyverno"] = config.ComponentConfig{AllowUpgrades: &no}
			}
			path := filepath.Join(t.TempDir(), "stack.yaml")
			if tc.name != "missing" {
				require.NoError(t, config.Save(cfg, path))
			}
			cmd := &cli.Command{Name: "foundry", Flags: []cli.Flag{&cli.StringFlag{Name: "config"}}, Commands: []*cli.Command{PoliciesCommand}}
			err := cmd.Run(context.Background(), []string{"foundry", "--config", path, "policies", "apply"})
			require.ErrorContains(t, err, tc.expected)
		})
	}
}
