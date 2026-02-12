package tailscale

import (
	"context"
	"fmt"
	"testing"
)

// mockOpenBAOClient is a mock implementation of OpenBAOClient for testing.
type mockOpenBAOClient struct {
	secrets map[string]*Secret
	readErr error
}

func (m *mockOpenBAOClient) Read(ctx context.Context, path string) (*Secret, error) {
	if m.readErr != nil {
		return nil, m.readErr
	}
	secret, ok := m.secrets[path]
	if !ok {
		return nil, fmt.Errorf("secret not found at path %q", path)
	}
	return secret, nil
}

func TestNewSecretResolver(t *testing.T) {
	client := &mockOpenBAOClient{}
	resolver := NewSecretResolver(client)
	if resolver == nil {
		t.Fatal("NewSecretResolver() returned nil")
	}
	if resolver.client != client {
		t.Error("NewSecretResolver() did not set client")
	}
}

func TestIsSecretReference(t *testing.T) {
	tests := []struct {
		name  string
		value string
		want  bool
	}{
		{
			name:  "valid secret reference",
			value: "${secret:path/to/secret:key}",
			want:  true,
		},
		{
			name:  "valid secret reference with colon in path",
			value: "${secret:foundry-core/tailscale:client_id}",
			want:  true,
		},
		{
			name:  "literal value",
			value: "literal-client-id",
			want:  false,
		},
		{
			name:  "missing closing brace",
			value: "${secret:path:key",
			want:  false,
		},
		{
			name:  "wrong prefix",
			value: "${config:path:key}",
			want:  false,
		},
		{
			name:  "empty string",
			value: "",
			want:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isSecretReference(tt.value)
			if got != tt.want {
				t.Errorf("isSecretReference(%q) = %v, want %v", tt.value, got, tt.want)
			}
		})
	}
}

func TestParseSecretReference(t *testing.T) {
	tests := []struct {
		name     string
		value    string
		wantPath string
		wantKey  string
		wantErr  bool
	}{
		{
			name:     "simple reference",
			value:    "${secret:path/to/secret:key}",
			wantPath: "path/to/secret",
			wantKey:  "key",
			wantErr:  false,
		},
		{
			name:     "reference with colon in path",
			value:    "${secret:foundry-core/tailscale:client_id}",
			wantPath: "foundry-core/tailscale",
			wantKey:  "client_id",
			wantErr:  false,
		},
		{
			name:     "reference with multiple colons",
			value:    "${secret:a:b:c:key}",
			wantPath: "a:b:c",
			wantKey:  "key",
			wantErr:  false,
		},
		{
			name:    "missing key",
			value:   "${secret:path}",
			wantErr: true,
		},
		{
			name:    "not a reference",
			value:   "literal-value",
			wantErr: true,
		},
		{
			name:    "empty path",
			value:   "${secret::key}",
			wantErr: true,
		},
		{
			name:    "empty key",
			value:   "${secret:path:}",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path, key, err := parseSecretReference(tt.value)
			if (err != nil) != tt.wantErr {
				t.Errorf("parseSecretReference(%q) error = %v, wantErr %v", tt.value, err, tt.wantErr)
				return
			}
			if !tt.wantErr {
				if path != tt.wantPath {
					t.Errorf("parseSecretReference(%q) path = %q, want %q", tt.value, path, tt.wantPath)
				}
				if key != tt.wantKey {
					t.Errorf("parseSecretReference(%q) key = %q, want %q", tt.value, key, tt.wantKey)
				}
			}
		})
	}
}

func TestSecretResolver_Resolve_LiteralValues(t *testing.T) {
	client := &mockOpenBAOClient{}
	resolver := NewSecretResolver(client)

	tests := []struct {
		name  string
		value string
		want  string
	}{
		{
			name:  "simple literal",
			value: "literal-client-id",
			want:  "literal-client-id",
		},
		{
			name:  "UUID literal",
			value: "550e8400-e29b-41d4-a716-446655440000",
			want:  "550e8400-e29b-41d4-a716-446655440000",
		},
		{
			name:  "empty string",
			value: "",
			want:  "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := resolver.Resolve(context.Background(), tt.value)
			if err != nil {
				t.Errorf("Resolve() unexpected error: %v", err)
				return
			}
			if got != tt.want {
				t.Errorf("Resolve(%q) = %q, want %q", tt.value, got, tt.want)
			}
		})
	}
}

