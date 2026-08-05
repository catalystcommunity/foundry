package storage

import (
	"context"
	"testing"
	"time"

	"github.com/catalystcommunity/foundry/v1/internal/helm"
	"github.com/catalystcommunity/foundry/v1/internal/k8s"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestInstall_LocalPath_Success(t *testing.T) {
	helmClient := &mockHelmClient{
		listErr: assert.AnError, // Force List to fail so code proceeds to install
	}
	k8sClient := &mockK8sClient{}

	cfg := DefaultConfig()
	err := Install(context.Background(), helmClient, k8sClient, cfg)
	require.NoError(t, err)

	// The official Rancher chart is in an OCI registry.
	assert.Empty(t, helmClient.reposAdded)

	// Verify chart was installed
	require.Len(t, helmClient.chartsInstalled, 1)
	assert.Equal(t, "local-path-provisioner", helmClient.chartsInstalled[0].ReleaseName)
	assert.Equal(t, "kube-system", helmClient.chartsInstalled[0].Namespace)
	assert.Equal(t, localPathChart, helmClient.chartsInstalled[0].Chart)
	assert.Equal(t, "0.0.36", helmClient.chartsInstalled[0].Version)
	assert.True(t, helmClient.chartsInstalled[0].CreateNamespace)
	assert.True(t, helmClient.chartsInstalled[0].Wait)
	assert.Equal(t, 5*time.Minute, helmClient.chartsInstalled[0].Timeout)
}

func TestInstall_LocalPath_MapsLegacyDefaultVersion(t *testing.T) {
	helmClient := &mockHelmClient{listErr: assert.AnError}
	cfg := DefaultConfig()
	cfg.Version = "0.0.28"

	err := Install(context.Background(), helmClient, &mockK8sClient{}, cfg)
	require.NoError(t, err)
	require.Len(t, helmClient.chartsInstalled, 1)
	assert.Equal(t, "0.0.36", helmClient.chartsInstalled[0].Version)
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

func TestInstall_LocalPath_K3sBuiltIn(t *testing.T) {
	helmClient := &mockHelmClient{}
	k8sClient := &mockK8sClient{pods: []*k8s.Pod{{Name: "local-path-provisioner-abc123"}}}

	err := Install(context.Background(), helmClient, k8sClient, DefaultConfig())
	require.NoError(t, err)

	assert.Empty(t, helmClient.reposAdded)
	assert.Empty(t, helmClient.chartsInstalled)
}

func TestInstall_LocalPath_PodCheckError(t *testing.T) {
	helmClient := &mockHelmClient{}
	k8sClient := &mockK8sClient{podsErr: assert.AnError}

	err := Install(context.Background(), helmClient, k8sClient, DefaultConfig())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to check for local-path-provisioner")
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

func TestInstall_Longhorn_Success(t *testing.T) {
	helmClient := &mockHelmClient{}
	k8sClient := &mockK8sClient{}

	cfg := &Config{
		Backend:          BackendLonghorn,
		Namespace:        "longhorn-system",
		StorageClassName: "longhorn",
		SetDefault:       true,
		Longhorn: &LonghornConfig{
			ReplicaCount:                 3,
			DataPath:                     "/var/lib/longhorn",
			GuaranteedInstanceManagerCPU: 12,
			DefaultDataLocality:          "disabled",
		},
	}

	err := Install(context.Background(), helmClient, k8sClient, cfg)
	require.NoError(t, err)

	// Verify repo was added
	require.Len(t, helmClient.reposAdded, 1)
	assert.Equal(t, longhornRepoName, helmClient.reposAdded[0].Name)
	assert.Equal(t, longhornRepoURL, helmClient.reposAdded[0].URL)

	// Verify chart was installed
	require.Len(t, helmClient.chartsInstalled, 1)
	assert.Equal(t, "longhorn", helmClient.chartsInstalled[0].ReleaseName)
	assert.Equal(t, "longhorn-system", helmClient.chartsInstalled[0].Namespace)
	assert.Equal(t, longhornChart, helmClient.chartsInstalled[0].Chart)
	assert.Equal(t, 10*time.Minute, helmClient.chartsInstalled[0].Timeout)
}

func TestInstall_Longhorn_AlreadyInstalled(t *testing.T) {
	helmClient := &mockHelmClient{
		listReleases: []helm.Release{
			{
				Name:      "longhorn",
				Namespace: "longhorn-system",
				Status:    "deployed",
			},
		},
	}
	k8sClient := &mockK8sClient{}

	cfg := &Config{
		Backend:          BackendLonghorn,
		Namespace:        "longhorn-system",
		StorageClassName: "longhorn",
		SetDefault:       true,
		Longhorn: &LonghornConfig{
			ReplicaCount: 3,
			DataPath:     "/var/lib/longhorn",
		},
	}

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
	k8sClient := &mockK8sClient{}

	// Should use default config (local-path)
	err := Install(context.Background(), helmClient, k8sClient, nil)
	require.NoError(t, err)

	// No built-in provisioner exists, so Foundry installs one.
	require.Len(t, helmClient.chartsInstalled, 1)
}

func TestInstall_AddRepoError(t *testing.T) {
	helmClient := &mockHelmClient{
		addRepoErr: assert.AnError,
	}
	k8sClient := &mockK8sClient{}
	cfg := &Config{
		Backend:   BackendNFS,
		Namespace: "kube-system",
		NFS:       &NFSConfig{Server: "192.0.2.1", Path: "/exports"},
	}

	err := Install(context.Background(), helmClient, k8sClient, cfg)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to add helm repository")
}

func TestInstall_InstallChartError(t *testing.T) {
	helmClient := &mockHelmClient{
		installErr: assert.AnError,
		listErr:    assert.AnError, // Force List to fail so code falls through to Install
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

func TestInstall_FailedReleaseUpgraded(t *testing.T) {
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

	// Should have attempted to upgrade the failed release (not uninstall - to avoid data loss)
	require.Len(t, helmClient.upgradeCalls, 1)
	assert.Equal(t, "local-path-provisioner", helmClient.upgradeCalls[0].ReleaseName)

	// Should NOT have installed fresh (upgrade was used instead)
	assert.Empty(t, helmClient.chartsInstalled)
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

func TestBuildLonghornValues(t *testing.T) {
	cfg := &Config{
		StorageClassName: "longhorn",
		SetDefault:       true,
		Longhorn: &LonghornConfig{
			ReplicaCount:                 3,
			DataPath:                     "/var/lib/longhorn",
			GuaranteedInstanceManagerCPU: 12,
			DefaultDataLocality:          "best-effort",
		},
		Values: map[string]interface{}{},
	}

	values := buildLonghornValues(cfg)

	// Check default settings
	defaultSettings, ok := values["defaultSettings"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, 3, defaultSettings["defaultReplicaCount"])
	assert.Equal(t, "/var/lib/longhorn", defaultSettings["defaultDataPath"])
	assert.Equal(t, 12, defaultSettings["guaranteedInstanceManagerCPU"])
	assert.Equal(t, "best-effort", defaultSettings["defaultDataLocality"])

	// Check persistence settings
	persistence, ok := values["persistence"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, true, persistence["defaultClass"])
	assert.Equal(t, 3, persistence["defaultClassReplicaCount"])
	assert.Equal(t, "Delete", persistence["reclaimPolicy"])

	// Check ingress disabled
	ingress, ok := values["ingress"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, false, ingress["enabled"])
}

func TestBuildLonghornValues_CustomValues(t *testing.T) {
	cfg := &Config{
		StorageClassName: "longhorn",
		SetDefault:       false,
		Longhorn: &LonghornConfig{
			ReplicaCount: 2,
		},
		Values: map[string]interface{}{
			"customSetting": "customValue",
		},
	}

	values := buildLonghornValues(cfg)

	// Check custom values preserved
	assert.Equal(t, "customValue", values["customSetting"])

	// Check persistence reflects SetDefault
	persistence, ok := values["persistence"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, false, persistence["defaultClass"])
}

func TestInstall_Longhorn_ConfiguresNodeDisks(t *testing.T) {
	helmClient := &mockHelmClient{}
	k8sClient := &mockK8sClient{}
	cfg := &Config{
		Backend:   BackendLonghorn,
		Namespace: "longhorn-system",
		Longhorn: &LonghornConfig{
			ReplicaCount: 3,
			NodeDisks: map[string]LonghornNodeDiskConfig{
				"worker-b": {Path: "/data/b", StorageReserved: 200},
				"worker-a": {Path: "/data/a", StorageReserved: 100},
			},
		},
	}

	err := Install(context.Background(), helmClient, k8sClient, cfg)
	require.NoError(t, err)
	require.Len(t, k8sClient.patches, 2)

	assert.Equal(t, "worker-a", k8sClient.patches[0].name)
	assert.Equal(t, "longhorn-system", k8sClient.patches[0].namespace)
	assert.Equal(t, "longhorn.io", k8sClient.patches[0].gvr.Group)
	assert.JSONEq(t, `{
		"spec": {"disks": {"foundry-managed": {
			"allowScheduling": true,
			"diskDriver": "",
			"diskType": "filesystem",
			"evictionRequested": false,
			"path": "/data/a",
			"storageReserved": 100,
			"tags": []
		}}}
	}`, string(k8sClient.patches[0].patch))
}

func TestInstall_Longhorn_NodeDiskPatchError(t *testing.T) {
	helmClient := &mockHelmClient{}
	k8sClient := &mockK8sClient{patchErr: assert.AnError}
	cfg := &Config{
		Backend:   BackendLonghorn,
		Namespace: "longhorn-system",
		Longhorn: &LonghornConfig{
			ReplicaCount: 3,
			NodeDisks: map[string]LonghornNodeDiskConfig{
				"worker-a": {Path: "/data/a"},
			},
		},
	}

	err := Install(context.Background(), helmClient, k8sClient, cfg)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to configure Longhorn disk on node worker-a")
}
