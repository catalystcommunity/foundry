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

func TestShowCommand_BasicConfig(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "foundry-test-*")
	require.NoError(t, err)
	defer os.RemoveAll(tempDir)

	configContent := `
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
	configPath := filepath.Join(tempDir, "test.yaml")
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
					ShowCommand,
				},
			},
		},
	}

	err = cmd.Run(context.Background(), []string{"foundry", "config", "show", configPath})
	require.NoError(t, err)

	w.Close()
	os.Stdout = oldStdout

	var buf bytes.Buffer
	buf.ReadFrom(r)
	output := buf.String()

	assert.Contains(t, output, "Configuration:")
	assert.Contains(t, output, "name: test-cluster")
	assert.Contains(t, output, "domain: test.local")
}

func TestShowCommand_WithSecretsRedacted(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "foundry-test-*")
	require.NoError(t, err)
	defer os.RemoveAll(tempDir)

	configContent := `
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
      password: ${secret:database/prod:password}
      api_key: ${secret:external/api:key}
`
	configPath := filepath.Join(tempDir, "test.yaml")
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
					ShowCommand,
				},
			},
		},
	}

	err = cmd.Run(context.Background(), []string{"foundry", "config", "show", configPath})
	require.NoError(t, err)

	w.Close()
	os.Stdout = oldStdout

	var buf bytes.Buffer
	buf.ReadFrom(r)
	output := buf.String()

	// Secrets should be redacted
	assert.Contains(t, output, "[SECRET]")
	assert.NotContains(t, output, "${secret:database/prod:password}")
	assert.NotContains(t, output, "${secret:external/api:key}")

	// Regular content should be visible
	assert.Contains(t, output, "name: test-cluster")
}

func TestShowCommand_WithShowSecretRefs(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "foundry-test-*")
	require.NoError(t, err)
	defer os.RemoveAll(tempDir)

	configContent := `
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
      password: ${secret:database/prod:password}
      api_key: ${secret:external/api:key}
`
	configPath := filepath.Join(tempDir, "test.yaml")
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
					ShowCommand,
				},
			},
		},
	}

	err = cmd.Run(context.Background(), []string{"foundry", "config", "show", "--show-secret-refs", configPath})
	require.NoError(t, err)

	w.Close()
	os.Stdout = oldStdout

	var buf bytes.Buffer
	buf.ReadFrom(r)
	output := buf.String()

	// Secret references should be visible
	assert.Contains(t, output, "${secret:database/prod:password}")
	assert.Contains(t, output, "${secret:external/api:key}")
	assert.NotContains(t, output, "[SECRET]")
}

func TestShowCommand_DefaultConfig(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "foundry-test-*")
	require.NoError(t, err)
	defer os.RemoveAll(tempDir)

	oldHome := os.Getenv("HOME")
	os.Setenv("HOME", tempDir)
	defer os.Setenv("HOME", oldHome)

	// Create default config
	configDir := filepath.Join(tempDir, ".foundry")
	require.NoError(t, os.MkdirAll(configDir, 0755))

	configContent := `
cluster:
  name: default-cluster
  domain: default.local
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
					ShowCommand,
				},
			},
		},
	}

	err = cmd.Run(context.Background(), []string{"foundry", "config", "show"})
	require.NoError(t, err)

	w.Close()
	os.Stdout = oldStdout

	var buf bytes.Buffer
	buf.ReadFrom(r)
	output := buf.String()

	assert.Contains(t, output, "default-cluster")
	assert.Contains(t, output, "default.local")
}

func TestShowCommand_FileNotFound(t *testing.T) {
	cmd := &cli.Command{
		Name: "foundry",
		Commands: []*cli.Command{
			{
				Name: "config",
				Commands: []*cli.Command{
					ShowCommand,
				},
			},
		},
	}

	err := cmd.Run(context.Background(), []string{"foundry", "config", "show", "/nonexistent/config.yaml"})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to load config")
}

func TestShowCommand_NoConfigFound(t *testing.T) {
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
					ShowCommand,
				},
			},
		},
	}

	err = cmd.Run(context.Background(), []string{"foundry", "config", "show"})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "no config file specified")
}

func TestShowCommand_MultipleSecrets(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "foundry-test-*")
	require.NoError(t, err)
	defer os.RemoveAll(tempDir)

	configContent := `
cluster:
  name: test-cluster
  domain: test.local
  nodes:
    - hostname: node1
      role: control-plane
components:
  app1:
    config:
      password: ${secret:app1/db:password}
      token: ${secret:app1/api:token}
  app2:
    config:
      key: ${secret:app2/service:key}
`
	configPath := filepath.Join(tempDir, "test.yaml")
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
					ShowCommand,
				},
			},
		},
	}

	err = cmd.Run(context.Background(), []string{"foundry", "config", "show", configPath})
	require.NoError(t, err)

	w.Close()
	os.Stdout = oldStdout

	var buf bytes.Buffer
	buf.ReadFrom(r)
	output := buf.String()

	// All secrets should be redacted
	assert.NotContains(t, output, "${secret:")
	// Count [SECRET] occurrences - should be 3
	secretCount := 0
	for i := 0; i < len(output)-8; i++ {
		if output[i:i+8] == "[SECRET]" {
			secretCount++
		}
	}
	assert.Equal(t, 3, secretCount)
}
