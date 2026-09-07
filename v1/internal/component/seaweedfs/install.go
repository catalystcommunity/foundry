package seaweedfs

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/catalystcommunity/foundry/v1/internal/helm"
	"github.com/catalystcommunity/foundry/v1/internal/k8s"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
)

const (
	seaweedfsRepoName = "seaweedfs"
	seaweedfsRepoURL  = "https://seaweedfs.github.io/seaweedfs/helm"
	seaweedfsChart    = "seaweedfs/seaweedfs"
	seaweedfsS3Secret = "foundry-seaweedfs-s3"
)

// Install installs SeaweedFS using Helm
func Install(ctx context.Context, helmClient HelmClient, k8sClient K8sClient, cfg *Config) error {
	if helmClient == nil {
		return fmt.Errorf("helm client cannot be nil")
	}
	if cfg == nil {
		cfg = DefaultConfig()
		cfg.AccessKey = generateRandomKey(16)
		cfg.SecretKey = generateRandomKey(32)
	}

	// Check for ServiceMonitor CRD if ServiceMonitor is enabled
	if cfg.ServiceMonitorEnabled {
		if k8sClient != nil {
			crdExists, err := k8sClient.ServiceMonitorCRDExists(ctx)
			if err != nil {
				return fmt.Errorf("failed to check for ServiceMonitor CRD: %w", err)
			}
			if !crdExists {
				return fmt.Errorf("ServiceMonitor CRD not found but service_monitor_enabled is true. " +
					"Either install Prometheus first (which includes the CRD), or set service_monitor_enabled: false " +
					"in your stack.yaml under components.seaweedfs")
			}
		}
	}

	fmt.Println("  Installing SeaweedFS...")

	// SeaweedFS must know the same S3 credentials that Foundry gives to
	// dependent components. Apply the Secret before Helm renders the workload.
	if cfg.S3Enabled {
		if k8sClient == nil {
			return fmt.Errorf("kubernetes client cannot be nil when S3 is enabled")
		}
		if err := k8sClient.CreateNamespace(ctx, cfg.Namespace); err != nil && !k8serrors.IsAlreadyExists(err) {
			return fmt.Errorf("failed to create namespace for S3 credentials: %w", err)
		}
		var existingS3Config []byte
		existingSecret, err := k8sClient.GetSecret(ctx, cfg.Namespace, seaweedfsS3Secret)
		if err == nil && existingSecret != nil {
			existingS3Config = existingSecret.Data["seaweedfs_s3_config"]
		} else if err != nil && !k8serrors.IsNotFound(err) {
			return fmt.Errorf("failed to read existing S3 credentials secret: %w", err)
		}
		secretManifest, err := buildS3SecretManifest(cfg, existingS3Config)
		if err != nil {
			return fmt.Errorf("failed to build S3 credentials secret: %w", err)
		}
		if err := k8sClient.ApplyManifest(ctx, secretManifest); err != nil {
			return fmt.Errorf("failed to apply S3 credentials secret: %w", err)
		}
	}

	// Check current state from Kubernetes
	var releaseExists bool
	var releaseStatus string
	releases, err := helmClient.List(ctx, cfg.Namespace)
	if err == nil {
		for _, rel := range releases {
			if rel.Name == "seaweedfs" {
				releaseExists = true
				releaseStatus = rel.Status
				break
			}
		}
	}

	// Step 1: Reconcile the Helm release.
	if err := helmClient.AddRepo(ctx, helm.RepoAddOptions{
		Name:        seaweedfsRepoName,
		URL:         seaweedfsRepoURL,
		ForceUpdate: true,
	}); err != nil {
		return fmt.Errorf("failed to add helm repository: %w", err)
	}

	values := buildHelmValues(cfg)
	if releaseExists {
		fmt.Printf("  Upgrading SeaweedFS (current status: %s)...\n", releaseStatus)
		if err := helmClient.Upgrade(ctx, helm.UpgradeOptions{
			ReleaseName: "seaweedfs",
			Namespace:   cfg.Namespace,
			Chart:       seaweedfsChart,
			Version:     cfg.Version,
			Values:      values,
			Wait:        true,
			Timeout:     10 * time.Minute,
		}); err != nil {
			fmt.Printf("  ⚠ Warning: Failed to upgrade release (status: %s): %v\n", releaseStatus, err)
			fmt.Println("  ⚠ Manual intervention required. You may need to:")
			fmt.Println("    1. Check pod status: kubectl get pods -n", cfg.Namespace)
			fmt.Println("    2. Check PVC status: kubectl get pvc -n", cfg.Namespace)
			fmt.Println("    3. If data loss is acceptable, uninstall manually: helm uninstall seaweedfs -n", cfg.Namespace)
			return fmt.Errorf("failed to upgrade seaweedfs (manual intervention required): %w", err)
		}
	} else {
		if err := helmClient.Install(ctx, helm.InstallOptions{
			ReleaseName:     "seaweedfs",
			Namespace:       cfg.Namespace,
			Chart:           seaweedfsChart,
			Version:         cfg.Version,
			Values:          values,
			CreateNamespace: true,
			Wait:            true,
			Timeout:         10 * time.Minute,
		}); err != nil {
			return fmt.Errorf("failed to install seaweedfs: %w", err)
		}
	}

	// Step 2: Verify pods are running.
	if k8sClient != nil {
		if err := verifyInstallation(ctx, k8sClient, cfg.Namespace); err != nil {
			return fmt.Errorf("installation verification failed: %w", err)
		}
	}

	// Step 3: Ensure configured buckets exist on every reconciliation.
	if len(cfg.Buckets) > 0 && k8sClient != nil {
		fmt.Printf("  Ensuring S3 buckets exist: %v\n", cfg.Buckets)
		if err := createBuckets(ctx, k8sClient, cfg); err != nil {
			return fmt.Errorf("failed to create buckets: %w", err)
		}
		fmt.Println("  S3 buckets are ready")
	}

	fmt.Println("  SeaweedFS installed successfully")
	fmt.Printf("  S3 Endpoint: %s\n", cfg.GetS3Endpoint())
	fmt.Printf("  Master Endpoint: %s\n", cfg.GetMasterEndpoint())
	return nil
}

