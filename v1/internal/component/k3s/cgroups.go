package k3s

import (
	"fmt"
	"strings"
)

// memoryCgroupBootFlags are the kernel command line flags that enable the
// memory cgroup controller. Raspberry Pi OS ships with the controller
// disabled, and K3s refuses to start without it.
const memoryCgroupBootFlags = "cgroup_memory=1 cgroup_enable=memory"

// bootCmdlinePaths are checked in order for the kernel command line file.
// Raspberry Pi OS bookworm uses /boot/firmware/cmdline.txt; older releases
// use /boot/cmdline.txt.
var bootCmdlinePaths = []string{"/boot/firmware/cmdline.txt", "/boot/cmdline.txt"}

// HasMemoryCgroup reports whether the memory cgroup controller is available
// on the host, for both cgroup v2 and v1 hierarchies.
func HasMemoryCgroup(executor SSHExecutor) (bool, error) {
	// cgroup v2: the controller appears in cgroup.controllers
	result, err := executor.Exec("grep -qw memory /sys/fs/cgroup/cgroup.controllers 2>/dev/null && echo ok")
	if err != nil {
		return false, fmt.Errorf("failed to check cgroup v2 controllers: %w", err)
	}
	if strings.TrimSpace(result.Stdout) == "ok" {
		return true, nil
	}

	// cgroup v1: the memory line in /proc/cgroups has enabled=1
	result, err = executor.Exec(`awk '$1 == "memory" {print $4}' /proc/cgroups 2>/dev/null`)
	if err != nil {
		return false, fmt.Errorf("failed to check cgroup v1 controllers: %w", err)
	}
	return strings.TrimSpace(result.Stdout) == "1", nil
}

// EnableMemoryCgroupBootFlags appends the memory cgroup kernel flags to the
// host's boot command line file. Returns the path of the file it modified.
// Idempotent: if the flags are already present (host awaiting reboot), the
// file is left untouched. A reboot is required for the flags to take effect.
func EnableMemoryCgroupBootFlags(executor SSHExecutor) (string, error) {
	var cmdlinePath string
	for _, path := range bootCmdlinePaths {
		result, err := executor.Exec(fmt.Sprintf("test -f %s && echo ok", path))
		if err == nil && strings.TrimSpace(result.Stdout) == "ok" {
			cmdlinePath = path
			break
		}
	}
	if cmdlinePath == "" {
		return "", fmt.Errorf("no boot cmdline file found (checked %s); enable the memory cgroup manually", strings.Join(bootCmdlinePaths, ", "))
	}

	result, err := executor.Exec(fmt.Sprintf("grep -q cgroup_enable=memory %s && echo ok", cmdlinePath))
	if err != nil {
		return "", fmt.Errorf("failed to inspect %s: %w", cmdlinePath, err)
	}
	if strings.TrimSpace(result.Stdout) == "ok" {
		// Flags already present; the host just needs a reboot
		return cmdlinePath, nil
	}

	// The cmdline file must stay a single line; append to it in place
	commands := []string{
		fmt.Sprintf("sudo cp %s %s.foundry-bak", cmdlinePath, cmdlinePath),
		fmt.Sprintf(`sudo sed -i "s/$/ %s/" %s`, memoryCgroupBootFlags, cmdlinePath),
	}
	for _, cmd := range commands {
		result, err := executor.Exec(cmd)
		if err != nil {
			return "", fmt.Errorf("failed to update %s: %w", cmdlinePath, err)
		}
		if result.ExitCode != 0 {
			return "", fmt.Errorf("failed to update %s: %s", cmdlinePath, strings.TrimSpace(result.Stderr))
		}
	}

	return cmdlinePath, nil
}

// EnsureMemoryCgroup verifies the memory cgroup controller is available
// before a K3s install or join. If it is missing (common on Raspberry Pi OS),
// the boot flags are applied and an error is returned telling the operator to
// reboot the host, so callers fail fast instead of waiting on a K3s service
// that can never start.
func EnsureMemoryCgroup(executor SSHExecutor) error {
	hasMemory, err := HasMemoryCgroup(executor)
	if err != nil {
		return err
	}
	if hasMemory {
		return nil
	}

	cmdlinePath, enableErr := EnableMemoryCgroupBootFlags(executor)
	if enableErr != nil {
		return fmt.Errorf("host lacks the memory cgroup controller required by K3s, and it could not be enabled automatically: %w", enableErr)
	}
	return fmt.Errorf("host lacks the memory cgroup controller required by K3s; boot flags added to %s — reboot the host and retry", cmdlinePath)
}
