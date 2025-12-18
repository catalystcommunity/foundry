package openbao

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGenerateConfig(t *testing.T) {
	tests := []struct {
		name           string
		cfg            *Config
		wantErr        bool
		wantContains   []string
		wantNotContain []string
	}{
		{
			name:    "default config",
			cfg:     DefaultConfig(),
			wantErr: false,
			wantContains: []string{
				"ui = true",
				`storage "file"`,
				`path = "/vault/data"`,
				`listener "tcp"`,
				`address     = "0.0.0.0:8200"`,
				`tls_disable = 1`,
				`api_addr = "http://0.0.0.0:8200"`,
				// Telemetry for Prometheus metrics
				"telemetry {",
				"disable_hostname = true",
				`prometheus_retention_time = "60s"`,
			},
		},
		{
			name: "custom address",
			cfg: &Config{
				Version:          "2.0.0",
				DataPath:         "/var/lib/openbao",
				ConfigPath:       "/etc/openbao",
				Address:          "127.0.0.1:8300",
				ContainerRuntime: "docker",
			},
			wantErr: false,
			wantContains: []string{
				`address     = "127.0.0.1:8300"`,
				`api_addr = "http://127.0.0.1:8300"`,
			},
			wantNotContain: []string{
				"0.0.0.0:8200",
			},
		},
		{
			name: "invalid config",
			cfg: &Config{
				Version:          "",
				DataPath:         "/var/lib/openbao",
				ConfigPath:       "/etc/openbao",
				Address:          "0.0.0.0:8200",
				ContainerRuntime: "docker",
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := GenerateConfig(tt.cfg)

			if tt.wantErr {
				require.Error(t, err)
				return
			}

			require.NoError(t, err)
			assert.NotEmpty(t, result)

			for _, want := range tt.wantContains {
				assert.Contains(t, result, want, "config should contain %q", want)
			}

			for _, notWant := range tt.wantNotContain {
				assert.NotContains(t, result, notWant, "config should not contain %q", notWant)
			}
		})
	}
}

func TestGenerateConfig_ValidHCL(t *testing.T) {
	cfg := DefaultConfig()
	result, err := GenerateConfig(cfg)
	require.NoError(t, err)

	// Basic syntax checks
	assert.True(t, strings.Contains(result, "{") && strings.Contains(result, "}"), "should have braces")
	assert.True(t, strings.Contains(result, "storage"), "should have storage block")
	assert.True(t, strings.Contains(result, "listener"), "should have listener block")
	assert.True(t, strings.Contains(result, "telemetry"), "should have telemetry block for Prometheus metrics")
}
