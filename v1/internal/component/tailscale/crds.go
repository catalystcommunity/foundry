package tailscale

import (
	"context"
	"fmt"
)

// ConfigMap represents a Kubernetes ConfigMap resource.
type ConfigMap struct {
	Name      string
	Namespace string
	Data      map[string]string
}

// KubernetesClient defines the interface for Kubernetes operations needed by Tailscale installer.
// This interface allows for easier testing with mock implementations.
type KubernetesClient interface {
	Apply(ctx context.Context, manifest map[string]interface{}) error
	GetServiceIP(ctx context.Context, namespace, name string) (string, error)
	GetConfigMap(ctx context.Context, namespace, name string) (*ConfigMap, error)
	UpdateConfigMap(ctx context.Context, cm *ConfigMap) error
}

// CRDInstaller handles CRD deployment for Tailscale operator.
type CRDInstaller struct {
	client KubernetesClient
	config *Config
	vip    string
}

// NewCRDInstaller creates a new CRD installer for Tailscale.
func NewCRDInstaller(client KubernetesClient, config *Config, vip string) (*CRDInstaller, error) {
	if client == nil {
		return nil, fmt.Errorf("kubernetes client cannot be nil")
	}
	if config == nil {
		return nil, fmt.Errorf("config cannot be nil")
	}
	if vip == "" {
		return nil, fmt.Errorf("VIP cannot be empty")
	}

	return &CRDInstaller{
		client: client,
		config: config,
		vip:    vip,
	}, nil
}

// DeployConnector deploys the Tailscale Connector CRD for subnet route advertisement.
func (c *CRDInstaller) DeployConnector(ctx context.Context) error {
	connector, err := c.generateConnectorManifest()
	if err != nil {
		return fmt.Errorf("failed to generate Connector manifest: %w", err)
	}

	if err := c.client.Apply(ctx, connector); err != nil {
		return fmt.Errorf("failed to apply Connector CRD: %w", err)
	}

	return nil
}

// DeployDNSConfig deploys the Tailscale DNSConfig CRD for Magic DNS.
func (c *CRDInstaller) DeployDNSConfig(ctx context.Context) error {
	dnsConfig, err := c.generateDNSConfigManifest()
	if err != nil {
		return fmt.Errorf("failed to generate DNSConfig manifest: %w", err)
	}

	if err := c.client.Apply(ctx, dnsConfig); err != nil {
		return fmt.Errorf("failed to apply DNSConfig CRD: %w", err)
	}

	return nil
}

// generateConnectorManifest creates the Connector CRD manifest.
func (c *CRDInstaller) generateConnectorManifest() (map[string]interface{}, error) {
	// Build advertised routes: VIP + any additional routes from config
	routes := []string{fmt.Sprintf("%s/32", c.vip)}
	if c.config.AdvertiseRoutes != nil {
		routes = append(routes, c.config.AdvertiseRoutes...)
	}

	// Build tags: default tag + any additional tags from config
	tags := []string{"tag:k8s-foundry"}
	if c.config.Tags != nil {
		tags = append(tags, c.config.Tags...)
	}

	connector := map[string]interface{}{
		"apiVersion": "tailscale.com/v1alpha1",
		"kind":       "Connector",
		"metadata": map[string]interface{}{
			"name":      "foundry-vip-connector",
			"namespace": DefaultNamespace,
		},
		"spec": map[string]interface{}{
			"tags": tags,
			"subnetRouter": map[string]interface{}{
				"advertiseRoutes": routes,
			},
		},
	}

	return connector, nil
}

// generateDNSConfigManifest creates the DNSConfig CRD manifest.
func (c *CRDInstaller) generateDNSConfigManifest() (map[string]interface{}, error) {
	dnsConfig := map[string]interface{}{
		"apiVersion": "tailscale.com/v1alpha1",
		"kind":       "DNSConfig",
		"metadata": map[string]interface{}{
			"name":      "ts-dns",
			"namespace": DefaultNamespace,
		},
		"spec": map[string]interface{}{
			"nameserver": map[string]interface{}{
				"image": map[string]interface{}{
					"repo": "tailscale/k8s-nameserver",
					"tag":  "unstable",
				},
			},
		},
	}

	return dnsConfig, nil
}
