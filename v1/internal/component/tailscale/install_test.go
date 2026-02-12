package tailscale

import (
	"context"
	"fmt"
	"testing"

	"github.com/catalystcommunity/foundry/v1/internal/helm"
)

// mockHelmClientForInstaller is a simple mock for testing installer orchestration
type mockHelmClientForInstaller struct {
	addRepoErr      error
	installErr      error
	uninstallErr    error
	addRepoCalled   bool
	installCalled   bool
	uninstallCalled bool
}

func (m *mockHelmClientForInstaller) AddRepo(ctx context.Context, opts helm.RepoAddOptions) error {
	m.addRepoCalled = true
	return m.addRepoErr
}

func (m *mockHelmClientForInstaller) Install(ctx context.Context, opts helm.InstallOptions) error {
	m.installCalled = true
	return m.installErr
}

func (m *mockHelmClientForInstaller) Uninstall(ctx context.Context, opts helm.UninstallOptions) error {
	m.uninstallCalled = true
	return m.uninstallErr
}

// mockKubeClientForInstaller is a simple mock for testing installer orchestration
type mockKubeClientForInstaller struct {
	applyErr             error
	getServiceIPErr      error
	getConfigMapErr      error
	updateConfigMapErr   error
	applyCalled          int
	getServiceIPCalled   bool
	getConfigMapCalled   bool
	updateConfigMapCalled bool
	serviceIP            string
	configMap            *ConfigMap
}

func (m *mockKubeClientForInstaller) Apply(ctx context.Context, manifest map[string]interface{}) error {
	m.applyCalled++
	return m.applyErr
}

func (m *mockKubeClientForInstaller) GetServiceIP(ctx context.Context, namespace, name string) (string, error) {
	m.getServiceIPCalled = true
	if m.getServiceIPErr != nil {
		return "", m.getServiceIPErr
	}
	return m.serviceIP, nil
}

func (m *mockKubeClientForInstaller) GetConfigMap(ctx context.Context, namespace, name string) (*ConfigMap, error) {
	m.getConfigMapCalled = true
	if m.getConfigMapErr != nil {
		return nil, m.getConfigMapErr
	}
	return m.configMap, nil
}

func (m *mockKubeClientForInstaller) UpdateConfigMap(ctx context.Context, cm *ConfigMap) error {
	m.updateConfigMapCalled = true
	return m.updateConfigMapErr
}

func TestNewInstaller(t *testing.T) {
	tests := []struct {
		name       string
		config     *Config
		vip        string
		helmClient HelmClient
		kubeClient KubernetesClient
		wantErr    bool
		errMsg     string
	}{
		{
			name:       "nil config",
			config:     nil,
			vip:        "100.81.89.100",
			helmClient: &mockHelmClientForInstaller{},
			kubeClient: &mockKubeClientForInstaller{},
			wantErr:    true,
			errMsg:     "config cannot be nil",
		},
		{
			name: "nil helm client",
			config: &Config{
				OAuthClientID:     stringPtr("client-123"),
				OAuthClientSecret: stringPtr("secret-456"),
			},
			vip:        "100.81.89.100",
			helmClient: nil,
			kubeClient: &mockKubeClientForInstaller{},
			wantErr:    true,
			errMsg:     "helm client cannot be nil",
		},
		{
			name: "nil kube client",
			config: &Config{
				OAuthClientID:     stringPtr("client-123"),
				OAuthClientSecret: stringPtr("secret-456"),
			},
			vip:        "100.81.89.100",
			helmClient: &mockHelmClientForInstaller{},
			kubeClient: nil,
			wantErr:    true,
			errMsg:     "kubernetes client cannot be nil",
		},
		{
			name: "invalid config - missing oauth_client_id",
			config: &Config{
				OAuthClientSecret: stringPtr("secret-456"),
			},
			vip:        "100.81.89.100",
			helmClient: &mockHelmClientForInstaller{},
			kubeClient: &mockKubeClientForInstaller{},
			wantErr:    true,
			errMsg:     "invalid configuration: oauth_client_id is required",
		},
		{
			name: "valid config",
			config: &Config{
				OAuthClientID:     stringPtr("client-123"),
				OAuthClientSecret: stringPtr("secret-456"),
			},
			vip:        "100.81.89.100",
			helmClient: &mockHelmClientForInstaller{},
			kubeClient: &mockKubeClientForInstaller{},
			wantErr:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			installer, err := NewInstaller(tt.config, tt.vip, tt.helmClient, tt.kubeClient)
			if (err != nil) != tt.wantErr {
				t.Errorf("NewInstaller() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if err != nil && tt.errMsg != "" && err.Error() != tt.errMsg {
				t.Errorf("NewInstaller() error message = %q, want %q", err.Error(), tt.errMsg)
				return
			}
			if !tt.wantErr && installer == nil {
				t.Error("NewInstaller() returned nil installer without error")
			}
			if !tt.wantErr {
				// Verify sub-installers were created
				if installer.helmInstaller == nil {
					t.Error("helmInstaller not initialized")
				}
				if installer.crdInstaller == nil {
					t.Error("crdInstaller not initialized")
				}
				if installer.coreDNSPatcher == nil {
					t.Error("coreDNSPatcher not initialized")
				}
			}
		})
	}
}

