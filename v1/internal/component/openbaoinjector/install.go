package openbaoinjector

import (
	"context"
	"fmt"
	"time"

	"github.com/catalystcommunity/foundry/v1/internal/helm"
)

const (
	// DefaultNamespace is the namespace where the injector will be installed
	DefaultNamespace = "openbao"

	// ReleaseName is the Helm release name
	ReleaseName = "openbao-injector"

	// Helm repo constants
	repoName = "openbao"
	repoURL  = "https://openbao.github.io/openbao-helm"
	chart    = "openbao/openbao"

	installTimeout = 5 * time.Minute
)

// Install installs the OpenBao agent injector using Helm.
// Only the injector is deployed (server.enabled=false). The injector registers
// a MutatingWebhookConfiguration so that pods with vault.hashicorp.com/agent-inject
// annotations automatically get secrets mounted from OpenBao.
func Install(ctx context.Context, helmClient HelmClient, cfg *Config) error {
	if helmClient == nil {
		return fmt.Errorf("helm client cannot be nil")
	}
	if cfg == nil {
		return fmt.Errorf("config cannot be nil")
	}

	// Add OpenBao Helm repository
	if err := helmClient.AddRepo(ctx, helm.RepoAddOptions{
		Name:        repoName,
		URL:         repoURL,
		ForceUpdate: true,
	}); err != nil {
		return fmt.Errorf("failed to add openbao helm repo: %w", err)
	}

	values := buildHelmValues(cfg)

	// Check if release already exists
	releases, err := helmClient.List(ctx, cfg.Namespace)
	if err == nil {
		for _, rel := range releases {
			if rel.Name == ReleaseName {
				if rel.Status == "deployed" {
					fmt.Println("  Upgrading existing OpenBao injector deployment...")
					return helmClient.Upgrade(ctx, helm.UpgradeOptions{
						ReleaseName: ReleaseName,
						Namespace:   cfg.Namespace,
						Chart:       chart,
						Version:     cfg.Version,
						Values:      values,
						Wait:        true,
						Timeout:     installTimeout,
					})
				}
				// Failed/pending release — uninstall and reinstall
				fmt.Printf("  Removing failed release (status: %s)...\n", rel.Status)
				if err := helmClient.Uninstall(ctx, helm.UninstallOptions{
					ReleaseName: ReleaseName,
					Namespace:   cfg.Namespace,
				}); err != nil {
					return fmt.Errorf("failed to remove existing release: %w", err)
				}
				break
			}
		}
	}

	if err := helmClient.Install(ctx, helm.InstallOptions{
		ReleaseName:     ReleaseName,
		Namespace:       cfg.Namespace,
		Chart:           chart,
		Version:         cfg.Version,
		Values:          values,
		CreateNamespace: true,
		Wait:            true,
		Timeout:         installTimeout,
	}); err != nil {
		return fmt.Errorf("failed to install openbao injector: %w", err)
	}

	fmt.Printf("  ✓ OpenBao agent injector installed\n")
	fmt.Printf("  ✓ MutatingWebhookConfiguration registered\n")
	fmt.Printf("  Pods annotated with vault.hashicorp.com/agent-inject=true will now\n")
	fmt.Printf("  automatically receive secrets from OpenBao at %s\n", cfg.ExternalVaultAddr)

	return nil
}

// buildHelmValues constructs the Helm values for injector-only installation.
// server.enabled=false — we already have OpenBao running on the host.
// injector.enabled=true — this is the only thing we're installing.
func buildHelmValues(cfg *Config) map[string]interface{} {
	return map[string]interface{}{
		"server": map[string]interface{}{
			"enabled": false,
		},
		"injector": map[string]interface{}{
			"enabled":             true,
			"externalVaultAddr":   cfg.ExternalVaultAddr,
		},
	}
}
