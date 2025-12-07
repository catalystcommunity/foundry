package externaldns

import (
	"context"
	"testing"
	"time"

	"github.com/catalystcommunity/foundry/v1/internal/helm"
	"github.com/catalystcommunity/foundry/v1/internal/k8s"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestInstall_Success(t *testing.T) {
	helmClient := &mockHelmClient{}
	k8sClient := &mockK8sClient{
		pods: []*k8s.Pod{
			{Name: "external-dns-abc123", Namespace: "external-dns", Status: "Running"},
		},
	}

	cfg := DefaultConfig()
	err := Install(context.Background(), helmClient, k8sClient, cfg)
	require.NoError(t, err)

	// Verify repo was added
	require.Len(t, helmClient.reposAdded, 1)
	assert.Equal(t, externalDNSRepoName, helmClient.reposAdded[0].Name)
	assert.Equal(t, externalDNSRepoURL, helmClient.reposAdded[0].URL)

	// Verify chart was installed
	require.Len(t, helmClient.chartsInstalled, 1)
	assert.Equal(t, releaseName, helmClient.chartsInstalled[0].ReleaseName)
	assert.Equal(t, "external-dns", helmClient.chartsInstalled[0].Namespace)
	assert.Equal(t, externalDNSChart, helmClient.chartsInstalled[0].Chart)
	assert.True(t, helmClient.chartsInstalled[0].CreateNamespace)
	assert.True(t, helmClient.chartsInstalled[0].Wait)
	assert.Equal(t, 5*time.Minute, helmClient.chartsInstalled[0].Timeout)
}

func TestInstall_AlreadyInstalled(t *testing.T) {
	helmClient := &mockHelmClient{
		listReleases: []helm.Release{
			{
				Name:      releaseName,
				Namespace: "external-dns",
				Status:    "deployed",
			},
		},
	}
	k8sClient := &mockK8sClient{
		pods: []*k8s.Pod{
			{Name: "external-dns-abc123", Namespace: "external-dns", Status: "Running"},
		},
	}

	cfg := DefaultConfig()
	err := Install(context.Background(), helmClient, k8sClient, cfg)
	require.NoError(t, err)

	// Should not install again
	assert.Empty(t, helmClient.chartsInstalled)
}

func TestInstall_NilHelmClient(t *testing.T) {
	err := Install(context.Background(), nil, &mockK8sClient{}, DefaultConfig())
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "helm client cannot be nil")
}

func TestInstall_NilConfig(t *testing.T) {
	helmClient := &mockHelmClient{}
	k8sClient := &mockK8sClient{
		pods: []*k8s.Pod{
			{Name: "external-dns-abc123", Namespace: "external-dns", Status: "Running"},
		},
	}

	// Should use default config
	err := Install(context.Background(), helmClient, k8sClient, nil)
	require.NoError(t, err)

	// Verify installation happened with defaults
	require.Len(t, helmClient.chartsInstalled, 1)
	assert.Equal(t, releaseName, helmClient.chartsInstalled[0].ReleaseName)
}

func TestInstall_AddRepoError(t *testing.T) {
	helmClient := &mockHelmClient{
		addRepoErr: assert.AnError,
	}
	k8sClient := &mockK8sClient{}

	err := Install(context.Background(), helmClient, k8sClient, DefaultConfig())
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to add helm repository")
}

func TestInstall_InstallChartError(t *testing.T) {
	helmClient := &mockHelmClient{
		installErr: assert.AnError,
	}
	k8sClient := &mockK8sClient{}

	err := Install(context.Background(), helmClient, k8sClient, DefaultConfig())
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to install external-dns")
}

func TestInstall_FailedReleaseCleanedup(t *testing.T) {
	helmClient := &mockHelmClient{
		listReleases: []helm.Release{
			{
				Name:      releaseName,
				Namespace: "external-dns",
				Status:    "failed",
			},
		},
	}
	k8sClient := &mockK8sClient{
		pods: []*k8s.Pod{
			{Name: "external-dns-abc123", Namespace: "external-dns", Status: "Running"},
		},
	}

	cfg := DefaultConfig()
	err := Install(context.Background(), helmClient, k8sClient, cfg)
	require.NoError(t, err)

	// Should have uninstalled the failed release
	require.Len(t, helmClient.uninstallCalls, 1)
	assert.Equal(t, releaseName, helmClient.uninstallCalls[0].ReleaseName)

	// And installed fresh
	require.Len(t, helmClient.chartsInstalled, 1)
}

