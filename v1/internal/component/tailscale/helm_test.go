package tailscale

import (
	"context"
	"testing"
	"time"

	"github.com/catalystcommunity/foundry/v1/internal/helm"
)

// mockHelmClient implements a mock Helm client for testing
type mockHelmClient struct {
	addRepoErr     error
	installErr     error
	uninstallErr   error
	reposAdded     []helm.RepoAddOptions
	chartsInstalled []helm.InstallOptions
	chartsUninstalled []helm.UninstallOptions
}

func (m *mockHelmClient) AddRepo(ctx context.Context, opts helm.RepoAddOptions) error {
	m.reposAdded = append(m.reposAdded, opts)
	return m.addRepoErr
}

func (m *mockHelmClient) Install(ctx context.Context, opts helm.InstallOptions) error {
	m.chartsInstalled = append(m.chartsInstalled, opts)
	return m.installErr
}

func (m *mockHelmClient) Uninstall(ctx context.Context, opts helm.UninstallOptions) error {
	m.chartsUninstalled = append(m.chartsUninstalled, opts)
	return m.uninstallErr
}

func (m *mockHelmClient) Upgrade(ctx context.Context, opts helm.UpgradeOptions) error {
	return nil
}

func (m *mockHelmClient) List(ctx context.Context, namespace string) ([]helm.Release, error) {
	return nil, nil
}

func (m *mockHelmClient) Get(ctx context.Context, releaseName, namespace string) (*helm.Release, error) {
	return nil, nil
}

func (m *mockHelmClient) Close() error {
	return nil
}

func TestNewHelmInstaller(t *testing.T) {
	tests := []struct {
		name    string
		client  *mockHelmClient
		config  *Config
		wantErr bool
		errMsg  string
	}{
		{
			name:    "nil client",
			client:  nil,
			config:  &Config{},
			wantErr: true,
			errMsg:  "helm client cannot be nil",
		},
		{
			name:    "nil config",
			client:  &mockHelmClient{},
			config:  nil,
			wantErr: true,
			errMsg:  "config cannot be nil",
		},
		{
			name:   "valid inputs",
			client: &mockHelmClient{},
			config: &Config{
				OAuthClientID:     stringPtr("client-123"),
				OAuthClientSecret: stringPtr("secret-456"),
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var client HelmClient
			if tt.client != nil {
				client = tt.client
			}
			// If tt.client is nil, client remains nil (typed nil interface)

			installer, err := NewHelmInstaller(client, tt.config)
			if (err != nil) != tt.wantErr {
				t.Errorf("NewHelmInstaller() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if err != nil && tt.errMsg != "" && err.Error() != tt.errMsg {
				t.Errorf("NewHelmInstaller() error message = %q, want %q", err.Error(), tt.errMsg)
				return
			}
			if !tt.wantErr && installer == nil {
				t.Error("NewHelmInstaller() returned nil without error")
			}
		})
	}
}

func TestHelmInstaller_AddRepository(t *testing.T) {
	tests := []struct {
		name       string
		addRepoErr error
		wantErr    bool
	}{
		{
			name:       "successful repo add",
			addRepoErr: nil,
			wantErr:    false,
		},
		{
			name:       "repo add failure",
			addRepoErr: context.DeadlineExceeded,
			wantErr:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockClient := &mockHelmClient{
				addRepoErr: tt.addRepoErr,
			}
			config := &Config{
				OAuthClientID:     stringPtr("client-123"),
				OAuthClientSecret: stringPtr("secret-456"),
			}

			installer, err := NewHelmInstaller(mockClient, config)
			if err != nil {
				t.Fatalf("NewHelmInstaller() unexpected error: %v", err)
			}

			err = installer.AddRepository(context.Background())
			if (err != nil) != tt.wantErr {
				t.Errorf("AddRepository() error = %v, wantErr %v", err, tt.wantErr)
			}

			// Verify repo was added with correct parameters
			if !tt.wantErr && len(mockClient.reposAdded) == 1 {
				repo := mockClient.reposAdded[0]
				if repo.Name != TailscaleRepoName {
					t.Errorf("Repository name = %q, want %q", repo.Name, TailscaleRepoName)
				}
				if repo.URL != TailscaleRepoURL {
					t.Errorf("Repository URL = %q, want %q", repo.URL, TailscaleRepoURL)
				}
				if !repo.ForceUpdate {
					t.Error("Expected ForceUpdate to be true")
				}
			}
		})
	}
}

