package cluster

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/catalystcommunity/foundry/v1/internal/component/k3s"
	"github.com/catalystcommunity/foundry/v1/internal/config"
	"github.com/catalystcommunity/foundry/v1/internal/host"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/urfave/cli/v3"
)

func TestNewNodeAddCommand(t *testing.T) {
	cmd := NewNodeAddCommand()
	assert.NotNil(t, cmd)
	assert.Equal(t, "add", cmd.Name)
	assert.Equal(t, "<hostname>", cmd.ArgsUsage)

	// Check flags
	assert.NotNil(t, cmd.Flags)
	flagNames := make(map[string]bool)
	for _, flag := range cmd.Flags {
		for _, name := range flag.Names() {
			flagNames[name] = true
		}
	}
	assert.True(t, flagNames["role"])
	assert.True(t, flagNames["dry-run"])
	// --config is now inherited from root command, not defined on subcommand
}

func TestNodeAddCommand_DryRun(t *testing.T) {
	// Initialize registry and add test host
	host.SetDefaultRegistry(host.NewMemoryRegistry())
	testHost := &host.Host{
		Hostname: "newnode.example.com",
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
			Name:          "test-cluster",
			PrimaryDomain: "example.com",
			VIP:           "192.168.1.100",
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

	// Create CLI app (--config flag on root, inherited by subcommands)
	app := &cli.Command{
		Name: "foundry",
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:    "config",
				Aliases: []string{"c"},
				Usage:   "path to config file",
			},
		},
		Commands: []*cli.Command{
			Commands(),
		},
	}

	// Run with dry-run flag
	args := []string{"foundry", "cluster", "node", "add", "newnode.example.com", "--config", configPath, "--dry-run"}

	ctx := context.Background()
	err = app.Run(ctx, args)

	// Dry run should succeed
	assert.NoError(t, err)
}

func TestNodeAddCommand_MissingHostname(t *testing.T) {
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

	// Create CLI app (--config flag on root, inherited by subcommands)
	app := &cli.Command{
		Name: "foundry",
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:    "config",
				Aliases: []string{"c"},
				Usage:   "path to config file",
			},
		},
		Commands: []*cli.Command{
			Commands(),
		},
	}

	args := []string{"foundry", "cluster", "node", "add", "--config", configPath}

	ctx := context.Background()
	err = app.Run(ctx, args)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "hostname argument required")
}

func TestNodeAddCommand_HostNotFound(t *testing.T) {
	// Initialize empty registry
	host.SetDefaultRegistry(host.NewMemoryRegistry())

	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "test-config.yaml")

	testConfig := &config.Config{
		Cluster: config.ClusterConfig{
			Name:          "test",
			PrimaryDomain: "example.com",
			VIP:           "192.168.1.100",
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

	// Create CLI app (--config flag on root, inherited by subcommands)
	app := &cli.Command{
		Name: "foundry",
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:    "config",
				Aliases: []string{"c"},
				Usage:   "path to config file",
			},
		},
		Commands: []*cli.Command{
			Commands(),
		},
	}

	args := []string{"foundry", "cluster", "node", "add", "nonexistent.example.com", "--config", configPath}

	ctx := context.Background()
	err = app.Run(ctx, args)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not found in registry")
}

func TestDetermineNodeRole_ExplicitControlPlane(t *testing.T) {
	cfg := &config.Config{
		Cluster: config.ClusterConfig{
			Name:          "test-cluster",
			PrimaryDomain: "example.com",
		},
		Hosts: []*host.Host{
			{
				Hostname: "node1.example.com",
				Roles:    []string{host.RoleClusterControlPlane},
			},
		},
	}

	role, err := determineNodeRole(cfg, "control-plane")
	require.NoError(t, err)
	assert.True(t, role.IsControlPlane)
	assert.False(t, role.IsWorker)
}

