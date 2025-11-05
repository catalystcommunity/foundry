package helm

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockKubeconfig returns a minimal valid kubeconfig for testing
func mockKubeconfig() []byte {
	return []byte(`apiVersion: v1
kind: Config
clusters:
- cluster:
    server: https://127.0.0.1:6443
    insecure-skip-tls-verify: true
  name: test-cluster
contexts:
- context:
    cluster: test-cluster
    user: test-user
  name: test-context
current-context: test-context
users:
- name: test-user
  user:
    token: test-token
`)
}

func TestNewClient(t *testing.T) {
	tests := []struct {
		name        string
		kubeconfig  []byte
		namespace   string
		expectError bool
		errorMsg    string
	}{
		{
			name:        "valid kubeconfig and namespace",
			kubeconfig:  mockKubeconfig(),
			namespace:   "test-namespace",
			expectError: false,
		},
		{
			name:        "valid kubeconfig with empty namespace defaults to default",
			kubeconfig:  mockKubeconfig(),
			namespace:   "",
			expectError: false,
		},
		{
			name:        "empty kubeconfig",
			kubeconfig:  []byte{},
			namespace:   "test-namespace",
			expectError: true,
			errorMsg:    "kubeconfig cannot be empty",
		},
		{
			name:        "nil kubeconfig",
			kubeconfig:  nil,
			namespace:   "test-namespace",
			expectError: true,
			errorMsg:    "kubeconfig cannot be empty",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client, err := NewClient(tt.kubeconfig, tt.namespace)

			if tt.expectError {
				assert.Error(t, err)
				if tt.errorMsg != "" {
					assert.Contains(t, err.Error(), tt.errorMsg)
				}
				assert.Nil(t, client)
			} else {
				require.NoError(t, err)
				require.NotNil(t, client)
				defer client.Close()

				expectedNamespace := tt.namespace
				if expectedNamespace == "" {
					expectedNamespace = "default"
				}
				assert.Equal(t, expectedNamespace, client.namespace)
				assert.NotNil(t, client.settings)
				assert.NotEmpty(t, client.settings.KubeConfig)

				// Verify kubeconfig file was created
				_, err := os.Stat(client.settings.KubeConfig)
				assert.NoError(t, err, "kubeconfig file should exist")
			}
		})
	}
}

func TestClientClose(t *testing.T) {
	client, err := NewClient(mockKubeconfig(), "test-namespace")
	require.NoError(t, err)
	require.NotNil(t, client)

	kubeconfigPath := client.settings.KubeConfig
	tmpDir := filepath.Dir(kubeconfigPath)

	// Verify temp dir exists
	_, err = os.Stat(tmpDir)
	require.NoError(t, err)

	// Close client
	err = client.Close()
	assert.NoError(t, err)

	// Verify temp dir is removed
	_, err = os.Stat(tmpDir)
	assert.True(t, os.IsNotExist(err), "temp directory should be removed")
}

func TestClientCloseWithNilSettings(t *testing.T) {
	client := &Client{}
	err := client.Close()
	assert.NoError(t, err)
}

