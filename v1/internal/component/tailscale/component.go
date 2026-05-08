package tailscale

import (
	"context"
	"fmt"

	"github.com/catalystcommunity/foundry/v1/internal/component"
	"github.com/catalystcommunity/foundry/v1/internal/helm"
	"github.com/catalystcommunity/foundry/v1/internal/k8s"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/yaml"
)

// Adapters to convert concrete Foundry clients to Tailscale interfaces

// helmClientAdapter adapts *helm.Client to HelmClient interface
type helmClientAdapter struct {
	client *helm.Client
}

func (a *helmClientAdapter) AddRepo(ctx context.Context, opts helm.RepoAddOptions) error {
	return a.client.AddRepo(ctx, opts)
}

func (a *helmClientAdapter) Install(ctx context.Context, opts helm.InstallOptions) error {
	return a.client.Install(ctx, opts)
}

func (a *helmClientAdapter) Uninstall(ctx context.Context, opts helm.UninstallOptions) error {
	return a.client.Uninstall(ctx, opts)
}

// kubeClientAdapter adapts *k8s.Client to KubernetesClient interface
type kubeClientAdapter struct {
	client *k8s.Client
}

func (a *kubeClientAdapter) Apply(ctx context.Context, manifest map[string]interface{}) error {
	// Convert map to YAML string
	yamlBytes, err := yaml.Marshal(manifest)
	if err != nil {
		return fmt.Errorf("failed to marshal manifest to YAML: %w", err)
	}

	// Use k8s client's ApplyManifest method
	return a.client.ApplyManifest(ctx, string(yamlBytes))
}

func (a *kubeClientAdapter) GetServiceIP(ctx context.Context, namespace, name string) (string, error) {
	// Get the service using the underlying clientset
	svc, err := a.client.Clientset().CoreV1().Services(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return "", fmt.Errorf("failed to get service %s/%s: %w", namespace, name, err)
	}

	// Return ClusterIP
	if svc.Spec.ClusterIP == "" {
		return "", fmt.Errorf("service %s/%s has no ClusterIP", namespace, name)
	}

	return svc.Spec.ClusterIP, nil
}

func (a *kubeClientAdapter) GetConfigMap(ctx context.Context, namespace, name string) (*ConfigMap, error) {
	// Get ConfigMap using the underlying clientset
	cm, err := a.client.Clientset().CoreV1().ConfigMaps(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to get ConfigMap %s/%s: %w", namespace, name, err)
	}

	// Convert to our ConfigMap type
	return &ConfigMap{
		Name:      cm.Name,
		Namespace: cm.Namespace,
		Data:      cm.Data,
	}, nil
}

func (a *kubeClientAdapter) UpdateConfigMap(ctx context.Context, cm *ConfigMap) error {
	// Get existing ConfigMap first
	existingCM, err := a.client.Clientset().CoreV1().ConfigMaps(cm.Namespace).Get(ctx, cm.Name, metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("failed to get ConfigMap %s/%s: %w", cm.Namespace, cm.Name, err)
	}

	// Update the data
	existingCM.Data = cm.Data

	// Update ConfigMap using the underlying clientset
	_, err = a.client.Clientset().CoreV1().ConfigMaps(cm.Namespace).Update(ctx, existingCM, metav1.UpdateOptions{})
	if err != nil {
		return fmt.Errorf("failed to update ConfigMap %s/%s: %w", cm.Namespace, cm.Name, err)
	}

	return nil
}

// Component implements the component.Component interface for Tailscale operator
type Component struct {
	config     *Config
	vip        string
	helmClient HelmClient
	kubeClient KubernetesClient
}

// NewComponent creates a new Tailscale component with the given configuration
// vip parameter can be empty - it will be extracted from ComponentConfig during Install
func NewComponent(cfg *Config, vip string) *Component {
	if cfg == nil {
		cfg = &Config{}
	}

	// Set defaults
	cfg.SetDefaults()

	return &Component{
		config: cfg,
		vip:    vip,
	}
}

