package config

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/urfave/cli/v3"
)

func TestListCommand_EmptyDirectory(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "foundry-test-*")
	require.NoError(t, err)
	defer os.RemoveAll(tempDir)

	oldHome := os.Getenv("HOME")
	os.Setenv("HOME", tempDir)
	defer os.Setenv("HOME", oldHome)

	// Capture stdout
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	cmd := &cli.Command{
		Name: "foundry",
		Commands: []*cli.Command{
			{
				Name: "config",
				Commands: []*cli.Command{
					ListCommand,
				},
			},
		},
	}

	err = cmd.Run(context.Background(), []string{"foundry", "config", "list"})
	require.NoError(t, err)

	w.Close()
	os.Stdout = oldStdout

	var buf bytes.Buffer
	buf.ReadFrom(r)
	output := buf.String()

	assert.Contains(t, output, "No configuration directory found")
	assert.Contains(t, output, "foundry config init")
}

func TestListCommand_NoConfigs(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "foundry-test-*")
	require.NoError(t, err)
	defer os.RemoveAll(tempDir)

	oldHome := os.Getenv("HOME")
	os.Setenv("HOME", tempDir)
	defer os.Setenv("HOME", oldHome)

	// Create config directory but no files
	configDir := filepath.Join(tempDir, ".foundry")
	require.NoError(t, os.MkdirAll(configDir, 0755))

	// Capture stdout
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	cmd := &cli.Command{
		Name: "foundry",
		Commands: []*cli.Command{
			{
				Name: "config",
				Commands: []*cli.Command{
					ListCommand,
				},
			},
		},
	}

	err = cmd.Run(context.Background(), []string{"foundry", "config", "list"})
	require.NoError(t, err)

	w.Close()
	os.Stdout = oldStdout

	var buf bytes.Buffer
	buf.ReadFrom(r)
	output := buf.String()

	assert.Contains(t, output, "No configuration files found")
	assert.Contains(t, output, "foundry config init")
}

func TestListCommand_SingleConfig(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "foundry-test-*")
	require.NoError(t, err)
	defer os.RemoveAll(tempDir)

	oldHome := os.Getenv("HOME")
	os.Setenv("HOME", tempDir)
	defer os.Setenv("HOME", oldHome)

	// Create config directory and file
	configDir := filepath.Join(tempDir, ".foundry")
	require.NoError(t, os.MkdirAll(configDir, 0755))

	configContent := `version: v1
cluster:
  name: test-cluster
  domain: test.local
  nodes:
    - hostname: node1
      role: control-plane
components:
  k3s:
    version: latest
`
	configPath := filepath.Join(configDir, "stack.yaml")
	require.NoError(t, os.WriteFile(configPath, []byte(configContent), 0644))

	// Capture stdout
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	cmd := &cli.Command{
		Name: "foundry",
		Commands: []*cli.Command{
			{
				Name: "config",
				Commands: []*cli.Command{
					ListCommand,
				},
			},
		},
	}

	err = cmd.Run(context.Background(), []string{"foundry", "config", "list"})
	require.NoError(t, err)

	w.Close()
	os.Stdout = oldStdout

	var buf bytes.Buffer
	buf.ReadFrom(r)
	output := buf.String()

	assert.Contains(t, output, "stack.yaml")
	assert.Contains(t, output, "test-cluster")
	assert.Contains(t, output, "test.local")
	assert.Contains(t, output, "Hosts:")
	assert.Contains(t, output, "Components: 1")
	assert.Contains(t, output, "* = default config")
	assert.Contains(t, output, "* stack.yaml") // Should be marked as default
}

