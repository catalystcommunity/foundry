package tailscale

import (
	"fmt"
)

// Config holds Tailscale operator component configuration
type Config struct {
	// OAuthClientID is the OAuth client ID for Tailscale
	OAuthClientID *string `json:"oauth_client_id" yaml:"oauth_client_id"`

	// OAuthClientSecret is the OAuth client secret for Tailscale
	OAuthClientSecret *string `json:"oauth_client_secret" yaml:"oauth_client_secret"`

	// OperatorImage is the image for the Tailscale operator
	OperatorImage *string `json:"operator_image" yaml:"operator_image"`

	// Tags are the Tailscale tags to apply to the node
	Tags []string `json:"tags" yaml:"tags"`

	// AdvertiseRoutes are the routes to advertise to Tailscale
	AdvertiseRoutes []string `json:"advertise_routes" yaml:"advertise_routes"`

	// Version is the Helm chart version to install
	Version string `json:"version" yaml:"version"`

	// Namespace for Tailscale operator deployment
	Namespace string `json:"namespace" yaml:"namespace"`

	// CustomDomain is a custom DNS domain to use instead of the default ts.net
	// For example: "soypetetech.local" will make ingresses available at
	// grafana.soypetetech.local instead of grafana.<random>.ts.net
	CustomDomain string `json:"custom_domain" yaml:"custom_domain"`
}

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

	// CustomDomain defaults to empty (uses default ts.net domain)
	if c.CustomDomain == "" {
		c.CustomDomain = ""
	}
}
