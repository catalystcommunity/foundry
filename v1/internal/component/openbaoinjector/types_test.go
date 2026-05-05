package openbaoinjector

import (
	"testing"

	"github.com/catalystcommunity/foundry/v1/internal/component"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()

	assert.Equal(t, "0.26.2", cfg.Version)
	assert.Equal(t, "openbao", cfg.Namespace)
	assert.Equal(t, "", cfg.ExternalVaultAddr)
}

func TestConfig_Validate(t *testing.T) {
	tests := []struct {
		name    string
		cfg     *Config
		wantErr bool
		errMsg  string
	}{
		{
			name: "valid config",
			cfg: &Config{
				Version:           "0.26.2",
				Namespace:         "openbao",
				ExternalVaultAddr: "http://10.0.0.1:8200",
			},
			wantErr: false,
		},
		{
			name: "missing version",
			cfg: &Config{
				Version:           "",
				Namespace:         "openbao",
				ExternalVaultAddr: "http://10.0.0.1:8200",
			},
			wantErr: true,
			errMsg:  "version is required",
		},
		{
			name: "missing namespace",
			cfg: &Config{
				Version:           "0.26.2",
				Namespace:         "",
				ExternalVaultAddr: "http://10.0.0.1:8200",
			},
			wantErr: true,
			errMsg:  "namespace is required",
		},
		{
			name: "missing external_vault_addr",
			cfg: &Config{
				Version:           "0.26.2",
				Namespace:         "openbao",
				ExternalVaultAddr: "",
			},
			wantErr: true,
			errMsg:  "external_vault_addr is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.cfg.Validate()
			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errMsg)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestParseConfig(t *testing.T) {
	tests := []struct {
		name     string
		input    component.ComponentConfig
		expected *Config
		wantErr  bool
		errMsg   string
	}{
		{
			name:    "empty config fails - missing external_vault_addr",
			input:   component.ComponentConfig{},
			wantErr: true,
			errMsg:  "external_vault_addr is required",
		},
		{
			name: "valid config with defaults",
			input: component.ComponentConfig{
				"external_vault_addr": "http://10.0.0.1:8200",
			},
			expected: &Config{
				Version:           "0.26.2",
				Namespace:         "openbao",
				ExternalVaultAddr: "http://10.0.0.1:8200",
			},
			wantErr: false,
		},
		{
			name: "override version",
			input: component.ComponentConfig{
				"external_vault_addr": "http://10.0.0.1:8200",
				"version":             "0.27.0",
			},
			expected: &Config{
				Version:           "0.27.0",
				Namespace:         "openbao",
				ExternalVaultAddr: "http://10.0.0.1:8200",
			},
			wantErr: false,
		},
		{
			name: "override namespace",
			input: component.ComponentConfig{
				"external_vault_addr": "http://10.0.0.1:8200",
				"namespace":           "custom-ns",
			},
			expected: &Config{
				Version:           "0.26.2",
				Namespace:         "custom-ns",
				ExternalVaultAddr: "http://10.0.0.1:8200",
			},
			wantErr: false,
		},
		{
			name: "override all fields",
			input: component.ComponentConfig{
				"external_vault_addr": "https://openbao.example.com:8200",
				"version":             "0.27.0",
				"namespace":           "vaultns",
			},
			expected: &Config{
				Version:           "0.27.0",
				Namespace:         "vaultns",
				ExternalVaultAddr: "https://openbao.example.com:8200",
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := ParseConfig(tt.input)
			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errMsg)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.expected, result)
			}
		})
	}
}

func TestComponent_Name(t *testing.T) {
	comp := NewComponent(nil, nil, nil)
	assert.Equal(t, "openbao-injector", comp.Name())
}

func TestComponent_Dependencies(t *testing.T) {
	comp := NewComponent(nil, nil, nil)
	deps := comp.Dependencies()
	assert.Contains(t, deps, "openbao")
	assert.Contains(t, deps, "k3s")
}
