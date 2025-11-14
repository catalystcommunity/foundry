package openbao

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
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
				"host":              nil, // Will be set in setup
				"cluster_name":      "test-cluster",
				// keys_dir and api_url will be set in test loop
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
				"host":              nil, // Will be set in setup
				"cluster_name":      "test-cluster",
				// keys_dir and api_url will be set in test loop
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
				"host":    nil, // Will be set in setup
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
				"host":              nil, // Will be set in setup
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

			// Set up keys_dir for tests that need initialization (has cluster_name)
			if clusterName, ok := tt.cfg["cluster_name"].(string); ok && clusterName != "" {
				tt.cfg["keys_dir"] = t.TempDir()
			}

			// Set up mock OpenBAO API server if this test needs initialization
			if tt.cfg["keys_dir"] != nil {
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					switch r.URL.Path {
					case "/v1/sys/health":
						// Return uninitialized, sealed status
						w.WriteHeader(http.StatusOK)
						json.NewEncoder(w).Encode(map[string]interface{}{
							"initialized": false,
							"sealed":      true,
						})
					case "/v1/sys/init":
						// Return successful initialization
						w.WriteHeader(http.StatusOK)
						json.NewEncoder(w).Encode(InitResponse{
							Keys:       []string{"key1", "key2", "key3", "key4", "key5"},
							KeysBase64: []string{"a2V5MQ==", "a2V5Mg==", "a2V5Mw==", "a2V5NA==", "a2V5NQ=="},
							RootToken:  "test-root-token",
						})
					case "/v1/sys/unseal":
						// Return unsealed status after enough keys
						w.WriteHeader(http.StatusOK)
						json.NewEncoder(w).Encode(UnsealResponse{
							Sealed: false,
							T:      3,
							N:      5,
						})
					}
				}))
				defer server.Close()
				tt.cfg["api_url"] = server.URL
			}

			comp := NewComponent(mock) // Deprecated: component doesn't need conn anymore

			// Add mock connection to config (this is how Install() receives it now)
			tt.cfg["host"] = mock

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

				// Verify key material was saved if initialization was expected
				if tt.cfg["keys_dir"] != nil {
					keysDir := tt.cfg["keys_dir"].(string)
					clusterName := tt.cfg["cluster_name"].(string)
					assert.True(t, KeyMaterialExists(keysDir, clusterName),
						"expected key material to be saved")
				}
			}
		})
	}
}

func TestComponent_createDirectories(t *testing.T) {
	mock := newMockSSHExecutor()
	cfg := DefaultConfig()

	err := createDirectories(mock, cfg)
	require.NoError(t, err)

	assert.True(t, mock.hasCommand("/var/lib/openbao"))
	assert.True(t, mock.hasCommand("/etc/openbao"))
	assert.True(t, mock.hasCommand("sudo mkdir -p"))
	assert.True(t, mock.hasCommand("sudo chown 374:374"))
	assert.True(t, mock.hasCommand("sudo chmod 755"))
}

func TestComponent_writeConfigFile(t *testing.T) {
	mock := newMockSSHExecutor()
	cfg := DefaultConfig()

	err := writeConfigFile(mock, cfg)
	require.NoError(t, err)

	assert.True(t, mock.hasCommand("/etc/openbao/config.hcl"))
	assert.True(t, mock.hasCommand("sudo tee"))
}

func TestComponent_buildExecStart(t *testing.T) {
	tests := []struct {
		name         string
		cfg          *Config
		runtimePath  string
		wantContains []string
	}{
		{
			name:        "docker runtime",
			cfg:         DefaultConfig(),
			runtimePath: "/usr/local/bin/docker",
			wantContains: []string{
				"/usr/local/bin/docker run",
				"--name openbao",
				"--user 374:374",
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
			name:        "podman runtime",
			cfg:         DefaultConfig(),
			runtimePath: "/usr/bin/podman",
			wantContains: []string{
				"/usr/bin/podman run",
				"--user 374:374",
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
			runtimePath: "/usr/local/bin/docker",
			wantContains: []string{
				"-p 9200:8200",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := buildExecStart(tt.cfg, tt.runtimePath)

			for _, want := range tt.wantContains {
				assert.Contains(t, result, want)
			}
		})
	}
}
