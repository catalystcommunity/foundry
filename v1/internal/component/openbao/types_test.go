package openbao

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()

	assert.Equal(t, "2.0.0", cfg.Version)
	assert.Equal(t, "/var/lib/openbao", cfg.DataPath)
	assert.Equal(t, "/etc/openbao", cfg.ConfigPath)
	assert.Equal(t, "0.0.0.0:8200", cfg.Address)
	assert.Equal(t, "docker", cfg.ContainerRuntime)
}

func TestConfig_Validate(t *testing.T) {
	tests := []struct {
		name    string
		cfg     *Config
		wantErr bool
		errMsg  string
	}{
		{
			name:    "valid config",
			cfg:     DefaultConfig(),
			wantErr: false,
		},
		{
			name: "missing version",
			cfg: &Config{
				Version:          "",
				DataPath:         "/var/lib/openbao",
				ConfigPath:       "/etc/openbao",
				Address:          "0.0.0.0:8200",
				ContainerRuntime: "docker",
			},
			wantErr: true,
			errMsg:  "version is required",
		},
		{
			name: "missing data_path",
			cfg: &Config{
				Version:          "2.0.0",
				DataPath:         "",
				ConfigPath:       "/etc/openbao",
				Address:          "0.0.0.0:8200",
				ContainerRuntime: "docker",
			},
			wantErr: true,
			errMsg:  "data_path is required",
		},
		{
			name: "missing config_path",
			cfg: &Config{
				Version:          "2.0.0",
				DataPath:         "/var/lib/openbao",
				ConfigPath:       "",
				Address:          "0.0.0.0:8200",
				ContainerRuntime: "docker",
			},
			wantErr: true,
			errMsg:  "config_path is required",
		},
		{
			name: "missing address",
			cfg: &Config{
				Version:          "2.0.0",
				DataPath:         "/var/lib/openbao",
				ConfigPath:       "/etc/openbao",
				Address:          "",
				ContainerRuntime: "docker",
			},
			wantErr: true,
			errMsg:  "address is required",
		},
		{
			name: "invalid container runtime",
			cfg: &Config{
				Version:          "2.0.0",
				DataPath:         "/var/lib/openbao",
				ConfigPath:       "/etc/openbao",
				Address:          "0.0.0.0:8200",
				ContainerRuntime: "containerd",
			},
			wantErr: true,
			errMsg:  "container_runtime must be 'docker' or 'podman'",
		},
		{
			name: "valid podman runtime",
			cfg: &Config{
				Version:          "2.0.0",
				DataPath:         "/var/lib/openbao",
				ConfigPath:       "/etc/openbao",
				Address:          "0.0.0.0:8200",
				ContainerRuntime: "podman",
			},
			wantErr: false,
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

func TestComponent_Name(t *testing.T) {
	comp := NewComponent(nil)
	assert.Equal(t, "openbao", comp.Name())
}
