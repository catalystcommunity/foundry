package nodeconfig

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/catalystcommunity/foundry/v1/internal/ssh"
	"github.com/stretchr/testify/require"
)

// Docker replaces only SSH transport. The shell, files, and sysctl are real.
// kernel.msgmax is isolated by the container's private IPC namespace. Tests
// must not write fs.inotify sysctls, which would affect the Docker host.
type containerExecutor struct{ id string }

func (e containerExecutor) Exec(command string) (*ssh.ExecResult, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "docker", "exec", e.id, "sh", "-c", strings.TrimPrefix(command, "sudo "))
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	err := cmd.Run()
	result := &ssh.ExecResult{Stdout: stdout.String(), Stderr: stderr.String()}
	if exit, ok := err.(*exec.ExitError); ok {
		result.ExitCode = exit.ExitCode()
		return result, nil
	}
	return result, err
}

func TestSysctlMinimumContainer(t *testing.T) {
	if testing.Short() {
		t.Skip("requires Docker and a private IPC namespace")
	}
	if _, err := exec.LookPath("docker"); err != nil {
		t.Skip("Docker is unavailable")
	}
	before, err := os.ReadFile("/proc/sys/kernel/msgmax")
	require.NoError(t, err)
	t.Cleanup(func() {
		after, err := os.ReadFile("/proc/sys/kernel/msgmax")
		require.NoError(t, err)
		require.Equal(t, string(before), string(after), "host sysctl must not change")
	})
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	out, err := exec.CommandContext(ctx, "docker", "run", "-d", "--rm", "--privileged", "--ipc=private", "--network=none", "alpine:3.22", "sleep", "300").CombinedOutput()
	require.NoError(t, err, string(out))
	id := strings.TrimSpace(string(out))
	t.Cleanup(func() { require.NoError(t, exec.Command("docker", "rm", "-f", id).Run()) })
	e := containerExecutor{id}
	run := func(command string) string {
		t.Helper()
		result, err := e.Exec(command)
		require.NoError(t, err)
		require.Zero(t, result.ExitCode, result.Stderr)
		return strings.TrimSpace(result.Stdout)
	}
	const key = "kernel.msgmax"
	const path = "/etc/sysctl.d/90-foundry-test.conf"
	for _, tc := range []struct{ name, current, saved, expected string }{
		{"raise low limit", "128", "", "8192"},
		{"preserve higher active limit", "16384", "", "16384"},
		{"restore higher saved limit", "128", "32768", "32768"},
		{"raise lower saved limit", "128", "256", "8192"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			run("rm -f " + path + "; sysctl -w " + key + "=" + tc.current)
			if tc.saved != "" {
				run("mkdir -p /etc/sysctl.d; echo 'kernel.msgmax = " + tc.saved + "' > " + path)
			}
			require.NoError(t, ensureSysctlMinimum(e, key, InotifyInstancesMinimum, path))
			require.Equal(t, tc.expected, run("sysctl -n "+key))
			require.Contains(t, run("cat "+path), key+" = "+tc.expected)
			require.Equal(t, "644", run("stat -c %a "+path))
			run("touch -t 202001010000 " + path)
			stamp := run("stat -c %Y " + path)
			require.NoError(t, ensureSysctlMinimum(e, key, InotifyInstancesMinimum, path))
			require.Equal(t, stamp, run("stat -c %Y "+path), "second run must not rewrite the file")
			// Simulate a runtime reset and reload the persistent file.
			run("sysctl -w " + key + "=128; sysctl -p " + path)
			require.Equal(t, tc.expected, run("sysctl -n "+key))
		})
	}
	t.Run("invalid managed file", func(t *testing.T) {
		for _, saved := range []string{"kernel.msgmax = invalid", "kernelXmsgmax = 16384", "kernel.msgmax = 0", "kernel.msgmax = 8192\nkernel.msgmax = 16384", "kernel.msgmax = 8192\nfs.inotify.max_user_watches = 1048576"} {
			run("printf '%s\\n' '" + saved + "' > " + path)
			require.ErrorContains(t, ensureSysctlMinimum(e, key, InotifyInstancesMinimum, path), "Invalid")
			require.Equal(t, saved, run("cat "+path))
		}
	})
	t.Run("missing sysctl", func(t *testing.T) {
		require.Error(t, ensureSysctlMinimum(e, "kernel.foundry_nonexistent", InotifyInstancesMinimum, "/tmp/missing.conf"))
		run("test ! -e /tmp/missing.conf")
	})
	t.Run("cannot persist", func(t *testing.T) {
		run("touch /tmp/not-a-directory")
		require.Error(t, ensureSysctlMinimum(e, key, InotifyInstancesMinimum, "/tmp/not-a-directory/value.conf"))
	})
	t.Run("kernel refuses value", func(t *testing.T) {
		run("rm -f " + path + "; sysctl -w " + key + "=128")
		require.Error(t, ensureSysctlMinimum(e, key, 2147483647+1, path))
		require.Equal(t, "128", run("sysctl -n "+key))
	})
}
