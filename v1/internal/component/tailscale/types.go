package tailscale

import (
	"fmt"
)

// Config is the Tailscale operator component configuration.
//
// Hand-written (like grafana/velero) rather than CSIL-generated: the Tailscale
// component is not persisted through the stack config schema, so it does not
// have a csil/v1/components/tailscale.csil source.
//
// OAuth credential and operator-image fields are pointers so callers can
// distinguish "not set" (nil) from an explicit empty string; SetDefaults fills
// in defaults only when a field is nil.
type Config struct {
	// OAuthClientID is the Tailscale OAuth client ID (required).
	OAuthClientID *string `json:"oauth_client_id" yaml:"oauth_client_id"`

	// OAuthClientSecret is the Tailscale OAuth client secret (required).
	OAuthClientSecret *string `json:"oauth_client_secret" yaml:"oauth_client_secret"`

	// OperatorImage overrides the Tailscale operator image
	// (default: tailscale/operator:latest).
	OperatorImage *string `json:"operator_image" yaml:"operator_image"`

	// Tags are the Tailscale ACL tags applied to operator-managed devices
	// (default: ["tag:k8s-foundry"]).
	Tags []string `json:"tags" yaml:"tags"`

	// AdvertiseRoutes are the subnet routes the operator advertises; the VIP
	// route is appended during installation.
	AdvertiseRoutes []string `json:"advertise_routes" yaml:"advertise_routes"`
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
}
