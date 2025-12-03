package grafana

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

	assert.Equal(t, "8.8.2", config.Version)
	assert.Equal(t, "grafana", config.Namespace)
	assert.Equal(t, "admin", config.AdminUser)
	assert.Equal(t, "", config.AdminPassword)
	assert.Equal(t, "", config.StorageClass)
	assert.Equal(t, "5Gi", config.StorageSize)
	assert.Equal(t, "http://kube-prometheus-stack-prometheus.monitoring.svc.cluster.local:9090", config.PrometheusURL)
	assert.Equal(t, "http://loki-gateway.loki.svc.cluster.local:80", config.LokiURL)
	assert.False(t, config.IngressEnabled)
	assert.True(t, config.DefaultDashboardsEnabled)
	assert.True(t, config.SidecarEnabled)
	assert.NotNil(t, config.Values)
}

func TestParseConfig_Defaults(t *testing.T) {
	cfg := component.ComponentConfig{}

	config, err := ParseConfig(cfg)
	require.NoError(t, err)

	assert.Equal(t, "8.8.2", config.Version)
	assert.Equal(t, "grafana", config.Namespace)
	assert.Equal(t, "admin", config.AdminUser)
}

func TestParseConfig_CustomValues(t *testing.T) {
	cfg := component.ComponentConfig{
		"version":                    "8.5.0",
		"namespace":                  "custom-grafana",
		"admin_user":                 "superadmin",
		"admin_password":             "supersecret",
		"storage_class":              "fast-storage",
		"storage_size":               "20Gi",
		"prometheus_url":             "http://custom-prometheus:9090",
		"loki_url":                   "http://custom-loki:3100",
		"ingress_enabled":            true,
		"ingress_host":               "grafana.example.com",
		"default_dashboards_enabled": false,
		"sidecar_enabled":            false,
	}

	config, err := ParseConfig(cfg)
	require.NoError(t, err)

	assert.Equal(t, "8.5.0", config.Version)
	assert.Equal(t, "custom-grafana", config.Namespace)
	assert.Equal(t, "superadmin", config.AdminUser)
	assert.Equal(t, "supersecret", config.AdminPassword)
	assert.Equal(t, "fast-storage", config.StorageClass)
	assert.Equal(t, "20Gi", config.StorageSize)
	assert.Equal(t, "http://custom-prometheus:9090", config.PrometheusURL)
	assert.Equal(t, "http://custom-loki:3100", config.LokiURL)
	assert.True(t, config.IngressEnabled)
	assert.Equal(t, "grafana.example.com", config.IngressHost)
	assert.False(t, config.DefaultDashboardsEnabled)
	assert.False(t, config.SidecarEnabled)
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
	config := &Config{}

	err := config.Validate()
	assert.NoError(t, err)
}

func TestValidate_IngressEnabled_NoHost(t *testing.T) {
	config := &Config{
		IngressEnabled: true,
		IngressHost:    "",
	}

	err := config.Validate()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "ingress_host is required")
}

func TestValidate_IngressEnabled_WithHost(t *testing.T) {
	config := &Config{
		IngressEnabled: true,
		IngressHost:    "grafana.example.com",
	}

	err := config.Validate()
	assert.NoError(t, err)
}

func TestGetGrafanaEndpoint(t *testing.T) {
	config := &Config{
		Namespace: "grafana",
	}

	endpoint := config.GetGrafanaEndpoint()
	assert.Equal(t, "http://grafana.grafana.svc.cluster.local:80", endpoint)
}

func TestGetGrafanaEndpoint_CustomNamespace(t *testing.T) {
	config := &Config{
		Namespace: "custom-ns",
	}

	endpoint := config.GetGrafanaEndpoint()
	assert.Equal(t, "http://grafana.custom-ns.svc.cluster.local:80", endpoint)
}

func TestComponent_Name(t *testing.T) {
	comp := NewComponent(nil, nil)
	assert.Equal(t, "grafana", comp.Name())
}

func TestComponent_Dependencies(t *testing.T) {
	comp := NewComponent(nil, nil)
	deps := comp.Dependencies()

	require.Len(t, deps, 2)
	assert.Contains(t, deps, "prometheus")
	assert.Contains(t, deps, "loki")
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

func TestComponent_Status_GrafanaInstalled(t *testing.T) {
	helmClient := &mockHelmClient{
		listReleases: []helm.Release{
			{
				Name:       "grafana",
				Namespace:  "grafana",
				Status:     "deployed",
				AppVersion: "10.2.0",
			},
		},
	}

	comp := NewComponent(helmClient, nil)
	status, err := comp.Status(context.Background())

	assert.NoError(t, err)
	assert.True(t, status.Installed)
	assert.True(t, status.Healthy)
	assert.Equal(t, "10.2.0", status.Version)
}

func TestComponent_Status_GrafanaNotInstalled(t *testing.T) {
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

func TestComponent_Status_GrafanaFailed(t *testing.T) {
	helmClient := &mockHelmClient{
		listReleases: []helm.Release{
			{
				Name:       "grafana",
				Namespace:  "grafana",
				Status:     "failed",
				AppVersion: "10.2.0",
			},
		},
	}

	comp := NewComponent(helmClient, nil)
	status, err := comp.Status(context.Background())

	assert.NoError(t, err)
	assert.True(t, status.Installed)
	assert.False(t, status.Healthy)
}
