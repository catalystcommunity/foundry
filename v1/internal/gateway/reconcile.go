// Package gateway implements a route-driven reconciler that opens L4
// (TCP/TLS) listeners on the shared Contour Gateway based on TLSRoute and
// TCPRoute resources. Apps declare intent purely in their own chart by
// creating a route whose parentRef targets the Contour Gateway; the listener
// port comes from the parentRef's port or, when that is omitted, the route's
// backendRefs port. This reconciler programs the matching Gateway listener, the Envoy service
// port (exposed on the cluster VIP via the existing externalIPs), and the
// Envoy NetworkPolicy ingress so traffic actually flows.
//
// It owns only the entries it creates: Gateway listeners and Envoy service
// ports are named with the "gw-" prefix, and NetworkPolicy ports it manages
// are tracked via an annotation. The built-in HTTP/HTTPS listeners and any
// operator-pinned static listeners are never touched.
package gateway

import (
	"context"
	"fmt"
	"reflect"
	"sort"
	"strconv"
	"strings"

	"github.com/catalystcommunity/foundry/v1/internal/component/contour"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
)

const (
	// managedPrefix marks Gateway listeners and Envoy service ports created by
	// this reconciler so it can prune its own entries without disturbing the
	// built-in or operator-pinned ones.
	managedPrefix = "gw-"

	// managedPortsAnnotation records, on the Envoy NetworkPolicy, the set of
	// target ports this reconciler manages (NetworkPolicy ports are unnamed, so
	// ownership is tracked here instead of by name).
	managedPortsAnnotation = "gateway.foundry.dev/managed-target-ports"

	protocolTLS = "TLS"
	protocolTCP = "TCP"

	// envoyHTTPTargetPort is the Envoy container port for the built-in HTTP
	// listener (80 + 8000). Used to locate the chart's main ingress rule.
	envoyHTTPTargetPort = 8080
)

var (
	gatewayGVR  = schema.GroupVersionResource{Group: "gateway.networking.k8s.io", Version: "v1", Resource: "gateways"}
	tlsRouteGVR = schema.GroupVersionResource{Group: "gateway.networking.k8s.io", Version: "v1alpha2", Resource: "tlsroutes"}
	tcpRouteGVR = schema.GroupVersionResource{Group: "gateway.networking.k8s.io", Version: "v1alpha2", Resource: "tcproutes"}
)

// Options configures which resources the reconciler manages.
type Options struct {
	GatewayName      string
	GatewayNamespace string
	EnvoyService     string
	NetworkPolicy    string
}

// DefaultOptions returns the options matching a Foundry-installed Contour.
func DefaultOptions() Options {
	return Options{
		GatewayName:      "contour",
		GatewayNamespace: "projectcontour",
		EnvoyService:     "contour-envoy",
		NetworkPolicy:    "contour-envoy",
	}
}

// DesiredListener is a single L4 listener derived from the routes.
type DesiredListener struct {
	Port     int32
	Protocol string // TLS (Passthrough) or TCP
}

// Result reports what a reconcile pass found and changed.
type Result struct {
	Desired              []DesiredListener
	Conflicts            []string
	Skipped              []string // routes seen targeting the Gateway but ignored, with the reason
	GatewayUpdated       bool
	ServiceUpdated       bool
	NetworkPolicyUpdated bool
}

// Changed reports whether the pass modified any cluster resource.
func (r *Result) Changed() bool {
	return r.GatewayUpdated || r.ServiceUpdated || r.NetworkPolicyUpdated
}

// routeCandidate is a single (port, protocol) request extracted from a route.
type routeCandidate struct {
	Port     int32
	Protocol string
	Source   string // namespace/name, for diagnostics
}

// Reconcile performs a single reconcile pass: it reads the routes targeting the
// Gateway and converges the Gateway listeners, Envoy service ports, and Envoy
// NetworkPolicy ingress to match.
func Reconcile(ctx context.Context, dyn dynamic.Interface, kube kubernetes.Interface, opts Options) (*Result, error) {
	candidates, skipped, err := collectCandidates(ctx, dyn, opts)
	if err != nil {
		return nil, err
	}
	desired, conflicts := computeDesired(candidates)

	result := &Result{Desired: desired, Conflicts: conflicts, Skipped: skipped}

	if result.GatewayUpdated, err = reconcileGateway(ctx, dyn, opts, desired); err != nil {
		return nil, fmt.Errorf("reconcile gateway: %w", err)
	}
	if result.ServiceUpdated, err = reconcileService(ctx, kube, opts, desired); err != nil {
		return nil, fmt.Errorf("reconcile envoy service: %w", err)
	}
	if result.NetworkPolicyUpdated, err = reconcileNetworkPolicy(ctx, kube, opts, desired); err != nil {
		return nil, fmt.Errorf("reconcile network policy: %w", err)
	}
	return result, nil
}

