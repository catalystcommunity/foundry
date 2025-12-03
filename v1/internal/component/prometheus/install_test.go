package prometheus

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
			{Name: "prometheus-kube-prometheus-stack-prometheus-0", Namespace: "monitoring", Status: "Running"},
		},
	}

	cfg := DefaultConfig()
	err := Install(context.Background(), helmClient, k8sClient, cfg)
	require.NoError(t, err)

	// Verify repo was added
	require.Len(t, helmClient.reposAdded, 1)
	assert.Equal(t, prometheusRepoName, helmClient.reposAdded[0].Name)
	assert.Equal(t, prometheusRepoURL, helmClient.reposAdded[0].URL)

	// Verify chart was installed
	require.Len(t, helmClient.chartsInstalled, 1)
	assert.Equal(t, releaseName, helmClient.chartsInstalled[0].ReleaseName)
	assert.Equal(t, "monitoring", helmClient.chartsInstalled[0].Namespace)
	assert.Equal(t, prometheusChart, helmClient.chartsInstalled[0].Chart)
	assert.True(t, helmClient.chartsInstalled[0].CreateNamespace)
	assert.True(t, helmClient.chartsInstalled[0].Wait)
	assert.Equal(t, 15*time.Minute, helmClient.chartsInstalled[0].Timeout)
}