func TestHelmInstaller_InstallOperator(t *testing.T) {
	tests := []struct {
		name       string
		config     *Config
		installErr error
		wantErr    bool
		checkOpts  func(t *testing.T, opts helm.InstallOptions)
	}{
		{
			name: "successful install with minimal config",
			config: &Config{
				OAuthClientID:     stringPtr("client-123"),
				OAuthClientSecret: stringPtr("secret-456"),
			},
			installErr: nil,
			wantErr:    false,
			checkOpts: func(t *testing.T, opts helm.InstallOptions) {
				if opts.ReleaseName != OperatorReleaseName {
					t.Errorf("ReleaseName = %q, want %q", opts.ReleaseName, OperatorReleaseName)
				}
				if opts.Namespace != DefaultNamespace {
					t.Errorf("Namespace = %q, want %q", opts.Namespace, DefaultNamespace)
				}
				expectedChart := TailscaleRepoName + "/" + OperatorChartName
				if opts.Chart != expectedChart {
					t.Errorf("Chart = %q, want %q", opts.Chart, expectedChart)
				}
				if !opts.CreateNamespace {
					t.Error("Expected CreateNamespace to be true")
				}
				if !opts.Wait {
					t.Error("Expected Wait to be true")
				}
				if opts.Timeout != DefaultInstallTimeout {
					t.Errorf("Timeout = %v, want %v", opts.Timeout, DefaultInstallTimeout)
				}
			},
		},
		{
			name: "install with custom operator image",
			config: &Config{
				OAuthClientID:     stringPtr("client-123"),
				OAuthClientSecret: stringPtr("secret-456"),
				OperatorImage:     stringPtr("custom/operator:v1.0.0"),
			},
			installErr: nil,
			wantErr:    false,
			checkOpts: func(t *testing.T, opts helm.InstallOptions) {
				// Verify values contain custom image
				if opts.Values == nil {
					t.Fatal("Expected Values to be set")
				}
				if imageMap, ok := opts.Values["image"].(map[string]interface{}); ok {
					if imageMap["repository"] != "custom/operator:v1.0.0" {
						t.Errorf("image.repository = %v, want custom/operator:v1.0.0", imageMap["repository"])
					}
				} else {
					t.Error("Expected image in values")
				}
			},
		},
		{
			name: "install failure",
			config: &Config{
				OAuthClientID:     stringPtr("client-123"),
				OAuthClientSecret: stringPtr("secret-456"),
			},
			installErr: context.DeadlineExceeded,
			wantErr:    true,
			checkOpts:  nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockClient := &mockHelmClient{
				installErr: tt.installErr,
			}

			installer, err := NewHelmInstaller(mockClient, tt.config)
			if err != nil {
				t.Fatalf("NewHelmInstaller() unexpected error: %v", err)
			}

			err = installer.InstallOperator(context.Background())
			if (err != nil) != tt.wantErr {
				t.Errorf("InstallOperator() error = %v, wantErr %v", err, tt.wantErr)
			}

			// Verify install was called with correct parameters
			if !tt.wantErr && len(mockClient.chartsInstalled) == 1 && tt.checkOpts != nil {
				tt.checkOpts(t, mockClient.chartsInstalled[0])
			}
		})
	}
}