func TestBuildHelmValues_NoProvider(t *testing.T) {
	cfg := DefaultConfig()
	values := buildHelmValues(cfg)

	// Check basic configuration
	assert.Equal(t, "upsert-only", values["policy"])
	assert.Equal(t, "foundry", values["txtOwnerId"])
	assert.Equal(t, []string{"ingress", "service"}, values["sources"])
	assert.Equal(t, "info", values["logLevel"])

	// Check ServiceMonitor is enabled
	serviceMonitor, ok := values["serviceMonitor"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, true, serviceMonitor["enabled"])

	// No provider should be set
	_, hasProvider := values["provider"]
	assert.False(t, hasProvider)
}

func TestBuildHelmValues_WithDomainFilters(t *testing.T) {
	cfg := &Config{
		DomainFilters: []string{"example.com", "test.com"},
		Sources:       []string{"ingress"},
		Policy:        "sync",
		TxtOwnerId:    "test",
		Values:        map[string]interface{}{},
	}

	values := buildHelmValues(cfg)

	domainFilters, ok := values["domainFilters"].([]string)
	require.True(t, ok)
	assert.Equal(t, []string{"example.com", "test.com"}, domainFilters)
}

func TestBuildHelmValues_PowerDNS(t *testing.T) {
	cfg := &Config{
		Provider: ProviderPowerDNS,
		PowerDNS: &PowerDNSConfig{
			APIUrl: "http://powerdns:8081",
			APIKey: "secret-key",
		},
		Sources:    []string{"ingress"},
		Policy:     "sync",
		TxtOwnerId: "test",
		Values:     map[string]interface{}{},
	}

	values := buildHelmValues(cfg)

	// Check provider
	provider, ok := values["provider"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "pdns", provider["name"])

	// Check environment variables
	env, ok := values["env"].([]map[string]interface{})
	require.True(t, ok)
	require.Len(t, env, 2)

	// Check extraArgs - now []interface{} for Bitnami chart compatibility
	extraArgs, ok := values["extraArgs"].([]interface{})
	require.True(t, ok)
	assert.Contains(t, extraArgs, "--pdns-server=http://powerdns:8081")
	assert.Contains(t, extraArgs, "--pdns-api-key=secret-key")
}

func TestBuildHelmValues_Cloudflare(t *testing.T) {
	cfg := &Config{
		Provider: ProviderCloudflare,
		Cloudflare: &CloudflareConfig{
			APIToken: "cf-token-123",
			Proxied:  true,
		},
		Sources:    []string{"ingress"},
		Policy:     "sync",
		TxtOwnerId: "test",
		Values:     map[string]interface{}{},
	}

	values := buildHelmValues(cfg)

	// Check provider
	provider, ok := values["provider"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "cloudflare", provider["name"])

	// Check environment variables
	env, ok := values["env"].([]map[string]interface{})
	require.True(t, ok)
	require.Len(t, env, 1)
	assert.Equal(t, "CF_API_TOKEN", env[0]["name"])
	assert.Equal(t, "cf-token-123", env[0]["value"])

	// Check proxied flag - now []interface{} for Bitnami chart compatibility
	extraArgs, ok := values["extraArgs"].([]interface{})
	require.True(t, ok)
	assert.Contains(t, extraArgs, "--cloudflare-proxied")
}

func TestBuildHelmValues_Cloudflare_NotProxied(t *testing.T) {
	cfg := &Config{
		Provider: ProviderCloudflare,
		Cloudflare: &CloudflareConfig{
			APIToken: "cf-token-123",
			Proxied:  false,
		},
		Sources:    []string{"ingress"},
		Policy:     "sync",
		TxtOwnerId: "test",
		Values:     map[string]interface{}{},
	}

	values := buildHelmValues(cfg)

	// Should not have proxied flag
	_, hasExtraArgs := values["extraArgs"]
	assert.False(t, hasExtraArgs, "Should not have extraArgs when not proxied")
}

