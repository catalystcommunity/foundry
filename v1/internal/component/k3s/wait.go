package k3s

import (
	"fmt"
	"strings"
	"time"
)

// Reconnector is implemented by executors that can re-establish their
// connection. The wait loops use it to survive a host reboot instead of
// retrying forever against a connection that is already dead.
type Reconnector interface {
	Reconnect() error
}

// progressInterval is how many attempts pass between progress messages, so a
// long wait reports what it is waiting for instead of going silent.
const progressInterval = 3

// waitForServiceActive waits for a systemd service to report "active".
// It reports progress, recovers the connection when the host reboots, and on
// failure returns an error carrying the service logs that explain why.
func waitForServiceActive(executor SSHExecutor, service string, retryCfg RetryConfig) error {
	lastState := "unknown"

	for attempt := 0; attempt < retryCfg.MaxRetries; attempt++ {
		result, err := executor.Exec(fmt.Sprintf("sudo systemctl is-active %s 2>/dev/null", service))
		if err == nil && result != nil {
			lastState = strings.TrimSpace(result.Stdout)
			if lastState == "active" {
				if attempt > 0 {
					fmt.Printf("   ✓ %s is active\n", service)
				}
				return nil
			}
		} else {
			// The host may be rebooting or the connection dropped
			lastState = "unreachable"
			if reconnect(executor) {
				fmt.Printf("   ℹ Lost the connection to the host and reconnected; still waiting for %s\n", service)
				continue
			}
		}

		if attempt == 0 || (attempt+1)%progressInterval == 0 {
			fmt.Printf("   … waiting for %s (state: %s, attempt %d/%d)\n",
				service, lastState, attempt+1, retryCfg.MaxRetries)
		}

		time.Sleep(retryCfg.RetryDelay)
	}

	waited := time.Duration(retryCfg.MaxRetries) * retryCfg.RetryDelay
	if lastState == "unreachable" {
		return fmt.Errorf("%s did not become ready after %v: the host stayed unreachable, check that it finished rebooting and that SSH is available",
			service, waited)
	}
	return fmt.Errorf("%s did not become ready after %v (last state: %s)%s",
		service, waited, lastState, diagnoseServiceFailure(executor, service))
}

// reconnect asks the executor to re-establish its connection, if it can.
func reconnect(executor SSHExecutor) bool {
	reconnector, ok := executor.(Reconnector)
	if !ok {
		return false
	}
	return reconnector.Reconnect() == nil
}

// diagnoseServiceFailure collects the failing service's own log lines so the
// operator sees the real cause instead of only a timeout, and names the fix
// for causes we recognize.
func diagnoseServiceFailure(executor SSHExecutor, service string) string {
	result, err := executor.Exec(fmt.Sprintf(
		"sudo journalctl -u %s -n 50 --no-pager 2>/dev/null | grep -iE 'level=fatal|level=error|error:' | tail -3", service))
	if err != nil || result == nil {
		return ""
	}

	logLines := strings.TrimSpace(result.Stdout)
	if logLines == "" {
		return ""
	}

	diagnosis := fmt.Sprintf("\n\nLast errors from %s on the host:\n%s", service, logLines)
	if strings.Contains(strings.ToLower(logLines), "memory cgroup") {
		diagnosis += "\n\nThe host is missing the memory cgroup controller that K3s requires." +
			"\nRun 'foundry host configure <host>' to add the boot flags, reboot the host, then retry."
	}
	return diagnosis
}
