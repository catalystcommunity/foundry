package prometheus

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

	assert.Equal(t, "67.4.0", config.Version)
	assert.Equal(t, "monitoring", config.Namespace)
	assert.Equal(t, 15, config.RetentionDays)
	assert.Equal(t, "10GB", config.RetentionSize) // Must end with 'B' per Prometheus CRD spec
	assert.Equal(t, "", config.StorageClass)
	assert.Equal(t, "20Gi", config.StorageSize)
	assert.True(t, config.AlertmanagerEnabled)
	assert.False(t, config.GrafanaEnabled)
	assert.True(t, config.NodeExporterEnabled)
	assert.True(t, config.KubeStateMetricsEnabled)
	assert.Equal(t, "30s", config.ScrapeInterval)
	assert.False(t, config.IngressEnabled)
	assert.NotNil(t, config.Values)
}

func TestParseConfig_Defaults(t *testing.T) {
	cfg := component.ComponentConfig{}

	config, err := ParseConfig(cfg)
	require.NoError(t, err)

	assert.Equal(t, "67.4.0", config.Version)
	assert.Equal(t, "monitoring", config.Namespace)
	assert.Equal(t, 15, config.RetentionDays)
}

func TestParseConfig_CustomValues(t *testing.T) {
	cfg := component.ComponentConfig{
		"version":                   "66.0.0",
		"namespace":                 "custom-monitoring",
		"retention_days":            30,
		"retention_size":            "50Gi",
		"storage_class":             "fast-storage",
		"storage_size":              "100Gi",
		"alertmanager_enabled":      false,
		"grafana_enabled":           true,
		"node_exporter_enabled":     false,
		"kube_state_metrics_enabled": false,
		"scrape_interval":           "15s",
		"ingress_enabled":           true,
		"ingress_host":              "prometheus.example.com",
	}

	config, err := ParseConfig(cfg)
	require.NoError(t, err)

	assert.Equal(t, "66.0.0", config.Version)
	assert.Equal(t, "custom-monitoring", config.Namespace)
	assert.Equal(t, 30, config.RetentionDays)
	assert.Equal(t, "50Gi", config.RetentionSize)
	assert.Equal(t, "fast-storage", config.StorageClass)
	assert.Equal(t, "100Gi", config.StorageSize)
	assert.False(t, config.AlertmanagerEnabled)
	assert.True(t, config.GrafanaEnabled)
	assert.False(t, config.NodeExporterEnabled)
	assert.False(t, config.KubeStateMetricsEnabled)
	assert.Equal(t, "15s", config.ScrapeInterval)
	assert.True(t, config.IngressEnabled)
	assert.Equal(t, "prometheus.example.com", config.IngressHost)
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
		RetentionDays: 15,
	}

	err := config.Validate()
	assert.NoError(t, err)
}

func TestValidate_InvalidRetentionDays(t *testing.T) {
	config := &Config{
		RetentionDays: 0,
	}

	err := config.Validate()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "retention_days must be at least 1")
}

func TestValidate_IngressEnabled_NoHost(t *testing.T) {
	config := &Config{
		RetentionDays:  15,
		IngressEnabled: true,
		IngressHost:    "",
	}

	err := config.Validate()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "ingress_host is required")
}

func TestValidate_IngressEnabled_WithHost(t *testing.T) {
	config := &Config{
		RetentionDays:  15,
		IngressEnabled: true,
		IngressHost:    "prometheus.example.com",
	}

	err := config.Validate()
	assert.NoError(t, err)
}

func TestGetPrometheusEndpoint(t *testing.T) {
	config := &Config{
		Namespace: "monitoring",
	}

	endpoint := config.GetPrometheusEndpoint()
	assert.Equal(t, "http://kube-prometheus-stack-prometheus.monitoring.svc.cluster.local:9090", endpoint)
}

func TestGetPrometheusEndpoint_CustomNamespace(t *testing.T) {
	config := &Config{
		Namespace: "custom-ns",
	}

	endpoint := config.GetPrometheusEndpoint()
	assert.Equal(t, "http://kube-prometheus-stack-prometheus.custom-ns.svc.cluster.local:9090", endpoint)
}

func TestGetAlertmanagerEndpoint(t *testing.T) {
	config := &Config{
		Namespace: "monitoring",
	}

	endpoint := config.GetAlertmanagerEndpoint()
	assert.Equal(t, "http://kube-prometheus-stack-alertmanager.monitoring.svc.cluster.local:9093", endpoint)
}

func TestComponent_Name(t *testing.T) {
	comp := NewComponent(nil, nil)
	assert.Equal(t, "prometheus", comp.Name())
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

func TestComponent_Status_PrometheusInstalled(t *testing.T) {
	helmClient := &mockHelmClient{
		listReleases: []helm.Release{
			{
				Name:       "kube-prometheus-stack",
				Namespace:  "monitoring",
				Status:     "deployed",
				AppVersion: "0.72.0",
			},
		},
	}

	comp := NewComponent(helmClient, nil)
	status, err := comp.Status(context.Background())

	assert.NoError(t, err)
	assert.True(t, status.Installed)
	assert.True(t, status.Healthy)
	assert.Equal(t, "0.72.0", status.Version)
}

func TestComponent_Status_PrometheusNotInstalled(t *testing.T) {
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

func TestComponent_Status_PrometheusFailed(t *testing.T) {
	helmClient := &mockHelmClient{
		listReleases: []helm.Release{
			{
				Name:       "kube-prometheus-stack",
				Namespace:  "monitoring",
				Status:     "failed",
				AppVersion: "0.72.0",
			},
		},
	}

	comp := NewComponent(helmClient, nil)
	status, err := comp.Status(context.Background())

	assert.NoError(t, err)
	assert.True(t, status.Installed)
	assert.False(t, status.Healthy)
}
