package contour

import (
	"context"
	"fmt"

	"github.com/catalystcommunity/foundry/v1/internal/component"
	"github.com/catalystcommunity/foundry/v1/internal/helm"
	"github.com/catalystcommunity/foundry/v1/internal/k8s"
)

// HelmClient defines the Helm operations needed for Contour
type HelmClient interface {
	AddRepo(ctx context.Context, opts helm.RepoAddOptions) error
	Install(ctx context.Context, opts helm.InstallOptions) error
	Upgrade(ctx context.Context, opts helm.UpgradeOptions) error
	Uninstall(ctx context.Context, opts helm.UninstallOptions) error
	List(ctx context.Context, namespace string) ([]helm.Release, error)
}

// K8sClient defines the Kubernetes operations needed for Contour
type K8sClient interface {
	GetPods(ctx context.Context, namespace string) ([]*k8s.Pod, error)
	ServiceMonitorCRDExists(ctx context.Context) (bool, error)
	ApplyManifest(ctx context.Context, manifest string) error
	CRDExists(ctx context.Context, crdName string) (bool, error)
	PatchDeploymentArgs(ctx context.Context, namespace, name string, oldArg, newArg string) error
}

// Component implements the component.Component interface for Contour ingress controller
type Component struct {
	helmClient HelmClient
	k8sClient  K8sClient
}

// NewComponent creates a new Contour component instance
func NewComponent(helmClient HelmClient, k8sClient K8sClient) *Component {
	return &Component{
		helmClient: helmClient,
		k8sClient:  k8sClient,
	}
}

// Name returns the component name
func (c *Component) Name() string {
	return "contour"
}

// Install installs the Contour ingress controller via Helm
func (c *Component) Install(ctx context.Context, cfg component.ComponentConfig) error {
	config, err := ParseConfig(cfg)
	if err != nil {
		return fmt.Errorf("parse config: %w", err)
	}

	return Install(ctx, c.helmClient, c.k8sClient, config)
}

// Upgrade upgrades the Contour ingress controller to a new version
func (c *Component) Upgrade(ctx context.Context, cfg component.ComponentConfig) error {
	return fmt.Errorf("upgrade not yet implemented")
}

// Status returns the current status of the Contour ingress controller
func (c *Component) Status(ctx context.Context) (*component.ComponentStatus, error) {
	if c.helmClient == nil {
		return &component.ComponentStatus{
			Installed: false,
			Healthy:   false,
			Message:   "helm client not initialized",
		}, nil
	}

	// Check if Contour is installed by querying Helm releases
	releases, err := c.helmClient.List(ctx, "projectcontour")
	if err != nil {
		return &component.ComponentStatus{
			Installed: false,
			Healthy:   false,
			Message:   fmt.Sprintf("failed to list releases: %v", err),
		}, nil
	}

	// Look for contour release
	var contourRelease *helm.Release
	for i := range releases {
		if releases[i].Name == "contour" {
			contourRelease = &releases[i]
			break
		}
	}

	if contourRelease == nil {
		return &component.ComponentStatus{
			Installed: false,
			Healthy:   false,
			Message:   "contour release not found",
		}, nil
	}

	// Check if pods are running
	pods, err := c.k8sClient.GetPods(ctx, "projectcontour")
	if err != nil {
		return &component.ComponentStatus{
			Installed: true,
			Version:   contourRelease.AppVersion,
			Healthy:   false,
			Message:   fmt.Sprintf("failed to get pods: %v", err),
		}, nil
	}

	// Count running pods
	runningPods := 0
	for _, pod := range pods {
		if pod.Status == "Running" {
			runningPods++
		}
	}

	// Healthy means all pods are running and release is deployed
	healthy := len(pods) > 0 && runningPods == len(pods) && contourRelease.Status == "deployed"
	message := fmt.Sprintf("%d/%d pods running", runningPods, len(pods))
	if !healthy {
		message = fmt.Sprintf("release status: %s, %s", contourRelease.Status, message)
	}

	return &component.ComponentStatus{
		Installed: true,
		Version:   contourRelease.AppVersion,
		Healthy:   healthy,
		Message:   message,
	}, nil
}

// Uninstall removes the Contour ingress controller
func (c *Component) Uninstall(ctx context.Context) error {
	return fmt.Errorf("uninstall not yet implemented")
}

