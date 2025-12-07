package garage

import (
	"context"
	"testing"

	"github.com/catalystcommunity/foundry/v1/internal/component"
	"github.com/catalystcommunity/foundry/v1/internal/helm"
	"github.com/catalystcommunity/foundry/v1/internal/k8s"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDefaultConfig(t *testing.T) {
	config := DefaultConfig()

	assert.Equal(t, "1.0.1", config.Version)
	assert.Equal(t, "garage", config.Namespace)
	assert.Equal(t, 1, config.ReplicationFactor)
	assert.Equal(t, 1, config.Replicas)
	assert.Equal(t, "", config.StorageClass)
	assert.Equal(t, "50Gi", config.StorageSize)
	assert.Equal(t, "garage", config.S3Region)
	assert.Equal(t, "", config.AdminKey)
	assert.Equal(t, "", config.AdminSecret)
	assert.Empty(t, config.Buckets)
	assert.NotNil(t, config.Values)
}

func TestParseConfig_Defaults(t *testing.T) {
	cfg := component.ComponentConfig{}

	config, err := ParseConfig(cfg)
	require.NoError(t, err)

	assert.Equal(t, "1.0.1", config.Version)
	assert.Equal(t, "garage", config.Namespace)
	assert.Equal(t, 1, config.ReplicationFactor)
	assert.Equal(t, 1, config.Replicas)
	// Admin credentials should be auto-generated
	assert.NotEmpty(t, config.AdminKey)
	assert.NotEmpty(t, config.AdminSecret)
	assert.Len(t, config.AdminKey, 64)   // 32 bytes = 64 hex chars
	assert.Len(t, config.AdminSecret, 64)
}

func TestParseConfig_CustomValues(t *testing.T) {
	cfg := component.ComponentConfig{
		"version":            "1.0.2",
		"namespace":          "custom-garage",
		"replication_factor": 3,
		"replicas":           3,
		"storage_class":      "longhorn",
		"storage_size":       "100Gi",
		"s3_region":          "us-east-1",
		"admin_key":          "my-admin-key",
		"admin_secret":       "my-admin-secret",
		"buckets":            []interface{}{"loki", "velero"},
	}

	config, err := ParseConfig(cfg)
	require.NoError(t, err)

	assert.Equal(t, "1.0.2", config.Version)
	assert.Equal(t, "custom-garage", config.Namespace)
	assert.Equal(t, 3, config.ReplicationFactor)
	assert.Equal(t, 3, config.Replicas)
	assert.Equal(t, "longhorn", config.StorageClass)
	assert.Equal(t, "100Gi", config.StorageSize)
	assert.Equal(t, "us-east-1", config.S3Region)
	assert.Equal(t, "my-admin-key", config.AdminKey)
	assert.Equal(t, "my-admin-secret", config.AdminSecret)
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
		ReplicationFactor: 1,
		Replicas:          1,
	}

	err := config.Validate()
	assert.NoError(t, err)
}

func TestValidate_MultiReplica_Success(t *testing.T) {
	config := &Config{
		ReplicationFactor: 3,
		Replicas:          3,
	}

	err := config.Validate()
	assert.NoError(t, err)
}

func TestValidate_ReplicationFactorTooLow(t *testing.T) {
	config := &Config{
		ReplicationFactor: 0,
		Replicas:          3,
	}

	err := config.Validate()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "replication_factor must be at least 1")
}

func TestValidate_ReplicasTooLow(t *testing.T) {
	config := &Config{
		ReplicationFactor: 1,
		Replicas:          0,
	}

	err := config.Validate()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "replicas must be at least 1")
}

func TestValidate_ReplicationFactorGreaterThanReplicas(t *testing.T) {
	config := &Config{
		ReplicationFactor: 3,
		Replicas:          2,
	}

	err := config.Validate()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "cannot be greater than replicas")
}

func TestGetEndpoint(t *testing.T) {
	config := &Config{
		Namespace: "garage",
	}

	endpoint := config.GetEndpoint()
	assert.Equal(t, "http://garage.garage.svc.cluster.local:3900", endpoint)
}

func TestGetEndpoint_CustomNamespace(t *testing.T) {
	config := &Config{
		Namespace: "custom-ns",
	}

	endpoint := config.GetEndpoint()
	assert.Equal(t, "http://garage.custom-ns.svc.cluster.local:3900", endpoint)
}

func TestGetAdminEndpoint(t *testing.T) {
	config := &Config{
		Namespace: "garage",
	}

	endpoint := config.GetAdminEndpoint()
	assert.Equal(t, "http://garage.garage.svc.cluster.local:3903", endpoint)
}

func TestGetRPCEndpoint(t *testing.T) {
	config := &Config{
		Namespace: "garage",
	}

	endpoint := config.GetRPCEndpoint()
	assert.Equal(t, "http://garage.garage.svc.cluster.local:3901", endpoint)
}

func TestComponent_Name(t *testing.T) {
	comp := NewComponent(nil, nil)
	assert.Equal(t, "garage", comp.Name())
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
	listReleases    []helm.Release
	listErr         error
	reposAdded      []helm.RepoAddOptions
	chartsInstalled []helm.InstallOptions
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
	return nil
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
	pods    []*k8s.Pod
	podsErr error
}

func (m *mockK8sClient) GetPods(ctx context.Context, namespace string) ([]*k8s.Pod, error) {
	return m.pods, m.podsErr
}

func TestComponent_Status_GarageInstalled(t *testing.T) {
	helmClient := &mockHelmClient{
		listReleases: []helm.Release{
			{
				Name:       "garage",
				Namespace:  "garage",
				Status:     "deployed",
				AppVersion: "1.0.1",
			},
		},
	}

	comp := NewComponent(helmClient, nil)
	status, err := comp.Status(context.Background())

	assert.NoError(t, err)
	assert.True(t, status.Installed)
	assert.True(t, status.Healthy)
	assert.Equal(t, "1.0.1", status.Version)
}

func TestComponent_Status_GarageNotInstalled(t *testing.T) {
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

func TestComponent_Status_GarageFailed(t *testing.T) {
	helmClient := &mockHelmClient{
		listReleases: []helm.Release{
			{
				Name:       "garage",
				Namespace:  "garage",
				Status:     "failed",
				AppVersion: "1.0.1",
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
		Namespace:   "garage",
		AdminKey:    "access-key",
		AdminSecret: "secret-key",
		S3Region:    "garage",
	}

	info := GetConnectionInfo(cfg)

	assert.Equal(t, "garage.garage.svc.cluster.local:3900", info.Endpoint)
	assert.Equal(t, "access-key", info.AccessKey)
	assert.Equal(t, "secret-key", info.SecretKey)
	assert.False(t, info.UseSSL)
	assert.Equal(t, "garage", info.Region)
}

func TestGenerateRandomKey(t *testing.T) {
	key1 := generateRandomKey(32)
	key2 := generateRandomKey(32)

	// Keys should be 64 chars (32 bytes * 2 for hex encoding)
	assert.Len(t, key1, 64)
	assert.Len(t, key2, 64)

	// Keys should be different
	assert.NotEqual(t, key1, key2)
}
