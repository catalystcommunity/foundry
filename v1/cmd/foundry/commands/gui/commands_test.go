package gui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/catalystcommunity/foundry/v1/internal/config"
	"github.com/catalystcommunity/foundry/v1/internal/host"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestReadTokenFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "token")
	token := strings.Repeat("a", 43)
	require.NoError(t, os.WriteFile(path, []byte(token+"\n"), 0600))

	actual, err := readTokenFile(path)
	require.NoError(t, err)
	assert.Equal(t, token, actual)
}

func TestReadTokenFileRejectsWeakOrExposedToken(t *testing.T) {
	t.Run("weak", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "token")
		require.NoError(t, os.WriteFile(path, []byte("short"), 0600))
		_, err := readTokenFile(path)
		require.Error(t, err)
	})
	t.Run("group readable", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "token")
		require.NoError(t, os.WriteFile(path, []byte(strings.Repeat("a", 43)), 0640))
		_, err := readTokenFile(path)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "permissions")
	})
}

func TestShouldUseManager(t *testing.T) {
	path := filepath.Join(t.TempDir(), "stack.yaml")
	cfg := &config.Config{
		Hosts:      []*host.Host{{Hostname: "manager", Address: "192.0.2.10", Port: 22, User: "root", Roles: []string{host.RoleManagement}}},
		Components: config.ComponentMap{"k3s": {}},
		Management: &config.ManagementConfig{Host: "manager", Port: 9080, Image: "foundry", Version: "latest", DataPath: "/var/lib/foundry"},
	}
	require.NoError(t, config.Save(cfg, path))
	assert.True(t, shouldUseManager(path, false, false))
	assert.False(t, shouldUseManager(path, false, true))

	cfg.Management = nil
	require.NoError(t, config.Save(cfg, path))
	assert.False(t, shouldUseManager(path, false, false))
	assert.True(t, shouldUseManager(path, true, false))
}
