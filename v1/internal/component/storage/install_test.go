package storage

import (
	"context"
	"testing"
	"time"

	"github.com/catalystcommunity/foundry/v1/internal/helm"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestInstall_LocalPath_Success(t *testing.T) {
	helmClient := &mockHelmClient{}
	k8sClient := &mockK8sClient{}

	cfg := DefaultConfig()
	err := Install(context.Background(), helmClient, k8sClient, cfg)
	require.NoError(t, err)

	// Verify repo was added
	require.Len(t, helmClient.reposAdded, 1)
	assert.Equal(t, localPathRepoName, helmClient.reposAdded[0].Name)
	assert.Equal(t, localPathRepoURL, helmClient.reposAdded[0].URL)

	// Verify chart was installed
	require.Len(t, helmClient.chartsInstalled, 1)
	assert.Equal(t, "local-path-provisioner", helmClient.chartsInstalled[0].ReleaseName)
	assert.Equal(t, "kube-system", helmClient.chartsInstalled[0].Namespace)
	assert.Equal(t, localPathChart, helmClient.chartsInstalled[0].Chart)
	assert.True(t, helmClient.chartsInstalled[0].CreateNamespace)
	assert.True(t, helmClient.chartsInstalled[0].Wait)
	assert.Equal(t, 5*time.Minute, helmClient.chartsInstalled[0].Timeout)
}

func TestInstall_LocalPath_AlreadyInstalled(t *testing.T) {
	helmClient := &mockHelmClient{
		listReleases: []helm.Release{
			{
				Name:      "local-path-provisioner",
				Namespace: "kube-system",
				Status:    "deployed",
			},
		},
	}
	k8sClient := &mockK8sClient{}

	cfg := DefaultConfig()
	err := Install(context.Background(), helmClient, k8sClient, cfg)
	require.NoError(t, err)

	// Should not install again
	assert.Empty(t, helmClient.chartsInstalled)
}

func TestInstall_NFS_Success(t *testing.T) {
	helmClient := &mockHelmClient{}
	k8sClient := &mockK8sClient{}

	cfg := &Config{
		Backend:          BackendNFS,
		Namespace:        "kube-system",
		StorageClassName: "nfs-storage",
		SetDefault:       true,
		NFS: &NFSConfig{
			Server:        "192.168.1.100",
			Path:          "/mnt/nfs/k8s",
			ReclaimPolicy: "Delete",
		},
	}

	err := Install(context.Background(), helmClient, k8sClient, cfg)
	require.NoError(t, err)

	// Verify repo was added
	require.Len(t, helmClient.reposAdded, 1)
	assert.Equal(t, nfsRepoName, helmClient.reposAdded[0].Name)
	assert.Equal(t, nfsRepoURL, helmClient.reposAdded[0].URL)

	// Verify chart was installed
	require.Len(t, helmClient.chartsInstalled, 1)
	assert.Equal(t, "nfs-subdir-external-provisioner", helmClient.chartsInstalled[0].ReleaseName)
	assert.Equal(t, "kube-system", helmClient.chartsInstalled[0].Namespace)
	assert.Equal(t, nfsChart, helmClient.chartsInstalled[0].Chart)
}

func TestInstall_TrueNASNFS_Success(t *testing.T) {
	helmClient := &mockHelmClient{}
	k8sClient := &mockK8sClient{}

	cfg := &Config{
		Backend:          BackendTrueNASNFS,
		Namespace:        "democratic-csi",
		StorageClassName: "truenas-nfs",
		SetDefault:       true,
		TrueNAS: &TrueNASCSIConfig{
			APIURL:        "https://truenas.local",
			APIKey:        "secret-key",
			Pool:          "tank",
			DatasetParent: "tank/k8s/volumes",
			ShareHost:     "192.168.1.50",
			ShareNetworks: []string{"192.168.1.0/24"},
		},
	}

	err := Install(context.Background(), helmClient, k8sClient, cfg)
	require.NoError(t, err)

	// Verify repo was added
	require.Len(t, helmClient.reposAdded, 1)
	assert.Equal(t, democraticCSIRepoName, helmClient.reposAdded[0].Name)
	assert.Equal(t, democraticCSIRepoURL, helmClient.reposAdded[0].URL)

	// Verify chart was installed
	require.Len(t, helmClient.chartsInstalled, 1)
	assert.Equal(t, "democratic-csi-nfs", helmClient.chartsInstalled[0].ReleaseName)
	assert.Equal(t, "democratic-csi", helmClient.chartsInstalled[0].Namespace)
	assert.Equal(t, democraticCSIChart, helmClient.chartsInstalled[0].Chart)
	assert.Equal(t, 10*time.Minute, helmClient.chartsInstalled[0].Timeout)
}

func TestInstall_TrueNASiSCSI_Success(t *testing.T) {
	helmClient := &mockHelmClient{}
	k8sClient := &mockK8sClient{}

	cfg := &Config{
		Backend:          BackendTrueNASiSCSI,
		Namespace:        "democratic-csi",
		StorageClassName: "truenas-iscsi",
		SetDefault:       true,
		TrueNAS: &TrueNASCSIConfig{
			APIURL:           "https://truenas.local",
			APIKey:           "secret-key",
			Pool:             "tank",
			DatasetParent:    "tank/k8s/zvols",
			PortalID:         1,
			InitiatorGroupID: 1,
		},
	}

	err := Install(context.Background(), helmClient, k8sClient, cfg)
	require.NoError(t, err)

	// Verify chart was installed
	require.Len(t, helmClient.chartsInstalled, 1)
	assert.Equal(t, "democratic-csi-iscsi", helmClient.chartsInstalled[0].ReleaseName)
	assert.Equal(t, "democratic-csi", helmClient.chartsInstalled[0].Namespace)
}

func TestInstall_NilHelmClient(t *testing.T) {
	err := Install(context.Background(), nil, &mockK8sClient{}, DefaultConfig())
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "helm client cannot be nil")
}

