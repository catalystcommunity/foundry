package ssh

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestConnection_Exec_ValidationErrors(t *testing.T) {
	t.Run("nil client", func(t *testing.T) {
		conn := &Connection{
			Host:   "example.com",
			Port:   22,
			User:   "test",
			client: nil,
		}

		_, err := conn.Exec("echo test")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "connection is not established")
	})

	t.Run("empty command with nil client", func(t *testing.T) {
		// Note: connection check happens before command validation,
		// so this will fail with "connection is not established"
		conn := &Connection{
			Host:   "example.com",
			Port:   22,
			User:   "test",
			client: nil,
		}

		_, err := conn.Exec("")
		require.Error(t, err)
		// Connection check comes first
		assert.Contains(t, err.Error(), "connection is not established")
	})
}

func TestConnection_ExecWithTimeout_ValidationErrors(t *testing.T) {
	t.Run("nil client", func(t *testing.T) {
		conn := &Connection{
			Host:   "example.com",
			Port:   22,
			User:   "test",
			client: nil,
		}

		_, err := conn.ExecWithTimeout("echo test", 5*time.Second)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "connection is not established")
	})

	t.Run("empty command with nil client", func(t *testing.T) {
		// Note: connection check happens before command validation
		conn := &Connection{
			Host:   "example.com",
			Port:   22,
			User:   "test",
			client: nil,
		}

		_, err := conn.ExecWithTimeout("", 5*time.Second)
		require.Error(t, err)
		// Connection check comes first
		assert.Contains(t, err.Error(), "connection is not established")
	})
}

func TestConnection_ExecMultiple_ValidationErrors(t *testing.T) {
	t.Run("nil client", func(t *testing.T) {
		conn := &Connection{
			Host:   "example.com",
			Port:   22,
			User:   "test",
			client: nil,
		}

		_, err := conn.ExecMultiple([]string{"echo test", "ls"})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "connection is not established")
	})

	t.Run("empty commands list", func(t *testing.T) {
		conn := &Connection{
			Host:   "example.com",
			Port:   22,
			User:   "test",
			client: nil,
		}

		results, err := conn.ExecMultiple([]string{})
		assert.NoError(t, err)
		assert.Empty(t, results)
	})
}

func TestExecResult(t *testing.T) {
	result := &ExecResult{
		Stdout:   "test output",
		Stderr:   "test error",
		ExitCode: 1,
	}

	assert.Equal(t, "test output", result.Stdout)
	assert.Equal(t, "test error", result.Stderr)
	assert.Equal(t, 1, result.ExitCode)
}
