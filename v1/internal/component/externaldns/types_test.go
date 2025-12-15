package externaldns

import (
	"context"
	"testing"

	"github.com/catalystcommunity/foundry/v1/internal/component"
	"github.com/catalystcommunity/foundry/v1/internal/helm"
	"github.com/catalystcommunity/foundry/v1/internal/k8s"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDefaultConfig(t *testing.T) {
	config := DefaultConfig()

	assert.Equal(t, "1.15.0", config.Version)
	assert.Equal(t, "external-dns", config.Namespace)
	assert.Equal(t, ProviderNone, config.Provider)
	assert.Empty(t, config.DomainFilters)
	assert.Equal(t, []string{"ingress", "service"}, config.Sources)
	assert.Equal(t, "upsert-only", config.Policy)
	assert.Equal(t, "foundry", config.TxtOwnerId)
	assert.NotNil(t, config.Values)
}

func TestParseConfig_Defaults(t *testing.T) {
	cfg := component.ComponentConfig{}

	config, err := ParseConfig(cfg)
	require.NoError(t, err)

	assert.Equal(t, "1.15.0", config.Version)
	assert.Equal(t, "external-dns", config.Namespace)
	assert.Equal(t, ProviderNone, config.Provider)
}

func TestParseConfig_CustomValues(t *testing.T) {
	cfg := component.ComponentConfig{
		"version":        "1.14.0",
		"namespace":      "custom-dns",
		"domain_filters": []interface{}{"example.com", "test.com"},
		"sources":        []interface{}{"ingress"},
		"policy":         "sync",
		"txt_owner_id":   "my-cluster",
	}

	config, err := ParseConfig(cfg)
	require.NoError(t, err)

	assert.Equal(t, "1.14.0", config.Version)
	assert.Equal(t, "custom-dns", config.Namespace)
	assert.Equal(t, ProviderNone, config.Provider) // No provider specified
	assert.Equal(t, []string{"example.com", "test.com"}, config.DomainFilters)
	assert.Equal(t, []string{"ingress"}, config.Sources)
	assert.Equal(t, "sync", config.Policy)
	assert.Equal(t, "my-cluster", config.TxtOwnerId)
}

func TestParseConfig_PowerDNS(t *testing.T) {
	cfg := component.ComponentConfig{
		"powerdns": map[string]interface{}{
			"api_url":   "http://powerdns:8081",
			"api_key":   "secret-key",
			"server_id": "localhost",
		},
	}

	config, err := ParseConfig(cfg)
	require.NoError(t, err)

	assert.Equal(t, ProviderPowerDNS, config.Provider)
	require.NotNil(t, config.PowerDNS)
	assert.Equal(t, "http://powerdns:8081", config.PowerDNS.APIUrl)
	assert.Equal(t, "secret-key", config.PowerDNS.APIKey)
	assert.Equal(t, "localhost", config.PowerDNS.ServerID)
}

func TestParseConfig_Cloudflare(t *testing.T) {
	cfg := component.ComponentConfig{
		"cloudflare": map[string]interface{}{
			"api_token": "cf-token-123",
			"proxied":   true,
		},
	}

	config, err := ParseConfig(cfg)
	require.NoError(t, err)

	assert.Equal(t, ProviderCloudflare, config.Provider)
	require.NotNil(t, config.Cloudflare)
	assert.Equal(t, "cf-token-123", config.Cloudflare.APIToken)
	assert.True(t, config.Cloudflare.Proxied)
}

func TestParseConfig_RFC2136(t *testing.T) {
	cfg := component.ComponentConfig{
		"rfc2136": map[string]interface{}{
			"host":            "dns-server.local",
			"port":            float64(5353),
			"zone":            "example.com.",
			"tsig_key_name":   "update-key",
			"tsig_secret":     "base64secret",
			"tsig_secret_alg": "hmac-sha512",
		},
	}

	config, err := ParseConfig(cfg)
	require.NoError(t, err)

	assert.Equal(t, ProviderRFC2136, config.Provider)
	require.NotNil(t, config.RFC2136)
	assert.Equal(t, "dns-server.local", config.RFC2136.Host)
	assert.Equal(t, 5353, config.RFC2136.Port)
	assert.Equal(t, "example.com.", config.RFC2136.Zone)
	assert.Equal(t, "update-key", config.RFC2136.TSIGKeyName)
	assert.Equal(t, "base64secret", config.RFC2136.TSIGSecret)
	assert.Equal(t, "hmac-sha512", config.RFC2136.TSIGSecretAlg)
}

func TestParseConfig_RFC2136_Defaults(t *testing.T) {
	cfg := component.ComponentConfig{
		"rfc2136": map[string]interface{}{
			"host": "dns-server.local",
		},
	}

	config, err := ParseConfig(cfg)
	require.NoError(t, err)

	require.NotNil(t, config.RFC2136)
	assert.Equal(t, 53, config.RFC2136.Port)
	assert.Equal(t, "hmac-sha256", config.RFC2136.TSIGSecretAlg)
}

func TestParseConfig_WithCustomValues(t *testing.T) {
	cfg := component.ComponentConfig{
		"values": map[string]interface{}{
			"custom": "value",
		},
	}

	config, err := ParseConfig(cfg)
	require.NoError(t, err)

	require.NotNil(t, config.Values)
	assert.Equal(t, "value", config.Values["custom"])
}

func TestValidate_NoProvider(t *testing.T) {
	config := &Config{
		Provider: ProviderNone,
		Policy:   "upsert-only",
	}

	err := config.Validate()
	assert.NoError(t, err)
}

func TestValidate_PowerDNS_Success(t *testing.T) {
	config := &Config{
		Provider: ProviderPowerDNS,
		Policy:   "sync",
		PowerDNS: &PowerDNSConfig{
			APIUrl: "http://powerdns:8081",
			APIKey: "secret",
		},
	}

	err := config.Validate()
	assert.NoError(t, err)
}

func TestValidate_PowerDNS_MissingAPIUrl(t *testing.T) {
	config := &Config{
		Provider: ProviderPowerDNS,
		Policy:   "sync",
		PowerDNS: &PowerDNSConfig{
			APIKey: "secret",
		},
	}

	err := config.Validate()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "api_url is required")
}

