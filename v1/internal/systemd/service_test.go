package systemd

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockSSHExecutor implements SSHExecutor for testing
type mockSSHExecutor struct {
	commands []string
	responses map[string]string
	errors map[string]error
	executeFunc func(string) (string, error)
}

func newMockSSHExecutor() *mockSSHExecutor {
	return &mockSSHExecutor{
		commands: make([]string, 0),
		responses: make(map[string]string),
		errors: make(map[string]error),
	}
}

func (m *mockSSHExecutor) Execute(cmd string) (string, error) {
	m.commands = append(m.commands, cmd)

	// Use custom execute function if provided
	if m.executeFunc != nil {
		return m.executeFunc(cmd)
	}

	// Check for prefix match for errors first
	for pattern, err := range m.errors {
		if strings.HasPrefix(cmd, pattern) {
			return "", err
		}
	}

	// Check for exact match
	if resp, ok := m.responses[cmd]; ok {
		return resp, nil
	}

	// Check for prefix match (for commands with variable parts)
	for pattern, resp := range m.responses {
		if strings.HasPrefix(cmd, pattern) {
			return resp, nil
		}
	}

	return "", nil
}

func (m *mockSSHExecutor) setResponse(cmd, response string) {
	m.responses[cmd] = response
}

func (m *mockSSHExecutor) setError(cmd string, err error) {
	m.errors[cmd] = err
}

func (m *mockSSHExecutor) getLastCommand() string {
	if len(m.commands) == 0 {
		return ""
	}
	return m.commands[len(m.commands)-1]
}

func TestCreateService(t *testing.T) {
	tests := []struct {
		name        string
		serviceName string
		unit        *UnitFile
		wantErr     bool
		checkCmd    func(t *testing.T, commands []string)
	}{
		{
			name:        "creates service with basic config",
			serviceName: "test-service",
			unit: &UnitFile{
				Description: "Test Service",
				Type:        "simple",
				ExecStart:   "/usr/bin/test",
				Restart:     "always",
				WantedBy:    []string{"multi-user.target"},
			},
			wantErr: false,
			checkCmd: func(t *testing.T, commands []string) {
				require.Len(t, commands, 2, "should execute 2 commands")
				assert.Contains(t, commands[0], "sudo tee /etc/systemd/system/test-service.service")
				assert.Contains(t, commands[0], "Description=Test Service")
				assert.Contains(t, commands[0], "ExecStart=/usr/bin/test")
				assert.Equal(t, "sudo systemctl daemon-reload", commands[1])
			},
		},
		{
			name:        "adds .service suffix if missing",
			serviceName: "myservice",
			unit:        DefaultUnitFile(),
			wantErr:     false,
			checkCmd: func(t *testing.T, commands []string) {
				assert.Contains(t, commands[0], "/etc/systemd/system/myservice.service")
			},
		},
		{
			name:        "preserves .service suffix if present",
			serviceName: "myservice.service",
			unit:        DefaultUnitFile(),
			wantErr:     false,
			checkCmd: func(t *testing.T, commands []string) {
				assert.Contains(t, commands[0], "/etc/systemd/system/myservice.service")
				assert.NotContains(t, commands[0], ".service.service")
			},
		},
		{
			name:        "handles unit file write error",
			serviceName: "fail-service",
			unit:        DefaultUnitFile(),
			wantErr:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mock := newMockSSHExecutor()

			if tt.wantErr {
				mock.setError("sudo tee", fmt.Errorf("write failed"))
			}

			err := CreateService(mock, tt.serviceName, tt.unit)

			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				if tt.checkCmd != nil {
					tt.checkCmd(t, mock.commands)
				}
			}
		})
	}
}

func TestEnableService(t *testing.T) {
	tests := []struct {
		name        string
		serviceName string
		wantErr     bool
		wantCmd     string
	}{
		{
			name:        "enables service",
			serviceName: "test-service",
			wantErr:     false,
			wantCmd:     "sudo systemctl enable test-service.service",
		},
		{
			name:        "adds .service suffix",
			serviceName: "test",
			wantErr:     false,
			wantCmd:     "sudo systemctl enable test.service",
		},
		{
			name:        "handles enable error",
			serviceName: "fail-service",
			wantErr:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mock := newMockSSHExecutor()

			if tt.wantErr {
				mock.setError("sudo systemctl enable", fmt.Errorf("enable failed"))
			}

			err := EnableService(mock, tt.serviceName)

			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.wantCmd, mock.getLastCommand())
			}
		})
	}
}

