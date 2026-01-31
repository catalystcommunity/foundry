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

	// Check for ServiceMonitor CRD if ServiceMonitor is enabled
	if cfg.ServiceMonitorEnabled {
		crdExists, err := k8sClient.ServiceMonitorCRDExists(ctx)
		if err != nil {
			return fmt.Errorf("failed to check for ServiceMonitor CRD: %w", err)
		}
		if !crdExists {
			return fmt.Errorf("ServiceMonitor CRD not found but service_monitor_enabled is true. " +
				"Either install Prometheus first (which includes the CRD), or set service_monitor_enabled: false " +
				"in your stack.yaml under components.contour")
		}
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
	var existingRelease *helm.Release
	releases, err := helmClient.List(ctx, cfg.Namespace)
	if err == nil {
		for i, rel := range releases {
			if rel.Name == "contour" {
				existingRelease = &releases[i]
				break
			}
		}
	}

	if existingRelease != nil {
		if existingRelease.Status == "deployed" {
			// Already deployed - upgrade to ensure latest configuration
			// This is important for applying Gateway API configuration changes
			fmt.Println("  Upgrading existing Contour deployment...")
			if err := helmClient.Upgrade(ctx, helm.UpgradeOptions{
				ReleaseName: "contour",
				Namespace:   cfg.Namespace,
				Chart:       contourChart,
				Version:     cfg.Version,
				Values:      values,
				Wait:        true,
				Timeout:     5 * time.Minute,
			}); err != nil {
				return fmt.Errorf("failed to upgrade contour: %w", err)
			}
		} else {
			// Release exists but not deployed (failed, pending, etc.)
			// Uninstall the failed release so we can reinstall cleanly
			fmt.Printf("  Removing failed release (status: %s)...\n", existingRelease.Status)
			if err := helmClient.Uninstall(ctx, helm.UninstallOptions{
				ReleaseName: "contour",
				Namespace:   cfg.Namespace,
			}); err != nil {
				return fmt.Errorf("failed to uninstall existing release: %w", err)
			}
			fmt.Println("  ✓ Failed release removed")

			// Fresh install after removing failed release
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
		}
	} else {
		// Fresh install
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
	}

	// If Gateway API is enabled, patch the deployment to use ContourConfiguration mode
	// The Helm chart adds --config-path by default which conflicts with --contour-config-name
	if cfg.CreateGateway {
		fmt.Println("  Configuring Contour for Gateway API support...")
		if err := patchContourForGatewayAPI(ctx, k8sClient, cfg.Namespace); err != nil {
			// Log warning but don't fail - the pod might already be configured correctly
			fmt.Printf("  ⚠ Could not patch Contour deployment: %v\n", err)
		}
	}

	// Verify installation by checking for running pods
	if err := verifyInstallation(ctx, k8sClient, cfg.Namespace); err != nil {
		return fmt.Errorf("installation verification failed: %w", err)
	}

	// Create Gateway API resources if configured (idempotent - safe to run on existing installs)
	if cfg.CreateGateway {
		if err := createGatewayResources(ctx, k8sClient, cfg); err != nil {
			return fmt.Errorf("failed to create Gateway API resources: %w", err)
		}
	}

	return nil
}

// patchContourForGatewayAPI patches the Contour deployment to use ContourConfiguration CRD
// instead of the ConfigMap-based configuration. This is necessary because:
// 1. The Helm chart adds --config-path by default
// 2. We need --contour-config-name to use ContourConfiguration for Gateway API
// 3. These two flags conflict - Contour rejects having both
func patchContourForGatewayAPI(ctx context.Context, k8sClient K8sClient, namespace string) error {
	// Replace --config-path with --contour-config-name
	// The oldArg prefix matches "--config-path=/config/contour.yaml" or similar
	if err := k8sClient.PatchDeploymentArgs(ctx, namespace, "contour-contour",
		"--config-path", "--contour-config-name=contour"); err != nil {
		return fmt.Errorf("failed to patch deployment args: %w", err)
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
	}

	// Note: Gateway API configuration is handled via ContourConfiguration CRD
	// The deployment is patched after Helm install to use --contour-config-name
	// instead of --config-path (see patchContourForGatewayAPI)
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
		vip := GetClusterVIP()
		if vip != "" {
			// When sharing the control plane VIP, use externalIPs instead of LoadBalancer
			// with kube-vip annotation. This avoids kube-vip creating a separate leader
			// election for the service, which causes split-brain where both the CP leader
			// and service leader advertise the same IP on different nodes.
			// See: https://github.com/kube-vip/kube-vip/issues/665
			//
			// With externalIPs, Kubernetes routes traffic to the VIP (ports 80/443) directly
			// to envoy pods via kube-proxy, while kube-vip manages only the CP VIP (port 6443).
			envoyConfig["service"] = map[string]interface{}{
				"type":        "ClusterIP",
				"externalIPs": []string{vip},
			}
		}
		// If no VIP is set, keep default LoadBalancer type for kube-vip to auto-assign
	}
	values["envoy"] = envoyConfig

	// Gateway API CRDs are managed by our gateway-api component, not the Contour chart
	// The official chart doesn't install CRDs by default anyway

	// Enable metrics for Prometheus
	metricsConfig := map[string]interface{}{
		"contour": map[string]interface{}{
			"enabled": true,
		},
		"envoy": map[string]interface{}{
			"enabled": true,
		},
	}

	// Only enable ServiceMonitor if configured (requires CRD from Prometheus Operator)
	if cfg.ServiceMonitorEnabled {
		metricsConfig["serviceMonitor"] = map[string]interface{}{
			"enabled": true,
		}
	}
	values["metrics"] = metricsConfig

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

