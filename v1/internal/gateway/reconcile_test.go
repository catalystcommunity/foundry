package gateway

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/intstr"
	dynamicfake "k8s.io/client-go/dynamic/fake"
	k8sfake "k8s.io/client-go/kubernetes/fake"
)

// --- pure helpers -----------------------------------------------------------

func TestComputeDesired_DedupeSortAndConflicts(t *testing.T) {
	candidates := []routeCandidate{
		{Port: 6000, Protocol: protocolTCP, Source: "a/x"},
		{Port: 4987, Protocol: protocolTLS, Source: "ns/lk"},
		{Port: 4987, Protocol: protocolTLS, Source: "ns/lk2"}, // duplicate, same proto -> shared
		{Port: 443, Protocol: protocolTLS, Source: "ns/bad"},  // reserved
		{Port: 7000, Protocol: protocolTLS, Source: "ns/c"},
		{Port: 7000, Protocol: protocolTCP, Source: "ns/d"}, // conflict -> dropped
	}

	desired, conflicts := computeDesired(candidates)

	require.Len(t, desired, 2)
	assert.Equal(t, DesiredListener{Port: 4987, Protocol: protocolTLS}, desired[0])
	assert.Equal(t, DesiredListener{Port: 6000, Protocol: protocolTCP}, desired[1])

	// reserved 443 and conflicting 7000 produce diagnostics
	require.Len(t, conflicts, 2)
	assert.Contains(t, conflicts[0], "443")
	assert.Contains(t, conflicts[1], "7000")
}

func TestExtractCandidates_PortDerivation(t *testing.T) {
	opts := DefaultOptions()
	const api = "gateway.networking.k8s.io/v1alpha2"

	t.Run("parentRef port only is unchanged", func(t *testing.T) {
		route := routeWithRefs("TLSRoute", api, "lk", "linkkeys", 4987, 0)
		got, skipped := extractCandidates(route, protocolTLS, opts)
		require.Len(t, got, 1)
		assert.Equal(t, int32(4987), got[0].Port)
		assert.Empty(t, skipped)
	})

	t.Run("backendRef port fallback when parentRef omits port", func(t *testing.T) {
		route := routeWithRefs("TLSRoute", api, "lk", "linkkeys", 0, 4987)
		got, skipped := extractCandidates(route, protocolTLS, opts)
		require.Len(t, got, 1)
		assert.Equal(t, int32(4987), got[0].Port)
		assert.Empty(t, skipped)
	})

	t.Run("parentRef port wins over backendRef port", func(t *testing.T) {
		route := routeWithRefs("TLSRoute", api, "lk", "linkkeys", 4987, 5000)
		got, skipped := extractCandidates(route, protocolTLS, opts)
		require.Len(t, got, 1)
		assert.Equal(t, int32(4987), got[0].Port)
		assert.Empty(t, skipped)
	})

	t.Run("no derivable port is skipped with a reason", func(t *testing.T) {
		route := routeWithRefs("TLSRoute", api, "lk", "linkkeys", 0, 0)
		got, skipped := extractCandidates(route, protocolTLS, opts)
		assert.Empty(t, got)
		require.Len(t, skipped, 1)
		assert.Contains(t, skipped[0], "linkkeys/lk")
		assert.Contains(t, skipped[0], "no listener port could be derived")
	})

	t.Run("conflicting backendRef ports are ambiguous and skipped", func(t *testing.T) {
		route := &unstructured.Unstructured{Object: map[string]interface{}{
			"apiVersion": api, "kind": "TLSRoute",
			"metadata": map[string]interface{}{"name": "lk", "namespace": "linkkeys"},
			"spec": map[string]interface{}{
				"parentRefs": []interface{}{
					map[string]interface{}{"name": "contour", "namespace": "projectcontour"},
				},
				"rules": []interface{}{
					map[string]interface{}{"backendRefs": []interface{}{
						map[string]interface{}{"name": "a", "port": int64(4987)},
						map[string]interface{}{"name": "b", "port": int64(5000)},
					}},
				},
			},
		}}
		got, skipped := extractCandidates(route, protocolTLS, opts)
		assert.Empty(t, got)
		require.Len(t, skipped, 1)
		assert.Contains(t, skipped[0], "multiple distinct ports")
	})

	t.Run("route for a different gateway is ignored without a skip", func(t *testing.T) {
		route := &unstructured.Unstructured{Object: map[string]interface{}{
			"apiVersion": api, "kind": "TLSRoute",
			"metadata": map[string]interface{}{"name": "other", "namespace": "x"},
			"spec": map[string]interface{}{
				"parentRefs": []interface{}{
					map[string]interface{}{"name": "someone-else", "namespace": "projectcontour"},
				},
				"rules": []interface{}{
					map[string]interface{}{"backendRefs": []interface{}{
						map[string]interface{}{"name": "a"}, // no port
					}},
				},
			},
		}}
		got, skipped := extractCandidates(route, protocolTLS, opts)
		assert.Empty(t, got)
		assert.Empty(t, skipped, "non-managed gateways must not produce skip diagnostics")
	})
}