func TestStartService(t *testing.T) {
	tests := []struct {
		name        string
		serviceName string
		wantErr     bool
		wantCmd     string
	}{
		{
			name:        "starts service",
			serviceName: "test-service",
			wantErr:     false,
			wantCmd:     "sudo systemctl start test-service.service",
		},
		{
			name:        "handles start error",
			serviceName: "fail-service",
			wantErr:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mock := newMockSSHExecutor()

			if tt.wantErr {
				mock.setError("sudo systemctl start", fmt.Errorf("start failed"))
			}

			err := StartService(mock, tt.serviceName)

			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.wantCmd, mock.getLastCommand())
			}
		})
	}
}

func TestStopService(t *testing.T) {
	tests := []struct {
		name        string
		serviceName string
		wantErr     bool
		wantCmd     string
	}{
		{
			name:        "stops service",
			serviceName: "test-service",
			wantErr:     false,
			wantCmd:     "sudo systemctl stop test-service.service",
		},
		{
			name:        "handles stop error",
			serviceName: "fail-service",
			wantErr:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mock := newMockSSHExecutor()

			if tt.wantErr {
				mock.setError("sudo systemctl stop", fmt.Errorf("stop failed"))
			}

			err := StopService(mock, tt.serviceName)

			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.wantCmd, mock.getLastCommand())
			}
		})
	}
}

func TestRestartService(t *testing.T) {
	tests := []struct {
		name        string
		serviceName string
		wantErr     bool
		wantCmd     string
	}{
		{
			name:        "restarts service",
			serviceName: "test-service",
			wantErr:     false,
			wantCmd:     "sudo systemctl restart test-service.service",
		},
		{
			name:        "handles restart error",
			serviceName: "fail-service",
			wantErr:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mock := newMockSSHExecutor()

			if tt.wantErr {
				mock.setError("sudo systemctl restart", fmt.Errorf("restart failed"))
			}

			err := RestartService(mock, tt.serviceName)

			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.wantCmd, mock.getLastCommand())
			}
		})
	}
}

func TestDisableService(t *testing.T) {
	tests := []struct {
		name        string
		serviceName string
		wantErr     bool
		wantCmd     string
	}{
		{
			name:        "disables service",
			serviceName: "test-service",
			wantErr:     false,
			wantCmd:     "sudo systemctl disable test-service.service",
		},
		{
			name:        "handles disable error",
			serviceName: "fail-service",
			wantErr:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mock := newMockSSHExecutor()

			if tt.wantErr {
				mock.setError("sudo systemctl disable", fmt.Errorf("disable failed"))
			}

			err := DisableService(mock, tt.serviceName)

			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.wantCmd, mock.getLastCommand())
			}
		})
	}
}

