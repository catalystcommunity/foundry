package openbao

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/catalystcommunity/foundry/v1/internal/component"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockSSHExecutor implements ssh.Executor for testing
type mockSSHExecutor struct {
	commands []string
	responses map[string]string
	errors    map[string]error
}

func newMockSSHExecutor() *mockSSHExecutor {
	return &mockSSHExecutor{
		commands:  make([]string, 0),
		responses: make(map[string]string),
		errors:    make(map[string]error),
	}
}

func (m *mockSSHExecutor) Execute(cmd string) (string, error) {
	m.commands = append(m.commands, cmd)

	// Check for exact match first
	if resp, ok := m.responses[cmd]; ok {
		if err, hasErr := m.errors[cmd]; hasErr {
			return resp, err
		}
		return resp, nil
	}

	// Check for partial matches (for commands with dynamic parts)
	// Longer patterns should be checked first for specificity
	for pattern, resp := range m.responses {
		if strings.Contains(cmd, pattern) {
			if err, hasErr := m.errors[pattern]; hasErr {
				return resp, err
			}
			return resp, nil
		}
	}

	// Check errors for partial matches
	for pattern, err := range m.errors {
		if strings.Contains(cmd, pattern) {
			return "", err
		}
	}

	// Default response
	return "", nil
}

func (m *mockSSHExecutor) setResponse(cmd, response string) {
	m.responses[cmd] = response
}

func (m *mockSSHExecutor) setError(cmd string, err error) {
	m.errors[cmd] = err
}

func (m *mockSSHExecutor) hasCommand(pattern string) bool {
	for _, cmd := range m.commands {
		if strings.Contains(cmd, pattern) {
			return true
		}
	}
	return false
}

