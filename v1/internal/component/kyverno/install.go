package kyverno

import (
	"context"
	"fmt"
	"time"

	"github.com/catalystcommunity/foundry/v1/internal/helm"
	"github.com/catalystcommunity/foundry/v1/internal/k8s"
	foundrykyverno "github.com/catalystcommunity/foundry/v1/internal/kyverno"
)

const (
	repositoryName = "kyverno"
	repositoryURL  = "https://kyverno.github.io/kyverno/"
	chartName      = "kyverno/kyverno"
	releaseName    = "kyverno"
)

// Install installs or upgrades Kyverno, then reconciles the selected policies.
func Install(ctx context.Context, helmClient HelmClient, k8sClient *k8s.Client, config *Config) error {
	if helmClient == nil {
		return fmt.Errorf("Helm client is required")
	}
	if k8sClient == nil {
		return fmt.Errorf("Kubernetes client is required")
	}
	if config == nil {
		config = DefaultConfig()
	}
	if err := installHelmRelease(ctx, helmClient, config); err != nil {
		return err
	}

	catalog, err := foundrykyverno.LoadCatalog()
	if err != nil {
		return err
	}
	reconciler, err := foundrykyverno.NewReconciler(k8sClient.DynamicClient(), catalog)
	if err != nil {
		return err
	}
	result, err := reconciler.Reconcile(ctx, config.Policies)
	if err != nil {
		return fmt.Errorf("reconcile Kyverno policies: %w", err)
	}
	fmt.Printf("  Kyverno policies: %d applied, %d removed, %d unchanged\n", result.Applied, result.Deleted, result.Skipped)
	return nil
}

func installHelmRelease(ctx context.Context, helmClient HelmClient, config *Config) error {
	if err := helmClient.AddRepo(ctx, helm.RepoAddOptions{
		Name: repositoryName, URL: repositoryURL, ForceUpdate: true,
	}); err != nil {
		return fmt.Errorf("add Kyverno Helm repository: %w", err)
	}

	releaseExists := false
	releases, err := helmClient.List(ctx, config.Namespace)
	if err != nil {
		return fmt.Errorf("list Kyverno releases: %w", err)
	}
	for _, release := range releases {
		if release.Name == releaseName {
			releaseExists = true
			break
		}
	}

	if releaseExists {
		if err := helmClient.Upgrade(ctx, helm.UpgradeOptions{
			ReleaseName: releaseName,
			Namespace:   config.Namespace,
			Chart:       chartName,
			Version:     config.Version,
			Values:      config.Values,
			Wait:        true,
			Timeout:     10 * time.Minute,
		}); err != nil {
			return fmt.Errorf("upgrade Kyverno: %w", err)
		}
	} else {
		if err := helmClient.Install(ctx, helm.InstallOptions{
			ReleaseName:     releaseName,
			Namespace:       config.Namespace,
			Chart:           chartName,
			Version:         config.Version,
			Values:          config.Values,
			CreateNamespace: true,
			Wait:            true,
			Timeout:         10 * time.Minute,
		}); err != nil {
			return fmt.Errorf("install Kyverno: %w", err)
		}
	}

	return nil
}
