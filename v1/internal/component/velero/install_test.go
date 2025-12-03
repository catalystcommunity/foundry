package velero

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
			{Name: "velero-abc123", Namespace: "velero", Status: "Running"},
		},
	}

	cfg := DefaultConfig()
	cfg.S3AccessKey = "testkey"
	cfg.S3SecretKey = "testsecret"
	err := Install(context.Background(), helmClient, k8sClient, cfg)
	require.NoError(t, err)

	// Verify repo was added
	require.Len(t, helmClient.reposAdded, 1)
	assert.Equal(t, veleroRepoName, helmClient.reposAdded[0].Name)
	assert.Equal(t, veleroRepoURL, helmClient.reposAdded[0].URL)

	// Verify chart was installed
	require.Len(t, helmClient.chartsInstalled, 1)
	assert.Equal(t, releaseName, helmClient.chartsInstalled[0].ReleaseName)
	assert.Equal(t, "velero", helmClient.chartsInstalled[0].Namespace)
	assert.Equal(t, veleroChart, helmClient.chartsInstalled[0].Chart)
	assert.True(t, helmClient.chartsInstalled[0].CreateNamespace)
	assert.True(t, helmClient.chartsInstalled[0].Wait)
	assert.Equal(t, 10*time.Minute, helmClient.chartsInstalled[0].Timeout)
}

