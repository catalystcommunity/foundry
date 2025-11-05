package dns

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGenerateAuthConfig(t *testing.T) {
	tests := []struct {
		name      string
		config    *Config
		wantErr   bool
		errMsg    string
		checkFunc func(t *testing.T, result string)
	}{
		{
			name:    "nil config",
			config:  nil,
			wantErr: true,
			errMsg:  "config cannot be nil",
		},
		{
			name: "missing API key",
			config: &Config{
				Backend: "sqlite",
				DataDir: "/var/lib/powerdns",
			},
			wantErr: true,
			errMsg:  "API key is required",
		},
		{
			name: "valid config",
			config: &Config{
				APIKey:  "test-api-key-123",
				Backend: "sqlite",
				DataDir: "/var/lib/powerdns",
			},
			wantErr: false,
			checkFunc: func(t *testing.T, result string) {
				assert.Contains(t, result, "api=yes")
				assert.Contains(t, result, "api-key=test-api-key-123")
				assert.Contains(t, result, "launch=sqlite")
				assert.Contains(t, result, "sqlite-database=/var/lib/powerdns/pdns.db")
				assert.Contains(t, result, "webserver=yes")
				assert.Contains(t, result, "webserver-port=8081")
			},
		},
		{
			name: "postgresql backend",
			config: &Config{
				APIKey:  "test-key",
				Backend: "postgresql",
				DataDir: "/custom/data",
			},
			wantErr: false,
			checkFunc: func(t *testing.T, result string) {
				assert.Contains(t, result, "launch=postgresql")
				assert.Contains(t, result, "postgresql-database=/custom/data/pdns.db")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := GenerateAuthConfig(tt.config)

			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errMsg)
				assert.Empty(t, result)
			} else {
				require.NoError(t, err)
				assert.NotEmpty(t, result)
				if tt.checkFunc != nil {
					tt.checkFunc(t, result)
				}
			}
		})
	}
}

func TestGenerateRecursorConfig(t *testing.T) {
	tests := []struct {
		name      string
		config    *Config
		wantErr   bool
		errMsg    string
		checkFunc func(t *testing.T, result string)
	}{
		{
			name:    "nil config",
			config:  nil,
			wantErr: true,
			errMsg:  "config cannot be nil",
		},
		{
			name: "missing API key",
			config: &Config{
				Forwarders: []string{"8.8.8.8"},
			},
			wantErr: true,
			errMsg:  "API key is required",
		},
		{
			name: "missing forwarders",
			config: &Config{
				APIKey:     "test-key",
				Forwarders: []string{},
			},
			wantErr: true,
			errMsg:  "at least one forwarder is required",
		},
		{
			name: "valid config with single forwarder",
			config: &Config{
				APIKey:     "test-api-key-456",
				Forwarders: []string{"8.8.8.8"},
			},
			wantErr: false,
			checkFunc: func(t *testing.T, result string) {
				assert.Contains(t, result, "api-key=test-api-key-456")
				assert.Contains(t, result, "forward-zones-recurse=.=8.8.8.8")
				assert.Contains(t, result, "local-address=0.0.0.0")
				assert.Contains(t, result, "local-port=53")
				assert.Contains(t, result, "webserver=yes")
				assert.Contains(t, result, "webserver-port=8082")
			},
		},
		{
			name: "valid config with multiple forwarders",
			config: &Config{
				APIKey:     "test-key",
				Forwarders: []string{"8.8.8.8", "1.1.1.1", "9.9.9.9"},
			},
			wantErr: false,
			checkFunc: func(t *testing.T, result string) {
				assert.Contains(t, result, "forward-zones-recurse=.=8.8.8.8;1.1.1.1;9.9.9.9")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := GenerateRecursorConfig(tt.config)

			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errMsg)
				assert.Empty(t, result)
			} else {
				require.NoError(t, err)
				assert.NotEmpty(t, result)
				if tt.checkFunc != nil {
					tt.checkFunc(t, result)
				}
			}
		})
	}
}

func TestGenerateAuthConfigFormat(t *testing.T) {
	cfg := &Config{
		APIKey:  "secret-key",
		Backend: "sqlite",
		DataDir: "/var/lib/powerdns",
	}

	result, err := GenerateAuthConfig(cfg)
	require.NoError(t, err)

	// Verify it's valid config format (key=value lines)
	lines := strings.Split(result, "\n")
	configLines := 0
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		// Should be key=value format
		if strings.Contains(line, "=") {
			configLines++
		}
	}

	assert.Greater(t, configLines, 0, "should have at least one config line")
}

func TestGenerateRecursorConfigFormat(t *testing.T) {
	cfg := &Config{
		APIKey:     "secret-key",
		Forwarders: []string{"8.8.8.8", "1.1.1.1"},
	}

	result, err := GenerateRecursorConfig(cfg)
	require.NoError(t, err)

	// Verify it's valid config format
	lines := strings.Split(result, "\n")
	configLines := 0
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if strings.Contains(line, "=") {
			configLines++
		}
	}

	assert.Greater(t, configLines, 0, "should have at least one config line")
}
