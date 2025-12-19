package seaweedfs

import (
	"context"
	"testing"
	"time"

	"github.com/catalystcommunity/foundry/v1/internal/component"
	"github.com/catalystcommunity/foundry/v1/internal/helm"
	"github.com/catalystcommunity/foundry/v1/internal/k8s"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDefaultConfig(t *testing.T) {
	config := DefaultConfig()

	assert.Equal(t, "4.0.401", config.Version)
	assert.Equal(t, "seaweedfs", config.Namespace)
	assert.Equal(t, 1, config.MasterReplicas)
	assert.Equal(t, 1, config.VolumeReplicas)
	assert.Equal(t, 1, config.FilerReplicas)
	assert.Equal(t, "", config.StorageClass)
	assert.Equal(t, "50Gi", config.StorageSize)
	assert.True(t, config.S3Enabled)
	assert.Equal(t, 8333, config.S3Port)
	assert.Equal(t, "", config.AccessKey)
	assert.Equal(t, "", config.SecretKey)
	assert.Empty(t, config.Buckets)
	assert.NotNil(t, config.Values)
}

func TestParseConfig_Defaults(t *testing.T) {
	cfg := component.ComponentConfig{}

	config, err := ParseConfig(cfg)
	require.NoError(t, err)

	assert.Equal(t, "4.0.401", config.Version)
	assert.Equal(t, "seaweedfs", config.Namespace)
	assert.Equal(t, 1, config.MasterReplicas)
	assert.Equal(t, 1, config.VolumeReplicas)
	assert.Equal(t, 1, config.FilerReplicas)
	// Credentials should be auto-generated
	assert.NotEmpty(t, config.AccessKey)
	assert.NotEmpty(t, config.SecretKey)
	assert.Len(t, config.AccessKey, 32) // 16 bytes = 32 hex chars
	assert.Len(t, config.SecretKey, 64) // 32 bytes = 64 hex chars
}

func TestParseConfig_CustomValues(t *testing.T) {
	cfg := component.ComponentConfig{
		"version":         "3.73.0",
		"namespace":       "custom-seaweedfs",
		"master_replicas": 3,
		"volume_replicas": 3,
		"filer_replicas":  2,
		"storage_class":   "longhorn",
		"storage_size":    "100Gi",
		"s3_enabled":      true,
		"s3_port":         9333,
		"access_key":      "my-access-key",
		"secret_key":      "my-secret-key",
		"buckets":         []interface{}{"loki", "velero"},
	}

	config, err := ParseConfig(cfg)
	require.NoError(t, err)

	assert.Equal(t, "3.73.0", config.Version)
	assert.Equal(t, "custom-seaweedfs", config.Namespace)
	assert.Equal(t, 3, config.MasterReplicas)
	assert.Equal(t, 3, config.VolumeReplicas)
	assert.Equal(t, 2, config.FilerReplicas)
	assert.Equal(t, "longhorn", config.StorageClass)
	assert.Equal(t, "100Gi", config.StorageSize)
	assert.True(t, config.S3Enabled)
	assert.Equal(t, 9333, config.S3Port)
	assert.Equal(t, "my-access-key", config.AccessKey)
	assert.Equal(t, "my-secret-key", config.SecretKey)
	assert.Len(t, config.Buckets, 2)
	assert.Contains(t, config.Buckets, "loki")
	assert.Contains(t, config.Buckets, "velero")
}

func TestParseConfig_WithCustomHelmValues(t *testing.T) {
	cfg := component.ComponentConfig{
		"values": map[string]interface{}{
			"custom": "value",
			"nested": map[string]interface{}{
				"key": "value",
			},
		},
	}

	config, err := ParseConfig(cfg)
	require.NoError(t, err)

	require.NotNil(t, config.Values)
	assert.Equal(t, "value", config.Values["custom"])
	assert.NotNil(t, config.Values["nested"])
}

func TestValidate_Success(t *testing.T) {
	config := &Config{
		MasterReplicas: 1,
		VolumeReplicas: 1,
		FilerReplicas:  1,
		S3Enabled:      true,
		S3Port:         8333,
	}

	err := config.Validate()
	assert.NoError(t, err)
}

func TestValidate_MultiReplica_Success(t *testing.T) {
	config := &Config{
		MasterReplicas: 3,
		VolumeReplicas: 3,
		FilerReplicas:  3,
		S3Enabled:      true,
		S3Port:         8333,
	}

	err := config.Validate()
	assert.NoError(t, err)
}

func TestValidate_MasterReplicasTooLow(t *testing.T) {
	config := &Config{
		MasterReplicas: 0,
		VolumeReplicas: 1,
		FilerReplicas:  1,
	}

	err := config.Validate()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "master_replicas must be at least 1")
}