// createGatewayResources creates GatewayClass, ContourConfiguration, and Gateway resources for Contour
func createGatewayResources(ctx context.Context, k8sClient K8sClient, cfg *Config) error {
	// Create GatewayClass
	gatewayClassManifest := generateGatewayClassManifest()
	if err := k8sClient.ApplyManifest(ctx, gatewayClassManifest); err != nil {
		return fmt.Errorf("failed to create GatewayClass: %w", err)
	}
	fmt.Println("  ✓ GatewayClass 'contour' created")

	// Create ContourConfiguration CRD to configure Gateway API support
	contourConfigManifest := generateContourConfigurationManifest(cfg.Namespace)
	if err := k8sClient.ApplyManifest(ctx, contourConfigManifest); err != nil {
		return fmt.Errorf("failed to create ContourConfiguration: %w", err)
	}
	fmt.Println("  ✓ ContourConfiguration 'contour' created")

	// Check if cert-manager is installed before trying to create Certificate
	domain := GetGatewayDomain()
	certManagerInstalled := false
	if cfg.CreateGatewayCertificate && domain != "" {
		var err error
		certManagerInstalled, err = k8sClient.CRDExists(ctx, "certificates.cert-manager.io")
		if err != nil {
			// Log warning but continue - we'll just skip TLS
			fmt.Printf("  ⚠ Could not check for cert-manager CRD: %v\n", err)
		}
	}

	// Create TLS Certificate if cert-manager is available
	createTLS := cfg.CreateGatewayCertificate && domain != "" && certManagerInstalled
	if createTLS {
		certManifest := generateGatewayCertificateManifest(cfg.Namespace, domain)
		if err := k8sClient.ApplyManifest(ctx, certManifest); err != nil {
			return fmt.Errorf("failed to create Gateway TLS Certificate: %w", err)
		}
		fmt.Printf("  ✓ Gateway TLS Certificate for '*.%s' created\n", domain)
	} else if cfg.CreateGatewayCertificate && domain != "" && !certManagerInstalled {
		fmt.Println("  ℹ Skipping TLS Certificate (cert-manager not yet installed)")
		fmt.Println("    Re-run 'foundry stack install' after cert-manager is installed to add HTTPS support")
	}

	// Create Gateway (with HTTPS only if certificate was created)
	gatewayManifest := generateGatewayManifest(cfg.Namespace, domain, createTLS)
	if err := k8sClient.ApplyManifest(ctx, gatewayManifest); err != nil {
		return fmt.Errorf("failed to create Gateway: %w", err)
	}
	fmt.Println("  ✓ Gateway 'contour' created")

	return nil
}

// generateGatewayClassManifest generates the GatewayClass manifest for Contour
func generateGatewayClassManifest() string {
	return `apiVersion: gateway.networking.k8s.io/v1
kind: GatewayClass
metadata:
  name: contour
spec:
  controllerName: projectcontour.io/gateway-controller
`
}

// generateContourConfigurationManifest generates a ContourConfiguration CRD
// to configure Contour for Gateway API support (static provisioning)
func generateContourConfigurationManifest(namespace string) string {
	return fmt.Sprintf(`apiVersion: projectcontour.io/v1alpha1
kind: ContourConfiguration
metadata:
  name: contour
  namespace: %s
spec:
  gateway:
    gatewayRef:
      name: contour
      namespace: %s
`, namespace, namespace)
}

// generateGatewayCertificateManifest generates a Certificate resource for the Gateway's wildcard TLS
func generateGatewayCertificateManifest(namespace, domain string) string {
	return fmt.Sprintf(`apiVersion: cert-manager.io/v1
kind: Certificate
metadata:
  name: gateway-wildcard-tls
  namespace: %s
spec:
  secretName: gateway-wildcard-tls
  dnsNames:
    - "*.%s"
    - "%s"
  issuerRef:
    name: foundry-ca-issuer
    kind: ClusterIssuer
    group: cert-manager.io
  duration: 8760h
  renewBefore: 720h
`, namespace, domain, domain)
}

// generateGatewayManifest generates the Gateway manifest with HTTP and HTTPS listeners
func generateGatewayManifest(namespace, domain string, withTLS bool) string {
	// Base Gateway with HTTP listener
	manifest := fmt.Sprintf(`apiVersion: gateway.networking.k8s.io/v1
kind: Gateway
metadata:
  name: contour
  namespace: %s
spec:
  gatewayClassName: contour
  listeners:
  - name: http
    port: 80
    protocol: HTTP
    allowedRoutes:
      namespaces:
        from: All
`, namespace)

	// Add HTTPS listener if TLS is configured
	if withTLS && domain != "" {
		manifest += fmt.Sprintf(`  - name: https
    port: 443
    protocol: HTTPS
    hostname: "*.%s"
    tls:
      mode: Terminate
      certificateRefs:
      - name: gateway-wildcard-tls
    allowedRoutes:
      namespaces:
        from: All
`, domain)
	}

	return manifest
}