func listenerName(d DesiredListener) string {
	return fmt.Sprintf("%s%s-%d", managedPrefix, strings.ToLower(d.Protocol), d.Port)
}

func routeKind(protocol string) string {
	if protocol == protocolTLS {
		return "TLSRoute"
	}
	return "TCPRoute"
}

// targetPort returns the Envoy container port for a listener port (the +8000
// Contour remap), shared with the static-listener config path.
func targetPort(port int32) int32 {
	return int32(contour.EnvoyContainerPort(uint64(port)))
}

// TargetPortFor exposes the listener-port → Envoy-port remap for callers that
// want to report where a listener lands (e.g. the controller command).
func TargetPortFor(port int32) int32 {
	return targetPort(port)
}

// collectCandidates lists TLSRoutes and TCPRoutes cluster-wide and extracts the
// (port, protocol) each one requests of the target Gateway. It also returns the
// routes that target the Gateway but yield no candidate, each with a reason, so
// the caller can surface them instead of dropping them silently.
func collectCandidates(ctx context.Context, dyn dynamic.Interface, opts Options) ([]routeCandidate, []string, error) {
	sources := []struct {
		gvr      schema.GroupVersionResource
		protocol string
	}{
		{tlsRouteGVR, protocolTLS},
		{tcpRouteGVR, protocolTCP},
	}

	var candidates []routeCandidate
	var skipped []string
	for _, s := range sources {
		list, err := dyn.Resource(s.gvr).Namespace(metav1.NamespaceAll).List(ctx, metav1.ListOptions{})
		if err != nil {
			// A missing route CRD just means no such routes exist yet.
			if apierrors.IsNotFound(err) {
				continue
			}
			return nil, nil, fmt.Errorf("list %s: %w", s.gvr.Resource, err)
		}
		for i := range list.Items {
			c, skip := extractCandidates(&list.Items[i], s.protocol, opts)
			candidates = append(candidates, c...)
			skipped = append(skipped, skip...)
		}
	}
	return candidates, skipped, nil
}

// extractCandidates pulls the parentRefs that target the Gateway and returns a
// candidate per ref. The listener port comes from the parentRef's own `port`
// when set; otherwise it falls back to the route's backend service port, which
// for these L4 passthrough routes equals the desired listener port (so apps can
// declare it once, on the backendRef). A parentRef that targets the Gateway but
// yields no port is reported in the second return value rather than dropped.
func extractCandidates(route *unstructured.Unstructured, protocol string, opts Options) ([]routeCandidate, []string) {
	routeNS := route.GetNamespace()
	source := routeNS + "/" + route.GetName()

	parentRefs, found, err := unstructured.NestedSlice(route.Object, "spec", "parentRefs")
	if !found || err != nil {
		return nil, nil
	}

	// Derive the backend port once; it backs every parentRef on this route that
	// omits an explicit port.
	fallback, fallbackOK, fallbackReason := backendFallbackPort(route)

	var out []routeCandidate
	var skipped []string
	for _, raw := range parentRefs {
		ref, ok := raw.(map[string]interface{})
		if !ok {
			continue
		}
		name, _ := ref["name"].(string)
		ns := routeNS
		if v, ok := ref["namespace"].(string); ok && v != "" {
			ns = v
		}
		if name != opts.GatewayName || ns != opts.GatewayNamespace {
			continue
		}
		port, ok := toInt32(ref["port"])
		if !ok {
			// No explicit parentRef port: fall back to the backend service port.
			if !fallbackOK {
				skipped = append(skipped, fmt.Sprintf(
					"%s: targets Gateway %s/%s but no listener port could be derived (%s)",
					source, opts.GatewayNamespace, opts.GatewayName, fallbackReason))
				continue
			}
			port = fallback
		}
		out = append(out, routeCandidate{Port: port, Protocol: protocol, Source: source})
	}
	return out, skipped
}