func TestParseConfig(t *testing.T) {
	tests := []struct {
		name     string
		input    component.ComponentConfig
		expected *Config
	}{
		{
			name:     "empty config uses defaults",
			input:    component.ComponentConfig{},
			expected: DefaultConfig(),
		},
		{
			name: "override version",
			input: component.ComponentConfig{
				"version": "2.1.0",
			},
			expected: &Config{
				Version:          "2.1.0",
				DataPath:         "/var/lib/openbao",
				ConfigPath:       "/etc/openbao",
				Address:          "0.0.0.0:8200",
				ContainerRuntime: "docker",
			},
		},
		{
			name: "override all fields",
			input: component.ComponentConfig{
				"version":           "2.1.0",
				"data_path":         "/custom/data",
				"config_path":       "/custom/config",
				"address":           "127.0.0.1:9200",
				"container_runtime": "podman",
			},
			expected: &Config{
				Version:          "2.1.0",
				DataPath:         "/custom/data",
				ConfigPath:       "/custom/config",
				Address:          "127.0.0.1:9200",
				ContainerRuntime: "podman",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := parseConfig(tt.input)
			require.NoError(t, err)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestComponent_Install(t *testing.T) {
	tests := []struct {
		name        string
		cfg         component.ComponentConfig
		setupMock   func(*mockSSHExecutor)
		wantErr     bool
		errContains string
		checkCmds   []string
	}{
		{
			name: "successful installation with docker",
			cfg: component.ComponentConfig{
				"version":           "2.0.0",
				"container_runtime": "docker",
			},
			setupMock: func(m *mockSSHExecutor) {
				// Docker detection
				m.setResponse("command -v docker", "/usr/bin/docker")
				m.setResponse("docker --version", "Docker version 24.0.0")

				// Directory creation
				m.setResponse("mkdir", "")

				// Config file write
				m.setResponse("cat >", "")

				// Docker pull
				m.setResponse("docker pull", "")

				// Systemd commands - specific patterns first
				m.setResponse("systemctl show openbao.service", `LoadState=loaded
ActiveState=active
SubState=running`)
				m.setResponse("systemctl enable", "")
				m.setResponse("systemctl start", "")
			},
			wantErr: false,
			checkCmds: []string{
				"mkdir -p /var/lib/openbao",
				"mkdir -p /etc/openbao",
				"docker pull quay.io/openbao/openbao:2.0.0",
				"systemctl enable openbao",
				"systemctl start openbao",
			},
		},
		{
			name: "successful installation with podman",
			cfg: component.ComponentConfig{
				"version":           "2.0.0",
				"container_runtime": "podman",
			},
			setupMock: func(m *mockSSHExecutor) {
				// Podman detection
				m.setResponse("command -v docker", "")
				m.setError("command -v docker", fmt.Errorf("not found"))
				m.setResponse("command -v podman", "/usr/bin/podman")
				m.setResponse("podman --version", "podman version 4.0.0")

				// Directory creation
				m.setResponse("mkdir", "")

				// Config file write
				m.setResponse("cat >", "")

				// Podman pull
				m.setResponse("podman pull", "")

				// Systemd commands - specific patterns first
				m.setResponse("systemctl show openbao.service", `LoadState=loaded
ActiveState=active
SubState=running`)
				m.setResponse("systemctl enable", "")
				m.setResponse("systemctl start", "")
			},
			wantErr: false,
			checkCmds: []string{
				"podman pull quay.io/openbao/openbao:2.0.0",
			},
		},
		{
			name: "invalid config",
			cfg: component.ComponentConfig{
				"version": "", // Invalid: empty version
			},
			setupMock:   func(m *mockSSHExecutor) {},
			wantErr:     true,
			errContains: "version is required",
		},
		{
			name: "directory creation fails",
			cfg: component.ComponentConfig{
				"version":           "2.0.0",
				"container_runtime": "docker",
			},
			setupMock: func(m *mockSSHExecutor) {
				m.setResponse("command -v docker", "/usr/bin/docker")
				m.setResponse("docker --version", "Docker version 24.0.0")
				m.setError("mkdir", fmt.Errorf("permission denied"))
			},
			wantErr:     true,
			errContains: "failed to create directory",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mock := newMockSSHExecutor()
			tt.setupMock(mock)

			comp := NewComponent(mock)
			err := comp.Install(context.Background(), tt.cfg)

			if tt.wantErr {
				require.Error(t, err)
				if tt.errContains != "" {
					assert.Contains(t, err.Error(), tt.errContains)
				}
			} else {
				require.NoError(t, err)

				// Verify expected commands were executed
				for _, expected := range tt.checkCmds {
					assert.True(t, mock.hasCommand(expected),
						"expected command containing %q to be executed", expected)
				}
			}
		})
	}
}

func TestComponent_createDirectories(t *testing.T) {
	mock := newMockSSHExecutor()
	comp := NewComponent(mock)
	cfg := DefaultConfig()

	err := comp.createDirectories(cfg)
	require.NoError(t, err)

	assert.True(t, mock.hasCommand("/var/lib/openbao"))
	assert.True(t, mock.hasCommand("/etc/openbao"))
	assert.True(t, mock.hasCommand("mkdir -p"))
	assert.True(t, mock.hasCommand("chmod 755"))
}

func TestComponent_writeConfigFile(t *testing.T) {
	mock := newMockSSHExecutor()
	comp := NewComponent(mock)
	cfg := DefaultConfig()

	err := comp.writeConfigFile(cfg)
	require.NoError(t, err)

	assert.True(t, mock.hasCommand("/etc/openbao/config.hcl"))
	assert.True(t, mock.hasCommand("cat >"))
}

func TestComponent_buildExecStart(t *testing.T) {
	tests := []struct {
		name         string
		cfg          *Config
		runtime      string
		wantContains []string
	}{
		{
			name:    "docker runtime",
			cfg:     DefaultConfig(),
			runtime: "docker",
			wantContains: []string{
				"/usr/bin/docker run",
				"--rm",
				"--name openbao",
				"-p 8200:8200",
				"-v /var/lib/openbao:/vault/data",
				"-v /etc/openbao:/vault/config",
				"--cap-add=IPC_LOCK",
				"quay.io/openbao/openbao:2.0.0",
				"server",
				"-config=/vault/config/config.hcl",
			},
		},
		{
			name:    "podman runtime",
			cfg:     DefaultConfig(),
			runtime: "podman",
			wantContains: []string{
				"/usr/bin/podman run",
				"quay.io/openbao/openbao:2.0.0",
			},
		},
		{
			name: "custom port",
			cfg: &Config{
				Version:          "2.0.0",
				DataPath:         "/var/lib/openbao",
				ConfigPath:       "/etc/openbao",
				Address:          "0.0.0.0:9200",
				ContainerRuntime: "docker",
			},
			runtime: "docker",
			wantContains: []string{
				"-p 9200:8200",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			comp := NewComponent(nil)
			result := comp.buildExecStart(tt.cfg, tt.runtime)

			for _, want := range tt.wantContains {
				assert.Contains(t, result, want)
			}
		})
	}
}
