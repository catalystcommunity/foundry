package tailscale

import (
	"context"
	"fmt"
	"reflect"
	"testing"
)

// mockKubernetesClient is a mock implementation of KubernetesClient for testing.
type mockKubernetesClient struct {
	applyErr        error
	manifestsApplied []map[string]interface{}
}

func (m *mockKubernetesClient) Apply(ctx context.Context, manifest map[string]interface{}) error {
	if m.applyErr != nil {
		return m.applyErr
	}
	m.manifestsApplied = append(m.manifestsApplied, manifest)
	return nil
}

func TestNewCRDInstaller(t *testing.T) {
	tests := []struct {
		name    string
		client  KubernetesClient
		config  *Config
		vip     string
		wantErr bool
		errMsg  string
	}{
		{
			name:    "nil client",
			client:  nil,
			config:  &Config{},
			vip:     "100.81.89.100",
			wantErr: true,
			errMsg:  "kubernetes client cannot be nil",
		},
		{
			name:    "nil config",
			client:  &mockKubernetesClient{},
			config:  nil,
			vip:     "100.81.89.100",
			wantErr: true,
			errMsg:  "config cannot be nil",
		},
		{
			name:    "empty VIP",
			client:  &mockKubernetesClient{},
			config:  &Config{},
			vip:     "",
			wantErr: true,
			errMsg:  "VIP cannot be empty",
		},
		{
			name:    "valid parameters",
			client:  &mockKubernetesClient{},
			config:  &Config{},
			vip:     "100.81.89.100",
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			installer, err := NewCRDInstaller(tt.client, tt.config, tt.vip)
			if (err != nil) != tt.wantErr {
				t.Errorf("NewCRDInstaller() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if err != nil && tt.errMsg != "" && err.Error() != tt.errMsg {
				t.Errorf("NewCRDInstaller() error message = %q, want %q", err.Error(), tt.errMsg)
				return
			}
			if !tt.wantErr && installer == nil {
				t.Error("NewCRDInstaller() returned nil installer without error")
			}
		})
	}
}

