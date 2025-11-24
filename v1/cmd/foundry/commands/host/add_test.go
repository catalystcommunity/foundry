package host

import (
	"testing"

	"github.com/catalystcommunity/foundry/v1/internal/host"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPrompt(t *testing.T) {
	// Basic test to ensure prompt function exists and can be called
	// Note: Full interactive testing would require mocking stdin
	assert.NotNil(t, prompt)
}

func TestGetRegistry(t *testing.T) {
	// Reset registry before test
	ResetRegistry()

	// Add a test host using global functions
	h := host.DefaultHost("test-host", "192.168.1.100", "user")
	err := host.Add(h)
	require.NoError(t, err)

	// Verify host was added
	retrieved, err := host.Get("test-host")
	require.NoError(t, err)
	assert.Equal(t, "test-host", retrieved.Hostname)
	assert.Equal(t, "192.168.1.100", retrieved.Address)
	assert.Equal(t, "user", retrieved.User)
}

func TestResetRegistry(t *testing.T) {
	// Clean up first
	ResetRegistry()

	// Add a host
	h := host.DefaultHost("test-host", "192.168.1.100", "user")
	err := host.Add(h)
	require.NoError(t, err)

	// Reset
	ResetRegistry()

	// Verify registry is empty
	hosts, err := host.List()
	require.NoError(t, err)
	assert.Equal(t, 0, len(hosts))
}

func TestInstallPublicKey(t *testing.T) {
	// This is a unit test that validates the function exists
	// Full integration testing with an actual SSH connection is done in Task 26
	assert.NotNil(t, installPublicKey)
}
