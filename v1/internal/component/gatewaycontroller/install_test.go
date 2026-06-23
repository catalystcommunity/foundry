package gatewaycontroller

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/catalystcommunity/foundry/v1/internal/component"
	"github.com/catalystcommunity/foundry/v1/internal/helm"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"helm.sh/helm/v3/pkg/chart/loader"
)

type mockHelmClient struct {
	releases    []helm.Release
	installOpts *helm.InstallOptions
	upgradeOpts *helm.UpgradeOptions
	uninstalled bool
}

func (m *mockHelmClient) Install(_ context.Context, opts helm.InstallOptions) error {
	m.installOpts = &opts
	return nil
}

func (m *mockHelmClient) Upgrade(_ context.Context, opts helm.UpgradeOptions) error {
	m.upgradeOpts = &opts
	return nil
}

func (m *mockHelmClient) Uninstall(_ context.Context, _ helm.UninstallOptions) error {
	m.uninstalled = true
	return nil
}

func (m *mockHelmClient) List(_ context.Context, _ string) ([]helm.Release, error) {
	return m.releases, nil
}

// TestEmbeddedChartLoads proves the chart is embedded correctly (including
// templates/_helpers.tpl via the "all:" prefix) and is a structurally valid
// Helm chart that could be installed.
func TestEmbeddedChartLoads(t *testing.T) {
	chartPath, cleanup, err := extractChart()
	require.NoError(t, err)
	defer cleanup()

	// Chart.yaml and the helpers template must be on disk after extraction.
	assert.FileExists(t, filepath.Join(chartPath, "Chart.yaml"))
	assert.FileExists(t, filepath.Join(chartPath, "templates", "_helpers.tpl"))

	ch, err := loader.Load(chartPath)
	require.NoError(t, err)
	assert.Equal(t, "foundry-gateway-controller", ch.Metadata.Name)
	assert.NotEmpty(t, ch.Templates, "chart should carry its templates")
}

func TestInstall_FreshInstall(t *testing.T) {
	mock := &mockHelmClient{} // no existing releases

	err := Install(context.Background(), mock, DefaultConfig())
	require.NoError(t, err)

	require.NotNil(t, mock.installOpts)
	assert.Nil(t, mock.upgradeOpts)

	opts := mock.installOpts
	assert.Equal(t, "gateway-controller", opts.ReleaseName)
	assert.Equal(t, "foundry-system", opts.Namespace)
	assert.True(t, opts.CreateNamespace)
	assert.True(t, opts.Wait)
	assert.Equal(t, "foundry-gateway-controller", filepath.Base(opts.Chart))

	controller := opts.Values["controller"].(map[string]interface{})
	assert.Equal(t, "contour", controller["gatewayName"])
	assert.Equal(t, "projectcontour", controller["gatewayNamespace"])
	assert.Equal(t, "contour-envoy", controller["envoyService"])
	assert.Equal(t, "15s", controller["interval"])

	image := opts.Values["image"].(map[string]interface{})
	assert.Equal(t, "containers.catalystsquad.com/public/catalystcommunity/foundry", image["repository"])
	_, hasTag := image["tag"] // empty default tag -> not set, chart uses appVersion
	assert.False(t, hasTag)

	assert.Equal(t, uint64(1), opts.Values["replicaCount"])
}

func TestInstall_UpgradesExistingRelease(t *testing.T) {
	mock := &mockHelmClient{
		releases: []helm.Release{{Name: "gateway-controller", Status: "deployed"}},
	}

	err := Install(context.Background(), mock, DefaultConfig())
	require.NoError(t, err)

	assert.Nil(t, mock.installOpts)
	require.NotNil(t, mock.upgradeOpts)
	assert.True(t, mock.upgradeOpts.Install)
	assert.Equal(t, "foundry-gateway-controller", filepath.Base(mock.upgradeOpts.Chart))
}

func TestInstall_NilHelmClient(t *testing.T) {
	err := Install(context.Background(), nil, DefaultConfig())
	require.Error(t, err)
}

func TestInstall_ImageTagOverride(t *testing.T) {
	mock := &mockHelmClient{}
	cfg := DefaultConfig()
	cfg.ImageTag = "1.2.3"
	cfg.ReplicaCount = 3

	require.NoError(t, Install(context.Background(), mock, cfg))

	image := mock.installOpts.Values["image"].(map[string]interface{})
	assert.Equal(t, "1.2.3", image["tag"])
	assert.Equal(t, uint64(3), mock.installOpts.Values["replicaCount"])
}

func TestExtractChart_CleansUp(t *testing.T) {
	chartPath, cleanup, err := extractChart()
	require.NoError(t, err)
	parent := filepath.Dir(chartPath)
	require.DirExists(t, parent)

	cleanup()

	_, statErr := os.Stat(parent)
	assert.True(t, os.IsNotExist(statErr), "temp dir should be removed by cleanup")
}

func TestBuildHelmValues_ValuesOverlayDeepMerges(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Values = map[string]interface{}{
		"controller": map[string]interface{}{
			"interval": "60s", // override one leaf
		},
		"replicaCount": 5, // override a typed field
		"resources": map[string]interface{}{ // brand-new subtree
			"limits": map[string]interface{}{"cpu": "200m"},
		},
		"podAnnotations": map[string]interface{}{"team": "ingress"},
	}

	values := buildHelmValues(cfg)

	// overlay wins...
	controller := values["controller"].(map[string]interface{})
	assert.Equal(t, "60s", controller["interval"])
	// ...but sibling typed defaults are preserved (deep merge, not replace)
	assert.Equal(t, "contour", controller["gatewayName"])
	assert.Equal(t, "contour-envoy", controller["envoyService"])

	// overlay overrides a typed top-level field
	assert.Equal(t, 5, values["replicaCount"])

	// arbitrary passthrough values reach the chart
	resources := values["resources"].(map[string]interface{})
	limits := resources["limits"].(map[string]interface{})
	assert.Equal(t, "200m", limits["cpu"])
	assert.Equal(t, "ingress", values["podAnnotations"].(map[string]interface{})["team"])
}

func TestParseConfig_ValuesPassthrough(t *testing.T) {
	cc := component.ComponentConfig{
		"values": map[string]interface{}{
			"resources": map[string]interface{}{"requests": map[string]interface{}{"cpu": "50m"}},
		},
	}
	cfg, err := ParseConfig(cc)
	require.NoError(t, err)

	values := buildHelmValues(cfg)
	resources := values["resources"].(map[string]interface{})
	requests := resources["requests"].(map[string]interface{})
	assert.Equal(t, "50m", requests["cpu"])
}
