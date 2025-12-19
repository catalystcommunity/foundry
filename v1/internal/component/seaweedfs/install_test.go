package seaweedfs

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
			{Name: "seaweedfs-master-0", Namespace: "seaweedfs", Status: "Running"},
			{Name: "seaweedfs-volume-0", Namespace: "seaweedfs", Status: "Running"},
			{Name: "seaweedfs-filer-0", Namespace: "seaweedfs", Status: "Running"},
		},
	}

	cfg := &Config{
		Version:        "4.0.401",
		Namespace:      "seaweedfs",
		MasterReplicas: 1,
		VolumeReplicas: 1,
		FilerReplicas:  1,
		StorageSize:    "50Gi",
		S3Enabled:      true,
		S3Port:         8333,
		AccessKey:      "test-key",
		SecretKey:      "test-secret",
		Values:         map[string]interface{}{},
	}
	err := Install(context.Background(), helmClient, k8sClient, cfg)
	require.NoError(t, err)

	// Verify repo was added
	require.Len(t, helmClient.reposAdded, 1)
	assert.Equal(t, seaweedfsRepoName, helmClient.reposAdded[0].Name)
	assert.Equal(t, seaweedfsRepoURL, helmClient.reposAdded[0].URL)

	// Verify chart was installed
	require.Len(t, helmClient.chartsInstalled, 1)
	assert.Equal(t, "seaweedfs", helmClient.chartsInstalled[0].ReleaseName)
	assert.Equal(t, "seaweedfs", helmClient.chartsInstalled[0].Namespace)
	assert.Equal(t, seaweedfsChart, helmClient.chartsInstalled[0].Chart)
	assert.True(t, helmClient.chartsInstalled[0].CreateNamespace)
	assert.True(t, helmClient.chartsInstalled[0].Wait)
	assert.Equal(t, 10*time.Minute, helmClient.chartsInstalled[0].Timeout)
}

func TestInstall_AlreadyInstalled(t *testing.T) {
	helmClient := &mockHelmClient{
		listReleases: []helm.Release{
			{
				Name:      "seaweedfs",
				Namespace: "seaweedfs",
				Status:    "deployed",
			},
		},
	}
	k8sClient := &mockK8sClient{
		pods: []*k8s.Pod{
			{Name: "seaweedfs-master-0", Namespace: "seaweedfs", Status: "Running"},
		},
	}

	cfg := &Config{
		Version:        "4.0.401",
		Namespace:      "seaweedfs",
		MasterReplicas: 1,
		VolumeReplicas: 1,
		FilerReplicas:  1,
		AccessKey:      "test-key",
		SecretKey:      "test-secret",
		Values:         map[string]interface{}{},
	}
	err := Install(context.Background(), helmClient, k8sClient, cfg)
	require.NoError(t, err)

	// Should not install again
	assert.Empty(t, helmClient.chartsInstalled)
}

func TestInstall_NilHelmClient(t *testing.T) {
	cfg := &Config{
		Version:        "4.0.401",
		Namespace:      "seaweedfs",
		MasterReplicas: 1,
		VolumeReplicas: 1,
		FilerReplicas:  1,
		AccessKey:      "test-key",
		SecretKey:      "test-secret",
		Values:         map[string]interface{}{},
	}
	err := Install(context.Background(), nil, &mockK8sClient{}, cfg)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "helm client cannot be nil")
}

func TestInstall_NilConfig(t *testing.T) {
	helmClient := &mockHelmClient{}
	k8sClient := &mockK8sClient{
		pods: []*k8s.Pod{
			{Name: "seaweedfs-master-0", Namespace: "seaweedfs", Status: "Running"},
		},
		serviceMonitorCRDExists: true, // Default config has ServiceMonitorEnabled=true
	}

	// Should use default config
	err := Install(context.Background(), helmClient, k8sClient, nil)
	require.NoError(t, err)

	// Verify installation happened with defaults
	require.Len(t, helmClient.chartsInstalled, 1)
	assert.Equal(t, "seaweedfs", helmClient.chartsInstalled[0].ReleaseName)
}

func TestInstall_AddRepoError(t *testing.T) {
	helmClient := &mockHelmClient{
		addRepoErr: assert.AnError,
	}
	k8sClient := &mockK8sClient{}

	cfg := &Config{
		Version:        "4.0.401",
		Namespace:      "seaweedfs",
		MasterReplicas: 1,
		VolumeReplicas: 1,
		FilerReplicas:  1,
		AccessKey:      "test-key",
		SecretKey:      "test-secret",
		Values:         map[string]interface{}{},
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
		Version:        "4.0.401",
		Namespace:      "seaweedfs",
		MasterReplicas: 1,
		VolumeReplicas: 1,
		FilerReplicas:  1,
		AccessKey:      "test-key",
		SecretKey:      "test-secret",
		Values:         map[string]interface{}{},
	}
	err := Install(context.Background(), helmClient, k8sClient, cfg)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to install seaweedfs")
}

