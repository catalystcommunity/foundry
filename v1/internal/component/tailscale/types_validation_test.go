package tailscale

import (
	"testing"
)

func TestConfig_Validate(t *testing.T) {
	tests := []struct {
		name    string
		config  *Config
		wantErr bool
		errMsg  string
	}{
		{
			name:    "nil config",
			config:  nil,
			wantErr: true,
			errMsg:  "config cannot be nil",
		},
		{
			name: "missing oauth_client_id",
			config: &Config{
				OAuthClientSecret: stringPtr("secret-456"),
			},
			wantErr: true,
			errMsg:  "oauth_client_id is required",
		},
		{
			name: "empty oauth_client_id",
			config: &Config{
				OAuthClientID:     stringPtr(""),
				OAuthClientSecret: stringPtr("secret-456"),
			},
			wantErr: true,
			errMsg:  "oauth_client_id is required",
		},
		{
			name: "missing oauth_client_secret",
			config: &Config{
				OAuthClientID: stringPtr("client-123"),
			},
			wantErr: true,
			errMsg:  "oauth_client_secret is required",
		},
		{
			name: "empty oauth_client_secret",
			config: &Config{
				OAuthClientID:     stringPtr("client-123"),
				OAuthClientSecret: stringPtr(""),
			},
			wantErr: true,
			errMsg:  "oauth_client_secret is required",
		},
		{
			name: "valid minimal config",
			config: &Config{
				OAuthClientID:     stringPtr("client-123"),
				OAuthClientSecret: stringPtr("secret-456"),
			},
			wantErr: false,
		},
		{
			name: "valid full config",
			config: &Config{
				OAuthClientID:     stringPtr("client-123"),
				OAuthClientSecret: stringPtr("secret-456"),
				OperatorImage:     stringPtr("tailscale/operator:v1.2.3"),
				AdvertiseRoutes:   []string{"10.0.0.0/8"},
				Tags:              []string{"tag:k8s-foundry", "tag:production"},
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if err != nil && tt.errMsg != "" && err.Error() != tt.errMsg {
				t.Errorf("Validate() error message = %q, want %q", err.Error(), tt.errMsg)
			}
		})
	}
}

func TestConfig_SetDefaults(t *testing.T) {
	tests := []struct {
		name   string
		config *Config
		check  func(t *testing.T, cfg *Config)
	}{
		{
			name:   "nil config does not panic",
			config: nil,
			check: func(t *testing.T, cfg *Config) {
				// Should not panic
			},
		},
		{
			name: "sets default operator image",
			config: &Config{
				OAuthClientID:     stringPtr("client-123"),
				OAuthClientSecret: stringPtr("secret-456"),
			},
			check: func(t *testing.T, cfg *Config) {
				if cfg.OperatorImage == nil {
					t.Error("Expected OperatorImage to be set")
					return
				}
				if *cfg.OperatorImage != "tailscale/operator:latest" {
					t.Errorf("OperatorImage = %q, want %q", *cfg.OperatorImage, "tailscale/operator:latest")
				}
			},
		},
		{
			name: "preserves custom operator image",
			config: &Config{
				OAuthClientID:     stringPtr("client-123"),
				OAuthClientSecret: stringPtr("secret-456"),
				OperatorImage:     stringPtr("custom/operator:v1.0.0"),
			},
			check: func(t *testing.T, cfg *Config) {
				if cfg.OperatorImage == nil {
					t.Error("Expected OperatorImage to be set")
					return
				}
				if *cfg.OperatorImage != "custom/operator:v1.0.0" {
					t.Errorf("OperatorImage = %q, want %q", *cfg.OperatorImage, "custom/operator:v1.0.0")
				}
			},
		},
		{
			name: "sets default tags",
			config: &Config{
				OAuthClientID:     stringPtr("client-123"),
				OAuthClientSecret: stringPtr("secret-456"),
			},
			check: func(t *testing.T, cfg *Config) {
				if cfg.Tags == nil {
					t.Error("Expected Tags to be set")
					return
				}
				if len(cfg.Tags) != 1 || cfg.Tags[0] != "tag:k8s-foundry" {
					t.Errorf("Tags = %v, want [tag:k8s-foundry]", cfg.Tags)
				}
			},
		},
		{
			name: "preserves custom tags",
			config: &Config{
				OAuthClientID:     stringPtr("client-123"),
				OAuthClientSecret: stringPtr("secret-456"),
				Tags:              []string{"tag:custom", "tag:production"},
			},
			check: func(t *testing.T, cfg *Config) {
				if len(cfg.Tags) != 2 {
					t.Errorf("Tags length = %d, want 2", len(cfg.Tags))
				}
			},
		},
		{
			name: "initializes empty advertise routes",
			config: &Config{
				OAuthClientID:     stringPtr("client-123"),
				OAuthClientSecret: stringPtr("secret-456"),
			},
			check: func(t *testing.T, cfg *Config) {
				if cfg.AdvertiseRoutes == nil {
					t.Error("Expected AdvertiseRoutes to be initialized")
					return
				}
				if len(cfg.AdvertiseRoutes) != 0 {
					t.Errorf("AdvertiseRoutes length = %d, want 0", len(cfg.AdvertiseRoutes))
				}
			},
		},
		{
			name: "preserves custom advertise routes",
			config: &Config{
				OAuthClientID:     stringPtr("client-123"),
				OAuthClientSecret: stringPtr("secret-456"),
				AdvertiseRoutes:   []string{"10.0.0.0/8", "192.168.0.0/16"},
			},
			check: func(t *testing.T, cfg *Config) {
				if len(cfg.AdvertiseRoutes) != 2 {
					t.Errorf("AdvertiseRoutes length = %d, want 2", len(cfg.AdvertiseRoutes))
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := tt.config
			cfg.SetDefaults()
			tt.check(t, cfg)
		})
	}
}

// Helper function to create string pointers
func stringPtr(s string) *string {
	return &s
}
