// Package discovery reports stack links and Gateway API exposure.
package discovery

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"

	"github.com/catalystcommunity/foundry/v1/internal/config"
	"github.com/catalystcommunity/foundry/v1/internal/host"
	"github.com/catalystcommunity/foundry/v1/internal/k8s"
	networkingv1 "k8s.io/api/networking/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
)

var gatewayGVR = schema.GroupVersionResource{Group: "gateway.networking.k8s.io", Version: "v1", Resource: "gateways"}

var routeResources = []struct {
	Kind string
	GVR  schema.GroupVersionResource
}{
	{"HTTPRoute", schema.GroupVersionResource{Group: "gateway.networking.k8s.io", Version: "v1", Resource: "httproutes"}},
	{"GRPCRoute", schema.GroupVersionResource{Group: "gateway.networking.k8s.io", Version: "v1", Resource: "grpcroutes"}},
	{"TLSRoute", schema.GroupVersionResource{Group: "gateway.networking.k8s.io", Version: "v1alpha2", Resource: "tlsroutes"}},
	{"TCPRoute", schema.GroupVersionResource{Group: "gateway.networking.k8s.io", Version: "v1alpha2", Resource: "tcproutes"}},
}

type Snapshot struct {
	ClusterAvailable bool              `json:"cluster_available"`
	Services         []ServiceLink     `json:"services"`
	Gateways         []GatewayExposure `json:"gateways"`
	Warnings         []string          `json:"warnings,omitempty"`
}

type ServiceLink struct {
	Name      string `json:"name"`
	Component string `json:"component"`
	URL       string `json:"url"`
	Source    string `json:"source"`
	Namespace string `json:"namespace,omitempty"`
	External  bool   `json:"external"`
}

type GatewayExposure struct {
	Namespace  string     `json:"namespace"`
	Name       string     `json:"name"`
	ClassName  string     `json:"class_name"`
	Addresses  []string   `json:"addresses,omitempty"`
	Programmed bool       `json:"programmed"`
	Listeners  []Listener `json:"listeners"`
	Routes     []Route    `json:"routes"`
}

type Listener struct {
	Name           string `json:"name"`
	Protocol       string `json:"protocol"`
	Port           int64  `json:"port"`
	Hostname       string `json:"hostname,omitempty"`
	AttachedRoutes int64  `json:"attached_routes"`
}

type Route struct {
	Kind      string   `json:"kind"`
	Namespace string   `json:"namespace"`
	Name      string   `json:"name"`
	Hostnames []string `json:"hostnames,omitempty"`
	Backends  []string `json:"backends,omitempty"`
	Accepted  bool     `json:"accepted"`
	URLs      []string `json:"urls,omitempty"`
}

// Inspect returns configured links even when Kubernetes is not available.
func Inspect(ctx context.Context, cfg *config.Config, kubeconfigPath string) Snapshot {
	snapshot := Snapshot{Services: configuredLinks(cfg)}
	kubeconfig, err := os.ReadFile(kubeconfigPath)
	if err != nil {
		snapshot.Warnings = append(snapshot.Warnings, fmt.Sprintf("Cluster discovery is unavailable: %v", err))
		return snapshot
	}
	client, err := k8s.NewClientFromKubeconfig(kubeconfig)
	if err != nil {
		snapshot.Warnings = append(snapshot.Warnings, fmt.Sprintf("Cluster discovery is unavailable: %v", err))
		return snapshot
	}
	return InspectClients(ctx, cfg, client.Clientset(), client.DynamicClient())
}

// InspectClients builds a snapshot from Kubernetes interfaces.
func InspectClients(ctx context.Context, cfg *config.Config, kube kubernetes.Interface, dyn dynamic.Interface) Snapshot {
	snapshot := Snapshot{Services: configuredLinks(cfg)}
	ingresses, err := kube.NetworkingV1().Ingresses(metav1.NamespaceAll).List(ctx, metav1.ListOptions{})
	if err != nil {
		snapshot.Warnings = append(snapshot.Warnings, fmt.Sprintf("Cannot list Ingress resources: %v", err))
	} else {
		snapshot.ClusterAvailable = true
		for i := range ingresses.Items {
			snapshot.Services = append(snapshot.Services, ingressLinks(&ingresses.Items[i])...)
		}
	}
	gateways, err := discoverGateways(ctx, dyn)
	snapshot.Gateways = gateways
	if err != nil {
		snapshot.Warnings = append(snapshot.Warnings, err.Error())
		if gateways != nil {
			snapshot.ClusterAvailable = true
		}
	} else {
		snapshot.ClusterAvailable = true
	}
	snapshot.Services = uniqueLinks(snapshot.Services)
	return snapshot
}