// backendFallbackPort derives a listener port from a route's backend service
// ports. For the L4 passthrough routes this controller handles, the backend
// port equals the desired listener port, so an app can declare the port once on
// its `rules[].backendRefs[].port` and leave it off the parentRef. It returns
// the single distinct backend port, or ok=false with a reason when none is set
// or the backends disagree on the port (ambiguous — refuse to guess).
func backendFallbackPort(route *unstructured.Unstructured) (int32, bool, string) {
	rules, found, err := unstructured.NestedSlice(route.Object, "spec", "rules")
	if !found || err != nil {
		return 0, false, "no backendRefs port set"
	}

	seen := map[int32]bool{}
	var first int32
	for _, rawRule := range rules {
		rule, ok := rawRule.(map[string]interface{})
		if !ok {
			continue
		}
		backendRefs, ok := rule["backendRefs"].([]interface{})
		if !ok {
			continue
		}
		for _, rawRef := range backendRefs {
			ref, ok := rawRef.(map[string]interface{})
			if !ok {
				continue
			}
			port, ok := toInt32(ref["port"])
			if !ok {
				continue
			}
			if len(seen) == 0 {
				first = port
			}
			seen[port] = true
		}
	}

	switch len(seen) {
	case 0:
		return 0, false, "no backendRefs port set"
	case 1:
		return first, true, ""
	default:
		return 0, false, "backendRefs declare multiple distinct ports"
	}
}

// computeDesired deduplicates candidates into a sorted set of listeners and
// reports conflicts (reserved ports, or a port claimed by both TLS and TCP).
func computeDesired(candidates []routeCandidate) ([]DesiredListener, []string) {
	reserved := map[int32]bool{80: true, 443: true}
	byPort := map[int32]string{}
	conflicted := map[int32]bool{}
	var conflicts []string

	for _, c := range candidates {
		if reserved[c.Port] {
			conflicts = append(conflicts, fmt.Sprintf("%s: port %d is reserved for the built-in HTTP/HTTPS listener; ignoring", c.Source, c.Port))
			continue
		}
		existing, seen := byPort[c.Port]
		if !seen {
			byPort[c.Port] = c.Protocol
			continue
		}
		if existing != c.Protocol && !conflicted[c.Port] {
			conflicted[c.Port] = true
			conflicts = append(conflicts, fmt.Sprintf("port %d requested as both %s and %s; ignoring", c.Port, existing, c.Protocol))
		}
	}

	desired := make([]DesiredListener, 0, len(byPort))
	for port, protocol := range byPort {
		if conflicted[port] {
			continue
		}
		desired = append(desired, DesiredListener{Port: port, Protocol: protocol})
	}
	sort.Slice(desired, func(i, j int) bool { return desired[i].Port < desired[j].Port })
	return desired, conflicts
}

func reconcileGateway(ctx context.Context, dyn dynamic.Interface, opts Options, desired []DesiredListener) (bool, error) {
	client := dyn.Resource(gatewayGVR).Namespace(opts.GatewayNamespace)
	gw, err := client.Get(ctx, opts.GatewayName, metav1.GetOptions{})
	if err != nil {
		return false, fmt.Errorf("get gateway %s/%s: %w", opts.GatewayNamespace, opts.GatewayName, err)
	}

	existing, _, err := unstructured.NestedSlice(gw.Object, "spec", "listeners")
	if err != nil {
		return false, fmt.Errorf("read gateway listeners: %w", err)
	}
	merged, changed := mergeGatewayListeners(existing, desired)
	if !changed {
		return false, nil
	}
	if err := unstructured.SetNestedSlice(gw.Object, merged, "spec", "listeners"); err != nil {
		return false, fmt.Errorf("set gateway listeners: %w", err)
	}
	if _, err := client.Update(ctx, gw, metav1.UpdateOptions{}); err != nil {
		return false, fmt.Errorf("update gateway: %w", err)
	}
	return true, nil
}

// mergeGatewayListeners keeps every non-managed listener untouched and replaces
// the managed (gw-) listeners with the desired set.
func mergeGatewayListeners(existing []interface{}, desired []DesiredListener) ([]interface{}, bool) {
	kept := make([]interface{}, 0, len(existing))
	for _, raw := range existing {
		m, ok := raw.(map[string]interface{})
		if ok {
			if name, _ := m["name"].(string); strings.HasPrefix(name, managedPrefix) {
				continue
			}
		}
		kept = append(kept, raw)
	}
	for _, d := range desired {
		kept = append(kept, gatewayListenerObject(d))
	}
	return kept, !reflect.DeepEqual(existing, kept)
}

