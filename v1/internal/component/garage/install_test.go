package garage

import (
	"context"
	"testing"
	"time"

	"github.com/catalystcommunity/foundry/v1/internal/helm"
	"github.com/catalystcommunity/foundry/v1/internal/k8s"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestInstall_Success(t *testing.T) {
	helmClient := &mockHelmClient{}
	k8sClient := &mockK8sClient{
		pods: []*k8s.Pod{
			{Name: "garage-0", Namespace: "garage", Status: "Running"},
		},
	}

	cfg := &Config{
		Version:           "1.0.1",
		Namespace:         "garage",
		ReplicationFactor: 1,
		Replicas:          1,
		StorageSize:       "50Gi",
		S3Region:          "garage",
		AdminKey:          "test-key",
		AdminSecret:       "test-secret",
		Values:            map[string]interface{}{},
	}
	err := Install(context.Background(), helmClient, k8sClient, cfg)
	require.NoError(t, err)

	// Verify repo was added
	require.Len(t, helmClient.reposAdded, 1)
	assert.Equal(t, garageRepoName, helmClient.reposAdded[0].Name)
	assert.Equal(t, garageRepoURL, helmClient.reposAdded[0].URL)

	// Verify chart was installed
	require.Len(t, helmClient.chartsInstalled, 1)
	assert.Equal(t, "garage", helmClient.chartsInstalled[0].ReleaseName)
	assert.Equal(t, "garage", helmClient.chartsInstalled[0].Namespace)
	assert.Equal(t, garageChart, helmClient.chartsInstalled[0].Chart)
	assert.True(t, helmClient.chartsInstalled[0].CreateNamespace)
	assert.True(t, helmClient.chartsInstalled[0].Wait)
	assert.Equal(t, 10*time.Minute, helmClient.chartsInstalled[0].Timeout)
}

func TestInstall_AlreadyInstalled(t *testing.T) {
	helmClient := &mockHelmClient{
		listReleases: []helm.Release{
			{
				Name:      "garage",
				Namespace: "garage",
				Status:    "deployed",
			},
		},
	}
	k8sClient := &mockK8sClient{
		pods: []*k8s.Pod{
			{Name: "garage-0", Namespace: "garage", Status: "Running"},
		},
	}

	cfg := &Config{
		Version:           "1.0.1",
		Namespace:         "garage",
		ReplicationFactor: 1,
		Replicas:          1,
		AdminKey:          "test-key",
		AdminSecret:       "test-secret",
		Values:            map[string]interface{}{},
	}
	err := Install(context.Background(), helmClient, k8sClient, cfg)
	require.NoError(t, err)

	// Should not install again
	assert.Empty(t, helmClient.chartsInstalled)
}

func TestInstall_NilHelmClient(t *testing.T) {
	cfg := &Config{
		Version:           "1.0.1",
		Namespace:         "garage",
		ReplicationFactor: 1,
		Replicas:          1,
		AdminKey:          "test-key",
		AdminSecret:       "test-secret",
		Values:            map[string]interface{}{},
	}
	err := Install(context.Background(), nil, &mockK8sClient{}, cfg)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "helm client cannot be nil")
}

func TestInstall_NilConfig(t *testing.T) {
	helmClient := &mockHelmClient{}
	k8sClient := &mockK8sClient{
		pods: []*k8s.Pod{
			{Name: "garage-0", Namespace: "garage", Status: "Running"},
		},
	}

	// Should use default config
	err := Install(context.Background(), helmClient, k8sClient, nil)
	require.NoError(t, err)

	// Verify installation happened with defaults
	require.Len(t, helmClient.chartsInstalled, 1)
	assert.Equal(t, "garage", helmClient.chartsInstalled[0].ReleaseName)
}

func TestInstall_AddRepoError(t *testing.T) {
	helmClient := &mockHelmClient{
		addRepoErr: assert.AnError,
	}
	k8sClient := &mockK8sClient{}

	cfg := &Config{
		Version:           "1.0.1",
		Namespace:         "garage",
		ReplicationFactor: 1,
		Replicas:          1,
		AdminKey:          "test-key",
		AdminSecret:       "test-secret",
		Values:            map[string]interface{}{},
	}
	err := Install(context.Background(), helmClient, k8sClient, cfg)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to add helm repository")
}

func TestInstall_InstallChartError(t *testing.T) {
	helmClient := &mockHelmClient{
		installErr: assert.AnError,
	}
	k8sClient := &mockK8sClient{}

	cfg := &Config{
		Version:           "1.0.1",
		Namespace:         "garage",
		ReplicationFactor: 1,
		Replicas:          1,
		AdminKey:          "test-key",
		AdminSecret:       "test-secret",
		Values:            map[string]interface{}{},
	}
	err := Install(context.Background(), helmClient, k8sClient, cfg)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to install garage")
}

