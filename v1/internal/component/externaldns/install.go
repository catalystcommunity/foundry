package externaldns

import (
	"context"
	"fmt"
	"time"

	"github.com/catalystcommunity/foundry/v1/internal/helm"
)

const (
	externalDNSRepoName = "external-dns"
	externalDNSRepoURL  = "https://kubernetes-sigs.github.io/external-dns/"
	externalDNSChart    = "external-dns/external-dns"
	releaseName         = "external-dns"
)

// Install installs External-DNS using Helm
func Install(ctx context.Context, helmClient HelmClient, k8sClient K8sClient, cfg *Config) error {
	if helmClient == nil {
		return fmt.Errorf("helm client cannot be nil")
	}
	if cfg == nil {
		cfg = DefaultConfig()
	}

	fmt.Println("  Installing External-DNS...")

	// Add Helm repository
	if err := helmClient.AddRepo(ctx, helm.RepoAddOptions{
		Name:        externalDNSRepoName,
		URL:         externalDNSRepoURL,
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
			if rel.Name == releaseName {
				if rel.Status == "deployed" {
					fmt.Println("  External-DNS already installed")
					return verifyInstallation(ctx, k8sClient, cfg.Namespace)
				}
				// Uninstall failed release
				fmt.Printf("  Removing failed release (status: %s)...\n", rel.Status)
				if err := helmClient.Uninstall(ctx, helm.UninstallOptions{
					ReleaseName: releaseName,
					Namespace:   cfg.Namespace,
				}); err != nil {
					return fmt.Errorf("failed to uninstall existing release: %w", err)
				}
				break
			}
		}
	}

	// Install External-DNS via Helm
	if err := helmClient.Install(ctx, helm.InstallOptions{
		ReleaseName:     releaseName,
		Namespace:       cfg.Namespace,
		Chart:           externalDNSChart,
		Version:         cfg.Version,
		Values:          values,
		CreateNamespace: true,
		Wait:            true,
		Timeout:         5 * time.Minute,
	}); err != nil {
		return fmt.Errorf("failed to install external-dns: %w", err)
	}

	// Verify installation
	if k8sClient != nil {
		if err := verifyInstallation(ctx, k8sClient, cfg.Namespace); err != nil {
			return fmt.Errorf("installation verification failed: %w", err)
		}
	}

	fmt.Println("  External-DNS installed successfully")
	if cfg.IsProviderConfigured() {
		fmt.Printf("  Provider: %s\n", cfg.Provider)
	} else {
		fmt.Println("  Note: No DNS provider configured. External-DNS will not manage DNS records until configured.")
	}
	if len(cfg.DomainFilters) > 0 {
		fmt.Printf("  Domain filters: %v\n", cfg.DomainFilters)
	}
	return nil
}

// buildHelmValues constructs Helm values for External-DNS installation
func buildHelmValues(cfg *Config) map[string]interface{} {
	values := make(map[string]interface{})

	// Start with user-provided values
	for k, v := range cfg.Values {
		values[k] = v
	}

	// Sources configuration
	if len(cfg.Sources) > 0 {
		values["sources"] = cfg.Sources
	}

	// Policy configuration
	values["policy"] = cfg.Policy

	// TXT owner ID for record ownership
	values["txtOwnerId"] = cfg.TxtOwnerId

	// Domain filters
	if len(cfg.DomainFilters) > 0 {
		values["domainFilters"] = cfg.DomainFilters
	}

	// Resource configuration (reasonable defaults for homelab)
	values["resources"] = map[string]interface{}{
		"requests": map[string]interface{}{
			"cpu":    "50m",
			"memory": "64Mi",
		},
	}

	// ServiceMonitor for Prometheus
	values["serviceMonitor"] = map[string]interface{}{
		"enabled": true,
	}

	// Provider-specific configuration
	if cfg.Provider != ProviderNone {
		values["provider"] = map[string]interface{}{
			"name": string(cfg.Provider),
		}

		switch cfg.Provider {
		case ProviderPowerDNS:
			values["provider"] = map[string]interface{}{
				"name": "pdns",
			}
			// PowerDNS requires environment variables
			values["env"] = []map[string]interface{}{
				{
					"name":  "EXTERNAL_DNS_PDNS_SERVER",
					"value": cfg.PowerDNS.APIUrl,
				},
				{
					"name":  "EXTERNAL_DNS_PDNS_API_KEY",
					"value": cfg.PowerDNS.APIKey,
				},
			}
			// Bitnami chart expects extraArgs as an array of strings
			// Note: --pdns-server-id is no longer a valid flag in recent versions
			values["extraArgs"] = []interface{}{
				fmt.Sprintf("--pdns-server=%s", cfg.PowerDNS.APIUrl),
				fmt.Sprintf("--pdns-api-key=%s", cfg.PowerDNS.APIKey),
			}

		case ProviderCloudflare:
			values["provider"] = map[string]interface{}{
				"name": "cloudflare",
			}
			values["env"] = []map[string]interface{}{
				{
					"name":  "CF_API_TOKEN",
					"value": cfg.Cloudflare.APIToken,
				},
			}
			if cfg.Cloudflare.Proxied {
				values["extraArgs"] = []interface{}{
					"--cloudflare-proxied",
				}
			}

		case ProviderRFC2136:
			values["provider"] = map[string]interface{}{
				"name": "rfc2136",
			}
			port := cfg.RFC2136.Port
			if port == 0 {
				port = 53
			}
			tsigAlg := cfg.RFC2136.TSIGSecretAlg
			if tsigAlg == "" {
				tsigAlg = "hmac-sha256"
			}
			extraArgs := []string{
				fmt.Sprintf("--rfc2136-host=%s", cfg.RFC2136.Host),
				fmt.Sprintf("--rfc2136-port=%d", port),
			}
			if cfg.RFC2136.Zone != "" {
				extraArgs = append(extraArgs, fmt.Sprintf("--rfc2136-zone=%s", cfg.RFC2136.Zone))
			}
			if cfg.RFC2136.TSIGKeyName != "" {
				extraArgs = append(extraArgs, fmt.Sprintf("--rfc2136-tsig-keyname=%s", cfg.RFC2136.TSIGKeyName))
			}
			if cfg.RFC2136.TSIGSecret != "" {
				extraArgs = append(extraArgs, fmt.Sprintf("--rfc2136-tsig-secret=%s", cfg.RFC2136.TSIGSecret))
				extraArgs = append(extraArgs, fmt.Sprintf("--rfc2136-tsig-secret-alg=%s", tsigAlg))
			}
			values["extraArgs"] = extraArgs
		}
	}

	// Log level for debugging
	values["logLevel"] = "info"

	return values
}

// verifyInstallation verifies that External-DNS pods are running
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
			return fmt.Errorf("timeout waiting for external-dns pods to be ready")
		case <-ticker.C:
			pods, err := k8sClient.GetPods(ctx, namespace)
			if err != nil {
				continue // Retry on error
			}

			if len(pods) == 0 {
				continue // Wait for pods to appear
			}

			// Check if external-dns pod is running
			externalDNSFound := false
			for _, pod := range pods {
				if pod.Name == "" {
					continue
				}
				// Look for external-dns pod
				if containsSubstring(pod.Name, "external-dns") {
					externalDNSFound = true
					if pod.Status != "Running" {
						break
					}
				}
			}

			if externalDNSFound {
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
