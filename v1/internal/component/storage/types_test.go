package storage

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

	assert.Equal(t, BackendLocalPath, config.Backend)
	assert.Equal(t, "0.0.28", config.Version)
	assert.Equal(t, "kube-system", config.Namespace)
	assert.Equal(t, "local-path", config.StorageClassName)
	assert.True(t, config.SetDefault)
	assert.NotNil(t, config.LocalPath)
	assert.Equal(t, "/opt/local-path-provisioner", config.LocalPath.Path)
	assert.Equal(t, "Delete", config.LocalPath.ReclaimPolicy)
}

func TestParseConfig_Defaults(t *testing.T) {
	cfg := component.ComponentConfig{}

	config, err := ParseConfig(cfg)
	require.NoError(t, err)

	assert.Equal(t, BackendLocalPath, config.Backend)
	assert.Equal(t, "0.0.28", config.Version)
	assert.Equal(t, "kube-system", config.Namespace)
	assert.Equal(t, "local-path", config.StorageClassName)
	assert.True(t, config.SetDefault)
}

func TestParseConfig_LocalPath(t *testing.T) {
	cfg := component.ComponentConfig{
		"backend":            "local-path",
		"version":            "0.0.30",
		"namespace":          "storage",
		"storage_class_name": "custom-local",
		"set_default":        false,
		"local_path": map[string]interface{}{
			"path":           "/data/volumes",
			"reclaim_policy": "Retain",
		},
	}

	config, err := ParseConfig(cfg)
	require.NoError(t, err)

	assert.Equal(t, BackendLocalPath, config.Backend)
	assert.Equal(t, "0.0.30", config.Version)
	assert.Equal(t, "storage", config.Namespace)
	assert.Equal(t, "custom-local", config.StorageClassName)
	assert.False(t, config.SetDefault)
	assert.NotNil(t, config.LocalPath)
	assert.Equal(t, "/data/volumes", config.LocalPath.Path)
	assert.Equal(t, "Retain", config.LocalPath.ReclaimPolicy)
}

func TestParseConfig_NFS(t *testing.T) {
	cfg := component.ComponentConfig{
		"backend":            "nfs",
		"storage_class_name": "nfs-storage",
		"nfs": map[string]interface{}{
			"server":            "192.168.1.100",
			"path":              "/mnt/nfs/k8s",
			"reclaim_policy":    "Retain",
			"archive_on_delete": true,
		},
	}

	config, err := ParseConfig(cfg)
	require.NoError(t, err)

	assert.Equal(t, BackendNFS, config.Backend)
	assert.Equal(t, "nfs-storage", config.StorageClassName)
	assert.NotNil(t, config.NFS)
	assert.Equal(t, "192.168.1.100", config.NFS.Server)
	assert.Equal(t, "/mnt/nfs/k8s", config.NFS.Path)
	assert.Equal(t, "Retain", config.NFS.ReclaimPolicy)
	assert.True(t, config.NFS.ArchiveOnDelete)
}

func TestParseConfig_TrueNASNFS(t *testing.T) {
	cfg := component.ComponentConfig{
		"backend":            "truenas-nfs",
		"storage_class_name": "truenas-nfs",
		"truenas": map[string]interface{}{
			"api_url":        "https://truenas.local",
			"api_key":        "secret-key",
			"pool":           "tank",
			"dataset_parent": "tank/k8s/volumes",
			"share_host":     "192.168.1.50",
			"share_networks": []interface{}{"192.168.1.0/24", "10.0.0.0/8"},
		},
	}

	config, err := ParseConfig(cfg)
	require.NoError(t, err)

	assert.Equal(t, BackendTrueNASNFS, config.Backend)
	assert.Equal(t, "truenas-nfs", config.StorageClassName)
	assert.NotNil(t, config.TrueNAS)
	assert.Equal(t, "https://truenas.local", config.TrueNAS.APIURL)
	assert.Equal(t, "secret-key", config.TrueNAS.APIKey)
	assert.Equal(t, "tank", config.TrueNAS.Pool)
	assert.Equal(t, "tank/k8s/volumes", config.TrueNAS.DatasetParent)
	assert.Equal(t, "192.168.1.50", config.TrueNAS.ShareHost)
	assert.Len(t, config.TrueNAS.ShareNetworks, 2)
	assert.Contains(t, config.TrueNAS.ShareNetworks, "192.168.1.0/24")
	assert.Contains(t, config.TrueNAS.ShareNetworks, "10.0.0.0/8")
}

func TestParseConfig_TrueNASiSCSI(t *testing.T) {
	cfg := component.ComponentConfig{
		"backend":            "truenas-iscsi",
		"storage_class_name": "truenas-iscsi",
		"truenas": map[string]interface{}{
			"api_url":            "https://truenas.local",
			"api_key":            "secret-key",
			"pool":               "tank",
			"dataset_parent":     "tank/k8s/zvols",
			"portal_id":          float64(1),
			"initiator_group_id": float64(1),
		},
	}

	config, err := ParseConfig(cfg)
	require.NoError(t, err)

	assert.Equal(t, BackendTrueNASiSCSI, config.Backend)
	assert.NotNil(t, config.TrueNAS)
	assert.Equal(t, 1, config.TrueNAS.PortalID)
	assert.Equal(t, 1, config.TrueNAS.InitiatorGroupID)
}

func TestParseConfig_WithCustomValues(t *testing.T) {
	cfg := component.ComponentConfig{
		"values": map[string]interface{}{
			"custom": "value",
			"nested": map[string]interface{}{
				"key": "value",
			},
		},
	}

	config, err := ParseConfig(cfg)
	require.NoError(t, err)

	require.NotNil(t, config.Values)
	assert.Equal(t, "value", config.Values["custom"])
	assert.NotNil(t, config.Values["nested"])
}