func TestGetServiceStatus(t *testing.T) {
	tests := []struct {
		name        string
		serviceName string
		output      string
		want        *ServiceStatus
		wantErr     bool
	}{
		{
			name:        "parses running service status",
			serviceName: "test-service",
			output: `LoadState=loaded
ActiveState=active
SubState=running
UnitFileState=enabled
MainPID=1234
ActiveEnterTimestamp=Mon 2024-01-15 10:30:45 UTC
MemoryCurrent=52428800
TasksCurrent=5`,
			want: &ServiceStatus{
				Name:        "test-service.service",
				Loaded:      true,
				Active:      true,
				Running:     true,
				Enabled:     true,
				MainPID:     1234,
				LoadState:   "loaded",
				ActiveState: "active",
				SubState:    "running",
				Memory:      52428800,
				Tasks:       5,
			},
			wantErr: false,
		},
		{
			name:        "parses stopped service status",
			serviceName: "stopped-service",
			output: `LoadState=loaded
ActiveState=inactive
SubState=dead
UnitFileState=disabled
MainPID=0`,
			want: &ServiceStatus{
				Name:        "stopped-service.service",
				Loaded:      true,
				Active:      false,
				Running:     false,
				Enabled:     false,
				MainPID:     0,
				LoadState:   "loaded",
				ActiveState: "inactive",
				SubState:    "dead",
			},
			wantErr: false,
		},
		{
			name:        "parses failed service status",
			serviceName: "failed-service",
			output: `LoadState=loaded
ActiveState=failed
SubState=failed
UnitFileState=enabled`,
			want: &ServiceStatus{
				Name:        "failed-service.service",
				Loaded:      true,
				Active:      false,
				Running:     false,
				Enabled:     true,
				LoadState:   "loaded",
				ActiveState: "failed",
				SubState:    "failed",
			},
			wantErr: false,
		},
		{
			name:        "handles not-found service",
			serviceName: "notfound-service",
			output: `LoadState=not-found
ActiveState=inactive
SubState=dead`,
			want: &ServiceStatus{
				Name:        "notfound-service.service",
				Loaded:      false,
				Active:      false,
				Running:     false,
				Enabled:     false,
				LoadState:   "not-found",
				ActiveState: "inactive",
				SubState:    "dead",
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mock := newMockSSHExecutor()
			mock.setResponse("systemctl show", tt.output)

			got, err := GetServiceStatus(mock, tt.serviceName)

			if tt.wantErr {
				assert.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.want.Name, got.Name)
				assert.Equal(t, tt.want.Loaded, got.Loaded)
				assert.Equal(t, tt.want.Active, got.Active)
				assert.Equal(t, tt.want.Running, got.Running)
				assert.Equal(t, tt.want.Enabled, got.Enabled)
				assert.Equal(t, tt.want.MainPID, got.MainPID)
				assert.Equal(t, tt.want.LoadState, got.LoadState)
				assert.Equal(t, tt.want.ActiveState, got.ActiveState)
				assert.Equal(t, tt.want.SubState, got.SubState)
				assert.Equal(t, tt.want.Memory, got.Memory)
				assert.Equal(t, tt.want.Tasks, got.Tasks)
			}
		})
	}
}

func TestIsServiceRunning(t *testing.T) {
	tests := []struct {
		name        string
		serviceName string
		output      string
		want        bool
		wantErr     bool
	}{
		{
			name:        "returns true for running service",
			serviceName: "running-service",
			output:      "SubState=running\nActiveState=active",
			want:        true,
			wantErr:     false,
		},
		{
			name:        "returns false for stopped service",
			serviceName: "stopped-service",
			output:      "SubState=dead\nActiveState=inactive",
			want:        false,
			wantErr:     false,
		},
		{
			name:        "returns false for exited service",
			serviceName: "exited-service",
			output:      "SubState=exited\nActiveState=inactive",
			want:        false,
			wantErr:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mock := newMockSSHExecutor()
			mock.setResponse("systemctl show", tt.output)

			got, err := IsServiceRunning(mock, tt.serviceName)

			if tt.wantErr {
				assert.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.want, got)
			}
		})
	}
}

func TestIsServiceEnabled(t *testing.T) {
	tests := []struct {
		name        string
		serviceName string
		output      string
		want        bool
		wantErr     bool
	}{
		{
			name:        "returns true for enabled service",
			serviceName: "enabled-service",
			output:      "UnitFileState=enabled",
			want:        true,
			wantErr:     false,
		},
		{
			name:        "returns false for disabled service",
			serviceName: "disabled-service",
			output:      "UnitFileState=disabled",
			want:        false,
			wantErr:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mock := newMockSSHExecutor()
			mock.setResponse("systemctl show", tt.output)

			got, err := IsServiceEnabled(mock, tt.serviceName)

			if tt.wantErr {
				assert.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.want, got)
			}
		})
	}
}

func TestWaitForService(t *testing.T) {
	t.Run("waits for service to be running", func(t *testing.T) {
		mock := newMockSSHExecutor()

		callCount := 0
		mock.executeFunc = func(cmd string) (string, error) {
			if strings.HasPrefix(cmd, "systemctl show") {
				callCount++
				if callCount == 1 {
					return "SubState=activating\nActiveState=activating", nil
				}
				return "SubState=running\nActiveState=active", nil
			}
			return "", nil
		}

		err := WaitForService(mock, "test-service", "running", 5*time.Second)
		assert.NoError(t, err)
		assert.GreaterOrEqual(t, callCount, 2, "should poll multiple times")
	})

	t.Run("times out if service never reaches target state", func(t *testing.T) {
		mock := newMockSSHExecutor()
		mock.setResponse("systemctl show", "SubState=activating\nActiveState=activating")

		err := WaitForService(mock, "slow-service", "running", 2*time.Second)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "timeout")
	})
}

