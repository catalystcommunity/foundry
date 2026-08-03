package gui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

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