// NewComponentWithClients creates a new Tailscale component with clients pre-configured
// This is used by the component installer to pass concrete client implementations
// It wraps the concrete types with adapters that implement our interfaces
func NewComponentWithClients(cfg *Config, vip string, helmClient *helm.Client, kubeClient *k8s.Client) *Component {
	if cfg == nil {
		cfg = &Config{}
	}

	// Set defaults
	cfg.SetDefaults()

	// Wrap concrete clients with adapters
	var hc HelmClient
	var kc KubernetesClient
	if helmClient != nil {
		hc = &helmClientAdapter{client: helmClient}
	}
	if kubeClient != nil {
		kc = &kubeClientAdapter{client: kubeClient}
	}

	return &Component{
		config:     cfg,
		vip:        vip,
		helmClient: hc,
		kubeClient: kc,
	}
}

// Name returns the component name
func (c *Component) Name() string {
	return "tailscale"
}

// Dependencies returns the list of components that Tailscale depends on
func (c *Component) Dependencies() []string {
	// Tailscale operator requires a running Kubernetes cluster
	return []string{"k3s"}
}

// Install installs the Tailscale operator component
func (c *Component) Install(ctx context.Context, cfg component.ComponentConfig) error {
	// Use pre-configured clients if available, otherwise get from config
	helmClient := c.helmClient
	kubeClient := c.kubeClient

	// Fallback to getting clients from component config if not pre-configured
	if helmClient == nil {
		if hc, ok := cfg["helm_client"].(HelmClient); ok {
			helmClient = hc
		} else {
			return fmt.Errorf("helm_client not provided")
		}
	}

	if kubeClient == nil {
		if kc, ok := cfg["k8s_client"].(KubernetesClient); ok {
			kubeClient = kc
		} else {
			return fmt.Errorf("k8s_client not provided")
		}
	}

	// Extract VIP from config if not already set
	vip := c.vip
	if vip == "" {
		if vipVal, ok := cfg.GetString("vip"); ok {
			vip = vipVal
		} else {
			return fmt.Errorf("vip is required for Tailscale installation")
		}
	}

	// Use pre-configured config if available, otherwise parse from ComponentConfig
	tailscaleConfig := c.config
	if tailscaleConfig == nil {
		var err error
		tailscaleConfig, err = c.parseConfig(cfg)
		if err != nil {
			return fmt.Errorf("failed to parse Tailscale config: %w", err)
		}
	}

	// Create installer
	installer, err := NewInstaller(tailscaleConfig, vip, helmClient, kubeClient)
	if err != nil {
		return fmt.Errorf("failed to create Tailscale installer: %w", err)
	}

	// Run installation
	if err := installer.Install(ctx); err != nil {
		return fmt.Errorf("Tailscale installation failed: %w", err)
	}

	return nil
}

// Upgrade upgrades the Tailscale operator component
func (c *Component) Upgrade(ctx context.Context, cfg component.ComponentConfig) error {
	// For now, upgrade uses the same logic as install (Helm handles this)
	return c.Install(ctx, cfg)
}

// Status returns the current status of the Tailscale operator
func (c *Component) Status(ctx context.Context) (*component.ComponentStatus, error) {
	// TODO: Implement proper status checking
	// For now, return a basic status
	return &component.ComponentStatus{
		Installed: false,
		Healthy:   false,
		Version:   "",
		Message:   "status checking not yet implemented",
	}, nil
}

// Uninstall removes the Tailscale operator
func (c *Component) Uninstall(ctx context.Context) error {
	// TODO: Implement uninstall
	return fmt.Errorf("uninstall not yet implemented")
}

// Config returns the component configuration
func (c *Component) Config() interface{} {
	return c.config
}

// parseConfig parses Tailscale configuration from component config map
func (c *Component) parseConfig(cfg component.ComponentConfig) (*Config, error) {
	config := &Config{}

	// OAuth credentials (required)
	if clientID, ok := cfg.GetString("oauth_client_id"); ok {
		config.OAuthClientID = &clientID
	}
	if clientSecret, ok := cfg.GetString("oauth_client_secret"); ok {
		config.OAuthClientSecret = &clientSecret
	}

	// Optional fields
	if operatorImage, ok := cfg.GetString("operator_image"); ok {
		config.OperatorImage = &operatorImage
	}
	if routes, ok := cfg.GetStringSlice("advertise_routes"); ok {
		config.AdvertiseRoutes = routes  // Direct assignment, routes is []string
	}
	if tags, ok := cfg.GetStringSlice("tags"); ok {
		config.Tags = tags  // Direct assignment, tags is []string
	}

	// Validate
	if err := config.Validate(); err != nil {
		return nil, fmt.Errorf("invalid Tailscale configuration: %w", err)
	}

	// Set defaults
	config.SetDefaults()

	return config, nil
}