func configuredLinks(cfg *config.Config) []ServiceLink {
	var links []ServiceLink
	for name, component := range cfg.Components {
		settings := component.Config
		switch name {
		case "grafana", "prometheus", "loki":
			links = appendIngressSetting(links, name, displayName(name), settings, "ingress_host")
		case "seaweedfs":
			if boolSetting(settings, "ingress_enabled") {
				links = appendHostLink(links, name, "SeaweedFS filer", stringSetting(settings, "ingress_host_filer"))
				links = appendHostLink(links, name, "SeaweedFS S3", stringSetting(settings, "ingress_host_s3"))
			}
		case "storage":
			longhorn, _ := settings["longhorn"].(map[string]interface{})
			links = appendIngressSetting(links, name, "Longhorn", longhorn, "ingress_host")
		}
	}
	if _, ok := cfg.Components["openbao"]; ok {
		if url, err := cfg.GetPrimaryOpenBAOURL(); err == nil {
			links = append(links, ServiceLink{Name: "OpenBAO", Component: "openbao", URL: url, Source: "configuration", External: true})
		}
	}
	if component, ok := cfg.Components["zot"]; ok {
		port := intSetting(component.Config, "port", 5000)
		for _, configuredHost := range cfg.GetHostsByRole(host.RoleZot) {
			links = append(links, ServiceLink{Name: "Zot registry", Component: "zot", URL: "http://" + configuredHost.Address + ":" + strconv.Itoa(port), Source: "configuration", External: true})
		}
	}
	return uniqueLinks(links)
}

func appendIngressSetting(links []ServiceLink, component, name string, settings map[string]interface{}, field string) []ServiceLink {
	if boolSetting(settings, "ingress_enabled") {
		return appendHostLink(links, component, name, stringSetting(settings, field))
	}
	return links
}

func appendHostLink(links []ServiceLink, component, name, hostname string) []ServiceLink {
	if hostname != "" {
		links = append(links, ServiceLink{Name: name, Component: component, URL: "https://" + hostname, Source: "configuration", External: true})
	}
	return links
}

func ingressLinks(ingress *networkingv1.Ingress) []ServiceLink {
	tlsHosts := map[string]bool{}
	for _, tls := range ingress.Spec.TLS {
		for _, hostname := range tls.Hosts {
			tlsHosts[hostname] = true
		}
	}
	name := displayName(ingress.Name)
	if app := ingress.Labels["app.kubernetes.io/name"]; app != "" {
		name = displayName(app)
	}
	var links []ServiceLink
	for _, rule := range ingress.Spec.Rules {
		if rule.Host == "" {
			continue
		}
		scheme := "http"
		if tlsHosts[rule.Host] {
			scheme = "https"
		}
		links = append(links, ServiceLink{Name: name, Component: ingress.Labels["app.kubernetes.io/name"], URL: scheme + "://" + rule.Host, Source: "Ingress", Namespace: ingress.Namespace, External: true})
	}
	return links
}

