package component

import (
	"bytes"
	"context"
	"io"
	"os"
	"testing"

	"github.com/catalystcommunity/foundry/v1/cmd/foundry/registry"
	"github.com/catalystcommunity/foundry/v1/internal/component"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/urfave/cli/v3"
)

func TestListCommand(t *testing.T) {
	// Create a fresh registry for testing
	testRegistry := component.NewRegistry()

	// Temporarily replace the default registry
	oldRegistry := component.DefaultRegistry
	component.DefaultRegistry = testRegistry
	defer func() { component.DefaultRegistry = oldRegistry }()

	// Initialize component registry
	err := registry.InitComponents()
	require.NoError(t, err)

	// Capture stdout
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	// Create CLI command
	cmd := &cli.Command{
		Commands: []*cli.Command{
			ListCommand,
		},
	}

	// Run the list command
	err = cmd.Run(context.Background(), []string{"test", "list"})
	require.NoError(t, err)

	// Restore stdout and read captured output
	w.Close()
	os.Stdout = oldStdout
	var buf bytes.Buffer
	io.Copy(&buf, r)
	output := buf.String()

	// Verify output contains expected components
	assert.Contains(t, output, "Available Components:")
	assert.Contains(t, output, "openbao")
	assert.Contains(t, output, "dns")
	assert.Contains(t, output, "zot")
	assert.Contains(t, output, "k3s")

	// Verify dependency information
	assert.Contains(t, output, "Dependencies: none") // OpenBAO has no deps
	assert.Contains(t, output, "openbao, dns, zot")  // K3s dependencies
}

func TestListCommand_EmptyRegistry(t *testing.T) {
	// Create a fresh empty registry for testing
	testRegistry := component.NewRegistry()

	// Temporarily replace the default registry
	oldRegistry := component.DefaultRegistry
	component.DefaultRegistry = testRegistry
	defer func() { component.DefaultRegistry = oldRegistry }()

	// Capture stdout
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	// Create CLI command
	cmd := &cli.Command{
		Commands: []*cli.Command{
			ListCommand,
		},
	}

	// Run the list command
	err := cmd.Run(context.Background(), []string{"test", "list"})
	require.NoError(t, err)

	// Restore stdout and read captured output
	w.Close()
	os.Stdout = oldStdout
	var buf bytes.Buffer
	io.Copy(&buf, r)
	output := buf.String()

	// Verify output indicates no components
	assert.Contains(t, output, "No components registered")
}
