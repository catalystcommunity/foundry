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
	// First check if sudo command exists
	result, err := executor.Exec("which sudo")
	if err != nil {
		return SudoNotInstalled, fmt.Errorf("failed to check for sudo: %w", err)
	}

	if result.ExitCode != 0 {
		return SudoNotInstalled, nil
	}

	// Check if user can run sudo without password
	result, err = executor.Exec("sudo -n true 2>&1")
	if err != nil {
		return SudoNoAccess, fmt.Errorf("failed to test sudo access: %w", err)
	}

	// Exit code 0 means passwordless sudo works
	if result.ExitCode == 0 {
		return SudoPasswordless, nil
	}

	// Check stderr for password requirement indicator
	stderr := result.Stderr
	if stderr == "" {
		stderr = result.Stdout // Some systems output to stdout
	}

	if strings.Contains(stderr, "password is required") ||
		strings.Contains(stderr, "a password is required") {
		return SudoRequiresPassword, nil
	}

	// User is not in sudoers or other access issue
	return SudoNoAccess, nil
}

// CheckSudoAccess checks if the current user has sudo access (with or without password)
// Deprecated: Use GetSudoStatus for more granular information
func CheckSudoAccess(executor CommandExecutor, user string) (bool, error) {
	status, err := GetSudoStatus(executor)
	if err != nil {
		return false, err
	}
	return status == SudoPasswordless || status == SudoRequiresPassword, nil
}

// SetupSudo installs sudo and configures it for the specified user
// This requires the root password to execute commands as root via su
func SetupSudo(executor CommandExecutor, user string, rootPassword string) error {
	// Commands to run as root to setup sudo
	// Using full paths for system commands since su -c may have limited PATH
	commands := []string{
		// Install sudo if not present
		"/usr/bin/apt-get update -qq && /usr/bin/apt-get install -y sudo",

		// Add user to sudo group (usermod is in /usr/sbin)
		fmt.Sprintf("/usr/sbin/usermod -aG sudo %s", user),

		// Configure passwordless sudo for this user
		// This makes future operations smoother
		fmt.Sprintf("echo '%s ALL=(ALL) NOPASSWD:ALL' > /etc/sudoers.d/%s", user, user),
		fmt.Sprintf("/bin/chmod 0440 /etc/sudoers.d/%s", user),
	}

	// Execute each command as root using su
	for i, cmd := range commands {
		// Construct su command with password from stdin
		// Using -c to run a single command as root
		// The - flag ensures a login shell with proper PATH
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

// escapeForShell escapes a string for use in single-quoted shell command
// Single quotes can't be escaped inside single quotes, so we end the quote,
// add an escaped single quote, then start a new quote
func escapeForShell(s string) string {
	// Replace single quotes with '\'' (end quote, escaped quote, start quote)
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
