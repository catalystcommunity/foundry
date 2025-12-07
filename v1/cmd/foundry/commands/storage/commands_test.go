package storage

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestStorageCommand(t *testing.T) {
	assert.NotNil(t, Command, "Command should not be nil")
	assert.Equal(t, "storage", Command.Name)
	assert.Len(t, Command.Commands, 3, "Should have 3 subcommands")

	// Verify subcommands exist
	var foundList, foundProvision, foundPVC bool
	for _, cmd := range Command.Commands {
		switch cmd.Name {
		case "list":
			foundList = true
		case "provision":
			foundProvision = true
		case "pvc":
			foundPVC = true
		}
	}

	assert.True(t, foundList, "Should have list command")
	assert.True(t, foundProvision, "Should have provision command")
	assert.True(t, foundPVC, "Should have pvc command")
}

func TestListCommand(t *testing.T) {
	assert.NotNil(t, ListCommand, "ListCommand should not be nil")
	assert.Equal(t, "list", ListCommand.Name)
	assert.NotNil(t, ListCommand.Action, "ListCommand should have an action")

	// Check flags
	var foundConfig bool
	for _, flag := range ListCommand.Flags {
		switch flag.Names()[0] {
		case "config":
			foundConfig = true
		}
	}

	assert.True(t, foundConfig, "Should have --config flag")
}
