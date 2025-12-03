package storage

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestStorageCommand(t *testing.T) {
	assert.NotNil(t, Command, "Command should not be nil")
	assert.Equal(t, "storage", Command.Name)
	assert.Len(t, Command.Commands, 5, "Should have 5 subcommands")

	// Verify subcommands exist
	var foundConfigure, foundList, foundTest, foundProvision, foundPVC bool
	for _, cmd := range Command.Commands {
		switch cmd.Name {
		case "configure":
			foundConfigure = true
		case "list":
			foundList = true
		case "test":
			foundTest = true
		case "provision":
			foundProvision = true
		case "pvc":
			foundPVC = true
		}
	}

	assert.True(t, foundConfigure, "Should have configure command")
	assert.True(t, foundList, "Should have list command")
	assert.True(t, foundTest, "Should have test command")
	assert.True(t, foundProvision, "Should have provision command")
	assert.True(t, foundPVC, "Should have pvc command")
}

func TestConfigureCommand(t *testing.T) {
	assert.NotNil(t, ConfigureCommand, "ConfigureCommand should not be nil")
	assert.Equal(t, "configure", ConfigureCommand.Name)
	assert.NotNil(t, ConfigureCommand.Action, "ConfigureCommand should have an action")

	// Check flags
	var foundConfig, foundAPIURL, foundAPIKey, foundSkipTest bool
	for _, flag := range ConfigureCommand.Flags {
		switch flag.Names()[0] {
		case "config":
			foundConfig = true
		case "api-url":
			foundAPIURL = true
		case "api-key":
			foundAPIKey = true
		case "skip-test":
			foundSkipTest = true
		}
	}

	assert.True(t, foundConfig, "Should have --config flag")
	assert.True(t, foundAPIURL, "Should have --api-url flag")
	assert.True(t, foundAPIKey, "Should have --api-key flag")
	assert.True(t, foundSkipTest, "Should have --skip-test flag")
}

func TestListCommand(t *testing.T) {
	assert.NotNil(t, ListCommand, "ListCommand should not be nil")
	assert.Equal(t, "list", ListCommand.Name)
	assert.NotNil(t, ListCommand.Action, "ListCommand should have an action")

	// Check flags
	var foundConfig, foundDetailed bool
	for _, flag := range ListCommand.Flags {
		switch flag.Names()[0] {
		case "config":
			foundConfig = true
		case "detailed":
			foundDetailed = true
		}
	}

	assert.True(t, foundConfig, "Should have --config flag")
	assert.True(t, foundDetailed, "Should have --detailed flag")
}

func TestTestCommand(t *testing.T) {
	assert.NotNil(t, TestCommand, "TestCommand should not be nil")
	assert.Equal(t, "test", TestCommand.Name)
	assert.NotNil(t, TestCommand.Action, "TestCommand should have an action")

	// Check flags
	var foundConfig, foundFullTest, foundPool bool
	for _, flag := range TestCommand.Flags {
		switch flag.Names()[0] {
		case "config":
			foundConfig = true
		case "full-test":
			foundFullTest = true
		case "pool":
			foundPool = true
		}
	}

	assert.True(t, foundConfig, "Should have --config flag")
	assert.True(t, foundFullTest, "Should have --full-test flag")
	assert.True(t, foundPool, "Should have --pool flag")
}
