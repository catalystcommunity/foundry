// Package nodeconfig configures operating system settings for Foundry nodes.
package nodeconfig

import (
	_ "embed"
	"fmt"
	"strconv"
	"strings"

	"github.com/catalystcommunity/foundry/v1/internal/ssh"
)

const (
	InotifyInstancesMinimum = 8192
	InotifyConfigPath       = "/etc/sysctl.d/90-foundry-inotify.conf"
)

//go:embed sysctl-minimum.sh
var sysctlMinimumScript string

type Executor interface {
	Exec(string) (*ssh.ExecResult, error)
}

// EnsureInotifyInstances persists and applies the minimum without a restart.
// Higher active or previously managed limits are preserved.
func EnsureInotifyInstances(executor Executor) error {
	return ensureSysctlMinimum(executor, "fs.inotify.max_user_instances", InotifyInstancesMinimum, InotifyConfigPath)
}

func ensureSysctlMinimum(executor Executor, key string, minimum int, path string) error {
	quote := func(s string) string { return "'" + strings.ReplaceAll(s, "'", "'\"'\"'") + "'" }
	command := "sudo sh -c " + quote(sysctlMinimumScript) + " -- " + quote(key) + " " + quote(strconv.Itoa(minimum)) + " " + quote(path)
	result, err := executor.Exec(command)
	if err != nil {
		return fmt.Errorf("configure %s: %w", key, err)
	}
	if result == nil {
		return fmt.Errorf("configure %s: remote command returned no result", key)
	}
	if result.ExitCode != 0 {
		return fmt.Errorf("configure %s (exit %d): %s", key, result.ExitCode, strings.TrimSpace(result.Stderr))
	}
	return nil
}
