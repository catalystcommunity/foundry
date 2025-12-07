package loki

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

	assert.Equal(t, "6.23.0", config.Version)
	assert.Equal(t, "loki", config.Namespace)
	assert.Equal(t, "SingleBinary", config.DeploymentMode)
	assert.Equal(t, BackendS3, config.StorageBackend)
	assert.Equal(t, 30, config.RetentionDays)
	assert.Equal(t, "", config.StorageClass)
	assert.Equal(t, "10Gi", config.StorageSize)
	assert.Equal(t, "http://garage.garage.svc.cluster.local:3900", config.S3Endpoint)
	assert.Equal(t, "loki", config.S3Bucket)
	assert.Equal(t, "garage", config.S3Region)
	assert.True(t, config.PromtailEnabled)
	assert.False(t, config.GrafanaAgentEnabled)
	assert.False(t, config.IngressEnabled)
	assert.NotNil(t, config.Values)
}

func TestParseConfig_Defaults(t *testing.T) {
	cfg := component.ComponentConfig{}

	config, err := ParseConfig(cfg)
	require.NoError(t, err)

	assert.Equal(t, "6.23.0", config.Version)
	assert.Equal(t, "loki", config.Namespace)
	assert.Equal(t, BackendS3, config.StorageBackend)
}

func TestParseConfig_CustomValues(t *testing.T) {
	cfg := component.ComponentConfig{
		"version":               "6.20.0",
		"namespace":             "custom-loki",
		"deployment_mode":       "SimpleScalable",
		"storage_backend":       "filesystem",
		"retention_days":        60,
		"storage_class":         "fast-storage",
		"storage_size":          "50Gi",
		"promtail_enabled":      false,
		"grafana_agent_enabled": true,
		"ingress_enabled":       true,
		"ingress_host":          "loki.example.com",
	}

	config, err := ParseConfig(cfg)
	require.NoError(t, err)

	assert.Equal(t, "6.20.0", config.Version)
	assert.Equal(t, "custom-loki", config.Namespace)
	assert.Equal(t, "SimpleScalable", config.DeploymentMode)
	assert.Equal(t, BackendFilesystem, config.StorageBackend)
	assert.Equal(t, 60, config.RetentionDays)
	assert.Equal(t, "fast-storage", config.StorageClass)
	assert.Equal(t, "50Gi", config.StorageSize)
	assert.False(t, config.PromtailEnabled)
	assert.True(t, config.GrafanaAgentEnabled)
	assert.True(t, config.IngressEnabled)
	assert.Equal(t, "loki.example.com", config.IngressHost)
}

func TestParseConfig_S3Config(t *testing.T) {
	cfg := component.ComponentConfig{
		"storage_backend": "s3",
		"s3_endpoint":     "http://custom-minio:9000",
		"s3_bucket":       "custom-bucket",
		"s3_access_key":   "myaccesskey",
		"s3_secret_key":   "mysecretkey",
		"s3_region":       "eu-west-1",
	}

	config, err := ParseConfig(cfg)
	require.NoError(t, err)

	assert.Equal(t, BackendS3, config.StorageBackend)
	assert.Equal(t, "http://custom-minio:9000", config.S3Endpoint)
	assert.Equal(t, "custom-bucket", config.S3Bucket)
	assert.Equal(t, "myaccesskey", config.S3AccessKey)
	assert.Equal(t, "mysecretkey", config.S3SecretKey)
	assert.Equal(t, "eu-west-1", config.S3Region)
}

func TestParseConfig_WithCustomValues(t *testing.T) {
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
		RetentionDays:  30,
		StorageBackend: BackendFilesystem,
	}

	err := config.Validate()
	assert.NoError(t, err)
}

func TestValidate_S3_Success(t *testing.T) {
	config := &Config{
		RetentionDays:  30,
		StorageBackend: BackendS3,
		S3Endpoint:     "http://garage:3900",
		S3Bucket:       "loki",
	}

	err := config.Validate()
	assert.NoError(t, err)
}

