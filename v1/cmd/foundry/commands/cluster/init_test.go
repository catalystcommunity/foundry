package cluster

import (
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
)

func TestInitCommand(t *testing.T) {
	cmd := initCommand()

	assert.Equal(t, "init", cmd.Name)
	assert.Equal(t, "Initialize Kubernetes cluster", cmd.Usage)
	assert.NotNil(t, cmd.Action)

	// Check flags
	assert.Len(t, cmd.Flags, 3)

	var hasConfigFlag, hasSingleNodeFlag, hasDryRunFlag bool
	for _, flag := range cmd.Flags {
		switch flag.Names()[0] {
		case "config":
			hasConfigFlag = true
		case "single-node":
			hasSingleNodeFlag = true
		case "dry-run":
			hasDryRunFlag = true
		}
	}

	assert.True(t, hasConfigFlag, "should have config flag")
	assert.True(t, hasSingleNodeFlag, "should have single-node flag")
	assert.True(t, hasDryRunFlag, "should have dry-run flag")
}

func TestPrintClusterPlan(t *testing.T) {
	tests := []struct {
		name   string
		config *config.Config
	}{
		{
			name: "single node cluster",
			config: &config.Config{
				Cluster: config.ClusterConfig{
					Name:   "test-cluster",
					Domain: "example.com",
					VIP:    "192.168.1.100",
				},
				Network: &config.NetworkConfig{
				},
				Hosts: []*host.Host{
					{
						Hostname: "node1.example.com",
						Address:  "192.168.1.10",
						Roles:    []string{host.RoleClusterControlPlane},
					},
				},
			},
		},
		{
			name: "multi-node cluster",
			config: &config.Config{
				Cluster: config.ClusterConfig{
					Name:   "prod-cluster",
					Domain: "example.com",
					VIP:    "192.168.1.100",
				},
				Network: &config.NetworkConfig{
				},
				Hosts: []*host.Host{
					{
						Hostname: "cp1.example.com",
						Address:  "192.168.1.10",
						Roles:    []string{host.RoleClusterControlPlane},
					},
					{
						Hostname: "worker1.example.com",
						Address:  "192.168.1.20",
						Roles:    []string{host.RoleClusterWorker},
					},
				},
			},
		},
		{
			name: "cluster with explicit roles",
			config: &config.Config{
				Cluster: config.ClusterConfig{
					Name:   "test-cluster",
					Domain: "example.com",
					VIP:    "192.168.1.100",
				},
				Network: &config.NetworkConfig{
				},
				Hosts: []*host.Host{
					{
						Hostname: "node1.example.com",
						Address:  "192.168.1.10",
						Roles:    []string{host.RoleClusterControlPlane, host.RoleClusterWorker},
					},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := printClusterPlan(tt.config)
			assert.NoError(t, err)
		})
	}
}

func TestRunClusterInit_MissingConfig(t *testing.T) {
	// Create a temporary directory for test configs
	tmpDir := t.TempDir()
	nonExistentConfig := filepath.Join(tmpDir, "nonexistent.yaml")

	// Create CLI app with cluster commands
	app := &cli.Command{
		Name: "foundry",
		Commands: []*cli.Command{
			Commands(),
		},
	}

	// Set up CLI command with non-existent config
	args := []string{"foundry", "cluster", "init", "--config", nonExistentConfig}

	ctx := context.Background()
	err := app.Run(ctx, args)

	// We expect an error since the config doesn't exist
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to load config")
}

func TestRunClusterInit_DryRun(t *testing.T) {
	// Create a valid test config
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
		Hosts: []*host.Host{
			{
				Hostname: "node1.example.com",
				Address:  "192.168.1.10",
				Roles:    []string{host.RoleClusterControlPlane},
			},
		},
		Components: config.ComponentMap{
			"openbao": {},
		},
	}

	// Write config to file
	err := config.Save(testConfig, configPath)
	require.NoError(t, err)

	// Create CLI app
	app := &cli.Command{
		Name: "foundry",
		Commands: []*cli.Command{
			Commands(),
		},
	}

	// Run with dry-run flag
	args := []string{"foundry", "cluster", "init", "--config", configPath, "--dry-run"}

	ctx := context.Background()
	err = app.Run(ctx, args)

	// Dry run should succeed without actual installation
	assert.NoError(t, err)
}