func TestListCommand_MultipleConfigs(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "foundry-test-*")
	require.NoError(t, err)
	defer os.RemoveAll(tempDir)

	oldHome := os.Getenv("HOME")
	os.Setenv("HOME", tempDir)
	defer os.Setenv("HOME", oldHome)

	// Create config directory
	configDir := filepath.Join(tempDir, ".foundry")
	require.NoError(t, os.MkdirAll(configDir, 0755))

	// Create multiple config files
	configs := []struct {
		name        string
		clusterName string
		domain      string
		nodes       int
	}{
		{"stack.yaml", "main-cluster", "main.local", 1},
		{"dev.yaml", "dev-cluster", "dev.local", 2},
		{"prod.yaml", "prod-cluster", "prod.local", 3},
	}

	for _, cfg := range configs {
		content := fmt.Sprintf(`version: v1
cluster:
  name: %s
  domain: %s
  nodes:
`, cfg.clusterName, cfg.domain)

		for i := 0; i < cfg.nodes; i++ {
			content += fmt.Sprintf(`    - hostname: node%d
      role: control-plane
`, i+1)
		}

		content += `components:
  k3s:
    version: latest
`
		configPath := filepath.Join(configDir, cfg.name)
		require.NoError(t, os.WriteFile(configPath, []byte(content), 0644))
	}

	// Capture stdout
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	cmd := &cli.Command{
		Name: "foundry",
		Commands: []*cli.Command{
			{
				Name: "config",
				Commands: []*cli.Command{
					ListCommand,
				},
			},
		},
	}

	err = cmd.Run(context.Background(), []string{"foundry", "config", "list"})
	require.NoError(t, err)

	w.Close()
	os.Stdout = oldStdout

	var buf bytes.Buffer
	buf.ReadFrom(r)
	output := buf.String()

	// All configs should be listed
	assert.Contains(t, output, "stack.yaml")
	assert.Contains(t, output, "dev.yaml")
	assert.Contains(t, output, "prod.yaml")

	// All cluster names should appear
	assert.Contains(t, output, "main-cluster")
	assert.Contains(t, output, "dev-cluster")
	assert.Contains(t, output, "prod-cluster")

	// stack.yaml should be marked as default
	assert.Contains(t, output, "* stack.yaml")
}

func TestListCommand_InvalidConfig(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "foundry-test-*")
	require.NoError(t, err)
	defer os.RemoveAll(tempDir)

	oldHome := os.Getenv("HOME")
	os.Setenv("HOME", tempDir)
	defer os.Setenv("HOME", oldHome)

	// Create config directory
	configDir := filepath.Join(tempDir, ".foundry")
	require.NoError(t, os.MkdirAll(configDir, 0755))

	// Create valid config
	validContent := `version: v1
cluster:
  name: valid-cluster
  domain: valid.local
  nodes:
    - hostname: node1
      role: control-plane
components:
  k3s:
    version: latest
`
	require.NoError(t, os.WriteFile(filepath.Join(configDir, "valid.yaml"), []byte(validContent), 0644))

	// Create invalid config
	invalidContent := `invalid yaml content: [[[[`
	require.NoError(t, os.WriteFile(filepath.Join(configDir, "invalid.yaml"), []byte(invalidContent), 0644))

	// Capture stdout
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	cmd := &cli.Command{
		Name: "foundry",
		Commands: []*cli.Command{
			{
				Name: "config",
				Commands: []*cli.Command{
					ListCommand,
				},
			},
		},
	}

	err = cmd.Run(context.Background(), []string{"foundry", "config", "list"})
	require.NoError(t, err)

	w.Close()
	os.Stdout = oldStdout

	var buf bytes.Buffer
	buf.ReadFrom(r)
	output := buf.String()

	// Valid config should be listed normally
	assert.Contains(t, output, "valid.yaml")
	assert.Contains(t, output, "valid-cluster")

	// Invalid config should be listed with error
	assert.Contains(t, output, "invalid.yaml")
	assert.Contains(t, output, "invalid:")
}

func TestListCommand_AliasWorks(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "foundry-test-*")
	require.NoError(t, err)
	defer os.RemoveAll(tempDir)

	oldHome := os.Getenv("HOME")
	os.Setenv("HOME", tempDir)
	defer os.Setenv("HOME", oldHome)

	// Capture stdout
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	cmd := &cli.Command{
		Name: "foundry",
		Commands: []*cli.Command{
			{
				Name: "config",
				Commands: []*cli.Command{
					ListCommand,
				},
			},
		},
	}

	// Use "ls" alias instead of "list"
	err = cmd.Run(context.Background(), []string{"foundry", "config", "ls"})
	require.NoError(t, err)

	w.Close()
	os.Stdout = oldStdout

	var buf bytes.Buffer
	buf.ReadFrom(r)
	output := buf.String()

	// Should work the same as "list"
	assert.Contains(t, output, "configuration")
}
