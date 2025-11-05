package contour

import (
	"context"
	"fmt"
	"time"

	"github.com/catalystcommunity/foundry/v1/internal/helm"
)

const (
	contourRepoName = "bitnami"
	contourRepoURL  = "https://charts.bitnami.com/bitnami"
	contourChart    = "bitnami/contour"
)

// Install installs the Contour ingress controller using Helm
func Install(ctx context.Context, helmClient HelmClient, k8sClient K8sClient, cfg *Config) error {
	if helmClient == nil {
		return fmt.Errorf("helm client cannot be nil")
	}
	if k8sClient == nil {
		return fmt.Errorf("k8s client cannot be nil")
	}
	if cfg == nil {
		cfg = DefaultConfig()
	}

	// Add Bitnami Helm repository
	if err := helmClient.AddRepo(ctx, helm.RepoAddOptions{
		Name:        contourRepoName,
		URL:         contourRepoURL,
		ForceUpdate: true,
	}); err != nil {
		return fmt.Errorf("failed to add helm repository: %w", err)
	}

	// Build Helm values
	values := buildHelmValues(cfg)

	// Install Contour via Helm
	if err := helmClient.Install(ctx, helm.InstallOptions{
		ReleaseName:     "contour",
		Namespace:       cfg.Namespace,
		Chart:           contourChart,
		Version:         cfg.Version,
		Values:          values,
		CreateNamespace: true,
		Wait:            true,
		Timeout:         5 * time.Minute,
	}); err != nil {
		return fmt.Errorf("failed to install contour: %w", err)
	}

	// Verify installation by checking for running pods
	if err := verifyInstallation(ctx, k8sClient, cfg.Namespace); err != nil {
		return fmt.Errorf("installation verification failed: %w", err)
	}

	return nil
}

// buildHelmValues constructs the Helm values for Contour installation
func buildHelmValues(cfg *Config) map[string]interface{} {
	values := make(map[string]interface{})

	// Start with user-provided values
	for k, v := range cfg.Values {
		values[k] = v
	}

	// Set Contour replicas
	if cfg.ReplicaCount > 0 {
		values["contour"] = map[string]interface{}{
			"replicaCount": cfg.ReplicaCount,
		}
	}

	// Set Envoy replicas
	if cfg.EnvoyReplicaCount > 0 {
		values["envoy"] = map[string]interface{}{
			"replicaCount": cfg.EnvoyReplicaCount,
			"service": map[string]interface{}{
				"type": "LoadBalancer",
			},
		}
	}

	// Configure for bare metal with kube-vip
	if cfg.UseKubeVIP {
		// kube-vip cloud provider will automatically assign LoadBalancer IPs
		if envoy, ok := values["envoy"].(map[string]interface{}); ok {
			if service, ok := envoy["service"].(map[string]interface{}); ok {
				service["annotations"] = map[string]interface{}{
					"kube-vip.io/loadbalancerIPs": "auto",
				}
			}
		} else {
			values["envoy"] = map[string]interface{}{
				"service": map[string]interface{}{
					"type": "LoadBalancer",
					"annotations": map[string]interface{}{
						"kube-vip.io/loadbalancerIPs": "auto",
					},
				},
			}
		}
	}

	// Set as default IngressClass
	if cfg.DefaultIngressClass {
		values["ingressClass"] = map[string]interface{}{
			"default": true,
		}
	}

	return values
}

// verifyInstallation verifies that Contour pods are running
func verifyInstallation(ctx context.Context, k8sClient K8sClient, namespace string) error {
	// Wait for pods to be ready (up to 2 minutes)
	timeout := time.After(2 * time.Minute)
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-timeout:
			return fmt.Errorf("timeout waiting for contour pods to be ready")
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
			for _, pod := range pods {
				if pod.Status != "Running" {
					allRunning = false
					break
				}
			}

			if allRunning {
				return nil
			}
		}
	}
}