// buildHelmValues constructs Helm values for SeaweedFS installation
func buildHelmValues(cfg *Config) map[string]interface{} {
	values := make(map[string]interface{})

	// Start with user-provided values
	for k, v := range cfg.Values {
		values[k] = v
	}

	// Master server configuration
	masterConfig := map[string]interface{}{
		"replicas": cfg.MasterReplicas,
		"port":     9333,
		"grpcPort": 19333,
		"persistence": map[string]interface{}{
			"enabled": true,
			"size":    "1Gi",
		},
		"resources": map[string]interface{}{
			"requests": map[string]interface{}{
				"memory": "128Mi",
				"cpu":    "50m",
			},
			"limits": map[string]interface{}{
				"memory": "512Mi",
			},
		},
	}
	if cfg.StorageClass != "" {
		masterConfig["persistence"].(map[string]interface{})["storageClass"] = cfg.StorageClass
	}
	values["master"] = masterConfig

	// Volume server configuration
	volumeConfig := map[string]interface{}{
		"replicas": cfg.VolumeReplicas,
		"port":     8080,
		"grpcPort": 18080,
		"dataDirs": []map[string]interface{}{
			{
				"name":       "data",
				"type":       "persistentVolumeClaim",
				"size":       cfg.StorageSize,
				"maxVolumes": 0,
			},
		},
		"resources": map[string]interface{}{
			"requests": map[string]interface{}{
				"memory": "256Mi",
				"cpu":    "100m",
			},
			"limits": map[string]interface{}{
				"memory": "1Gi",
			},
		},
	}
	if cfg.StorageClass != "" {
		volumeConfig["dataDirs"].([]map[string]interface{})[0]["storageClass"] = cfg.StorageClass
	}
	values["volume"] = volumeConfig

	// Filer configuration
	filerConfig := map[string]interface{}{
		"replicas": cfg.FilerReplicas,
		"port":     8888,
		"grpcPort": 18888,
		"persistence": map[string]interface{}{
			"enabled": true,
			"size":    "5Gi",
		},
		"resources": map[string]interface{}{
			"requests": map[string]interface{}{
				"memory": "128Mi",
				"cpu":    "50m",
			},
			"limits": map[string]interface{}{
				"memory": "512Mi",
			},
		},
	}
	if cfg.StorageClass != "" {
		filerConfig["persistence"].(map[string]interface{})["storageClass"] = cfg.StorageClass
	}
	values["filer"] = filerConfig

	// S3 gateway configuration
	s3Config := map[string]interface{}{
		"enabled":    cfg.S3Enabled,
		"port":       cfg.S3Port,
		"enableAuth": cfg.S3Enabled,
		"resources": map[string]interface{}{
			"requests": map[string]interface{}{
				"memory": "128Mi",
				"cpu":    "50m",
			},
			"limits": map[string]interface{}{
				"memory": "512Mi",
			},
		},
	}
	if cfg.S3Enabled {
		s3Config["existingConfigSecret"] = seaweedfsS3Secret
	}

	// S3 ingress configuration
	if cfg.IngressEnabled && cfg.IngressHostS3 != "" {
		s3Config["ingress"] = map[string]interface{}{
			"enabled":   true,
			"className": "contour",
			"host":      cfg.IngressHostS3,
			"annotations": map[string]interface{}{
				"cert-manager.io/cluster-issuer": "foundry-ca-issuer",
			},
			"tls": []map[string]interface{}{
				{
					"hosts":      []string{cfg.IngressHostS3},
					"secretName": "seaweedfs-s3-tls",
				},
			},
		}
	}
	values["s3"] = s3Config

	// Filer ingress configuration
	if cfg.IngressEnabled && cfg.IngressHostFiler != "" {
		filerConfig["ingress"] = map[string]interface{}{
			"enabled":   true,
			"className": "contour",
			"host":      cfg.IngressHostFiler,
			"annotations": map[string]interface{}{
				"cert-manager.io/cluster-issuer": "foundry-ca-issuer",
			},
			"tls": []map[string]interface{}{
				{
					"hosts":      []string{cfg.IngressHostFiler},
					"secretName": "seaweedfs-filer-tls",
				},
			},
		}
	}

	// Enable global monitoring with ServiceMonitors for Prometheus (if configured)
	if cfg.ServiceMonitorEnabled {
		values["global"] = map[string]interface{}{
			"monitoring": map[string]interface{}{
				"enabled": true,
			},
		}
	}

	return values
}

