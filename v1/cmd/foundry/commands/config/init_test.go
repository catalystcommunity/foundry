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

func TestInitCommand_NonInteractive(t *testing.T) {
	// Create temp directory for testing
	tempDir, err := os.MkdirTemp("", "foundry-test-*")
	require.NoError(t, err)
	defer os.RemoveAll(tempDir)

	// Override config dir
	oldHome := os.Getenv("HOME")
	os.Setenv("HOME", tempDir)
	defer os.Setenv("HOME", oldHome)

	// Create CLI app
	cmd := &cli.Command{
		Name: "foundry",
		Commands: []*cli.Command{
			{
				Name: "config",
				Commands: []*cli.Command{
					InitCommand,
				},
			},
		},
	}

	// Run init command
	err = cmd.Run(context.Background(), []string{"foundry", "config", "init", "--non-interactive"})
	require.NoError(t, err)

	// Verify file was created
	configPath := filepath.Join(tempDir, ".foundry", "stack.yaml")
	assert.FileExists(t, configPath)

	// Verify file contains expected template content (placeholders)
	// Note: Non-interactive mode creates a template with placeholders that
	// must be edited before the config will pass validation
	data, err := os.ReadFile(configPath)
	require.NoError(t, err)
	content := string(data)
	assert.Contains(t, content, "cluster:")
	assert.Contains(t, content, "network:")
	assert.Contains(t, content, "hosts:")
	assert.Contains(t, content, "components:")
}

func TestInitCommand_WithCustomName(t *testing.T) {
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
					InitCommand,
				},
			},
		},
	}

	err = cmd.Run(context.Background(), []string{"foundry", "config", "init", "--non-interactive", "--name", "test-cluster"})
	require.NoError(t, err)

	configPath := filepath.Join(tempDir, ".foundry", "test-cluster.yaml")
	assert.FileExists(t, configPath)

	// Verify file contains expected template content
	data, err := os.ReadFile(configPath)
	require.NoError(t, err)
	content := string(data)
	assert.Contains(t, content, "cluster:")
	assert.Contains(t, content, "components:")
}

func TestInitCommand_FileExists_NoForce(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "foundry-test-*")
	require.NoError(t, err)
	defer os.RemoveAll(tempDir)

	oldHome := os.Getenv("HOME")
	os.Setenv("HOME", tempDir)
	defer os.Setenv("HOME", oldHome)

	// Create config directory and file
	configDir := filepath.Join(tempDir, ".foundry")
	require.NoError(t, os.MkdirAll(configDir, 0755))
	configPath := filepath.Join(configDir, "stack.yaml")
	require.NoError(t, os.WriteFile(configPath, []byte("existing: content"), 0644))

	cmd := &cli.Command{
		Name: "foundry",
		Commands: []*cli.Command{
			{
				Name: "config",
				Commands: []*cli.Command{
					InitCommand,
				},
			},
		},
	}

	// Try to init without force - should fail
	err = cmd.Run(context.Background(), []string{"foundry", "config", "init", "--non-interactive"})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "already exists")

	// Verify original file wasn't modified
	data, err := os.ReadFile(configPath)
	require.NoError(t, err)
	assert.Equal(t, "existing: content", string(data))
}

func TestInitCommand_FileExists_WithForce(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "foundry-test-*")
	require.NoError(t, err)
	defer os.RemoveAll(tempDir)

	oldHome := os.Getenv("HOME")
	os.Setenv("HOME", tempDir)
	defer os.Setenv("HOME", oldHome)

	// Create config directory and file
	configDir := filepath.Join(tempDir, ".foundry")
	require.NoError(t, os.MkdirAll(configDir, 0755))
	configPath := filepath.Join(configDir, "stack.yaml")
	require.NoError(t, os.WriteFile(configPath, []byte("existing: content"), 0644))

	cmd := &cli.Command{
		Name: "foundry",
		Commands: []*cli.Command{
			{
				Name: "config",
				Commands: []*cli.Command{
					InitCommand,
				},
			},
		},
	}

	// Init with force - should succeed
	err = cmd.Run(context.Background(), []string{"foundry", "config", "init", "--non-interactive", "--force"})
	require.NoError(t, err)

	// Verify file was overwritten with new template (not old content)
	data, err := os.ReadFile(configPath)
	require.NoError(t, err)
	content := string(data)
	assert.NotContains(t, content, "existing: content")
	assert.Contains(t, content, "cluster:")
	assert.Contains(t, content, "components:")
}

func TestPrompt(t *testing.T) {
	tests := []struct {
		name         string
		input        string
		defaultValue string
		expected     string
	}{
		{
			name:         "uses default on empty input",
			input:        "\n",
			defaultValue: "default-value",
			expected:     "default-value",
		},
		{
			name:         "uses provided input",
			input:        "custom-value\n",
			defaultValue: "default-value",
			expected:     "custom-value",
		},
		{
			name:         "trims whitespace",
			input:        "  value-with-spaces  \n",
			defaultValue: "default",
			expected:     "value-with-spaces",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Redirect stdin
			oldStdin := os.Stdin
			r, w, _ := os.Pipe()
			os.Stdin = r

			// Write test input
			go func() {
				w.Write([]byte(tt.input))
				w.Close()
			}()

			result, err := prompt("test question", tt.defaultValue)
			require.NoError(t, err)
			assert.Equal(t, tt.expected, result)

			os.Stdin = oldStdin
		})
	}
}

func TestInitCommand_CreatesDirectory(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "foundry-test-*")
	require.NoError(t, err)
	defer os.RemoveAll(tempDir)

	oldHome := os.Getenv("HOME")
	os.Setenv("HOME", tempDir)
	defer os.Setenv("HOME", oldHome)

	// Ensure .foundry directory doesn't exist
	configDir := filepath.Join(tempDir, ".foundry")
	_, err = os.Stat(configDir)
	assert.True(t, os.IsNotExist(err))

	cmd := &cli.Command{
		Name: "foundry",
		Commands: []*cli.Command{
			{
				Name: "config",
				Commands: []*cli.Command{
					InitCommand,
				},
			},
		},
	}

	err = cmd.Run(context.Background(), []string{"foundry", "config", "init", "--non-interactive"})
	require.NoError(t, err)

	// Verify directory was created
	info, err := os.Stat(configDir)
	require.NoError(t, err)
	assert.True(t, info.IsDir())
}

func TestInitCommand_OutputMessage(t *testing.T) {
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
					InitCommand,
				},
			},
		},
	}

	err = cmd.Run(context.Background(), []string{"foundry", "config", "init", "--non-interactive"})
	require.NoError(t, err)

	w.Close()
	os.Stdout = oldStdout

	var buf bytes.Buffer
	buf.ReadFrom(r)
	output := buf.String()

	assert.Contains(t, output, "Created config file")
	assert.Contains(t, output, ".foundry/stack.yaml")
}
