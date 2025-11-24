package stack

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/catalystcommunity/foundry/v1/internal/component"
	"github.com/catalystcommunity/foundry/v1/internal/config"
	"github.com/catalystcommunity/foundry/v1/internal/host"
	"github.com/catalystcommunity/foundry/v1/internal/setup"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// strPtr is a helper to create string pointers for optional fields
func strPtr(s string) *string {
	return &s
}

// mockComponent implements the Component interface for testing
type mockComponent struct {
	name         string
	dependencies []string
}

func (m *mockComponent) Name() string {
	return m.name
}

func (m *mockComponent) Dependencies() []string {
	return m.dependencies
}

func (m *mockComponent) Install(ctx context.Context, cfg component.ComponentConfig) error {
	return nil
}

func (m *mockComponent) Upgrade(ctx context.Context, cfg component.ComponentConfig) error {
	return nil
}

func (m *mockComponent) Status(ctx context.Context) (*component.ComponentStatus, error) {
	return &component.ComponentStatus{
		Installed: false,
		Version:   "unknown",
		Healthy:   false,
		Message:   "not implemented",
	}, nil
}

func (m *mockComponent) Uninstall(ctx context.Context) error {
	return nil
}

// setupTestRegistry creates a test component registry with mock components
func setupTestRegistry() *component.Registry {
	registry := component.NewRegistry()

	// Register mock components matching the stack installation order
	registry.Register(&mockComponent{name: "openbao", dependencies: []string{}})
	registry.Register(&mockComponent{name: "dns", dependencies: []string{"openbao"}})
	registry.Register(&mockComponent{name: "zot", dependencies: []string{"dns", "openbao"}})
	registry.Register(&mockComponent{name: "k3s", dependencies: []string{"openbao", "dns", "zot"}})
	registry.Register(&mockComponent{name: "contour", dependencies: []string{"k3s"}})
	registry.Register(&mockComponent{name: "certmanager", dependencies: []string{"k3s"}})

	return registry
}

// createTestConfig creates a minimal valid test configuration
func createTestConfig(t *testing.T) *config.Config {
	return &config.Config{
		Network: &config.NetworkConfig{
			Gateway:       "192.168.1.1",
			Netmask:       "255.255.255.0",
		},
		DNS: &config.DNSConfig{
			InfrastructureZones: []config.DNSZone{
				{Name: "infra.local", Public: false},
			},
			KubernetesZones: []config.DNSZone{
				{Name: "k8s.local", Public: false},
			},
			Forwarders: []string{"8.8.8.8", "1.1.1.1"},
			Backend:    "sqlite",
			APIKey:     "${secret:foundry-core/dns:api_key}",
		},
		Hosts: []*host.Host{
			{
				Hostname: "infra1",
				Address:  "192.168.1.10",
				Roles:    []string{host.RoleOpenBAO, host.RoleDNS, host.RoleZot},
				State:    host.StateConfigured,
			},
			{
				Hostname: "node1",
				Address:  "192.168.1.20",
				Roles:    []string{host.RoleClusterControlPlane},
				State:    host.StateConfigured,
			},
		},
		Cluster: config.ClusterConfig{
			Name:   "test-cluster",
			Domain: "example.com",
			VIP:    "192.168.1.100",
		},
		Components: config.ComponentMap{
			"openbao": {},
			"dns":     {},
			"zot":     {},
			"k3s":     {},
		},
	}
}

// writeTestConfigFile creates a temporary config file for testing
func writeTestConfigFile(t *testing.T, cfg *config.Config) string {
	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "stack.yaml")

	err := config.Save(cfg, configPath)
	require.NoError(t, err)

	return configPath
}

