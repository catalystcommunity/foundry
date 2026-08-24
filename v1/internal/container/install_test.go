package container

import (
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockCommandExecutor is a mock for CommandExecutor interface
type mockCommandExecutor struct {
	commands    map[string]*ExecResult // command pattern -> result
	execHistory []string               // commands that were executed
}

func newMockCommandExecutor() *mockCommandExecutor {
	return &mockCommandExecutor{
		commands:    make(map[string]*ExecResult),
		execHistory: []string{},
	}
}

func (m *mockCommandExecutor) Exec(cmd string) (*ExecResult, error) {
	m.execHistory = append(m.execHistory, cmd)

	// Check for exact match first
	if result, ok := m.commands[cmd]; ok {
		return result, nil
	}

	// Check for prefix match
	for pattern, result := range m.commands {
		if strings.Contains(cmd, pattern) {
			return result, nil
		}
	}

	// Default: return success with empty output
	return &ExecResult{Stdout: "", Stderr: "", ExitCode: 0}, nil
}

func (m *mockCommandExecutor) setResult(cmdPattern string, stdout string, exitCode int) {
	m.commands[cmdPattern] = &ExecResult{
		Stdout:   stdout,
		Stderr:   "",
		ExitCode: exitCode,
	}
}

func (m *mockCommandExecutor) setError(cmdPattern string, stderr string, exitCode int) {
	m.commands[cmdPattern] = &ExecResult{
		Stdout:   "",
		Stderr:   stderr,
		ExitCode: exitCode,
	}
}

// Test constants are correctly defined
func TestCNIConstants(t *testing.T) {
	assert.Equal(t, "/etc/cni/net.d/10-containerd-net.conflist", CNIConfigPath)
	assert.Equal(t, "/etc/cni/net.d", CNIConfigDir)
	assert.Contains(t, BridgeNetworkingServices, "openbao")
	assert.Contains(t, BridgeNetworkingServices, "foundry-zot")
}

func TestCNIConfigContent(t *testing.T) {
	// Verify CNI config contains expected fields
	assert.Contains(t, CNIConfigContent, "containerd-net")
	assert.Contains(t, CNIConfigContent, "bridge")
	assert.Contains(t, CNIConfigContent, "portmap")
	assert.Contains(t, CNIConfigContent, "firewall")
	assert.Contains(t, CNIConfigContent, "tuning")
	assert.Contains(t, CNIConfigContent, "10.88.0.0/16")
}

func TestIsCNIConfigValid_Valid(t *testing.T) {
	mock := newMockCommandExecutor()

	// Config file exists
	mock.setResult(fmt.Sprintf("test -f %s && echo 'ok'", CNIConfigPath), "ok", 0)
	// Config contains expected content
	mock.setResult("grep -q 'containerd-net'", "ok", 0)

	valid := isCNIConfigValid(mock)
	assert.True(t, valid)
}

func TestIsCNIConfigValid_Missing(t *testing.T) {
	mock := newMockCommandExecutor()

	// Config file doesn't exist
	mock.setResult(fmt.Sprintf("test -f %s && echo 'ok'", CNIConfigPath), "", 1)

	valid := isCNIConfigValid(mock)
	assert.False(t, valid)
}

func TestIsCNIConfigValid_InvalidContent(t *testing.T) {
	mock := newMockCommandExecutor()

	// Config file exists
	mock.setResult(fmt.Sprintf("test -f %s && echo 'ok'", CNIConfigPath), "ok", 0)
	// But doesn't contain expected content
	mock.setError("grep -q 'containerd-net'", "", 1)

	valid := isCNIConfigValid(mock)
	assert.False(t, valid)
}

func TestInstallCNIConfig_Success(t *testing.T) {
	mock := newMockCommandExecutor()

	// All commands succeed
	mock.setResult(fmt.Sprintf("sudo mkdir -p %s", CNIConfigDir), "", 0)
	mock.setResult("sudo tee", "", 0)

	err := installCNIConfig(mock)
	assert.NoError(t, err)

	// Verify commands were called
	assert.GreaterOrEqual(t, len(mock.execHistory), 2)
}

func TestInstallCNIConfig_MkdirFails(t *testing.T) {
	mock := newMockCommandExecutor()

	// mkdir fails
	mock.setError(fmt.Sprintf("sudo mkdir -p %s", CNIConfigDir), "permission denied", 1)

	err := installCNIConfig(mock)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to create CNI config directory")
}

func TestEnsureCNIConfig_AlreadyExists(t *testing.T) {
	mock := newMockCommandExecutor()

	// Config exists and is valid
	mock.setResult(fmt.Sprintf("test -f %s && echo 'ok'", CNIConfigPath), "ok", 0)
	mock.setResult("grep -q 'containerd-net'", "ok", 0)

	created, err := EnsureCNIConfig(mock)
	require.NoError(t, err)
	assert.False(t, created, "should not create config when it already exists")
}

func TestEnsureCNIConfig_CreatesNew(t *testing.T) {
	mock := newMockCommandExecutor()

	// Config doesn't exist
	mock.setResult(fmt.Sprintf("test -f %s && echo 'ok'", CNIConfigPath), "", 1)
	// mkdir and tee succeed
	mock.setResult(fmt.Sprintf("sudo mkdir -p %s", CNIConfigDir), "", 0)
	mock.setResult("sudo tee", "", 0)

	created, err := EnsureCNIConfig(mock)
	require.NoError(t, err)
	assert.True(t, created, "should create config when it doesn't exist")
}

func TestDetectRuntimeInstallation_NerdctlComplete(t *testing.T) {
	mock := newMockCommandExecutor()

	// docker command exists
	mock.setResult("which docker", "/usr/local/bin/docker", 0)
	// docker version shows nerdctl
	mock.setResult("docker version 2>&1", "nerdctl version 1.7.2", 0)
	// CNI plugins installed
	mock.setResult("test -f /opt/cni/bin/bridge && echo 'ok'", "ok", 0)
	// CNI config valid
	mock.setResult(fmt.Sprintf("test -f %s && echo 'ok'", CNIConfigPath), "ok", 0)
	mock.setResult("grep -q 'containerd-net'", "ok", 0)

	runtimeType := DetectRuntimeInstallation(mock)
	assert.Equal(t, RuntimeNerdctl, runtimeType)
}

func TestDetectRuntimeInstallation_NerdctlIncompleteMissingConfig(t *testing.T) {
	mock := newMockCommandExecutor()

	// docker command exists
	mock.setResult("which docker", "/usr/local/bin/docker", 0)
	// docker version shows nerdctl
	mock.setResult("docker version 2>&1", "nerdctl version 1.7.2", 0)
	// CNI plugins installed
	mock.setResult("test -f /opt/cni/bin/bridge && echo 'ok'", "ok", 0)
	// CNI config missing
	mock.setResult(fmt.Sprintf("test -f %s && echo 'ok'", CNIConfigPath), "", 1)

	runtimeType := DetectRuntimeInstallation(mock)
	assert.Equal(t, RuntimeNerdctlIncomplete, runtimeType)
}

func TestDetectRuntimeInstallation_NerdctlIncompleteMissingPlugins(t *testing.T) {
	mock := newMockCommandExecutor()

	// docker command exists
	mock.setResult("which docker", "/usr/local/bin/docker", 0)
	// docker version shows nerdctl
	mock.setResult("docker version 2>&1", "nerdctl version 1.7.2", 0)
	// CNI plugins missing
	mock.setResult("test -f /opt/cni/bin/bridge && echo 'ok'", "", 1)

	runtimeType := DetectRuntimeInstallation(mock)
	assert.Equal(t, RuntimeNerdctlIncomplete, runtimeType)
}

func TestDetectRuntimeInstallation_Docker(t *testing.T) {
	mock := newMockCommandExecutor()

	// docker command exists
	mock.setResult("which docker", "/usr/bin/docker", 0)
	// docker version shows Docker Engine
	mock.setResult("docker version 2>&1", "Docker Engine - Community\nVersion: 24.0.0\ndocker.com", 0)

	runtimeType := DetectRuntimeInstallation(mock)
	assert.Equal(t, RuntimeDocker, runtimeType)
}

func TestDetectRuntimeInstallation_None(t *testing.T) {
	mock := newMockCommandExecutor()

	// docker command doesn't exist
	mock.setResult("which docker", "", 1)

	runtimeType := DetectRuntimeInstallation(mock)
	assert.Equal(t, RuntimeNone, runtimeType)
}

func TestRemoteArchitecture(t *testing.T) {
	cases := []struct {
		machine  string
		expected string
	}{
		{"x86_64", "amd64"},
		{"amd64", "amd64"},
		{"aarch64", "arm64"},
		{"arm64", "arm64"},
	}
	for _, tc := range cases {
		t.Run(tc.machine, func(t *testing.T) {
			mock := newMockCommandExecutor()
			mock.setResult("uname -m", tc.machine+"\n", 0)
			arch, err := remoteArchitecture(mock)
			require.NoError(t, err)
			assert.Equal(t, tc.expected, arch)
		})
	}
}

func TestRemoteArchitecture_Unsupported(t *testing.T) {
	mock := newMockCommandExecutor()
	mock.setResult("uname -m", "armv7l\n", 0)
	_, err := remoteArchitecture(mock)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unsupported host architecture")
}

func TestRemoteArchitecture_CommandFails(t *testing.T) {
	mock := newMockCommandExecutor()
	mock.setError("uname -m", "uname: not found", 127)
	_, err := remoteArchitecture(mock)
	require.Error(t, err)
}

func TestInstallRuntime_UsesHostArchitecture(t *testing.T) {
	mock := newMockCommandExecutor()
	// Nothing installed: detection falls through to RuntimeNone
	mock.setResult("which docker", "", 1)
	mock.setResult("uname -m", "aarch64\n", 0)

	require.NoError(t, InstallRuntime(mock, "testuser"))

	history := strings.Join(mock.execHistory, "\n")
	assert.Contains(t, history, "cni-plugins-linux-arm64-v1.4.0.tgz")
	assert.Contains(t, history, "nerdctl-1.7.2-linux-arm64.tar.gz")
	assert.NotContains(t, history, "amd64")
}
