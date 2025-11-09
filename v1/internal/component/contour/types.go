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
	List(ctx context.Context, namespace string) ([]helm.Release, error)
}

// K8sClient defines the Kubernetes operations needed for Contour
type K8sClient interface {
	GetPods(ctx context.Context, namespace string) ([]*k8s.Pod, error)
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
	return []string{"k3s"} // Contour depends on Kubernetes being available
}

// Config type is generated from CSIL in types.gen.go
// This file extends the generated type with methods

// DefaultConfig returns a Config with sensible defaults
func DefaultConfig() *Config {
	return &Config{
		Version:             "", // Latest stable version
		Namespace:           "projectcontour",
		ReplicaCount:        2,
		EnvoyReplicaCount:   2,
		UseKubeVIP:          true,  // Enable for bare metal
		DefaultIngressClass: true,  // Set as default
		Values:              make(map[string]interface{}),
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

	if values, ok := cfg.GetMap("values"); ok {
		config.Values = values
	}

	return config, nil
}
