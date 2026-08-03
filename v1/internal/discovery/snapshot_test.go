package discovery

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/catalystcommunity/foundry/v1/internal/config"
	"github.com/catalystcommunity/foundry/v1/internal/host"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	dynamicfake "k8s.io/client-go/dynamic/fake"
	kubernetesfake "k8s.io/client-go/kubernetes/fake"
	k8stesting "k8s.io/client-go/testing"
)

func TestInspectWithoutClusterReturnsConfiguredLinks(t *testing.T) {
	snapshot := Inspect(context.Background(), serviceConfig(), filepath.Join(t.TempDir(), "missing-kubeconfig"))

	assert.False(t, snapshot.ClusterAvailable)
	assert.Contains(t, serviceURLs(snapshot.Services), "https://grafana.example.test")
	assert.Contains(t, serviceURLs(snapshot.Services), "http://192.0.2.11:8200")
	assert.Contains(t, serviceURLs(snapshot.Services), "http://192.0.2.12:5000")
	require.NotEmpty(t, snapshot.Warnings)
}

func TestInspectClientsReportsIngressGatewayListenersAndRoutes(t *testing.T) {
	kube := kubernetesfake.NewSimpleClientset(&networkingv1.Ingress{
		ObjectMeta: metav1.ObjectMeta{Name: "grafana", Namespace: "grafana", Labels: map[string]string{"app.kubernetes.io/name": "grafana"}},
		Spec: networkingv1.IngressSpec{
			TLS:   []networkingv1.IngressTLS{{Hosts: []string{"grafana.live.test"}}},
			Rules: []networkingv1.IngressRule{{Host: "grafana.live.test"}},
		},
	})
	gateway := &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "gateway.networking.k8s.io/v1", "kind": "Gateway",
		"metadata": map[string]interface{}{"name": "contour", "namespace": "projectcontour"},
		"spec": map[string]interface{}{
			"gatewayClassName": "contour",
			"listeners": []interface{}{
				map[string]interface{}{"name": "https", "protocol": "HTTPS", "port": int64(443), "hostname": "*.example.test"},
				map[string]interface{}{"name": "postgres", "protocol": "TCP", "port": int64(5432)},
			},
		},
		"status": map[string]interface{}{
			"addresses":  []interface{}{map[string]interface{}{"value": "192.0.2.10"}},
			"conditions": []interface{}{map[string]interface{}{"type": "Programmed", "status": "True"}},
			"listeners":  []interface{}{map[string]interface{}{"name": "https", "attachedRoutes": int64(1)}},
		},
	}}
	route := &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "gateway.networking.k8s.io/v1", "kind": "HTTPRoute",
		"metadata": map[string]interface{}{"name": "grafana", "namespace": "grafana"},
		"spec": map[string]interface{}{
			"hostnames":  []interface{}{"grafana.example.test"},
			"parentRefs": []interface{}{map[string]interface{}{"name": "contour", "namespace": "projectcontour"}},
			"rules": []interface{}{map[string]interface{}{"backendRefs": []interface{}{
				map[string]interface{}{"name": "grafana", "port": int64(80)},
			}}},
		},
		"status": map[string]interface{}{"parents": []interface{}{map[string]interface{}{
			"conditions": []interface{}{map[string]interface{}{"type": "Accepted", "status": "True"}},
		}}},
	}}
	gateway.SetGroupVersionKind(schema.GroupVersionKind{Group: "gateway.networking.k8s.io", Version: "v1", Kind: "Gateway"})
	route.SetGroupVersionKind(schema.GroupVersionKind{Group: "gateway.networking.k8s.io", Version: "v1", Kind: "HTTPRoute"})
	listKinds := map[schema.GroupVersionResource]string{gatewayGVR: "GatewayList"}
	for _, resource := range routeResources {
		listKinds[resource.GVR] = resource.Kind + "List"
	}
	dynamicClient := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(runtime.NewScheme(), listKinds)
	dynamicClient.PrependReactor("list", "gateways", func(k8stesting.Action) (bool, runtime.Object, error) {
		return true, &unstructured.UnstructuredList{Items: []unstructured.Unstructured{*gateway}}, nil
	})
	dynamicClient.PrependReactor("list", "httproutes", func(k8stesting.Action) (bool, runtime.Object, error) {
		return true, &unstructured.UnstructuredList{Items: []unstructured.Unstructured{*route}}, nil
	})
	direct, err := dynamicClient.Resource(gatewayGVR).Namespace(metav1.NamespaceAll).List(context.Background(), metav1.ListOptions{})
	require.NoError(t, err)
	require.Len(t, direct.Items, 1)

	snapshot := InspectClients(context.Background(), serviceConfig(), kube, dynamicClient)

	assert.True(t, snapshot.ClusterAvailable)
	assert.Contains(t, serviceURLs(snapshot.Services), "https://grafana.live.test")
	assert.Empty(t, snapshot.Warnings)
	require.Len(t, snapshot.Gateways, 1)
	actualGateway := snapshot.Gateways[0]
	assert.True(t, actualGateway.Programmed)
	assert.Equal(t, []string{"192.0.2.10"}, actualGateway.Addresses)
	require.Len(t, actualGateway.Listeners, 2)
	require.Len(t, actualGateway.Routes, 1)
	assert.True(t, actualGateway.Routes[0].Accepted)
	assert.Equal(t, []string{"grafana:80"}, actualGateway.Routes[0].Backends)
	assert.Equal(t, []string{"https://grafana.example.test"}, actualGateway.Routes[0].URLs)

	data, err := json.Marshal(snapshot)
	require.NoError(t, err)
	assert.Contains(t, string(data), "\"cluster_available\":true")
	assert.Contains(t, string(data), "\"attached_routes\":1")
}

