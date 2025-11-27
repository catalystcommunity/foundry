package contour

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

	assert.Equal(t, "0.1.0", config.Version) // Official Project Contour chart version
	assert.Equal(t, "projectcontour", config.Namespace)
	assert.Equal(t, uint64(2), config.ReplicaCount)
	assert.Equal(t, uint64(2), config.EnvoyReplicaCount)
	assert.True(t, config.UseKubeVIP)
	assert.True(t, config.DefaultIngressClass)
	assert.NotNil(t, config.Values)
}

func TestParseConfig_Defaults(t *testing.T) {
	cfg := component.ComponentConfig{}

	config, err := ParseConfig(cfg)
	require.NoError(t, err)

	assert.Equal(t, "0.1.0", config.Version) // Official Project Contour chart version
	assert.Equal(t, "projectcontour", config.Namespace)
	assert.Equal(t, uint64(2), config.ReplicaCount)
	assert.Equal(t, uint64(2), config.EnvoyReplicaCount)
	assert.True(t, config.UseKubeVIP)
	assert.True(t, config.DefaultIngressClass)
}

func TestParseConfig_CustomValues(t *testing.T) {
	cfg := component.ComponentConfig{
		"version":               "1.28.0",
		"namespace":             "custom-contour",
		"replica_count":         3,
		"envoy_replica_count":   4,
		"use_kubevip":           false,
		"default_ingress_class": false,
	}

	config, err := ParseConfig(cfg)
	require.NoError(t, err)

	assert.Equal(t, "1.28.0", config.Version)
	assert.Equal(t, "custom-contour", config.Namespace)
	assert.Equal(t, uint64(3), config.ReplicaCount)
	assert.Equal(t, uint64(4), config.EnvoyReplicaCount)
	assert.False(t, config.UseKubeVIP)
	assert.False(t, config.DefaultIngressClass)
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

func TestComponent_Name(t *testing.T) {
	comp := NewComponent(nil, nil)
	assert.Equal(t, "contour", comp.Name())
}

func TestComponent_Dependencies(t *testing.T) {
	comp := NewComponent(nil, nil)
	deps := comp.Dependencies()

	require.Len(t, deps, 2)
	assert.Contains(t, deps, "k3s")
	assert.Contains(t, deps, "gateway-api")
}

func TestComponent_Install_NilHelmClient(t *testing.T) {
	comp := NewComponent(nil, nil)
	err := comp.Install(context.Background(), component.ComponentConfig{})

	// Should fail with nil helm client error
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

	assert.NoError(t, err) // Status returns error as part of ComponentStatus
	assert.False(t, status.Installed)
	assert.False(t, status.Healthy)
	assert.Contains(t, status.Message, "not initialized")
}

func TestComponent_Status_Success(t *testing.T) {
	helmClient := &mockHelmClient{
		listReleases: []helm.Release{
			{
				Name:       "contour",
				Namespace:  "projectcontour",
				Status:     "deployed",
				AppVersion: "1.28.0",
			},
		},
	}
	k8sClient := &mockK8sClient{
		pods: []*k8s.Pod{
			{Name: "contour-1", Namespace: "projectcontour", Status: "Running"},
			{Name: "envoy-1", Namespace: "projectcontour", Status: "Running"},
		},
	}

	comp := NewComponent(helmClient, k8sClient)
	status, err := comp.Status(context.Background())

	assert.NoError(t, err)
	assert.True(t, status.Installed)
	assert.True(t, status.Healthy)
	assert.Equal(t, "1.28.0", status.Version)
	assert.Contains(t, status.Message, "2/2 pods running")
}

func TestComponent_Status_NoRelease(t *testing.T) {
	helmClient := &mockHelmClient{
		listReleases: []helm.Release{},
	}
	k8sClient := &mockK8sClient{}

	comp := NewComponent(helmClient, k8sClient)
	status, err := comp.Status(context.Background())

	assert.NoError(t, err)
	assert.False(t, status.Installed)
	assert.False(t, status.Healthy)
	assert.Contains(t, status.Message, "not found")
}

func TestComponent_Status_PodsNotRunning(t *testing.T) {
	helmClient := &mockHelmClient{
		listReleases: []helm.Release{
			{
				Name:       "contour",
				Namespace:  "projectcontour",
				Status:     "deployed",
				AppVersion: "1.28.0",
			},
		},
	}
	k8sClient := &mockK8sClient{
		pods: []*k8s.Pod{
			{Name: "contour-1", Namespace: "projectcontour", Status: "Pending"},
			{Name: "envoy-1", Namespace: "projectcontour", Status: "Running"},
		},
	}

	comp := NewComponent(helmClient, k8sClient)
	status, err := comp.Status(context.Background())

	assert.NoError(t, err)
	assert.True(t, status.Installed)
	assert.False(t, status.Healthy)
	assert.Equal(t, "1.28.0", status.Version)
	assert.Contains(t, status.Message, "1/2 pods running")
}

func TestComponent_Status_PodsFetchError(t *testing.T) {
	helmClient := &mockHelmClient{
		listReleases: []helm.Release{
			{
				Name:       "contour",
				Namespace:  "projectcontour",
				Status:     "deployed",
				AppVersion: "1.28.0",
			},
		},
	}
	k8sClient := &mockK8sClient{
		podsErr: assert.AnError,
	}

	comp := NewComponent(helmClient, k8sClient)
	status, err := comp.Status(context.Background())

	assert.NoError(t, err)
	assert.True(t, status.Installed)
	assert.False(t, status.Healthy)
	assert.Contains(t, status.Message, "failed to get pods")
}