func TestInstall_AlreadyInstalled(t *testing.T) {
	helmClient := &mockHelmClient{
		listReleases: []helm.Release{
			{
				Name:      releaseName,
				Namespace: "velero",
				Status:    "deployed",
			},
		},
	}
	k8sClient := &mockK8sClient{
		pods: []*k8s.Pod{
			{Name: "velero-abc123", Namespace: "velero", Status: "Running"},
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
			{Name: "velero-abc123", Namespace: "velero", Status: "Running"},
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
	assert.Contains(t, err.Error(), "failed to install velero")
}

func TestInstall_FailedReleaseCleanedup(t *testing.T) {
	helmClient := &mockHelmClient{
		listReleases: []helm.Release{
			{
				Name:      releaseName,
				Namespace: "velero",
				Status:    "failed",
			},
		},
	}
	k8sClient := &mockK8sClient{
		pods: []*k8s.Pod{
			{Name: "velero-abc123", Namespace: "velero", Status: "Running"},
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

func TestBuildHelmValues_Default(t *testing.T) {
	cfg := DefaultConfig()
	cfg.S3AccessKey = "testkey"
	cfg.S3SecretKey = "testsecret"
	values := buildHelmValues(cfg)

	// Check init containers for AWS plugin
	initContainers, ok := values["initContainers"].([]map[string]interface{})
	require.True(t, ok)
	require.Len(t, initContainers, 1)
	assert.Equal(t, "velero-plugin-for-aws", initContainers[0]["name"])

	// Check configuration
	configuration, ok := values["configuration"].(map[string]interface{})
	require.True(t, ok)

	// Check backup storage location
	bsl, ok := configuration["backupStorageLocation"].([]map[string]interface{})
	require.True(t, ok)
	require.Len(t, bsl, 1)
	assert.Equal(t, "default", bsl[0]["name"])
	assert.Equal(t, "aws", bsl[0]["provider"])
	assert.Equal(t, "velero", bsl[0]["bucket"])

	// Check credentials
	credentials, ok := values["credentials"].(map[string]interface{})
	require.True(t, ok)
	assert.True(t, credentials["useSecret"].(bool))

	// Check metrics are enabled
	metrics, ok := values["metrics"].(map[string]interface{})
	require.True(t, ok)
	assert.True(t, metrics["enabled"].(bool))
}

func TestBuildHelmValues_MinIOConfiguration(t *testing.T) {
	cfg := &Config{
		Provider:                ProviderMinIO,
		S3Endpoint:             "http://minio:9000",
		S3Bucket:               "velero",
		S3Region:               "us-east-1",
		S3AccessKey:            "minioadmin",
		S3SecretKey:            "minioadmin",
		S3InsecureSkipTLSVerify: true,
		S3ForcePathStyle:       true,
		Values:                 map[string]interface{}{},
	}

	values := buildHelmValues(cfg)

	configuration, ok := values["configuration"].(map[string]interface{})
	require.True(t, ok)

	bsl, ok := configuration["backupStorageLocation"].([]map[string]interface{})
	require.True(t, ok)
	require.Len(t, bsl, 1)

	bslConfig, ok := bsl[0]["config"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "http://minio:9000", bslConfig["s3Url"])
	assert.True(t, bslConfig["s3ForcePathStyle"].(bool))
	assert.Equal(t, "true", bslConfig["insecureSkipTLSVerify"])
}

func TestBuildHelmValues_AWSConfiguration(t *testing.T) {
	cfg := &Config{
		Provider:    ProviderAWS,
		S3Bucket:    "my-backups",
		S3Region:    "us-west-2",
		S3AccessKey: "AKIAIOSFODNN7EXAMPLE",
		S3SecretKey: "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY",
		Values:      map[string]interface{}{},
	}

	values := buildHelmValues(cfg)

	configuration, ok := values["configuration"].(map[string]interface{})
	require.True(t, ok)

	bsl, ok := configuration["backupStorageLocation"].([]map[string]interface{})
	require.True(t, ok)
	require.Len(t, bsl, 1)

	bslConfig, ok := bsl[0]["config"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "us-west-2", bslConfig["region"])
	// Should not have MinIO-specific settings
	assert.Nil(t, bslConfig["s3Url"])
	assert.Nil(t, bslConfig["s3ForcePathStyle"])
}

func TestBuildHelmValues_WithSnapshots(t *testing.T) {
	cfg := &Config{
		Provider:         ProviderMinIO,
		S3Endpoint:      "http://minio:9000",
		S3Bucket:        "velero",
		S3Region:        "us-east-1",
		S3AccessKey:     "minioadmin",
		S3SecretKey:     "minioadmin",
		SnapshotsEnabled: true,
		Values:          map[string]interface{}{},
	}

	values := buildHelmValues(cfg)

	assert.True(t, values["snapshotsEnabled"].(bool))
	assert.Equal(t, "EnableCSI", values["features"])

	// Check volume snapshot location is configured
	configuration, ok := values["configuration"].(map[string]interface{})
	require.True(t, ok)
	assert.NotNil(t, configuration["volumeSnapshotLocation"])
	assert.NotNil(t, configuration["defaultVolumeSnapshotLocations"])
}

func TestBuildHelmValues_WithSchedule(t *testing.T) {
	cfg := &Config{
		Provider:                   ProviderMinIO,
		S3Endpoint:                "http://minio:9000",
		S3Bucket:                  "velero",
		S3Region:                  "us-east-1",
		S3AccessKey:               "minioadmin",
		S3SecretKey:               "minioadmin",
		ScheduleName:              "daily-backup",
		ScheduleCron:              "0 2 * * *",
		BackupRetentionDays:       30,
		ScheduleExcludedNamespaces: []string{"kube-system", "velero"},
		Values:                    map[string]interface{}{},
	}

	values := buildHelmValues(cfg)

	schedules, ok := values["schedules"].(map[string]interface{})
	require.True(t, ok)

	dailyBackup, ok := schedules["daily-backup"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "0 2 * * *", dailyBackup["schedule"])

	template, ok := dailyBackup["template"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "720h", template["ttl"]) // 30 days * 24 hours
	assert.Equal(t, []string{"kube-system", "velero"}, template["excludedNamespaces"])
	assert.Equal(t, "default", template["storageLocation"])
	assert.True(t, template["includeClusterResources"].(bool))
}

func TestBuildHelmValues_WithIncludedNamespaces(t *testing.T) {
	cfg := &Config{
		Provider:                   ProviderMinIO,
		S3Endpoint:                "http://minio:9000",
		S3Bucket:                  "velero",
		S3Region:                  "us-east-1",
		S3AccessKey:               "minioadmin",
		S3SecretKey:               "minioadmin",
		ScheduleName:              "prod-backup",
		ScheduleCron:              "0 1 * * *",
		ScheduleIncludedNamespaces: []string{"production", "staging"},
		Values:                    map[string]interface{}{},
	}

	values := buildHelmValues(cfg)

	schedules, ok := values["schedules"].(map[string]interface{})
	require.True(t, ok)

	prodBackup, ok := schedules["prod-backup"].(map[string]interface{})
	require.True(t, ok)

	template, ok := prodBackup["template"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, []string{"production", "staging"}, template["includedNamespaces"])
}

func TestBuildHelmValues_WithResourceLimits(t *testing.T) {
	cfg := &Config{
		Provider:    ProviderMinIO,
		S3Endpoint: "http://minio:9000",
		S3Bucket:   "velero",
		S3Region:   "us-east-1",
		S3AccessKey: "minioadmin",
		S3SecretKey: "minioadmin",
		ResourceRequests: map[string]string{
			"cpu":    "200m",
			"memory": "256Mi",
		},
		ResourceLimits: map[string]string{
			"cpu":    "1000m",
			"memory": "1Gi",
		},
		Values: map[string]interface{}{},
	}

	values := buildHelmValues(cfg)

	resources, ok := values["resources"].(map[string]interface{})
	require.True(t, ok)

	requests, ok := resources["requests"].(map[string]string)
	require.True(t, ok)
	assert.Equal(t, "200m", requests["cpu"])
	assert.Equal(t, "256Mi", requests["memory"])

	limits, ok := resources["limits"].(map[string]string)
	require.True(t, ok)
	assert.Equal(t, "1000m", limits["cpu"])
	assert.Equal(t, "1Gi", limits["memory"])
}

func TestBuildHelmValues_CustomValues(t *testing.T) {
	cfg := &Config{
		Provider:    ProviderMinIO,
		S3Endpoint: "http://minio:9000",
		S3Bucket:   "velero",
		S3Region:   "us-east-1",
		S3AccessKey: "minioadmin",
		S3SecretKey: "minioadmin",
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

func TestBuildHelmValues_NoSchedule(t *testing.T) {
	cfg := &Config{
		Provider:     ProviderMinIO,
		S3Endpoint:  "http://minio:9000",
		S3Bucket:    "velero",
		S3Region:    "us-east-1",
		S3AccessKey:  "minioadmin",
		S3SecretKey:  "minioadmin",
		ScheduleCron: "", // No schedule
		Values:      map[string]interface{}{},
	}

	values := buildHelmValues(cfg)

	// Should not have schedules when cron is empty
	_, hasSchedules := values["schedules"]
	assert.False(t, hasSchedules)
}

func TestBuildBackupStorageLocation_MinIO(t *testing.T) {
	cfg := &Config{
		Provider:                     ProviderMinIO,
		S3Endpoint:                  "http://minio:9000",
		S3Bucket:                    "velero",
		S3Region:                    "us-east-1",
		S3InsecureSkipTLSVerify:      true,
		S3ForcePathStyle:            true,
		DefaultBackupStorageLocation: true,
	}

	bsl := buildBackupStorageLocation(cfg)

	assert.Equal(t, "default", bsl["name"])
	assert.Equal(t, "aws", bsl["provider"])
	assert.Equal(t, "velero", bsl["bucket"])
	assert.True(t, bsl["default"].(bool))

	config, ok := bsl["config"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "us-east-1", config["region"])
	assert.Equal(t, "http://minio:9000", config["s3Url"])
	assert.True(t, config["s3ForcePathStyle"].(bool))
	assert.Equal(t, "true", config["insecureSkipTLSVerify"])
}

func TestBuildBackupStorageLocation_AWS(t *testing.T) {
	cfg := &Config{
		Provider:                     ProviderAWS,
		S3Bucket:                    "my-backups",
		S3Region:                    "us-west-2",
		DefaultBackupStorageLocation: false,
	}

	bsl := buildBackupStorageLocation(cfg)

	assert.Equal(t, "default", bsl["name"])
	assert.Equal(t, "aws", bsl["provider"])
	assert.Equal(t, "my-backups", bsl["bucket"])
	assert.False(t, bsl["default"].(bool))

	config, ok := bsl["config"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "us-west-2", config["region"])
	// Should not have MinIO-specific settings
	assert.Nil(t, config["s3Url"])
}

func TestBuildVolumeSnapshotLocation(t *testing.T) {
	cfg := &Config{
		S3Region: "us-west-2",
	}

	vsl := buildVolumeSnapshotLocation(cfg)

	assert.Equal(t, "default", vsl["name"])
	assert.Equal(t, "aws", vsl["provider"])

	config, ok := vsl["config"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "us-west-2", config["region"])
}

func TestBuildCredentialsData(t *testing.T) {
	cfg := &Config{
		S3AccessKey: "AKIAIOSFODNN7EXAMPLE",
		S3SecretKey: "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY",
	}

	creds := buildCredentialsData(cfg)

	cloud, ok := creds["cloud"].(string)
	require.True(t, ok)
	assert.Contains(t, cloud, "aws_access_key_id=AKIAIOSFODNN7EXAMPLE")
	assert.Contains(t, cloud, "aws_secret_access_key=wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY")
	assert.Contains(t, cloud, "[default]")
}

func TestBuildSchedules_WithTTL(t *testing.T) {
	cfg := &Config{
		ScheduleName:        "daily-backup",
		ScheduleCron:        "0 2 * * *",
		BackupRetentionDays: 7,
	}

	schedules := buildSchedules(cfg)

	daily, ok := schedules["daily-backup"].(map[string]interface{})
	require.True(t, ok)

	template, ok := daily["template"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "168h", template["ttl"]) // 7 * 24 = 168
}

func TestBuildSchedules_NoTTL(t *testing.T) {
	cfg := &Config{
		ScheduleName:        "daily-backup",
		ScheduleCron:        "0 2 * * *",
		BackupRetentionDays: 0, // No TTL
	}

	schedules := buildSchedules(cfg)

	daily, ok := schedules["daily-backup"].(map[string]interface{})
	require.True(t, ok)

	template, ok := daily["template"].(map[string]interface{})
	require.True(t, ok)
	_, hasTTL := template["ttl"]
	assert.False(t, hasTTL)
}

func TestVerifyInstallation_Success(t *testing.T) {
	k8sClient := &mockK8sClient{
		pods: []*k8s.Pod{
			{Name: "velero-abc123", Namespace: "velero", Status: "Running"},
		},
	}

	ctx := context.Background()
	err := verifyInstallation(ctx, k8sClient, "velero")
	assert.NoError(t, err)
}

func TestVerifyInstallation_NilClient(t *testing.T) {
	ctx := context.Background()
	err := verifyInstallation(ctx, nil, "velero")
	assert.NoError(t, err) // Should skip verification
}

func TestVerifyInstallation_PodsNotReady(t *testing.T) {
	k8sClient := &mockK8sClient{
		pods: []*k8s.Pod{
			{Name: "velero-abc123", Namespace: "velero", Status: "Pending"},
		},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	err := verifyInstallation(ctx, k8sClient, "velero")
	assert.Error(t, err)
}

func TestVerifyInstallation_ContextCanceled(t *testing.T) {
	k8sClient := &mockK8sClient{
		pods: []*k8s.Pod{},
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	err := verifyInstallation(ctx, k8sClient, "velero")
	assert.Error(t, err)
	assert.Equal(t, context.Canceled, err)
}

func TestContainsSubstring(t *testing.T) {
	tests := []struct {
		s        string
		substr   string
		expected bool
	}{
		{"velero-abc123", "velero", true},
		{"velero-server-xyz", "velero", true},
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