func TestNewInstaller_SetsDefaults(t *testing.T) {
	config := &Config{
		OAuthClientID:     stringPtr("client-123"),
		OAuthClientSecret: stringPtr("secret-456"),
	}
	helmClient := &mockHelmClientForInstaller{}
	kubeClient := &mockKubeClientForInstaller{}

	installer, err := NewInstaller(config, "100.81.89.100", helmClient, kubeClient)
	if err != nil {
		t.Fatalf("NewInstaller() unexpected error: %v", err)
	}

	// Verify defaults were set
	if installer.config.OperatorImage == nil {
		t.Error("Expected OperatorImage to be set by defaults")
	}
	if installer.config.Tags == nil || len(installer.config.Tags) == 0 {
		t.Error("Expected Tags to be set by defaults")
	}
	if installer.config.AdvertiseRoutes == nil {
		t.Error("Expected AdvertiseRoutes to be initialized by defaults")
	}
}

func TestInstaller_Install_Success(t *testing.T) {
	config := &Config{
		OAuthClientID:     stringPtr("client-123"),
		OAuthClientSecret: stringPtr("secret-456"),
	}
	helmClient := &mockHelmClientForInstaller{}
	kubeClient := &mockKubeClientForInstaller{
		serviceIP: "10.96.0.100",
		configMap: &ConfigMap{
			Name:      "coredns",
			Namespace: "kube-system",
			Data: map[string]string{
				"Corefile": `.:53 {
    errors
    health
}`,
			},
		},
	}

	installer, err := NewInstaller(config, "100.81.89.100", helmClient, kubeClient)
	if err != nil {
		t.Fatalf("NewInstaller() unexpected error: %v", err)
	}

	// Run install
	err = installer.Install(context.Background())
	if err != nil {
		t.Fatalf("Install() unexpected error: %v", err)
	}

	// Verify all steps were called in order
	if !helmClient.addRepoCalled {
		t.Error("Helm AddRepo was not called")
	}
	if !helmClient.installCalled {
		t.Error("Helm Install was not called")
	}
	if kubeClient.applyCalled < 3 {
		// Should be called for: namespace, connector CRD, DNSConfig CRD
		t.Errorf("Kube Apply called %d times, want at least 3", kubeClient.applyCalled)
	}
	if !kubeClient.getServiceIPCalled {
		t.Error("GetServiceIP was not called for CoreDNS patching")
	}
	if !kubeClient.getConfigMapCalled {
		t.Error("GetConfigMap was not called for CoreDNS patching")
	}
	if !kubeClient.updateConfigMapCalled {
		t.Error("UpdateConfigMap was not called for CoreDNS patching")
	}
}

func TestInstaller_Install_FailureScenarios(t *testing.T) {
	baseConfig := &Config{
		OAuthClientID:     stringPtr("client-123"),
		OAuthClientSecret: stringPtr("secret-456"),
	}

	tests := []struct {
		name         string
		helmClient   *mockHelmClientForInstaller
		kubeClient   *mockKubeClientForInstaller
		wantErrContains string
	}{
		{
			name: "helm add repo failure",
			helmClient: &mockHelmClientForInstaller{
				addRepoErr: fmt.Errorf("repo add failed"),
			},
			kubeClient: &mockKubeClientForInstaller{},
			wantErrContains: "failed to add Helm repository",
		},
		{
			name: "helm install failure",
			helmClient: &mockHelmClientForInstaller{
				installErr: fmt.Errorf("install failed"),
			},
			kubeClient: &mockKubeClientForInstaller{},
			wantErrContains: "failed to install operator",
		},
		{
			name:       "connector CRD apply failure",
			helmClient: &mockHelmClientForInstaller{},
			kubeClient: &mockKubeClientForInstaller{
				applyErr: fmt.Errorf("apply failed"),
			},
			wantErrContains: "failed to create namespace",
		},
		{
			name:       "coredns patch failure - service IP",
			helmClient: &mockHelmClientForInstaller{},
			kubeClient: &mockKubeClientForInstaller{
				getServiceIPErr: fmt.Errorf("service not found"),
			},
			wantErrContains: "failed to patch CoreDNS",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			installer, err := NewInstaller(baseConfig, "100.81.89.100", tt.helmClient, tt.kubeClient)
			if err != nil {
				t.Fatalf("NewInstaller() unexpected error: %v", err)
			}

			err = installer.Install(context.Background())
			if err == nil {
				t.Fatal("Install() expected error, got nil")
			}
			if tt.wantErrContains != "" && !contains(err.Error(), tt.wantErrContains) {
				t.Errorf("Install() error = %q, want to contain %q", err.Error(), tt.wantErrContains)
			}
		})
	}
}

