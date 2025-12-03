package loki

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
			{Name: "loki-0", Namespace: "loki", Status: "Running"},
		},
	}

	cfg := DefaultConfig()
	cfg.PromtailEnabled = false // Disable Promtail for simpler test
	err := Install(context.Background(), helmClient, k8sClient, cfg)
	require.NoError(t, err)

	// Verify repo was added
	require.Len(t, helmClient.reposAdded, 1)
	assert.Equal(t, lokiRepoName, helmClient.reposAdded[0].Name)
	assert.Equal(t, lokiRepoURL, helmClient.reposAdded[0].URL)

	// Verify chart was installed
	require.Len(t, helmClient.chartsInstalled, 1)
	assert.Equal(t, releaseName, helmClient.chartsInstalled[0].ReleaseName)
	assert.Equal(t, "loki", helmClient.chartsInstalled[0].Namespace)
	assert.Equal(t, lokiChart, helmClient.chartsInstalled[0].Chart)
	assert.True(t, helmClient.chartsInstalled[0].CreateNamespace)
	assert.True(t, helmClient.chartsInstalled[0].Wait)
	assert.Equal(t, 10*time.Minute, helmClient.chartsInstalled[0].Timeout)
}

func TestInstall_WithPromtail(t *testing.T) {
	helmClient := &mockHelmClient{}
	k8sClient := &mockK8sClient{
		pods: []*k8s.Pod{
			{Name: "loki-0", Namespace: "loki", Status: "Running"},
		},
	}

	cfg := DefaultConfig()
	cfg.PromtailEnabled = true
	err := Install(context.Background(), helmClient, k8sClient, cfg)
	require.NoError(t, err)

	// Verify both Loki and Promtail were installed
	require.Len(t, helmClient.chartsInstalled, 2)
	assert.Equal(t, releaseName, helmClient.chartsInstalled[0].ReleaseName)
	assert.Equal(t, "promtail", helmClient.chartsInstalled[1].ReleaseName)
	assert.Equal(t, promtailChart, helmClient.chartsInstalled[1].Chart)
}

func TestInstall_AlreadyInstalled(t *testing.T) {
	helmClient := &mockHelmClient{
		listReleases: []helm.Release{
			{
				Name:      releaseName,
				Namespace: "loki",
				Status:    "deployed",
			},
		},
	}
	k8sClient := &mockK8sClient{
		pods: []*k8s.Pod{
			{Name: "loki-0", Namespace: "loki", Status: "Running"},
		},
	}

	cfg := DefaultConfig()
	cfg.PromtailEnabled = false
	err := Install(context.Background(), helmClient, k8sClient, cfg)
	require.NoError(t, err)

	// Should not install again
	assert.Empty(t, helmClient.chartsInstalled)
}

func TestInstall_AlreadyInstalled_WithPromtail(t *testing.T) {
	helmClient := &mockHelmClient{
		listReleases: []helm.Release{
			{
				Name:      releaseName,
				Namespace: "loki",
				Status:    "deployed",
			},
		},
	}
	k8sClient := &mockK8sClient{
		pods: []*k8s.Pod{
			{Name: "loki-0", Namespace: "loki", Status: "Running"},
		},
	}

	cfg := DefaultConfig()
	cfg.PromtailEnabled = true
	err := Install(context.Background(), helmClient, k8sClient, cfg)
	require.NoError(t, err)

	// Should only install Promtail
	require.Len(t, helmClient.chartsInstalled, 1)
	assert.Equal(t, "promtail", helmClient.chartsInstalled[0].ReleaseName)
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
			{Name: "loki-0", Namespace: "loki", Status: "Running"},
		},
	}

	// Should use default config
	err := Install(context.Background(), helmClient, k8sClient, nil)
	require.NoError(t, err)

	// Verify installation happened with defaults (Loki + Promtail)
	require.Len(t, helmClient.chartsInstalled, 2)
	assert.Equal(t, releaseName, helmClient.chartsInstalled[0].ReleaseName)
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
	assert.Contains(t, err.Error(), "failed to install loki")
}

func TestInstall_FailedReleaseCleanedup(t *testing.T) {
	helmClient := &mockHelmClient{
		listReleases: []helm.Release{
			{
				Name:      releaseName,
				Namespace: "loki",
				Status:    "failed",
			},
		},
	}
	k8sClient := &mockK8sClient{
		pods: []*k8s.Pod{
			{Name: "loki-0", Namespace: "loki", Status: "Running"},
		},
	}

	cfg := DefaultConfig()
	cfg.PromtailEnabled = false
	err := Install(context.Background(), helmClient, k8sClient, cfg)
	require.NoError(t, err)

	// Should have uninstalled the failed release
	require.Len(t, helmClient.uninstallCalls, 1)
	assert.Equal(t, releaseName, helmClient.uninstallCalls[0].ReleaseName)

	// And installed fresh
	require.Len(t, helmClient.chartsInstalled, 1)
}