// buildS3SecretManifest returns a Kubernetes Secret with the S3 identity and
// the individual keys that the bucket setup Job uses.
func buildS3SecretManifest(cfg *Config, existingConfig []byte) (string, error) {
	if cfg.AccessKey == "" || cfg.SecretKey == "" {
		return "", fmt.Errorf("access_key and secret_key are required when S3 is enabled")
	}

	s3Config, err := mergeS3IdentityConfig(existingConfig, cfg.AccessKey, cfg.SecretKey)
	if err != nil {
		return "", err
	}

	manifest, err := json.Marshal(map[string]interface{}{
		"apiVersion": "v1",
		"kind":       "Secret",
		"metadata": map[string]interface{}{
			"name":      seaweedfsS3Secret,
			"namespace": cfg.Namespace,
			"labels": map[string]string{
				"app.kubernetes.io/managed-by": "foundry",
				"app.kubernetes.io/name":       "seaweedfs",
			},
		},
		"type": "Opaque",
		"data": map[string]string{
			"access_key":          base64.StdEncoding.EncodeToString([]byte(cfg.AccessKey)),
			"secret_key":          base64.StdEncoding.EncodeToString([]byte(cfg.SecretKey)),
			"seaweedfs_s3_config": base64.StdEncoding.EncodeToString(s3Config),
		},
	})
	if err != nil {
		return "", err
	}
	return string(manifest), nil
}

func mergeS3IdentityConfig(existingConfig []byte, accessKey, secretKey string) ([]byte, error) {
	config := make(map[string]interface{})
	if len(existingConfig) > 0 {
		if err := json.Unmarshal(existingConfig, &config); err != nil {
			return nil, fmt.Errorf("existing SeaweedFS S3 configuration is invalid: %w", err)
		}
	}

	identities := make([]interface{}, 0)
	if existingIdentities, ok := config["identities"]; ok {
		items, ok := existingIdentities.([]interface{})
		if !ok {
			return nil, fmt.Errorf("existing SeaweedFS S3 identities are invalid")
		}
		for _, item := range items {
			identity, ok := item.(map[string]interface{})
			if !ok {
				return nil, fmt.Errorf("existing SeaweedFS S3 identity is invalid")
			}
			name, ok := identity["name"].(string)
			if !ok || name == "" {
				return nil, fmt.Errorf("existing SeaweedFS S3 identity name is invalid")
			}
			if name != "foundry-admin" {
				identities = append(identities, identity)
			}
		}
	}

	identities = append(identities, map[string]interface{}{
		"name": "foundry-admin",
		"credentials": []interface{}{
			map[string]interface{}{
				"accessKey": accessKey,
				"secretKey": secretKey,
			},
		},
		"actions": []string{"Admin", "Read", "Write"},
	})
	config["identities"] = identities

	encoded, err := json.Marshal(config)
	if err != nil {
		return nil, fmt.Errorf("failed to encode SeaweedFS S3 configuration: %w", err)
	}
	return encoded, nil
}