func TestValidate_PowerDNS_MissingAPIKey(t *testing.T) {
	config := &Config{
		Provider: ProviderPowerDNS,
		Policy:   "sync",
		PowerDNS: &PowerDNSConfig{
			APIUrl: "http://powerdns:8081",
		},
	}

	err := config.Validate()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "api_key is required")
}

func TestValidate_PowerDNS_NilConfig(t *testing.T) {
	config := &Config{
		Provider: ProviderPowerDNS,
		Policy:   "sync",
		PowerDNS: nil,
	}

	err := config.Validate()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "api_url is required")
}

func TestValidate_Cloudflare_Success(t *testing.T) {
	config := &Config{
		Provider: ProviderCloudflare,
		Policy:   "sync",
		Cloudflare: &CloudflareConfig{
			APIToken: "token123",
		},
	}

	err := config.Validate()
	assert.NoError(t, err)
}

func TestValidate_Cloudflare_MissingToken(t *testing.T) {
	config := &Config{
		Provider:   ProviderCloudflare,
		Policy:     "sync",
		Cloudflare: &CloudflareConfig{},
	}

	err := config.Validate()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "api_token is required")
}

func TestValidate_RFC2136_Success(t *testing.T) {
	config := &Config{
		Provider: ProviderRFC2136,
		Policy:   "sync",
		RFC2136: &RFC2136Config{
			Host: "dns-server.local",
		},
	}

	err := config.Validate()
	assert.NoError(t, err)
}

func TestValidate_RFC2136_MissingHost(t *testing.T) {
	config := &Config{
		Provider: ProviderRFC2136,
		Policy:   "sync",
		RFC2136:  &RFC2136Config{},
	}

	err := config.Validate()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "host is required")
}

func TestValidate_InvalidPolicy(t *testing.T) {
	config := &Config{
		Provider: ProviderNone,
		Policy:   "invalid-policy",
	}

	err := config.Validate()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid policy")
}

func TestValidate_UnsupportedProvider(t *testing.T) {
	config := &Config{
		Provider: Provider("unknown-provider"),
		Policy:   "sync",
	}

	err := config.Validate()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "unsupported provider")
}

func TestValidate_CloudProviders(t *testing.T) {
	// These providers pass through for custom configuration
	providers := []Provider{ProviderRoute53, ProviderGoogle, ProviderAzure}

	for _, provider := range providers {
		config := &Config{
			Provider: provider,
			Policy:   "sync",
		}

		err := config.Validate()
		assert.NoError(t, err, "provider %s should be valid", provider)
	}
}

func TestIsProviderConfigured(t *testing.T) {
	tests := []struct {
		provider Provider
		expected bool
	}{
		{ProviderNone, false},
		{ProviderPowerDNS, true},
		{ProviderCloudflare, true},
		{ProviderRoute53, true},
		{ProviderRFC2136, true},
	}

	for _, tt := range tests {
		config := &Config{Provider: tt.provider}
		assert.Equal(t, tt.expected, config.IsProviderConfigured(), "provider: %s", tt.provider)
	}
}

