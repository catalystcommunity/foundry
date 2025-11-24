package config

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/urfave/cli/v3"
)

func TestValidateCommand_ValidConfig(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "foundry-test-*")
	require.NoError(t, err)
	defer os.RemoveAll(tempDir)

	// Create a valid config file
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
    hosts:
      - node1
`
	configPath := filepath.Join(tempDir, "valid.yaml")
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
					ValidateCommand,
				},
			},
		},
	}

	err = cmd.Run(context.Background(), []string{"foundry", "config", "validate", configPath})
	require.NoError(t, err)

	w.Close()
	os.Stdout = oldStdout

	var buf bytes.Buffer
	buf.ReadFrom(r)
	output := buf.String()

	assert.Contains(t, output, "Configuration is valid")
	assert.Contains(t, output, "Cluster: test-cluster")
	assert.Contains(t, output, "Cluster: test-cluster")
	assert.Contains(t, output, "Hosts:")
	assert.Contains(t, output, "Components: 1")
}

func TestValidateCommand_InvalidConfig(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "foundry-test-*")
	require.NoError(t, err)
	defer os.RemoveAll(tempDir)

	// Create an invalid config file (missing required fields)
	configContent := `version: v1
cluster:
  name: ""
  domain: ""
`
	configPath := filepath.Join(tempDir, "invalid.yaml")
	require.NoError(t, os.WriteFile(configPath, []byte(configContent), 0644))

	cmd := &cli.Command{
		Name: "foundry",
		Commands: []*cli.Command{
			{
				Name: "config",
				Commands: []*cli.Command{
					ValidateCommand,
				},
			},
		},
	}

	err = cmd.Run(context.Background(), []string{"foundry", "config", "validate", configPath})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "validation failed")
}

func TestValidateCommand_MalformedYAML(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "foundry-test-*")
	require.NoError(t, err)
	defer os.RemoveAll(tempDir)

	// Create a malformed YAML file
	configContent := `version: v1
cluster:
  name: test
  invalid yaml here: [[[
`
	configPath := filepath.Join(tempDir, "malformed.yaml")
	require.NoError(t, os.WriteFile(configPath, []byte(configContent), 0644))

	cmd := &cli.Command{
		Name: "foundry",
		Commands: []*cli.Command{
			{
				Name: "config",
				Commands: []*cli.Command{
					ValidateCommand,
				},
			},
		},
	}

	err = cmd.Run(context.Background(), []string{"foundry", "config", "validate", configPath})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "validation failed")
}

func TestValidateCommand_FileNotFound(t *testing.T) {
	cmd := &cli.Command{
		Name: "foundry",
		Commands: []*cli.Command{
			{
				Name: "config",
				Commands: []*cli.Command{
					ValidateCommand,
				},
			},
		},
	}

	err := cmd.Run(context.Background(), []string{"foundry", "config", "validate", "/nonexistent/config.yaml"})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "validation failed")
}

func TestValidateCommand_WithSecretRefs(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "foundry-test-*")
	require.NoError(t, err)
	defer os.RemoveAll(tempDir)

	// Create a config file with valid secret references
	configContent := `version: v1
cluster:
  name: test-cluster
  domain: test.local
  nodes:
    - hostname: node1
      role: control-plane
components:
  openbao:
    version: latest
    config:
      token: ${secret:foundry-core/openbao:root_token}
  database:
    version: latest
    config:
      password: ${secret:database/prod:password}
`
	configPath := filepath.Join(tempDir, "with-secrets.yaml")
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
					ValidateCommand,
				},
			},
		},
	}

	err = cmd.Run(context.Background(), []string{"foundry", "config", "validate", configPath})
	require.NoError(t, err)

	w.Close()
	os.Stdout = oldStdout

	var buf bytes.Buffer
	buf.ReadFrom(r)
	output := buf.String()

	assert.Contains(t, output, "Configuration is valid")
}

func TestValidateCommand_WithInvalidSecretRefs(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "foundry-test-*")
	require.NoError(t, err)
	defer os.RemoveAll(tempDir)

	// Create a config file with invalid secret references
	configContent := `version: v1
cluster:
  name: test-cluster
  domain: test.local
  nodes:
    - hostname: node1
      role: control-plane
components:
  database:
    version: latest
    config:
      password: ${secret:invalid}
`
	configPath := filepath.Join(tempDir, "invalid-secrets.yaml")
	require.NoError(t, os.WriteFile(configPath, []byte(configContent), 0644))

	cmd := &cli.Command{
		Name: "foundry",
		Commands: []*cli.Command{
			{
				Name: "config",
				Commands: []*cli.Command{
					ValidateCommand,
				},
			},
		},
	}

	err = cmd.Run(context.Background(), []string{"foundry", "config", "validate", configPath})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "secret reference validation failed")
}

func TestValidateCommand_DefaultConfig(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "foundry-test-*")
	require.NoError(t, err)
	defer os.RemoveAll(tempDir)

	oldHome := os.Getenv("HOME")
	os.Setenv("HOME", tempDir)
	defer os.Setenv("HOME", oldHome)

	// Create default config
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
					ValidateCommand,
				},
			},
		},
	}

	// Run without specifying config path - should find default
	err = cmd.Run(context.Background(), []string{"foundry", "config", "validate"})
	require.NoError(t, err)

	w.Close()
	os.Stdout = oldStdout

	var buf bytes.Buffer
	buf.ReadFrom(r)
	output := buf.String()

	assert.Contains(t, output, "Configuration is valid")
}

func TestValidateCommand_NoConfigFound(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "foundry-test-*")
	require.NoError(t, err)
	defer os.RemoveAll(tempDir)

	oldHome := os.Getenv("HOME")
	os.Setenv("HOME", tempDir)
	defer os.Setenv("HOME", oldHome)

	cmd := &cli.Command{
		Name: "foundry",
		Commands: []*cli.Command{
			{
				Name: "config",
				Commands: []*cli.Command{
					ValidateCommand,
				},
			},
		},
	}

	err = cmd.Run(context.Background(), []string{"foundry", "config", "validate"})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "no config file specified")
}
