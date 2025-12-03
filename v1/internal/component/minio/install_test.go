package minio

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
			{Name: "minio-0", Namespace: "minio", Status: "Running"},
		},
	}

	cfg := DefaultConfig()
	err := Install(context.Background(), helmClient, k8sClient, cfg)
	require.NoError(t, err)

	// Verify repo was added
	require.Len(t, helmClient.reposAdded, 1)
	assert.Equal(t, minioRepoName, helmClient.reposAdded[0].Name)
	assert.Equal(t, minioRepoURL, helmClient.reposAdded[0].URL)

	// Verify chart was installed
	require.Len(t, helmClient.chartsInstalled, 1)
	assert.Equal(t, "minio", helmClient.chartsInstalled[0].ReleaseName)
	assert.Equal(t, "minio", helmClient.chartsInstalled[0].Namespace)
	assert.Equal(t, minioChart, helmClient.chartsInstalled[0].Chart)
	assert.True(t, helmClient.chartsInstalled[0].CreateNamespace)
	assert.True(t, helmClient.chartsInstalled[0].Wait)
	assert.Equal(t, 10*time.Minute, helmClient.chartsInstalled[0].Timeout)
}

func TestInstall_AlreadyInstalled(t *testing.T) {
	helmClient := &mockHelmClient{
		listReleases: []helm.Release{
			{
				Name:      "minio",
				Namespace: "minio",
				Status:    "deployed",
			},
		},
	}
	k8sClient := &mockK8sClient{
		pods: []*k8s.Pod{
			{Name: "minio-0", Namespace: "minio", Status: "Running"},
		},
	}

	cfg := DefaultConfig()
	err := Install(context.Background(), helmClient, k8sClient, cfg)
	require.NoError(t, err)

	// Should not install again
	assert.Empty(t, helmClient.chartsInstalled)
}

func TestInstall_NilHelmClient(t *testing.T) {
	err := Install(context.Background(), nil, &mockK8sClient{}, DefaultConfig())
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "helm client cannot be nil")
}

func TestInstall_NilConfig(t *testing.T) {
	helmClient := &mockHelmClient{}
	k8sClient := &mockK8sClient{
		pods: []*k8s.Pod{
			{Name: "minio-0", Namespace: "minio", Status: "Running"},
		},
	}

	// Should use default config
	err := Install(context.Background(), helmClient, k8sClient, nil)
	require.NoError(t, err)

	// Verify installation happened with defaults
	require.Len(t, helmClient.chartsInstalled, 1)
	assert.Equal(t, "minio", helmClient.chartsInstalled[0].ReleaseName)
}

func TestInstall_AddRepoError(t *testing.T) {
	helmClient := &mockHelmClient{
		addRepoErr: assert.AnError,
	}
	k8sClient := &mockK8sClient{}

	err := Install(context.Background(), helmClient, k8sClient, DefaultConfig())
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to add helm repository")
}

func TestInstall_InstallChartError(t *testing.T) {
	helmClient := &mockHelmClient{
		installErr: assert.AnError,
	}
	k8sClient := &mockK8sClient{}

	err := Install(context.Background(), helmClient, k8sClient, DefaultConfig())
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to install minio")
}

func TestInstall_FailedReleaseCleanedup(t *testing.T) {
	helmClient := &mockHelmClient{
		listReleases: []helm.Release{
			{
				Name:      "minio",
				Namespace: "minio",
				Status:    "failed",
			},
		},
	}
	k8sClient := &mockK8sClient{
		pods: []*k8s.Pod{
			{Name: "minio-0", Namespace: "minio", Status: "Running"},
		},
	}

	cfg := DefaultConfig()
	err := Install(context.Background(), helmClient, k8sClient, cfg)
	require.NoError(t, err)

	// Should have uninstalled the failed release
	require.Len(t, helmClient.uninstallCalls, 1)
	assert.Equal(t, "minio", helmClient.uninstallCalls[0].ReleaseName)

	// And installed fresh
	require.Len(t, helmClient.chartsInstalled, 1)
}

