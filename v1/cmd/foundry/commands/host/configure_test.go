package host

import (
	"context"
	"testing"

	"github.com/catalystcommunity/foundry/v1/internal/config"
	"github.com/catalystcommunity/foundry/v1/internal/host"
	"github.com/catalystcommunity/foundry/v1/internal/ssh"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/urfave/cli/v3"
)

func TestRunConfigure_NoHostname(t *testing.T) {
	// Reset registry
	ResetRegistry()

	// Create a test app and run with no args
	app := &cli.Command{
		Name: "test",
		Commands: []*cli.Command{
			ConfigureCommand,
		},
	}

	// Run with no hostname argument
	err := app.Run(context.Background(), []string{"test", "configure"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "hostname is required")
}

func TestRunConfigure_HostNotFound(t *testing.T) {
	// Reset registry
	ResetRegistry()

	// Create a test command
	app := &cli.Command{
		Name: "test",
		Commands: []*cli.Command{
			ConfigureCommand,
		},
	}

	// Run with non-existent hostname
	err := app.Run(context.Background(), []string{"test", "configure", "nonexistent"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "host not found")
}

func TestRunConfigure_NoSSHKey(t *testing.T) {
	// Reset registry
	ResetRegistry()

	// Create temp directory for config
	tmpDir := t.TempDir()
	configPath := tmpDir + "/config.yaml"

	// Create a test config file
	testConfig := &config.Config{
		Cluster: config.ClusterConfig{
			Name:          "test-cluster",
			PrimaryDomain: "example.com",
		},
		Network: &config.NetworkConfig{
			Gateway: "192.168.1.1",
			Netmask: "255.255.255.0",
		},
		Components: config.ComponentMap{
			"k3s": {},
		},
	}
	err := config.Save(testConfig, configPath)
	require.NoError(t, err)

	// Add a host without SSH key
	h := &host.Host{
		Hostname:  "test-host",
		Address:   "192.168.1.100",
		Port:      22,
		User:      "testuser",
		SSHKeySet: false,
	}
	err = host.Add(h)
	require.NoError(t, err)

	// Create a test command (--config flag on root, inherited by subcommands)
	app := &cli.Command{
		Name: "test",
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:    "config",
				Aliases: []string{"c"},
				Usage:   "path to config file",
			},
		},
		Commands: []*cli.Command{
			ConfigureCommand,
		},
	}

	// Run configure command with config flag
	err = app.Run(context.Background(), []string{"test", "configure", "test-host", "--config", configPath})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "SSH key not found")
}

func TestRunConfigure_WithSSHKey(t *testing.T) {
	// This test validates the logic flow but doesn't actually connect
	// Full integration testing is done in Task 26

	// Reset registry
	ResetRegistry()

	// Create a temporary keys directory for this test
	tempDir := t.TempDir()
	t.Setenv("FOUNDRY_CONFIG_DIR", tempDir)

	// Add a host with SSH key
	h := &host.Host{
		Hostname:  "test-host",
		Address:   "192.168.1.100",
		Port:      22,
		User:      "testuser",
		SSHKeySet: true,
	}
	err := host.Add(h)
	require.NoError(t, err)

	// Generate and store a key pair in filesystem
	keyPair, err := ssh.GenerateKeyPair()
	require.NoError(t, err)

	keysDir, err := config.GetKeysDir()
	require.NoError(t, err)

	keyStorage, err := ssh.NewFilesystemKeyStorage(keysDir)
	require.NoError(t, err)

	err = keyStorage.Store("test-host", keyPair)
	require.NoError(t, err)

	// Note: We can't actually run the configure command without a real SSH connection
	// This test just validates that the setup is correct
	// Full integration testing with testcontainers is done in Task 26

	// Verify the key is stored in filesystem
	exists, err := keyStorage.Exists("test-host")
	require.NoError(t, err)
	assert.True(t, exists)

	storedKey, err := keyStorage.Load("test-host")
	require.NoError(t, err)
	assert.NotNil(t, storedKey)
}

func TestConfigStep(t *testing.T) {
	step := ConfigStep{
		Name:        "Test Step",
		Description: "Testing...",
		Commands:    []string{"echo 'test'"},
	}

	assert.Equal(t, "Test Step", step.Name)
	assert.Equal(t, "Testing...", step.Description)
	assert.Len(t, step.Commands, 1)
}

// mockConnection implements a minimal ssh.Connection for testing
type mockConnection struct {
	commands []string
	responses map[string]*ssh.ExecResult
}

func newMockConnection() *mockConnection {
	return &mockConnection{
		commands:  []string{},
		responses: make(map[string]*ssh.ExecResult),
	}
}