// verifyInstallation verifies that SeaweedFS pods are running
func verifyInstallation(ctx context.Context, k8sClient K8sClient, namespace string) error {
	if k8sClient == nil {
		return nil // Skip verification if no k8s client
	}

	// Wait for pods to be ready (up to 2 minutes).
	timeout := time.After(2 * time.Minute)
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		pods, err := k8sClient.GetPods(ctx, namespace)
		if err == nil && seaweedFSPodsRunning(pods) {
			return nil
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-timeout:
			return fmt.Errorf("timeout waiting for seaweedfs pods to be ready")
		case <-ticker.C:
		}
	}
}

func seaweedFSPodsRunning(pods []*k8s.Pod) bool {
	seaweedfsPodFound := false
	for _, pod := range pods {
		if pod.Name == "" || !containsSubstring(pod.Name, "seaweedfs") {
			continue
		}
		seaweedfsPodFound = true
		if pod.Status != "Running" {
			return false
		}
	}
	return seaweedfsPodFound
}

// containsSubstring checks if s contains substr
func containsSubstring(s, substr string) bool {
	if len(substr) > len(s) {
		return false
	}
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

// createBuckets creates S3 buckets using a Kubernetes Job
func createBuckets(ctx context.Context, k8sClient K8sClient, cfg *Config) error {
	if len(cfg.Buckets) == 0 {
		return nil
	}

	// Build the command to create all buckets
	s3Endpoint := fmt.Sprintf("http://seaweedfs-s3.%s.svc.cluster.local:%d", cfg.Namespace, cfg.S3Port)

	// A missing bucket is created. Any other command failure fails the Job.
	bucketCommands := make([]string, 0, len(cfg.Buckets))
	for _, bucket := range cfg.Buckets {
		bucketCommands = append(bucketCommands, fmt.Sprintf(
			"aws s3api head-bucket --bucket %s --endpoint-url %s >/dev/null 2>&1 || aws s3api create-bucket --bucket %s --endpoint-url %s",
			shellQuote(bucket), shellQuote(s3Endpoint), shellQuote(bucket), shellQuote(s3Endpoint),
		))
	}

	jobName := "seaweedfs-bucket-setup"
	job := map[string]interface{}{
		"apiVersion": "batch/v1",
		"kind":       "Job",
		"metadata": map[string]interface{}{
			"name":      jobName,
			"namespace": cfg.Namespace,
		},
		"spec": map[string]interface{}{
			"ttlSecondsAfterFinished": 60,
			"backoffLimit":            3,
			"template": map[string]interface{}{
				"spec": map[string]interface{}{
					"restartPolicy": "Never",
					"containers": []interface{}{
						map[string]interface{}{
							"name":    "bucket-setup",
							"image":   "amazon/aws-cli:2.15.0",
							"command": []string{"/bin/sh", "-ec", strings.Join(bucketCommands, "\n")},
							"env": []interface{}{
								secretEnvironmentVariable("AWS_ACCESS_KEY_ID", "access_key"),
								secretEnvironmentVariable("AWS_SECRET_ACCESS_KEY", "secret_key"),
								map[string]interface{}{"name": "AWS_DEFAULT_REGION", "value": "us-east-1"},
							},
						},
					},
				},
			},
		},
	}
	jobManifest, err := json.Marshal(job)
	if err != nil {
		return fmt.Errorf("failed to build bucket setup job: %w", err)
	}

	// Delete any existing job with the same name
	_ = k8sClient.DeleteJob(ctx, cfg.Namespace, jobName)
	time.Sleep(2 * time.Second) // Give time for cleanup

	// Create the job
	if err := k8sClient.ApplyManifest(ctx, string(jobManifest)); err != nil {
		return fmt.Errorf("failed to create bucket setup job: %w", err)
	}

	// Wait for job to complete
	if err := k8sClient.WaitForJobComplete(ctx, cfg.Namespace, jobName, 2*time.Minute); err != nil {
		return fmt.Errorf("bucket setup job failed: %w", err)
	}

	// Cleanup the job
	_ = k8sClient.DeleteJob(ctx, cfg.Namespace, jobName)

	return nil
}

func secretEnvironmentVariable(name, key string) map[string]interface{} {
	return map[string]interface{}{
		"name": name,
		"valueFrom": map[string]interface{}{
			"secretKeyRef": map[string]interface{}{
				"name": seaweedfsS3Secret,
				"key":  key,
			},
		},
	}
}

func shellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\"'\"'") + "'"
}
