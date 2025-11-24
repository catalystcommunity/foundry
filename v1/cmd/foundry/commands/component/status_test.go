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

func TestStatusCommand(t *testing.T) {
	// Create a fresh registry for testing
	testRegistry := component.NewRegistry()

	// Temporarily replace the default registry
	oldRegistry := component.DefaultRegistry
	component.DefaultRegistry = testRegistry
	defer func() { component.DefaultRegistry = oldRegistry }()

	// Initialize component registry
	err := registry.InitComponents()
	require.NoError(t, err)

	tests := []struct {
		name          string
		args          []string
		expectError   bool
		expectOutput  []string
		errorContains string
	}{
		{
			name:          "no arguments",
			args:          []string{"test", "status"},
			expectError:   true,
			errorContains: "component name required",
		},
		{
			name:          "non-existent component",
			args:          []string{"test", "status", "nonexistent"},
			expectError:   true,
			errorContains: "not found",
		},
		{
			name:         "valid component - openbao",
			args:         []string{"test", "status", "openbao"},
			expectError:  false,
			expectOutput: []string{"Checking status", "openbao", "Installed:"},
		},
		{
			name:         "valid component - dns",
			args:         []string{"test", "status", "dns"},
			expectError:  false,
			expectOutput: []string{"Checking status", "dns", "Installed:"},
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
					StatusCommand,
				},
			}

			// Run the status command
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
