package tailscale

import (
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

	// Validate VIP is set
	if vip == "" {
		return nil, fmt.Errorf("VIP cannot be empty")
	}

	// Set defaults after validation succeeds
	cfg.SetDefaults()

	return &Installer{
		config: cfg,
		vip:    vip,
	}, nil
}
