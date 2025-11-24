package cluster

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNewNodeListCommand(t *testing.T) {
	cmd := NewNodeListCommand()

	assert.NotNil(t, cmd)
	assert.Equal(t, "list", cmd.Name)
	assert.NotEmpty(t, cmd.Usage)
	assert.NotNil(t, cmd.Action)
}

// Note: Integration tests for runNodeList require a live Kubernetes cluster
// and OpenBAO instance. These will be covered in the integration test suite
// once the full stack is deployed.
