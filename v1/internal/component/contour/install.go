package contour

import (
	"context"
	"fmt"
	"time"

	"github.com/catalystcommunity/foundry/v1/internal/helm"
)

const (
	contourRepoName = "projectcontour"
	contourRepoURL  = "https://projectcontour.github.io/helm-charts/"
	contourChart    = "projectcontour/contour"
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

	// Add Project Contour Helm repository
	if err := helmClient.AddRepo(ctx, helm.RepoAddOptions{
		Name:        contourRepoName,
		URL:         contourRepoURL,
		ForceUpdate: true,
	}); err != nil {
		return fmt.Errorf("failed to add helm repository: %w", err)
	}

	// Build Helm values
	values := buildHelmValues(cfg)

	// Check if release already exists
	releases, err := helmClient.List(ctx, cfg.Namespace)
	if err == nil {
		for _, rel := range releases {
			if rel.Name == "contour" {
				// Release exists - check status
				if rel.Status == "deployed" {
					// Already deployed successfully, just verify pods are running
					return verifyInstallation(ctx, k8sClient, cfg.Namespace)
				}
				// Release exists but not deployed (failed, pending, etc.)
				// Uninstall the failed release so we can reinstall cleanly
				fmt.Printf("  Removing failed release (status: %s)...\n", rel.Status)
				if err := helmClient.Uninstall(ctx, helm.UninstallOptions{
					ReleaseName: "contour",
					Namespace:   cfg.Namespace,
				}); err != nil {
					return fmt.Errorf("failed to uninstall existing release: %w", err)
				}
				fmt.Println("  ✓ Failed release removed")
				break // Continue with fresh install
			}
		}
	}

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

// buildHelmValues constructs the Helm values for official Contour chart installation
// See: https://projectcontour.github.io/helm-charts/
func buildHelmValues(cfg *Config) map[string]interface{} {
	values := make(map[string]interface{})

	// Start with user-provided values
	for k, v := range cfg.Values {
		values[k] = v
	}

	// Configure Contour
	contourConfig := map[string]interface{}{
		"replicas": cfg.ReplicaCount,
		// IngressClass configuration
		"ingressClass": map[string]interface{}{
			"create":  true,
			"default": cfg.DefaultIngressClass,
		},
		// Gateway API controller configuration
		"configInline": map[string]interface{}{
			"gateway": map[string]interface{}{
				"controllerName": "projectcontour.io/gateway-controller",
			},
		},
	}
	values["contour"] = contourConfig

	// Configure Envoy
	envoyConfig := map[string]interface{}{
		"replicas": cfg.EnvoyReplicaCount,
		"service": map[string]interface{}{
			"type": "LoadBalancer",
		},
	}

	// Configure for bare metal with kube-vip
	if cfg.UseKubeVIP {
		serviceConfig := map[string]interface{}{
			"type": "LoadBalancer",
		}

		// If we have a cluster VIP, set it explicitly to share with K3s API
		// (K3s API uses port 6443, Contour uses 80/443 - no conflict)
		// If no VIP is set, don't add annotation - let kube-vip-cloud-provider auto-assign
		if vip := GetClusterVIP(); vip != "" {
			serviceConfig["annotations"] = map[string]interface{}{
				"kube-vip.io/loadbalancerIPs": vip,
			}
		}
		// Note: "auto" is NOT a valid value for kube-vip - it tries to resolve it as a hostname

		envoyConfig["service"] = serviceConfig
	}
	values["envoy"] = envoyConfig

	// Gateway API CRDs are managed by our gateway-api component, not the Contour chart
	// The official chart doesn't install CRDs by default anyway

	// Enable metrics and ServiceMonitor for Prometheus
	values["metrics"] = map[string]interface{}{
		"contour": map[string]interface{}{
			"enabled": true,
		},
		"envoy": map[string]interface{}{
			"enabled": true,
		},
		"serviceMonitor": map[string]interface{}{
			"enabled": true,
		},
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
