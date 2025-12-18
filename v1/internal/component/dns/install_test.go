package dns

import (
	"context"
	"strings"
	"testing"

	"github.com/catalystcommunity/foundry/v1/internal/component"
	"github.com/catalystcommunity/foundry/v1/internal/ssh"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockSSHExecutor implements ssh.Executor for testing
type mockSSHExecutor struct {
	commands []string
	outputs  map[string]string
	errors   map[string]error
}

func newMockSSHExecutor() *mockSSHExecutor {
	return &mockSSHExecutor{
		commands: []string{},
		outputs:  make(map[string]string),
		errors:   make(map[string]error),
	}
}

func (m *mockSSHExecutor) Execute(cmd string) (string, error) {
	m.commands = append(m.commands, cmd)

	// Check for error response
	if err, ok := m.errors[cmd]; ok {
		return "", err
	}

	// Check for specific output
	if out, ok := m.outputs[cmd]; ok {
		return out, nil
	}

	// Default success
	return "", nil
}

func (m *mockSSHExecutor) setOutput(cmd, output string) {
	m.outputs[cmd] = output
}

func (m *mockSSHExecutor) setError(cmd string, err error) {
	m.errors[cmd] = err
}

func (m *mockSSHExecutor) hasCommand(substr string) bool {
	for _, cmd := range m.commands {
		if strings.Contains(cmd, substr) {
			return true
		}
	}
	return false
}

func TestGenerateAPIKey(t *testing.T) {
	key1, err := generateAPIKey()
	require.NoError(t, err)
	assert.NotEmpty(t, key1)
	assert.Len(t, key1, 64) // 32 bytes * 2 (hex encoding)

	// Generate another to ensure they're different
	key2, err := generateAPIKey()
	require.NoError(t, err)
	assert.NotEqual(t, key1, key2)
}

func TestConfigFromComponentConfig(t *testing.T) {
	tests := []struct {
		name      string
		input     component.ComponentConfig
		checkFunc func(t *testing.T, cfg *Config)
	}{
		{
			name:  "empty config uses defaults",
			input: component.ComponentConfig{},
			checkFunc: func(t *testing.T, cfg *Config) {
				assert.Equal(t, "49", cfg.ImageTag)
				assert.Equal(t, "gsqlite3", cfg.Backend)
				assert.Equal(t, []string{"8.8.8.8", "1.1.1.1"}, cfg.Forwarders)
			},
		},
		{
			name: "custom image tag",
			input: component.ComponentConfig{
				"image_tag": "50",
			},
			checkFunc: func(t *testing.T, cfg *Config) {
				assert.Equal(t, "50", cfg.ImageTag)
			},
		},
		{
			name: "custom api key",
			input: component.ComponentConfig{
				"api_key": "custom-key-123",
			},
			checkFunc: func(t *testing.T, cfg *Config) {
				assert.Equal(t, "custom-key-123", cfg.APIKey)
			},
		},
		{
			name: "custom forwarders",
			input: component.ComponentConfig{
				"forwarders": []string{"9.9.9.9"},
			},
			checkFunc: func(t *testing.T, cfg *Config) {
				assert.Equal(t, []string{"9.9.9.9"}, cfg.Forwarders)
			},
		},
		{
			name: "custom backend",
			input: component.ComponentConfig{
				"backend": "postgresql",
			},
			checkFunc: func(t *testing.T, cfg *Config) {
				assert.Equal(t, "postgresql", cfg.Backend)
			},
		},
		{
			name: "custom directories",
			input: component.ComponentConfig{
				"data_dir":   "/custom/data",
				"config_dir": "/custom/config",
			},
			checkFunc: func(t *testing.T, cfg *Config) {
				assert.Equal(t, "/custom/data", cfg.DataDir)
				assert.Equal(t, "/custom/config", cfg.ConfigDir)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg, _, err := configFromComponentConfig(tt.input)
			require.NoError(t, err)
			assert.NotNil(t, cfg)
			if tt.checkFunc != nil {
				tt.checkFunc(t, cfg)
			}
		})
	}
}

func TestBuildAuthExecStart(t *testing.T) {
	cfg := &Config{
		ConfigDir: "/etc/powerdns",
		DataDir:   "/var/lib/powerdns",
	}

	result := buildAuthExecStart("docker.io/powerdns/pdns-auth:49", cfg)

	assert.Contains(t, result, "docker run")
	assert.Contains(t, result, "--name powerdns-auth")
	assert.Contains(t, result, "--network=host")
	assert.Contains(t, result, "-v /etc/powerdns/auth:/etc/powerdns")
	assert.Contains(t, result, "-v /var/lib/powerdns:/var/lib/powerdns")
	assert.Contains(t, result, "docker.io/powerdns/pdns-auth:49")
	assert.Contains(t, result, "--config-dir=/etc/powerdns")
}

func TestBuildRecursorExecStart(t *testing.T) {
	cfg := &Config{
		ConfigDir: "/etc/powerdns",
	}

	result := buildRecursorExecStart("docker.io/powerdns/pdns-recursor:49", cfg)

	assert.Contains(t, result, "docker run")
	assert.Contains(t, result, "--name powerdns-recursor")
	assert.Contains(t, result, "--user root")
	assert.Contains(t, result, "--network=host")
	assert.Contains(t, result, "-v /etc/powerdns/recursor:/etc/powerdns-recursor")
	assert.Contains(t, result, "docker.io/powerdns/pdns-recursor:49")
	assert.Contains(t, result, "--config-dir=/etc/powerdns-recursor")
}

// Note: ExecStop was removed in favor of letting systemd handle container lifecycle
// via SIGTERM forwarding. See createSystemdServices for details on the pattern used.

// Note: These tests only verify command construction and error handling.
// Full integration tests with actual SSH connections are in integration tests.

func TestComponentInstallValidation(t *testing.T) {
	comp := NewComponent()
	ctx := context.Background()

	tests := []struct {
		name    string
		config  component.ComponentConfig
		wantErr bool
		errMsg  string
	}{
		{
			name:    "missing host",
			config:  component.ComponentConfig{},
			wantErr: true,
			errMsg:  "host connection is required",
		},
		{
			name: "missing api key",
			config: component.ComponentConfig{
				"host": &ssh.Connection{}, // Empty connection for validation test
			},
			wantErr: true,
			errMsg:  "API key is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := comp.Install(ctx, tt.config)
			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errMsg)
			} else {
				require.NoError(t, err)
			}
		})
	}
}
