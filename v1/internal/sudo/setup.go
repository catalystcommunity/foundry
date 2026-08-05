package sudo

import (
	"fmt"
	"strings"
)

// CommandExecutor is an interface for executing remote commands.
// This avoids importing the ssh package which could create an import cycle.
type CommandExecutor interface {
	Exec(cmd string) (*ExecResult, error)
}

// ExecResult represents the result of a command execution
type ExecResult struct {
	Stdout   string
	Stderr   string
	ExitCode int
}

// SudoStatus represents the state of sudo access for a user
type SudoStatus int

const (
	// SudoNotInstalled means the sudo command is not available on the system
	SudoNotInstalled SudoStatus = iota
	// SudoNoAccess means sudo is installed but user is not in sudoers
	SudoNoAccess
	// SudoRequiresPassword means user has sudo but must enter a password
	SudoRequiresPassword
	// SudoPasswordless means user has full passwordless sudo access
	SudoPasswordless
)

// String returns a human-readable description of the sudo status
func (s SudoStatus) String() string {
	switch s {
	case SudoNotInstalled:
		return "sudo not installed"
	case SudoNoAccess:
		return "user not in sudoers"
	case SudoRequiresPassword:
		return "sudo requires password"
	case SudoPasswordless:
		return "passwordless sudo configured"
	default:
		return "unknown"
	}
}

// GetSudoStatus returns the detailed sudo status for the current user
func GetSudoStatus(executor CommandExecutor) (SudoStatus, error) {
	result, err := executor.Exec("which sudo")
	if err != nil {
		return SudoNotInstalled, fmt.Errorf("failed to check for sudo: %w", err)
	}

	if result.ExitCode != 0 {
		return SudoNotInstalled, nil
	}

	result, err = executor.Exec("sudo -n true 2>&1")
	if err != nil {
		return SudoNoAccess, fmt.Errorf("failed to test sudo access: %w", err)
	}

	if result.ExitCode == 0 {
		return SudoPasswordless, nil
	}

	stderr := result.Stderr
	if stderr == "" {
		stderr = result.Stdout
	}

	if strings.Contains(stderr, "password is required") ||
		strings.Contains(stderr, "a password is required") {
		return SudoRequiresPassword, nil
	}

	return SudoNoAccess, nil
}

// SetupSudo installs sudo and configures it for the specified user
// This requires the root password to execute commands as root via su
func SetupSudo(executor CommandExecutor, user string, rootPassword string) error {
	// A login through su can have a restricted PATH.
	commands := []string{
		"/usr/bin/apt-get update -qq && /usr/bin/apt-get install -y sudo",
		fmt.Sprintf("/usr/sbin/usermod -aG sudo %s", user),
		fmt.Sprintf("echo '%s ALL=(ALL) NOPASSWD:ALL' > /etc/sudoers.d/%s", user, user),
		fmt.Sprintf("/bin/chmod 0440 /etc/sudoers.d/%s", user),
	}

	for i, cmd := range commands {
		suCmd := fmt.Sprintf("echo '%s' | su - root -c '%s'", rootPassword, escapeForShell(cmd))

		result, err := executor.Exec(suCmd)
		if err != nil {
			return fmt.Errorf("command %d failed: %w", i+1, err)
		}

		if result.ExitCode != 0 {
			errMsg := result.Stderr
			if errMsg == "" {
				errMsg = result.Stdout
			}
			return fmt.Errorf("command %d exited with code %d: %s", i+1, result.ExitCode, strings.TrimSpace(errMsg))
		}
	}

	return nil
}

// escapeForShell protects a command that is placed inside single quotes.
func escapeForShell(s string) string {
	return strings.ReplaceAll(s, "'", "'\\''")
}

// IsSudoPasswordless checks if the user can run sudo without a password
func IsSudoPasswordless(executor CommandExecutor) bool {
	result, err := executor.Exec("sudo -n true 2>&1")
	if err != nil {
		return false
	}
	return result.ExitCode == 0
}
