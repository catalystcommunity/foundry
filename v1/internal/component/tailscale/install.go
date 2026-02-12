package tailscale

import (
	"context"
	"fmt"
)

const (
	// DefaultNamespace is the namespace where Tailscale operator will be installed
	DefaultNamespace = "tailscale"
)

// Installer handles Tailscale operator installation and configuration.
type Installer struct {
	config         *Config
	vip            string
	helmClient     HelmClient
	kubeClient     KubernetesClient
	helmInstaller  *HelmInstaller
	crdInstaller   *CRDInstaller
	coreDNSPatcher *CoreDNSPatcher
}

// NewInstaller creates a new Tailscale installer with the given configuration.
func NewInstaller(cfg *Config, vip string, helmClient HelmClient, kubeClient KubernetesClient) (*Installer, error) {
	if cfg == nil {
		return nil, fmt.Errorf("config cannot be nil")
	}
	if helmClient == nil {
		return nil, fmt.Errorf("helm client cannot be nil")
	}
	if kubeClient == nil {
		return nil, fmt.Errorf("kubernetes client cannot be nil")
	}

	// Validate configuration
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("invalid configuration: %w", err)
	}

	// Set defaults
	cfg.SetDefaults()

	// Create sub-installers
	helmInstaller, err := NewHelmInstaller(helmClient, cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to create Helm installer: %w", err)
	}

	crdInstaller, err := NewCRDInstaller(kubeClient, cfg, vip)
	if err != nil {
		return nil, fmt.Errorf("failed to create CRD installer: %w", err)
	}

	coreDNSPatcher, err := NewCoreDNSPatcher(kubeClient)
	if err != nil {
		return nil, fmt.Errorf("failed to create CoreDNS patcher: %w", err)
	}

	return &Installer{
		config:         cfg,
		vip:            vip,
		helmClient:     helmClient,
		kubeClient:     kubeClient,
		helmInstaller:  helmInstaller,
		crdInstaller:   crdInstaller,
		coreDNSPatcher: coreDNSPatcher,
	}, nil
}

// Install performs the complete Tailscale operator installation.
// This is the main entry point called by the Foundry stack installer.
func (i *Installer) Install(ctx context.Context) error {
	// Step 1: Validate prerequisites
	if err := i.validatePrerequisites(ctx); err != nil {
		return fmt.Errorf("prerequisites check failed: %w", err)
	}

	// Step 2: Create namespace
	if err := i.createNamespace(ctx); err != nil {
		return fmt.Errorf("failed to create namespace: %w", err)
	}

	// Step 3: Add Helm repository
	if err := i.helmInstaller.AddRepository(ctx); err != nil {
		return fmt.Errorf("failed to add Helm repository: %w", err)
	}

	// Step 4: Install Tailscale operator via Helm
	if err := i.helmInstaller.InstallOperator(ctx); err != nil {
		return fmt.Errorf("failed to install operator: %w", err)
	}

	// Step 5: Deploy Connector CRD for subnet route advertisement
	if err := i.crdInstaller.DeployConnector(ctx); err != nil {
		return fmt.Errorf("failed to deploy Connector CRD: %w", err)
	}

	// Step 6: Deploy DNSConfig CRD for Magic DNS
	if err := i.crdInstaller.DeployDNSConfig(ctx); err != nil {
		return fmt.Errorf("failed to deploy DNSConfig CRD: %w", err)
	}

	// Step 7: Patch CoreDNS for .ts.net forwarding
	if err := i.coreDNSPatcher.PatchCoreDNS(ctx); err != nil {
		return fmt.Errorf("failed to patch CoreDNS: %w", err)
	}

	return nil
}

// Uninstall removes the Tailscale operator and all associated resources.
func (i *Installer) Uninstall(ctx context.Context) error {
	// Uninstall in reverse order
	if err := i.helmInstaller.UninstallOperator(ctx); err != nil {
		return fmt.Errorf("failed to uninstall operator: %w", err)
	}

	return nil
}

// Status returns the current status of the Tailscale operator installation.
func (i *Installer) Status(ctx context.Context) (*Status, error) {
	// For now, return a basic status
	// In a real implementation, this would query Kubernetes for pod status, etc.
	return &Status{
		Installed: true,
		Namespace: DefaultNamespace,
		Operator:  "running", // Placeholder
		Connector: "active",  // Placeholder
		DNSConfig: "active",  // Placeholder
	}, nil
}

// Status represents the current status of the Tailscale installation.
type Status struct {
	Installed bool
	Namespace string
	Operator  string
	Connector string
	DNSConfig string
}

// validatePrerequisites checks that all required dependencies are available.
func (i *Installer) validatePrerequisites(ctx context.Context) error {
	// TODO (PR #2b): Check K3s is running
	// TODO (PR #2g): Check OpenBAO is available (for secrets)

	// For now, just validate config again
	if err := i.config.Validate(); err != nil {
		return fmt.Errorf("configuration invalid: %w", err)
	}

	// Validate VIP is set
	if i.vip == "" {
		return fmt.Errorf("VIP cannot be empty")
	}

	return nil
}

// createNamespace creates the Tailscale namespace if it doesn't exist.
func (i *Installer) createNamespace(ctx context.Context) error {
	// Create namespace manifest
	namespace := map[string]interface{}{
		"apiVersion": "v1",
		"kind":       "Namespace",
		"metadata": map[string]interface{}{
			"name": DefaultNamespace,
		},
	}

	// Apply namespace (idempotent - will create if doesn't exist)
	if err := i.kubeClient.Apply(ctx, namespace); err != nil {
		return fmt.Errorf("failed to create namespace %s: %w", DefaultNamespace, err)
	}

	return nil
}
