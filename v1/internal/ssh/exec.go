package ssh

import (
	"bytes"
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
	c.mu.Lock()
	client := c.client
	c.mu.Unlock()

	if client == nil {
		return nil, fmt.Errorf("connection is not established")
	}

	if command == "" {
		return nil, fmt.Errorf("command cannot be empty")
	}

	type execOutcome struct {
		result *ExecResult
		err    error
	}
	outcome := make(chan execOutcome, 1)

	go func() {
		// NewSession must run inside the timed goroutine. On a half-open
		// connection (the host rebooted) it blocks until the transport
		// fails, so leaving it outside makes the timeout meaningless.
		session, err := client.NewSession()
		if err != nil {
			outcome <- execOutcome{err: fmt.Errorf("failed to create session: %w", err)}
			return
		}
		defer session.Close()

		var stdout, stderr bytes.Buffer
		session.Stdout = &stdout
		session.Stderr = &stderr

		runErr := session.Run(command)
		result := &ExecResult{
			Stdout:   stdout.String(),
			Stderr:   stderr.String(),
			ExitCode: 0,
		}

		if runErr != nil {
			// An exit error is a result, not a failure to execute
			if exitErr, ok := runErr.(*ssh.ExitError); ok {
				result.ExitCode = exitErr.ExitStatus()
			} else {
				outcome <- execOutcome{err: fmt.Errorf("failed to execute command: %w", runErr)}
				return
			}
		}

		outcome <- execOutcome{result: result}
	}()

	timer := time.NewTimer(timeout)
	defer timer.Stop()

	select {
	case <-timer.C:
		return nil, fmt.Errorf("command timed out after %v", timeout)
	case out := <-outcome:
		return out.result, out.err
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
