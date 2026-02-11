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
	config *Config
	vip    string
}

// NewInstaller creates a new Tailscale installer with the given configuration.
func NewInstaller(cfg *Config, vip string) (*Installer, error) {
	if cfg == nil {
		return nil, fmt.Errorf("config cannot be nil")
	}

	// Validate configuration
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("invalid configuration: %w", err)
	}

	// Set defaults
	cfg.SetDefaults()

	return &Installer{
		config: cfg,
		vip:    vip,
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

	// TODO (PR #2c): Install operator via Helm
	// TODO (PR #2d): Deploy Connector CRD
	// TODO (PR #2d): Deploy DNSConfig CRD
	// TODO (PR #2e): Patch CoreDNS

	return nil
}

// Uninstall removes the Tailscale operator and all associated resources.
func (i *Installer) Uninstall(ctx context.Context) error {
	// TODO: Implement uninstall logic
	return fmt.Errorf("uninstall not yet implemented")
}

// Status returns the current status of the Tailscale operator installation.
func (i *Installer) Status(ctx context.Context) (string, error) {
	// TODO: Implement status check
	return "not implemented", nil
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
	// TODO (PR #2b): Actually create namespace via Kubernetes client
	// For now, this is a stub that will be implemented when we have
	// access to the Kubernetes client (passed in constructor or via context)

	// Placeholder implementation
	_ = DefaultNamespace
	return nil
}