func TestValidateStackConfig(t *testing.T) {
	tests := []struct {
		name      string
		cfg       *config.Config
		wantError bool
		errMsg    string
	}{
		{
			name:      "valid configuration",
			cfg:       createTestConfig(t),
			wantError: false,
		},
		{
			name: "missing network gateway",
			cfg: &config.Config{
				Network: &config.NetworkConfig{
					Netmask:      "255.255.255.0",
				},
				DNS: &config.DNSConfig{
					InfrastructureZones: []config.DNSZone{{Name: "infra.local"}},
					KubernetesZones:     []config.DNSZone{{Name: "k8s.local"}},
					Backend:             "sqlite",
					APIKey:              "test-api-key",
				},
				Cluster: config.ClusterConfig{
					Name:   "test",
					Domain: "example.com",
				},
				Components: config.ComponentMap{
					"k3s": {},
				},
			},
			wantError: true,
			errMsg:    "network.gateway is required",
		},
		{
			name: "missing OpenBAO hosts",
			cfg: &config.Config{
				Network: &config.NetworkConfig{
					Gateway: "192.168.1.1",
					Netmask: "255.255.255.0",
				},
				DNS: &config.DNSConfig{
					InfrastructureZones: []config.DNSZone{{Name: "infra.local"}},
					KubernetesZones:     []config.DNSZone{{Name: "k8s.local"}},
					Backend:             "sqlite",
					APIKey:              "test-api-key",
				},
				Cluster: config.ClusterConfig{
					Name:   "test",
					Domain: "example.com",
				},
				Components: config.ComponentMap{
					"k3s": {},
				},
			},
			wantError: true,
			errMsg:    "no hosts with openbao role",
		},
		{
			name: "missing DNS hosts",
			cfg: &config.Config{
				Network: &config.NetworkConfig{
					Gateway: "192.168.1.1",
					Netmask: "255.255.255.0",
				},
				DNS: &config.DNSConfig{
					InfrastructureZones: []config.DNSZone{{Name: "infra.local"}},
					KubernetesZones:     []config.DNSZone{{Name: "k8s.local"}},
					Backend:             "sqlite",
					APIKey:              "test-api-key",
				},
				Cluster: config.ClusterConfig{
					Name:   "test",
					Domain: "example.com",
				},
				Hosts: []*host.Host{
					{Hostname: "openbao1", Address: "192.168.1.10", Roles: []string{host.RoleOpenBAO}},
				},
				Components: config.ComponentMap{
					"k3s": {},
				},
			},
			wantError: true,
			errMsg:    "no hosts with dns role",
		},
		{
			name: "missing K8s VIP",
			cfg: &config.Config{
				Network: &config.NetworkConfig{
					Gateway: "192.168.1.1",
					Netmask: "255.255.255.0",
				},
				DNS: &config.DNSConfig{
					InfrastructureZones: []config.DNSZone{{Name: "infra.local"}},
					KubernetesZones:     []config.DNSZone{{Name: "k8s.local"}},
					Backend:             "sqlite",
					APIKey:              "test-api-key",
				},
				Cluster: config.ClusterConfig{
					Name:   "test",
					Domain: "example.com",
					// VIP is intentionally missing
				},
				Hosts: []*host.Host{
					{Hostname: "openbao1", Address: "192.168.1.10", Roles: []string{host.RoleOpenBAO}},
					{Hostname: "dns1", Address: "192.168.1.11", Roles: []string{host.RoleDNS}},
					{Hostname: "node1", Address: "192.168.1.20", Roles: []string{host.RoleClusterControlPlane}},
				},
				Components: config.ComponentMap{
					"k3s": {},
				},
			},
			wantError: true,
			errMsg:    "vip is required",
		},
		{
			name: "missing infrastructure zones",
			cfg: &config.Config{
				Network: &config.NetworkConfig{
					Gateway:      "192.168.1.1",
					Netmask:      "255.255.255.0",
				},
				DNS: &config.DNSConfig{
					KubernetesZones: []config.DNSZone{{Name: "k8s.local"}},
					Backend:         "sqlite",
					APIKey:          "test-api-key",
					// InfrastructureZones intentionally missing
				},
				Cluster: config.ClusterConfig{
					Name:   "test",
					Domain: "example.com",
					VIP:    "192.168.1.100",
				},
				Hosts: []*host.Host{
					{Hostname: "openbao1", Address: "192.168.1.10", Roles: []string{host.RoleOpenBAO}},
					{Hostname: "dns1", Address: "192.168.1.11", Roles: []string{host.RoleDNS}},
					{Hostname: "node1", Address: "192.168.1.20", Roles: []string{host.RoleClusterControlPlane}},
				},
				Components: config.ComponentMap{
					"k3s": {},
				},
			},
			wantError: true,
			errMsg:    "dns.infrastructure_zones is required",
		},
		{
			name: "missing kubernetes zones",
			cfg: &config.Config{
				Network: &config.NetworkConfig{
					Gateway:      "192.168.1.1",
					Netmask:      "255.255.255.0",
				},
				DNS: &config.DNSConfig{
					InfrastructureZones: []config.DNSZone{{Name: "infra.local"}},
					Backend:             "sqlite",
					APIKey:              "test-api-key",
					// KubernetesZones intentionally missing
				},
				Cluster: config.ClusterConfig{
					Name:   "test",
					Domain: "example.com",
					VIP:    "192.168.1.100",
				},
				Hosts: []*host.Host{
					{Hostname: "openbao1", Address: "192.168.1.10", Roles: []string{host.RoleOpenBAO}},
					{Hostname: "dns1", Address: "192.168.1.11", Roles: []string{host.RoleDNS}},
					{Hostname: "node1", Address: "192.168.1.20", Roles: []string{host.RoleClusterControlPlane}},
				},
				Components: config.ComponentMap{
					"k3s": {},
				},
			},
			wantError: true,
			errMsg:    "dns.kubernetes_zones is required",
		},
		{
			name: "missing cluster name",
			cfg: &config.Config{
				Network: &config.NetworkConfig{
					Gateway:      "192.168.1.1",
					Netmask:      "255.255.255.0",
				},
				DNS: &config.DNSConfig{
					InfrastructureZones: []config.DNSZone{{Name: "infra.local"}},
					KubernetesZones:     []config.DNSZone{{Name: "k8s.local"}},
					Backend:             "sqlite",
					APIKey:              "test-api-key",
				},
				Cluster: config.ClusterConfig{
					Domain: "example.com",
					VIP:    "192.168.1.100",
					// Name intentionally missing
				},
				Hosts: []*host.Host{
					{Hostname: "openbao1", Address: "192.168.1.10", Roles: []string{host.RoleOpenBAO}},
					{Hostname: "dns1", Address: "192.168.1.11", Roles: []string{host.RoleDNS}},
					{Hostname: "node1", Address: "192.168.1.20", Roles: []string{host.RoleClusterControlPlane}},
				},
				Components: config.ComponentMap{
					"k3s": {},
				},
			},
			wantError: true,
			errMsg:    "cluster.name is required",
		},
		{
			name: "missing cluster nodes",
			cfg: &config.Config{
				Network: &config.NetworkConfig{
					Gateway: "192.168.1.1",
					Netmask: "255.255.255.0",
				},
				DNS: &config.DNSConfig{
					InfrastructureZones: []config.DNSZone{{Name: "infra.local"}},
					KubernetesZones:     []config.DNSZone{{Name: "k8s.local"}},
					Backend:             "sqlite",
					APIKey:              "test-api-key",
				},
				Cluster: config.ClusterConfig{
					Name:   "test",
					Domain: "example.com",
					VIP:    "192.168.1.100",
				},
				Hosts: []*host.Host{
					{Hostname: "openbao1", Address: "192.168.1.10", Roles: []string{host.RoleOpenBAO}},
					{Hostname: "dns1", Address: "192.168.1.11", Roles: []string{host.RoleDNS}},
					// No cluster role hosts - intentionally missing
				},
				Components: config.ComponentMap{
					"k3s": {},
				},
			},
			wantError: true,
			errMsg:    "no hosts with cluster roles configured",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateStackConfig(tt.cfg)
			if tt.wantError {
				assert.Error(t, err)
				if tt.errMsg != "" {
					assert.Contains(t, err.Error(), tt.errMsg)
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestDetermineInstallationOrder(t *testing.T) {
	// Save original registry
	origRegistry := component.DefaultRegistry

	// Create test registry
	component.DefaultRegistry = setupTestRegistry()

	// Restore original registry after test
	defer func() {
		component.DefaultRegistry = origRegistry
	}()

	order, err := determineInstallationOrder()
	require.NoError(t, err)

	// Verify we have all expected components (core 4 only, cert-manager/contour are add-ons)
	assert.Len(t, order, 4, "should have 4 core components")
	assert.Contains(t, order, "openbao")
	assert.Contains(t, order, "dns")
	assert.Contains(t, order, "zot")
	assert.Contains(t, order, "k3s")

	// Verify OpenBAO comes before everything else
	openbaoIdx := indexOf(order, "openbao")
	assert.Equal(t, 0, openbaoIdx, "OpenBAO should be first")

	// Verify DNS comes after OpenBAO but before Zot
	dnsIdx := indexOf(order, "dns")
	zotIdx := indexOf(order, "zot")
	assert.Less(t, openbaoIdx, dnsIdx, "OpenBAO should come before DNS")
	assert.Less(t, dnsIdx, zotIdx, "DNS should come before Zot")

	// Verify K3s comes after all infrastructure components
	k3sIdx := indexOf(order, "k3s")
	assert.Less(t, openbaoIdx, k3sIdx, "OpenBAO should come before K3s")
	assert.Less(t, dnsIdx, k3sIdx, "DNS should come before K3s")
	assert.Less(t, zotIdx, k3sIdx, "Zot should come before K3s")
}

func TestPrintStackPlan(t *testing.T) {
	cfg := createTestConfig(t)

	// This test just ensures printStackPlan doesn't panic
	err := printStackPlan(cfg, setup.StepNetworkPlan)
	assert.NoError(t, err)
}

func TestPrintStackPlanWithTrueNAS(t *testing.T) {
	cfg := createTestConfig(t)
	cfg.Hosts = append(cfg.Hosts, &host.Host{
		Hostname: "storage1",
		Address:  "192.168.1.15",
		Roles:    []string{}, // Storage host with no component roles
		State:    host.StateConfigured,
	})

	err := printStackPlan(cfg, setup.StepOpenBAOInstall)
	assert.NoError(t, err)
}

func TestPrintStackPlanWithPublicDNS(t *testing.T) {
	cfg := createTestConfig(t)
	cfg.DNS.InfrastructureZones = []config.DNSZone{
		{Name: "infra.example.com", Public: true, PublicCNAME: strPtr("home.example.com")},
	}
	cfg.DNS.KubernetesZones = []config.DNSZone{
		{Name: "k8s.example.com", Public: true, PublicCNAME: strPtr("home.example.com")},
	}

	err := printStackPlan(cfg, setup.StepDNSInstall)
	assert.NoError(t, err)
}

func TestPrintStackPlanMultipleNodes(t *testing.T) {
	cfg := createTestConfig(t)
	cfg.Hosts = append(cfg.Hosts, &host.Host{
		Hostname: "node3.example.com",
		Address:  "192.168.1.30",
		Roles:    []string{host.RoleClusterWorker},
		State:    host.StateConfigured,
	})

	err := printStackPlan(cfg, setup.StepK8sInstall)
	assert.NoError(t, err)
}

func TestInstallStack(t *testing.T) {
	cfg := createTestConfig(t)
	installOrder := []string{"openbao", "dns", "zot", "k3s", "contour", "certmanager"}

	// Save original registry
	origRegistry := component.DefaultRegistry

	// Create test registry
	component.DefaultRegistry = setupTestRegistry()

	// Restore original registry after test
	defer func() {
		component.DefaultRegistry = origRegistry
	}()

	ctx := context.Background()
	err := installStack(ctx, cfg, installOrder)
	assert.NoError(t, err)
}

func TestInstallStackComponentNotFound(t *testing.T) {
	cfg := createTestConfig(t)
	installOrder := []string{"nonexistent"}

	// Save original registry
	origRegistry := component.DefaultRegistry

	// Create empty test registry
	component.DefaultRegistry = component.NewRegistry()

	// Restore original registry after test
	defer func() {
		component.DefaultRegistry = origRegistry
	}()

	ctx := context.Background()
	err := installStack(ctx, cfg, installOrder)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not found in registry")
}

// Helper function to find index of string in slice
func indexOf(slice []string, item string) int {
	for i, v := range slice {
		if v == item {
			return i
		}
	}
	return -1
}

// TestRunStackInstallDryRun tests the dry-run mode without actually running the CLI
func TestRunStackInstallDryRun(t *testing.T) {
	// Save original registry
	origRegistry := component.DefaultRegistry

	// Create test registry
	component.DefaultRegistry = setupTestRegistry()

	// Restore original registry after test
	defer func() {
		component.DefaultRegistry = origRegistry
	}()

	// Create test config
	cfg := createTestConfig(t)
	configPath := writeTestConfigFile(t, cfg)

	// Set environment variable to suppress os.Stdin prompts in tests
	os.Setenv("FOUNDRY_TEST_MODE", "1")
	defer os.Unsetenv("FOUNDRY_TEST_MODE")

	// Load config
	loadedCfg, err := config.Load(configPath)
	require.NoError(t, err)

	// Ensure SetupState is initialized
	if loadedCfg.SetupState == nil {
		loadedCfg.SetupState = &setup.SetupState{}
	}

	// Test dry-run mode by directly calling printStackPlan
	nextStep := setup.DetermineNextStep(loadedCfg.SetupState)

	// Run dry-run plan
	err = printStackPlan(loadedCfg, nextStep)
	assert.NoError(t, err)
}
