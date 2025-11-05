package integration

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/catalystcommunity/foundry/v1/internal/helm"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"sigs.k8s.io/kind/pkg/cluster"
	kindconfig "sigs.k8s.io/kind/pkg/apis/config/v1alpha4"
)

// TestHelmIntegration tests Helm operations against a real Kind cluster
func TestHelmIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	ctx := context.Background()

	// Create unique cluster name
	clusterName := fmt.Sprintf("foundry-helm-test-%d", time.Now().Unix())

	// Create Kind provider
	provider := cluster.NewProvider()

	// Create Kind cluster configuration
	kindCfg := &kindconfig.Cluster{
		TypeMeta: kindconfig.TypeMeta{
			APIVersion: "kind.x-k8s.io/v1alpha4",
			Kind:       "Cluster",
		},
		Nodes: []kindconfig.Node{
			{
				Role: kindconfig.ControlPlaneRole,
			},
		},
	}

	// Create the cluster
	t.Logf("Creating Kind cluster: %s", clusterName)
	err := provider.Create(
		clusterName,
		cluster.CreateWithV1Alpha4Config(kindCfg),
		cluster.CreateWithWaitForReady(time.Minute*5),
	)
	require.NoError(t, err, "Failed to create Kind cluster")

	// Ensure cleanup
	defer func() {
		t.Logf("Deleting Kind cluster: %s", clusterName)
		if err := provider.Delete(clusterName, ""); err != nil {
			t.Logf("Failed to delete cluster: %v", err)
		}
	}()

	// Get kubeconfig from Kind
	kubeconfigPath := filepath.Join(os.Getenv("HOME"), ".kube", "kind-config-"+clusterName)

	// Export kubeconfig for this specific cluster
	err = provider.ExportKubeConfig(clusterName, kubeconfigPath, false)
	require.NoError(t, err, "Failed to export kubeconfig")
	t.Logf("Kubeconfig path: %s", kubeconfigPath)

	// Read kubeconfig file
	kubeconfigData, err := os.ReadFile(kubeconfigPath)
	require.NoError(t, err, "Failed to read kubeconfig")
	require.NotEmpty(t, kubeconfigData, "Kubeconfig should not be empty")

	// Create a shared Helm client for all tests to use the same repo configuration
	sharedHelmClient, err := helm.NewClient(kubeconfigData, "default")
	require.NoError(t, err, "Failed to create shared Helm client")
	defer sharedHelmClient.Close()

	// Add Bitnami repository once for all tests
	err = sharedHelmClient.AddRepo(ctx, helm.RepoAddOptions{
		Name: "bitnami",
		URL:  "https://charts.bitnami.com/bitnami",
	})
	require.NoError(t, err, "Failed to add Bitnami repository")
	t.Logf("Added Bitnami repository")

	t.Run("Helm_Client_Initialization", func(t *testing.T) {
		// Create Helm client from kubeconfig
		helmClient, err := helm.NewClient(kubeconfigData, "default")
		require.NoError(t, err, "Should create Helm client")
		require.NotNil(t, helmClient, "Helm client should not be nil")

		// Clean up client resources
		defer helmClient.Close()

		// Test that client was created successfully
		assert.NotNil(t, helmClient, "Client should be initialized")
	})

	t.Run("Helm_Repo_Add", func(t *testing.T) {
		// Test adding the same repo again without ForceUpdate (should fail)
		err := sharedHelmClient.AddRepo(ctx, helm.RepoAddOptions{
			Name: "bitnami",
			URL:  "https://charts.bitnami.com/bitnami",
		})
		assert.Error(t, err, "Should fail to add duplicate repository")
		assert.Contains(t, err.Error(), "already exists", "Error should mention repository already exists")

		// Add with ForceUpdate should succeed
		err = sharedHelmClient.AddRepo(ctx, helm.RepoAddOptions{
			Name:        "bitnami",
			URL:         "https://charts.bitnami.com/bitnami",
			ForceUpdate: true,
		})
		assert.NoError(t, err, "Should update existing repository with ForceUpdate")

		// Test adding a new repository
		err = sharedHelmClient.AddRepo(ctx, helm.RepoAddOptions{
			Name: "jetstack",
			URL:  "https://charts.jetstack.io",
		})
		assert.NoError(t, err, "Should add new repository")
	})

	t.Run("Helm_Chart_Install", func(t *testing.T) {
		// Install a simple chart (nginx - lightweight and fast to deploy)
		err := sharedHelmClient.Install(ctx, helm.InstallOptions{
			ReleaseName:     "test-nginx",
			Namespace:       "default",
			Chart:           "bitnami/nginx",
			CreateNamespace: false,
			Wait:            true,
			Timeout:         time.Minute * 3,
			Values: map[string]interface{}{
				// Use minimal resources for testing
				"service": map[string]interface{}{
					"type": "ClusterIP",
				},
				"replicaCount": 1,
			},
		})
		require.NoError(t, err, "Should install nginx chart")

		// Try installing the same release again (should fail)
		err = sharedHelmClient.Install(ctx, helm.InstallOptions{
			ReleaseName: "test-nginx",
			Namespace:   "default",
			Chart:       "bitnami/nginx",
		})
		assert.Error(t, err, "Should fail to install duplicate release")
	})

	t.Run("Helm_Release_List", func(t *testing.T) {
		// List releases in default namespace
		releases, err := sharedHelmClient.List(ctx, "default")
		require.NoError(t, err, "Should list releases")
		require.NotEmpty(t, releases, "Should have at least one release (test-nginx)")

		// Verify the nginx release exists
		found := false
		for _, release := range releases {
			if release.Name == "test-nginx" {
				found = true
				assert.Equal(t, "default", release.Namespace, "Release should be in default namespace")
				assert.Contains(t, release.Chart, "nginx", "Chart should be nginx")
				assert.NotEmpty(t, release.Status, "Release should have a status")
				t.Logf("Found release: %s, Status: %s, Chart: %s", release.Name, release.Status, release.Chart)
				break
			}
		}
		assert.True(t, found, "Should find test-nginx release")
	})

	t.Run("Helm_Chart_Upgrade", func(t *testing.T) {
		// Upgrade the nginx release with different values
		err := sharedHelmClient.Upgrade(ctx, helm.UpgradeOptions{
			ReleaseName: "test-nginx",
			Namespace:   "default",
			Chart:       "bitnami/nginx",
			Wait:        true,
			Timeout:     time.Minute * 3,
			Values: map[string]interface{}{
				"service": map[string]interface{}{
					"type": "ClusterIP",
				},
				"replicaCount": 2, // Change replica count
			},
		})
		require.NoError(t, err, "Should upgrade nginx release")

		// Verify the upgrade succeeded by checking version incremented
		releases, err := sharedHelmClient.List(ctx, "default")
		require.NoError(t, err, "Should list releases")

		for _, release := range releases {
			if release.Name == "test-nginx" {
				assert.Equal(t, 2, release.Version, "Release version should be 2 after upgrade")
				t.Logf("Release upgraded to version: %d", release.Version)
				break
			}
		}
	})

	t.Run("Helm_Chart_Install_With_Namespace_Creation", func(t *testing.T) {
		// Install chart in a new namespace that doesn't exist yet
		testNamespace := "helm-test-ns"
		err := sharedHelmClient.Install(ctx, helm.InstallOptions{
			ReleaseName:     "test-app",
			Namespace:       testNamespace,
			Chart:           "bitnami/nginx",
			CreateNamespace: true,
			Wait:            true,
			Timeout:         time.Minute * 3,
			Values: map[string]interface{}{
				"service": map[string]interface{}{
					"type": "ClusterIP",
				},
				"replicaCount": 1,
			},
		})
		require.NoError(t, err, "Should install chart with namespace creation")

		// Verify release exists in the new namespace
		releases, err := sharedHelmClient.List(ctx, testNamespace)
		require.NoError(t, err, "Should list releases in new namespace")
		assert.NotEmpty(t, releases, "Should have release in new namespace")

		found := false
		for _, release := range releases {
			if release.Name == "test-app" {
				found = true
				assert.Equal(t, testNamespace, release.Namespace, "Release should be in test namespace")
				break
			}
		}
		assert.True(t, found, "Should find test-app release in new namespace")
	})

	t.Run("Helm_Chart_Uninstall", func(t *testing.T) {
		// Uninstall the nginx release
		err := sharedHelmClient.Uninstall(ctx, helm.UninstallOptions{
			ReleaseName: "test-nginx",
			Namespace:   "default",
			Wait:        true,
			Timeout:     time.Minute * 2,
		})
		require.NoError(t, err, "Should uninstall nginx release")

		// Verify release is gone
		releases, err := sharedHelmClient.List(ctx, "default")
		require.NoError(t, err, "Should list releases")

		for _, release := range releases {
			assert.NotEqual(t, "test-nginx", release.Name, "test-nginx should be uninstalled")
		}

		// Uninstall the test-app release from the test namespace
		err = sharedHelmClient.Uninstall(ctx, helm.UninstallOptions{
			ReleaseName: "test-app",
			Namespace:   "helm-test-ns",
			Wait:        true,
			Timeout:     time.Minute * 2,
		})
		require.NoError(t, err, "Should uninstall test-app release")

		// Try uninstalling a non-existent release (should fail)
		err = sharedHelmClient.Uninstall(ctx, helm.UninstallOptions{
			ReleaseName: "non-existent-release",
			Namespace:   "default",
		})
		assert.Error(t, err, "Should fail to uninstall non-existent release")
	})

	t.Run("Helm_Error_Handling", func(t *testing.T) {
		helmClient, err := helm.NewClient(kubeconfigData, "default")
		require.NoError(t, err, "Should create Helm client")
		defer helmClient.Close()

		// Try to install without release name (should fail)
		err = helmClient.Install(ctx, helm.InstallOptions{
			ReleaseName: "",
			Chart:       "bitnami/nginx",
		})
		assert.Error(t, err, "Should fail with empty release name")
		assert.Contains(t, err.Error(), "release name cannot be empty", "Error should mention release name")

		// Try to install without chart (should fail)
		err = helmClient.Install(ctx, helm.InstallOptions{
			ReleaseName: "test",
			Chart:       "",
		})
		assert.Error(t, err, "Should fail with empty chart")
		assert.Contains(t, err.Error(), "chart cannot be empty", "Error should mention chart")

		// Try to install non-existent chart (should fail)
		err = helmClient.Install(ctx, helm.InstallOptions{
			ReleaseName: "test-fail",
			Chart:       "bitnami/non-existent-chart-xyz",
			Timeout:     time.Second * 30,
		})
		assert.Error(t, err, "Should fail with non-existent chart")
	})
}