func TestDetermineNodeRole_ExplicitWorker(t *testing.T) {
	cfg := &config.Config{
		Cluster: config.ClusterConfig{
			Name:          "test-cluster",
			PrimaryDomain: "example.com",
		},
		Hosts: []*host.Host{
			{
				Hostname: "node1.example.com",
				Roles:    []string{host.RoleClusterControlPlane},
			},
		},
	}

	role, err := determineNodeRole(cfg, "worker")
	require.NoError(t, err)
	assert.False(t, role.IsControlPlane)
	assert.True(t, role.IsWorker)
}

func TestDetermineNodeRole_ExplicitBoth(t *testing.T) {
	cfg := &config.Config{
		Cluster: config.ClusterConfig{
			Name:          "test-cluster",
			PrimaryDomain: "example.com",
		},
		Hosts: []*host.Host{
			{
				Hostname: "node1.example.com",
				Roles:    []string{host.RoleClusterControlPlane},
			},
		},
	}

	role, err := determineNodeRole(cfg, "both")
	require.NoError(t, err)
	assert.True(t, role.IsControlPlane)
	assert.True(t, role.IsWorker)
}

func TestDetermineNodeRole_InvalidRole(t *testing.T) {
	cfg := &config.Config{
		Cluster: config.ClusterConfig{
			Name:          "test-cluster",
			PrimaryDomain: "example.com",
		},
		Hosts: []*host.Host{
			{
				Hostname: "node1.example.com",
				Roles:    []string{host.RoleClusterControlPlane},
			},
		},
	}

	_, err := determineNodeRole(cfg, "invalid")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid role")
}

func TestDetermineNodeRole_AutoLessThan3ControlPlanes(t *testing.T) {
	tests := []struct {
		name              string
		existingHosts     []*host.Host
		expectedCP        bool
		expectedWorker    bool
		controlPlaneCount int
	}{
		{
			name:              "0 control planes - add as both",
			existingHosts:     []*host.Host{},
			expectedCP:        true,
			expectedWorker:    true,
			controlPlaneCount: 0,
		},
		{
			name: "1 control plane - add as both",
			existingHosts: []*host.Host{
				{
					Hostname: "cp1.example.com",
					Roles:    []string{host.RoleClusterControlPlane},
				},
			},
			expectedCP:        true,
			expectedWorker:    true,
			controlPlaneCount: 1,
		},
		{
			name: "2 control planes - add as both",
			existingHosts: []*host.Host{
				{
					Hostname: "cp1.example.com",
					Roles:    []string{host.RoleClusterControlPlane},
				},
				{
					Hostname: "cp2.example.com",
					Roles:    []string{host.RoleClusterControlPlane},
				},
			},
			expectedCP:        true,
			expectedWorker:    true,
			controlPlaneCount: 2,
		},
		{
			name: "3 control planes - add as worker",
			existingHosts: []*host.Host{
				{
					Hostname: "cp1.example.com",
					Roles:    []string{host.RoleClusterControlPlane},
				},
				{
					Hostname: "cp2.example.com",
					Roles:    []string{host.RoleClusterControlPlane},
				},
				{
					Hostname: "cp3.example.com",
					Roles:    []string{host.RoleClusterControlPlane},
				},
			},
			expectedCP:        false,
			expectedWorker:    true,
			controlPlaneCount: 3,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &config.Config{
				Cluster: config.ClusterConfig{
					Name:          "test-cluster",
					PrimaryDomain: "example.com",
				},
				Hosts: tt.existingHosts,
			}

			role, err := determineNodeRole(cfg, "") // Auto
			require.NoError(t, err)
			assert.Equal(t, tt.expectedCP, role.IsControlPlane, "IsControlPlane mismatch")
			assert.Equal(t, tt.expectedWorker, role.IsWorker, "IsWorker mismatch")
		})
	}
}