func TestInspectClientsKeepsServiceLinksWhenGatewayDiscoveryFails(t *testing.T) {
	listKinds := map[schema.GroupVersionResource]string{gatewayGVR: "GatewayList"}
	for _, resource := range routeResources {
		listKinds[resource.GVR] = resource.Kind + "List"
	}
	dynamicClient := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(runtime.NewScheme(), listKinds)
	dynamicClient.PrependReactor("list", "gateways", func(k8stesting.Action) (bool, runtime.Object, error) {
		return true, nil, errors.New("API unavailable")
	})

	snapshot := InspectClients(context.Background(), serviceConfig(), kubernetesfake.NewSimpleClientset(), dynamicClient)

	assert.Contains(t, serviceURLs(snapshot.Services), "https://grafana.example.test")
	require.NotEmpty(t, snapshot.Warnings)
	assert.Contains(t, snapshot.Warnings[0], "API unavailable")
}

func TestCompatibilityConfigServiceDiscovery(t *testing.T) {
	paths := strings.Fields(os.Getenv("FOUNDRY_COMPAT_CONFIGS"))
	if len(paths) == 0 {
		t.Skip("FOUNDRY_COMPAT_CONFIGS is not set")
	}
	for _, path := range paths {
		cfg, err := config.Load(path)
		require.NoError(t, err)
		snapshot := Inspect(context.Background(), cfg, filepath.Join(t.TempDir(), "missing-kubeconfig"))
		assert.NotEmpty(t, snapshot.Services)
	}
}

func TestLiveDiscoveryReadOnly(t *testing.T) {
	configPath := os.Getenv("FOUNDRY_LIVE_DISCOVERY_CONFIG")
	kubeconfigPath := os.Getenv("FOUNDRY_LIVE_DISCOVERY_KUBECONFIG")
	if configPath == "" || kubeconfigPath == "" {
		t.Skip("live discovery paths are not set")
	}
	cfg, err := config.Load(configPath)
	require.NoError(t, err)
	snapshot := Inspect(context.Background(), cfg, kubeconfigPath)
	require.True(t, snapshot.ClusterAvailable, "warnings: %v", snapshot.Warnings)
	t.Logf("discovered %d service links and %d Gateways", len(snapshot.Services), len(snapshot.Gateways))
}

func serviceConfig() *config.Config {
	return &config.Config{
		Hosts: []*host.Host{
			{Hostname: "bao", Address: "192.0.2.11", Roles: []string{host.RoleOpenBAO}},
			{Hostname: "zot", Address: "192.0.2.12", Roles: []string{host.RoleZot}},
		},
		Components: config.ComponentMap{
			"grafana": {Config: map[string]interface{}{"ingress_enabled": true, "ingress_host": "grafana.example.test"}},
			"openbao": {},
			"zot":     {},
		},
	}
}

func serviceURLs(links []ServiceLink) []string {
	result := make([]string, 0, len(links))
	for _, link := range links {
		result = append(result, link.URL)
	}
	return result
}
