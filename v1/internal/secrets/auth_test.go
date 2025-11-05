package secrets

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zalando/go-keyring"
)

func TestStoreAuthToken(t *testing.T) {
	// Clean up before and after test
	t.Cleanup(func() {
		_ = ClearAuthToken()
	})

	t.Run("empty token", func(t *testing.T) {
		err := StoreAuthToken("")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "token cannot be empty")
	})

	t.Run("successful storage", func(t *testing.T) {
		token := "test-token-12345"
		err := StoreAuthToken(token)
		require.NoError(t, err)

		// Verify we can retrieve it
		retrieved, err := LoadAuthToken()
		require.NoError(t, err)
		assert.Equal(t, token, retrieved)
	})

	t.Run("overwrites existing token", func(t *testing.T) {
		// Store first token
		err := StoreAuthToken("first-token")
		require.NoError(t, err)

		// Store second token (should overwrite)
		err = StoreAuthToken("second-token")
		require.NoError(t, err)

		// Verify second token is retrieved
		retrieved, err := LoadAuthToken()
		require.NoError(t, err)
		assert.Equal(t, "second-token", retrieved)
	})
}

func TestLoadAuthToken(t *testing.T) {
	// Clean up before and after test
	t.Cleanup(func() {
		_ = ClearAuthToken()
	})

	t.Run("no token stored", func(t *testing.T) {
		// Ensure no token exists
		_ = ClearAuthToken()

		_, err := LoadAuthToken()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "no auth token found")
	})

	t.Run("loads stored token", func(t *testing.T) {
		token := "test-token-67890"
		err := StoreAuthToken(token)
		require.NoError(t, err)

		retrieved, err := LoadAuthToken()
		require.NoError(t, err)
		assert.Equal(t, token, retrieved)
	})
}

func TestClearAuthToken(t *testing.T) {
	t.Cleanup(func() {
		_ = ClearAuthToken()
	})

	t.Run("clears stored token", func(t *testing.T) {
		// Store a token
		err := StoreAuthToken("test-token-clear")
		require.NoError(t, err)

		// Verify it's stored
		_, err = LoadAuthToken()
		require.NoError(t, err)

		// Clear it
		err = ClearAuthToken()
		require.NoError(t, err)

		// Verify it's gone
		_, err = LoadAuthToken()
		require.Error(t, err)
	})

	t.Run("no error when nothing to clear", func(t *testing.T) {
		// Ensure nothing is stored
		_ = ClearAuthToken()

		// Clear again (should not error)
		err := ClearAuthToken()
		require.NoError(t, err)
	})
}

func TestFileFallback(t *testing.T) {
	t.Cleanup(func() {
		_ = deleteTokenFile()
	})

	t.Run("stores in file when keyring unavailable", func(t *testing.T) {
		// This test verifies the file fallback mechanism works
		token := "fallback-token-123"

		// Store directly in file (simulating keyring failure)
		err := storeTokenInFile(token)
		require.NoError(t, err)

		// Verify file was created with correct permissions
		tokenPath, err := getTokenFilePath()
		require.NoError(t, err)

		info, err := os.Stat(tokenPath)
		require.NoError(t, err)

		// Check permissions (should be 0600 on Unix-like systems)
		mode := info.Mode()
		assert.True(t, mode&0400 != 0, "file should be readable by owner")
		assert.True(t, mode&0200 != 0, "file should be writable by owner")
		// On Unix-like systems, we can check that group/other bits are not set
		// On Windows, this may not apply, so we skip this check on Windows
		if os.PathSeparator == '/' {
			assert.True(t, mode&0077 == 0, "file should not be readable/writable by group or others")
		}

		// Verify we can load it
		retrieved, err := loadTokenFromFile()
		require.NoError(t, err)
		assert.Equal(t, token, retrieved)
	})

	t.Run("loads from file", func(t *testing.T) {
		token := "file-token-456"
		err := storeTokenInFile(token)
		require.NoError(t, err)

		retrieved, err := loadTokenFromFile()
		require.NoError(t, err)
		assert.Equal(t, token, retrieved)
	})

	t.Run("deletes file", func(t *testing.T) {
		token := "delete-token-789"
		err := storeTokenInFile(token)
		require.NoError(t, err)

		// Verify file exists
		tokenPath, err := getTokenFilePath()
		require.NoError(t, err)
		_, err = os.Stat(tokenPath)
		require.NoError(t, err)

		// Delete it
		err = deleteTokenFile()
		require.NoError(t, err)

		// Verify it's gone
		_, err = os.Stat(tokenPath)
		assert.True(t, os.IsNotExist(err))
	})

	t.Run("creates directory if needed", func(t *testing.T) {
		// Get the token file path
		tokenPath, err := getTokenFilePath()
		require.NoError(t, err)

		// Remove the entire .foundry directory
		foundryDir := filepath.Dir(tokenPath)
		_ = os.RemoveAll(foundryDir)

		// Store token (should create directory)
		token := "create-dir-token"
		err = storeTokenInFile(token)
		require.NoError(t, err)

		// Verify directory was created
		info, err := os.Stat(foundryDir)
		require.NoError(t, err)
		assert.True(t, info.IsDir())

		// Verify directory has correct permissions (0700 on Unix-like systems)
		mode := info.Mode()
		assert.True(t, mode.IsDir())
		if os.PathSeparator == '/' {
			assert.True(t, mode&0700 != 0, "directory should be accessible by owner")
			assert.True(t, mode&0077 == 0, "directory should not be accessible by group or others")
		}
	})
}

