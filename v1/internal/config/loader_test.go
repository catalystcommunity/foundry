package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoad(t *testing.T) {
	// Get the absolute path to test fixtures
	fixturesDir, err := filepath.Abs("../../test/fixtures")
	require.NoError(t, err)

	tests := []struct {
		name    string
		path    string
		wantErr bool
		errMsg  string
	}{
		{
			name:    "valid config file",
			path:    filepath.Join(fixturesDir, "valid-config.yaml"),
			wantErr: false,
		},
		{
			name:    "non-existent file",
			path:    filepath.Join(fixturesDir, "does-not-exist.yaml"),
			wantErr: true,
			errMsg:  "config file not found",
		},
		// NOTE: "invalid role" test removed - NodeConfig validation no longer exists
		{
			name:    "invalid config - no components",
			path:    filepath.Join(fixturesDir, "invalid-config-no-components.yaml"),
			wantErr: true,
			errMsg:  "config validation failed",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config, err := Load(tt.path)
			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errMsg)
				assert.Nil(t, config)
			} else {
				require.NoError(t, err)
				require.NotNil(t, config)
				assert.NotEmpty(t, config.Cluster.Name)
			}
		})
	}
}

func TestLoadFromReader(t *testing.T) {
	tests := []struct {
		name    string
		yaml    string
		wantErr bool
		errMsg  string
	}{
		{
			name: "valid yaml",
			yaml: `
version: "1.0"
cluster:
  name: test
  domain: example.com

components:
  k3s:
    version: "v1.28.5+k3s1"
`,
			wantErr: false,
		},
		{
			name: "invalid yaml syntax",
			yaml: `
version: "1.0"
cluster:
  name: test
  invalid yaml here [[[
`,
			wantErr: true,
			errMsg:  "failed to parse YAML",
		},
		// NOTE: "invalid role" test removed - NodeConfig validation no longer exists
		{
			name:    "empty yaml",
			yaml:    "",
			wantErr: true,
			errMsg:  "config validation failed",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reader := strings.NewReader(tt.yaml)
			config, err := LoadFromReader(reader)
			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errMsg)
				assert.Nil(t, config)
			} else {
				require.NoError(t, err)
				require.NotNil(t, config)
			}
		})
	}
}

func TestLoadAppliesLegacyHostAndManagementDefaults(t *testing.T) {
	cfg, err := LoadFromReader(strings.NewReader(`
cluster:
  name: test
  primary_domain: example.test
hosts:
  - hostname: manager
    address: 192.0.2.11
    roles: [management]
management:
  host: manager
components:
  k3s: {}
`))
	require.NoError(t, err)
	require.Len(t, cfg.Hosts, 1)
	assert.Equal(t, 22, cfg.Hosts[0].Port)
	assert.Equal(t, "root", cfg.Hosts[0].User)
	require.NotNil(t, cfg.Management)
	assert.EqualValues(t, 9080, cfg.Management.Port)
	assert.Equal(t, "ghcr.io/catalystcommunity/foundry", cfg.Management.Image)
	assert.Equal(t, "latest", cfg.Management.Version)
	assert.Equal(t, "/var/lib/foundry", cfg.Management.DataPath)
}

func TestLegacyConfigRoundTripFromProtectedCopies(t *testing.T) {
	paths := strings.Fields(os.Getenv("FOUNDRY_COMPAT_CONFIGS"))
	if len(paths) == 0 {
		t.Skip("FOUNDRY_COMPAT_CONFIGS is not set")
	}
	for _, source := range paths {
		t.Run(filepath.Base(source), func(t *testing.T) {
			cfg, err := Load(source)
			require.NoError(t, err)
			assert.Nil(t, cfg.Management, "legacy configuration must not enable the optional manager")
			output := filepath.Join(t.TempDir(), filepath.Base(source))
			require.NoError(t, Save(cfg, output))
			reloaded, err := Load(output)
			require.NoError(t, err)
			assert.Equal(t, cfg.Cluster.Name, reloaded.Cluster.Name)
			assert.Len(t, reloaded.Hosts, len(cfg.Hosts))
			assert.Len(t, reloaded.Components, len(cfg.Components))
			assert.Nil(t, reloaded.Management)
		})
	}
}

func TestLoad_ValidConfigFile(t *testing.T) {
	// Test that we can load and validate the actual valid-config.yaml fixture
	fixturesDir, err := filepath.Abs("../../test/fixtures")
	require.NoError(t, err)

	config, err := Load(filepath.Join(fixturesDir, "valid-config.yaml"))
	require.NoError(t, err)
	require.NotNil(t, config)

	// Verify some key fields
	assert.Equal(t, "production", config.Cluster.Name)
	assert.Equal(t, "example.com", config.Cluster.PrimaryDomain)

	// Verify hosts (nodes are now in hosts array with cluster-* roles)
	clusterHosts := config.GetClusterControlPlaneHosts()
	assert.GreaterOrEqual(t, len(clusterHosts), 1, "should have at least one control plane host")

	// Verify components
	assert.Contains(t, config.Components, "openbao")
	assert.Contains(t, config.Components, "k3s")
	assert.Contains(t, config.Components, "zot")

	// Verify observability
	require.NotNil(t, config.Observability)
	require.NotNil(t, config.Observability.Prometheus)
	require.NotNil(t, config.Observability.Prometheus.Retention)
	assert.Equal(t, "30d", *config.Observability.Prometheus.Retention)

	// Verify storage
	require.NotNil(t, config.Storage)
	assert.Equal(t, "longhorn", config.Storage.Backend)
}

func TestLoad_FilePermissions(t *testing.T) {
	// Create a temporary file with restricted permissions
	tmpFile, err := os.CreateTemp("", "test-config-*.yaml")
	require.NoError(t, err)
	tmpPath := tmpFile.Name()
	defer os.Remove(tmpPath)

	// Write valid config
	validYAML := `
version: "1.0"
cluster:
  name: test
  domain: example.com
  nodes:
    - hostname: node1
      role: control-plane
components:
  k3s: {}
`
	_, err = tmpFile.WriteString(validYAML)
	require.NoError(t, err)
	tmpFile.Close()

	// Make file unreadable (Unix-like systems only)
	if err := os.Chmod(tmpPath, 0000); err == nil {
		defer os.Chmod(tmpPath, 0644) // Restore permissions for cleanup

		// Try to load - should fail with permission error
		_, err = Load(tmpPath)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to open config file")
	}
}
