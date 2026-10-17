package kyverno

import (
	"context"
	"errors"
	"testing"

	"github.com/catalystcommunity/foundry/v1/internal/component"
	"github.com/catalystcommunity/foundry/v1/internal/helm"
	foundrykyverno "github.com/catalystcommunity/foundry/v1/internal/kyverno"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDefaultConfigPinsCurrentKyvernoRelease(t *testing.T) {
	config := DefaultConfig()
	assert.Equal(t, "3.9.1", config.Version)
	assert.Equal(t, "kyverno", config.Namespace)
	assert.Equal(t, "v1.19.1", DefaultAppVersion)
}

func TestParseConfigPreservesValuesAndPolicies(t *testing.T) {
	policies := []foundrykyverno.Selection{{Name: "disallow-host-path", State: foundrykyverno.StateAudit}}
	config, err := ParseConfig(component.ComponentConfig{
		"version": "3.9.0", "namespace": "policy-system",
		"values":   map[string]interface{}{"admissionController": map[string]interface{}{"replicas": 2}},
		"policies": policies,
	})
	require.NoError(t, err)
	assert.Equal(t, "3.9.0", config.Version)
	assert.Equal(t, "policy-system", config.Namespace)
	assert.Equal(t, policies, config.Policies)
	assert.Contains(t, config.Values, "admissionController")
}

func TestInstallHelmReleaseInstallsMissingRelease(t *testing.T) {
	client := &fakeHelmClient{}
	err := installHelmRelease(context.Background(), client, DefaultConfig())
	require.NoError(t, err)
	require.NotNil(t, client.install)
	assert.Nil(t, client.upgrade)
	assert.Equal(t, "3.9.1", client.install.Version)
	assert.Equal(t, "kyverno/kyverno", client.install.Chart)
	assert.True(t, client.install.CreateNamespace)
}

func TestInstallHelmReleaseUpgradesExistingRelease(t *testing.T) {
	client := &fakeHelmClient{releases: []helm.Release{{Name: "kyverno", Status: "deployed"}}}
	err := installHelmRelease(context.Background(), client, DefaultConfig())
	require.NoError(t, err)
	require.NotNil(t, client.upgrade)
	assert.Nil(t, client.install)
}

func TestInstallHelmReleaseReturnsRepositoryErrors(t *testing.T) {
	client := &fakeHelmClient{addRepoErr: errors.New("offline")}
	err := installHelmRelease(context.Background(), client, DefaultConfig())
	assert.ErrorContains(t, err, "add Kyverno Helm repository")
}

func TestComponentInstallRequiresClients(t *testing.T) {
	component := NewComponent(nil, nil)
	err := component.Install(context.Background(), nil)
	assert.ErrorContains(t, err, "Helm client is required")
}

type fakeHelmClient struct {
	releases   []helm.Release
	addRepoErr error
	install    *helm.InstallOptions
	upgrade    *helm.UpgradeOptions
}

func (f *fakeHelmClient) AddRepo(context.Context, helm.RepoAddOptions) error { return f.addRepoErr }
func (f *fakeHelmClient) List(context.Context, string) ([]helm.Release, error) {
	return f.releases, nil
}
func (f *fakeHelmClient) Install(_ context.Context, options helm.InstallOptions) error {
	f.install = &options
	return nil
}
func (f *fakeHelmClient) Upgrade(_ context.Context, options helm.UpgradeOptions) error {
	f.upgrade = &options
	return nil
}
func (f *fakeHelmClient) Uninstall(context.Context, helm.UninstallOptions) error { return nil }