func TestListenerNameAndTargetPort(t *testing.T) {
	d := DesiredListener{Port: 4987, Protocol: protocolTLS}
	assert.Equal(t, "gw-tls-4987", listenerName(d))
	assert.LessOrEqual(t, len(listenerName(d)), 15, "service port names must be <=15 chars")
	assert.Equal(t, int32(12987), targetPort(4987))
}

func TestMergeServicePorts_KeepsBaseReplacesManaged(t *testing.T) {
	existing := []corev1.ServicePort{
		{Name: "http", Port: 80, TargetPort: intstr.FromString("http"), Protocol: corev1.ProtocolTCP},
		{Name: "https", Port: 443, TargetPort: intstr.FromString("https"), Protocol: corev1.ProtocolTCP},
		{Name: "gw-tcp-9999", Port: 9999, TargetPort: intstr.FromInt32(17999), Protocol: corev1.ProtocolTCP}, // stale managed
	}
	desired := []DesiredListener{{Port: 4987, Protocol: protocolTLS}}

	merged, changed := mergeServicePorts(existing, desired)
	require.True(t, changed)

	names := map[string]int32{}
	for _, p := range merged {
		names[p.Name] = p.Port
	}
	assert.Contains(t, names, "http")
	assert.Contains(t, names, "https")
	assert.NotContains(t, names, "gw-tcp-9999", "stale managed port should be pruned")
	assert.Equal(t, int32(4987), names["gw-tls-4987"])

	// idempotent on the already-merged set
	_, changed2 := mergeServicePorts(merged, desired)
	assert.False(t, changed2)
}

func TestMergeGatewayListeners_KeepsBaseReplacesManaged(t *testing.T) {
	existing := []interface{}{
		map[string]interface{}{"name": "http", "port": int64(80), "protocol": "HTTP"},
		map[string]interface{}{"name": "gw-tls-1111", "port": int64(1111), "protocol": "TLS"}, // stale
	}
	desired := []DesiredListener{{Port: 4987, Protocol: protocolTLS}}

	merged, changed := mergeGatewayListeners(existing, desired)
	require.True(t, changed)

	gotNames := map[string]bool{}
	for _, raw := range merged {
		m := raw.(map[string]interface{})
		gotNames[m["name"].(string)] = true
	}
	assert.True(t, gotNames["http"])
	assert.False(t, gotNames["gw-tls-1111"], "stale managed listener pruned")
	assert.True(t, gotNames["gw-tls-4987"])

	_, changed2 := mergeGatewayListeners(merged, desired)
	assert.False(t, changed2)
}

func TestMergeRulePorts_AddsDesiredRemovesStaleManaged(t *testing.T) {
	existing := []networkingv1.NetworkPolicyPort{
		networkPolicyTCPPort(8080),
		networkPolicyTCPPort(8443),
		networkPolicyTCPPort(8002),
		networkPolicyTCPPort(17999), // previously managed, now stale
	}
	prior := []int32{17999}
	desired := []int32{12987}

	merged, changed := mergeRulePorts(existing, prior, desired)
	require.True(t, changed)

	got := map[int32]bool{}
	for _, p := range merged {
		v, _ := portValue(p)
		got[v] = true
	}
	assert.True(t, got[8080], "base envoy port preserved")
	assert.True(t, got[8443])
	assert.True(t, got[8002])
	assert.False(t, got[17999], "stale managed target pruned")
	assert.True(t, got[12987], "desired target added")
}

// --- integration with fake clients -----------------------------------------

func newFakeDynamic() *dynamicfake.FakeDynamicClient {
	scheme := runtime.NewScheme()
	listKinds := map[schema.GroupVersionResource]string{
		gatewayGVR:  "GatewayList",
		tlsRouteGVR: "TLSRouteList",
		tcpRouteGVR: "TCPRouteList",
	}
	return dynamicfake.NewSimpleDynamicClientWithCustomListKinds(scheme, listKinds)
}

