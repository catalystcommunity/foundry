package k3s

import (
	"bytes"
	"context"
	"encoding/json"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/catalystcommunity/foundry/v1/internal/ssh"
	"github.com/stretchr/testify/require"
)

type isolationContainer struct{ id string }

func (e isolationContainer) Exec(command string) (*ssh.ExecResult, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "docker", "exec", e.id, "sh", "-c", strings.TrimPrefix(command, "sudo "))
	var out, stderr bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &stderr
	err := cmd.Run()
	result := &ssh.ExecResult{Stdout: out.String(), Stderr: stderr.String()}
	if exit, ok := err.(*exec.ExitError); ok {
		result.ExitCode = exit.ExitCode()
		return result, nil
	}
	return result, err
}

// The interface, ARP, routes, files and sysctls are real, in disposable network
// namespaces. No host interfaces or host sysctls are changed.
func TestVIPIsolationLinux(t *testing.T) {
	if testing.Short() {
		t.Skip("requires Docker")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	build := exec.CommandContext(ctx, "docker", "build", "-t", "foundry-vip-network-test", "-")
	build.Stdin = strings.NewReader("FROM python:3.13-alpine\nRUN apk add --no-cache iproute2 procps iputils\n")
	out, err := build.CombinedOutput()
	require.NoError(t, err, string(out))
	out, err = exec.CommandContext(ctx, "docker", "run", "-d", "--rm", "--privileged", "--network=none", "foundry-vip-network-test", "sleep", "300").CombinedOutput()
	require.NoError(t, err, string(out))
	id := strings.TrimSpace(string(out))
	t.Cleanup(func() { require.NoError(t, exec.Command("docker", "rm", "-f", id).Run()) })
	e := isolationContainer{id}
	run := func(command string) string {
		t.Helper()
		out, err := restartExec(e, command)
		require.NoError(t, err)
		return out
	}
	run("ip netns add peer")
	run("ip link add eth0 type veth peer name peer0")
	run("ip link set peer0 netns peer")
	run("ip address add 192.0.2.10/24 dev eth0")
	run("ip link set eth0 up")
	run("ip netns exec peer ip address add 192.0.2.20/24 dev peer0")
	run("ip netns exec peer ip link set peer0 up")
	apply := func() error {
		_, err := restartExec(e, "python3 -c "+shellQuote(vipInterfaceScript)+" eth0")
		return err
	}
	require.NoError(t, apply())
	first := run("cat /sys/class/net/foundry-vip/ifindex")
	require.NoError(t, apply())
	require.Equal(t, first, run("cat /sys/class/net/foundry-vip/ifindex"))
	run("ip address add 192.0.2.100/32 dev foundry-vip")
	run("ip netns exec peer ping -c 2 -W 2 192.0.2.100")
	var links []isolationLink
	require.NoError(t, json.Unmarshal([]byte(run("ip -j -d address show")), &links))
	parent, flannel, address, err := selectIsolationInterfaces(links, "192.0.2.10", "eth0", VIPInterfaceName)
	require.NoError(t, err)
	require.Equal(t, "eth0", parent)
	require.Equal(t, "eth0", flannel)
	require.Equal(t, "192.0.2.10", address)
	// A routed CGNAT VIP reaches the local address without ARP on an overlay.
	run("ip address add 100.80.0.100/32 dev foundry-vip")
	run("ip netns exec peer ip route add 100.80.0.100/32 via 192.0.2.10")
	run("ip netns exec peer ping -c 2 -W 2 100.80.0.100")
	require.NotContains(t, run("ip -4 address show dev eth0"), "192.0.2.100")
	require.Equal(t, "2", run("sysctl -n net.ipv4.conf.foundry-vip.rp_filter"))
	require.NoError(t, writeIsolationFile(e, "/tmp/foundry-network/config", isolationConfig(isolationPlan{IP: "192.0.2.10", Flannel: "eth0"}), 0644))
	stamp := run("stat -c '%i:%Y' /tmp/foundry-network/config")
	require.NoError(t, writeIsolationFile(e, "/tmp/foundry-network/config", isolationConfig(isolationPlan{IP: "192.0.2.10", Flannel: "eth0"}), 0644))
	require.Equal(t, stamp, run("stat -c '%i:%Y' /tmp/foundry-network/config"))
	run("printf user-owned > /tmp/unmanaged")
	require.ErrorContains(t, writeIsolationFile(e, "/tmp/unmanaged", "replacement", 0644), "unmanaged")
	require.Equal(t, "user-owned", run("cat /tmp/unmanaged"))
	run("ln -s /tmp/unmanaged /tmp/link")
	require.ErrorContains(t, writeIsolationFile(e, "/tmp/link", "replacement", 0644), "symlink")
	run("ip link delete foundry-vip")
	require.NoError(t, apply()) // Recreate after a reboot-like loss of the interface.
	run("ip link set foundry-vip alias user-owned")
	require.ErrorContains(t, apply(), "unmanaged")
	run("ip link delete foundry-vip")
	run("ip link set eth0 down")
	require.ErrorContains(t, apply(), "not up")
	_, err = restartExec(e, "python3 -c "+shellQuote(vipInterfaceScript)+" lo")
	require.ErrorContains(t, err, "Ethernet")
}
