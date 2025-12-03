package grafana

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
			{Name: "grafana-abc123", Namespace: "grafana", Status: "Running"},
		},
	}

	cfg := DefaultConfig()
	err := Install(context.Background(), helmClient, k8sClient, cfg)
	require.NoError(t, err)

	// Verify repo was added
	require.Len(t, helmClient.reposAdded, 1)
	assert.Equal(t, grafanaRepoName, helmClient.reposAdded[0].Name)
	assert.Equal(t, grafanaRepoURL, helmClient.reposAdded[0].URL)

	// Verify chart was installed
	require.Len(t, helmClient.chartsInstalled, 1)
	assert.Equal(t, releaseName, helmClient.chartsInstalled[0].ReleaseName)
	assert.Equal(t, "grafana", helmClient.chartsInstalled[0].Namespace)
	assert.Equal(t, grafanaChart, helmClient.chartsInstalled[0].Chart)
	assert.True(t, helmClient.chartsInstalled[0].CreateNamespace)
	assert.True(t, helmClient.chartsInstalled[0].Wait)
	assert.Equal(t, 10*time.Minute, helmClient.chartsInstalled[0].Timeout)
}

func TestInstall_AlreadyInstalled(t *testing.T) {
	helmClient := &mockHelmClient{
		listReleases: []helm.Release{
			{
				Name:      releaseName,
				Namespace: "grafana",
				Status:    "deployed",
			},
		},
	}
	k8sClient := &mockK8sClient{
		pods: []*k8s.Pod{
			{Name: "grafana-abc123", Namespace: "grafana", Status: "Running"},
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
			{Name: "grafana-abc123", Namespace: "grafana", Status: "Running"},
		},
	}

	// Should use default config
	err := Install(context.Background(), helmClient, k8sClient, nil)
	require.NoError(t, err)

	// Verify installation happened with defaults
	require.Len(t, helmClient.chartsInstalled, 1)
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
	assert.Contains(t, err.Error(), "failed to install grafana")
}

func TestInstall_FailedReleaseCleanedup(t *testing.T) {
	helmClient := &mockHelmClient{
		listReleases: []helm.Release{
			{
				Name:      releaseName,
				Namespace: "grafana",
				Status:    "failed",
			},
		},
	}
	k8sClient := &mockK8sClient{
		pods: []*k8s.Pod{
			{Name: "grafana-abc123", Namespace: "grafana", Status: "Running"},
		},
	}

	cfg := DefaultConfig()
	err := Install(context.Background(), helmClient, k8sClient, cfg)
	require.NoError(t, err)

	// Should have uninstalled the failed release
	require.Len(t, helmClient.uninstallCalls, 1)
	assert.Equal(t, releaseName, helmClient.uninstallCalls[0].ReleaseName)

	// And installed fresh
	require.Len(t, helmClient.chartsInstalled, 1)
}

func TestBuildHelmValues_Default(t *testing.T) {
	cfg := DefaultConfig()
	values := buildHelmValues(cfg)

	// Check admin user
	assert.Equal(t, "admin", values["adminUser"])

	// Check persistence
	persistence, ok := values["persistence"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, true, persistence["enabled"])
	assert.Equal(t, "5Gi", persistence["size"])

	// Check service
	service, ok := values["service"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "ClusterIP", service["type"])
	assert.Equal(t, 80, service["port"])

	// Check ServiceMonitor is enabled
	serviceMonitor, ok := values["serviceMonitor"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, true, serviceMonitor["enabled"])
}

func TestBuildHelmValues_WithAdminPassword(t *testing.T) {
	cfg := &Config{
		AdminUser:     "admin",
		AdminPassword: "supersecret",
		StorageSize:   "5Gi",
		Values:        map[string]interface{}{},
	}

	values := buildHelmValues(cfg)

	assert.Equal(t, "admin", values["adminUser"])
	assert.Equal(t, "supersecret", values["adminPassword"])
}

func TestBuildHelmValues_WithStorageClass(t *testing.T) {
	cfg := &Config{
		AdminUser:    "admin",
		StorageClass: "fast-storage",
		StorageSize:  "10Gi",
		Values:       map[string]interface{}{},
	}

	values := buildHelmValues(cfg)

	persistence, ok := values["persistence"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "fast-storage", persistence["storageClassName"])
	assert.Equal(t, "10Gi", persistence["size"])
}

func TestBuildHelmValues_WithIngress(t *testing.T) {
	cfg := &Config{
		AdminUser:      "admin",
		IngressEnabled: true,
		IngressHost:    "grafana.example.com",
		StorageSize:    "5Gi",
		Values:         map[string]interface{}{},
	}

	values := buildHelmValues(cfg)

	ingress, ok := values["ingress"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, true, ingress["enabled"])
	assert.Equal(t, "contour", ingress["ingressClassName"])

	hosts, ok := ingress["hosts"].([]string)
	require.True(t, ok)
	assert.Contains(t, hosts, "grafana.example.com")

	// Check grafana.ini for root_url
	grafanaIni, ok := values["grafana.ini"].(map[string]interface{})
	require.True(t, ok)
	server, ok := grafanaIni["server"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "https://grafana.example.com", server["root_url"])
}