func TestGetTokenFilePath(t *testing.T) {
	t.Run("returns valid path", func(t *testing.T) {
		path, err := getTokenFilePath()
		require.NoError(t, err)
		assert.NotEmpty(t, path)

		// Path should end with .foundry/.foundry-token
		assert.Contains(t, path, ".foundry")
		assert.Contains(t, path, FallbackFileName)
	})

	t.Run("path is in user home directory", func(t *testing.T) {
		path, err := getTokenFilePath()
		require.NoError(t, err)

		homeDir, err := os.UserHomeDir()
		require.NoError(t, err)

		assert.Contains(t, path, homeDir)
	})
}

func TestKeyringIntegration(t *testing.T) {
	// This test verifies actual keyring functionality
	// May fail in CI environments without keyring support
	t.Run("keyring store and retrieve", func(t *testing.T) {
		// Clean up
		defer func() {
			_ = keyring.Delete(KeyringService, KeyringUser)
		}()

		token := "keyring-test-token"
		err := keyring.Set(KeyringService, KeyringUser, token)
		if err != nil {
			t.Skip("Keyring not available in this environment:", err)
		}

		retrieved, err := keyring.Get(KeyringService, KeyringUser)
		require.NoError(t, err)
		assert.Equal(t, token, retrieved)
	})

	t.Run("keyring delete", func(t *testing.T) {
		// Store a token
		token := "keyring-delete-token"
		err := keyring.Set(KeyringService, KeyringUser, token)
		if err != nil {
			t.Skip("Keyring not available in this environment:", err)
		}

		// Delete it
		err = keyring.Delete(KeyringService, KeyringUser)
		require.NoError(t, err)

		// Verify it's gone
		_, err = keyring.Get(KeyringService, KeyringUser)
		require.Error(t, err)
	})
}

func TestEndToEndFlow(t *testing.T) {
	t.Cleanup(func() {
		_ = ClearAuthToken()
	})

	t.Run("complete auth flow", func(t *testing.T) {
		// 1. Store token
		originalToken := "end-to-end-token-123"
		err := StoreAuthToken(originalToken)
		require.NoError(t, err)

		// 2. Load token
		retrieved, err := LoadAuthToken()
		require.NoError(t, err)
		assert.Equal(t, originalToken, retrieved)

		// 3. Update token
		newToken := "updated-token-456"
		err = StoreAuthToken(newToken)
		require.NoError(t, err)

		// 4. Load updated token
		retrieved, err = LoadAuthToken()
		require.NoError(t, err)
		assert.Equal(t, newToken, retrieved)

		// 5. Clear token
		err = ClearAuthToken()
		require.NoError(t, err)

		// 6. Verify it's cleared
		_, err = LoadAuthToken()
		require.Error(t, err)
	})
}
