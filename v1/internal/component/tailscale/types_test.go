package tailscale

import (
	"testing"

	"gopkg.in/yaml.v3"
)

func TestConfig_MarshalUnmarshal(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		wantNil  map[string]bool // which fields should be nil
		wantVals map[string]interface{} // which fields should have values
	}{
		{
			name: "all fields populated",
			input: `
oauth_client_id: "client-123"
oauth_client_secret: "secret-456"
operator_image: "tailscale/operator:v1.2.3"
advertise_routes:
  - "10.0.0.0/8"
  - "192.168.0.0/16"
tags:
  - "tag:k8s-foundry"
  - "tag:production"
`,
			wantVals: map[string]interface{}{
				"oauth_client_id":     "client-123",
				"oauth_client_secret": "secret-456",
				"operator_image":      "tailscale/operator:v1.2.3",
			},
		},
		{
			name: "minimal config with defaults",
			input: `
oauth_client_id: "client-123"
oauth_client_secret: "secret-456"
`,
			wantNil: map[string]bool{
				"operator_image":    true,
				"advertise_routes":  false, // slice, not pointer
				"tags":              false, // slice, not pointer
			},
			wantVals: map[string]interface{}{
				"oauth_client_id":     "client-123",
				"oauth_client_secret": "secret-456",
			},
		},
		{
			name: "with secret references",
			input: `
oauth_client_id: "${secret:foundry-core/tailscale:client_id}"
oauth_client_secret: "${secret:foundry-core/tailscale:client_secret}"
`,
			wantVals: map[string]interface{}{
				"oauth_client_id":     "${secret:foundry-core/tailscale:client_id}",
				"oauth_client_secret": "${secret:foundry-core/tailscale:client_secret}",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var cfg Config
			if err := yaml.Unmarshal([]byte(tt.input), &cfg); err != nil {
				t.Fatalf("Failed to unmarshal: %v", err)
			}

			// Check nil fields
			for field, shouldBeNil := range tt.wantNil {
				switch field {
				case "operator_image":
					if shouldBeNil && cfg.OperatorImage != nil {
						t.Errorf("Expected %s to be nil, got: %v", field, *cfg.OperatorImage)
					}
				}
			}

			// Check values
			for field, wantVal := range tt.wantVals {
				var gotVal interface{}
				switch field {
				case "oauth_client_id":
					if cfg.OAuthClientID == nil {
						t.Errorf("Expected %s to be set, got nil", field)
						continue
					}
					gotVal = *cfg.OAuthClientID
				case "oauth_client_secret":
					if cfg.OAuthClientSecret == nil {
						t.Errorf("Expected %s to be set, got nil", field)
						continue
					}
					gotVal = *cfg.OAuthClientSecret
				case "operator_image":
					if cfg.OperatorImage == nil {
						t.Errorf("Expected %s to be set, got nil", field)
						continue
					}
					gotVal = *cfg.OperatorImage
				}

				if gotVal != wantVal {
					t.Errorf("%s: got %v, want %v", field, gotVal, wantVal)
				}
			}
		})
	}
}

func TestConfig_PointerTypes(t *testing.T) {
	// Test that optional fields are pointer types (*string)
	cfg := Config{}

	// All string fields should be nil by default
	if cfg.OAuthClientID != nil {
		t.Error("Expected OAuthClientID to be nil by default")
	}
	if cfg.OAuthClientSecret != nil {
		t.Error("Expected OAuthClientSecret to be nil by default")
	}
	if cfg.OperatorImage != nil {
		t.Error("Expected OperatorImage to be nil by default")
	}

	// Slices should be nil (not allocated)
	if cfg.AdvertiseRoutes != nil {
		t.Error("Expected AdvertiseRoutes to be nil by default")
	}
	if cfg.Tags != nil {
		t.Error("Expected Tags to be nil by default")
	}
}

func TestConfig_YAMLRoundTrip(t *testing.T) {
	clientID := "client-123"
	clientSecret := "secret-456"
	image := "tailscale/operator:latest"

	original := Config{
		OAuthClientID:     &clientID,
		OAuthClientSecret: &clientSecret,
		OperatorImage:     &image,
		AdvertiseRoutes:   []string{"10.0.0.0/8", "192.168.0.0/16"},
		Tags:              []string{"tag:k8s-foundry"},
	}

	// Marshal to YAML
	data, err := yaml.Marshal(&original)
	if err != nil {
		t.Fatalf("Failed to marshal: %v", err)
	}

	// Unmarshal back
	var roundtrip Config
	if err := yaml.Unmarshal(data, &roundtrip); err != nil {
		t.Fatalf("Failed to unmarshal: %v", err)
	}

	// Compare
	if *roundtrip.OAuthClientID != *original.OAuthClientID {
		t.Errorf("OAuthClientID mismatch: got %v, want %v", *roundtrip.OAuthClientID, *original.OAuthClientID)
	}
	if *roundtrip.OAuthClientSecret != *original.OAuthClientSecret {
		t.Errorf("OAuthClientSecret mismatch: got %v, want %v", *roundtrip.OAuthClientSecret, *original.OAuthClientSecret)
	}
	if *roundtrip.OperatorImage != *original.OperatorImage {
		t.Errorf("OperatorImage mismatch: got %v, want %v", *roundtrip.OperatorImage, *original.OperatorImage)
	}
	if len(roundtrip.AdvertiseRoutes) != len(original.AdvertiseRoutes) {
		t.Errorf("AdvertiseRoutes length mismatch: got %d, want %d", len(roundtrip.AdvertiseRoutes), len(original.AdvertiseRoutes))
	}
	if len(roundtrip.Tags) != len(original.Tags) {
		t.Errorf("Tags length mismatch: got %d, want %d", len(roundtrip.Tags), len(original.Tags))
	}
}

func TestConfig_EmptyYAML(t *testing.T) {
	// Test that empty YAML unmarshals to zero value
	var cfg Config
	if err := yaml.Unmarshal([]byte("{}"), &cfg); err != nil {
		t.Fatalf("Failed to unmarshal empty YAML: %v", err)
	}

	if cfg.OAuthClientID != nil {
		t.Error("Expected nil OAuthClientID from empty YAML")
	}
	if cfg.OAuthClientSecret != nil {
		t.Error("Expected nil OAuthClientSecret from empty YAML")
	}
}