func TestCRDInstaller_GenerateConnectorManifest(t *testing.T) {
	tests := []struct {
		name           string
		config         *Config
		vip            string
		wantRoutes     []string
		wantTags       []string
		wantName       string
		wantNamespace  string
		wantAPIVersion string
		wantKind       string
	}{
		{
			name: "minimal config - VIP only",
			config: &Config{
				Tags:            []string{},
				AdvertiseRoutes: []string{},
			},
			vip:            "100.81.89.100",
			wantRoutes:     []string{"100.81.89.100/32"},
			wantTags:       []string{"tag:k8s-foundry"},
			wantName:       "foundry-vip-connector",
			wantNamespace:  "tailscale",
			wantAPIVersion: "tailscale.com/v1alpha1",
			wantKind:       "Connector",
		},
		{
			name: "VIP with additional routes",
			config: &Config{
				Tags:            []string{},
				AdvertiseRoutes: []string{"10.0.0.0/8", "192.168.0.0/16"},
			},
			vip:            "100.81.89.100",
			wantRoutes:     []string{"100.81.89.100/32", "10.0.0.0/8", "192.168.0.0/16"},
			wantTags:       []string{"tag:k8s-foundry"},
			wantName:       "foundry-vip-connector",
			wantNamespace:  "tailscale",
			wantAPIVersion: "tailscale.com/v1alpha1",
			wantKind:       "Connector",
		},
		{
			name: "custom tags",
			config: &Config{
				Tags:            []string{"tag:production", "tag:us-west"},
				AdvertiseRoutes: []string{},
			},
			vip:            "100.81.89.100",
			wantRoutes:     []string{"100.81.89.100/32"},
			wantTags:       []string{"tag:k8s-foundry", "tag:production", "tag:us-west"},
			wantName:       "foundry-vip-connector",
			wantNamespace:  "tailscale",
			wantAPIVersion: "tailscale.com/v1alpha1",
			wantKind:       "Connector",
		},
		{
			name: "full config",
			config: &Config{
				Tags:            []string{"tag:production"},
				AdvertiseRoutes: []string{"10.0.0.0/8"},
			},
			vip:            "100.125.196.1",
			wantRoutes:     []string{"100.125.196.1/32", "10.0.0.0/8"},
			wantTags:       []string{"tag:k8s-foundry", "tag:production"},
			wantName:       "foundry-vip-connector",
			wantNamespace:  "tailscale",
			wantAPIVersion: "tailscale.com/v1alpha1",
			wantKind:       "Connector",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := &mockKubernetesClient{}
			installer, err := NewCRDInstaller(client, tt.config, tt.vip)
			if err != nil {
				t.Fatalf("NewCRDInstaller() unexpected error: %v", err)
			}

			manifest, err := installer.generateConnectorManifest()
			if err != nil {
				t.Fatalf("generateConnectorManifest() unexpected error: %v", err)
			}

			// Verify API version
			if apiVersion, ok := manifest["apiVersion"].(string); !ok || apiVersion != tt.wantAPIVersion {
				t.Errorf("apiVersion = %q, want %q", apiVersion, tt.wantAPIVersion)
			}

			// Verify kind
			if kind, ok := manifest["kind"].(string); !ok || kind != tt.wantKind {
				t.Errorf("kind = %q, want %q", kind, tt.wantKind)
			}

			// Verify metadata
			metadata, ok := manifest["metadata"].(map[string]interface{})
			if !ok {
				t.Fatal("metadata is not a map")
			}
			if name, ok := metadata["name"].(string); !ok || name != tt.wantName {
				t.Errorf("metadata.name = %q, want %q", name, tt.wantName)
			}
			if namespace, ok := metadata["namespace"].(string); !ok || namespace != tt.wantNamespace {
				t.Errorf("metadata.namespace = %q, want %q", namespace, tt.wantNamespace)
			}

			// Verify spec
			spec, ok := manifest["spec"].(map[string]interface{})
			if !ok {
				t.Fatal("spec is not a map")
			}

			// Verify tags
			tags, ok := spec["tags"].([]string)
			if !ok {
				t.Fatal("spec.tags is not a string slice")
			}
			if !reflect.DeepEqual(tags, tt.wantTags) {
				t.Errorf("spec.tags = %v, want %v", tags, tt.wantTags)
			}

			// Verify subnet router
			subnetRouter, ok := spec["subnetRouter"].(map[string]interface{})
			if !ok {
				t.Fatal("spec.subnetRouter is not a map")
			}
			routes, ok := subnetRouter["advertiseRoutes"].([]string)
			if !ok {
				t.Fatal("spec.subnetRouter.advertiseRoutes is not a string slice")
			}
			if !reflect.DeepEqual(routes, tt.wantRoutes) {
				t.Errorf("spec.subnetRouter.advertiseRoutes = %v, want %v", routes, tt.wantRoutes)
			}
		})
	}
}

