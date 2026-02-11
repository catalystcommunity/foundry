package tailscale

import (
	"fmt"
)

// Validate checks that the Tailscale configuration is valid.
// It ensures required OAuth credentials are provided.
func (c *Config) Validate() error {
	if c == nil {
		return fmt.Errorf("config cannot be nil")
	}

	// OAuth credentials are required
	if c.OAuthClientID == nil || *c.OAuthClientID == "" {
		return fmt.Errorf("oauth_client_id is required")
	}
	if c.OAuthClientSecret == nil || *c.OAuthClientSecret == "" {
		return fmt.Errorf("oauth_client_secret is required")
	}

	return nil
}

// SetDefaults sets default values for optional fields if not provided.
func (c *Config) SetDefaults() {
	if c == nil {
		return
	}

	// Set default operator image if not specified
	if c.OperatorImage == nil {
		image := "tailscale/operator:latest"
		c.OperatorImage = &image
	}

	// Set default tags if not specified
	if c.Tags == nil {
		c.Tags = []string{"tag:k8s-foundry"}
	}

	// Initialize advertise_routes as empty slice if nil
	// (will be populated with VIP route during installation)
	if c.AdvertiseRoutes == nil {
		c.AdvertiseRoutes = []string{}
	}
}