func TestBuildHelmValues_SingleBinary(t *testing.T) {
	cfg := &Config{
		DeploymentMode: "SingleBinary",
		StorageBackend: BackendFilesystem,
		RetentionDays:  30,
		StorageSize:    "10Gi",
		Values:         map[string]interface{}{},
	}

	values := buildHelmValues(cfg)

	assert.Equal(t, "SingleBinary", values["deploymentMode"])

	// Check single binary config
	singleBinary, ok := values["singleBinary"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, 1, singleBinary["replicas"])

	persistence, ok := singleBinary["persistence"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, true, persistence["enabled"])
	assert.Equal(t, "10Gi", persistence["size"])

	// Check other components are disabled
	backend, ok := values["backend"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, 0, backend["replicas"])
}

func TestBuildHelmValues_S3Storage(t *testing.T) {
	cfg := &Config{
		DeploymentMode: "SingleBinary",
		StorageBackend: BackendS3,
		RetentionDays:  30,
		S3Endpoint:     "http://minio:9000",
		S3Bucket:       "loki",
		S3AccessKey:    "mykey",
		S3SecretKey:    "mysecret",
		S3Region:       "us-east-1",
		StorageSize:    "10Gi",
		Values:         map[string]interface{}{},
	}

	values := buildHelmValues(cfg)

	lokiConfig, ok := values["loki"].(map[string]interface{})
	require.True(t, ok)

	storage, ok := lokiConfig["storage"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "s3", storage["type"])

	s3Config, ok := storage["s3"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "http://minio:9000", s3Config["endpoint"])
	assert.Equal(t, "loki", s3Config["bucketnames"])
	assert.Equal(t, "mykey", s3Config["access_key_id"])
	assert.Equal(t, "mysecret", s3Config["secret_access_key"])
	assert.Equal(t, "us-east-1", s3Config["region"])
	assert.Equal(t, true, s3Config["insecure"])
	assert.Equal(t, true, s3Config["s3ForcePathStyle"])
}

func TestBuildHelmValues_FilesystemStorage(t *testing.T) {
	cfg := &Config{
		DeploymentMode: "SingleBinary",
		StorageBackend: BackendFilesystem,
		RetentionDays:  30,
		StorageSize:    "10Gi",
		Values:         map[string]interface{}{},
	}

	values := buildHelmValues(cfg)

	lokiConfig, ok := values["loki"].(map[string]interface{})
	require.True(t, ok)

	storage, ok := lokiConfig["storage"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "filesystem", storage["type"])
}

func TestBuildHelmValues_WithIngress(t *testing.T) {
	cfg := &Config{
		DeploymentMode: "SingleBinary",
		StorageBackend: BackendFilesystem,
		RetentionDays:  30,
		IngressEnabled: true,
		IngressHost:    "loki.example.com",
		StorageSize:    "10Gi",
		Values:         map[string]interface{}{},
	}

	values := buildHelmValues(cfg)

	gateway, ok := values["gateway"].(map[string]interface{})
	require.True(t, ok)

	ingress, ok := gateway["ingress"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, true, ingress["enabled"])
	assert.Equal(t, "contour", ingress["ingressClassName"])
}

func TestBuildHelmValues_WithStorageClass(t *testing.T) {
	cfg := &Config{
		DeploymentMode: "SingleBinary",
		StorageBackend: BackendFilesystem,
		RetentionDays:  30,
		StorageClass:   "fast-storage",
		StorageSize:    "10Gi",
		Values:         map[string]interface{}{},
	}

	values := buildHelmValues(cfg)

	singleBinary, ok := values["singleBinary"].(map[string]interface{})
	require.True(t, ok)

	persistence, ok := singleBinary["persistence"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "fast-storage", persistence["storageClass"])
}

func TestBuildHelmValues_Retention(t *testing.T) {
	cfg := &Config{
		DeploymentMode: "SingleBinary",
		StorageBackend: BackendFilesystem,
		RetentionDays:  15,
		StorageSize:    "10Gi",
		Values:         map[string]interface{}{},
	}

	values := buildHelmValues(cfg)

	lokiConfig, ok := values["loki"].(map[string]interface{})
	require.True(t, ok)

	limitsConfig, ok := lokiConfig["limits_config"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "360h", limitsConfig["retention_period"]) // 15 * 24 = 360
}