func TestInstall_NilConfig(t *testing.T) {
	helmClient := &mockHelmClient{}
	k8sClient := &mockK8sClient{}

	// Should use default config (local-path)
	err := Install(context.Background(), helmClient, k8sClient, nil)
	require.NoError(t, err)

	// Verify installation happened with defaults
	require.Len(t, helmClient.chartsInstalled, 1)
	assert.Equal(t, "local-path-provisioner", helmClient.chartsInstalled[0].ReleaseName)
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
	assert.Contains(t, err.Error(), "failed to install local-path-provisioner")
}

func TestInstall_UnsupportedBackend(t *testing.T) {
	helmClient := &mockHelmClient{}
	k8sClient := &mockK8sClient{}

	cfg := &Config{
		Backend: StorageBackend("unknown"),
	}

	err := Install(context.Background(), helmClient, k8sClient, cfg)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "unsupported storage backend")
}

func TestInstall_FailedReleaseCleanedup(t *testing.T) {
	helmClient := &mockHelmClient{
		listReleases: []helm.Release{
			{
				Name:      "local-path-provisioner",
				Namespace: "kube-system",
				Status:    "failed",
			},
		},
	}
	k8sClient := &mockK8sClient{}

	cfg := DefaultConfig()
	err := Install(context.Background(), helmClient, k8sClient, cfg)
	require.NoError(t, err)

	// Should have uninstalled the failed release
	require.Len(t, helmClient.uninstallCalls, 1)
	assert.Equal(t, "local-path-provisioner", helmClient.uninstallCalls[0].ReleaseName)

	// And installed fresh
	require.Len(t, helmClient.chartsInstalled, 1)
}

func TestBuildLocalPathValues(t *testing.T) {
	cfg := &Config{
		StorageClassName: "local-path",
		SetDefault:       true,
		LocalPath: &LocalPathConfig{
			Path:          "/data/volumes",
			ReclaimPolicy: "Retain",
		},
		Values: map[string]interface{}{
			"custom": "value",
		},
	}

	values := buildLocalPathValues(cfg)

	// Check custom values preserved
	assert.Equal(t, "value", values["custom"])

	// Check storage class config
	storageClass, ok := values["storageClass"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, true, storageClass["create"])
	assert.Equal(t, "local-path", storageClass["name"])
	assert.Equal(t, true, storageClass["defaultClass"])
	assert.Equal(t, "Retain", storageClass["reclaimPolicy"])

	// Check node path map
	nodePathMap, ok := values["nodePathMap"].([]map[string]interface{})
	require.True(t, ok)
	require.Len(t, nodePathMap, 1)
	paths, ok := nodePathMap[0]["paths"].([]string)
	require.True(t, ok)
	assert.Contains(t, paths, "/data/volumes")
}

