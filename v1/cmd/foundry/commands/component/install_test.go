package component

import (
	"bytes"
	"context"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/catalystcommunity/foundry/v1/cmd/foundry/registry"
	"github.com/catalystcommunity/foundry/v1/internal/component"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/urfave/cli/v3"
)

func TestInstallCommand_DryRun(t *testing.T) {
	// Create a fresh registry for testing
	testRegistry := component.NewRegistry()

	// Temporarily replace the default registry
	oldRegistry := component.DefaultRegistry
	component.DefaultRegistry = testRegistry
	defer func() { component.DefaultRegistry = oldRegistry }()

	// Initialize component registry
	err := registry.InitComponents()
	require.NoError(t, err)

	// Initialize host registry (needed for install command)
	err = registry.InitHostRegistry()
	require.NoError(t, err)

	tests := []struct {
		name         string
		args         []string
		expectError  bool
		expectOutput []string
		errorContains string
	}{
		{
			name:          "no arguments",
			args:          []string{"test", "install"},
			expectError:   true,
			errorContains: "component name required",
		},
		{
			name:          "non-existent component",
			args:          []string{"test", "install", "nonexistent"},
			expectError:   true,
			errorContains: "not found",
		},
		{
			name:          "dry-run openbao (requires config)",
			args:          []string{"test", "install", "openbao", "--dry-run"},
			expectError:   true,  // Will fail without stack config
			errorContains: "failed to load stack config",
		},
		{
			name:          "dry-run with version (requires config)",
			args:          []string{"test", "install", "openbao", "--dry-run", "--version", "2.0.0"},
			expectError:   true,  // Will fail without stack config
			errorContains: "failed to load stack config",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Capture stdout
			oldStdout := os.Stdout
			r, w, _ := os.Pipe()
			os.Stdout = w

			// Create CLI command
			cmd := &cli.Command{
				Commands: []*cli.Command{
					InstallCommand,
				},
			}

			// Run the install command
			err := cmd.Run(context.Background(), tt.args)

			// Restore stdout and read captured output
			w.Close()
			os.Stdout = oldStdout
			var buf bytes.Buffer
			io.Copy(&buf, r)
			output := buf.String()

			// Check error expectation
			if tt.expectError {
				assert.Error(t, err)
				if tt.errorContains != "" {
					assert.Contains(t, err.Error(), tt.errorContains)
				}
			} else {
				assert.NoError(t, err)
			}

			// Check output contains expected strings
			for _, expected := range tt.expectOutput {
				assert.Contains(t, output, expected)
			}
		})
	}
}

func TestInstallCommand_DependencyCheck(t *testing.T) {
	// Create a temporary directory for the test config
	tmpDir := t.TempDir()

	// Set HOME environment variable to use temp directory
	oldHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", oldHome)

	// Create .foundry directory
	foundryDir := filepath.Join(tmpDir, ".foundry")
	err := os.MkdirAll(foundryDir, 0755)
	require.NoError(t, err)

	// Create a minimal stack config
	configPath := filepath.Join(foundryDir, "stack.yaml")
	configContent := `cluster:
  name: test-cluster
  domain: test.local
  vip: 192.168.1.100
network:
  gateway: 192.168.1.1
  netmask: 255.255.255.0
hosts:
  - hostname: test-host
    address: 192.168.1.10
    port: 22
    user: root
    roles:
      - openbao
      - dns
      - zot
dns:
  backend: gsqlite3
  api_key: test-api-key
  infrastructure_zones:
    - name: infra.local
  kubernetes_zones:
    - name: k8s.local
components:
  openbao: {}
  dns: {}
  zot: {}
`
	err = os.WriteFile(configPath, []byte(configContent), 0644)
	require.NoError(t, err)

	// Create a fresh registry for testing
	testRegistry := component.NewRegistry()

	// Temporarily replace the default registry
	oldRegistry := component.DefaultRegistry
	component.DefaultRegistry = testRegistry
	defer func() { component.DefaultRegistry = oldRegistry }()

	// Initialize component registry
	err = registry.InitComponents()
	require.NoError(t, err)

	// Capture stdout
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	// Create CLI command
	cmd := &cli.Command{
		Commands: []*cli.Command{
			InstallCommand,
		},
	}

	// Try to install k3s with --dry-run (should show dependencies)
	err = cmd.Run(context.Background(), []string{"test", "install", "k3s", "--dry-run"})

	// Restore stdout and read captured output
	w.Close()
	os.Stdout = oldStdout
	var buf bytes.Buffer
	io.Copy(&buf, r)
	output := buf.String()

	// K3s depends on openbao, dns, and zot
	// Since they're not installed, the command should fail
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not installed")
	// Check that dependencies are shown in output
	assert.Contains(t, output, "depends on")
}