func TestBuildHelmValues_CustomValues(t *testing.T) {
	cfg := &Config{
		DeploymentMode: "SingleBinary",
		StorageBackend: BackendFilesystem,
		RetentionDays:  30,
		StorageSize:    "10Gi",
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

func TestBuildPromtailValues(t *testing.T) {
	cfg := &Config{
		Namespace: "loki",
	}

	values := buildPromtailValues(cfg)

	config, ok := values["config"].(map[string]interface{})
	require.True(t, ok)

	clients, ok := config["clients"].([]map[string]interface{})
	require.True(t, ok)
	require.Len(t, clients, 1)
	assert.Equal(t, "http://loki-gateway.loki.svc.cluster.local:80/loki/api/v1/push", clients[0]["url"])

	// Check ServiceMonitor is enabled
	serviceMonitor, ok := values["serviceMonitor"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, true, serviceMonitor["enabled"])
}

func TestVerifyInstallation_Success(t *testing.T) {
	k8sClient := &mockK8sClient{
		pods: []*k8s.Pod{
			{Name: "loki-0", Namespace: "loki", Status: "Running"},
		},
	}

	ctx := context.Background()
	err := verifyInstallation(ctx, k8sClient, "loki")
	assert.NoError(t, err)
}

func TestVerifyInstallation_GatewayPod(t *testing.T) {
	k8sClient := &mockK8sClient{
		pods: []*k8s.Pod{
			{Name: "loki-gateway-abc123", Namespace: "loki", Status: "Running"},
		},
	}

	ctx := context.Background()
	err := verifyInstallation(ctx, k8sClient, "loki")
	assert.NoError(t, err)
}

func TestVerifyInstallation_NilClient(t *testing.T) {
	ctx := context.Background()
	err := verifyInstallation(ctx, nil, "loki")
	assert.NoError(t, err) // Should skip verification
}

func TestVerifyInstallation_PodsNotReady(t *testing.T) {
	k8sClient := &mockK8sClient{
		pods: []*k8s.Pod{
			{Name: "loki-0", Namespace: "loki", Status: "Pending"},
		},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	err := verifyInstallation(ctx, k8sClient, "loki")
	assert.Error(t, err)
}

func TestVerifyInstallation_ContextCanceled(t *testing.T) {
	k8sClient := &mockK8sClient{
		pods: []*k8s.Pod{},
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	err := verifyInstallation(ctx, k8sClient, "loki")
	assert.Error(t, err)
	assert.Equal(t, context.Canceled, err)
}

func TestContainsSubstring(t *testing.T) {
	tests := []struct {
		s        string
		substr   string
		expected bool
	}{
		{"loki-0", "loki", true},
		{"loki-gateway-abc123", "loki", true},
		{"promtail-xyz", "promtail", true},
		{"hello-world", "world", true},
		{"hello-world", "foo", false},
		{"short", "longer-than-short", false},
		{"", "", true},
		{"abc", "", true},
	}

	for _, tt := range tests {
		result := containsSubstring(tt.s, tt.substr)
		assert.Equal(t, tt.expected, result, "containsSubstring(%q, %q)", tt.s, tt.substr)
	}
}

func TestInstallPromtail_AlreadyInstalled(t *testing.T) {
	helmClient := &mockHelmClient{
		listReleases: []helm.Release{
			{
				Name:      "promtail",
				Namespace: "loki",
				Status:    "deployed",
			},
		},
	}
	k8sClient := &mockK8sClient{}

	cfg := DefaultConfig()
	err := installPromtail(context.Background(), helmClient, k8sClient, cfg)
	require.NoError(t, err)

	// Should not install again
	assert.Empty(t, helmClient.chartsInstalled)
}

func TestInstallPromtail_FailedReleaseCleanup(t *testing.T) {
	helmClient := &mockHelmClient{
		listReleases: []helm.Release{
			{
				Name:      "promtail",
				Namespace: "loki",
				Status:    "failed",
			},
		},
	}
	k8sClient := &mockK8sClient{}

	cfg := DefaultConfig()
	err := installPromtail(context.Background(), helmClient, k8sClient, cfg)
	require.NoError(t, err)

	// Should have uninstalled failed release
	require.Len(t, helmClient.uninstallCalls, 1)
	assert.Equal(t, "promtail", helmClient.uninstallCalls[0].ReleaseName)

	// And installed fresh
	require.Len(t, helmClient.chartsInstalled, 1)
	assert.Equal(t, "promtail", helmClient.chartsInstalled[0].ReleaseName)
}