func gatewayObject() *unstructured.Unstructured {
	return &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "gateway.networking.k8s.io/v1",
		"kind":       "Gateway",
		"metadata":   map[string]interface{}{"name": "contour", "namespace": "projectcontour"},
		"spec": map[string]interface{}{
			"gatewayClassName": "contour",
			"listeners": []interface{}{
				map[string]interface{}{
					"name": "http", "port": int64(80), "protocol": "HTTP",
					"allowedRoutes": map[string]interface{}{"namespaces": map[string]interface{}{"from": "All"}},
				},
			},
		},
	}}
}

func routeObject(kind, apiVersion, name, ns string, port int64) *unstructured.Unstructured {
	return &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": apiVersion,
		"kind":       kind,
		"metadata":   map[string]interface{}{"name": name, "namespace": ns},
		"spec": map[string]interface{}{
			"parentRefs": []interface{}{
				map[string]interface{}{"name": "contour", "namespace": "projectcontour", "port": port},
			},
		},
	}}
}

// routeWithRefs builds a route with a fully-specified parentRef (port=parentPort,
// or omitted when parentPort<=0) and a single backendRef (port=backendPort, or
// omitted when backendPort<=0).
func routeWithRefs(kind, apiVersion, name, ns string, parentPort, backendPort int64) *unstructured.Unstructured {
	parentRef := map[string]interface{}{"name": "contour", "namespace": "projectcontour"}
	if parentPort > 0 {
		parentRef["port"] = parentPort
	}
	backendRef := map[string]interface{}{"name": name}
	if backendPort > 0 {
		backendRef["port"] = backendPort
	}
	return &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": apiVersion,
		"kind":       kind,
		"metadata":   map[string]interface{}{"name": name, "namespace": ns},
		"spec": map[string]interface{}{
			"parentRefs": []interface{}{parentRef},
			"rules": []interface{}{
				map[string]interface{}{
					"backendRefs": []interface{}{backendRef},
				},
			},
		},
	}}
}

func envoyService() *corev1.Service {
	return &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{Name: "contour-envoy", Namespace: "projectcontour"},
		Spec: corev1.ServiceSpec{
			Type:        corev1.ServiceTypeClusterIP,
			ExternalIPs: []string{"10.16.0.30"},
			Ports: []corev1.ServicePort{
				{Name: "http", Port: 80, TargetPort: intstr.FromString("http"), Protocol: corev1.ProtocolTCP},
				{Name: "https", Port: 443, TargetPort: intstr.FromString("https"), Protocol: corev1.ProtocolTCP},
			},
		},
	}
}

func envoyNetworkPolicy() *networkingv1.NetworkPolicy {
	return &networkingv1.NetworkPolicy{
		ObjectMeta: metav1.ObjectMeta{Name: "contour-envoy", Namespace: "projectcontour"},
		Spec: networkingv1.NetworkPolicySpec{
			Ingress: []networkingv1.NetworkPolicyIngressRule{
				{Ports: []networkingv1.NetworkPolicyPort{
					networkPolicyTCPPort(8080),
					networkPolicyTCPPort(8443),
					networkPolicyTCPPort(8002),
				}},
			},
		},
	}
}

