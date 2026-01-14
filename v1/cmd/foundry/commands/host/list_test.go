package host

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/catalystcommunity/foundry/v1/internal/config"
	"github.com/catalystcommunity/foundry/v1/internal/host"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/urfave/cli/v3"
	"gopkg.in/yaml.v3"
)

// setupTestConfigWithHosts creates a temporary config file for testing
func setupTestConfigWithHosts(t *testing.T, hosts []*host.Host) string {
	tempDir := t.TempDir()
	t.Setenv("FOUNDRY_CONFIG_DIR", tempDir)

	configPath := filepath.Join(tempDir, "stack.yaml")

	cfg := &config.Config{
		Cluster: config.ClusterConfig{
			Name:          "test-cluster",
			PrimaryDomain: "test.local",
			VIP:           "192.168.1.100",
		},
		Components: config.ComponentMap{
			"openbao": config.ComponentConfig{},
		},
		Hosts: hosts,
	}

	data, err := yaml.Marshal(cfg)
	require.NoError(t, err)

	err = os.WriteFile(configPath, data, 0644)
	require.NoError(t, err)

	return configPath
}

func TestRunList_Empty(t *testing.T) {
	// Set up empty config
	setupTestConfigWithHosts(t, []*host.Host{})

	// Create a test app
	var output bytes.Buffer
	app := &cli.Command{
		Name: "test",
		Commands: []*cli.Command{
			ListCommand,
		},
		Writer: &output,
	}

	// Run list command
	err := app.Run(context.Background(), []string{"test", "list"})
	require.NoError(t, err)

	// Check output
	outputStr := output.String()
	assert.Contains(t, outputStr, "No hosts registered")
	assert.Contains(t, outputStr, "foundry host add")
}

func TestRunList_WithHosts(t *testing.T) {
	// Set up config with test hosts
	h1 := &host.Host{
		Hostname:  "host1",
		Address:   "192.168.1.100",
		Port:      22,
		User:      "user1",
		SSHKeySet: true,
	}
	h2 := &host.Host{
		Hostname:  "host2",
		Address:   "192.168.1.101",
		Port:      2222,
		User:      "user2",
		SSHKeySet: false,
	}

	setupTestConfigWithHosts(t, []*host.Host{h1, h2})

	// Create a test app
	var output bytes.Buffer
	app := &cli.Command{
		Name: "test",
		Commands: []*cli.Command{
			ListCommand,
		},
		Writer: &output,
	}

	// Run list command (non-verbose)
	err := app.Run(context.Background(), []string{"test", "list"})
	require.NoError(t, err)

	// Check output
	outputStr := output.String()
	assert.Contains(t, outputStr, "HOSTNAME")
	assert.Contains(t, outputStr, "ADDRESS")
	assert.Contains(t, outputStr, "USER")
	assert.Contains(t, outputStr, "host1")
	assert.Contains(t, outputStr, "host2")
	assert.Contains(t, outputStr, "192.168.1.100")
	assert.Contains(t, outputStr, "192.168.1.101")
	assert.Contains(t, outputStr, "user1")
	assert.Contains(t, outputStr, "user2")

	// Should not contain verbose-only fields
	assert.NotContains(t, outputStr, "PORT")
	assert.NotContains(t, outputStr, "SSH KEY")
}

func TestRunList_Verbose(t *testing.T) {
	// Set up config with a test host
	h := &host.Host{
		Hostname:  "test-host",
		Address:   "192.168.1.100",
		Port:      2222,
		User:      "testuser",
		SSHKeySet: true,
	}

	setupTestConfigWithHosts(t, []*host.Host{h})

	// Create a test command with verbose flag
	var output bytes.Buffer
	app := &cli.Command{
		Name: "test",
		Commands: []*cli.Command{
			ListCommand,
		},
		Writer: &output,
	}

	// Run with verbose flag
	err := app.Run(context.Background(), []string{"test", "list", "--verbose"})
	require.NoError(t, err)

	// Check output contains verbose fields
	outputStr := output.String()
	assert.Contains(t, outputStr, "PORT")
	assert.Contains(t, outputStr, "SSH KEY")
	assert.Contains(t, outputStr, "2222")
	assert.Contains(t, outputStr, "configured")
}

func TestRunList_Sorted(t *testing.T) {
	// Set up config with hosts in random order
	hosts := []*host.Host{
		{Hostname: "zebra", Address: "192.168.1.3", Port: 22, User: "user3", SSHKeySet: false},
		{Hostname: "alpha", Address: "192.168.1.1", Port: 22, User: "user1", SSHKeySet: true},
		{Hostname: "beta", Address: "192.168.1.2", Port: 22, User: "user2", SSHKeySet: false},
	}

	setupTestConfigWithHosts(t, hosts)

	// Create a test app
	var output bytes.Buffer
	app := &cli.Command{
		Name: "test",
		Commands: []*cli.Command{
			ListCommand,
		},
		Writer: &output,
	}

	// Run list command
	err := app.Run(context.Background(), []string{"test", "list"})
	require.NoError(t, err)

	// Check output is sorted
	outputStr := output.String()
	lines := strings.Split(outputStr, "\n")

	// Find the host lines (skip header lines)
	var hostLines []string
	for _, line := range lines {
		if strings.Contains(line, "192.168.1.") {
			hostLines = append(hostLines, line)
		}
	}

	// Verify alphabetical order
	require.Len(t, hostLines, 3)
	assert.Contains(t, hostLines[0], "alpha")
	assert.Contains(t, hostLines[1], "beta")
	assert.Contains(t, hostLines[2], "zebra")
}
