package velero

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

	assert.Equal(t, "8.0.0", config.Version)
	assert.Equal(t, "velero", config.Namespace)
	assert.Equal(t, ProviderS3, config.Provider)
	assert.Equal(t, "http://seaweedfs-s3.seaweedfs.svc.cluster.local:8333", config.S3Endpoint)
	assert.Equal(t, "velero", config.S3Bucket)
	assert.Equal(t, "us-east-1", config.S3Region)
	assert.True(t, config.S3InsecureSkipTLSVerify)
	assert.True(t, config.S3ForcePathStyle)
	assert.True(t, config.DefaultBackupStorageLocation)
	assert.False(t, config.DefaultVolumeSnapshotLocations)
	assert.Equal(t, 30, config.BackupRetentionDays)
	assert.Equal(t, "daily-backup", config.ScheduleName)
	assert.Equal(t, "", config.ScheduleCron)
	assert.Empty(t, config.ScheduleIncludedNamespaces)
	assert.Contains(t, config.ScheduleExcludedNamespaces, "kube-system")
	assert.Contains(t, config.ScheduleExcludedNamespaces, "velero")
	assert.False(t, config.SnapshotsEnabled)
	assert.Equal(t, "10m", config.CSISnapshotTimeout)
	assert.NotNil(t, config.ResourceRequests)
	assert.Equal(t, "128Mi", config.ResourceRequests["memory"])
	assert.NotNil(t, config.Values)
}

func TestParseConfig_Defaults(t *testing.T) {
	cfg := component.ComponentConfig{}

	config, err := ParseConfig(cfg)
	require.NoError(t, err)

	assert.Equal(t, "8.0.0", config.Version)
	assert.Equal(t, "velero", config.Namespace)
	assert.Equal(t, ProviderS3, config.Provider)
}

func TestParseConfig_CustomValues(t *testing.T) {
	cfg := component.ComponentConfig{
		"version":                           "7.0.0",
		"namespace":                         "custom-velero",
		"provider":                          "aws",
		"s3_endpoint":                       "https://s3.amazonaws.com",
		"s3_bucket":                         "my-backups",
		"s3_region":                         "us-west-2",
		"s3_access_key":                     "AKIAIOSFODNN7EXAMPLE",
		"s3_secret_key":                     "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY",
		"s3_insecure_skip_tls_verify":       false,
		"s3_force_path_style":               false,
		"default_backup_storage_location":   false,
		"default_volume_snapshot_locations": true,
		"backup_retention_days":             14,
		"schedule_name":                     "weekly-backup",
		"schedule_cron":                     "0 2 * * 0",
		"snapshots_enabled":                 true,
		"csi_snapshot_timeout":              "15m",
	}

	config, err := ParseConfig(cfg)
	require.NoError(t, err)

	assert.Equal(t, "7.0.0", config.Version)
	assert.Equal(t, "custom-velero", config.Namespace)
	assert.Equal(t, ProviderAWS, config.Provider)
	assert.Equal(t, "https://s3.amazonaws.com", config.S3Endpoint)
	assert.Equal(t, "my-backups", config.S3Bucket)
	assert.Equal(t, "us-west-2", config.S3Region)
	assert.Equal(t, "AKIAIOSFODNN7EXAMPLE", config.S3AccessKey)
	assert.Equal(t, "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY", config.S3SecretKey)
	assert.False(t, config.S3InsecureSkipTLSVerify)
	assert.False(t, config.S3ForcePathStyle)
	assert.False(t, config.DefaultBackupStorageLocation)
	assert.True(t, config.DefaultVolumeSnapshotLocations)
	assert.Equal(t, 14, config.BackupRetentionDays)
	assert.Equal(t, "weekly-backup", config.ScheduleName)
	assert.Equal(t, "0 2 * * 0", config.ScheduleCron)
	assert.True(t, config.SnapshotsEnabled)
	assert.Equal(t, "15m", config.CSISnapshotTimeout)
}

func TestParseConfig_WithNamespaceFilters(t *testing.T) {
	cfg := component.ComponentConfig{
		"schedule_included_namespaces": []interface{}{"default", "production"},
		"schedule_excluded_namespaces": []interface{}{"kube-system", "velero", "monitoring"},
	}

	config, err := ParseConfig(cfg)
	require.NoError(t, err)

	assert.Equal(t, []string{"default", "production"}, config.ScheduleIncludedNamespaces)
	assert.Equal(t, []string{"kube-system", "velero", "monitoring"}, config.ScheduleExcludedNamespaces)
}

