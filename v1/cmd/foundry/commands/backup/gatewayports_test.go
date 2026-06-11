package backup

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/catalystcommunity/foundry/v1/internal/ssh"
)

func TestEnableGatewayPortsScript(t *testing.T) {
	s := enableGatewayPortsScript(45 * time.Minute)

	// Only ever adds a drop-in; never edits the main config.
	assert.Contains(t, s, gatewayPortsDropIn)
	assert.NotContains(t, s, "/etc/ssh/sshd_config\n")
	assert.Contains(t, s, "GatewayPorts clientspecified")

	// Validates before reloading; reloads (not restarts).
	tIdx := strings.Index(s, `"$SSHD" -t`)
	rIdx := strings.Index(s, "systemctl reload ssh")
	require.Greater(t, tIdx, 0)
	require.Greater(t, rIdx, tIdx, "must validate (sshd -t) before reloading")
	assert.NotContains(t, s, "systemctl restart ssh")

	// Arms the dead-man timer with the requested timeout.
	assert.Contains(t, s, "systemd-run --on-active=45min")
	assert.Contains(t, s, "--unit="+gatewayPortsRevertUnit)
	// The timer command itself removes the drop-in and reloads sshd.
	assert.Contains(t, s, "rm -f "+gatewayPortsDropIn)
}

func TestEnableGatewayPortsScript_MinTimeout(t *testing.T) {
	// Sub-minute timeouts clamp to 1 minute (systemd-run --on-active min granularity).
	s := enableGatewayPortsScript(10 * time.Second)
	assert.Contains(t, s, "systemd-run --on-active=1min")
}

func TestDisableGatewayPortsScript(t *testing.T) {
	s := disableGatewayPortsScript()
	assert.Contains(t, s, "systemctl stop "+gatewayPortsRevertUnit+".timer")
	assert.Contains(t, s, "rm -f "+gatewayPortsDropIn)
	tIdx := strings.Index(s, `"$SSHD" -t`)
	rIdx := strings.Index(s, "systemctl reload ssh")
	require.Greater(t, tIdx, 0)
	require.Greater(t, rIdx, tIdx, "must validate before reloading on revert too")
}

func TestShellSingleQuote(t *testing.T) {
	assert.Equal(t, `'abc'`, shellSingleQuote("abc"))
	// Embedded single quotes are escaped so the whole thing is one sh -c arg.
	assert.Equal(t, `'a'\''b'`, shellSingleQuote("a'b"))
	// A script with embedded quotes round-trips to valid quoting.
	assert.True(t, strings.HasPrefix(shellSingleQuote("x 'y' z"), "'"))
}

// fakeExec records the command and returns a canned result.
type fakeExec struct {
	lastCmd  string
	stdout   string
	stderr   string
	exitCode int
	err      error
}

func (f *fakeExec) Exec(command string) (*ssh.ExecResult, error) {
	f.lastCmd = command
	if f.err != nil {
		return nil, f.err
	}
	return &ssh.ExecResult{Stdout: f.stdout, Stderr: f.stderr, ExitCode: f.exitCode}, nil
}

func TestGatewayPortsManager_EnableDisable(t *testing.T) {
	fe := &fakeExec{stdout: "gatewayports clientspecified\n"}
	m := newGatewayPortsManager(fe)

	out, err := m.Enable(30 * time.Minute)
	require.NoError(t, err)
	assert.Equal(t, "gatewayports clientspecified", out)
	// Runs through passwordless sudo + sh -c.
	assert.True(t, strings.HasPrefix(fe.lastCmd, "sudo -n /bin/sh -c "))
	assert.Contains(t, fe.lastCmd, "GatewayPorts clientspecified")

	fe.stdout = "gatewayports no\n"
	out, err = m.Disable()
	require.NoError(t, err)
	assert.Equal(t, "gatewayports no", out)
}

func TestGatewayPortsManager_NonZeroExitErrors(t *testing.T) {
	fe := &fakeExec{exitCode: 1, stderr: "sshd: bad config"}
	m := newGatewayPortsManager(fe)
	_, err := m.Enable(5 * time.Minute)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "sshd: bad config")
}
