package component

import (
	"testing"

	"github.com/catalystcommunity/foundry/v1/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestGetSeaweedFSCredentials_FromStackConfig verifies the authoritative path:
// SeaweedFS S3 keys are read from the stack config's seaweedfs component, with no
// k8s client required (SeaweedFS runs with S3 auth disabled, so there is no secret).
func TestGetSeaweedFSCredentials_FromStackConfig(t *testing.T) {
	cfg := &config.Config{
		Components: config.ComponentMap{
			"seaweedfs": config.ComponentConfig{
				Config: map[string]any{
					"access_key": "AKIA_TEST",
					"secret_key": "SECRET_TEST",
				},
			},
		},
	}

	key, secret, err := getSeaweedFSCredentials(cfg, nil)
	require.NoError(t, err)
	assert.Equal(t, "AKIA_TEST", key)
	assert.Equal(t, "SECRET_TEST", secret)
}

// TestGetSeaweedFSCredentials_MissingErrors verifies the error path when neither
// the stack config nor a k8s secret can supply credentials.
func TestGetSeaweedFSCredentials_MissingErrors(t *testing.T) {
	// No seaweedfs component at all.
	cfg := &config.Config{Components: config.ComponentMap{}}
	_, _, err := getSeaweedFSCredentials(cfg, nil)
	require.Error(t, err)

	// seaweedfs component present but keys empty.
	cfg = &config.Config{
		Components: config.ComponentMap{
			"seaweedfs": config.ComponentConfig{
				Config: map[string]any{"access_key": "", "secret_key": ""},
			},
		},
	}
	_, _, err = getSeaweedFSCredentials(cfg, nil)
	require.Error(t, err)

	// Nil config, nil client.
	_, _, err = getSeaweedFSCredentials(nil, nil)
	require.Error(t, err)
}