func TestHelmInstaller_UninstallOperator(t *testing.T) {
	tests := []struct {
		name         string
		uninstallErr error
		wantErr      bool
	}{
		{
			name:         "successful uninstall",
			uninstallErr: nil,
			wantErr:      false,
		},
		{
			name:         "uninstall failure",
			uninstallErr: context.DeadlineExceeded,
			wantErr:      true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockClient := &mockHelmClient{
				uninstallErr: tt.uninstallErr,
			}
			config := &Config{
				OAuthClientID:     stringPtr("client-123"),
				OAuthClientSecret: stringPtr("secret-456"),
			}

			installer, err := NewHelmInstaller(mockClient, config)
			if err != nil {
				t.Fatalf("NewHelmInstaller() unexpected error: %v", err)
			}

			err = installer.UninstallOperator(context.Background())
			if (err != nil) != tt.wantErr {
				t.Errorf("UninstallOperator() error = %v, wantErr %v", err, tt.wantErr)
			}

			// Verify uninstall was called with correct parameters
			if !tt.wantErr && len(mockClient.chartsUninstalled) == 1 {
				opts := mockClient.chartsUninstalled[0]
				if opts.ReleaseName != OperatorReleaseName {
					t.Errorf("ReleaseName = %q, want %q", opts.ReleaseName, OperatorReleaseName)
				}
				if opts.Namespace != DefaultNamespace {
					t.Errorf("Namespace = %q, want %q", opts.Namespace, DefaultNamespace)
				}
				if !opts.Wait {
					t.Error("Expected Wait to be true")
				}
				if opts.Timeout != DefaultInstallTimeout {
					t.Errorf("Timeout = %v, want %v", opts.Timeout, DefaultInstallTimeout)
				}
			}
		})
	}
}

