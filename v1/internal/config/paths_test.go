package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetConfigDir(t *testing.T) {
	// Save original env var
	originalEnv := os.Getenv("FOUNDRY_CONFIG_DIR")
	defer func() {
		if originalEnv != "" {
			os.Setenv("FOUNDRY_CONFIG_DIR", originalEnv)
		} else {
			os.Unsetenv("FOUNDRY_CONFIG_DIR")
		}
	}()

	t.Run("default directory", func(t *testing.T) {
		os.Unsetenv("FOUNDRY_CONFIG_DIR")
		dir, err := GetConfigDir()
		require.NoError(t, err)

		homeDir, err := os.UserHomeDir()
		require.NoError(t, err)

		expected := filepath.Join(homeDir, DefaultConfigDir)
		assert.Equal(t, expected, dir)
	})

	t.Run("environment override", func(t *testing.T) {
		customDir := "/custom/config/dir"
		os.Setenv("FOUNDRY_CONFIG_DIR", customDir)
		defer os.Unsetenv("FOUNDRY_CONFIG_DIR")

		dir, err := GetConfigDir()
		require.NoError(t, err)
		assert.Equal(t, customDir, dir)
	})
}

func TestFindConfig(t *testing.T) {
	// Create a temporary directory structure for testing
	tmpDir, err := os.MkdirTemp("", "foundry-test-*")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	// Set config dir to our temp directory
	os.Setenv("FOUNDRY_CONFIG_DIR", tmpDir)
	defer os.Unsetenv("FOUNDRY_CONFIG_DIR")

	// Create test config files
	err = os.WriteFile(filepath.Join(tmpDir, "stack.yaml"), []byte("test"), 0644)
	require.NoError(t, err)

	err = os.WriteFile(filepath.Join(tmpDir, "production.yaml"), []byte("test"), 0644)
	require.NoError(t, err)

	err = os.WriteFile(filepath.Join(tmpDir, "staging.yml"), []byte("test"), 0644)
	require.NoError(t, err)

	tests := []struct {
		name       string
		configName string
		wantErr    bool
		errMsg     string
		checkPath  func(t *testing.T, path string)
	}{
		{
			name:       "default config",
			configName: "",
			wantErr:    false,
			checkPath: func(t *testing.T, path string) {
				assert.Equal(t, filepath.Join(tmpDir, "stack.yaml"), path)
			},
		},
		{
			name:       "explicit config name",
			configName: "production",
			wantErr:    false,
			checkPath: func(t *testing.T, path string) {
				assert.Equal(t, filepath.Join(tmpDir, "production.yaml"), path)
			},
		},
		{
			name:       "config name with extension",
			configName: "staging.yml",
			wantErr:    false,
			checkPath: func(t *testing.T, path string) {
				assert.Equal(t, filepath.Join(tmpDir, "staging.yml"), path)
			},
		},
		{
			name:       "non-existent config",
			configName: "nonexistent",
			wantErr:    true,
			errMsg:     "config file not found",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path, err := FindConfig(tt.configName)
			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errMsg)
			} else {
				require.NoError(t, err)
				if tt.checkPath != nil {
					tt.checkPath(t, path)
				}
			}
		})
	}
}

func TestFindConfig_AbsolutePath(t *testing.T) {
	// Create a temporary file
	tmpFile, err := os.CreateTemp("", "test-config-*.yaml")
	require.NoError(t, err)
	tmpPath := tmpFile.Name()
	defer os.Remove(tmpPath)

	tmpFile.WriteString("test")
	tmpFile.Close()

	// Test with absolute path
	path, err := FindConfig(tmpPath)
	require.NoError(t, err)
	assert.Equal(t, tmpPath, path)

	// Test with non-existent absolute path
	_, err = FindConfig("/nonexistent/path/to/config.yaml")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "config file not found")
}

func TestListConfigs(t *testing.T) {
	// Create a temporary directory structure for testing
	tmpDir, err := os.MkdirTemp("", "foundry-test-*")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	// Set config dir to our temp directory
	os.Setenv("FOUNDRY_CONFIG_DIR", tmpDir)
	defer os.Unsetenv("FOUNDRY_CONFIG_DIR")

	t.Run("empty directory", func(t *testing.T) {
		configs, err := ListConfigs()
		require.NoError(t, err)
		assert.Empty(t, configs)
	})

	// Create test config files
	testFiles := []string{
		"stack.yaml",
		"production.yaml",
		"staging.yml",
		"dev.yaml",
	}

	for _, file := range testFiles {
		err = os.WriteFile(filepath.Join(tmpDir, file), []byte("test"), 0644)
		require.NoError(t, err)
	}

	// Create a non-YAML file (should be ignored)
	err = os.WriteFile(filepath.Join(tmpDir, "README.md"), []byte("test"), 0644)
	require.NoError(t, err)

	// Create a subdirectory (should be ignored)
	err = os.Mkdir(filepath.Join(tmpDir, "subdir"), 0755)
	require.NoError(t, err)

	t.Run("multiple configs", func(t *testing.T) {
		configs, err := ListConfigs()
		require.NoError(t, err)
		assert.Len(t, configs, len(testFiles))

		// Check that all test files are in the list
		configMap := make(map[string]bool)
		for _, config := range configs {
			configMap[filepath.Base(config)] = true
		}

		for _, file := range testFiles {
			assert.True(t, configMap[file], "Expected %s to be in config list", file)
		}

		// Verify non-YAML file is not included
		assert.False(t, configMap["README.md"])
	})
}

func TestListConfigs_NoDirectory(t *testing.T) {
	// Use a non-existent directory
	os.Setenv("FOUNDRY_CONFIG_DIR", "/nonexistent/foundry/dir")
	defer os.Unsetenv("FOUNDRY_CONFIG_DIR")

	configs, err := ListConfigs()
	require.NoError(t, err)
	assert.Empty(t, configs)
}

func TestEnsureConfigDir(t *testing.T) {
	// Create a temporary parent directory
	tmpParent, err := os.MkdirTemp("", "foundry-parent-*")
	require.NoError(t, err)
	defer os.RemoveAll(tmpParent)

	tmpDir := filepath.Join(tmpParent, "test-foundry")

	// Set config dir to our temp directory
	os.Setenv("FOUNDRY_CONFIG_DIR", tmpDir)
	defer os.Unsetenv("FOUNDRY_CONFIG_DIR")

	// Directory shouldn't exist yet
	_, err = os.Stat(tmpDir)
	require.True(t, os.IsNotExist(err))

	// Ensure config dir
	dir, err := EnsureConfigDir()
	require.NoError(t, err)
	assert.Equal(t, tmpDir, dir)

	// Directory should now exist
	info, err := os.Stat(tmpDir)
	require.NoError(t, err)
	assert.True(t, info.IsDir())

	// Calling again should be idempotent
	dir2, err := EnsureConfigDir()
	require.NoError(t, err)
	assert.Equal(t, dir, dir2)
}
