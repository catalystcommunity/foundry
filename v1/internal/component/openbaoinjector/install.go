package openbaoinjector

import (
	"context"
	"fmt"
	"time"

	"github.com/catalystcommunity/foundry/v1/internal/helm"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	// DefaultNamespace is the namespace where the injector will be installed
	DefaultNamespace = "openbao"

	// ReleaseName is the Helm release name
	ReleaseName = "openbao-injector"

	// Helm repo constants
	repoName = "openbao"
	repoURL  = "https://openbao.github.io/openbao-helm"
	chart    = "openbao/openbao"

	installTimeout = 5 * time.Minute

	// vault-reviewer ServiceAccount for Kubernetes auth
	vaultReviewerSA          = "vault-reviewer"
	vaultReviewerClusterRole = "system:auth-delegator"
	kubeSystemNamespace      = "kube-system"
)

// Install installs the OpenBao agent injector using Helm.
// Only the injector is deployed (server.enabled=false). The injector registers
// a MutatingWebhookConfiguration so that pods with vault.hashicorp.com/agent-inject
// annotations automatically get secrets mounted from OpenBao.
// If configureK8sAuth is true, it also sets up Kubernetes auth in OpenBao.
func Install(ctx context.Context, helmClient HelmClient, k8sClient K8sClient, openbaoClient OpenBAOClient, cfg *Config, configureK8sAuth bool) error {
	if helmClient == nil {
		return fmt.Errorf("helm client cannot be nil")
	}
	if cfg == nil {
		return fmt.Errorf("config cannot be nil")
	}

	// Add OpenBao Helm repository
	if err := helmClient.AddRepo(ctx, helm.RepoAddOptions{
		Name:        repoName,
		URL:         repoURL,
		ForceUpdate: true,
	}); err != nil {
		return fmt.Errorf("failed to add openbao helm repo: %w", err)
	}

	values := buildHelmValues(cfg)

	// Check if release already exists
	releases, err := helmClient.List(ctx, cfg.Namespace)
	if err == nil {
		for _, rel := range releases {
			if rel.Name == ReleaseName {
				if rel.Status == "deployed" {
					fmt.Println("  Upgrading existing OpenBao injector deployment...")
					if err := helmClient.Upgrade(ctx, helm.UpgradeOptions{
						ReleaseName: ReleaseName,
						Namespace:   cfg.Namespace,
						Chart:       chart,
						Version:     cfg.Version,
						Values:      values,
						Wait:        true,
						Timeout:     installTimeout,
					}); err != nil {
						return fmt.Errorf("failed to upgrade openbao injector: %w", err)
					}
				} else {
					// Failed/pending release — uninstall and reinstall
					fmt.Printf("  Removing failed release (status: %s)...\n", rel.Status)
					if err := helmClient.Uninstall(ctx, helm.UninstallOptions{
						ReleaseName: ReleaseName,
						Namespace:   cfg.Namespace,
					}); err != nil {
						return fmt.Errorf("failed to remove existing release: %w", err)
					}
				}
				break
			}
		}
	}

	// Only install if release wasn't already deployed (we didn't upgrade above)
	releaseExists := false
	releases, _ = helmClient.List(ctx, cfg.Namespace)
	for _, rel := range releases {
		if rel.Name == ReleaseName && rel.Status == "deployed" {
			releaseExists = true
			break
		}
	}

	if !releaseExists {
		if err := helmClient.Install(ctx, helm.InstallOptions{
			ReleaseName:     ReleaseName,
			Namespace:       cfg.Namespace,
			Chart:           chart,
			Version:         cfg.Version,
			Values:          values,
			CreateNamespace: true,
			Wait:            true,
			Timeout:         installTimeout,
		}); err != nil {
			return fmt.Errorf("failed to install openbao injector: %w", err)
		}
	}

	fmt.Printf("  ✓ OpenBao agent injector installed\n")
	fmt.Printf("  ✓ MutatingWebhookConfiguration registered\n")
	fmt.Printf("  Pods annotated with vault.hashicorp.com/agent-inject=true will now\n")
	fmt.Printf("  automatically receive secrets from OpenBao at %s\n", cfg.ExternalVaultAddr)

	// Configure Kubernetes auth if requested
	if configureK8sAuth {
		if k8sClient == nil || openbaoClient == nil {
			return fmt.Errorf("k8s client and openbao client are required for k8s auth configuration")
		}
		if err := configureKubernetesAuth(ctx, k8sClient, openbaoClient, cfg); err != nil {
			return fmt.Errorf("failed to configure Kubernetes auth: %w", err)
		}
	}

	return nil
}