func TestInstaller_Uninstall(t *testing.T) {
	config := &Config{
		OAuthClientID:     stringPtr("client-123"),
		OAuthClientSecret: stringPtr("secret-456"),
	}

	tests := []struct {
		name       string
		helmClient *mockHelmClientForInstaller
		wantErr    bool
		errContains string
	}{
		{
			name:       "successful uninstall",
			helmClient: &mockHelmClientForInstaller{},
			wantErr:    false,
		},
		{
			name: "uninstall error",
			helmClient: &mockHelmClientForInstaller{
				uninstallErr: fmt.Errorf("uninstall failed"),
			},
			wantErr:     true,
			errContains: "failed to uninstall operator",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			kubeClient := &mockKubeClientForInstaller{}
			installer, err := NewInstaller(config, "100.81.89.100", tt.helmClient, kubeClient)
			if err != nil {
				t.Fatalf("NewInstaller() unexpected error: %v", err)
			}

			err = installer.Uninstall(context.Background())
			if (err != nil) != tt.wantErr {
				t.Errorf("Uninstall() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if err != nil && tt.errContains != "" && !contains(err.Error(), tt.errContains) {
				t.Errorf("Uninstall() error = %q, want to contain %q", err.Error(), tt.errContains)
			}
			if !tt.wantErr && !tt.helmClient.uninstallCalled {
				t.Error("Helm Uninstall was not called")
			}
		})
	}
}

func TestInstaller_Status(t *testing.T) {
	config := &Config{
		OAuthClientID:     stringPtr("client-123"),
		OAuthClientSecret: stringPtr("secret-456"),
	}
	helmClient := &mockHelmClientForInstaller{}
	kubeClient := &mockKubeClientForInstaller{}

	installer, err := NewInstaller(config, "100.81.89.100", helmClient, kubeClient)
	if err != nil {
		t.Fatalf("NewInstaller() unexpected error: %v", err)
	}

	status, err := installer.Status(context.Background())
	if err != nil {
		t.Errorf("Status() unexpected error: %v", err)
	}
	if status == nil {
		t.Fatal("Status() returned nil status")
	}
	if !status.Installed {
		t.Error("Status.Installed = false, want true")
	}
	if status.Namespace != DefaultNamespace {
		t.Errorf("Status.Namespace = %q, want %q", status.Namespace, DefaultNamespace)
	}
}

func TestInstaller_CreateNamespace(t *testing.T) {
	config := &Config{
		OAuthClientID:     stringPtr("client-123"),
		OAuthClientSecret: stringPtr("secret-456"),
	}

	tests := []struct {
		name       string
		kubeClient *mockKubeClientForInstaller
		wantErr    bool
	}{
		{
			name:       "successful creation",
			kubeClient: &mockKubeClientForInstaller{},
			wantErr:    false,
		},
		{
			name: "apply error",
			kubeClient: &mockKubeClientForInstaller{
				applyErr: fmt.Errorf("namespace creation failed"),
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			helmClient := &mockHelmClientForInstaller{}
			installer, err := NewInstaller(config, "100.81.89.100", helmClient, tt.kubeClient)
			if err != nil {
				t.Fatalf("NewInstaller() unexpected error: %v", err)
			}

			err = installer.createNamespace(context.Background())
			if (err != nil) != tt.wantErr {
				t.Errorf("createNamespace() error = %v, wantErr %v", err, tt.wantErr)
			}
			if !tt.wantErr && tt.kubeClient.applyCalled != 1 {
				t.Errorf("Apply called %d times, want 1", tt.kubeClient.applyCalled)
			}
		})
	}
}

func TestInstaller_ValidatePrerequisites(t *testing.T) {
	helmClient := &mockHelmClientForInstaller{}
	kubeClient := &mockKubeClientForInstaller{}

	tests := []struct {
		name    string
		config  *Config
		vip     string
		wantErr bool
		errMsg  string
	}{
		{
			name: "valid prerequisites",
			config: &Config{
				OAuthClientID:     stringPtr("client-123"),
				OAuthClientSecret: stringPtr("secret-456"),
			},
			vip:     "100.81.89.100",
			wantErr: false,
		},
		{
			name: "empty VIP",
			config: &Config{
				OAuthClientID:     stringPtr("client-123"),
				OAuthClientSecret: stringPtr("secret-456"),
			},
			vip:     "",
			wantErr: true,
			errMsg:  "VIP cannot be empty",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			installer, err := NewInstaller(tt.config, tt.vip, helmClient, kubeClient)
			if err != nil && !tt.wantErr {
				t.Fatalf("NewInstaller() unexpected error: %v", err)
			}
			if err != nil && tt.wantErr {
				// Expected error during construction
				return
			}

			err = installer.validatePrerequisites(context.Background())
			if (err != nil) != tt.wantErr {
				t.Errorf("validatePrerequisites() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if err != nil && tt.errMsg != "" && err.Error() != tt.errMsg {
				t.Errorf("validatePrerequisites() error message = %q, want %q", err.Error(), tt.errMsg)
			}
		})
	}
}

// Helper function
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(substr) == 0 ||
		(len(s) > 0 && len(substr) > 0 && string(s[0:len(substr)]) == substr) ||
		(len(s) > len(substr) && containsHelper(s, substr)))
}

func containsHelper(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
