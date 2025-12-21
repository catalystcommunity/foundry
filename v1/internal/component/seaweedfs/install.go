package seaweedfs

import (
	"context"
	"fmt"
	"time"

	"github.com/catalystcommunity/foundry/v1/internal/helm"
)

const (
	seaweedfsRepoName = "seaweedfs"
	seaweedfsRepoURL  = "https://seaweedfs.github.io/seaweedfs/helm"
	seaweedfsChart    = "seaweedfs/seaweedfs"
)

// Install installs SeaweedFS using Helm
func Install(ctx context.Context, helmClient HelmClient, k8sClient K8sClient, cfg *Config) error {
	if helmClient == nil {
		return fmt.Errorf("helm client cannot be nil")
	}
	if cfg == nil {
		cfg = DefaultConfig()
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

	// Step 1: Handle Helm release
	if releaseExists && releaseStatus == "deployed" {
		// Already deployed and healthy - skip helm operations
		fmt.Println("  ✓ SeaweedFS helm release already deployed")
	} else if releaseExists {
		// Exists but not healthy - try to upgrade/repair
		fmt.Printf("  Upgrading SeaweedFS (current status: %s)...\n", releaseStatus)

		// Add Helm repository
		if err := helmClient.AddRepo(ctx, helm.RepoAddOptions{
			Name:        seaweedfsRepoName,
			URL:         seaweedfsRepoURL,
			ForceUpdate: true,
		}); err != nil {
			return fmt.Errorf("failed to add helm repository: %w", err)
		}

		values := buildHelmValues(cfg)
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
		// Not installed - do fresh install
		// Add Helm repository
		if err := helmClient.AddRepo(ctx, helm.RepoAddOptions{
			Name:        seaweedfsRepoName,
			URL:         seaweedfsRepoURL,
			ForceUpdate: true,
		}); err != nil {
			return fmt.Errorf("failed to add helm repository: %w", err)
		}

		values := buildHelmValues(cfg)
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

	// Step 2: Verify pods are running (only if we did install/upgrade)
	if k8sClient != nil && !(releaseExists && releaseStatus == "deployed") {
		if err := verifyInstallation(ctx, k8sClient, cfg.Namespace); err != nil {
			return fmt.Errorf("installation verification failed: %w", err)
		}
	}

	// Step 3: Create buckets if needed
	// Skip if release was already deployed (buckets were created on first install)
	if len(cfg.Buckets) > 0 && k8sClient != nil {
		if releaseExists && releaseStatus == "deployed" {
			fmt.Println("  ✓ S3 buckets already configured (skipping)")
		} else {
			fmt.Printf("  Creating S3 buckets: %v\n", cfg.Buckets)
			if err := createBuckets(ctx, k8sClient, cfg); err != nil {
				return fmt.Errorf("failed to create buckets: %w", err)
			}
			fmt.Println("  S3 buckets created successfully")
		}
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
		"enableAuth": false, // Auth configured via secret if needed
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

	// S3 ingress configuration
	if cfg.IngressEnabled && cfg.IngressHostS3 != "" {
		s3Config["ingress"] = map[string]interface{}{
			"enabled":          true,
			"className":        "contour",
			"host":             cfg.IngressHostS3,
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
			"enabled":          true,
			"className":        "contour",
			"host":             cfg.IngressHostFiler,
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

// verifyInstallation verifies that SeaweedFS pods are running
func verifyInstallation(ctx context.Context, k8sClient K8sClient, namespace string) error {
	if k8sClient == nil {
		return nil // Skip verification if no k8s client
	}

	// Wait for pods to be ready (up to 2 minutes)
	timeout := time.After(2 * time.Minute)
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-timeout:
			return fmt.Errorf("timeout waiting for seaweedfs pods to be ready")
		case <-ticker.C:
			pods, err := k8sClient.GetPods(ctx, namespace)
			if err != nil {
				continue // Retry on error
			}

			if len(pods) == 0 {
				continue // Wait for pods to appear
			}

			// Check if all pods are running
			allRunning := true
			seaweedfsPodFound := false
			for _, pod := range pods {
				if pod.Name == "" {
					continue
				}
				// Check for seaweedfs pods specifically
				if containsSubstring(pod.Name, "seaweedfs") {
					seaweedfsPodFound = true
					if pod.Status != "Running" {
						allRunning = false
						break
					}
				}
			}

			if seaweedfsPodFound && allRunning {
				return nil
			}
		}
	}
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

	// Create a shell command that creates all buckets (semicolon separated)
	bucketCommands := ""
	for i, bucket := range cfg.Buckets {
		if i > 0 {
			bucketCommands += " && "
		}
		bucketCommands += fmt.Sprintf("aws s3 mb s3://%s --endpoint-url %s || true", bucket, s3Endpoint)
	}

	jobName := "seaweedfs-bucket-setup"
	jobManifest := fmt.Sprintf(`apiVersion: batch/v1
kind: Job
metadata:
  name: %s
  namespace: %s
spec:
  ttlSecondsAfterFinished: 60
  backoffLimit: 3
  template:
    spec:
      restartPolicy: Never
      containers:
      - name: bucket-setup
        image: amazon/aws-cli:2.15.0
        env:
        - name: AWS_ACCESS_KEY_ID
          value: "%s"
        - name: AWS_SECRET_ACCESS_KEY
          value: "%s"
        command: ["/bin/sh", "-c", "%s && echo done"]
`, jobName, cfg.Namespace, cfg.AccessKey, cfg.SecretKey, bucketCommands)

	// Delete any existing job with the same name
	_ = k8sClient.DeleteJob(ctx, cfg.Namespace, jobName)
	time.Sleep(2 * time.Second) // Give time for cleanup

	// Create the job
	if err := k8sClient.ApplyManifest(ctx, jobManifest); err != nil {
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
