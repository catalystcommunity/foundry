package certmanager

import (
	"context"
	"fmt"
	"time"

	"github.com/catalystcommunity/foundry/v1/internal/component"
	"github.com/catalystcommunity/foundry/v1/internal/helm"
	"github.com/catalystcommunity/foundry/v1/internal/k8s"
)

const (
	// DefaultRepoURL is the Jetstack Helm repository URL
	DefaultRepoURL = "https://charts.jetstack.io"
	// DefaultRepoName is the name for the Jetstack repo
	DefaultRepoName = "jetstack"
	// DefaultChartName is the cert-manager chart name
	DefaultChartName = "jetstack/cert-manager"
	// DefaultReleaseName is the default Helm release name
	DefaultReleaseName = "cert-manager"
)

// HelmClient interface for Helm operations (for testing)
type HelmClient interface {
	AddRepo(ctx context.Context, opts helm.RepoAddOptions) error
	Install(ctx context.Context, opts helm.InstallOptions) error
	Upgrade(ctx context.Context, opts helm.UpgradeOptions) error
	List(ctx context.Context, namespace string) ([]helm.Release, error)
	Uninstall(ctx context.Context, opts helm.UninstallOptions) error
}

// K8sClient interface for Kubernetes operations (for testing)
type K8sClient interface {
	GetPods(ctx context.Context, namespace string) ([]*k8s.Pod, error)
	GetNamespace(ctx context.Context, name string) (*k8s.Namespace, error)
	CreateNamespace(ctx context.Context, name string) error
	ApplyManifest(ctx context.Context, manifest string) error
}

// Install installs cert-manager via Helm
func Install(ctx context.Context, cfg *Config, componentCfg component.ComponentConfig) error {
	// Get Helm and K8s clients from component config
	helmClient, ok := componentCfg["helm_client"].(HelmClient)
	if !ok || helmClient == nil {
		return fmt.Errorf("helm_client not provided in component config")
	}

	k8sClient, ok := componentCfg["k8s_client"].(K8sClient)
	if !ok || k8sClient == nil {
		return fmt.Errorf("k8s_client not provided in component config")
	}

	// Ensure namespace exists
	if err := ensureNamespace(ctx, k8sClient, cfg.Namespace); err != nil {
		return fmt.Errorf("failed to ensure namespace: %w", err)
	}

	// Add Jetstack Helm repository
	if err := helmClient.AddRepo(ctx, helm.RepoAddOptions{
		Name: DefaultRepoName,
		URL:  DefaultRepoURL,
	}); err != nil {
		return fmt.Errorf("failed to add Jetstack Helm repo: %w", err)
	}

	// Prepare Helm values
	values := map[string]interface{}{
		"installCRDs": cfg.InstallCRDs,
		"global": map[string]interface{}{
			"leaderElection": map[string]interface{}{
				"namespace": cfg.Namespace,
			},
		},
		// Enable ServiceMonitor for Prometheus
		"prometheus": map[string]interface{}{
			"servicemonitor": map[string]interface{}{
				"enabled": true,
			},
		},
	}

	// Check if release already exists
	var releaseExists bool
	var releaseStatus string
	releases, err := helmClient.List(ctx, cfg.Namespace)
	if err == nil {
		for _, rel := range releases {
			if rel.Name == DefaultReleaseName {
				releaseExists = true
				releaseStatus = rel.Status
				break
			}
		}
	}

	if releaseExists {
		// Try to upgrade existing release (even if failed - avoid data loss)
		fmt.Printf("  Upgrading cert-manager (current status: %s)...\n", releaseStatus)
		if err := helmClient.Upgrade(ctx, helm.UpgradeOptions{
			ReleaseName: DefaultReleaseName,
			Chart:       DefaultChartName,
			Namespace:   cfg.Namespace,
			Version:     cfg.Version,
			Values:      values,
			Wait:        true,
			Timeout:     5 * time.Minute,
		}); err != nil {
			if releaseStatus != "deployed" {
				// Upgrade of failed release didn't work - warn and skip
				fmt.Printf("  ⚠ Warning: Failed to upgrade release (status: %s): %v\n", releaseStatus, err)
				fmt.Println("  ⚠ Manual intervention required. You may need to:")
				fmt.Println("    1. Check pod status: kubectl get pods -n", cfg.Namespace, "-l app.kubernetes.io/name=cert-manager")
				fmt.Println("    2. If data loss is acceptable, uninstall manually: helm uninstall cert-manager -n", cfg.Namespace)
				return fmt.Errorf("failed to upgrade cert-manager (manual intervention required): %w", err)
			}
			return fmt.Errorf("failed to upgrade cert-manager: %w", err)
		}
	} else {
		// Install cert-manager via Helm
		if err := helmClient.Install(ctx, helm.InstallOptions{
			ReleaseName: DefaultReleaseName,
			Chart:       DefaultChartName,
			Namespace:   cfg.Namespace,
			Version:     cfg.Version,
			Values:      values,
			Wait:        true,
			Timeout:     5 * time.Minute,
		}); err != nil {
			return fmt.Errorf("failed to install cert-manager: %w", err)
		}
	}

	// Wait for cert-manager pods to be ready
	if err := waitForCertManager(ctx, k8sClient, cfg.Namespace); err != nil {
		return fmt.Errorf("cert-manager pods not ready: %w", err)
	}

	// Create default issuer if configured
	if cfg.CreateDefaultIssuer {
		if err := createDefaultIssuer(ctx, k8sClient, cfg); err != nil {
			return fmt.Errorf("failed to create default issuer: %w", err)
		}
	}

	return nil
}