func TestBuildHelmValues_RFC2136(t *testing.T) {
	cfg := &Config{
		Provider: ProviderRFC2136,
		RFC2136: &RFC2136Config{
			Host:          "dns-server.local",
			Port:          5353,
			Zone:          "example.com.",
			TSIGKeyName:   "update-key",
			TSIGSecret:    "base64secret",
			TSIGSecretAlg: "hmac-sha512",
		},
		Sources:    []string{"ingress"},
		Policy:     "sync",
		TxtOwnerId: "test",
		Values:     map[string]interface{}{},
	}

	values := buildHelmValues(cfg)

	// Check provider
	provider, ok := values["provider"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "rfc2136", provider["name"])

	// Check extraArgs
	extraArgs, ok := values["extraArgs"].([]string)
	require.True(t, ok)
	assert.Contains(t, extraArgs, "--rfc2136-host=dns-server.local")
	assert.Contains(t, extraArgs, "--rfc2136-port=5353")
	assert.Contains(t, extraArgs, "--rfc2136-zone=example.com.")
	assert.Contains(t, extraArgs, "--rfc2136-tsig-keyname=update-key")
	assert.Contains(t, extraArgs, "--rfc2136-tsig-secret=base64secret")
	assert.Contains(t, extraArgs, "--rfc2136-tsig-secret-alg=hmac-sha512")
}

func TestBuildHelmValues_RFC2136_Defaults(t *testing.T) {
	cfg := &Config{
		Provider: ProviderRFC2136,
		RFC2136: &RFC2136Config{
			Host: "dns-server.local",
			// Port and TSIGSecretAlg should use defaults
		},
		Sources:    []string{"ingress"},
		Policy:     "sync",
		TxtOwnerId: "test",
		Values:     map[string]interface{}{},
	}

	values := buildHelmValues(cfg)

	extraArgs, ok := values["extraArgs"].([]string)
	require.True(t, ok)
	assert.Contains(t, extraArgs, "--rfc2136-port=53")
}

func TestBuildHelmValues_CustomValues(t *testing.T) {
	cfg := &Config{
		Sources:    []string{"ingress"},
		Policy:     "sync",
		TxtOwnerId: "test",
		Values: map[string]interface{}{
			"custom": "value",
			"nested": map[string]interface{}{
				"key": "value",
			},
		},
	}

	values := buildHelmValues(cfg)

	// Custom values should be preserved
	assert.Equal(t, "value", values["custom"])
	assert.NotNil(t, values["nested"])
}

func TestVerifyInstallation_Success(t *testing.T) {
	k8sClient := &mockK8sClient{
		pods: []*k8s.Pod{
			{Name: "external-dns-abc123", Namespace: "external-dns", Status: "Running"},
		},
	}

	ctx := context.Background()
	err := verifyInstallation(ctx, k8sClient, "external-dns")
	assert.NoError(t, err)
}

func TestVerifyInstallation_NilClient(t *testing.T) {
	ctx := context.Background()
	err := verifyInstallation(ctx, nil, "external-dns")
	assert.NoError(t, err) // Should skip verification
}

func TestVerifyInstallation_PodsNotReady(t *testing.T) {
	k8sClient := &mockK8sClient{
		pods: []*k8s.Pod{
			{Name: "external-dns-abc123", Namespace: "external-dns", Status: "Pending"},
		},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	err := verifyInstallation(ctx, k8sClient, "external-dns")
	assert.Error(t, err)
}

func TestVerifyInstallation_ContextCanceled(t *testing.T) {
	k8sClient := &mockK8sClient{
		pods: []*k8s.Pod{},
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	err := verifyInstallation(ctx, k8sClient, "external-dns")
	assert.Error(t, err)
	assert.Equal(t, context.Canceled, err)
}

func TestContainsSubstring(t *testing.T) {
	tests := []struct {
		s        string
		substr   string
		expected bool
	}{
		{"external-dns-abc123", "external-dns", true},
		{"hello-world", "world", true},
		{"hello-world", "foo", false},
		{"short", "longer-than-short", false},
		{"", "", true},
		{"abc", "", true},
	}

	for _, tt := range tests {
		result := containsSubstring(tt.s, tt.substr)
		assert.Equal(t, tt.expected, result, "containsSubstring(%q, %q)", tt.s, tt.substr)
	}
}