func TestValidate_VolumeReplicasTooLow(t *testing.T) {
	config := &Config{
		MasterReplicas: 1,
		VolumeReplicas: 0,
		FilerReplicas:  1,
	}

	err := config.Validate()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "volume_replicas must be at least 1")
}

func TestValidate_FilerReplicasTooLow(t *testing.T) {
	config := &Config{
		MasterReplicas: 1,
		VolumeReplicas: 1,
		FilerReplicas:  0,
	}

	err := config.Validate()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "filer_replicas must be at least 1")
}

func TestValidate_InvalidS3Port(t *testing.T) {
	config := &Config{
		MasterReplicas: 1,
		VolumeReplicas: 1,
		FilerReplicas:  1,
		S3Enabled:      true,
		S3Port:         0,
	}

	err := config.Validate()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "s3_port must be between 1 and 65535")
}

func TestValidate_S3PortTooHigh(t *testing.T) {
	config := &Config{
		MasterReplicas: 1,
		VolumeReplicas: 1,
		FilerReplicas:  1,
		S3Enabled:      true,
		S3Port:         70000,
	}

	err := config.Validate()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "s3_port must be between 1 and 65535")
}

func TestGetS3Endpoint(t *testing.T) {
	config := &Config{
		Namespace: "seaweedfs",
		S3Port:    8333,
	}

	endpoint := config.GetS3Endpoint()
	assert.Equal(t, "http://seaweedfs-s3.seaweedfs.svc.cluster.local:8333", endpoint)
}

func TestGetS3Endpoint_CustomNamespace(t *testing.T) {
	config := &Config{
		Namespace: "custom-ns",
		S3Port:    8333,
	}

	endpoint := config.GetS3Endpoint()
	assert.Equal(t, "http://seaweedfs-s3.custom-ns.svc.cluster.local:8333", endpoint)
}

func TestGetS3Endpoint_CustomPort(t *testing.T) {
	config := &Config{
		Namespace: "seaweedfs",
		S3Port:    9333,
	}

	endpoint := config.GetS3Endpoint()
	assert.Equal(t, "http://seaweedfs-s3.seaweedfs.svc.cluster.local:9333", endpoint)
}

func TestGetMasterEndpoint(t *testing.T) {
	config := &Config{
		Namespace: "seaweedfs",
	}

	endpoint := config.GetMasterEndpoint()
	assert.Equal(t, "http://seaweedfs-master.seaweedfs.svc.cluster.local:9333", endpoint)
}

func TestGetFilerEndpoint(t *testing.T) {
	config := &Config{
		Namespace: "seaweedfs",
	}

	endpoint := config.GetFilerEndpoint()
	assert.Equal(t, "http://seaweedfs-filer.seaweedfs.svc.cluster.local:8888", endpoint)
}

func TestComponent_Name(t *testing.T) {
	comp := NewComponent(nil, nil)
	assert.Equal(t, "seaweedfs", comp.Name())
}

func TestComponent_Dependencies(t *testing.T) {
	comp := NewComponent(nil, nil)
	deps := comp.Dependencies()

	require.Len(t, deps, 1)
	assert.Contains(t, deps, "storage")
}

func TestComponent_Install_NilHelmClient(t *testing.T) {
	comp := NewComponent(nil, nil)
	err := comp.Install(context.Background(), component.ComponentConfig{})

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "helm client cannot be nil")
}

func TestComponent_Upgrade_NotImplemented(t *testing.T) {
	comp := NewComponent(nil, nil)
	err := comp.Upgrade(context.Background(), component.ComponentConfig{})

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not yet implemented")
}

func TestComponent_Uninstall_NotImplemented(t *testing.T) {
	comp := NewComponent(nil, nil)
	err := comp.Uninstall(context.Background())

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not yet implemented")
}

func TestComponent_Status_NoHelmClient(t *testing.T) {
	comp := NewComponent(nil, nil)
	status, err := comp.Status(context.Background())

	assert.NoError(t, err)
	assert.False(t, status.Installed)
	assert.False(t, status.Healthy)
	assert.Contains(t, status.Message, "not initialized")
}

// mockHelmClient is a mock implementation of HelmClient for testing
type mockHelmClient struct {
	addRepoErr      error
	installErr      error
	upgradeErr      error
	listReleases    []helm.Release
	listErr         error
	reposAdded      []helm.RepoAddOptions
	chartsInstalled []helm.InstallOptions
	upgradeCalls    []helm.UpgradeOptions
	uninstallCalls  []helm.UninstallOptions
}

