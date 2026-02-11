package tailscale

import (
	"context"
	"fmt"
	"time"

	"github.com/catalystcommunity/foundry/v1/internal/helm"
)

// HelmClient defines the interface for Helm operations needed by Tailscale installer.
// This interface allows for easier testing with mock implementations.
type HelmClient interface {
	AddRepo(ctx context.Context, opts helm.RepoAddOptions) error
	Install(ctx context.Context, opts helm.InstallOptions) error
	Uninstall(ctx context.Context, opts helm.UninstallOptions) error
}

const (
	// Tailscale Helm repository constants
	TailscaleRepoName = "tailscale"
	TailscaleRepoURL  = "https://pkgs.tailscale.com/helmcharts"

	// Operator chart constants
	OperatorChartName    = "tailscale-operator"
	OperatorReleaseName  = "tailscale-operator"

	// Installation timeouts
	DefaultInstallTimeout = 5 * time.Minute
)

// HelmInstaller handles Helm operations for Tailscale operator.
// This is separated from the main Installer to allow for easier testing.
type HelmInstaller struct {
	client HelmClient
	config *Config
}

// NewHelmInstaller creates a new Helm installer for Tailscale.
func NewHelmInstaller(client HelmClient, config *Config) (*HelmInstaller, error) {
	if client == nil {
		return nil, fmt.Errorf("helm client cannot be nil")
	}
	if config == nil {
		return nil, fmt.Errorf("config cannot be nil")
	}

	return &HelmInstaller{
		client: client,
		config: config,
	}, nil
}

// AddRepository adds the Tailscale Helm repository.
func (h *HelmInstaller) AddRepository(ctx context.Context) error {
	opts := helm.RepoAddOptions{
		Name:        TailscaleRepoName,
		URL:         TailscaleRepoURL,
		ForceUpdate: true, // Update if already exists
	}

	if err := h.client.AddRepo(ctx, opts); err != nil {
		return fmt.Errorf("failed to add Tailscale repository: %w", err)
	}

	return nil
}

// InstallOperator installs the Tailscale operator Helm chart.
func (h *HelmInstaller) InstallOperator(ctx context.Context) error {
	// Generate Helm values from config
	values, err := h.generateHelmValues()
	if err != nil {
		return fmt.Errorf("failed to generate Helm values: %w", err)
	}

	opts := helm.InstallOptions{
		ReleaseName:     OperatorReleaseName,
		Namespace:       DefaultNamespace,
		Chart:           fmt.Sprintf("%s/%s", TailscaleRepoName, OperatorChartName),
		Values:          values,
		CreateNamespace: true,
		Wait:            true,
		Timeout:         DefaultInstallTimeout,
	}

	if err := h.client.Install(ctx, opts); err != nil {
		return fmt.Errorf("failed to install Tailscale operator: %w", err)
	}

	return nil
}

// UninstallOperator uninstalls the Tailscale operator Helm chart.
func (h *HelmInstaller) UninstallOperator(ctx context.Context) error {
	opts := helm.UninstallOptions{
		ReleaseName: OperatorReleaseName,
		Namespace:   DefaultNamespace,
		Wait:        true,
		Timeout:     DefaultInstallTimeout,
	}

	if err := h.client.Uninstall(ctx, opts); err != nil {
		return fmt.Errorf("failed to uninstall Tailscale operator: %w", err)
	}

	return nil
}

// generateHelmValues creates the Helm values map from Tailscale config.
func (h *HelmInstaller) generateHelmValues() (map[string]interface{}, error) {
	if h.config == nil {
		return nil, fmt.Errorf("config cannot be nil")
	}

	values := make(map[string]interface{})

	// OAuth credentials (required)
	if h.config.OAuthClientID == nil || h.config.OAuthClientSecret == nil {
		return nil, fmt.Errorf("OAuth credentials are required")
	}

	values["oauth"] = map[string]interface{}{
		"clientId":     *h.config.OAuthClientID,
		"clientSecret": *h.config.OAuthClientSecret,
	}

	// Custom operator image (optional)
	if h.config.OperatorImage != nil && *h.config.OperatorImage != "" {
		// Parse image into repository and tag
		// Format: "registry/repository:tag" or "repository:tag"
		image := *h.config.OperatorImage
		values["image"] = map[string]interface{}{
			"repository": image, // Helm chart will parse this
		}
	}

	return values, nil
}

// GenerateSecretData creates the secret data structure for OAuth credentials.
// This is used when creating a Kubernetes secret directly (alternative to Helm).
func GenerateSecretData(config *Config) (map[string]string, error) {
	if config == nil {
		return nil, fmt.Errorf("config cannot be nil")
	}
	if config.OAuthClientID == nil || *config.OAuthClientID == "" {
		return nil, fmt.Errorf("OAuth client ID is required")
	}
	if config.OAuthClientSecret == nil || *config.OAuthClientSecret == "" {
		return nil, fmt.Errorf("OAuth client secret is required")
	}

	return map[string]string{
		"client_id":     *config.OAuthClientID,
		"client_secret": *config.OAuthClientSecret,
	}, nil
}