func (m *mockConnection) Exec(cmd string) (*ssh.ExecResult, error) {
	m.commands = append(m.commands, cmd)

	// Check for exact match first
	if result, ok := m.responses[cmd]; ok {
		return result, nil
	}

	// Check for partial matches (for commands with dynamic parts)
	for pattern, result := range m.responses {
		if len(pattern) > 0 && len(cmd) > 0 && cmd[:min(len(cmd), len(pattern))] == pattern {
			return result, nil
		}
	}

	// Default success
	return &ssh.ExecResult{ExitCode: 0, Stdout: "", Stderr: ""}, nil
}

func (m *mockConnection) Close() error {
	return nil
}

func (m *mockConnection) setResponse(cmdPrefix string, result *ssh.ExecResult) {
	m.responses[cmdPrefix] = result
}

func (m *mockConnection) hasCommand(pattern string) bool {
	for _, cmd := range m.commands {
		if len(cmd) >= len(pattern) && cmd[:len(pattern)] == pattern {
			return true
		}
	}
	return false
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func TestFixAppArmorForContainers_AppArmorNotEnabled(t *testing.T) {
	conn := newMockConnection()
	conn.setResponse("sudo apparmor_status --enabled", &ssh.ExecResult{
		ExitCode: 1, // AppArmor not enabled
		Stdout:   "",
		Stderr:   "",
	})

	err := fixAppArmorForContainers(conn)
	require.NoError(t, err)

	// Should only check AppArmor status, not proceed further
	assert.Len(t, conn.commands, 1)
}

func TestFixAppArmorForContainers_NerdctlNotInstalled(t *testing.T) {
	conn := newMockConnection()
	conn.setResponse("sudo apparmor_status --enabled", &ssh.ExecResult{
		ExitCode: 0, // AppArmor enabled
		Stdout:   "",
		Stderr:   "",
	})
	conn.setResponse("command -v nerdctl", &ssh.ExecResult{
		ExitCode: 1, // nerdctl not found
		Stdout:   "",
		Stderr:   "",
	})

	err := fixAppArmorForContainers(conn)
	require.NoError(t, err)

	// Should check AppArmor and nerdctl, then stop
	assert.Len(t, conn.commands, 2)
}

func TestFixAppArmorForContainers_ProfileAlreadyExists(t *testing.T) {
	conn := newMockConnection()
	conn.setResponse("sudo apparmor_status --enabled", &ssh.ExecResult{
		ExitCode: 0,
		Stdout:   "",
		Stderr:   "",
	})
	conn.setResponse("command -v nerdctl", &ssh.ExecResult{
		ExitCode: 0,
		Stdout:   "/usr/local/bin/nerdctl",
		Stderr:   "",
	})
	conn.setResponse("test -f /etc/apparmor.d/nerdctl-default", &ssh.ExecResult{
		ExitCode: 0, // Profile already exists with fix
		Stdout:   "",
		Stderr:   "",
	})

	err := fixAppArmorForContainers(conn)
	require.NoError(t, err)

	// Should check status, nerdctl, and existing profile
	assert.Len(t, conn.commands, 3)
	assert.False(t, conn.hasCommand("bash -c 'sudo tee"), "should not create profile if it exists")
}

func TestFixAppArmorForContainers_CreatesProfile(t *testing.T) {
	conn := newMockConnection()
	conn.setResponse("sudo apparmor_status --enabled", &ssh.ExecResult{
		ExitCode: 0,
		Stdout:   "",
		Stderr:   "",
	})
	conn.setResponse("command -v nerdctl", &ssh.ExecResult{
		ExitCode: 0,
		Stdout:   "/usr/local/bin/nerdctl",
		Stderr:   "",
	})
	// First test: profile doesn't exist (returns 1)
	conn.setResponse("test -f /etc/apparmor.d/nerdctl-default && grep", &ssh.ExecResult{
		ExitCode: 1, // Profile doesn't exist yet
		Stdout:   "",
		Stderr:   "",
	})
	// Profile creation succeeds
	conn.setResponse("bash -c 'sudo tee", &ssh.ExecResult{
		ExitCode: 0, // Profile created successfully
		Stdout:   "",
		Stderr:   "",
	})
	// Second test: verify file exists after creation (returns 0)
	conn.setResponse("test -f /etc/apparmor.d/nerdctl-default", &ssh.ExecResult{
		ExitCode: 0, // File now exists
		Stdout:   "",
		Stderr:   "",
	})
	// AppArmor parser succeeds
	conn.setResponse("sudo apparmor_parser", &ssh.ExecResult{
		ExitCode: 0,
		Stdout:   "",
		Stderr:   "",
	})

	err := fixAppArmorForContainers(conn)
	require.NoError(t, err)

	// Should create and load the profile
	assert.True(t, conn.hasCommand("bash -c 'sudo tee"), "should create profile file")
	assert.True(t, conn.hasCommand("sudo apparmor_parser"), "should load profile")
}