func (m *mockHelmClient) AddRepo(ctx context.Context, opts helm.RepoAddOptions) error {
	m.reposAdded = append(m.reposAdded, opts)
	return m.addRepoErr
}

func (m *mockHelmClient) Install(ctx context.Context, opts helm.InstallOptions) error {
	m.chartsInstalled = append(m.chartsInstalled, opts)
	return m.installErr
}

func (m *mockHelmClient) Upgrade(ctx context.Context, opts helm.UpgradeOptions) error {
	m.upgradeCalls = append(m.upgradeCalls, opts)
	return m.upgradeErr
}

func (m *mockHelmClient) List(ctx context.Context, namespace string) ([]helm.Release, error) {
	return m.listReleases, m.listErr
}

func (m *mockHelmClient) Uninstall(ctx context.Context, opts helm.UninstallOptions) error {
	m.uninstallCalls = append(m.uninstallCalls, opts)
	return nil
}

// mockK8sClient is a mock implementation of K8sClient for testing
type mockK8sClient struct {
	pods                       []*k8s.Pod
	podsErr                    error
	applyManifestErr           error
	deleteJobErr               error
	waitJobErr                 error
	serviceMonitorCRDExists    bool
	serviceMonitorCRDExistsErr error
}

func (m *mockK8sClient) GetPods(ctx context.Context, namespace string) ([]*k8s.Pod, error) {
	return m.pods, m.podsErr
}

func (m *mockK8sClient) ApplyManifest(ctx context.Context, manifest string) error {
	return m.applyManifestErr
}

func (m *mockK8sClient) DeleteJob(ctx context.Context, namespace, name string) error {
	return m.deleteJobErr
}

func (m *mockK8sClient) WaitForJobComplete(ctx context.Context, namespace, name string, timeout time.Duration) error {
	return m.waitJobErr
}

func (m *mockK8sClient) ServiceMonitorCRDExists(ctx context.Context) (bool, error) {
	return m.serviceMonitorCRDExists, m.serviceMonitorCRDExistsErr
}

func TestComponent_Status_SeaweedFSInstalled(t *testing.T) {
	helmClient := &mockHelmClient{
		listReleases: []helm.Release{
			{
				Name:       "seaweedfs",
				Namespace:  "seaweedfs",
				Status:     "deployed",
				AppVersion: "4.0.401",
			},
		},
	}

	comp := NewComponent(helmClient, nil)
	status, err := comp.Status(context.Background())

	assert.NoError(t, err)
	assert.True(t, status.Installed)
	assert.True(t, status.Healthy)
	assert.Equal(t, "4.0.401", status.Version)
}

func TestComponent_Status_SeaweedFSNotInstalled(t *testing.T) {
	helmClient := &mockHelmClient{
		listReleases: []helm.Release{},
	}

	comp := NewComponent(helmClient, nil)
	status, err := comp.Status(context.Background())

	assert.NoError(t, err)
	assert.False(t, status.Installed)
	assert.False(t, status.Healthy)
	assert.Contains(t, status.Message, "not found")
}

func TestComponent_Status_SeaweedFSFailed(t *testing.T) {
	helmClient := &mockHelmClient{
		listReleases: []helm.Release{
			{
				Name:       "seaweedfs",
				Namespace:  "seaweedfs",
				Status:     "failed",
				AppVersion: "4.0.401",
			},
		},
	}

	comp := NewComponent(helmClient, nil)
	status, err := comp.Status(context.Background())

	assert.NoError(t, err)
	assert.True(t, status.Installed)
	assert.False(t, status.Healthy)
}

func TestGetConnectionInfo(t *testing.T) {
	cfg := &Config{
		Namespace: "seaweedfs",
		S3Port:    8333,
		AccessKey: "access-key",
		SecretKey: "secret-key",
	}

	info := GetConnectionInfo(cfg)

	assert.Equal(t, "seaweedfs-s3.seaweedfs.svc.cluster.local:8333", info.Endpoint)
	assert.Equal(t, "access-key", info.AccessKey)
	assert.Equal(t, "secret-key", info.SecretKey)
	assert.False(t, info.UseSSL)
	assert.Equal(t, "us-east-1", info.Region)
}

func TestGenerateRandomKey(t *testing.T) {
	key1 := generateRandomKey(16)
	key2 := generateRandomKey(16)

	// Keys should be 32 chars (16 bytes * 2 for hex encoding)
	assert.Len(t, key1, 32)
	assert.Len(t, key2, 32)

	// Keys should be different
	assert.NotEqual(t, key1, key2)
}

func TestGenerateRandomKey_DifferentLength(t *testing.T) {
	key := generateRandomKey(32)

	// Keys should be 64 chars (32 bytes * 2 for hex encoding)
	assert.Len(t, key, 64)
}