func discoverGateways(ctx context.Context, dyn dynamic.Interface) ([]GatewayExposure, error) {
	list, err := dyn.Resource(gatewayGVR).Namespace(metav1.NamespaceAll).List(ctx, metav1.ListOptions{})
	if err != nil {
		if apierrors.IsNotFound(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("cannot list Gateway resources: %w", err)
	}
	gateways := make([]GatewayExposure, 0, len(list.Items))
	for i := range list.Items {
		gateways = append(gateways, gatewayFromObject(&list.Items[i]))
	}
	for _, resource := range routeResources {
		routes, err := dyn.Resource(resource.GVR).Namespace(metav1.NamespaceAll).List(ctx, metav1.ListOptions{})
		if err != nil {
			if apierrors.IsNotFound(err) {
				continue
			}
			return gateways, fmt.Errorf("cannot list %s resources: %w", resource.Kind, err)
		}
		for i := range routes.Items {
			attachRoute(gateways, resource.Kind, &routes.Items[i])
		}
	}
	sort.Slice(gateways, func(i, j int) bool {
		return gateways[i].Namespace+"/"+gateways[i].Name < gateways[j].Namespace+"/"+gateways[j].Name
	})
	return gateways, nil
}

func gatewayFromObject(object *unstructured.Unstructured) GatewayExposure {
	gateway := GatewayExposure{Namespace: object.GetNamespace(), Name: object.GetName()}
	gateway.ClassName, _, _ = unstructured.NestedString(object.Object, "spec", "gatewayClassName")
	addresses, _, _ := unstructured.NestedSlice(object.Object, "status", "addresses")
	for _, raw := range addresses {
		if address, ok := raw.(map[string]interface{}); ok {
			if value, _ := address["value"].(string); value != "" {
				gateway.Addresses = append(gateway.Addresses, value)
			}
		}
	}
	conditions, _, _ := unstructured.NestedSlice(object.Object, "status", "conditions")
	gateway.Programmed = conditionTrue(conditions, "Programmed")
	statusListeners, _, _ := unstructured.NestedSlice(object.Object, "status", "listeners")
	attached := map[string]int64{}
	for _, raw := range statusListeners {
		if status, ok := raw.(map[string]interface{}); ok {
			name, _ := status["name"].(string)
			attached[name], _ = number(status["attachedRoutes"])
		}
	}
	listeners, _, _ := unstructured.NestedSlice(object.Object, "spec", "listeners")
	for _, raw := range listeners {
		listener, ok := raw.(map[string]interface{})
		if !ok {
			continue
		}
		name, _ := listener["name"].(string)
		port, _ := number(listener["port"])
		protocol, _ := listener["protocol"].(string)
		hostname, _ := listener["hostname"].(string)
		gateway.Listeners = append(gateway.Listeners, Listener{Name: name, Port: port, Protocol: protocol, Hostname: hostname, AttachedRoutes: attached[name]})
	}
	return gateway
}

func attachRoute(gateways []GatewayExposure, kind string, object *unstructured.Unstructured) {
	parents, _, _ := unstructured.NestedSlice(object.Object, "spec", "parentRefs")
	for _, raw := range parents {
		parent, ok := raw.(map[string]interface{})
		if !ok {
			continue
		}
		name, _ := parent["name"].(string)
		parentKind, _ := parent["kind"].(string)
		group, _ := parent["group"].(string)
		if (parentKind != "" && parentKind != "Gateway") || (group != "" && group != "gateway.networking.k8s.io") {
			continue
		}
		namespace := object.GetNamespace()
		if value, _ := parent["namespace"].(string); value != "" {
			namespace = value
		}
		for i := range gateways {
			if gateways[i].Name == name && gateways[i].Namespace == namespace {
				gateways[i].Routes = append(gateways[i].Routes, routeFromObject(kind, object, gateways[i]))
			}
		}
	}
}

func routeFromObject(kind string, object *unstructured.Unstructured, gateway GatewayExposure) Route {
	route := Route{Kind: kind, Namespace: object.GetNamespace(), Name: object.GetName()}
	route.Hostnames, _, _ = unstructured.NestedStringSlice(object.Object, "spec", "hostnames")
	rules, _, _ := unstructured.NestedSlice(object.Object, "spec", "rules")
	seen := map[string]bool{}
	for _, rawRule := range rules {
		rule, ok := rawRule.(map[string]interface{})
		if !ok {
			continue
		}
		backends, _ := rule["backendRefs"].([]interface{})
		for _, rawBackend := range backends {
			backend, ok := rawBackend.(map[string]interface{})
			if !ok {
				continue
			}
			name, _ := backend["name"].(string)
			port, _ := number(backend["port"])
			value := name
			if port > 0 {
				value += ":" + strconv.FormatInt(port, 10)
			}
			if value != "" && !seen[value] {
				seen[value] = true
				route.Backends = append(route.Backends, value)
			}
		}
	}
	parents, _, _ := unstructured.NestedSlice(object.Object, "status", "parents")
	for _, raw := range parents {
		if parent, ok := raw.(map[string]interface{}); ok {
			conditions, _ := parent["conditions"].([]interface{})
			route.Accepted = route.Accepted || conditionTrue(conditions, "Accepted")
		}
	}
	if kind == "HTTPRoute" {
		scheme := gatewayScheme(gateway)
		for _, hostname := range route.Hostnames {
			route.URLs = append(route.URLs, scheme+"://"+hostname)
		}
	}
	return route
}

func gatewayScheme(gateway GatewayExposure) string {
	for _, listener := range gateway.Listeners {
		if listener.Protocol == "HTTPS" || listener.Port == 443 {
			return "https"
		}
	}
	return "http"
}

func conditionTrue(raw []interface{}, conditionType string) bool {
	for _, item := range raw {
		condition, ok := item.(map[string]interface{})
		if ok && condition["type"] == conditionType && condition["status"] == "True" {
			return true
		}
	}
	return false
}

func uniqueLinks(links []ServiceLink) []ServiceLink {
	seen := map[string]int{}
	result := make([]ServiceLink, 0, len(links))
	for _, link := range links {
		if link.URL == "" {
			continue
		}
		if index, exists := seen[link.URL]; exists {
			if result[index].Source == "configuration" && link.Source != "configuration" {
				result[index] = link
			}
			continue
		}
		seen[link.URL] = len(result)
		result = append(result, link)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Name+result[i].URL < result[j].Name+result[j].URL })
	return result
}

func boolSetting(values map[string]interface{}, key string) bool {
	value, _ := values[key].(bool)
	return value
}

func stringSetting(values map[string]interface{}, key string) string {
	value, _ := values[key].(string)
	return strings.TrimSpace(value)
}

func intSetting(values map[string]interface{}, key string, fallback int) int {
	value, ok := number(values[key])
	if !ok || value < 1 || value > 65535 {
		return fallback
	}
	return int(value)
}

func number(value interface{}) (int64, bool) {
	switch typed := value.(type) {
	case int:
		return int64(typed), true
	case int32:
		return int64(typed), true
	case int64:
		return typed, true
	case float64:
		return int64(typed), true
	case json.Number:
		value, err := typed.Int64()
		return value, err == nil
	default:
		return 0, false
	}
}

func displayName(value string) string {
	value = strings.ReplaceAll(value, "-", " ")
	if value == "" {
		return "Service"
	}
	return strings.ToUpper(value[:1]) + value[1:]
}