func TestBuildNFSValues(t *testing.T) {
	cfg := &Config{
		StorageClassName: "nfs-storage",
		SetDefault:       true,
		NFS: &NFSConfig{
			Server:          "192.168.1.100",
			Path:            "/mnt/nfs/k8s",
			ReclaimPolicy:   "Retain",
			ArchiveOnDelete: true,
		},
		Values: map[string]interface{}{},
	}

	values := buildNFSValues(cfg)

	// Check NFS config
	nfs, ok := values["nfs"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "192.168.1.100", nfs["server"])
	assert.Equal(t, "/mnt/nfs/k8s", nfs["path"])

	// Check storage class config
	storageClass, ok := values["storageClass"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, true, storageClass["create"])
	assert.Equal(t, "nfs-storage", storageClass["name"])
	assert.Equal(t, true, storageClass["defaultClass"])
	assert.Equal(t, "Retain", storageClass["reclaimPolicy"])
	assert.Equal(t, true, storageClass["archiveOnDelete"])
}

func TestBuildTrueNASNFSValues(t *testing.T) {
	cfg := &Config{
		StorageClassName: "truenas-nfs",
		SetDefault:       true,
		TrueNAS: &TrueNASCSIConfig{
			APIURL:        "https://truenas.local",
			APIKey:        "secret-key",
			Pool:          "tank",
			DatasetParent: "tank/k8s/volumes",
			ShareHost:     "192.168.1.50",
			ShareNetworks: []string{"192.168.1.0/24"},
		},
		Values: map[string]interface{}{},
	}

	values := buildTrueNASNFSValues(cfg)

	// Check CSI driver name
	csiDriver, ok := values["csiDriver"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "org.democratic-csi.nfs", csiDriver["name"])

	// Check storage classes
	storageClasses, ok := values["storageClasses"].([]map[string]interface{})
	require.True(t, ok)
	require.Len(t, storageClasses, 1)
	assert.Equal(t, "truenas-nfs", storageClasses[0]["name"])
	assert.Equal(t, true, storageClasses[0]["defaultClass"])

	// Check driver config
	driver, ok := values["driver"].(map[string]interface{})
	require.True(t, ok)
	driverConfig, ok := driver["config"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "freenas-nfs", driverConfig["driver"])

	// Check HTTP connection
	httpConn, ok := driverConfig["httpConnection"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "https://truenas.local", httpConn["host"])
	assert.Equal(t, "secret-key", httpConn["apiKey"])

	// Check ZFS config
	zfs, ok := driverConfig["zfs"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "tank/k8s/volumes", zfs["datasetParentName"])

	// Check NFS config
	nfs, ok := driverConfig["nfs"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "192.168.1.50", nfs["shareHost"])
	shareNetworks, ok := nfs["shareAllowedNetworks"].([]string)
	require.True(t, ok)
	assert.Contains(t, shareNetworks, "192.168.1.0/24")
}

func TestBuildTrueNASNFSValues_DefaultNetworks(t *testing.T) {
	cfg := &Config{
		StorageClassName: "truenas-nfs",
		SetDefault:       true,
		TrueNAS: &TrueNASCSIConfig{
			APIURL:        "https://truenas.local",
			APIKey:        "secret-key",
			Pool:          "tank",
			DatasetParent: "tank/k8s/volumes",
			ShareHost:     "192.168.1.50",
			ShareNetworks: nil, // Empty - should default to 0.0.0.0/0
		},
		Values: map[string]interface{}{},
	}

	values := buildTrueNASNFSValues(cfg)

	driver, ok := values["driver"].(map[string]interface{})
	require.True(t, ok)
	driverConfig, ok := driver["config"].(map[string]interface{})
	require.True(t, ok)
	nfs, ok := driverConfig["nfs"].(map[string]interface{})
	require.True(t, ok)
	shareNetworks, ok := nfs["shareAllowedNetworks"].([]string)
	require.True(t, ok)
	assert.Contains(t, shareNetworks, "0.0.0.0/0")
}

