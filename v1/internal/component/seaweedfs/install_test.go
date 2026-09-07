package seaweedfs

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"strings"
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
	assert.Equal(t, []string{"seaweedfs"}, k8sClient.namespacesCreated)

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
		S3Enabled:      true,
		S3Port:         8333,
		AccessKey:      "test-key",
		SecretKey:      "test-secret",
		Buckets:        []string{"loki"},
		Values:         map[string]interface{}{},
	}
	err := Install(context.Background(), helmClient, k8sClient, cfg)
	require.NoError(t, err)

	// A deployed release must still be reconciled.
	assert.Empty(t, helmClient.chartsInstalled)
	require.Len(t, helmClient.upgradeCalls, 1)
	require.Len(t, k8sClient.manifests, 2)
	assert.Contains(t, k8sClient.manifests[1], "head-bucket")
	assert.Contains(t, k8sClient.manifests[1], "create-bucket")
	assert.Contains(t, k8sClient.manifests[1], "secretKeyRef")
	assert.NotContains(t, k8sClient.manifests[1], "test-secret")
	assert.NotContains(t, k8sClient.manifests[1], "|| true")
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

func TestInstall_RejectsS3SecretReadFailure(t *testing.T) {
	helmClient := &mockHelmClient{}
	k8sClient := &mockK8sClient{secretErr: assert.AnError}
	cfg := &Config{
		Namespace: "seaweedfs",
		S3Enabled: true,
		AccessKey: "test-key",
		SecretKey: "test-secret",
	}

	err := Install(context.Background(), helmClient, k8sClient, cfg)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to read existing S3 credentials secret")
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
		AccessKey:      "test-key",
		SecretKey:      "test-secret",
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
	assert.Equal(t, true, s3Config["enableAuth"])
	assert.Equal(t, seaweedfsS3Secret, s3Config["existingConfigSecret"])
}

func TestBuildS3SecretManifest(t *testing.T) {
	cfg := &Config{
		Namespace: "seaweedfs",
		AccessKey: "test-key",
		SecretKey: "test-secret",
	}

	manifest, err := buildS3SecretManifest(cfg, nil)
	require.NoError(t, err)
	assert.False(t, strings.Contains(manifest, "test-secret"))

	var secret struct {
		Metadata struct {
			Name      string `json:"name"`
			Namespace string `json:"namespace"`
		} `json:"metadata"`
		Data map[string]string `json:"data"`
	}
	require.NoError(t, json.Unmarshal([]byte(manifest), &secret))
	assert.Equal(t, seaweedfsS3Secret, secret.Metadata.Name)
	assert.Equal(t, "seaweedfs", secret.Metadata.Namespace)

	encodedConfig, ok := secret.Data["seaweedfs_s3_config"]
	require.True(t, ok)
	decodedConfig, err := base64.StdEncoding.DecodeString(encodedConfig)
	require.NoError(t, err)
	assert.Contains(t, string(decodedConfig), `"accessKey":"test-key"`)
	assert.Contains(t, string(decodedConfig), `"secretKey":"test-secret"`)
}

func TestBuildS3SecretManifest_RequiresCredentials(t *testing.T) {
	_, err := buildS3SecretManifest(&Config{Namespace: "seaweedfs"}, nil)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "access_key and secret_key are required")
}

func TestMergeS3IdentityConfig_PreservesSupplementalIdentities(t *testing.T) {
	existing := []byte(`{
		"identities": [
			{
				"name": "asset-cache",
				"credentials": [{"accessKey": "cache-key", "secretKey": "cache-secret"}],
				"actions": ["Read", "Write", "List"]
			},
			{
				"name": "foundry-admin",
				"credentials": [{"accessKey": "old-key", "secretKey": "old-secret"}],
				"actions": ["Admin"]
			}
		],
		"extra": "preserved"
	}`)

	encoded, err := mergeS3IdentityConfig(existing, "new-key", "new-secret")
	require.NoError(t, err)

	var config struct {
		Identities []struct {
			Name        string `json:"name"`
			Credentials []struct {
				AccessKey string `json:"accessKey"`
				SecretKey string `json:"secretKey"`
			} `json:"credentials"`
		} `json:"identities"`
		Extra string `json:"extra"`
	}
	require.NoError(t, json.Unmarshal(encoded, &config))
	require.Len(t, config.Identities, 2)
	assert.Equal(t, "asset-cache", config.Identities[0].Name)
	assert.Equal(t, "cache-key", config.Identities[0].Credentials[0].AccessKey)
	assert.Equal(t, "foundry-admin", config.Identities[1].Name)
	assert.Equal(t, "new-key", config.Identities[1].Credentials[0].AccessKey)
	assert.Equal(t, "new-secret", config.Identities[1].Credentials[0].SecretKey)
	assert.Equal(t, "preserved", config.Extra)
	assert.NotContains(t, string(encoded), "old-secret")
}

func TestMergeS3IdentityConfig_RejectsInvalidExistingConfig(t *testing.T) {
	_, err := mergeS3IdentityConfig([]byte(`{"identities":`), "new-key", "new-secret")

	require.Error(t, err)
	assert.Contains(t, err.Error(), "existing SeaweedFS S3 configuration is invalid")
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