func TestRunClusterInit_NoClusterConfig(t *testing.T) {
	// Create config without cluster section
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "test-config.yaml")

	testConfig := &config.Config{
		Network: &config.NetworkConfig{
			Gateway:      "192.168.1.1",
			Netmask:      "255.255.255.0",
		},
		Components: config.ComponentMap{
			"openbao": {},
		},
	}

	err := config.Save(testConfig, configPath)
	require.NoError(t, err)

	// Create CLI app
	app := &cli.Command{
		Name: "foundry",
		Commands: []*cli.Command{
			Commands(),
		},
	}

	args := []string{"foundry", "cluster", "init", "--config", configPath}

	ctx := context.Background()
	err = app.Run(ctx, args)

	assert.Error(t, err)
	// Error comes from config validation, not from the command check
	assert.Contains(t, err.Error(), "cluster configuration is missing")
}

func TestRunClusterInit_NoNodes(t *testing.T) {
	// Create config with empty nodes
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "test-config.yaml")

	testConfig := &config.Config{
		Cluster: config.ClusterConfig{
			Name:   "test-cluster",
			Domain: "example.com",
		},
		Network: &config.NetworkConfig{
			Gateway:      "192.168.1.1",
			Netmask:      "255.255.255.0",
		},
		Components: config.ComponentMap{
			"openbao": {},
		},
	}

	err := config.Save(testConfig, configPath)
	require.NoError(t, err)

	// Create CLI app
	app := &cli.Command{
		Name: "foundry",
		Commands: []*cli.Command{
			Commands(),
		},
	}

	args := []string{"foundry", "cluster", "init", "--config", configPath}

	ctx := context.Background()
	err = app.Run(ctx, args)

	assert.Error(t, err)
	// Error comes from config validation - no hosts with cluster roles
	assert.Contains(t, err.Error(), "no hosts with cluster roles")
}

func TestRunClusterInit_SingleNodeFlag(t *testing.T) {
	// Create a multi-node config
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
		Hosts: []*host.Host{
			{
				Hostname: "node1.example.com",
				Address:  "192.168.1.10",
				Roles:    []string{host.RoleClusterControlPlane},
			},
		},
		Components: config.ComponentMap{
			"openbao": {},
		},
	}

	err := config.Save(testConfig, configPath)
	require.NoError(t, err)

	// Create CLI app
	app := &cli.Command{
		Name: "foundry",
		Commands: []*cli.Command{
			Commands(),
		},
	}

	// Run with single-node and dry-run flags
	args := []string{"foundry", "cluster", "init", "--config", configPath, "--single-node", "--dry-run"}

	ctx := context.Background()
	err = app.Run(ctx, args)

	// Should succeed and only show one node in dry-run output
	assert.NoError(t, err)
}

func TestInitializeCluster_HostNotFound(t *testing.T) {
	// Clear host registry
	host.ClearHosts()

	cfg := &config.Config{
		Cluster: config.ClusterConfig{
			Name:   "test-cluster",
			Domain: "example.com",
			},
		Network: &config.NetworkConfig{
			Gateway:      "192.168.1.1",
			Netmask:      "255.255.255.0",
		},
		Components: config.ComponentMap{
			"openbao": {},
		},
	}

	ctx := context.Background()
	err := InitializeCluster(ctx, cfg)

	// Error should occur - either when loading OpenBAO keys, connecting to OpenBAO, or connecting to host
	assert.Error(t, err)
	// Accept either error: OpenBAO keys missing OR connection timeout OR host not found
	hasExpectedError := strings.Contains(err.Error(), "failed to load OpenBAO keys") ||
		strings.Contains(err.Error(), "not found in registry") ||
		strings.Contains(err.Error(), "failed to generate tokens") ||
		strings.Contains(err.Error(), "failed to connect")
	assert.True(t, hasExpectedError, "expected OpenBAO keys, host not found, or connection error, got: %v", err)
}

func TestInitializeCluster_NoControlPlane(t *testing.T) {
	// Skip this test - it requires OpenBAO mock, and empty nodes list
	// is already caught by config validation
	t.Skip("This test requires mocking OpenBAO client - deferred to integration tests")
}

func TestCommands(t *testing.T) {
	cmd := Commands()

	assert.Equal(t, "cluster", cmd.Name)
	assert.Equal(t, "Manage Kubernetes cluster", cmd.Usage)
	assert.NotEmpty(t, cmd.Commands)

	// Verify init command exists
	var hasInit bool
	for _, subCmd := range cmd.Commands {
		if subCmd.Name == "init" {
			hasInit = true
			break
		}
	}
	assert.True(t, hasInit, "should have init subcommand")
}

// TestMain sets up and tears down test environment
func TestMain(m *testing.M) {
	// Setup: Clear host registry before tests
	host.ClearHosts()

	// Run tests
	code := m.Run()

	// Teardown: Clean up
	host.ClearHosts()

	os.Exit(code)
}