func TestCRDInstaller_GenerateDNSConfigManifest(t *testing.T) {
	tests := []struct {
		name           string
		wantName       string
		wantNamespace  string
		wantAPIVersion string
		wantKind       string
		wantImageRepo  string
		wantImageTag   string
	}{
		{
			name:           "default DNSConfig",
			wantName:       "ts-dns",
			wantNamespace:  "tailscale",
			wantAPIVersion: "tailscale.com/v1alpha1",
			wantKind:       "DNSConfig",
			wantImageRepo:  "tailscale/k8s-nameserver",
			wantImageTag:   "unstable",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := &mockKubernetesClient{}
			config := &Config{
				Tags:            []string{},
				AdvertiseRoutes: []string{},
			}
			installer, err := NewCRDInstaller(client, config, "100.81.89.100")
			if err != nil {
				t.Fatalf("NewCRDInstaller() unexpected error: %v", err)
			}

			manifest, err := installer.generateDNSConfigManifest()
			if err != nil {
				t.Fatalf("generateDNSConfigManifest() unexpected error: %v", err)
			}

			// Verify API version
			if apiVersion, ok := manifest["apiVersion"].(string); !ok || apiVersion != tt.wantAPIVersion {
				t.Errorf("apiVersion = %q, want %q", apiVersion, tt.wantAPIVersion)
			}

			// Verify kind
			if kind, ok := manifest["kind"].(string); !ok || kind != tt.wantKind {
				t.Errorf("kind = %q, want %q", kind, tt.wantKind)
			}

			// Verify metadata
			metadata, ok := manifest["metadata"].(map[string]interface{})
			if !ok {
				t.Fatal("metadata is not a map")
			}
			if name, ok := metadata["name"].(string); !ok || name != tt.wantName {
				t.Errorf("metadata.name = %q, want %q", name, tt.wantName)
			}
			if namespace, ok := metadata["namespace"].(string); !ok || namespace != tt.wantNamespace {
				t.Errorf("metadata.namespace = %q, want %q", namespace, tt.wantNamespace)
			}

			// Verify spec
			spec, ok := manifest["spec"].(map[string]interface{})
			if !ok {
				t.Fatal("spec is not a map")
			}

			// Verify nameserver
			nameserver, ok := spec["nameserver"].(map[string]interface{})
			if !ok {
				t.Fatal("spec.nameserver is not a map")
			}

			// Verify image
			image, ok := nameserver["image"].(map[string]interface{})
			if !ok {
				t.Fatal("spec.nameserver.image is not a map")
			}
			if repo, ok := image["repo"].(string); !ok || repo != tt.wantImageRepo {
				t.Errorf("spec.nameserver.image.repo = %q, want %q", repo, tt.wantImageRepo)
			}
			if tag, ok := image["tag"].(string); !ok || tag != tt.wantImageTag {
				t.Errorf("spec.nameserver.image.tag = %q, want %q", tag, tt.wantImageTag)
			}
		})
	}
}

func TestCRDInstaller_DeployConnector(t *testing.T) {
	tests := []struct {
		name      string
		client    *mockKubernetesClient
		config    *Config
		vip       string
		wantErr   bool
		errMsg    string
		wantCalls int
	}{
		{
			name: "successful deployment",
			client: &mockKubernetesClient{
				manifestsApplied: []map[string]interface{}{},
			},
			config: &Config{
				Tags:            []string{},
				AdvertiseRoutes: []string{},
			},
			vip:       "100.81.89.100",
			wantErr:   false,
			wantCalls: 1,
		},
		{
			name: "apply error",
			client: &mockKubernetesClient{
				applyErr: fmt.Errorf("kubernetes API error"),
			},
			config: &Config{
				Tags:            []string{},
				AdvertiseRoutes: []string{},
			},
			vip:     "100.81.89.100",
			wantErr: true,
			errMsg:  "failed to apply Connector CRD: kubernetes API error",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			installer, err := NewCRDInstaller(tt.client, tt.config, tt.vip)
			if err != nil {
				t.Fatalf("NewCRDInstaller() unexpected error: %v", err)
			}

			err = installer.DeployConnector(context.Background())
			if (err != nil) != tt.wantErr {
				t.Errorf("DeployConnector() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if err != nil && tt.errMsg != "" && err.Error() != tt.errMsg {
				t.Errorf("DeployConnector() error message = %q, want %q", err.Error(), tt.errMsg)
				return
			}

			if !tt.wantErr {
				if len(tt.client.manifestsApplied) != tt.wantCalls {
					t.Errorf("Apply() called %d times, want %d", len(tt.client.manifestsApplied), tt.wantCalls)
				}

				// Verify Connector manifest was applied
				if len(tt.client.manifestsApplied) > 0 {
					manifest := tt.client.manifestsApplied[0]
					if kind, ok := manifest["kind"].(string); !ok || kind != "Connector" {
						t.Errorf("Applied manifest kind = %q, want %q", kind, "Connector")
					}
				}
			}
		})
	}
}