func TestInstall_AlreadyInstalled(t *testing.T) {
	helmClient := &mockHelmClient{
		listReleases: []helm.Release{
			{
				Name:      releaseName,
				Namespace: "monitoring",
				Status:    "deployed",
			},
		},
	}
	k8sClient := &mockK8sClient{
		pods: []*k8s.Pod{
			{Name: "prometheus-kube-prometheus-stack-prometheus-0", Namespace: "monitoring", Status: "Running"},
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
			{Name: "prometheus-kube-prometheus-stack-prometheus-0", Namespace: "monitoring", Status: "Running"},
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
	assert.Contains(t, err.Error(), "failed to install prometheus stack")
}

func TestInstall_FailedReleaseCleanedup(t *testing.T) {
	helmClient := &mockHelmClient{
		listReleases: []helm.Release{
			{
				Name:      releaseName,
				Namespace: "monitoring",
				Status:    "failed",
			},
		},
	}
	k8sClient := &mockK8sClient{
		pods: []*k8s.Pod{
			{Name: "prometheus-kube-prometheus-stack-prometheus-0", Namespace: "monitoring", Status: "Running"},
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

	// Check Prometheus config
	prometheus, ok := values["prometheus"].(map[string]interface{})
	require.True(t, ok)

	prometheusSpec, ok := prometheus["prometheusSpec"].(map[string]interface{})
	require.True(t, ok)

	assert.Equal(t, "15d", prometheusSpec["retention"])
	assert.Equal(t, "10Gi", prometheusSpec["retentionSize"])
	assert.Equal(t, "30s", prometheusSpec["scrapeInterval"])

	// Check Grafana is disabled
	grafana, ok := values["grafana"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, false, grafana["enabled"])

	// Check Alertmanager is enabled
	alertmanager, ok := values["alertmanager"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, true, alertmanager["enabled"])
}

func TestBuildHelmValues_WithStorage(t *testing.T) {
	cfg := &Config{
		RetentionDays:           30,
		RetentionSize:           "50Gi",
		StorageClass:            "fast-storage",
		StorageSize:             "100Gi",
		ScrapeInterval:          "15s",
		AlertmanagerEnabled:     true,
		NodeExporterEnabled:     true,
		KubeStateMetricsEnabled: true,
		Values:                  map[string]interface{}{},
	}

	values := buildHelmValues(cfg)

	prometheus, ok := values["prometheus"].(map[string]interface{})
	require.True(t, ok)

	prometheusSpec, ok := prometheus["prometheusSpec"].(map[string]interface{})
	require.True(t, ok)

	assert.Equal(t, "30d", prometheusSpec["retention"])
	assert.Equal(t, "50Gi", prometheusSpec["retentionSize"])

	// Check storage spec
	storageSpec, ok := prometheusSpec["storageSpec"].(map[string]interface{})
	require.True(t, ok)

	vct, ok := storageSpec["volumeClaimTemplate"].(map[string]interface{})
	require.True(t, ok)

	spec, ok := vct["spec"].(map[string]interface{})
	require.True(t, ok)

	assert.Equal(t, "fast-storage", spec["storageClassName"])

	resources, ok := spec["resources"].(map[string]interface{})
	require.True(t, ok)

	requests, ok := resources["requests"].(map[string]interface{})
	require.True(t, ok)

	assert.Equal(t, "100Gi", requests["storage"])
}

func TestBuildHelmValues_WithIngress(t *testing.T) {
	cfg := &Config{
		RetentionDays:       15,
		RetentionSize:       "10Gi",
		ScrapeInterval:      "30s",
		IngressEnabled:      true,
		IngressHost:         "prometheus.example.com",
		AlertmanagerEnabled: false,
		Values:              map[string]interface{}{},
	}

	values := buildHelmValues(cfg)

	prometheus, ok := values["prometheus"].(map[string]interface{})
	require.True(t, ok)

	ingress, ok := prometheus["ingress"].(map[string]interface{})
	require.True(t, ok)

	assert.Equal(t, true, ingress["enabled"])
	assert.Equal(t, "contour", ingress["ingressClassName"])

	hosts, ok := ingress["hosts"].([]string)
	require.True(t, ok)
	assert.Contains(t, hosts, "prometheus.example.com")
}

func TestBuildHelmValues_AlertmanagerDisabled(t *testing.T) {
	cfg := &Config{
		RetentionDays:       15,
		RetentionSize:       "10Gi",
		ScrapeInterval:      "30s",
		AlertmanagerEnabled: false,
		Values:              map[string]interface{}{},
	}

	values := buildHelmValues(cfg)

	alertmanager, ok := values["alertmanager"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, false, alertmanager["enabled"])
}

func TestBuildHelmValues_ServiceMonitorSelectors(t *testing.T) {
	cfg := DefaultConfig()
	values := buildHelmValues(cfg)

	prometheus, ok := values["prometheus"].(map[string]interface{})
	require.True(t, ok)

	prometheusSpec, ok := prometheus["prometheusSpec"].(map[string]interface{})
	require.True(t, ok)

	// Check that selectors are set to discover all monitors
	assert.Equal(t, false, prometheusSpec["serviceMonitorSelectorNilUsesHelmValues"])
	assert.Equal(t, false, prometheusSpec["podMonitorSelectorNilUsesHelmValues"])
	assert.Equal(t, false, prometheusSpec["probeSelectorNilUsesHelmValues"])
	assert.Equal(t, false, prometheusSpec["ruleSelectorNilUsesHelmValues"])
}

func TestBuildHelmValues_CustomValues(t *testing.T) {
	cfg := &Config{
		RetentionDays:  15,
		RetentionSize:  "10Gi",
		ScrapeInterval: "30s",
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
			{Name: "prometheus-kube-prometheus-stack-prometheus-0", Namespace: "monitoring", Status: "Running"},
		},
	}

	ctx := context.Background()
	err := verifyInstallation(ctx, k8sClient, "monitoring")
	assert.NoError(t, err)
}

func TestVerifyInstallation_AlternativePodName(t *testing.T) {
	k8sClient := &mockK8sClient{
		pods: []*k8s.Pod{
			{Name: "prometheus-prometheus-0", Namespace: "monitoring", Status: "Running"},
		},
	}

	ctx := context.Background()
	err := verifyInstallation(ctx, k8sClient, "monitoring")
	assert.NoError(t, err)
}

func TestVerifyInstallation_NilClient(t *testing.T) {
	ctx := context.Background()
	err := verifyInstallation(ctx, nil, "monitoring")
	assert.NoError(t, err) // Should skip verification
}

func TestVerifyInstallation_PodsNotReady(t *testing.T) {
	k8sClient := &mockK8sClient{
		pods: []*k8s.Pod{
			{Name: "prometheus-kube-prometheus-stack-prometheus-0", Namespace: "monitoring", Status: "Pending"},
		},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	err := verifyInstallation(ctx, k8sClient, "monitoring")
	assert.Error(t, err)
}

func TestVerifyInstallation_ContextCanceled(t *testing.T) {
	k8sClient := &mockK8sClient{
		pods: []*k8s.Pod{},
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	err := verifyInstallation(ctx, k8sClient, "monitoring")
	assert.Error(t, err)
	assert.Equal(t, context.Canceled, err)
}

func TestContainsSubstring(t *testing.T) {
	tests := []struct {
		s        string
		substr   string
		expected bool
	}{
		{"prometheus-kube-prometheus-stack-prometheus-0", "prometheus-kube-prometheus-stack-prometheus", true},
		{"prometheus-prometheus-0", "prometheus-prometheus", true},
		{"prometheus-0", "prometheus", true},
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

func TestGetServiceMonitorYAML(t *testing.T) {
	spec := ServiceMonitorSpec{
		Name:      "test-monitor",
		Namespace: "default",
		Selector: map[string]string{
			"app": "myapp",
		},
		Port:     "metrics",
		Path:     "/metrics",
		Interval: "30s",
	}

	yaml := GetServiceMonitorYAML(spec)

	assert.Contains(t, yaml, "kind: ServiceMonitor")
	assert.Contains(t, yaml, "name: test-monitor")
	assert.Contains(t, yaml, "namespace: default")
	assert.Contains(t, yaml, "app: myapp")
	assert.Contains(t, yaml, "port: metrics")
	assert.Contains(t, yaml, "path: /metrics")
	assert.Contains(t, yaml, "interval: 30s")
}

func TestGetServiceMonitorYAML_DefaultValues(t *testing.T) {
	spec := ServiceMonitorSpec{
		Name:      "test-monitor",
		Namespace: "default",
		Selector: map[string]string{
			"app": "myapp",
		},
		Port: "metrics",
		// Path and Interval not specified, should use defaults
	}

	yaml := GetServiceMonitorYAML(spec)

	assert.Contains(t, yaml, "path: /metrics")
	assert.Contains(t, yaml, "interval: 30s")
}

func TestGetServiceMonitorYAML_TargetPort(t *testing.T) {
	spec := ServiceMonitorSpec{
		Name:      "test-monitor",
		Namespace: "default",
		Selector: map[string]string{
			"app": "myapp",
		},
		TargetPort: 9090,
	}

	yaml := GetServiceMonitorYAML(spec)

	assert.Contains(t, yaml, "targetPort: 9090")
}