// gatewayListenerObject builds an unstructured Gateway listener for a desired
// L4 listener. TLS listeners use Passthrough mode and allow TLSRoute (SNI-based
// routing); TCP listeners allow TCPRoute. Numbers are int64 so the value
// round-trips through the dynamic client.
func gatewayListenerObject(d DesiredListener) map[string]interface{} {
	obj := map[string]interface{}{
		"name":     listenerName(d),
		"port":     int64(d.Port),
		"protocol": d.Protocol,
		"allowedRoutes": map[string]interface{}{
			"kinds": []interface{}{
				map[string]interface{}{"kind": routeKind(d.Protocol)},
			},
			"namespaces": map[string]interface{}{"from": "All"},
		},
	}
	if d.Protocol == protocolTLS {
		obj["tls"] = map[string]interface{}{"mode": "Passthrough"}
	}
	return obj
}

func reconcileService(ctx context.Context, kube kubernetes.Interface, opts Options, desired []DesiredListener) (bool, error) {
	svcClient := kube.CoreV1().Services(opts.GatewayNamespace)
	svc, err := svcClient.Get(ctx, opts.EnvoyService, metav1.GetOptions{})
	if err != nil {
		return false, fmt.Errorf("get service %s/%s: %w", opts.GatewayNamespace, opts.EnvoyService, err)
	}

	merged, changed := mergeServicePorts(svc.Spec.Ports, desired)
	if !changed {
		return false, nil
	}
	svc.Spec.Ports = merged
	if _, err := svcClient.Update(ctx, svc, metav1.UpdateOptions{}); err != nil {
		return false, fmt.Errorf("update service: %w", err)
	}
	return true, nil
}

// mergeServicePorts keeps every non-managed port and replaces the managed (gw-)
// ports with one per desired listener, targeting the remapped Envoy port.
func mergeServicePorts(existing []corev1.ServicePort, desired []DesiredListener) ([]corev1.ServicePort, bool) {
	kept := make([]corev1.ServicePort, 0, len(existing))
	for _, p := range existing {
		if strings.HasPrefix(p.Name, managedPrefix) {
			continue
		}
		kept = append(kept, p)
	}
	for _, d := range desired {
		kept = append(kept, corev1.ServicePort{
			Name:       listenerName(d),
			Port:       d.Port,
			TargetPort: intstr.FromInt32(targetPort(d.Port)),
			Protocol:   corev1.ProtocolTCP,
		})
	}
	return kept, !reflect.DeepEqual(existing, kept)
}

func reconcileNetworkPolicy(ctx context.Context, kube kubernetes.Interface, opts Options, desired []DesiredListener) (bool, error) {
	// An empty name disables NetworkPolicy reconciliation (e.g. when the Envoy
	// NetworkPolicy is absent or managed elsewhere).
	if opts.NetworkPolicy == "" {
		return false, nil
	}

	npClient := kube.NetworkingV1().NetworkPolicies(opts.GatewayNamespace)
	np, err := npClient.Get(ctx, opts.NetworkPolicy, metav1.GetOptions{})
	if err != nil {
		// No NetworkPolicy means nothing is gating the traffic; that's fine.
		if apierrors.IsNotFound(err) {
			return false, nil
		}
		return false, fmt.Errorf("get network policy %s/%s: %w", opts.GatewayNamespace, opts.NetworkPolicy, err)
	}

	prior := parsePorts(np.Annotations[managedPortsAnnotation])
	desiredTargets := targetPortsOf(desired)

	updated, changed := mergeNetworkPolicy(np.DeepCopy(), prior, desiredTargets)
	if !changed {
		return false, nil
	}
	if _, err := npClient.Update(ctx, updated, metav1.UpdateOptions{}); err != nil {
		return false, fmt.Errorf("update network policy: %w", err)
	}
	return true, nil
}