func TestInstall_FailedReleaseCleanedup(t *testing.T) {
	helmClient := &mockHelmClient{
		listReleases: []helm.Release{
			{
				Name:      "garage",
				Namespace: "garage",
				Status:    "failed",
			},
		},
	}
	k8sClient := &mockK8sClient{
		pods: []*k8s.Pod{
			{Name: "garage-0", Namespace: "garage", Status: "Running"},
		},
	}

	cfg := &Config{
		Version:           "1.0.1",
		Namespace:         "garage",
		ReplicationFactor: 1,
		Replicas:          1,
		AdminKey:          "test-key",
		AdminSecret:       "test-secret",
		Values:            map[string]interface{}{},
	}
	err := Install(context.Background(), helmClient, k8sClient, cfg)
	require.NoError(t, err)

	// Should have uninstalled the failed release
	require.Len(t, helmClient.uninstallCalls, 1)
	assert.Equal(t, "garage", helmClient.uninstallCalls[0].ReleaseName)

	// And installed fresh
	require.Len(t, helmClient.chartsInstalled, 1)
}

func TestBuildHelmValues_Basic(t *testing.T) {
	cfg := &Config{
		ReplicationFactor: 1,
		Replicas:          1,
		StorageSize:       "50Gi",
		S3Region:          "garage",
		AdminKey:          "test-admin-key",
		Values:            map[string]interface{}{},
	}

	values := buildHelmValues(cfg)

	// Check garage config
	garageConfig, ok := values["garage"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, 1, garageConfig["replication_factor"])

	s3Config, ok := garageConfig["s3_api"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "garage", s3Config["s3_region"])

	// Check persistence
	persistence, ok := values["persistence"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, true, persistence["enabled"])

	dataConfig, ok := persistence["data"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "50Gi", dataConfig["size"])

	// Check replicas
	assert.Equal(t, 1, values["replicaCount"])
}

func TestBuildHelmValues_MultiReplica(t *testing.T) {
	cfg := &Config{
		ReplicationFactor: 3,
		Replicas:          3,
		StorageSize:       "100Gi",
		StorageClass:      "longhorn",
		S3Region:          "us-east-1",
		AdminKey:          "test-admin-key",
		Values:            map[string]interface{}{},
	}

	values := buildHelmValues(cfg)

	// Check garage config
	garageConfig, ok := values["garage"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, 3, garageConfig["replication_factor"])

	// Check persistence with storage class
	persistence, ok := values["persistence"].(map[string]interface{})
	require.True(t, ok)

	dataConfig, ok := persistence["data"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "100Gi", dataConfig["size"])
	assert.Equal(t, "longhorn", dataConfig["storageClass"])

	// Check replicas
	assert.Equal(t, 3, values["replicaCount"])
}

func TestBuildHelmValues_CustomValues(t *testing.T) {
	cfg := &Config{
		ReplicationFactor: 1,
		Replicas:          1,
		AdminKey:          "test-admin-key",
		Values: map[string]interface{}{
			"custom": "value",
			"nested": map[string]interface{}{
				"key": "value",
			},
		},
	}

	values := buildHelmValues(cfg)

	// Custom values should be preserved
	assert.Equal(t, "value", values["custom"])
	assert.NotNil(t, values["nested"])
}

func TestVerifyInstallation_Success(t *testing.T) {
	k8sClient := &mockK8sClient{
		pods: []*k8s.Pod{
			{Name: "garage-0", Namespace: "garage", Status: "Running"},
		},
	}

	ctx := context.Background()
	err := verifyInstallation(ctx, k8sClient, "garage")
	assert.NoError(t, err)
}

func TestVerifyInstallation_NilClient(t *testing.T) {
	ctx := context.Background()
	err := verifyInstallation(ctx, nil, "garage")
	assert.NoError(t, err) // Should skip verification
}

func TestVerifyInstallation_PodsNotReady(t *testing.T) {
	k8sClient := &mockK8sClient{
		pods: []*k8s.Pod{
			{Name: "garage-0", Namespace: "garage", Status: "Pending"},
		},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	err := verifyInstallation(ctx, k8sClient, "garage")
	assert.Error(t, err)
}

func TestVerifyInstallation_ContextCanceled(t *testing.T) {
	k8sClient := &mockK8sClient{
		pods: []*k8s.Pod{},
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	err := verifyInstallation(ctx, k8sClient, "garage")
	assert.Error(t, err)
	assert.Equal(t, context.Canceled, err)
}

func TestContainsSubstring(t *testing.T) {
	tests := []struct {
		s      string
		substr string
		want   bool
	}{
		{"garage-0", "garage", true},
		{"my-garage-pod", "garage", true},
		{"other-pod", "garage", false},
		{"", "garage", false},
		{"garage", "", true},
		{"ga", "garage", false},
	}

	for _, tt := range tests {
		t.Run(tt.s+"_"+tt.substr, func(t *testing.T) {
			got := containsSubstring(tt.s, tt.substr)
			assert.Equal(t, tt.want, got)
		})
	}
}