func TestReconcile_OpensListenersFromRoutes(t *testing.T) {
	ctx := context.Background()
	opts := DefaultOptions()

	dyn := newFakeDynamic()
	_, err := dyn.Resource(gatewayGVR).Namespace("projectcontour").Create(ctx, gatewayObject(), metav1.CreateOptions{})
	require.NoError(t, err)
	_, err = dyn.Resource(tlsRouteGVR).Namespace("linkkeys").Create(ctx,
		routeObject("TLSRoute", "gateway.networking.k8s.io/v1alpha2", "lk", "linkkeys", 4987), metav1.CreateOptions{})
	require.NoError(t, err)
	_, err = dyn.Resource(tcpRouteGVR).Namespace("apps").Create(ctx,
		routeObject("TCPRoute", "gateway.networking.k8s.io/v1alpha2", "raw", "apps", 6000), metav1.CreateOptions{})
	require.NoError(t, err)

	kube := k8sfake.NewSimpleClientset(envoyService(), envoyNetworkPolicy())

	result, err := Reconcile(ctx, dyn, kube, opts)
	require.NoError(t, err)
	require.True(t, result.Changed())
	require.Len(t, result.Desired, 2)
	assert.Empty(t, result.Conflicts)

	// Gateway gained both managed listeners, kept http
	gw, err := dyn.Resource(gatewayGVR).Namespace("projectcontour").Get(ctx, "contour", metav1.GetOptions{})
	require.NoError(t, err)
	listeners, _, _ := unstructured.NestedSlice(gw.Object, "spec", "listeners")
	gwNames := map[string]bool{}
	for _, raw := range listeners {
		gwNames[raw.(map[string]interface{})["name"].(string)] = true
	}
	assert.True(t, gwNames["http"])
	assert.True(t, gwNames["gw-tls-4987"])
	assert.True(t, gwNames["gw-tcp-6000"])

	// Service gained both ports with the remapped targetPorts, kept the VIP
	svc, err := kube.CoreV1().Services("projectcontour").Get(ctx, "contour-envoy", metav1.GetOptions{})
	require.NoError(t, err)
	assert.Equal(t, []string{"10.16.0.30"}, svc.Spec.ExternalIPs)
	svcPorts := map[string]int32{}   // name -> port
	svcTargets := map[string]int32{} // name -> targetPort
	for _, p := range svc.Spec.Ports {
		svcPorts[p.Name] = p.Port
		svcTargets[p.Name] = p.TargetPort.IntVal
	}
	assert.Equal(t, int32(4987), svcPorts["gw-tls-4987"])
	assert.Equal(t, int32(12987), svcTargets["gw-tls-4987"])
	assert.Equal(t, int32(6000), svcPorts["gw-tcp-6000"])
	assert.Equal(t, int32(14000), svcTargets["gw-tcp-6000"])

	// NetworkPolicy admits both remapped target ports and records ownership
	np, err := kube.NetworkingV1().NetworkPolicies("projectcontour").Get(ctx, "contour-envoy", metav1.GetOptions{})
	require.NoError(t, err)
	assert.True(t, portPresent(np.Spec.Ingress[0].Ports, 12987))
	assert.True(t, portPresent(np.Spec.Ingress[0].Ports, 14000))
	assert.True(t, portPresent(np.Spec.Ingress[0].Ports, 8080), "base port preserved")
	assert.Equal(t, "12987,14000", np.Annotations[managedPortsAnnotation])

	// Second pass is a no-op (idempotent)
	result2, err := Reconcile(ctx, dyn, kube, opts)
	require.NoError(t, err)
	assert.False(t, result2.Changed(), "reconcile should be idempotent")
}

func TestReconcile_OpensListenerFromBackendRefPort(t *testing.T) {
	ctx := context.Background()
	opts := DefaultOptions()
	const api = "gateway.networking.k8s.io/v1alpha2"

	dyn := newFakeDynamic()
	_, err := dyn.Resource(gatewayGVR).Namespace("projectcontour").Create(ctx, gatewayObject(), metav1.CreateOptions{})
	require.NoError(t, err)

	// Two TLSRoutes for distinct SNIs, both deriving 4987 from backendRefs (no
	// parentRef port) -> one shared gw-tls-4987 listener.
	_, err = dyn.Resource(tlsRouteGVR).Namespace("linkkeys").Create(ctx,
		routeWithRefs("TLSRoute", api, "lk-squizzlezig", "linkkeys", 0, 4987), metav1.CreateOptions{})
	require.NoError(t, err)
	_, err = dyn.Resource(tlsRouteGVR).Namespace("linkkeys").Create(ctx,
		routeWithRefs("TLSRoute", api, "lk-todandlorna", "linkkeys", 0, 4987), metav1.CreateOptions{})
	require.NoError(t, err)
	// A TCPRoute deriving its port from the backendRef too.
	_, err = dyn.Resource(tcpRouteGVR).Namespace("apps").Create(ctx,
		routeWithRefs("TCPRoute", api, "raw", "apps", 0, 6000), metav1.CreateOptions{})
	require.NoError(t, err)
	// A route targeting the gateway with no derivable port -> skipped + reported.
	_, err = dyn.Resource(tlsRouteGVR).Namespace("apps").Create(ctx,
		routeWithRefs("TLSRoute", api, "noport", "apps", 0, 0), metav1.CreateOptions{})
	require.NoError(t, err)

	kube := k8sfake.NewSimpleClientset(envoyService(), envoyNetworkPolicy())

	result, err := Reconcile(ctx, dyn, kube, opts)
	require.NoError(t, err)
	require.True(t, result.Changed())
	assert.Empty(t, result.Conflicts)

	require.Len(t, result.Desired, 2)
	assert.Equal(t, DesiredListener{Port: 4987, Protocol: protocolTLS}, result.Desired[0])
	assert.Equal(t, DesiredListener{Port: 6000, Protocol: protocolTCP}, result.Desired[1])

	require.Len(t, result.Skipped, 1)
	assert.Contains(t, result.Skipped[0], "apps/noport")

	gw, err := dyn.Resource(gatewayGVR).Namespace("projectcontour").Get(ctx, "contour", metav1.GetOptions{})
	require.NoError(t, err)
	listeners, _, _ := unstructured.NestedSlice(gw.Object, "spec", "listeners")
	gwNames := map[string]bool{}
	for _, raw := range listeners {
		gwNames[raw.(map[string]interface{})["name"].(string)] = true
	}
	assert.True(t, gwNames["gw-tls-4987"], "shared TLS listener derived from backendRefs")
	assert.True(t, gwNames["gw-tcp-6000"])
}