func TestBuildTrueNASiSCSIValues(t *testing.T) {
	cfg := &Config{
		StorageClassName: "truenas-iscsi",
		SetDefault:       true,
		TrueNAS: &TrueNASCSIConfig{
			APIURL:           "https://truenas.local",
			APIKey:           "secret-key",
			Pool:             "tank",
			DatasetParent:    "tank/k8s/zvols",
			PortalID:         1,
			InitiatorGroupID: 2,
		},
		Values: map[string]interface{}{},
	}

	values := buildTrueNASiSCSIValues(cfg)

	// Check CSI driver name
	csiDriver, ok := values["csiDriver"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "org.democratic-csi.iscsi", csiDriver["name"])

	// Check storage classes
	storageClasses, ok := values["storageClasses"].([]map[string]interface{})
	require.True(t, ok)
	require.Len(t, storageClasses, 1)
	params, ok := storageClasses[0]["parameters"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "ext4", params["fsType"])

	// Check driver config
	driver, ok := values["driver"].(map[string]interface{})
	require.True(t, ok)
	driverConfig, ok := driver["config"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "freenas-iscsi", driverConfig["driver"])

	// Check iSCSI config
	iscsi, ok := driverConfig["iscsi"].(map[string]interface{})
	require.True(t, ok)
	targetGroups, ok := iscsi["targetGroups"].([]map[string]interface{})
	require.True(t, ok)
	require.Len(t, targetGroups, 1)
	assert.Equal(t, 1, targetGroups[0]["targetGroupPortalGroup"])
	assert.Equal(t, 2, targetGroups[0]["targetGroupInitiatorGroup"])

	// Check node config
	node, ok := values["node"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, true, node["hostPID"])
}

// mockOpenBAOClient implements OpenBAOClient for testing
type mockOpenBAOClient struct {
	secrets    map[string]map[string]interface{}
	writeErr   error
	readErr    error
	writeCalls []mockOpenBAOWriteCall
}

type mockOpenBAOWriteCall struct {
	mount string
	path  string
	data  map[string]interface{}
}

func newMockOpenBAOClient() *mockOpenBAOClient {
	return &mockOpenBAOClient{
		secrets:    make(map[string]map[string]interface{}),
		writeCalls: make([]mockOpenBAOWriteCall, 0),
	}
}

func (m *mockOpenBAOClient) WriteSecretV2(ctx context.Context, mount, path string, data map[string]interface{}) error {
	m.writeCalls = append(m.writeCalls, mockOpenBAOWriteCall{mount: mount, path: path, data: data})
	if m.writeErr != nil {
		return m.writeErr
	}
	key := mount + "/" + path
	m.secrets[key] = data
	return nil
}

func (m *mockOpenBAOClient) ReadSecretV2(ctx context.Context, mount, path string) (map[string]interface{}, error) {
	if m.readErr != nil {
		return nil, m.readErr
	}
	key := mount + "/" + path
	data, ok := m.secrets[key]
	if !ok {
		return nil, nil
	}
	return data, nil
}

func TestEnsureTrueNASAPIKey_ExistingKey(t *testing.T) {
	ctx := context.Background()
	client := newMockOpenBAOClient()
	client.secrets["foundry-core/truenas"] = map[string]interface{}{
		"api_key": "existing-key",
	}

	key, err := EnsureTrueNASAPIKey(ctx, client, "")

	assert.NoError(t, err)
	assert.Equal(t, "existing-key", key)
	assert.Empty(t, client.writeCalls) // Should not write since key exists
}

func TestEnsureTrueNASAPIKey_StoreNewKey(t *testing.T) {
	ctx := context.Background()
	client := newMockOpenBAOClient()

	key, err := EnsureTrueNASAPIKey(ctx, client, "new-api-key")

	assert.NoError(t, err)
	assert.Equal(t, "new-api-key", key)
	require.Len(t, client.writeCalls, 1)
	assert.Equal(t, "foundry-core", client.writeCalls[0].mount)
	assert.Equal(t, "truenas", client.writeCalls[0].path)
	assert.Equal(t, "new-api-key", client.writeCalls[0].data["api_key"])
}

func TestEnsureTrueNASAPIKey_NoKeyAvailable(t *testing.T) {
	ctx := context.Background()
	client := newMockOpenBAOClient()

	_, err := EnsureTrueNASAPIKey(ctx, client, "")

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "no TrueNAS API key found")
}

func TestGetTrueNASAPIKey_Success(t *testing.T) {
	ctx := context.Background()
	client := newMockOpenBAOClient()
	client.secrets["foundry-core/truenas"] = map[string]interface{}{
		"api_key": "test-key",
	}

	key, err := GetTrueNASAPIKey(ctx, client)

	assert.NoError(t, err)
	assert.Equal(t, "test-key", key)
}

func TestGetTrueNASAPIKey_NotFound(t *testing.T) {
	ctx := context.Background()
	client := newMockOpenBAOClient()

	_, err := GetTrueNASAPIKey(ctx, client)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}