func TestBuildHelmValues_Standalone(t *testing.T) {
	cfg := &Config{
		Mode:         ModeStandalone,
		RootUser:     "admin",
		RootPassword: "secret",
		StorageSize:  "20Gi",
		Values:       map[string]interface{}{},
	}

	values := buildHelmValues(cfg)

	assert.Equal(t, "standalone", values["mode"])
	assert.Equal(t, "admin", values["rootUser"])
	assert.Equal(t, "secret", values["rootPassword"])

	persistence, ok := values["persistence"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, true, persistence["enabled"])
	assert.Equal(t, "20Gi", persistence["size"])
}

func TestBuildHelmValues_Distributed(t *testing.T) {
	cfg := &Config{
		Mode:         ModeDistributed,
		Replicas:     4,
		RootUser:     "admin",
		RootPassword: "secret",
		StorageSize:  "100Gi",
		StorageClass: "fast-storage",
		Values:       map[string]interface{}{},
	}

	values := buildHelmValues(cfg)

	assert.Equal(t, "distributed", values["mode"])
	assert.Equal(t, 4, values["replicas"])

	persistence, ok := values["persistence"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "100Gi", persistence["size"])
	assert.Equal(t, "fast-storage", persistence["storageClass"])
}

func TestBuildHelmValues_WithIngress(t *testing.T) {
	cfg := &Config{
		Mode:           ModeStandalone,
		RootUser:       "admin",
		IngressEnabled: true,
		IngressHost:    "minio.example.com",
		Values:         map[string]interface{}{},
	}

	values := buildHelmValues(cfg)

	ingress, ok := values["ingress"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, true, ingress["enabled"])
	assert.Equal(t, "contour", ingress["ingressClassName"])

	hosts, ok := ingress["hosts"].([]string)
	require.True(t, ok)
	assert.Contains(t, hosts, "minio.example.com")

	consoleIngress, ok := values["consoleIngress"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, true, consoleIngress["enabled"])

	consoleHosts, ok := consoleIngress["hosts"].([]string)
	require.True(t, ok)
	assert.Contains(t, consoleHosts, "console.minio.example.com")
}

func TestBuildHelmValues_WithBuckets(t *testing.T) {
	cfg := &Config{
		Mode:     ModeStandalone,
		RootUser: "admin",
		Buckets:  []string{"loki", "velero", "backups"},
		Values:   map[string]interface{}{},
	}

	values := buildHelmValues(cfg)

	buckets, ok := values["buckets"].([]map[string]interface{})
	require.True(t, ok)
	require.Len(t, buckets, 3)

	bucketNames := make([]string, 0, 3)
	for _, b := range buckets {
		name, ok := b["name"].(string)
		require.True(t, ok)
		bucketNames = append(bucketNames, name)
		assert.Equal(t, "none", b["policy"])
		assert.Equal(t, false, b["purge"])
	}
	assert.Contains(t, bucketNames, "loki")
	assert.Contains(t, bucketNames, "velero")
	assert.Contains(t, bucketNames, "backups")
}

func TestBuildHelmValues_WithTLS(t *testing.T) {
	cfg := &Config{
		Mode:       ModeStandalone,
		RootUser:   "admin",
		TLSEnabled: true,
		Values:     map[string]interface{}{},
	}

	values := buildHelmValues(cfg)

	tls, ok := values["tls"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, true, tls["enabled"])
}

func TestBuildHelmValues_CustomValues(t *testing.T) {
	cfg := &Config{
		Mode:     ModeStandalone,
		RootUser: "admin",
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
			{Name: "minio-0", Namespace: "minio", Status: "Running"},
		},
	}

	ctx := context.Background()
	err := verifyInstallation(ctx, k8sClient, "minio")
	assert.NoError(t, err)
}

func TestVerifyInstallation_NilClient(t *testing.T) {
	ctx := context.Background()
	err := verifyInstallation(ctx, nil, "minio")
	assert.NoError(t, err) // Should skip verification
}

func TestVerifyInstallation_PodsNotReady(t *testing.T) {
	k8sClient := &mockK8sClient{
		pods: []*k8s.Pod{
			{Name: "minio-0", Namespace: "minio", Status: "Pending"},
		},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	err := verifyInstallation(ctx, k8sClient, "minio")
	assert.Error(t, err)
}

func TestVerifyInstallation_ContextCanceled(t *testing.T) {
	k8sClient := &mockK8sClient{
		pods: []*k8s.Pod{},
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	err := verifyInstallation(ctx, k8sClient, "minio")
	assert.Error(t, err)
	assert.Equal(t, context.Canceled, err)
}