func TestParseConfig_WithResourceLimits(t *testing.T) {
	cfg := component.ComponentConfig{
		"resource_requests": map[string]interface{}{
			"cpu":    "200m",
			"memory": "256Mi",
		},
		"resource_limits": map[string]interface{}{
			"cpu":    "1000m",
			"memory": "1Gi",
		},
	}

	config, err := ParseConfig(cfg)
	require.NoError(t, err)

	assert.Equal(t, "200m", config.ResourceRequests["cpu"])
	assert.Equal(t, "256Mi", config.ResourceRequests["memory"])
	assert.Equal(t, "1000m", config.ResourceLimits["cpu"])
	assert.Equal(t, "1Gi", config.ResourceLimits["memory"])
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

func TestValidate_Success_S3(t *testing.T) {
	config := &Config{
		Provider:   ProviderS3,
		S3Endpoint: "http://garage:3900",
		S3Bucket:   "velero",
	}

	err := config.Validate()
	assert.NoError(t, err)
}

func TestValidate_Success_AWS(t *testing.T) {
	config := &Config{
		Provider: ProviderAWS,
		S3Bucket: "my-backup-bucket",
		// AWS provider doesn't require endpoint
	}

	err := config.Validate()
	assert.NoError(t, err)
}

func TestValidate_UnsupportedProvider(t *testing.T) {
	config := &Config{
		Provider: "gcs",
		S3Bucket: "bucket",
	}

	err := config.Validate()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "unsupported provider")
}

func TestValidate_MissingBucket(t *testing.T) {
	config := &Config{
		Provider: ProviderS3,
		S3Bucket: "",
	}

	err := config.Validate()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "s3_bucket is required")
}

func TestValidate_S3_MissingEndpoint(t *testing.T) {
	config := &Config{
		Provider:   ProviderS3,
		S3Bucket:   "velero",
		S3Endpoint: "",
	}

	err := config.Validate()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "s3_endpoint is required")
}

func TestValidate_NegativeRetentionDays(t *testing.T) {
	config := &Config{
		Provider:            ProviderS3,
		S3Endpoint:         "http://garage:3900",
		S3Bucket:           "velero",
		BackupRetentionDays: -1,
	}

	err := config.Validate()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "cannot be negative")
}

func TestGetVeleroEndpoint(t *testing.T) {
	config := &Config{
		Namespace: "velero",
	}

	endpoint := config.GetVeleroEndpoint()
	assert.Equal(t, "http://velero.velero.svc.cluster.local:8085", endpoint)
}

func TestGetVeleroEndpoint_CustomNamespace(t *testing.T) {
	config := &Config{
		Namespace: "custom-ns",
	}

	endpoint := config.GetVeleroEndpoint()
	assert.Equal(t, "http://velero.custom-ns.svc.cluster.local:8085", endpoint)
}

func TestGetBackupStorageLocationName(t *testing.T) {
	config := &Config{}
	assert.Equal(t, "default", config.GetBackupStorageLocationName())
}

func TestComponent_Name(t *testing.T) {
	comp := NewComponent(nil, nil)
	assert.Equal(t, "velero", comp.Name())
}

func TestComponent_Dependencies(t *testing.T) {
	comp := NewComponent(nil, nil)
	deps := comp.Dependencies()

	require.Len(t, deps, 1)
	assert.Contains(t, deps, "seaweedfs")
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

func TestComponent_Status_VeleroInstalled(t *testing.T) {
	helmClient := &mockHelmClient{
		listReleases: []helm.Release{
			{
				Name:       "velero",
				Namespace:  "velero",
				Status:     "deployed",
				AppVersion: "1.14.0",
			},
		},
	}

	comp := NewComponent(helmClient, nil)
	status, err := comp.Status(context.Background())

	assert.NoError(t, err)
	assert.True(t, status.Installed)
	assert.True(t, status.Healthy)
	assert.Equal(t, "1.14.0", status.Version)
}

func TestComponent_Status_VeleroNotInstalled(t *testing.T) {
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

func TestComponent_Status_VeleroFailed(t *testing.T) {
	helmClient := &mockHelmClient{
		listReleases: []helm.Release{
			{
				Name:       "velero",
				Namespace:  "velero",
				Status:     "failed",
				AppVersion: "1.14.0",
			},
		},
	}

	comp := NewComponent(helmClient, nil)
	status, err := comp.Status(context.Background())

	assert.NoError(t, err)
	assert.True(t, status.Installed)
	assert.False(t, status.Healthy)
}

func TestComponent_Status_ListError(t *testing.T) {
	helmClient := &mockHelmClient{
		listErr: assert.AnError,
	}

	comp := NewComponent(helmClient, nil)
	status, err := comp.Status(context.Background())

	assert.NoError(t, err)
	assert.False(t, status.Installed)
	assert.False(t, status.Healthy)
	assert.Contains(t, status.Message, "failed to list releases")
}
