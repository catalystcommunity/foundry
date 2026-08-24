package k3s

import (
	"strings"
	"testing"

	"github.com/catalystcommunity/foundry/v1/internal/ssh"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockCgroupExecutor is a mock for cgroup detection tests
type mockCgroupExecutor struct {
	execFunc func(command string) (*ssh.ExecResult, error)
	history  []string
}

func (m *mockCgroupExecutor) Exec(command string) (*ssh.ExecResult, error) {
	m.history = append(m.history, command)
	if m.execFunc != nil {
		return m.execFunc(command)
	}
	return &ssh.ExecResult{ExitCode: 0}, nil
}

func TestHasMemoryCgroup_V2Present(t *testing.T) {
	mock := &mockCgroupExecutor{execFunc: func(command string) (*ssh.ExecResult, error) {
		if strings.Contains(command, "cgroup.controllers") {
			return &ssh.ExecResult{Stdout: "ok\n", ExitCode: 0}, nil
		}
		return &ssh.ExecResult{ExitCode: 0}, nil
	}}

	hasMemory, err := HasMemoryCgroup(mock)
	require.NoError(t, err)
	assert.True(t, hasMemory)
}

func TestHasMemoryCgroup_V1Present(t *testing.T) {
	mock := &mockCgroupExecutor{execFunc: func(command string) (*ssh.ExecResult, error) {
		if strings.Contains(command, "cgroup.controllers") {
			return &ssh.ExecResult{ExitCode: 1}, nil
		}
		if strings.Contains(command, "/proc/cgroups") {
			return &ssh.ExecResult{Stdout: "1\n", ExitCode: 0}, nil
		}
		return &ssh.ExecResult{ExitCode: 0}, nil
	}}

	hasMemory, err := HasMemoryCgroup(mock)
	require.NoError(t, err)
	assert.True(t, hasMemory)
}

func TestHasMemoryCgroup_Missing(t *testing.T) {
	mock := &mockCgroupExecutor{execFunc: func(command string) (*ssh.ExecResult, error) {
		if strings.Contains(command, "cgroup.controllers") {
			return &ssh.ExecResult{ExitCode: 1}, nil
		}
		if strings.Contains(command, "/proc/cgroups") {
			return &ssh.ExecResult{Stdout: "0\n", ExitCode: 0}, nil
		}
		return &ssh.ExecResult{ExitCode: 0}, nil
	}}

	hasMemory, err := HasMemoryCgroup(mock)
	require.NoError(t, err)
	assert.False(t, hasMemory)
}

func TestEnableMemoryCgroupBootFlags_AppendsFlags(t *testing.T) {
	mock := &mockCgroupExecutor{execFunc: func(command string) (*ssh.ExecResult, error) {
		if strings.Contains(command, "test -f /boot/firmware/cmdline.txt") {
			return &ssh.ExecResult{Stdout: "ok\n", ExitCode: 0}, nil
		}
		if strings.Contains(command, "grep -q cgroup_enable=memory") {
			return &ssh.ExecResult{ExitCode: 1}, nil
		}
		return &ssh.ExecResult{ExitCode: 0}, nil
	}}

	path, err := EnableMemoryCgroupBootFlags(mock)
	require.NoError(t, err)
	assert.Equal(t, "/boot/firmware/cmdline.txt", path)

	history := strings.Join(mock.history, "\n")
	assert.Contains(t, history, "sudo cp /boot/firmware/cmdline.txt /boot/firmware/cmdline.txt.foundry-bak")
	assert.Contains(t, history, "cgroup_memory=1 cgroup_enable=memory")
}

func TestEnableMemoryCgroupBootFlags_LegacyPath(t *testing.T) {
	mock := &mockCgroupExecutor{execFunc: func(command string) (*ssh.ExecResult, error) {
		if strings.Contains(command, "test -f /boot/firmware/cmdline.txt") {
			return &ssh.ExecResult{ExitCode: 1}, nil
		}
		if strings.Contains(command, "test -f /boot/cmdline.txt") {
			return &ssh.ExecResult{Stdout: "ok\n", ExitCode: 0}, nil
		}
		if strings.Contains(command, "grep -q cgroup_enable=memory") {
			return &ssh.ExecResult{ExitCode: 1}, nil
		}
		return &ssh.ExecResult{ExitCode: 0}, nil
	}}

	path, err := EnableMemoryCgroupBootFlags(mock)
	require.NoError(t, err)
	assert.Equal(t, "/boot/cmdline.txt", path)
}

func TestEnableMemoryCgroupBootFlags_AlreadyApplied(t *testing.T) {
	mock := &mockCgroupExecutor{execFunc: func(command string) (*ssh.ExecResult, error) {
		if strings.Contains(command, "test -f /boot/firmware/cmdline.txt") {
			return &ssh.ExecResult{Stdout: "ok\n", ExitCode: 0}, nil
		}
		if strings.Contains(command, "grep -q cgroup_enable=memory") {
			return &ssh.ExecResult{Stdout: "ok\n", ExitCode: 0}, nil
		}
		return &ssh.ExecResult{ExitCode: 0}, nil
	}}

	path, err := EnableMemoryCgroupBootFlags(mock)
	require.NoError(t, err)
	assert.Equal(t, "/boot/firmware/cmdline.txt", path)

	history := strings.Join(mock.history, "\n")
	assert.NotContains(t, history, "sed -i")
}

func TestEnableMemoryCgroupBootFlags_NoCmdlineFile(t *testing.T) {
	mock := &mockCgroupExecutor{execFunc: func(command string) (*ssh.ExecResult, error) {
		if strings.Contains(command, "test -f") {
			return &ssh.ExecResult{ExitCode: 1}, nil
		}
		return &ssh.ExecResult{ExitCode: 0}, nil
	}}

	_, err := EnableMemoryCgroupBootFlags(mock)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no boot cmdline file found")
}

func TestEnsureMemoryCgroup_Present(t *testing.T) {
	mock := &mockCgroupExecutor{execFunc: func(command string) (*ssh.ExecResult, error) {
		if strings.Contains(command, "cgroup.controllers") {
			return &ssh.ExecResult{Stdout: "ok\n", ExitCode: 0}, nil
		}
		return &ssh.ExecResult{ExitCode: 0}, nil
	}}

	assert.NoError(t, EnsureMemoryCgroup(mock))
}

func TestEnsureMemoryCgroup_MissingAppliesFlagsAndErrors(t *testing.T) {
	mock := &mockCgroupExecutor{execFunc: func(command string) (*ssh.ExecResult, error) {
		switch {
		case strings.Contains(command, "cgroup.controllers"):
			return &ssh.ExecResult{ExitCode: 1}, nil
		case strings.Contains(command, "/proc/cgroups"):
			return &ssh.ExecResult{Stdout: "0\n", ExitCode: 0}, nil
		case strings.Contains(command, "test -f /boot/firmware/cmdline.txt"):
			return &ssh.ExecResult{Stdout: "ok\n", ExitCode: 0}, nil
		case strings.Contains(command, "grep -q cgroup_enable=memory"):
			return &ssh.ExecResult{ExitCode: 1}, nil
		}
		return &ssh.ExecResult{ExitCode: 0}, nil
	}}

	err := EnsureMemoryCgroup(mock)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "reboot the host and retry")

	history := strings.Join(mock.history, "\n")
	assert.Contains(t, history, "cgroup_memory=1 cgroup_enable=memory")
}
