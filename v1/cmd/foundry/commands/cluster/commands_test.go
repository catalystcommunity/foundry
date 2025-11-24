package cluster

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestCommandsStructure(t *testing.T) {
	cmd := Commands()

	assert.NotNil(t, cmd)
	assert.Equal(t, "cluster", cmd.Name)
	assert.Equal(t, "Manage Kubernetes cluster", cmd.Usage)
	assert.NotEmpty(t, cmd.Commands)
}

func TestCommandsHasInitSubcommand(t *testing.T) {
	cmd := Commands()

	var foundInit bool
	for _, subCmd := range cmd.Commands {
		if subCmd.Name == "init" {
			foundInit = true
			assert.Equal(t, "Initialize Kubernetes cluster", subCmd.Usage)
			break
		}
	}

	assert.True(t, foundInit, "Commands should include init subcommand")
}
