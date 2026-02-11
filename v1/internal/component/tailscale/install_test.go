package tailscale

import (
	"context"
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
				OAuthClientSecret: stringPtr("secret-456"),
			},
			vip:     "100.81.89.100",
			wantErr: true,
			errMsg:  "invalid configuration: oauth_client_id is required",
		},
		{
			name: "valid config",
			config: &Config{
				OAuthClientID:     stringPtr("client-123"),
				OAuthClientSecret: stringPtr("secret-456"),
			},
			vip:     "100.81.89.100",
			wantErr: false,
		},
		{
			name: "valid config with custom settings",
			config: &Config{
				OAuthClientID:     stringPtr("client-123"),
				OAuthClientSecret: stringPtr("secret-456"),
				OperatorImage:     stringPtr("custom/operator:v1.0.0"),
				Tags:              []string{"tag:custom"},
			},
			vip:     "100.81.89.100",
			wantErr: false,
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
		OAuthClientID:     stringPtr("client-123"),
		OAuthClientSecret: stringPtr("secret-456"),
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

func TestInstaller_ValidatePrerequisites(t *testing.T) {
	tests := []struct {
		name    string
		config  *Config
		vip     string
		wantErr bool
		errMsg  string
	}{
		{
			name: "valid prerequisites",
			config: &Config{
				OAuthClientID:     stringPtr("client-123"),
				OAuthClientSecret: stringPtr("secret-456"),
			},
			vip:     "100.81.89.100",
			wantErr: false,
		},
		{
			name: "empty VIP",
			config: &Config{
				OAuthClientID:     stringPtr("client-123"),
				OAuthClientSecret: stringPtr("secret-456"),
			},
			vip:     "",
			wantErr: true,
			errMsg:  "VIP cannot be empty",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			installer := &Installer{
				config: tt.config,
				vip:    tt.vip,
			}

			err := installer.validatePrerequisites(context.Background())
			if (err != nil) != tt.wantErr {
				t.Errorf("validatePrerequisites() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if err != nil && tt.errMsg != "" && err.Error() != tt.errMsg {
				t.Errorf("validatePrerequisites() error message = %q, want %q", err.Error(), tt.errMsg)
			}
		})
	}
}

func TestInstaller_Install(t *testing.T) {
	config := &Config{
		OAuthClientID:     stringPtr("client-123"),
		OAuthClientSecret: stringPtr("secret-456"),
	}

	installer, err := NewInstaller(config, "100.81.89.100")
	if err != nil {
		t.Fatalf("NewInstaller() unexpected error: %v", err)
	}

	// Install should succeed (currently just validates prerequisites and creates namespace stub)
	err = installer.Install(context.Background())
	if err != nil {
		t.Errorf("Install() unexpected error: %v", err)
	}
}

func TestInstaller_Uninstall(t *testing.T) {
	config := &Config{
		OAuthClientID:     stringPtr("client-123"),
		OAuthClientSecret: stringPtr("secret-456"),
	}

	installer, err := NewInstaller(config, "100.81.89.100")
	if err != nil {
		t.Fatalf("NewInstaller() unexpected error: %v", err)
	}

	// Uninstall is not implemented yet, should return error
	err = installer.Uninstall(context.Background())
	if err == nil {
		t.Error("Uninstall() should return error (not yet implemented)")
	}
}

func TestInstaller_Status(t *testing.T) {
	config := &Config{
		OAuthClientID:     stringPtr("client-123"),
		OAuthClientSecret: stringPtr("secret-456"),
	}

	installer, err := NewInstaller(config, "100.81.89.100")
	if err != nil {
		t.Fatalf("NewInstaller() unexpected error: %v", err)
	}

	// Status is not implemented yet
	status, err := installer.Status(context.Background())
	if err != nil {
		t.Errorf("Status() unexpected error: %v", err)
	}
	if status != "not implemented" {
		t.Errorf("Status() = %q, want %q", status, "not implemented")
	}
}