func TestReconcile_ReservedBackendPortRefused(t *testing.T) {
	ctx := context.Background()
	opts := DefaultOptions()
	const api = "gateway.networking.k8s.io/v1alpha2"

	dyn := newFakeDynamic()
	_, err := dyn.Resource(gatewayGVR).Namespace("projectcontour").Create(ctx, gatewayObject(), metav1.CreateOptions{})
	require.NoError(t, err)
	// A reserved port (443) derived from the backendRef is still refused.
	_, err = dyn.Resource(tlsRouteGVR).Namespace("apps").Create(ctx,
		routeWithRefs("TLSRoute", api, "https", "apps", 0, 443), metav1.CreateOptions{})
	require.NoError(t, err)

	kube := k8sfake.NewSimpleClientset(envoyService(), envoyNetworkPolicy())

	result, err := Reconcile(ctx, dyn, kube, opts)
	require.NoError(t, err)
	assert.Empty(t, result.Desired)
	require.Len(t, result.Conflicts, 1)
	assert.Contains(t, result.Conflicts[0], "443")
}

func TestReconcile_PrunesWhenRoutesRemoved(t *testing.T) {
	ctx := context.Background()
	opts := DefaultOptions()

	// Gateway/Service/NP already carry a managed entry for port 4987 but no
	// routes exist anymore -> the managed entry must be pruned.
	dyn := newFakeDynamic()
	gw := gatewayObject()
	listeners, _, _ := unstructured.NestedSlice(gw.Object, "spec", "listeners")
	listeners = append(listeners, gatewayListenerObject(DesiredListener{Port: 4987, Protocol: protocolTLS}))
	_ = unstructured.SetNestedSlice(gw.Object, listeners, "spec", "listeners")
	_, err := dyn.Resource(gatewayGVR).Namespace("projectcontour").Create(ctx, gw, metav1.CreateOptions{})
	require.NoError(t, err)

	svc := envoyService()
	svc.Spec.Ports = append(svc.Spec.Ports, corev1.ServicePort{
		Name: "gw-tls-4987", Port: 4987, TargetPort: intstr.FromInt32(12987), Protocol: corev1.ProtocolTCP,
	})
	np := envoyNetworkPolicy()
	np.Spec.Ingress[0].Ports = append(np.Spec.Ingress[0].Ports, networkPolicyTCPPort(12987))
	np.Annotations = map[string]string{managedPortsAnnotation: "12987"}
	kube := k8sfake.NewSimpleClientset(svc, np)

	result, err := Reconcile(ctx, dyn, kube, opts)
	require.NoError(t, err)
	require.True(t, result.Changed())
	assert.Empty(t, result.Desired)

	gotGw, _ := dyn.Resource(gatewayGVR).Namespace("projectcontour").Get(ctx, "contour", metav1.GetOptions{})
	gotListeners, _, _ := unstructured.NestedSlice(gotGw.Object, "spec", "listeners")
	for _, raw := range gotListeners {
		assert.NotEqual(t, "gw-tls-4987", raw.(map[string]interface{})["name"])
	}

	gotSvc, _ := kube.CoreV1().Services("projectcontour").Get(ctx, "contour-envoy", metav1.GetOptions{})
	for _, p := range gotSvc.Spec.Ports {
		assert.NotEqual(t, "gw-tls-4987", p.Name)
	}

	gotNp, _ := kube.NetworkingV1().NetworkPolicies("projectcontour").Get(ctx, "contour-envoy", metav1.GetOptions{})
	assert.False(t, portPresent(gotNp.Spec.Ingress[0].Ports, 12987), "pruned target removed from netpol")
	assert.True(t, portPresent(gotNp.Spec.Ingress[0].Ports, 8080))
	assert.Empty(t, gotNp.Annotations[managedPortsAnnotation])
}