func TestValidate_InvalidRetentionDays(t *testing.T) {
	config := &Config{
		RetentionDays:  0,
		StorageBackend: BackendFilesystem,
	}

	err := config.Validate()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "retention_days must be at least 1")
}

func TestValidate_S3_MissingEndpoint(t *testing.T) {
	config := &Config{
		RetentionDays:  30,
		StorageBackend: BackendS3,
		S3Endpoint:     "",
		S3Bucket:       "loki",
	}

	err := config.Validate()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "s3_endpoint is required")
}

func TestValidate_S3_MissingBucket(t *testing.T) {
	config := &Config{
		RetentionDays:  30,
		StorageBackend: BackendS3,
		S3Endpoint:     "http://garage:3900",
		S3Bucket:       "",
	}

	err := config.Validate()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "s3_bucket is required")
}

func TestValidate_IngressEnabled_NoHost(t *testing.T) {
	config := &Config{
		RetentionDays:  30,
		StorageBackend: BackendFilesystem,
		IngressEnabled: true,
		IngressHost:    "",
	}

	err := config.Validate()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "ingress_host is required")
}

func TestValidate_IngressEnabled_WithHost(t *testing.T) {
	config := &Config{
		RetentionDays:  30,
		StorageBackend: BackendFilesystem,
		IngressEnabled: true,
		IngressHost:    "loki.example.com",
	}

	err := config.Validate()
	assert.NoError(t, err)
}

func TestGetLokiEndpoint(t *testing.T) {
	config := &Config{
		Namespace: "loki",
	}

	endpoint := config.GetLokiEndpoint()
	assert.Equal(t, "http://loki-gateway.loki.svc.cluster.local:80", endpoint)
}

func TestGetLokiEndpoint_CustomNamespace(t *testing.T) {
	config := &Config{
		Namespace: "custom-ns",
	}

	endpoint := config.GetLokiEndpoint()
	assert.Equal(t, "http://loki-gateway.custom-ns.svc.cluster.local:80", endpoint)
}

func TestGetLokiPushEndpoint(t *testing.T) {
	config := &Config{
		Namespace: "loki",
	}

	endpoint := config.GetLokiPushEndpoint()
	assert.Equal(t, "http://loki-gateway.loki.svc.cluster.local:80/loki/api/v1/push", endpoint)
}

func TestComponent_Name(t *testing.T) {
	comp := NewComponent(nil, nil)
	assert.Equal(t, "loki", comp.Name())
}

func TestComponent_Dependencies(t *testing.T) {
	comp := NewComponent(nil, nil)
	deps := comp.Dependencies()

	require.Len(t, deps, 2)
	assert.Contains(t, deps, "storage")
	assert.Contains(t, deps, "garage")
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

func TestComponent_Status_LokiInstalled(t *testing.T) {
	helmClient := &mockHelmClient{
		listReleases: []helm.Release{
			{
				Name:       "loki",
				Namespace:  "loki",
				Status:     "deployed",
				AppVersion: "3.0.0",
			},
		},
	}

	comp := NewComponent(helmClient, nil)
	status, err := comp.Status(context.Background())

	assert.NoError(t, err)
	assert.True(t, status.Installed)
	assert.True(t, status.Healthy)
	assert.Equal(t, "3.0.0", status.Version)
}

func TestComponent_Status_LokiNotInstalled(t *testing.T) {
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

func TestComponent_Status_LokiFailed(t *testing.T) {
	helmClient := &mockHelmClient{
		listReleases: []helm.Release{
			{
				Name:       "loki",
				Namespace:  "loki",
				Status:     "failed",
				AppVersion: "3.0.0",
			},
		},
	}

	comp := NewComponent(helmClient, nil)
	status, err := comp.Status(context.Background())

	assert.NoError(t, err)
	assert.True(t, status.Installed)
	assert.False(t, status.Healthy)
}
