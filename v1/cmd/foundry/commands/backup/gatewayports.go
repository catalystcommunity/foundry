package backup

import (
	"fmt"
	"strings"
	"time"

	"github.com/catalystcommunity/foundry/v1/internal/ssh"
)

// The local-backup feature needs a reverse SSH tunnel to bind a cluster node's
// routable IP so that Velero's node-agent pods (on every node) can reach an S3
// endpoint running on the operator's machine. sshd only permits a reverse tunnel
// to bind a non-loopback address when GatewayPorts is yes/clientspecified, so
// foundry enables it on exactly one node for the duration of the backup.
//
// Reversibility is guaranteed through independent layers:
//  1. We only ever add a drop-in file; the main sshd_config is never touched.
//  2. The config is validated (sshd -t) BEFORE every reload, and we reload
//     (SIGHUP) rather than restart, so a bad config or live sessions are safe.
//  3. A systemd "dead-man" timer is armed at enable time that removes the drop-in
//     and reloads sshd after a timeout EVEN IF foundry crashes or loses
//     connectivity. Normal teardown cancels the timer and reverts immediately.
const (
	gatewayPortsDropIn     = "/etc/ssh/sshd_config.d/zz-foundry-backup-gatewayports.conf"
	gatewayPortsRevertUnit = "foundry-gatewayports-revert"
)

// remoteExecutor runs a command on a remote host. *ssh.Connection satisfies it.
type remoteExecutor interface {
	Exec(command string) (*ssh.ExecResult, error)
}

// enableGatewayPortsScript returns a root shell script that enables
// `GatewayPorts clientspecified` via a drop-in, validating before reload and
// arming a dead-man auto-revert timer that fires after revertAfter.
func enableGatewayPortsScript(revertAfter time.Duration) string {
	mins := int(revertAfter.Minutes())
	if mins < 1 {
		mins = 1
	}
	return fmt.Sprintf(`set -e
SSHD=$(command -v sshd || echo /usr/sbin/sshd)
# Clear any prior auto-revert timer so re-running is idempotent.
systemctl stop %[2]s.timer 2>/dev/null || true
systemctl reset-failed %[2]s.timer 2>/dev/null || true
systemctl reset-failed %[2]s.service 2>/dev/null || true
cat > %[1]s <<'CONF'
# Added by 'foundry backup local' to let a reverse SSH tunnel bind this node's IP
# as a temporary off-cluster Velero backup target. Safe to remove.
# Auto-reverts via systemd timer '%[2]s'.
GatewayPorts clientspecified
CONF
# Validate BEFORE reloading; a bad config must never be loaded.
"$SSHD" -t
systemctl reload ssh 2>/dev/null || systemctl reload sshd
# Dead-man's switch: revert even if foundry loses connectivity or crashes.
systemd-run --on-active=%[3]dmin --unit=%[2]s --description='foundry GatewayPorts auto-revert' \
  /bin/sh -c 'rm -f %[1]s; S=$(command -v sshd || echo /usr/sbin/sshd); "$S" -t && { systemctl reload ssh 2>/dev/null || systemctl reload sshd; }' >/dev/null 2>&1
"$SSHD" -T | grep -i gatewayports || true
`, gatewayPortsDropIn, gatewayPortsRevertUnit, mins)
}

// disableGatewayPortsScript returns a root shell script that cancels the
// dead-man timer, removes the drop-in, and reloads sshd (validating first).
func disableGatewayPortsScript() string {
	return fmt.Sprintf(`set -e
systemctl stop %[2]s.timer 2>/dev/null || true
systemctl reset-failed %[2]s.timer 2>/dev/null || true
systemctl reset-failed %[2]s.service 2>/dev/null || true
rm -f %[1]s
SSHD=$(command -v sshd || echo /usr/sbin/sshd)
"$SSHD" -t
systemctl reload ssh 2>/dev/null || systemctl reload sshd
"$SSHD" -T | grep -i gatewayports || true
`, gatewayPortsDropIn, gatewayPortsRevertUnit)
}

// gatewayPortsManager enables/reverts GatewayPorts on one node over SSH using
// passwordless sudo (which foundry configures on managed hosts).
type gatewayPortsManager struct {
	exec remoteExecutor
}

func newGatewayPortsManager(exec remoteExecutor) *gatewayPortsManager {
	return &gatewayPortsManager{exec: exec}
}

// Enable turns on GatewayPorts and arms the dead-man auto-revert timer. The
// returned effective value should be "clientspecified".
func (m *gatewayPortsManager) Enable(revertAfter time.Duration) (string, error) {
	return m.runSudoScript(enableGatewayPortsScript(revertAfter))
}

// Disable cancels the timer and reverts GatewayPorts; effective value -> "no".
func (m *gatewayPortsManager) Disable() (string, error) {
	return m.runSudoScript(disableGatewayPortsScript())
}

// runSudoScript runs the given script as root and returns the trimmed stdout
// (the trailing "sshd -T" GatewayPorts line).
func (m *gatewayPortsManager) runSudoScript(script string) (string, error) {
	cmd := "sudo -n /bin/sh -c " + shellSingleQuote(script)
	res, err := m.exec.Exec(cmd)
	if err != nil {
		return "", fmt.Errorf("ssh exec failed: %w", err)
	}
	if res.ExitCode != 0 {
		return "", fmt.Errorf("remote script failed (exit %d): %s", res.ExitCode, strings.TrimSpace(res.Stderr))
	}
	return strings.TrimSpace(res.Stdout), nil
}

// shellSingleQuote wraps s in single quotes, escaping any embedded single quotes,
// so it can be passed as a single argument to `sh -c`.
func shellSingleQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}