func TestCRDInstaller_DeployDNSConfig(t *testing.T) {
	tests := []struct {
		name      string
		client    *mockKubernetesClient
		config    *Config
		vip       string
		wantErr   bool
		errMsg    string
		wantCalls int
	}{
		{
			name: "successful deployment",
			client: &mockKubernetesClient{
				manifestsApplied: []map[string]interface{}{},
			},
			config: &Config{
				Tags:            []string{},
				AdvertiseRoutes: []string{},
			},
			vip:       "100.81.89.100",
			wantErr:   false,
			wantCalls: 1,
		},
		{
			name: "apply error",
			client: &mockKubernetesClient{
				applyErr: fmt.Errorf("kubernetes API error"),
			},
			config: &Config{
				Tags:            []string{},
				AdvertiseRoutes: []string{},
			},
			vip:     "100.81.89.100",
			wantErr: true,
			errMsg:  "failed to apply DNSConfig CRD: kubernetes API error",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			installer, err := NewCRDInstaller(tt.client, tt.config, tt.vip)
			if err != nil {
				t.Fatalf("NewCRDInstaller() unexpected error: %v", err)
			}

			err = installer.DeployDNSConfig(context.Background())
			if (err != nil) != tt.wantErr {
				t.Errorf("DeployDNSConfig() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if err != nil && tt.errMsg != "" && err.Error() != tt.errMsg {
				t.Errorf("DeployDNSConfig() error message = %q, want %q", err.Error(), tt.errMsg)
				return
			}

			if !tt.wantErr {
				if len(tt.client.manifestsApplied) != tt.wantCalls {
					t.Errorf("Apply() called %d times, want %d", len(tt.client.manifestsApplied), tt.wantCalls)
				}

				// Verify DNSConfig manifest was applied
				if len(tt.client.manifestsApplied) > 0 {
					manifest := tt.client.manifestsApplied[0]
					if kind, ok := manifest["kind"].(string); !ok || kind != "DNSConfig" {
						t.Errorf("Applied manifest kind = %q, want %q", kind, "DNSConfig")
					}
				}
			}
		})
	}
}

func TestCRDInstaller_Integration(t *testing.T) {
	// Integration test: Deploy both Connector and DNSConfig
	client := &mockKubernetesClient{
		manifestsApplied: []map[string]interface{}{},
	}
	config := &Config{
		Tags:            []string{"tag:production"},
		AdvertiseRoutes: []string{"10.0.0.0/8"},
	}
	vip := "100.81.89.100"

	installer, err := NewCRDInstaller(client, config, vip)
	if err != nil {
		t.Fatalf("NewCRDInstaller() unexpected error: %v", err)
	}

	// Deploy Connector
	if err := installer.DeployConnector(context.Background()); err != nil {
		t.Fatalf("DeployConnector() unexpected error: %v", err)
	}

	// Deploy DNSConfig
	if err := installer.DeployDNSConfig(context.Background()); err != nil {
		t.Fatalf("DeployDNSConfig() unexpected error: %v", err)
	}

	// Verify both manifests were applied
	if len(client.manifestsApplied) != 2 {
		t.Fatalf("Expected 2 manifests applied, got %d", len(client.manifestsApplied))
	}

	// Verify first manifest is Connector
	connectorManifest := client.manifestsApplied[0]
	if kind, ok := connectorManifest["kind"].(string); !ok || kind != "Connector" {
		t.Errorf("First manifest kind = %q, want %q", kind, "Connector")
	}

	// Verify second manifest is DNSConfig
	dnsConfigManifest := client.manifestsApplied[1]
	if kind, ok := dnsConfigManifest["kind"].(string); !ok || kind != "DNSConfig" {
		t.Errorf("Second manifest kind = %q, want %q", kind, "DNSConfig")
	}
}