func TestHelmInstaller_GenerateHelmValues(t *testing.T) {
	tests := []struct {
		name    string
		config  *Config
		wantErr bool
		check   func(t *testing.T, values map[string]interface{})
	}{
		{
			name:    "nil config",
			config:  nil,
			wantErr: true,
		},
		{
			name: "missing OAuth credentials",
			config: &Config{
				OAuthClientID: stringPtr("client-123"),
				// Missing OAuthClientSecret
			},
			wantErr: true,
		},
		{
			name: "minimal valid config",
			config: &Config{
				OAuthClientID:     stringPtr("client-123"),
				OAuthClientSecret: stringPtr("secret-456"),
			},
			wantErr: false,
			check: func(t *testing.T, values map[string]interface{}) {
				oauth, ok := values["oauth"].(map[string]interface{})
				if !ok {
					t.Fatal("Expected oauth in values")
				}
				if oauth["clientId"] != "client-123" {
					t.Errorf("oauth.clientId = %v, want client-123", oauth["clientId"])
				}
				if oauth["clientSecret"] != "secret-456" {
					t.Errorf("oauth.clientSecret = %v, want secret-456", oauth["clientSecret"])
				}
				// No image should be set
				if _, exists := values["image"]; exists {
					t.Error("Expected no image in values for minimal config")
				}
			},
		},
		{
			name: "config with custom operator image",
			config: &Config{
				OAuthClientID:     stringPtr("client-123"),
				OAuthClientSecret: stringPtr("secret-456"),
				OperatorImage:     stringPtr("custom/operator:v1.0.0"),
			},
			wantErr: false,
			check: func(t *testing.T, values map[string]interface{}) {
				image, ok := values["image"].(map[string]interface{})
				if !ok {
					t.Fatal("Expected image in values")
				}
				if image["repository"] != "custom/operator:v1.0.0" {
					t.Errorf("image.repository = %v, want custom/operator:v1.0.0", image["repository"])
				}
			},
		},
		{
			name: "config with empty operator image string",
			config: &Config{
				OAuthClientID:     stringPtr("client-123"),
				OAuthClientSecret: stringPtr("secret-456"),
				OperatorImage:     stringPtr(""),
			},
			wantErr: false,
			check: func(t *testing.T, values map[string]interface{}) {
				// Empty string should not add image to values
				if _, exists := values["image"]; exists {
					t.Error("Expected no image in values for empty string")
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var installer *HelmInstaller
			if tt.config != nil {
				mockClient := &mockHelmClient{}
				var err error
				installer, err = NewHelmInstaller(mockClient, tt.config)
				if err != nil && !tt.wantErr {
					t.Fatalf("NewHelmInstaller() unexpected error: %v", err)
				}
				if err != nil && tt.wantErr {
					return // Expected error during construction
				}
			} else {
				installer = &HelmInstaller{config: tt.config}
			}

			values, err := installer.generateHelmValues()
			if (err != nil) != tt.wantErr {
				t.Errorf("generateHelmValues() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if !tt.wantErr && tt.check != nil {
				tt.check(t, values)
			}
		})
	}
}

func TestGenerateSecretData(t *testing.T) {
	tests := []struct {
		name    string
		config  *Config
		wantErr bool
		errMsg  string
		check   func(t *testing.T, data map[string]string)
	}{
		{
			name:    "nil config",
			config:  nil,
			wantErr: true,
			errMsg:  "config cannot be nil",
		},
		{
			name: "missing client ID",
			config: &Config{
				OAuthClientSecret: stringPtr("secret-456"),
			},
			wantErr: true,
			errMsg:  "OAuth client ID is required",
		},
		{
			name: "empty client ID",
			config: &Config{
				OAuthClientID:     stringPtr(""),
				OAuthClientSecret: stringPtr("secret-456"),
			},
			wantErr: true,
			errMsg:  "OAuth client ID is required",
		},
		{
			name: "missing client secret",
			config: &Config{
				OAuthClientID: stringPtr("client-123"),
			},
			wantErr: true,
			errMsg:  "OAuth client secret is required",
		},
		{
			name: "empty client secret",
			config: &Config{
				OAuthClientID:     stringPtr("client-123"),
				OAuthClientSecret: stringPtr(""),
			},
			wantErr: true,
			errMsg:  "OAuth client secret is required",
		},
		{
			name: "valid credentials",
			config: &Config{
				OAuthClientID:     stringPtr("client-123"),
				OAuthClientSecret: stringPtr("secret-456"),
			},
			wantErr: false,
			check: func(t *testing.T, data map[string]string) {
				if data["client_id"] != "client-123" {
					t.Errorf("client_id = %q, want %q", data["client_id"], "client-123")
				}
				if data["client_secret"] != "secret-456" {
					t.Errorf("client_secret = %q, want %q", data["client_secret"], "secret-456")
				}
				if len(data) != 2 {
					t.Errorf("Expected 2 keys in secret data, got %d", len(data))
				}
			},
		},
		{
			name: "secret reference format",
			config: &Config{
				OAuthClientID:     stringPtr("${secret:foundry-core/tailscale:client_id}"),
				OAuthClientSecret: stringPtr("${secret:foundry-core/tailscale:client_secret}"),
			},
			wantErr: false,
			check: func(t *testing.T, data map[string]string) {
				// Secret references should be stored as-is (resolution happens later)
				if data["client_id"] != "${secret:foundry-core/tailscale:client_id}" {
					t.Errorf("client_id not preserved as secret reference")
				}
				if data["client_secret"] != "${secret:foundry-core/tailscale:client_secret}" {
					t.Errorf("client_secret not preserved as secret reference")
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data, err := GenerateSecretData(tt.config)
			if (err != nil) != tt.wantErr {
				t.Errorf("GenerateSecretData() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if err != nil && tt.errMsg != "" && err.Error() != tt.errMsg {
				t.Errorf("GenerateSecretData() error message = %q, want %q", err.Error(), tt.errMsg)
				return
			}

			if !tt.wantErr && tt.check != nil {
				tt.check(t, data)
			}
		})
	}
}

func TestHelmConstants(t *testing.T) {
	// Verify Helm constants are correct
	if TailscaleRepoName != "tailscale" {
		t.Errorf("TailscaleRepoName = %q, want %q", TailscaleRepoName, "tailscale")
	}
	if TailscaleRepoURL != "https://pkgs.tailscale.com/helmcharts" {
		t.Errorf("TailscaleRepoURL = %q, want %q", TailscaleRepoURL, "https://pkgs.tailscale.com/helmcharts")
	}
	if OperatorChartName != "tailscale-operator" {
		t.Errorf("OperatorChartName = %q, want %q", OperatorChartName, "tailscale-operator")
	}
	if OperatorReleaseName != "tailscale-operator" {
		t.Errorf("OperatorReleaseName = %q, want %q", OperatorReleaseName, "tailscale-operator")
	}
	if DefaultInstallTimeout != 5*time.Minute {
		t.Errorf("DefaultInstallTimeout = %v, want %v", DefaultInstallTimeout, 5*time.Minute)
	}
}