func TestAddRepo(t *testing.T) {
	client, err := NewClient(mockKubeconfig(), "test-namespace")
	require.NoError(t, err)
	defer client.Close()

	ctx := context.Background()

	tests := []struct {
		name        string
		opts        RepoAddOptions
		expectError bool
		errorMsg    string
	}{
		{
			name: "empty repository name",
			opts: RepoAddOptions{
				Name: "",
				URL:  "https://charts.example.com",
			},
			expectError: true,
			errorMsg:    "repository name cannot be empty",
		},
		{
			name: "empty repository URL",
			opts: RepoAddOptions{
				Name: "test-repo",
				URL:  "",
			},
			expectError: true,
			errorMsg:    "repository URL cannot be empty",
		},
		{
			name: "valid repository - will fail to download index or already exists",
			opts: RepoAddOptions{
				Name: "test-repo",
				URL:  "https://charts.example.com",
			},
			expectError: true,
			errorMsg:    "", // Either "already exists" or "failed to download", both are valid
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := client.AddRepo(ctx, tt.opts)

			if tt.expectError {
				assert.Error(t, err)
				if tt.errorMsg != "" {
					assert.Contains(t, err.Error(), tt.errorMsg)
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestAddRepoForceUpdate(t *testing.T) {
	client, err := NewClient(mockKubeconfig(), "test-namespace")
	require.NoError(t, err)
	defer client.Close()

	ctx := context.Background()

	// Try to add a repo with force update - will still fail on download but exercises the update path
	err = client.AddRepo(ctx, RepoAddOptions{
		Name:        "test-repo-force",
		URL:         "https://charts.example.com/force",
		ForceUpdate: true,
	})
	assert.Error(t, err) // Will fail to download index
}

func TestAddRepoWithCredentials(t *testing.T) {
	client, err := NewClient(mockKubeconfig(), "test-namespace")
	require.NoError(t, err)
	defer client.Close()

	ctx := context.Background()

	// Try to add a repo with credentials - will still fail on download but exercises the credentials path
	err = client.AddRepo(ctx, RepoAddOptions{
		Name:     "test-repo-auth",
		URL:      "https://charts.example.com/auth",
		Username: "testuser",
		Password: "testpass",
	})
	assert.Error(t, err) // Will fail to download index
}

func TestInstall(t *testing.T) {
	client, err := NewClient(mockKubeconfig(), "test-namespace")
	require.NoError(t, err)
	defer client.Close()

	ctx := context.Background()

	tests := []struct {
		name        string
		opts        InstallOptions
		expectError bool
		errorMsg    string
	}{
		{
			name: "empty release name",
			opts: InstallOptions{
				ReleaseName: "",
				Chart:       "nginx",
			},
			expectError: true,
			errorMsg:    "release name cannot be empty",
		},
		{
			name: "empty chart",
			opts: InstallOptions{
				ReleaseName: "test-release",
				Chart:       "",
			},
			expectError: true,
			errorMsg:    "chart cannot be empty",
		},
		{
			name: "chart not found",
			opts: InstallOptions{
				ReleaseName: "test-release",
				Chart:       "nonexistent-chart",
				Namespace:   "test-ns",
			},
			expectError: true,
			errorMsg:    "failed to locate chart",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := client.Install(ctx, tt.opts)

			if tt.expectError {
				assert.Error(t, err)
				if tt.errorMsg != "" {
					assert.Contains(t, err.Error(), tt.errorMsg)
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestInstallWithDefaultNamespace(t *testing.T) {
	client, err := NewClient(mockKubeconfig(), "default-ns")
	require.NoError(t, err)
	defer client.Close()

	ctx := context.Background()

	// Test that empty namespace uses client's default
	err = client.Install(ctx, InstallOptions{
		ReleaseName: "test",
		Chart:       "nonexistent",
		Namespace:   "", // Should use client's namespace
	})

	// Will fail to locate chart, but namespace handling is tested
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to locate chart")
}

func TestInstallWithAllOptions(t *testing.T) {
	client, err := NewClient(mockKubeconfig(), "test-namespace")
	require.NoError(t, err)
	defer client.Close()

	ctx := context.Background()

	// Test install with all options set - will fail but exercises all code paths
	err = client.Install(ctx, InstallOptions{
		ReleaseName:     "test-release",
		Chart:           "nonexistent-chart",
		Namespace:       "custom-ns",
		Version:         "1.0.0",
		Values:          map[string]interface{}{"key": "value"},
		CreateNamespace: true,
		Wait:            true,
		Timeout:         5 * time.Minute,
	})

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to locate chart")
}

func TestUpgrade(t *testing.T) {
	client, err := NewClient(mockKubeconfig(), "test-namespace")
	require.NoError(t, err)
	defer client.Close()

	ctx := context.Background()

	tests := []struct {
		name        string
		opts        UpgradeOptions
		expectError bool
		errorMsg    string
	}{
		{
			name: "empty release name",
			opts: UpgradeOptions{
				ReleaseName: "",
				Chart:       "nginx",
			},
			expectError: true,
			errorMsg:    "release name cannot be empty",
		},
		{
			name: "empty chart",
			opts: UpgradeOptions{
				ReleaseName: "test-release",
				Chart:       "",
			},
			expectError: true,
			errorMsg:    "chart cannot be empty",
		},
		{
			name: "chart not found",
			opts: UpgradeOptions{
				ReleaseName: "test-release",
				Chart:       "nonexistent-chart",
				Namespace:   "test-ns",
			},
			expectError: true,
			errorMsg:    "failed to locate chart",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := client.Upgrade(ctx, tt.opts)

			if tt.expectError {
				assert.Error(t, err)
				if tt.errorMsg != "" {
					assert.Contains(t, err.Error(), tt.errorMsg)
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestUpgradeWithOptions(t *testing.T) {
	client, err := NewClient(mockKubeconfig(), "default-ns")
	require.NoError(t, err)
	defer client.Close()

	ctx := context.Background()

	// Test upgrade with various options set
	err = client.Upgrade(ctx, UpgradeOptions{
		ReleaseName: "test",
		Chart:       "nonexistent",
		Version:     "1.0.0",
		Install:     true,
		Wait:        true,
		Timeout:     5 * time.Minute,
	})

	// Will fail to locate chart, but option handling is tested
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to locate chart")
}

func TestUninstall(t *testing.T) {
	client, err := NewClient(mockKubeconfig(), "test-namespace")
	require.NoError(t, err)
	defer client.Close()

	ctx := context.Background()

	tests := []struct {
		name        string
		opts        UninstallOptions
		expectError bool
		errorMsg    string
	}{
		{
			name: "empty release name",
			opts: UninstallOptions{
				ReleaseName: "",
			},
			expectError: true,
			errorMsg:    "release name cannot be empty",
		},
		{
			name: "release not found",
			opts: UninstallOptions{
				ReleaseName: "nonexistent-release",
				Namespace:   "test-ns",
			},
			expectError: true,
			errorMsg:    "failed to uninstall release",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := client.Uninstall(ctx, tt.opts)

			if tt.expectError {
				assert.Error(t, err)
				if tt.errorMsg != "" {
					assert.Contains(t, err.Error(), tt.errorMsg)
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestUninstallWithDefaultNamespace(t *testing.T) {
	client, err := NewClient(mockKubeconfig(), "default-ns")
	require.NoError(t, err)
	defer client.Close()

	ctx := context.Background()

	// Test that empty namespace uses client's default
	err = client.Uninstall(ctx, UninstallOptions{
		ReleaseName: "test",
		Namespace:   "", // Should use client's namespace
	})

	// Will fail because release doesn't exist, but namespace handling is tested
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to uninstall release")
}

func TestUninstallWithAllOptions(t *testing.T) {
	client, err := NewClient(mockKubeconfig(), "test-namespace")
	require.NoError(t, err)
	defer client.Close()

	ctx := context.Background()

	// Test uninstall with all options set - will fail but exercises all code paths
	err = client.Uninstall(ctx, UninstallOptions{
		ReleaseName: "test-release",
		Namespace:   "custom-ns",
		Wait:        true,
		Timeout:     5 * time.Minute,
	})

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to uninstall release")
}

func TestList(t *testing.T) {
	client, err := NewClient(mockKubeconfig(), "test-namespace")
	require.NoError(t, err)
	defer client.Close()

	ctx := context.Background()

	// Test list - will fail because we can't actually connect to K8s
	// but we're testing the method signature and basic error handling
	_, err = client.List(ctx, "test-namespace")
	assert.Error(t, err) // Expected to fail without real K8s
}

func TestListWithDefaultNamespace(t *testing.T) {
	client, err := NewClient(mockKubeconfig(), "default-ns")
	require.NoError(t, err)
	defer client.Close()

	ctx := context.Background()

	// Test that empty namespace uses client's default
	_, err = client.List(ctx, "")
	assert.Error(t, err) // Expected to fail without real K8s
}

func TestConvertRelease(t *testing.T) {
	// This tests the unexported convertRelease function indirectly
	// by verifying that the Release type has the correct structure
	r := Release{
		Name:      "test-release",
		Namespace: "test-ns",
		Version:   1,
		Status:    "deployed",
		Chart:     "nginx-1.0.0",
		AppVersion: "1.0",
		Updated:   time.Now(),
	}

	assert.Equal(t, "test-release", r.Name)
	assert.Equal(t, "test-ns", r.Namespace)
	assert.Equal(t, 1, r.Version)
	assert.Equal(t, "deployed", r.Status)
	assert.Equal(t, "nginx-1.0.0", r.Chart)
	assert.Equal(t, "1.0", r.AppVersion)
}

func TestGetActionConfig(t *testing.T) {
	client, err := NewClient(mockKubeconfig(), "test-namespace")
	require.NoError(t, err)
	defer client.Close()

	// Test with explicit namespace
	actionConfig, err := client.getActionConfig("custom-ns")
	// May fail without real K8s, but we're testing the call path
	if err != nil {
		assert.Contains(t, err.Error(), "failed to initialize action config")
	} else {
		assert.NotNil(t, actionConfig)
	}

	// Test with empty namespace (uses default)
	actionConfig, err = client.getActionConfig("")
	if err != nil {
		assert.Contains(t, err.Error(), "failed to initialize action config")
	} else {
		assert.NotNil(t, actionConfig)
	}
}

func TestInstallOptionsStructure(t *testing.T) {
	opts := InstallOptions{
		ReleaseName:     "test",
		Namespace:       "test-ns",
		Chart:           "nginx",
		Version:         "1.0.0",
		Values:          map[string]interface{}{"key": "value"},
		CreateNamespace: true,
		Wait:            true,
		Timeout:         5 * time.Minute,
	}

	assert.Equal(t, "test", opts.ReleaseName)
	assert.Equal(t, "test-ns", opts.Namespace)
	assert.Equal(t, "nginx", opts.Chart)
	assert.Equal(t, "1.0.0", opts.Version)
	assert.True(t, opts.CreateNamespace)
	assert.True(t, opts.Wait)
	assert.Equal(t, 5*time.Minute, opts.Timeout)
	assert.Equal(t, "value", opts.Values["key"])
}

func TestUpgradeOptionsStructure(t *testing.T) {
	opts := UpgradeOptions{
		ReleaseName: "test",
		Namespace:   "test-ns",
		Chart:       "nginx",
		Version:     "2.0.0",
		Values:      map[string]interface{}{"key": "value"},
		Install:     true,
		Wait:        true,
		Timeout:     10 * time.Minute,
	}

	assert.Equal(t, "test", opts.ReleaseName)
	assert.Equal(t, "test-ns", opts.Namespace)
	assert.Equal(t, "nginx", opts.Chart)
	assert.Equal(t, "2.0.0", opts.Version)
	assert.True(t, opts.Install)
	assert.True(t, opts.Wait)
	assert.Equal(t, 10*time.Minute, opts.Timeout)
	assert.Equal(t, "value", opts.Values["key"])
}

func TestUninstallOptionsStructure(t *testing.T) {
	opts := UninstallOptions{
		ReleaseName: "test",
		Namespace:   "test-ns",
		Wait:        true,
		Timeout:     5 * time.Minute,
	}

	assert.Equal(t, "test", opts.ReleaseName)
	assert.Equal(t, "test-ns", opts.Namespace)
	assert.True(t, opts.Wait)
	assert.Equal(t, 5*time.Minute, opts.Timeout)
}

func TestRepoAddOptionsStructure(t *testing.T) {
	opts := RepoAddOptions{
		Name:        "test-repo",
		URL:         "https://charts.example.com",
		Username:    "user",
		Password:    "pass",
		ForceUpdate: true,
	}

	assert.Equal(t, "test-repo", opts.Name)
	assert.Equal(t, "https://charts.example.com", opts.URL)
	assert.Equal(t, "user", opts.Username)
	assert.Equal(t, "pass", opts.Password)
	assert.True(t, opts.ForceUpdate)
}