func TestComponent_Name(t *testing.T) {
	comp := NewComponent(nil, nil)
	assert.Equal(t, "external-dns", comp.Name())
}

func TestComponent_Dependencies(t *testing.T) {
	comp := NewComponent(nil, nil)
	deps := comp.Dependencies()

	assert.Empty(t, deps) // No strict dependencies
}

func TestComponent_Install_NilHelmClient(t *testing.T) {
	comp := NewComponent(nil, nil)
	err := comp.Install(context.Background(), component.ComponentConfig{})

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "helm client cannot be nil")
}

func TestComponent_Upgrade_NotImplemented(t *testing.T) {
	comp := NewComponent(nil, nil)
	err := comp.Upgrade(context.Background(), component.ComponentConfig{})

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not yet implemented")
}

func TestComponent_Uninstall_NotImplemented(t *testing.T) {
	comp := NewComponent(nil, nil)
	err := comp.Uninstall(context.Background())

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not yet implemented")
}

func TestComponent_Status_NoHelmClient(t *testing.T) {
	comp := NewComponent(nil, nil)
	status, err := comp.Status(context.Background())

	assert.NoError(t, err)
	assert.False(t, status.Installed)
	assert.False(t, status.Healthy)
	assert.Contains(t, status.Message, "not initialized")
}

// mockHelmClient is a mock implementation of HelmClient for testing
type mockHelmClient struct {
	addRepoErr      error
	installErr      error
	upgradeErr      error
	listReleases    []helm.Release
	listErr         error
	reposAdded      []helm.RepoAddOptions
	chartsInstalled []helm.InstallOptions
	upgradeCalls    []helm.UpgradeOptions
	uninstallCalls  []helm.UninstallOptions
}

func (m *mockHelmClient) AddRepo(ctx context.Context, opts helm.RepoAddOptions) error {
	m.reposAdded = append(m.reposAdded, opts)
	return m.addRepoErr
}

func (m *mockHelmClient) Install(ctx context.Context, opts helm.InstallOptions) error {
	m.chartsInstalled = append(m.chartsInstalled, opts)
	return m.installErr
}

func (m *mockHelmClient) Upgrade(ctx context.Context, opts helm.UpgradeOptions) error {
	m.upgradeCalls = append(m.upgradeCalls, opts)
	return m.upgradeErr
}

func (m *mockHelmClient) List(ctx context.Context, namespace string) ([]helm.Release, error) {
	return m.listReleases, m.listErr
}

func (m *mockHelmClient) Uninstall(ctx context.Context, opts helm.UninstallOptions) error {
	m.uninstallCalls = append(m.uninstallCalls, opts)
	return nil
}

// mockK8sClient is a mock implementation of K8sClient for testing
type mockK8sClient struct {
	pods    []*k8s.Pod
	podsErr error
}

func (m *mockK8sClient) GetPods(ctx context.Context, namespace string) ([]*k8s.Pod, error) {
	return m.pods, m.podsErr
}

func TestComponent_Status_ExternalDNSInstalled(t *testing.T) {
	helmClient := &mockHelmClient{
		listReleases: []helm.Release{
			{
				Name:       "external-dns",
				Namespace:  "external-dns",
				Status:     "deployed",
				AppVersion: "0.14.0",
			},
		},
	}

	comp := NewComponent(helmClient, nil)
	status, err := comp.Status(context.Background())

	assert.NoError(t, err)
	assert.True(t, status.Installed)
	assert.True(t, status.Healthy)
	assert.Equal(t, "0.14.0", status.Version)
}

func TestComponent_Status_ExternalDNSNotInstalled(t *testing.T) {
	helmClient := &mockHelmClient{
		listReleases: []helm.Release{},
	}

	comp := NewComponent(helmClient, nil)
	status, err := comp.Status(context.Background())

	assert.NoError(t, err)
	assert.False(t, status.Installed)
	assert.False(t, status.Healthy)
	assert.Contains(t, status.Message, "not found")
}

func TestComponent_Status_ExternalDNSFailed(t *testing.T) {
	helmClient := &mockHelmClient{
		listReleases: []helm.Release{
			{
				Name:       "external-dns",
				Namespace:  "external-dns",
				Status:     "failed",
				AppVersion: "0.14.0",
			},
		},
	}

	comp := NewComponent(helmClient, nil)
	status, err := comp.Status(context.Background())

	assert.NoError(t, err)
	assert.True(t, status.Installed)
	assert.False(t, status.Healthy)
}
