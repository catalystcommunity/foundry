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
		{
			name:    "invalid config - invalid role",
			path:    filepath.Join(fixturesDir, "invalid-config-invalid-role.yaml"),
			wantErr: true,
			errMsg:  "config validation failed",
		},
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
  nodes:
    - hostname: node1
      role: control-plane

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
		{
			name: "valid yaml but invalid config",
			yaml: `
version: "1.0"
cluster:
  name: test
  domain: example.com
  nodes:
    - hostname: node1
      role: invalid-role

components:
  k3s: {}
`,
			wantErr: true,
			errMsg:  "config validation failed",
		},
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

func TestLoad_ValidConfigFile(t *testing.T) {
	// Test that we can load and validate the actual valid-config.yaml fixture
	fixturesDir, err := filepath.Abs("../../test/fixtures")
	require.NoError(t, err)

	config, err := Load(filepath.Join(fixturesDir, "valid-config.yaml"))
	require.NoError(t, err)
	require.NotNil(t, config)

	// Verify some key fields
	assert.Equal(t, "production", config.Cluster.Name)
	assert.Equal(t, "example.com", config.Cluster.Domain)
	assert.Len(t, config.Cluster.Nodes, 2)
	assert.Equal(t, "node1.example.com", config.Cluster.Nodes[0].Hostname)
	assert.Equal(t, NodeRoleControlPlane, config.Cluster.Nodes[0].Role)

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
	require.NotNil(t, config.Storage.TrueNAS)
	assert.Equal(t, "https://truenas.example.com", config.Storage.TrueNAS.APIURL)
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