func TestInstall_FailedReleaseUpgraded(t *testing.T) {
	helmClient := &mockHelmClient{
		listReleases: []helm.Release{
			{
				Name:      "seaweedfs",
				Namespace: "seaweedfs",
				Status:    "failed",
			},
		},
	}
	k8sClient := &mockK8sClient{
		pods: []*k8s.Pod{
			{Name: "seaweedfs-master-0", Namespace: "seaweedfs", Status: "Running"},
		},
	}

	cfg := &Config{
		Version:        "4.0.401",
		Namespace:      "seaweedfs",
		MasterReplicas: 1,
		VolumeReplicas: 1,
		FilerReplicas:  1,
		AccessKey:      "test-key",
		SecretKey:      "test-secret",
		Values:         map[string]interface{}{},
	}
	err := Install(context.Background(), helmClient, k8sClient, cfg)
	require.NoError(t, err)

	// Should have attempted to upgrade the failed release (not uninstall - to avoid data loss)
	require.Len(t, helmClient.upgradeCalls, 1)
	assert.Equal(t, "seaweedfs", helmClient.upgradeCalls[0].ReleaseName)

	// Should NOT have installed fresh (upgrade was used instead)
	assert.Empty(t, helmClient.chartsInstalled)
}

func TestBuildHelmValues_Basic(t *testing.T) {
	cfg := &Config{
		MasterReplicas: 1,
		VolumeReplicas: 1,
		FilerReplicas:  1,
		StorageSize:    "50Gi",
		S3Enabled:      true,
		S3Port:         8333,
		Values:         map[string]interface{}{},
	}

	values := buildHelmValues(cfg)

	// Check master config
	masterConfig, ok := values["master"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, 1, masterConfig["replicas"])

	// Check volume config
	volumeConfig, ok := values["volume"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, 1, volumeConfig["replicas"])

	dataDirs, ok := volumeConfig["dataDirs"].([]map[string]interface{})
	require.True(t, ok)
	require.Len(t, dataDirs, 1)
	assert.Equal(t, "50Gi", dataDirs[0]["size"])

	// Check filer config
	filerConfig, ok := values["filer"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, 1, filerConfig["replicas"])

	// Check S3 config
	s3Config, ok := values["s3"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, true, s3Config["enabled"])
	assert.Equal(t, 8333, s3Config["port"])
}

func TestBuildHelmValues_MultiReplica(t *testing.T) {
	cfg := &Config{
		MasterReplicas: 3,
		VolumeReplicas: 3,
		FilerReplicas:  2,
		StorageSize:    "100Gi",
		StorageClass:   "longhorn",
		S3Enabled:      true,
		S3Port:         8333,
		Values:         map[string]interface{}{},
	}

	values := buildHelmValues(cfg)

	// Check master config
	masterConfig, ok := values["master"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, 3, masterConfig["replicas"])

	masterPersistence, ok := masterConfig["persistence"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "longhorn", masterPersistence["storageClass"])

	// Check volume config
	volumeConfig, ok := values["volume"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, 3, volumeConfig["replicas"])

	dataDirs, ok := volumeConfig["dataDirs"].([]map[string]interface{})
	require.True(t, ok)
	require.Len(t, dataDirs, 1)
	assert.Equal(t, "100Gi", dataDirs[0]["size"])
	assert.Equal(t, "longhorn", dataDirs[0]["storageClass"])

	// Check filer config
	filerConfig, ok := values["filer"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, 2, filerConfig["replicas"])

	filerPersistence, ok := filerConfig["persistence"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "longhorn", filerPersistence["storageClass"])
}

func TestBuildHelmValues_CustomValues(t *testing.T) {
	cfg := &Config{
		MasterReplicas: 1,
		VolumeReplicas: 1,
		FilerReplicas:  1,
		S3Enabled:      true,
		S3Port:         8333,
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
			{Name: "seaweedfs-master-0", Namespace: "seaweedfs", Status: "Running"},
		},
	}

	ctx := context.Background()
	err := verifyInstallation(ctx, k8sClient, "seaweedfs")
	assert.NoError(t, err)
}

func TestVerifyInstallation_NilClient(t *testing.T) {
	ctx := context.Background()
	err := verifyInstallation(ctx, nil, "seaweedfs")
	assert.NoError(t, err) // Should skip verification
}

func TestVerifyInstallation_PodsNotReady(t *testing.T) {
	k8sClient := &mockK8sClient{
		pods: []*k8s.Pod{
			{Name: "seaweedfs-master-0", Namespace: "seaweedfs", Status: "Pending"},
		},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	err := verifyInstallation(ctx, k8sClient, "seaweedfs")
	assert.Error(t, err)
}

func TestVerifyInstallation_ContextCanceled(t *testing.T) {
	k8sClient := &mockK8sClient{
		pods: []*k8s.Pod{},
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	err := verifyInstallation(ctx, k8sClient, "seaweedfs")
	assert.Error(t, err)
	assert.Equal(t, context.Canceled, err)
}

func TestContainsSubstring(t *testing.T) {
	tests := []struct {
		s      string
		substr string
		want   bool
	}{
		{"seaweedfs-master-0", "seaweedfs", true},
		{"my-seaweedfs-pod", "seaweedfs", true},
		{"other-pod", "seaweedfs", false},
		{"", "seaweedfs", false},
		{"seaweedfs", "", true},
		{"sea", "seaweedfs", false},
	}

	for _, tt := range tests {
		t.Run(tt.s+"_"+tt.substr, func(t *testing.T) {
			got := containsSubstring(tt.s, tt.substr)
			assert.Equal(t, tt.want, got)
		})
	}
}