// Dependencies returns the list of components that Contour depends on
func (c *Component) Dependencies() []string {
	return []string{"k3s", "gateway-api"} // Contour depends on Kubernetes and Gateway API CRDs
}

// Config type is generated from CSIL in types.gen.go
// This file extends the generated type with methods

// Runtime overrides (passed from stack config, not persisted in component config)
var (
	// clusterVIPOverride stores the cluster VIP for LoadBalancer annotation
	clusterVIPOverride string
	// gatewayDomainOverride stores the domain for Gateway TLS certificate
	gatewayDomainOverride string
)

// SetClusterVIP sets the cluster VIP for the next installation
func SetClusterVIP(vip string) {
	clusterVIPOverride = vip
}

// GetClusterVIP returns the cluster VIP if set
func GetClusterVIP() string {
	return clusterVIPOverride
}

// SetGatewayDomain sets the domain for Gateway TLS certificate
func SetGatewayDomain(domain string) {
	gatewayDomainOverride = domain
}

// GetGatewayDomain returns the gateway domain if set
func GetGatewayDomain() string {
	return gatewayDomainOverride
}

// DefaultConfig returns a Config with sensible defaults
func DefaultConfig() *Config {
	return &Config{
		Version:                  "0.1.0", // Official Project Contour chart version (app version 1.32.0)
		Namespace:                "projectcontour",
		ReplicaCount:             2,
		EnvoyReplicaCount:        2,
		UseKubeVIP:               true,     // Enable for bare metal
		DefaultIngressClass:      true,     // Set as default
		GatewayAPIVersion:        "v1.3.0", // Gateway API version
		ServiceMonitorEnabled:    true,     // Enable ServiceMonitor for Prometheus (requires CRD)
		CreateGateway:            true,     // Create GatewayClass and Gateway resources
		CreateGatewayCertificate: true,     // Create TLS Certificate for HTTPS listener
		Values:                   make(map[string]interface{}),
	}
}

// ParseConfig parses a ComponentConfig into a Contour Config
func ParseConfig(cfg component.ComponentConfig) (*Config, error) {
	config := DefaultConfig()

	if version, ok := cfg.GetString("version"); ok {
		config.Version = version
	}

	if namespace, ok := cfg.GetString("namespace"); ok {
		config.Namespace = namespace
	}

	if replicas, ok := cfg.GetInt("replica_count"); ok {
		config.ReplicaCount = uint64(replicas)
	}

	if envoyReplicas, ok := cfg.GetInt("envoy_replica_count"); ok {
		config.EnvoyReplicaCount = uint64(envoyReplicas)
	}

	if useKubeVIP, ok := cfg.GetBool("use_kubevip"); ok {
		config.UseKubeVIP = useKubeVIP
	}

	if defaultClass, ok := cfg.GetBool("default_ingress_class"); ok {
		config.DefaultIngressClass = defaultClass
	}

	if gatewayAPIVersion, ok := cfg.GetString("gateway_api_version"); ok {
		config.GatewayAPIVersion = gatewayAPIVersion
	}

	if serviceMonitorEnabled, ok := cfg.GetBool("service_monitor_enabled"); ok {
		config.ServiceMonitorEnabled = serviceMonitorEnabled
	}

	if createGateway, ok := cfg.GetBool("create_gateway"); ok {
		config.CreateGateway = createGateway
	}

	if createGatewayCertificate, ok := cfg.GetBool("create_gateway_certificate"); ok {
		config.CreateGatewayCertificate = createGatewayCertificate
	}

	// Additional L4 (TCP/TLS) listeners
	if raw, ok := cfg.Get("listeners"); ok {
		if listeners, ok := raw.([]interface{}); ok {
			for _, entry := range listeners {
				m, ok := entry.(map[string]interface{})
				if !ok {
					continue
				}
				l := ContourListener{}
				if name, ok := m["name"].(string); ok {
					l.Name = name
				}
				if protocol, ok := m["protocol"].(string); ok {
					l.Protocol = protocol
				}
				switch port := m["port"].(type) {
				case int:
					l.Port = uint64(port)
				case float64:
					l.Port = uint64(port)
				}
				if tlsMode, ok := m["tls_mode"].(string); ok {
					l.TLSMode = &tlsMode
				}
				if hostname, ok := m["hostname"].(string); ok {
					l.Hostname = &hostname
				}
				if certRef, ok := m["certificate_ref"].(string); ok {
					l.CertificateRef = &certRef
				}
				config.Listeners = append(config.Listeners, l)
			}
		}
	}

	if values, ok := cfg.GetMap("values"); ok {
		config.Values = values
	}

	// Store cluster VIP in the override (used by buildHelmValues)
	if clusterVIP, ok := cfg.GetString("cluster_vip"); ok {
		SetClusterVIP(clusterVIP)
	}

	// Store gateway domain in the override (used by createGatewayResources)
	if gatewayDomain, ok := cfg.GetString("gateway_domain"); ok {
		SetGatewayDomain(gatewayDomain)
	}

	return config, nil
}