// mergeNetworkPolicy ensures the ingress rule that admits Envoy traffic also
// admits every desired target port, removes target ports it previously managed
// that are no longer desired, and records the managed set in an annotation.
func mergeNetworkPolicy(np *networkingv1.NetworkPolicy, prior, desired []int32) (*networkingv1.NetworkPolicy, bool) {
	changed := false

	idx := ingressRuleIndex(np.Spec.Ingress, envoyHTTPTargetPort)
	if idx == -1 {
		// No existing rule to extend: add a dedicated allow-from-anywhere rule.
		if len(desired) > 0 {
			np.Spec.Ingress = append(np.Spec.Ingress, networkingv1.NetworkPolicyIngressRule{
				Ports: portsFor(desired),
			})
			changed = true
		}
	} else {
		newPorts, ruleChanged := mergeRulePorts(np.Spec.Ingress[idx].Ports, prior, desired)
		if ruleChanged {
			np.Spec.Ingress[idx].Ports = newPorts
			changed = true
		}
	}

	newAnno := joinPorts(desired)
	if np.Annotations[managedPortsAnnotation] != newAnno {
		if np.Annotations == nil {
			np.Annotations = map[string]string{}
		}
		if newAnno == "" {
			delete(np.Annotations, managedPortsAnnotation)
		} else {
			np.Annotations[managedPortsAnnotation] = newAnno
		}
		changed = true
	}
	return np, changed
}

// mergeRulePorts adds desired ports not already present and removes ports that
// were previously managed but are no longer desired, preserving every other
// entry exactly as-is to avoid churn.
func mergeRulePorts(existing []networkingv1.NetworkPolicyPort, prior, desired []int32) ([]networkingv1.NetworkPolicyPort, bool) {
	desiredSet := intSet(desired)
	removeSet := map[int32]bool{}
	for _, p := range prior {
		if !desiredSet[p] {
			removeSet[p] = true
		}
	}

	changed := false
	result := make([]networkingv1.NetworkPolicyPort, 0, len(existing)+len(desired))
	for _, np := range existing {
		if v, ok := portValue(np); ok && removeSet[v] {
			changed = true
			continue
		}
		result = append(result, np)
	}
	for _, d := range desired {
		if !portPresent(result, d) {
			result = append(result, networkPolicyTCPPort(d))
			changed = true
		}
	}
	return result, changed
}

// ingressRuleIndex returns the index of the ingress rule that admits the given
// port, or -1 if none does.
func ingressRuleIndex(rules []networkingv1.NetworkPolicyIngressRule, port int32) int {
	for i, rule := range rules {
		if portPresent(rule.Ports, port) {
			return i
		}
	}
	return -1
}

func portsFor(ports []int32) []networkingv1.NetworkPolicyPort {
	out := make([]networkingv1.NetworkPolicyPort, 0, len(ports))
	for _, p := range ports {
		out = append(out, networkPolicyTCPPort(p))
	}
	return out
}

func networkPolicyTCPPort(port int32) networkingv1.NetworkPolicyPort {
	tcp := corev1.ProtocolTCP
	p := intstr.FromInt32(port)
	return networkingv1.NetworkPolicyPort{Protocol: &tcp, Port: &p}
}

func portPresent(ports []networkingv1.NetworkPolicyPort, port int32) bool {
	for _, np := range ports {
		if v, ok := portValue(np); ok && v == port {
			return true
		}
	}
	return false
}

func portValue(np networkingv1.NetworkPolicyPort) (int32, bool) {
	if np.Port == nil || np.Port.Type != intstr.Int {
		return 0, false
	}
	return np.Port.IntVal, true
}

// targetPortsOf returns the sorted, unique Envoy target ports for the desired
// listeners.
func targetPortsOf(desired []DesiredListener) []int32 {
	seen := map[int32]bool{}
	var out []int32
	for _, d := range desired {
		t := targetPort(d.Port)
		if !seen[t] {
			seen[t] = true
			out = append(out, t)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}

func intSet(vals []int32) map[int32]bool {
	s := make(map[int32]bool, len(vals))
	for _, v := range vals {
		s[v] = true
	}
	return s
}

func joinPorts(ports []int32) string {
	if len(ports) == 0 {
		return ""
	}
	parts := make([]string, len(ports))
	for i, p := range ports {
		parts[i] = strconv.Itoa(int(p))
	}
	return strings.Join(parts, ",")
}

func parsePorts(s string) []int32 {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	var out []int32
	for _, part := range strings.Split(s, ",") {
		v, err := strconv.Atoi(strings.TrimSpace(part))
		if err != nil {
			continue
		}
		out = append(out, int32(v))
	}
	return out
}

func toInt32(v interface{}) (int32, bool) {
	switch n := v.(type) {
	case int64:
		return int32(n), true
	case int32:
		return n, true
	case int:
		return int32(n), true
	case float64:
		return int32(n), true
	default:
		return 0, false
	}
}
