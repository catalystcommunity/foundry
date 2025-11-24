package cluster

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/catalystcommunity/foundry/v1/internal/config"
	"github.com/catalystcommunity/foundry/v1/internal/host"
	"github.com/catalystcommunity/foundry/v1/internal/ssh"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/urfave/cli/v3"
)

func TestNewNodeRemoveCommand(t *testing.T) {
	cmd := NewNodeRemoveCommand()
	assert.NotNil(t, cmd)
	assert.Equal(t, "remove", cmd.Name)
	assert.Equal(t, "<hostname>", cmd.ArgsUsage)

	// Check flags
	assert.NotNil(t, cmd.Flags)
	flagNames := make(map[string]bool)
	for _, flag := range cmd.Flags {
		for _, name := range flag.Names() {
			flagNames[name] = true
		}
	}
	assert.True(t, flagNames["config"])
	assert.True(t, flagNames["dry-run"])
	assert.True(t, flagNames["force"])
}

func TestNodeRemoveCommand_DryRun(t *testing.T) {
	// Clear registry and add test host
	host.ClearHosts()
	testHost := &host.Host{
		Hostname: "oldnode.example.com",
		Address:  "192.168.1.50",
		Port:     22,
		User:     "admin",
	}
	err := host.Add(testHost)
	require.NoError(t, err)

	// Create test config
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "test-config.yaml")

	testConfig := &config.Config{
		Cluster: config.ClusterConfig{
			Name:   "test-cluster",
			Domain: "example.com",
			VIP:    "192.168.1.100",
		},
		Network: &config.NetworkConfig{
			Gateway: "192.168.1.1",
			Netmask: "255.255.255.0",
		},
		Components: config.ComponentMap{
			"k3s": {},
		},
	}

	err = config.Save(testConfig, configPath)
	require.NoError(t, err)

	// Create CLI app
	app := &cli.Command{
		Name: "foundry",
		Commands: []*cli.Command{
			Commands(),
		},
	}

	// Run with dry-run flag
	args := []string{"foundry", "cluster", "node", "remove", "oldnode.example.com", "--config", configPath, "--dry-run"}

	ctx := context.Background()
	err = app.Run(ctx, args)

	// Dry run should succeed
	assert.NoError(t, err)
}

func TestNodeRemoveCommand_MissingHostname(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "test-config.yaml")

	testConfig := &config.Config{
		Cluster: config.ClusterConfig{
			Name: "test",
		},
		Network: &config.NetworkConfig{
			Gateway:      "192.168.1.1",
			Netmask:      "255.255.255.0",
		},
		Components: config.ComponentMap{
			"k3s": {},
		},
	}

	err := config.Save(testConfig, configPath)
	require.NoError(t, err)

	app := &cli.Command{
		Name: "foundry",
		Commands: []*cli.Command{
			Commands(),
		},
	}

	args := []string{"foundry", "cluster", "node", "remove", "--config", configPath}

	ctx := context.Background()
	err = app.Run(ctx, args)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "hostname argument required")
}

func TestNodeRemoveCommand_HostNotFound(t *testing.T) {
	host.ClearHosts()

	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "test-config.yaml")

	testConfig := &config.Config{
		Cluster: config.ClusterConfig{
			Name:   "test",
			Domain: "example.com",
			VIP:    "192.168.1.100",
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

	app := &cli.Command{
		Name: "foundry",
		Commands: []*cli.Command{
			Commands(),
		},
	}

	args := []string{"foundry", "cluster", "node", "remove", "nonexistent.example.com", "--config", configPath, "--dry-run"}

	ctx := context.Background()
	err = app.Run(ctx, args)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not found in registry")
}

func TestPrintNodeRemovePlan(t *testing.T) {
	cfg := &config.Config{
		Cluster: config.ClusterConfig{
			Name: "prod",
		},
		Network: &config.NetworkConfig{
		},
	}

	// Just verify the function doesn't panic
	// Output verification would require capturing stdout
	printNodeRemovePlan("test.example.com", cfg)
}

func TestUninstallK3s(t *testing.T) {
	tests := []struct {
		name           string
		execResponses  map[string]*ssh.ExecResult
		expectError    bool
		errorContains  string
	}{
		{
			name: "server uninstall script exists and succeeds",
			execResponses: map[string]*ssh.ExecResult{
				"test -f /usr/local/bin/k3s-uninstall.sh":     {ExitCode: 0},
				"/usr/local/bin/k3s-uninstall.sh":             {ExitCode: 0},
			},
			expectError: false,
		},
		{
			name: "agent uninstall script exists and succeeds",
			execResponses: map[string]*ssh.ExecResult{
				"test -f /usr/local/bin/k3s-uninstall.sh":           {ExitCode: 1},
				"test -f /usr/local/bin/k3s-agent-uninstall.sh":     {ExitCode: 0},
				"/usr/local/bin/k3s-agent-uninstall.sh":             {ExitCode: 0},
			},
			expectError: false,
		},
		{
			name: "no uninstall script found",
			execResponses: map[string]*ssh.ExecResult{
				"test -f /usr/local/bin/k3s-uninstall.sh":       {ExitCode: 1},
				"test -f /usr/local/bin/k3s-agent-uninstall.sh": {ExitCode: 1},
			},
			expectError:   true,
			errorContains: "no K3s uninstall script found",
		},
		{
			name: "uninstall script fails",
			execResponses: map[string]*ssh.ExecResult{
				"test -f /usr/local/bin/k3s-uninstall.sh": {ExitCode: 0},
				"/usr/local/bin/k3s-uninstall.sh":         {ExitCode: 1, Stderr: "uninstall failed"},
			},
			expectError:   true,
			errorContains: "failed with exit code 1",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockConn := &mockSSHConnection{
				responses: tt.execResponses,
			}

			err := uninstallK3s(mockConn)

			if tt.expectError {
				assert.Error(t, err)
				if tt.errorContains != "" {
					assert.Contains(t, err.Error(), tt.errorContains)
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

// mockSSHConnection implements the SSHExecutor interface for testing
type mockSSHConnection struct {
	responses map[string]*ssh.ExecResult
}

func (m *mockSSHConnection) Exec(cmd string) (*ssh.ExecResult, error) {
	if result, ok := m.responses[cmd]; ok {
		return result, nil
	}
	// Default: command not found
	return &ssh.ExecResult{ExitCode: 127, Stderr: "command not found"}, nil
}
