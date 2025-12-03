package minio

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

	assert.Equal(t, "5.2.0", config.Version)
	assert.Equal(t, "minio", config.Namespace)
	assert.Equal(t, ModeStandalone, config.Mode)
	assert.Equal(t, 1, config.Replicas)
	assert.Equal(t, "minioadmin", config.RootUser)
	assert.Equal(t, "", config.RootPassword)
	assert.Equal(t, "", config.StorageClass)
	assert.Equal(t, "10Gi", config.StorageSize)
	assert.Empty(t, config.Buckets)
	assert.False(t, config.IngressEnabled)
	assert.False(t, config.TLSEnabled)
	assert.NotNil(t, config.Values)
}

func TestParseConfig_Defaults(t *testing.T) {
	cfg := component.ComponentConfig{}

	config, err := ParseConfig(cfg)
	require.NoError(t, err)

	assert.Equal(t, "5.2.0", config.Version)
	assert.Equal(t, "minio", config.Namespace)
	assert.Equal(t, ModeStandalone, config.Mode)
}

func TestParseConfig_CustomValues(t *testing.T) {
	cfg := component.ComponentConfig{
		"version":       "5.3.0",
		"namespace":     "custom-minio",
		"mode":          "distributed",
		"replicas":      4,
		"root_user":     "admin",
		"root_password": "secret123",
		"storage_class": "local-path",
		"storage_size":  "50Gi",
		"buckets":       []interface{}{"loki", "velero", "backups"},
		"ingress_enabled": true,
		"ingress_host":    "minio.example.com",
		"tls_enabled":     true,
	}

	config, err := ParseConfig(cfg)
	require.NoError(t, err)

	assert.Equal(t, "5.3.0", config.Version)
	assert.Equal(t, "custom-minio", config.Namespace)
	assert.Equal(t, ModeDistributed, config.Mode)
	assert.Equal(t, 4, config.Replicas)
	assert.Equal(t, "admin", config.RootUser)
	assert.Equal(t, "secret123", config.RootPassword)
	assert.Equal(t, "local-path", config.StorageClass)
	assert.Equal(t, "50Gi", config.StorageSize)
	assert.Len(t, config.Buckets, 3)
	assert.Contains(t, config.Buckets, "loki")
	assert.Contains(t, config.Buckets, "velero")
	assert.Contains(t, config.Buckets, "backups")
	assert.True(t, config.IngressEnabled)
	assert.Equal(t, "minio.example.com", config.IngressHost)
	assert.True(t, config.TLSEnabled)
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

func TestValidate_StandaloneMode(t *testing.T) {
	config := &Config{
		Mode:     ModeStandalone,
		Replicas: 1,
	}

	err := config.Validate()
	assert.NoError(t, err)
}

func TestValidate_DistributedMode_Success(t *testing.T) {
	config := &Config{
		Mode:     ModeDistributed,
		Replicas: 4,
	}

	err := config.Validate()
	assert.NoError(t, err)
}

func TestValidate_DistributedMode_TooFewReplicas(t *testing.T) {
	config := &Config{
		Mode:     ModeDistributed,
		Replicas: 2,
	}

	err := config.Validate()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "at least 4 replicas")
}

func TestValidate_UnsupportedMode(t *testing.T) {
	config := &Config{
		Mode: DeploymentMode("invalid"),
	}

	err := config.Validate()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "unsupported deployment mode")
}

func TestValidate_IngressEnabled_NoHost(t *testing.T) {
	config := &Config{
		Mode:           ModeStandalone,
		IngressEnabled: true,
		IngressHost:    "",
	}

	err := config.Validate()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "ingress_host is required")
}

func TestValidate_IngressEnabled_WithHost(t *testing.T) {
	config := &Config{
		Mode:           ModeStandalone,
		IngressEnabled: true,
		IngressHost:    "minio.example.com",
	}

	err := config.Validate()
	assert.NoError(t, err)
}

func TestGetEndpoint(t *testing.T) {
	config := &Config{
		Namespace: "minio",
	}

	endpoint := config.GetEndpoint()
	assert.Equal(t, "http://minio.minio.svc.cluster.local:9000", endpoint)
}

func TestGetEndpoint_CustomNamespace(t *testing.T) {
	config := &Config{
		Namespace: "custom-ns",
	}

	endpoint := config.GetEndpoint()
	assert.Equal(t, "http://minio.custom-ns.svc.cluster.local:9000", endpoint)
}

func TestGetConsoleEndpoint(t *testing.T) {
	config := &Config{
		Namespace: "minio",
	}

	endpoint := config.GetConsoleEndpoint()
	assert.Equal(t, "http://minio-console.minio.svc.cluster.local:9001", endpoint)
}

func TestComponent_Name(t *testing.T) {
	comp := NewComponent(nil, nil)
	assert.Equal(t, "minio", comp.Name())
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

func TestComponent_Status_MinIOInstalled(t *testing.T) {
	helmClient := &mockHelmClient{
		listReleases: []helm.Release{
			{
				Name:       "minio",
				Namespace:  "minio",
				Status:     "deployed",
				AppVersion: "2024.1.1",
			},
		},
	}

	comp := NewComponent(helmClient, nil)
	status, err := comp.Status(context.Background())

	assert.NoError(t, err)
	assert.True(t, status.Installed)
	assert.True(t, status.Healthy)
	assert.Equal(t, "2024.1.1", status.Version)
}

func TestComponent_Status_MinIONotInstalled(t *testing.T) {
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

func TestComponent_Status_MinIOFailed(t *testing.T) {
	helmClient := &mockHelmClient{
		listReleases: []helm.Release{
			{
				Name:       "minio",
				Namespace:  "minio",
				Status:     "failed",
				AppVersion: "2024.1.1",
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
		Namespace:    "minio",
		RootUser:     "admin",
		RootPassword: "secret",
		TLSEnabled:   true,
	}

	info := GetConnectionInfo(cfg)

	assert.Equal(t, "minio.minio.svc.cluster.local:9000", info.Endpoint)
	assert.Equal(t, "admin", info.AccessKey)
	assert.Equal(t, "secret", info.SecretKey)
	assert.True(t, info.UseSSL)
	assert.Equal(t, "us-east-1", info.Region)
}
