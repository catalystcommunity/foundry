package tailscale

import (
	"slices"
	"testing"
)

func TestNewInstaller(t *testing.T) {
	tests := []struct {
		name    string
		config  *Config
		vip     string
		wantErr bool
		errMsg  string
	}{
		{
			name:    "nil config",
			config:  nil,
			vip:     "100.81.89.100",
			wantErr: true,
			errMsg:  "config cannot be nil",
		},
		{
			name: "invalid config - missing oauth_client_id",
			config: &Config{
				OAuthClientSecret: installerStringPtr("secret-456"),
			},
			vip:     "100.81.89.100",
			wantErr: true,
			errMsg:  "invalid configuration: oauth_client_id is required",
		},
		{
			name: "valid config",
			config: &Config{
				OAuthClientID:     installerStringPtr("client-123"),
				OAuthClientSecret: installerStringPtr("secret-456"),
			},
			vip:     "100.81.89.100",
			wantErr: false,
		},
		{
			name: "valid config with custom settings",
			config: &Config{
				OAuthClientID:     installerStringPtr("client-123"),
				OAuthClientSecret: installerStringPtr("secret-456"),
				OperatorImage:     installerStringPtr("custom/operator:v1.0.0"),
				Tags:              []string{"tag:custom"},
			},
			vip:     "100.81.89.100",
			wantErr: false,
		},
		{
			name: "empty VIP",
			config: &Config{
				OAuthClientID:     installerStringPtr("client-123"),
				OAuthClientSecret: installerStringPtr("secret-456"),
			},
			vip:     "",
			wantErr: true,
			errMsg:  "VIP cannot be empty",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			installer, err := NewInstaller(tt.config, tt.vip)
			if (err != nil) != tt.wantErr {
				t.Errorf("NewInstaller() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if err != nil && tt.errMsg != "" && err.Error() != tt.errMsg {
				t.Errorf("NewInstaller() error message = %q, want %q", err.Error(), tt.errMsg)
				return
			}
			if !tt.wantErr && installer == nil {
				t.Error("NewInstaller() returned nil installer without error")
			}
		})
	}
}

func TestNewInstaller_SetsDefaults(t *testing.T) {
	config := &Config{
		OAuthClientID:     installerStringPtr("client-123"),
		OAuthClientSecret: installerStringPtr("secret-456"),
	}

	installer, err := NewInstaller(config, "100.81.89.100")
	if err != nil {
		t.Fatalf("NewInstaller() unexpected error: %v", err)
	}

	// Verify defaults were set
	if installer.config.OperatorImage == nil {
		t.Error("Expected OperatorImage to be set by defaults")
	}
	if installer.config.Tags == nil || len(installer.config.Tags) == 0 {
		t.Error("Expected Tags to be set by defaults")
	}
	if installer.config.AdvertiseRoutes == nil {
		t.Error("Expected AdvertiseRoutes to be initialized by defaults")
	}
}

func TestNewInstaller_PreservesCustomSettings(t *testing.T) {
	operatorImage := "custom/operator:v1.0.0"
	tags := []string{"tag:custom", "tag:production"}
	routes := []string{"10.0.0.0/8", "192.168.0.0/16"}
	config := &Config{
		OAuthClientID:     installerStringPtr("client-123"),
		OAuthClientSecret: installerStringPtr("secret-456"),
		OperatorImage:     &operatorImage,
		Tags:              tags,
		AdvertiseRoutes:   routes,
	}

	installer, err := NewInstaller(config, "100.81.89.100")
	if err != nil {
		t.Fatalf("NewInstaller() unexpected error: %v", err)
	}

	if installer.config.OperatorImage == nil || *installer.config.OperatorImage != operatorImage {
		t.Errorf("OperatorImage = %v, want %q", installer.config.OperatorImage, operatorImage)
	}
	if !slices.Equal(installer.config.Tags, tags) {
		t.Errorf("Tags = %v, want %v", installer.config.Tags, tags)
	}
	if !slices.Equal(installer.config.AdvertiseRoutes, routes) {
		t.Errorf("AdvertiseRoutes = %v, want %v", installer.config.AdvertiseRoutes, routes)
	}
}

func installerStringPtr(s string) *string {
	return &s
}