// GetStatus returns the status of the cert-manager installation
func GetStatus(ctx context.Context, cfg *Config) (*component.ComponentStatus, error) {
	// This is a placeholder - in a real implementation, you'd check:
	// 1. Helm release status
	// 2. Pod readiness
	// 3. CRD existence
	// 4. Webhook availability
	return &component.ComponentStatus{
		Installed: true,
		Version:   cfg.Version,
		Healthy:   true,
		Message:   "cert-manager is running",
	}, nil
}

// Uninstall removes the cert-manager installation
func Uninstall(ctx context.Context, cfg *Config) error {
	// This is a placeholder - in a real implementation, you'd:
	// 1. Delete ClusterIssuers
	// 2. Uninstall Helm release
	// 3. Clean up CRDs if needed
	return fmt.Errorf("uninstall not yet implemented")
}

// ensureNamespace creates the namespace if it doesn't exist
func ensureNamespace(ctx context.Context, k8sClient K8sClient, namespace string) error {
	_, err := k8sClient.GetNamespace(ctx, namespace)
	if err == nil {
		// Namespace exists
		return nil
	}

	// Create namespace
	return k8sClient.CreateNamespace(ctx, namespace)
}

// waitForCertManager waits for cert-manager pods to be ready
func waitForCertManager(ctx context.Context, k8sClient K8sClient, namespace string) error {
	timeout := time.After(3 * time.Minute)
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-timeout:
			return fmt.Errorf("timeout waiting for cert-manager pods")
		case <-ticker.C:
			pods, err := k8sClient.GetPods(ctx, namespace)
			if err != nil {
				continue
			}

			if len(pods) == 0 {
				continue
			}

			// Check if all cert-manager pods are ready
			allReady := true
			for _, pod := range pods {
				if !pod.Ready {
					allReady = false
					break
				}
			}

			if allReady {
				return nil
			}
		}
	}
}

// createDefaultIssuer creates a default ClusterIssuer
func createDefaultIssuer(ctx context.Context, k8sClient K8sClient, cfg *Config) error {
	var manifest string

	switch cfg.DefaultIssuerType {
	case "self-signed":
		manifest = generateSelfSignedIssuer()
	case "acme":
		if cfg.ACMEEmail == "" {
			return fmt.Errorf("acme_email is required for ACME issuer")
		}
		manifest = generateACMEIssuer(cfg.ACMEEmail, cfg.ACMEServer)
	default:
		return fmt.Errorf("unsupported issuer type: %s", cfg.DefaultIssuerType)
	}

	return k8sClient.ApplyManifest(ctx, manifest)
}

// generateSelfSignedIssuer generates a self-signed ClusterIssuer and CA issuer manifest
// This creates a chain: selfsigned-issuer -> foundry-ca Certificate -> foundry-ca-issuer
// The foundry-ca-issuer can then sign certificates for all services
func generateSelfSignedIssuer() string {
	return `apiVersion: cert-manager.io/v1
kind: ClusterIssuer
metadata:
  name: selfsigned-issuer
spec:
  selfSigned: {}
---
apiVersion: cert-manager.io/v1
kind: Certificate
metadata:
  name: foundry-ca
  namespace: cert-manager
spec:
  isCA: true
  commonName: foundry-ca
  secretName: foundry-ca-secret
  duration: 87600h # 10 years
  renewBefore: 720h # 30 days
  privateKey:
    algorithm: ECDSA
    size: 256
  issuerRef:
    name: selfsigned-issuer
    kind: ClusterIssuer
    group: cert-manager.io
---
apiVersion: cert-manager.io/v1
kind: ClusterIssuer
metadata:
  name: foundry-ca-issuer
spec:
  ca:
    secretName: foundry-ca-secret
`
}

// generateACMEIssuer generates an ACME ClusterIssuer manifest
func generateACMEIssuer(email, server string) string {
	return fmt.Sprintf(`apiVersion: cert-manager.io/v1
kind: ClusterIssuer
metadata:
  name: letsencrypt-prod
spec:
  acme:
    server: %s
    email: %s
    privateKeySecretRef:
      name: letsencrypt-prod-account-key
    solvers:
    - http01:
        ingress:
          class: contour
`, server, email)
}