// buildHelmValues constructs the Helm values for injector-only installation.
// server.enabled=false — we already have OpenBao running on the host.
// injector.enabled=true — this is the only thing we're installing.
func buildHelmValues(cfg *Config) map[string]interface{} {
	return map[string]interface{}{
		"server": map[string]interface{}{
			"enabled": false,
		},
		"injector": map[string]interface{}{
			"enabled":           true,
			"externalVaultAddr": cfg.ExternalVaultAddr,
		},
	}
}

// configureKubernetesAuth sets up Kubernetes auth in OpenBao so pods can authenticate
func configureKubernetesAuth(ctx context.Context, k8sClient K8sClient, openbaoClient OpenBAOClient, cfg *Config) error {
	fmt.Println("\n  Configuring Kubernetes auth in OpenBao...")

	// 1. Create vault-reviewer ServiceAccount in kube-system
	fmt.Println("  Creating vault-reviewer ServiceAccount...")
	sa := &corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:      vaultReviewerSA,
			Namespace: kubeSystemNamespace,
		},
	}
	if err := k8sClient.CreateServiceAccount(ctx, vaultReviewerSA, sa); err != nil {
		return fmt.Errorf("failed to create vault-reviewer SA: %w", err)
	}

	// 2. Create ClusterRoleBinding to grant auth-delegator role
	fmt.Println("  Creating ClusterRoleBinding for vault-reviewer...")
	clusterRoleBindingManifest := fmt.Sprintf(`apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRoleBinding
metadata:
  name: vault-reviewer-auth-delegator
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: ClusterRole
  name: %s
subjects:
- kind: ServiceAccount
  name: %s
  namespace: %s
`, vaultReviewerClusterRole, vaultReviewerSA, kubeSystemNamespace)

	if err := k8sClient.ApplyClusterRoleBinding(ctx, clusterRoleBindingManifest); err != nil {
		return fmt.Errorf("failed to create ClusterRoleBinding: %w", err)
	}

	// 3. Get the ServiceAccount token
	fmt.Println("  Getting vault-reviewer ServiceAccount token...")
	token, err := k8sClient.GetServiceAccountToken(ctx, kubeSystemNamespace, vaultReviewerSA)
	if err != nil {
		return fmt.Errorf("failed to get SA token: %w", err)
	}

	// 4. Get cluster CA from kubeconfig
	clusterCA, err := k8sClient.GetClusterCACert(ctx)
	if err != nil {
		return fmt.Errorf("failed to get cluster CA: %w", err)
	}

	// 5. Get Kubernetes API server host from kubeconfig
	k8sHost := k8sClient.GetKubernetesHost()
	if k8sHost == "" {
		return fmt.Errorf("failed to get Kubernetes host from kubeconfig")
	}

	// 6. Enable Kubernetes auth method in OpenBao
	fmt.Println("  Enabling Kubernetes auth method in OpenBao...")
	if err := openbaoClient.EnableAuth(ctx, "kubernetes"); err != nil {
		return fmt.Errorf("failed to enable kubernetes auth: %w", err)
	}

	// 7. Write auth config - kubernetes_host must use ClusterIP, not Tailscale
	// Note: kubernetes_host must use the ClusterIP so it's reachable from the host node
	// Also disable_iss_validation=true because JWT issuer is in-cluster DNS name
	authConfig := map[string]interface{}{
		"kubernetes_host":        k8sHost,
		"kubernetes_ca_cert":     clusterCA,
		"token_reviewer_jwt":     token,
		"disable_iss_validation": "true",
		"disable_local_ca_jwt":   "true",
	}

	fmt.Println("  Writing Kubernetes auth configuration to OpenBao...")
	if err := openbaoClient.WriteAuthConfig(ctx, "kubernetes", authConfig); err != nil {
		return fmt.Errorf("failed to write kubernetes auth config: %w", err)
	}

	// 7. Create roles for configured apps (from cfg.K8sAuthRoles if present)
	for _, role := range cfg.K8sAuthRoles {
		roleData := map[string]interface{}{
			"bound_service_account_names":      role.ServiceAccountName,
			"bound_service_account_namespaces": role.ServiceAccountNamespace,
			"policies":                         role.Policies,
			"ttl":                              role.TTL,
		}
		fmt.Printf("  Creating Kubernetes auth role: %s\n", role.RoleName)
		if err := openbaoClient.WriteRole(ctx, "kubernetes", role.RoleName, roleData); err != nil {
			return fmt.Errorf("failed to write role %s: %w", role.RoleName, err)
		}
	}

	fmt.Println("  ✓ Kubernetes auth configured successfully")
	fmt.Println("  Pods can now authenticate to OpenBao using Kubernetes service accounts")

	return nil
}
