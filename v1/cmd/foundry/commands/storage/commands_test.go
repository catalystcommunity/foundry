package storage

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestStorageCommand(t *testing.T) {
	assert.NotNil(t, Command, "Command should not be nil")
	assert.Equal(t, "storage", Command.Name)
	assert.Len(t, Command.Commands, 4, "Should have 4 subcommands")

	// Verify subcommands exist
	var foundList, foundProvision, foundPVC, foundAddDisk bool
	for _, cmd := range Command.Commands {
		switch cmd.Name {
		case "list":
			foundList = true
		case "provision":
			foundProvision = true
		case "pvc":
			foundPVC = true
		case "add-disk":
			foundAddDisk = true
		}
	}

	assert.True(t, foundList, "Should have list command")
	assert.True(t, foundProvision, "Should have provision command")
	assert.True(t, foundPVC, "Should have pvc command")
	assert.True(t, foundAddDisk, "Should have add-disk command")
}

func TestListCommand(t *testing.T) {
	assert.NotNil(t, ListCommand, "ListCommand should not be nil")
	assert.Equal(t, "list", ListCommand.Name)
	assert.NotNil(t, ListCommand.Action, "ListCommand should have an action")
	// --config is now inherited from root command, not defined on subcommand
}