func TestBuildHelmValues_WithSidecar(t *testing.T) {
	cfg := &Config{
		AdminUser:      "admin",
		SidecarEnabled: true,
		StorageSize:    "5Gi",
		Values:         map[string]interface{}{},
	}

	values := buildHelmValues(cfg)

	sidecar, ok := values["sidecar"].(map[string]interface{})
	require.True(t, ok)

	dashboards, ok := sidecar["dashboards"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, true, dashboards["enabled"])
	assert.Equal(t, "ALL", dashboards["searchNamespace"])
	assert.Equal(t, "grafana_dashboard", dashboards["label"])

	datasources, ok := sidecar["datasources"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, true, datasources["enabled"])
}

func TestBuildHelmValues_WithDefaultDashboards(t *testing.T) {
	cfg := &Config{
		AdminUser:                "admin",
		DefaultDashboardsEnabled: true,
		StorageSize:              "5Gi",
		Values:                   map[string]interface{}{},
	}

	values := buildHelmValues(cfg)

	// Check dashboard providers
	providers, ok := values["dashboardProviders"].(map[string]interface{})
	require.True(t, ok)
	assert.NotNil(t, providers["dashboardproviders.yaml"])

	// Check dashboards
	dashboards, ok := values["dashboards"].(map[string]interface{})
	require.True(t, ok)

	defaultDashboards, ok := dashboards["default"].(map[string]interface{})
	require.True(t, ok)
	assert.NotNil(t, defaultDashboards["kubernetes-cluster"])
	assert.NotNil(t, defaultDashboards["node-exporter"])
}

func TestBuildHelmValues_CustomValues(t *testing.T) {
	cfg := &Config{
		AdminUser:   "admin",
		StorageSize: "5Gi",
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

func TestBuildDatasources_Default(t *testing.T) {
	cfg := DefaultConfig()
	datasources := buildDatasources(cfg)

	dsYaml, ok := datasources["datasources.yaml"].(map[string]interface{})
	require.True(t, ok)

	dsList, ok := dsYaml["datasources"].([]map[string]interface{})
	require.True(t, ok)
	require.Len(t, dsList, 2)

	// Check Prometheus datasource
	assert.Equal(t, "Prometheus", dsList[0]["name"])
	assert.Equal(t, "prometheus", dsList[0]["type"])
	assert.Equal(t, cfg.PrometheusURL, dsList[0]["url"])
	assert.Equal(t, true, dsList[0]["isDefault"])

	// Check Loki datasource
	assert.Equal(t, "Loki", dsList[1]["name"])
	assert.Equal(t, "loki", dsList[1]["type"])
	assert.Equal(t, cfg.LokiURL, dsList[1]["url"])
}

func TestBuildDatasources_OnlyPrometheus(t *testing.T) {
	cfg := &Config{
		PrometheusURL: "http://prometheus:9090",
		LokiURL:       "", // Empty, no Loki
	}

	datasources := buildDatasources(cfg)

	dsYaml, ok := datasources["datasources.yaml"].(map[string]interface{})
	require.True(t, ok)

	dsList, ok := dsYaml["datasources"].([]map[string]interface{})
	require.True(t, ok)
	require.Len(t, dsList, 1)

	assert.Equal(t, "Prometheus", dsList[0]["name"])
}

func TestBuildDatasources_OnlyLoki(t *testing.T) {
	cfg := &Config{
		PrometheusURL: "", // Empty, no Prometheus
		LokiURL:       "http://loki:3100",
	}

	datasources := buildDatasources(cfg)

	dsYaml, ok := datasources["datasources.yaml"].(map[string]interface{})
	require.True(t, ok)

	dsList, ok := dsYaml["datasources"].([]map[string]interface{})
	require.True(t, ok)
	require.Len(t, dsList, 1)

	assert.Equal(t, "Loki", dsList[0]["name"])
}

func TestBuildDatasources_NoDatasources(t *testing.T) {
	cfg := &Config{
		PrometheusURL: "",
		LokiURL:       "",
	}

	datasources := buildDatasources(cfg)

	dsYaml, ok := datasources["datasources.yaml"].(map[string]interface{})
	require.True(t, ok)

	dsList, ok := dsYaml["datasources"].([]map[string]interface{})
	require.True(t, ok)
	assert.Len(t, dsList, 0)
}

func TestVerifyInstallation_Success(t *testing.T) {
	k8sClient := &mockK8sClient{
		pods: []*k8s.Pod{
			{Name: "grafana-abc123", Namespace: "grafana", Status: "Running"},
		},
	}

	ctx := context.Background()
	err := verifyInstallation(ctx, k8sClient, "grafana")
	assert.NoError(t, err)
}

func TestVerifyInstallation_NilClient(t *testing.T) {
	ctx := context.Background()
	err := verifyInstallation(ctx, nil, "grafana")
	assert.NoError(t, err) // Should skip verification
}

func TestVerifyInstallation_PodsNotReady(t *testing.T) {
	k8sClient := &mockK8sClient{
		pods: []*k8s.Pod{
			{Name: "grafana-abc123", Namespace: "grafana", Status: "Pending"},
		},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	err := verifyInstallation(ctx, k8sClient, "grafana")
	assert.Error(t, err)
}

func TestVerifyInstallation_ContextCanceled(t *testing.T) {
	k8sClient := &mockK8sClient{
		pods: []*k8s.Pod{},
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	err := verifyInstallation(ctx, k8sClient, "grafana")
	assert.Error(t, err)
	assert.Equal(t, context.Canceled, err)
}

func TestContainsSubstring(t *testing.T) {
	tests := []struct {
		s        string
		substr   string
		expected bool
	}{
		{"grafana-abc123", "grafana", true},
		{"grafana-sidecar-xyz", "grafana", true},
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