func TestDetermineNodeRole_AutoCountsControlPlaneRole(t *testing.T) {
	// Hosts with cluster-control-plane role should count toward control plane count
	cfg := &config.Config{
		Cluster: config.ClusterConfig{
			Name:          "test-cluster",
			PrimaryDomain: "example.com",
		},
		Hosts: []*host.Host{
			{
				Hostname: "cp1.example.com",
				Roles:    []string{host.RoleClusterControlPlane},
			},
			{
				Hostname: "cp2.example.com",
				Roles:    []string{host.RoleClusterControlPlane},
			},
			{
				Hostname: "cp3.example.com",
				Roles:    []string{host.RoleClusterControlPlane},
			},
		},
	}

	role, err := determineNodeRole(cfg, "")
	require.NoError(t, err)
	// Should add as worker since we have 3 control planes
	assert.False(t, role.IsControlPlane)
	assert.True(t, role.IsWorker)
}

func TestPrintNodeAddPlan(t *testing.T) {
	tests := []struct {
		name     string
		role     *k3s.DeterminedRole
		labels   map[string]string
		expected []string
	}{
		{
			name:     "control-plane role",
			role:     &k3s.DeterminedRole{Role: k3s.RoleControlPlane, IsControlPlane: true, IsWorker: false},
			labels:   nil,
			expected: []string{"control-plane", "Join as control plane"},
		},
		{
			name:     "worker role",
			role:     &k3s.DeterminedRole{Role: k3s.RoleWorker, IsControlPlane: false, IsWorker: true},
			labels:   nil,
			expected: []string{"worker", "Join as worker"},
		},
		{
			name:     "both role",
			role:     &k3s.DeterminedRole{Role: k3s.RoleBoth, IsControlPlane: true, IsWorker: true},
			labels:   nil,
			expected: []string{"both (control-plane + worker)", "Join as control plane"},
		},
		{
			name:   "with labels",
			role:   &k3s.DeterminedRole{Role: k3s.RoleWorker, IsControlPlane: false, IsWorker: true},
			labels: map[string]string{"environment": "production"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &config.Config{
				Cluster: config.ClusterConfig{
					Name: "prod",
				},
				Network: &config.NetworkConfig{
				},
			}

			// Just verify the function doesn't panic
			// Output verification would require capturing stdout
			printNodeAddPlan("test.example.com", tt.role, cfg, tt.labels)
		})
	}
}

func TestParseNodeAddLabels(t *testing.T) {
	tests := []struct {
		name       string
		args       []string
		wantLabels map[string]string
		wantErr    bool
		errMsg     string
	}{
		{
			name:       "single label",
			args:       []string{"environment=production"},
			wantLabels: map[string]string{"environment": "production"},
			wantErr:    false,
		},
		{
			name:       "multiple labels",
			args:       []string{"environment=production", "zone=us-east-1a"},
			wantLabels: map[string]string{"environment": "production", "zone": "us-east-1a"},
			wantErr:    false,
		},
		{
			name:       "empty args",
			args:       []string{},
			wantLabels: map[string]string{},
			wantErr:    false,
		},
		{
			name:       "label with prefix",
			args:       []string{"app.example.com/tier=frontend"},
			wantLabels: map[string]string{"app.example.com/tier": "frontend"},
			wantErr:    false,
		},
		{
			name:    "invalid format - no equals",
			args:    []string{"invalid"},
			wantErr: true,
			errMsg:  "invalid label format",
		},
		{
			name:    "invalid key",
			args:    []string{"-invalid=value"},
			wantErr: true,
			errMsg:  "invalid label key",
		},
		{
			name:    "system label",
			args:    []string{"kubernetes.io/hostname=node1"},
			wantErr: true,
			errMsg:  "cannot set system label",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			labels, err := parseNodeAddLabels(tt.args)

			if tt.wantErr {
				require.Error(t, err)
				if tt.errMsg != "" {
					assert.Contains(t, err.Error(), tt.errMsg)
				}
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.wantLabels, labels)
			}
		})
	}
}