func TestSecretResolver_Resolve_SecretReferences(t *testing.T) {
	tests := []struct {
		name    string
		value   string
		secrets map[string]*Secret
		want    string
		wantErr bool
		errContains string
	}{
		{
			name:  "successful resolution",
			value: "${secret:foundry-core/tailscale:client_id}",
			secrets: map[string]*Secret{
				"foundry-core/tailscale": {
					Data: map[string]interface{}{
						"client_id":     "my-client-id",
						"client_secret": "my-client-secret",
					},
				},
			},
			want:    "my-client-id",
			wantErr: false,
		},
		{
			name:  "secret not found",
			value: "${secret:nonexistent/path:key}",
			secrets: map[string]*Secret{},
			wantErr: true,
			errContains: "failed to read secret from OpenBAO",
		},
		{
			name:  "key not found in secret",
			value: "${secret:foundry-core/tailscale:nonexistent_key}",
			secrets: map[string]*Secret{
				"foundry-core/tailscale": {
					Data: map[string]interface{}{
						"client_id": "my-client-id",
					},
				},
			},
			wantErr: true,
			errContains: "key \"nonexistent_key\" not found",
		},
		{
			name:  "value is not a string",
			value: "${secret:foundry-core/tailscale:numeric_value}",
			secrets: map[string]*Secret{
				"foundry-core/tailscale": {
					Data: map[string]interface{}{
						"numeric_value": 12345,
					},
				},
			},
			wantErr: true,
			errContains: "is not a string",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := &mockOpenBAOClient{
				secrets: tt.secrets,
			}
			resolver := NewSecretResolver(client)

			got, err := resolver.Resolve(context.Background(), tt.value)
			if (err != nil) != tt.wantErr {
				t.Errorf("Resolve() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if err != nil && tt.errContains != "" && !contains(err.Error(), tt.errContains) {
				t.Errorf("Resolve() error = %q, want to contain %q", err.Error(), tt.errContains)
				return
			}
			if !tt.wantErr && got != tt.want {
				t.Errorf("Resolve() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestSecretResolver_Resolve_NilClient(t *testing.T) {
	resolver := NewSecretResolver(nil)

	tests := []struct {
		name    string
		value   string
		want    string
		wantErr bool
	}{
		{
			name:    "literal value with nil client",
			value:   "literal-value",
			want:    "literal-value",
			wantErr: false,
		},
		{
			name:    "secret reference with nil client",
			value:   "${secret:path:key}",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := resolver.Resolve(context.Background(), tt.value)
			if (err != nil) != tt.wantErr {
				t.Errorf("Resolve() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && got != tt.want {
				t.Errorf("Resolve() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestSecretResolver_Resolve_OpenBAOError(t *testing.T) {
	client := &mockOpenBAOClient{
		readErr: fmt.Errorf("connection timeout"),
	}
	resolver := NewSecretResolver(client)

	_, err := resolver.Resolve(context.Background(), "${secret:path:key}")
	if err == nil {
		t.Fatal("Resolve() expected error, got nil")
	}
	if !contains(err.Error(), "failed to read secret from OpenBAO") {
		t.Errorf("Resolve() error = %q, want to contain %q", err.Error(), "failed to read secret from OpenBAO")
	}
}

func TestSecretResolver_ResolveConfig(t *testing.T) {
	tests := []struct {
		name              string
		config            *Config
		secrets           map[string]*Secret
		wantClientID      string
		wantClientSecret  string
		wantErr           bool
		errContains       string
	}{
		{
			name: "resolve both OAuth fields",
			config: &Config{
				OAuthClientID:     stringPtr("${secret:foundry-core/tailscale:client_id}"),
				OAuthClientSecret: stringPtr("${secret:foundry-core/tailscale:client_secret}"),
			},
			secrets: map[string]*Secret{
				"foundry-core/tailscale": {
					Data: map[string]interface{}{
						"client_id":     "resolved-client-id",
						"client_secret": "resolved-client-secret",
					},
				},
			},
			wantClientID:     "resolved-client-id",
			wantClientSecret: "resolved-client-secret",
			wantErr:          false,
		},
		{
			name: "literal values unchanged",
			config: &Config{
				OAuthClientID:     stringPtr("literal-client-id"),
				OAuthClientSecret: stringPtr("literal-client-secret"),
			},
			secrets:          map[string]*Secret{},
			wantClientID:     "literal-client-id",
			wantClientSecret: "literal-client-secret",
			wantErr:          false,
		},
		{
			name: "mixed literal and secret",
			config: &Config{
				OAuthClientID:     stringPtr("literal-client-id"),
				OAuthClientSecret: stringPtr("${secret:foundry-core/tailscale:client_secret}"),
			},
			secrets: map[string]*Secret{
				"foundry-core/tailscale": {
					Data: map[string]interface{}{
						"client_secret": "resolved-client-secret",
					},
				},
			},
			wantClientID:     "literal-client-id",
			wantClientSecret: "resolved-client-secret",
			wantErr:          false,
		},
		{
			name: "client_id resolution error",
			config: &Config{
				OAuthClientID:     stringPtr("${secret:nonexistent/path:key}"),
				OAuthClientSecret: stringPtr("literal-secret"),
			},
			secrets:     map[string]*Secret{},
			wantErr:     true,
			errContains: "failed to resolve oauth_client_id",
		},
		{
			name: "client_secret resolution error",
			config: &Config{
				OAuthClientID:     stringPtr("literal-id"),
				OAuthClientSecret: stringPtr("${secret:nonexistent/path:key}"),
			},
			secrets:     map[string]*Secret{},
			wantErr:     true,
			errContains: "failed to resolve oauth_client_secret",
		},
		{
			name:    "nil config",
			config:  nil,
			secrets: map[string]*Secret{},
			wantErr: true,
			errContains: "config cannot be nil",
		},
		{
			name: "empty strings not resolved",
			config: &Config{
				OAuthClientID:     stringPtr(""),
				OAuthClientSecret: stringPtr(""),
			},
			secrets:          map[string]*Secret{},
			wantClientID:     "",
			wantClientSecret: "",
			wantErr:          false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := &mockOpenBAOClient{
				secrets: tt.secrets,
			}
			resolver := NewSecretResolver(client)

			err := resolver.ResolveConfig(context.Background(), tt.config)
			if (err != nil) != tt.wantErr {
				t.Errorf("ResolveConfig() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if err != nil && tt.errContains != "" && !contains(err.Error(), tt.errContains) {
				t.Errorf("ResolveConfig() error = %q, want to contain %q", err.Error(), tt.errContains)
				return
			}

			if !tt.wantErr && tt.config != nil {
				if tt.config.OAuthClientID != nil && *tt.config.OAuthClientID != tt.wantClientID {
					t.Errorf("OAuthClientID = %q, want %q", *tt.config.OAuthClientID, tt.wantClientID)
				}
				if tt.config.OAuthClientSecret != nil && *tt.config.OAuthClientSecret != tt.wantClientSecret {
					t.Errorf("OAuthClientSecret = %q, want %q", *tt.config.OAuthClientSecret, tt.wantClientSecret)
				}
			}
		})
	}
}

func TestNewSecretResolverWithFallback(t *testing.T) {
	// Test with nil client
	resolver := NewSecretResolverWithFallback(nil)
	if resolver == nil {
		t.Fatal("NewSecretResolverWithFallback() returned nil")
	}

	// Should handle literal values even with nil client
	val, err := resolver.Resolve(context.Background(), "literal-value")
	if err != nil {
		t.Errorf("Resolve() with literal value unexpected error: %v", err)
	}
	if val != "literal-value" {
		t.Errorf("Resolve() = %q, want %q", val, "literal-value")
	}

	// Test with real client
	client := &mockOpenBAOClient{
		secrets: map[string]*Secret{
			"test": {
				Data: map[string]interface{}{
					"key": "value",
				},
			},
		},
	}
	resolver = NewSecretResolverWithFallback(client)
	val, err = resolver.Resolve(context.Background(), "${secret:test:key}")
	if err != nil {
		t.Errorf("Resolve() with secret reference unexpected error: %v", err)
	}
	if val != "value" {
		t.Errorf("Resolve() = %q, want %q", val, "value")
	}
}