// Listener protocol values supported for custom L4 listeners.
const (
	ListenerProtocolTCP = "TCP"
	ListenerProtocolTLS = "TLS"

	// TLSModePassthrough routes by SNI without terminating TLS (use with TLSRoute).
	TLSModePassthrough = "Passthrough"
	// TLSModeTerminate terminates TLS at the Gateway (requires a certificate secret).
	TLSModeTerminate = "Terminate"

	// envoyPortOffset is the fixed offset Contour applies when mapping a Gateway
	// listener port to the Envoy container port. See:
	// https://projectcontour.io/docs/main/config/gateway-api/
	envoyPortOffset = 8000
)

// EffectiveTLSMode returns the TLS mode for a listener, defaulting to
// Passthrough when unset. Meaningless for TCP listeners.
func (l ContourListener) EffectiveTLSMode() string {
	if l.TLSMode != nil && *l.TLSMode != "" {
		return *l.TLSMode
	}
	return TLSModePassthrough
}

// EnvoyContainerPort returns the port Envoy actually binds for this listener.
func (l ContourListener) EnvoyContainerPort() uint64 {
	return EnvoyContainerPort(l.Port)
}

// EnvoyContainerPort returns the port Envoy binds for a given Gateway listener
// port. Contour maps a listener port by adding 8000, wrapping above 65535 and
// lifting privileged results above 1023. This is the single source of truth for
// the remap, shared by the static listener config and the gateway controller.
func EnvoyContainerPort(listenerPort uint64) uint64 {
	p := listenerPort + envoyPortOffset
	if p > 65535 {
		p -= 65535
	}
	if p <= 1023 {
		p += 1023
	}
	return p
}

// ValidateListeners checks that each custom listener is well-formed and that
// names and ports do not collide (including with the built-in HTTP/HTTPS ports).
func (c *Config) ValidateListeners() error {
	seenNames := map[string]bool{}
	seenPorts := map[uint64]bool{
		80:  true, // built-in HTTP listener
		443: true, // built-in HTTPS listener
	}
	for i, l := range c.Listeners {
		if l.Name == "" {
			return fmt.Errorf("listener[%d]: name is required", i)
		}
		if seenNames[l.Name] {
			return fmt.Errorf("listener %q: duplicate name", l.Name)
		}
		seenNames[l.Name] = true

		switch l.Protocol {
		case ListenerProtocolTCP, ListenerProtocolTLS:
		default:
			return fmt.Errorf("listener %q: protocol must be %q or %q, got %q",
				l.Name, ListenerProtocolTCP, ListenerProtocolTLS, l.Protocol)
		}

		if l.Port < 1 || l.Port > 65535 {
			return fmt.Errorf("listener %q: port %d out of range (1-65535)", l.Name, l.Port)
		}
		if seenPorts[l.Port] {
			return fmt.Errorf("listener %q: port %d already in use (80/443 are reserved for the built-in HTTP/HTTPS listeners)", l.Name, l.Port)
		}
		seenPorts[l.Port] = true

		if l.Protocol == ListenerProtocolTLS {
			switch l.EffectiveTLSMode() {
			case TLSModePassthrough:
			case TLSModeTerminate:
				if l.CertificateRef == nil || *l.CertificateRef == "" {
					return fmt.Errorf("listener %q: certificate_ref is required for TLS Terminate mode", l.Name)
				}
			default:
				return fmt.Errorf("listener %q: tls_mode must be %q or %q", l.Name, TLSModePassthrough, TLSModeTerminate)
			}
		}
	}
	return nil
}