func TestValidate_LocalPath_Success(t *testing.T) {
	config := &Config{
		Backend:          BackendLocalPath,
		StorageClassName: "local-path",
	}

	err := config.Validate()
	assert.NoError(t, err)
	// Should have set default LocalPath config
	assert.NotNil(t, config.LocalPath)
}

func TestValidate_NFS_MissingServer(t *testing.T) {
	config := &Config{
		Backend:          BackendNFS,
		StorageClassName: "nfs",
		NFS: &NFSConfig{
			Path: "/mnt/nfs",
		},
	}

	err := config.Validate()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "nfs server is required")
}

func TestValidate_NFS_MissingPath(t *testing.T) {
	config := &Config{
		Backend:          BackendNFS,
		StorageClassName: "nfs",
		NFS: &NFSConfig{
			Server: "192.168.1.100",
		},
	}

	err := config.Validate()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "nfs path is required")
}

func TestValidate_NFS_MissingConfig(t *testing.T) {
	config := &Config{
		Backend:          BackendNFS,
		StorageClassName: "nfs",
	}

	err := config.Validate()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "nfs configuration required")
}

func TestValidate_TrueNASNFS_MissingAPIURL(t *testing.T) {
	config := &Config{
		Backend:          BackendTrueNASNFS,
		StorageClassName: "truenas",
		TrueNAS: &TrueNASCSIConfig{
			APIKey:        "key",
			Pool:          "tank",
			DatasetParent: "tank/k8s",
			ShareHost:     "192.168.1.50",
		},
	}

	err := config.Validate()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "truenas api_url is required")
}

func TestValidate_TrueNASNFS_MissingShareHost(t *testing.T) {
	config := &Config{
		Backend:          BackendTrueNASNFS,
		StorageClassName: "truenas",
		TrueNAS: &TrueNASCSIConfig{
			APIURL:        "https://truenas.local",
			APIKey:        "key",
			Pool:          "tank",
			DatasetParent: "tank/k8s",
		},
	}

	err := config.Validate()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "truenas share_host is required")
}

func TestValidate_TrueNASiSCSI_Success(t *testing.T) {
	config := &Config{
		Backend:          BackendTrueNASiSCSI,
		StorageClassName: "truenas-iscsi",
		TrueNAS: &TrueNASCSIConfig{
			APIURL:        "https://truenas.local",
			APIKey:        "key",
			Pool:          "tank",
			DatasetParent: "tank/k8s/zvols",
		},
	}

	err := config.Validate()
	assert.NoError(t, err)
}

func TestValidate_UnsupportedBackend(t *testing.T) {
	config := &Config{
		Backend:          StorageBackend("unknown"),
		StorageClassName: "test",
	}

	err := config.Validate()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "unsupported storage backend")
}

func TestComponent_Name(t *testing.T) {
	comp := NewComponent(nil, nil)
	assert.Equal(t, "storage", comp.Name())
}

func TestComponent_Dependencies(t *testing.T) {
	comp := NewComponent(nil, nil)
	deps := comp.Dependencies()

	require.Len(t, deps, 1)
	assert.Contains(t, deps, "k3s")
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
	listReleases    []helm.Release
	listErr         error
	reposAdded      []helm.RepoAddOptions
	chartsInstalled []helm.InstallOptions
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
	return nil
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
	pods         []*k8s.Pod
	podsErr      error
	manifests    []string
	manifestsErr error
}

func (m *mockK8sClient) GetPods(ctx context.Context, namespace string) ([]*k8s.Pod, error) {
	return m.pods, m.podsErr
}

func (m *mockK8sClient) ApplyManifest(ctx context.Context, manifest string) error {
	m.manifests = append(m.manifests, manifest)
	return m.manifestsErr
}

func TestComponent_Status_LocalPathInstalled(t *testing.T) {
	helmClient := &mockHelmClient{
		listReleases: []helm.Release{
			{
				Name:       "local-path-provisioner",
				Namespace:  "kube-system",
				Status:     "deployed",
				AppVersion: "0.0.28",
			},
		},
	}

	comp := NewComponent(helmClient, nil)
	status, err := comp.Status(context.Background())

	assert.NoError(t, err)
	assert.True(t, status.Installed)
	assert.True(t, status.Healthy)
	assert.Equal(t, "0.0.28", status.Version)
	assert.Contains(t, status.Message, "local-path-provisioner")
}

func TestComponent_Status_NFSInstalled(t *testing.T) {
	helmClient := &mockHelmClient{
		listReleases: []helm.Release{
			{
				Name:       "nfs-subdir-external-provisioner",
				Namespace:  "kube-system",
				Status:     "deployed",
				AppVersion: "4.0.18",
			},
		},
	}

	comp := NewComponent(helmClient, nil)
	status, err := comp.Status(context.Background())

	assert.NoError(t, err)
	assert.True(t, status.Installed)
	assert.True(t, status.Healthy)
	assert.Equal(t, "4.0.18", status.Version)
}

func TestComponent_Status_NoRelease(t *testing.T) {
	helmClient := &mockHelmClient{
		listReleases: []helm.Release{},
	}

	comp := NewComponent(helmClient, nil)
	status, err := comp.Status(context.Background())

	assert.NoError(t, err)
	assert.False(t, status.Installed)
	assert.False(t, status.Healthy)
	assert.Contains(t, status.Message, "no storage provisioner found")
}
