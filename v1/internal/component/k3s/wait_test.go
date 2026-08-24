package k3s

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/catalystcommunity/foundry/v1/internal/ssh"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockReconnectExecutor simulates a host that drops its connection (a reboot)
// and can be reconnected.
type mockReconnectExecutor struct {
	execFunc       func(command string) (*ssh.ExecResult, error)
	reconnectCalls int
	reconnectErr   error
}

func (m *mockReconnectExecutor) Exec(command string) (*ssh.ExecResult, error) {
	return m.execFunc(command)
}

func (m *mockReconnectExecutor) Reconnect() error {
	m.reconnectCalls++
	return m.reconnectErr
}

func TestWaitForServiceActive_ReconnectsAfterReboot(t *testing.T) {
	attempts := 0
	executor := &mockReconnectExecutor{}
	executor.execFunc = func(command string) (*ssh.ExecResult, error) {
		if !strings.Contains(command, "is-active") {
			return &ssh.ExecResult{ExitCode: 0}, nil
		}
		attempts++
		switch {
		case attempts <= 2:
			// Host is rebooting: the connection is gone
			return nil, fmt.Errorf("failed to create session: connection lost")
		default:
			return &ssh.ExecResult{Stdout: "active", ExitCode: 0}, nil
		}
	}

	err := waitForServiceActive(executor, "k3s-agent", RetryConfig{MaxRetries: 10, RetryDelay: time.Millisecond})
	require.NoError(t, err)
	assert.Equal(t, 2, executor.reconnectCalls, "should reconnect once per lost connection")
}

func TestWaitForServiceActive_ReconnectFailureStillTimesOut(t *testing.T) {
	executor := &mockReconnectExecutor{reconnectErr: fmt.Errorf("host still down")}
	executor.execFunc = func(command string) (*ssh.ExecResult, error) {
		if strings.Contains(command, "is-active") {
			return nil, fmt.Errorf("failed to create session: connection lost")
		}
		return &ssh.ExecResult{ExitCode: 0}, nil
	}

	err := waitForServiceActive(executor, "k3s-agent", RetryConfig{MaxRetries: 3, RetryDelay: time.Millisecond})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "stayed unreachable")
	assert.Equal(t, 3, executor.reconnectCalls)
}

func TestDiagnoseServiceFailure_NamesMemoryCgroupFix(t *testing.T) {
	executor := &mockCgroupExecutor{execFunc: func(command string) (*ssh.ExecResult, error) {
		if strings.Contains(command, "journalctl") {
			return &ssh.ExecResult{Stdout: `level=fatal msg="Error: failed to find memory cgroup (v2)"`, ExitCode: 0}, nil
		}
		return &ssh.ExecResult{ExitCode: 0}, nil
	}}

	diagnosis := diagnoseServiceFailure(executor, "k3s-agent")
	assert.Contains(t, diagnosis, "memory cgroup controller")
	assert.Contains(t, diagnosis, "foundry host configure")
}

func TestDiagnoseServiceFailure_NoLogsReturnsEmpty(t *testing.T) {
	executor := &mockCgroupExecutor{execFunc: func(command string) (*ssh.ExecResult, error) {
		return &ssh.ExecResult{Stdout: "", ExitCode: 0}, nil
	}}

	assert.Empty(t, diagnoseServiceFailure(executor, "k3s-agent"))
}
