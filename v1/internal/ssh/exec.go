package ssh

import (
	"bytes"
	"context"
	"fmt"
	"time"

	"golang.org/x/crypto/ssh"
)

// ExecResult contains the result of a command execution
type ExecResult struct {
	Stdout   string
	Stderr   string
	ExitCode int
}

// Exec executes a command on the remote host and returns the result
func (c *Connection) Exec(command string) (*ExecResult, error) {
	return c.ExecWithTimeout(command, 60*time.Second)
}

// ExecWithTimeout executes a command with a specified timeout
func (c *Connection) ExecWithTimeout(command string, timeout time.Duration) (*ExecResult, error) {
	if c.client == nil {
		return nil, fmt.Errorf("connection is not established")
	}

	if command == "" {
		return nil, fmt.Errorf("command cannot be empty")
	}

	// Create a context with timeout
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	// Create a new session
	session, err := c.client.NewSession()
	if err != nil {
		return nil, fmt.Errorf("failed to create session: %w", err)
	}
	defer session.Close()

	// Set up buffers for stdout and stderr
	var stdout, stderr bytes.Buffer
	session.Stdout = &stdout
	session.Stderr = &stderr

	// Channel to receive the result
	done := make(chan error, 1)

	// Run the command in a goroutine
	go func() {
		done <- session.Run(command)
	}()

	// Wait for either the command to complete or the context to timeout
	select {
	case <-ctx.Done():
		// Try to signal the session to stop (best effort)
		session.Signal(ssh.SIGTERM)
		session.Close()
		return nil, fmt.Errorf("command timed out after %v", timeout)
	case err := <-done:
		result := &ExecResult{
			Stdout:   stdout.String(),
			Stderr:   stderr.String(),
			ExitCode: 0,
		}

		if err != nil {
			// Check if it's an exit error
			if exitErr, ok := err.(*ssh.ExitError); ok {
				result.ExitCode = exitErr.ExitStatus()
			} else {
				// Other error (connection, etc.)
				return nil, fmt.Errorf("failed to execute command: %w", err)
			}
		}

		return result, nil
	}
}

// ExecMultiple executes multiple commands sequentially
// If any command fails (non-zero exit code), execution stops and returns the error
func (c *Connection) ExecMultiple(commands []string) ([]*ExecResult, error) {
	results := make([]*ExecResult, 0, len(commands))

	for i, cmd := range commands {
		result, err := c.Exec(cmd)
		if err != nil {
			return results, fmt.Errorf("command %d failed: %w", i, err)
		}

		results = append(results, result)

		// Stop on non-zero exit code
		if result.ExitCode != 0 {
			return results, fmt.Errorf("command %d exited with code %d: %s",
				i, result.ExitCode, result.Stderr)
		}
	}

	return results, nil
}