func TestRenderUnitFile(t *testing.T) {
	tests := []struct {
		name    string
		unit    *UnitFile
		want    []string // strings that should be in the output
		wantErr bool
	}{
		{
			name: "renders basic unit file",
			unit: &UnitFile{
				Description: "Test Service",
				Type:        "simple",
				ExecStart:   "/usr/bin/test",
				Restart:     "always",
				RestartSec:  10,
				WantedBy:    []string{"multi-user.target"},
			},
			want: []string{
				"[Unit]",
				"Description=Test Service",
				"[Service]",
				"Type=simple",
				"ExecStart=/usr/bin/test",
				"Restart=always",
				"RestartSec=10",
				"[Install]",
				"WantedBy=multi-user.target",
			},
			wantErr: false,
		},
		{
			name: "renders unit file with dependencies",
			unit: &UnitFile{
				Description: "Test Service",
				After:       []string{"network.target", "docker.service"},
				Requires:    []string{"docker.service"},
				Wants:       []string{"network-online.target"},
				Type:        "simple",
				ExecStart:   "/usr/bin/test",
				WantedBy:    []string{"multi-user.target"},
			},
			want: []string{
				"After=network.target docker.service",
				"Requires=docker.service",
				"Wants=network-online.target",
			},
			wantErr: false,
		},
		{
			name: "renders unit file with environment",
			unit: &UnitFile{
				Description: "Test Service",
				Type:        "simple",
				ExecStart:   "/usr/bin/test",
				Environment: map[string]string{
					"FOO": "bar",
					"BAZ": "qux",
				},
				WantedBy: []string{"multi-user.target"},
			},
			want: []string{
				"Environment=",
			},
			wantErr: false,
		},
		{
			name: "renders unit file with user and group",
			unit: &UnitFile{
				Description: "Test Service",
				Type:        "simple",
				ExecStart:   "/usr/bin/test",
				User:        "testuser",
				Group:       "testgroup",
				WantedBy:    []string{"multi-user.target"},
			},
			want: []string{
				"User=testuser",
				"Group=testgroup",
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := renderUnitFile(tt.unit)

			if tt.wantErr {
				assert.Error(t, err)
			} else {
				require.NoError(t, err)
				for _, want := range tt.want {
					assert.Contains(t, got, want)
				}
			}
		})
	}
}

func TestDefaultUnitFile(t *testing.T) {
	unit := DefaultUnitFile()

	assert.Equal(t, "simple", unit.Type)
	assert.Equal(t, "on-failure", unit.Restart)
	assert.Equal(t, 5, unit.RestartSec)
	assert.Equal(t, []string{"multi-user.target"}, unit.WantedBy)
}

func TestContainerUnitFile(t *testing.T) {
	unit := ContainerUnitFile("test-container", "Test Container Service", "/usr/bin/docker run test")

	assert.Equal(t, "Test Container Service", unit.Description)
	assert.Equal(t, "/usr/bin/docker run test", unit.ExecStart)
	assert.Equal(t, "simple", unit.Type)
	assert.Equal(t, "always", unit.Restart)
	assert.Equal(t, 10, unit.RestartSec)
	assert.Equal(t, []string{"network-online.target"}, unit.After)
	assert.Equal(t, []string{"network-online.target"}, unit.Wants)
	assert.Equal(t, []string{"multi-user.target"}, unit.WantedBy)
}

func TestParseSystemdTimestamp(t *testing.T) {
	tests := []struct {
		name    string
		ts      string
		wantErr bool
	}{
		{
			name:    "parses valid timestamp",
			ts:      "Mon 2024-01-15 10:30:45 UTC",
			wantErr: false,
		},
		{
			name:    "parses timestamp with different day",
			ts:      "Fri 2023-12-25 14:22:33 UTC",
			wantErr: false,
		},
		{
			name:    "handles empty timestamp",
			ts:      "",
			wantErr: true,
		},
		{
			name:    "handles n/a timestamp",
			ts:      "n/a",
			wantErr: true,
		},
		{
			name:    "handles invalid format",
			ts:      "invalid",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseSystemdTimestamp(tt.ts)

			if tt.wantErr {
				assert.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.False(t, got.IsZero())
			}
		})
	}
}
