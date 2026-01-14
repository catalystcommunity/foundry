package component

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/catalystcommunity/foundry/v1/internal/component"
	"github.com/catalystcommunity/foundry/v1/internal/component/certmanager"
	"github.com/catalystcommunity/foundry/v1/internal/component/contour"
	"github.com/catalystcommunity/foundry/v1/internal/component/dns"
	"github.com/catalystcommunity/foundry/v1/internal/component/externaldns"
	"github.com/catalystcommunity/foundry/v1/internal/component/gatewayapi"
	"github.com/catalystcommunity/foundry/v1/internal/component/grafana"
	"github.com/catalystcommunity/foundry/v1/internal/component/loki"
	"github.com/catalystcommunity/foundry/v1/internal/component/seaweedfs"
	"github.com/catalystcommunity/foundry/v1/internal/component/openbao"
	"github.com/catalystcommunity/foundry/v1/internal/component/prometheus"
	componentStorage "github.com/catalystcommunity/foundry/v1/internal/component/storage"
	"github.com/catalystcommunity/foundry/v1/internal/component/velero"
	"github.com/catalystcommunity/foundry/v1/internal/config"
	"github.com/catalystcommunity/foundry/v1/internal/helm"
	"github.com/catalystcommunity/foundry/v1/internal/host"
	"github.com/catalystcommunity/foundry/v1/internal/k8s"
	"github.com/catalystcommunity/foundry/v1/internal/secrets"
	"github.com/catalystcommunity/foundry/v1/internal/ssh"
	"github.com/urfave/cli/v3"
)

// sshExecutorAdapter adapts ssh.Connection to container.SSHExecutor interface
// by implementing the Execute(cmd string) (string, error) method
type sshExecutorAdapter struct {
	conn *ssh.Connection
}

func (a *sshExecutorAdapter) Execute(cmd string) (string, error) {
	result, err := a.conn.Exec(cmd)
	if err != nil {
		return "", err
	}
	if result.ExitCode != 0 {
		return result.Stdout, fmt.Errorf("command failed with exit code %d: %s", result.ExitCode, result.Stderr)
	}
	return result.Stdout, nil
}

// InstallCommand installs a component
var InstallCommand = &cli.Command{
	Name:      "install",
	Usage:     "Install a component",
	ArgsUsage: "<name>",
	Description: `Installs a component with its dependencies.

The component will be installed according to the configuration in ~/.foundry/stack.yaml.

Examples:
  # Phase 2 (container-based) components:
  foundry component install openbao
  foundry component install dns
  foundry component install zot

  # Phase 3 (Kubernetes-based) components:
  foundry component install storage --backend local-path
  foundry component install seaweedfs
  foundry component install prometheus
  foundry component install loki
  foundry component install grafana
  foundry component install external-dns
  foundry component install velero`,
	Flags: []cli.Flag{
		&cli.BoolFlag{
			Name:    "dry-run",
			Usage:   "Show what would be installed without actually installing",
			Aliases: []string{"n"},
		},
		&cli.StringFlag{
			Name:  "version",
			Usage: "Override the version to install",
		},
		// Storage-specific flags
		&cli.StringFlag{
			Name:  "backend",
			Usage: "Storage backend: local-path, nfs, longhorn (for storage component)",
			Value: "local-path",
		},
		&cli.StringFlag{
			Name:  "nfs-server",
			Usage: "NFS server address (for nfs backend)",
		},
		&cli.StringFlag{
			Name:  "nfs-path",
			Usage: "NFS export path (for nfs backend)",
		},
	},
	Action: runInstall,
}

// k8sComponents lists all components that are installed via kubeconfig (Helm/K8s)
var k8sComponents = map[string]bool{
	"gateway-api":   true,
	"contour":       true,
	"cert-manager":  true,
	"storage":       true,
	"seaweedfs":     true,
	"prometheus":    true,
	"loki":          true,
	"grafana":       true,
	"external-dns":  true,
	"velero":        true,
}

func runInstall(ctx context.Context, cmd *cli.Command) error {
	// Get component name from arguments
	if cmd.Args().Len() == 0 {
		return fmt.Errorf("component name required\n\nUsage: foundry component install <name>")
	}

	name := cmd.Args().Get(0)
	dryRun := cmd.Bool("dry-run")
	version := cmd.String("version")

	// Get component from registry
	comp := component.Get(name)
	if comp == nil {
		return component.ErrComponentNotFound(name)
	}

	// Load stack configuration (needed for dependency checking)
	fmt.Println("Loading stack configuration...")
	configPath, err := config.FindConfig(cmd.String("config"))
	if err != nil {
		return fmt.Errorf("failed to find config: %w", err)
	}
	stackConfig, err := config.Load(configPath)
	if err != nil {
		return fmt.Errorf("failed to load stack config: %w\n\nHint: Run 'foundry config init' to create a configuration", err)
	}

	// Check dependencies using setup state
	deps := comp.Dependencies()
	if len(deps) > 0 {
		fmt.Printf("Component %s depends on: %v\n", name, deps)
		fmt.Println("Checking dependencies...")

		for _, dep := range deps {
			installed := isDependencyInstalled(dep, stackConfig)
			if !installed {
				return fmt.Errorf("dependency %q is not installed\n\nPlease run: foundry component install %s", dep, dep)
			}

			fmt.Printf("  ✓ %s (installed)\n", dep)
		}
		fmt.Println()
	}

	// Check if this is a K8s-based component
	if k8sComponents[name] {
		return installK8sComponent(ctx, cmd, name, stackConfig, dryRun, version)
	}

	// SSH-based component installation (Phase 2 components)
	return installSSHComponent(ctx, cmd, name, stackConfig, dryRun, version)
}

// installK8sComponent installs a Kubernetes-based component using kubeconfig
func installK8sComponent(ctx context.Context, cmd *cli.Command, name string, stackConfig *config.Config, dryRun bool, version string) error {
	fmt.Printf("Installing Kubernetes component: %s\n", name)

	// Get kubeconfig path
	configDir, err := config.GetConfigDir()
	if err != nil {
		return fmt.Errorf("failed to get config directory: %w", err)
	}
	kubeconfigPath := filepath.Join(configDir, "kubeconfig")

	// Verify kubeconfig exists
	kubeconfigBytes, err := os.ReadFile(kubeconfigPath)
	if err != nil {
		return fmt.Errorf("kubeconfig not found at %s: %w\n\nHint: Install K3s first with 'foundry stack install' or 'foundry component install k3s'", kubeconfigPath, err)
	}

	if dryRun {
		fmt.Printf("\nWould install Kubernetes component: %s\n", name)
		if version != "" {
			fmt.Printf("Version: %s\n", version)
		}
		fmt.Println("\nNote: This is a dry-run. No changes will be made.")
		return nil
	}

	// Create Helm and K8s clients
	helmClient, err := helm.NewClient(kubeconfigBytes, "default")
	if err != nil {
		return fmt.Errorf("failed to create helm client: %w", err)
	}

	k8sClient, err := k8s.NewClientFromKubeconfig(kubeconfigBytes)
	if err != nil {
		return fmt.Errorf("failed to create k8s client: %w", err)
	}

	// Build component config
	cfg := component.ComponentConfig{}
	if version != "" {
		cfg["version"] = version
	}

	// Add cluster VIP for components that need it
	if stackConfig.Cluster.VIP != "" {
		cfg["cluster_vip"] = stackConfig.Cluster.VIP
	}

	// Create component-specific instance with clients and install
	var componentWithClients component.Component
	switch name {
	case "gateway-api":
		componentWithClients = gatewayapi.NewComponent(k8sClient)
	case "contour":
		componentWithClients = contour.NewComponent(helmClient, k8sClient)
	case "cert-manager":
		componentWithClients = certmanager.NewComponent(nil)
	case "storage":
		// Handle storage-specific flags
		backend := cmd.String("backend")
		cfg["backend"] = backend
		if backend == "nfs" {
			nfsServer := cmd.String("nfs-server")
			nfsPath := cmd.String("nfs-path")
			if nfsServer == "" || nfsPath == "" {
				return fmt.Errorf("--nfs-server and --nfs-path are required for nfs backend")
			}
			cfg["nfs"] = map[string]interface{}{
				"server": nfsServer,
				"path":   nfsPath,
			}
		}
		componentWithClients = componentStorage.NewComponent(helmClient, k8sClient)
	case "seaweedfs":
		componentWithClients = seaweedfs.NewComponent(helmClient, k8sClient)
	case "prometheus":
		// Auto-populate external targets for infrastructure services
		externalTargets := buildExternalTargetsFromStackConfig(stackConfig)
		if len(externalTargets) > 0 {
			cfg["external_targets"] = externalTargets
		}
		componentWithClients = prometheus.NewComponent(helmClient, k8sClient)
	case "loki":
		// Get SeaweedFS credentials from the SeaweedFS secret
		seaweedfsKey, seaweedfsSecret, err := getSeaweedFSCredentials(k8sClient)
		if err != nil {
			return fmt.Errorf("failed to get SeaweedFS credentials: %w", err)
		}
		cfg["s3_endpoint"] = "http://seaweedfs-s3.seaweedfs.svc.cluster.local:8333"
		cfg["s3_access_key"] = seaweedfsKey
		cfg["s3_secret_key"] = seaweedfsSecret
		cfg["s3_bucket"] = "loki"
		cfg["s3_region"] = "us-east-1"
		componentWithClients = loki.NewComponent(helmClient, k8sClient)
	case "grafana":
		// Get Prometheus and Loki endpoints for data sources
		cfg["prometheus_url"] = "http://kube-prometheus-stack-prometheus.monitoring.svc.cluster.local:9090"
		cfg["loki_url"] = "http://loki.loki.svc.cluster.local:3100"
		componentWithClients = grafana.NewComponent(helmClient, k8sClient)
	case "external-dns":
		// Get PowerDNS configuration if available
		if stackConfig.DNS != nil && stackConfig.SetupState != nil && stackConfig.SetupState.DNSInstalled {
			dnsAddr, err := stackConfig.GetPrimaryDNSAddress()
			if err == nil {
				cfg["provider"] = "pdns"
				// PowerDNS config must be a nested map
				pdnsConfig := map[string]interface{}{
					"api_url": fmt.Sprintf("http://%s:8081", dnsAddr),
				}
				// Try to get API key from OpenBAO
				apiKey, err := getDNSAPIKey(stackConfig)
				if err == nil {
					pdnsConfig["api_key"] = apiKey
				}
				cfg["powerdns"] = pdnsConfig
			}
		}
		componentWithClients = externaldns.NewComponent(helmClient, k8sClient)
	case "velero":
		// Get SeaweedFS credentials from the SeaweedFS secret
		seaweedfsKey, seaweedfsSecret, err := getSeaweedFSCredentials(k8sClient)
		if err != nil {
			return fmt.Errorf("failed to get SeaweedFS credentials: %w", err)
		}
		cfg["s3_endpoint"] = "http://seaweedfs-s3.seaweedfs.svc.cluster.local:8333"
		cfg["s3_access_key"] = seaweedfsKey
		cfg["s3_secret_key"] = seaweedfsSecret
		cfg["s3_bucket"] = "velero"
		cfg["s3_region"] = "us-east-1"
		componentWithClients = velero.NewComponent(helmClient, k8sClient)
	default:
		return fmt.Errorf("unknown kubernetes component: %s", name)
	}

	fmt.Printf("\nInstalling component: %s\n", name)

	// Install the component
	if err := componentWithClients.Install(ctx, cfg); err != nil {
		return fmt.Errorf("installation failed: %w", err)
	}

	fmt.Printf("\n✓ Component %s installed successfully\n", name)

	// Update setup state
	if err := updateSetupState(cmd, stackConfig, name, cfg); err != nil {
		fmt.Printf("\n⚠ Warning: Failed to update setup state: %v\n", err)
	}

	return nil
}

// getSeaweedFSCredentials retrieves SeaweedFS credentials from the seaweedfs secret
func getSeaweedFSCredentials(k8sClient *k8s.Client) (string, string, error) {
	if k8sClient == nil {
		return "", "", fmt.Errorf("k8s client is nil")
	}

	secret, err := k8sClient.GetSecret(context.Background(), "seaweedfs", "seaweedfs")
	if err != nil {
		return "", "", fmt.Errorf("failed to get seaweedfs secret: %w", err)
	}

	accessKey, ok := secret.Data["accessKey"]
	if !ok {
		return "", "", fmt.Errorf("accessKey not found in seaweedfs secret")
	}

	secretKey, ok := secret.Data["secretKey"]
	if !ok {
		return "", "", fmt.Errorf("secretKey not found in seaweedfs secret")
	}

	return string(accessKey), string(secretKey), nil
}

// getDNSAPIKey retrieves the DNS API key from OpenBAO
func getDNSAPIKey(stackConfig *config.Config) (string, error) {
	configDir, err := config.GetConfigDir()
	if err != nil {
		return "", err
	}

	addr, err := stackConfig.GetPrimaryOpenBAOAddress()
	if err != nil {
		return "", err
	}
	openBAOAddr := fmt.Sprintf("http://%s:8200", addr)

	keysPath := filepath.Join(configDir, "openbao-keys", stackConfig.Cluster.Name, "keys.json")
	keysData, err := os.ReadFile(keysPath)
	if err != nil {
		return "", err
	}

	var keys struct {
		RootToken string `json:"root_token"`
	}
	if err := json.Unmarshal(keysData, &keys); err != nil {
		return "", err
	}

	openBAOClient := openbao.NewClient(openBAOAddr, keys.RootToken)
	ctx := context.Background()
	data, err := openBAOClient.ReadSecretV2(ctx, "foundry-core", "dns")
	if err != nil {
		return "", err
	}
	if apiKey, ok := data["api_key"].(string); ok {
		return apiKey, nil
	}
	return "", fmt.Errorf("api_key not found in OpenBAO")
}

// installSSHComponent installs a container-based component via SSH
func installSSHComponent(ctx context.Context, cmd *cli.Command, name string, stackConfig *config.Config, dryRun bool, version string) error {
	// Determine target host for this component
	targetHost, err := getTargetHostForComponent(name, stackConfig)
	if err != nil {
		return fmt.Errorf("failed to determine target host: %w", err)
	}

	fmt.Printf("Target host: %s (%s)\n", targetHost.Hostname, targetHost.Address)

	if dryRun {
		fmt.Printf("\nWould install component: %s\n", name)
		fmt.Printf("Target host: %s\n", targetHost.Hostname)
		if version != "" {
			fmt.Printf("Version: %s\n", version)
		}
		fmt.Println("\nNote: This is a dry-run. No changes will be made.")
		return nil
	}

	// Get component from registry
	comp := component.Get(name)
	if comp == nil {
		return component.ErrComponentNotFound(name)
	}

	// Establish SSH connection to target host
	fmt.Printf("Connecting to %s...\n", targetHost.Hostname)
	conn, err := connectToHost(targetHost, stackConfig)
	if err != nil {
		return fmt.Errorf("failed to connect to host: %w", err)
	}
	defer conn.Close()
	fmt.Println("✓ Connected")

	// Create adapter for components that need container.SSHExecutor
	executor := &sshExecutorAdapter{conn: conn}

	// Build component config
	cfg := component.ComponentConfig{}
	if version != "" {
		cfg["version"] = version
	}

	// Add SSH connection to config (components extract this)
	// We pass both the raw connection and the executor adapter
	cfg["host"] = executor
	cfg["ssh_conn"] = conn // Some components might need the raw connection

	// Add cluster name for OpenBAO key storage
	cfg["cluster_name"] = stackConfig.Cluster.Name

	// Add keys directory for OpenBAO
	keysDir, err := config.GetKeysDir()
	if err != nil {
		return fmt.Errorf("failed to get keys directory: %w", err)
	}
	// Use openbao-keys subdirectory
	cfg["keys_dir"] = filepath.Join(filepath.Dir(keysDir), "openbao-keys")

	// Add OpenBAO API URL (for OpenBAO component initialization)
	if name == "openbao" {
		addr, err := stackConfig.GetPrimaryOpenBAOAddress()
		if err == nil {
			// Construct the API URL from the network config
			cfg["api_url"] = fmt.Sprintf("http://%s:8200", addr)
		}
	}

	// Handle PowerDNS API key generation and storage
	if (name == "dns" || name == "powerdns") && stackConfig.SetupState.OpenBAOInitialized {
		apiKey, err := ensureDNSAPIKey(stackConfig)
		if err != nil {
			return fmt.Errorf("failed to setup DNS API key: %w", err)
		}
		cfg["api_key"] = apiKey

		// Pass all zones as local zones for Recursor forwarding
		// This includes primary_domain + kubernetes_zones + infrastructure_zones (deduplicated)
		localZones := []string{}
		seen := make(map[string]bool)
		if stackConfig.Cluster.PrimaryDomain != "" {
			localZones = append(localZones, stackConfig.Cluster.PrimaryDomain)
			seen[stackConfig.Cluster.PrimaryDomain] = true
		}
		if stackConfig.DNS != nil {
			for _, zone := range stackConfig.DNS.KubernetesZones {
				if !seen[zone.Name] {
					localZones = append(localZones, zone.Name)
					seen[zone.Name] = true
				}
			}
			for _, zone := range stackConfig.DNS.InfrastructureZones {
				if !seen[zone.Name] {
					localZones = append(localZones, zone.Name)
					seen[zone.Name] = true
				}
			}
		}
		if len(localZones) > 0 {
			cfg["local_zones"] = localZones
			fmt.Printf("  Local zones for DNS recursor: %v\n", localZones)
		}
	}

	fmt.Printf("\nInstalling component: %s\n", name)

	// Install component
	if err := comp.Install(ctx, cfg); err != nil {
		return fmt.Errorf("failed to install component %s: %w", name, err)
	}

	fmt.Printf("\n✓ Component %s installed successfully\n", name)

	// Update setup state and component-specific config
	if err := updateSetupState(cmd, stackConfig, name, cfg); err != nil {
		// Don't fail the whole installation if state update fails
		// Just warn the user
		fmt.Printf("\n⚠ Warning: Failed to update setup state: %v\n", err)
		fmt.Println("The component is installed and working, but state tracking may be incorrect.")
	}

	// Handle DNS record registration (bidirectional)
	if err := handleDNSRegistration(ctx, name, stackConfig); err != nil {
		// Don't fail the installation if DNS registration fails
		// Just warn the user
		fmt.Printf("\n⚠ Warning: Failed to register DNS records: %v\n", err)
		fmt.Println("The component is installed and working, but DNS records may need manual creation.")
	}

	return nil
}

// handleDNSRegistration handles bidirectional DNS record registration
// - If DNS is being installed: look backward at installed components and create their records
// - If another component is being installed: if DNS exists, self-register
func handleDNSRegistration(ctx context.Context, componentName string, stackConfig *config.Config) error {
	// Skip if we don't have network config or cluster domain or setup state
	if stackConfig == nil || stackConfig.Network == nil || stackConfig.Cluster.PrimaryDomain == "" || stackConfig.SetupState == nil {
		return nil
	}

	switch componentName {
	case "dns", "powerdns":
		// DNS is being installed - look backward at installed components
		return registerExistingComponents(ctx, stackConfig)
	case "openbao":
		// OpenBAO is being installed - self-register if DNS exists
		if stackConfig.SetupState.DNSInstalled {
			return registerComponentDNS(ctx, "openbao", stackConfig)
		}
	case "zot":
		// Zot is being installed - self-register if DNS exists
		if stackConfig.SetupState.DNSInstalled {
			return registerComponentDNS(ctx, "zot", stackConfig)
		}
	case "k3s", "kubernetes":
		// K3s is being installed - self-register VIP if DNS exists
		if stackConfig.SetupState.DNSInstalled {
			return registerComponentDNS(ctx, "k8s", stackConfig)
		}
	}

	return nil
}

// registerExistingComponents creates DNS records for components that were installed before DNS
func registerExistingComponents(ctx context.Context, stackConfig *config.Config) error {
	fmt.Println("\nRegistering DNS records for existing components...")

	// Get DNS client
	dnsClient, err := getDNSClient(stackConfig)
	if err != nil {
		return fmt.Errorf("failed to create DNS client: %w", err)
	}

	// Import dns package types
	dnsZone := stackConfig.Cluster.PrimaryDomain

	// Register each installed component
	registered := 0

	if stackConfig.SetupState.OpenBAOInstalled {
		addr, err := stackConfig.GetPrimaryOpenBAOAddress()
		if err == nil {
			if err := registerServiceRecord(dnsClient, dnsZone, "openbao", addr); err != nil {
				return fmt.Errorf("failed to register openbao DNS record: %w", err)
			}
			registered++
		}
	}

	// Always register DNS service (pointing to itself)
	addr, err := stackConfig.GetPrimaryDNSAddress()
	if err == nil {
		if err := registerServiceRecord(dnsClient, dnsZone, "dns", addr); err != nil {
			return fmt.Errorf("failed to register dns DNS record: %w", err)
		}
		registered++
	}

	if stackConfig.SetupState.ZotInstalled {
		addr, err := stackConfig.GetPrimaryZotAddress()
		if err == nil {
			if err := registerServiceRecord(dnsClient, dnsZone, "zot", addr); err != nil {
				return fmt.Errorf("failed to register zot DNS record: %w", err)
			}
			registered++
		}
	}

	if stackConfig.SetupState.K8sInstalled && stackConfig.Cluster.VIP != "" {
		if err := registerServiceRecord(dnsClient, dnsZone, "k8s", stackConfig.Cluster.VIP); err != nil {
			return fmt.Errorf("failed to register k8s DNS record: %w", err)
		}
		registered++
	}

	if registered > 0 {
		fmt.Printf("✓ Registered %d DNS record(s)\n", registered)
	} else {
		fmt.Println("  No existing components to register")
	}

	return nil
}

// registerComponentDNS registers a DNS record for a single component
func registerComponentDNS(ctx context.Context, componentName string, stackConfig *config.Config) error {
	fmt.Printf("\nRegistering DNS record for %s...\n", componentName)

	// Get DNS client
	dnsClient, err := getDNSClient(stackConfig)
	if err != nil {
		return fmt.Errorf("failed to create DNS client: %w", err)
	}

	dnsZone := stackConfig.Cluster.PrimaryDomain

	// Determine IP address for this component
	var ip string
	switch componentName {
	case "openbao":
		ip, err = stackConfig.GetPrimaryOpenBAOAddress()
		if err != nil {
			return fmt.Errorf("no OpenBAO host configured: %w", err)
		}
	case "zot":
		ip, err = stackConfig.GetPrimaryZotAddress()
		if err != nil {
			return fmt.Errorf("no Zot host configured: %w", err)
		}
	case "k8s":
		if stackConfig.Cluster.VIP == "" {
			return fmt.Errorf("no K8s VIP configured")
		}
		ip = stackConfig.Cluster.VIP
	default:
		return fmt.Errorf("unknown component: %s", componentName)
	}

	if err := registerServiceRecord(dnsClient, dnsZone, componentName, ip); err != nil {
		return fmt.Errorf("failed to register DNS record: %w", err)
	}

	fmt.Printf("✓ DNS record registered: %s.%s -> %s\n", componentName, dnsZone, ip)
	return nil
}

// registerServiceRecord creates an A record for a service
// name should be a short hostname (e.g., "openbao"), not an FQDN
func registerServiceRecord(dnsClient *dns.Client, zone, name, ip string) error {
	fmt.Printf("  Creating A record: %s.%s -> %s\n", name, zone, ip)

	// Use the DNS package's AddARecord helper function
	// Pass short hostname (name) not FQDN to avoid double zone appending
	if err := dns.AddARecord(dnsClient, zone, name, ip); err != nil {
		return fmt.Errorf("failed to add A record: %w", err)
	}

	return nil
}

// getDNSClient creates a DNS client for managing records
func getDNSClient(stackConfig *config.Config) (*dns.Client, error) {
	// Get DNS server address
	dnsHost, err := stackConfig.GetPrimaryDNSAddress()
	if err != nil {
		return nil, fmt.Errorf("no DNS host configured: %w", err)
	}

	// Get DNS API key from config (it's a secret reference)
	// We'll need to resolve it from OpenBAO
	if stackConfig.DNS == nil || stackConfig.DNS.APIKey == "" {
		return nil, fmt.Errorf("DNS API key not configured")
	}

	// Resolve API key from secrets (including OpenBAO)
	resolver, resCtx, err := buildSecretResolver(stackConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to setup secret resolver: %w", err)
	}

	// Parse the API key secret reference
	secretRef, err := secrets.ParseSecretRef(stackConfig.DNS.APIKey)
	if err != nil {
		return nil, fmt.Errorf("failed to parse DNS API key secret reference: %w", err)
	}
	if secretRef == nil {
		// Not a secret reference, use as-is
		return dns.NewClient(fmt.Sprintf("http://%s:8081", dnsHost), stackConfig.DNS.APIKey), nil
	}

	// Resolve the API key
	apiKey, err := resolver.Resolve(resCtx, *secretRef)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve DNS API key: %w", err)
	}

	// Create PowerDNS client
	client := dns.NewClient(fmt.Sprintf("http://%s:8081", dnsHost), apiKey)

	return client, nil
}

// buildSecretResolver creates a secret resolver chain that includes OpenBAO if available
func buildSecretResolver(cfg *config.Config) (*secrets.ChainResolver, *secrets.ResolutionContext, error) {
	// Try to get OpenBAO address and token
	var openBAOAddr, openBAOToken string

	addr, err := cfg.GetPrimaryOpenBAOAddress()
	if err == nil {
		openBAOAddr = fmt.Sprintf("http://%s:8200", addr)

		// Try to read OpenBAO token from keys file
		configDir, errConfig := config.GetConfigDir()
		if errConfig == nil {
			keysPath := filepath.Join(configDir, "openbao-keys", cfg.Cluster.Name, "keys.json")
			if keysData, errRead := os.ReadFile(keysPath); errRead == nil {
				var keys struct {
					RootToken string `json:"root_token"`
				}
				if errUnmarshal := json.Unmarshal(keysData, &keys); errUnmarshal == nil {
					openBAOToken = keys.RootToken
				}
			}
		}
	}

	// ResolutionContext with empty instance since we're using foundry-core as the mount
	// The mount is specified in the resolver, not in instance scoping
	resCtx := &secrets.ResolutionContext{
		Instance: "",
	}

	// If we have OpenBAO configured, add it to the resolver chain
	if openBAOAddr != "" && openBAOToken != "" {
		// Use foundry-core mount (where we enabled the KV v2 engine)
		openBAOResolver, err := secrets.NewOpenBAOResolverWithMount(openBAOAddr, openBAOToken, "foundry-core")
		if err != nil {
			// Fall back to env-only if OpenBAO setup fails
			resolver := secrets.NewChainResolver(
				secrets.NewEnvResolver(),
			)
			return resolver, resCtx, nil
		}

		resolver := secrets.NewChainResolver(
			secrets.NewEnvResolver(),
			openBAOResolver,
		)
		return resolver, resCtx, nil
	}

	// OpenBAO not available, use env resolver only
	resolver := secrets.NewChainResolver(
		secrets.NewEnvResolver(),
	)
	return resolver, resCtx, nil
}

// updateSetupState updates the setup_state and component-specific config after successful installation
func updateSetupState(cmd *cli.Command, stackConfig *config.Config, componentName string, componentConfig component.ComponentConfig) error {
	// Map component names to their setup_state fields
	switch componentName {
	case "openbao":
		stackConfig.SetupState.OpenBAOInstalled = true
		stackConfig.SetupState.OpenBAOInitialized = true // Initialized during install
	case "dns", "powerdns":
		stackConfig.SetupState.DNSInstalled = true

		// Initialize DNS config section if needed
		if stackConfig.DNS == nil {
			stackConfig.DNS = &config.DNSConfig{
				Backend:    "gsqlite3",
				Forwarders: []string{"8.8.8.8", "1.1.1.1"},
			}
		}

		// Store API key reference (points to OpenBAO secret)
		// Instance scoping will add "foundry-core/" prefix automatically
		if apiKey, ok := componentConfig["api_key"].(string); ok && apiKey != "" {
			stackConfig.DNS.APIKey = "${secret:dns:api_key}"
		}
	case "zot":
		stackConfig.SetupState.ZotInstalled = true
	case "k3s", "kubernetes":
		stackConfig.SetupState.K8sInstalled = true
	default:
		// Unknown component, don't update state
		return nil
	}

	// Save the updated config
	configPath, err := config.FindConfig(cmd.String("config"))
	if err != nil {
		return fmt.Errorf("failed to find config path: %w", err)
	}
	if err := config.Save(stackConfig, configPath); err != nil {
		return fmt.Errorf("failed to save config: %w", err)
	}

	fmt.Printf("✓ Setup state updated\n")
	return nil
}

// getTargetHostForComponent determines which host a component should be installed on
func getTargetHostForComponent(componentName string, stackConfig *config.Config) (*host.Host, error) {
	// Map component names to their host using role-based discovery
	var targetHost *host.Host
	var err error

	switch componentName {
	case "openbao":
		targetHost, err = stackConfig.GetPrimaryOpenBAOHost()
		if err != nil {
			return nil, fmt.Errorf("no host with openbao role configured: %w", err)
		}
	case "dns", "powerdns":
		targetHost, err = stackConfig.GetPrimaryDNSHost()
		if err != nil {
			return nil, fmt.Errorf("no host with dns role configured: %w", err)
		}
	case "zot":
		targetHost, err = stackConfig.GetPrimaryZotHost()
		if err != nil {
			return nil, fmt.Errorf("no host with zot role configured: %w", err)
		}
	case "k3s", "kubernetes":
		// K3s is installed on nodes defined by cluster roles
		// For now, use the first control plane node
		// TODO: Implement proper node selection for K3s
		return nil, fmt.Errorf("k3s installation not yet implemented in component install command")
	default:
		return nil, fmt.Errorf("unknown component %q - cannot determine target host", componentName)
	}

	return targetHost, nil
}

// connectToHost establishes an SSH connection to the given host
func connectToHost(h *host.Host, cfg *config.Config) (*ssh.Connection, error) {
	// Get SSH key from storage (prefers OpenBAO, falls back to filesystem)
	configDir, err := config.GetConfigDir()
	if err != nil {
		return nil, fmt.Errorf("failed to get config directory: %w", err)
	}

	keyStorage, err := ssh.GetKeyStorage(configDir, cfg.Cluster.Name)
	if err != nil {
		return nil, fmt.Errorf("failed to create key storage: %w", err)
	}

	keyPair, err := keyStorage.Load(h.Hostname)
	if err != nil {
		return nil, fmt.Errorf("SSH key not found for host %s: %w\n\nHint: Run 'foundry host sync-keys %s' to reinstall SSH keys", h.Hostname, err, h.Hostname)
	}

	// Create auth method from key pair
	authMethod, err := keyPair.AuthMethod()
	if err != nil {
		return nil, fmt.Errorf("failed to create auth method: %w", err)
	}

	// Connect to host
	connOpts := &ssh.ConnectionOptions{
		Host:       h.Address,
		Port:       h.Port,
		User:       h.User,
		AuthMethod: authMethod,
		Timeout:    30,
	}

	conn, err := ssh.Connect(connOpts)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to %s: %w", h.Hostname, err)
	}

	return conn, nil
}

// ensureDNSAPIKey generates and stores a PowerDNS API key in OpenBAO if it doesn't exist
// Returns the API key (either existing or newly generated)
func ensureDNSAPIKey(stackConfig *config.Config) (string, error) {
	// Get config directory for OpenBAO client
	configDir, err := config.GetConfigDir()
	if err != nil {
		return "", fmt.Errorf("failed to get config directory: %w", err)
	}

	// Get OpenBAO address from config
	addr, err := stackConfig.GetPrimaryOpenBAOAddress()
	if err != nil {
		return "", fmt.Errorf("OpenBAO host not configured: %w", err)
	}
	openBAOAddr := fmt.Sprintf("http://%s:8200", addr)

	// Get OpenBAO token from keys file
	keysPath := filepath.Join(configDir, "openbao-keys", stackConfig.Cluster.Name, "keys.json")
	keysData, err := os.ReadFile(keysPath)
	if err != nil {
		return "", fmt.Errorf("failed to read OpenBAO keys: %w", err)
	}

	var keys struct {
		RootToken string `json:"root_token"`
	}
	if err := json.Unmarshal(keysData, &keys); err != nil {
		return "", fmt.Errorf("failed to parse OpenBAO keys: %w", err)
	}

	// Create OpenBAO client directly
	openBAOClient := openbao.NewClient(openBAOAddr, keys.RootToken)

	// Check if API key already exists in OpenBAO at foundry-core/dns
	ctx := context.Background()
	existingData, err := openBAOClient.ReadSecretV2(ctx, "foundry-core", "dns")
	if err == nil && existingData != nil {
		if apiKeyValue, ok := existingData["api_key"].(string); ok {
			fmt.Println("  ℹ Using existing DNS API key from OpenBAO")
			return apiKeyValue, nil
		}
	}

	// Generate new API key
	fmt.Println("  Generating DNS API key...")
	apiKey, err := generateDNSAPIKey()
	if err != nil {
		return "", fmt.Errorf("failed to generate API key: %w", err)
	}

	// Store in OpenBAO at foundry-core/dns
	fmt.Println("  Storing DNS API key in OpenBAO...")
	secretData := map[string]interface{}{
		"api_key": apiKey,
	}

	if err := openBAOClient.WriteSecretV2(ctx, "foundry-core", "dns", secretData); err != nil {
		return "", fmt.Errorf("failed to store API key in OpenBAO: %w", err)
	}

	fmt.Println("  ✓ DNS API key stored in OpenBAO")
	return apiKey, nil
}

// generateDNSAPIKey generates a secure random API key for PowerDNS
func generateDNSAPIKey() (string, error) {
	bytes := make([]byte, 32) // 256 bits
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return hex.EncodeToString(bytes), nil
}

// isDependencyInstalled checks if a component dependency is installed using setup state
func isDependencyInstalled(dep string, cfg *config.Config) bool {
	// If setup state is nil, assume nothing is installed
	if cfg.SetupState == nil {
		return false
	}

	switch dep {
	case "openbao":
		return cfg.SetupState.OpenBAOInstalled
	case "dns", "powerdns":
		return cfg.SetupState.DNSInstalled
	case "zot":
		return cfg.SetupState.ZotInstalled
	case "k3s", "kubernetes":
		return cfg.SetupState.K8sInstalled
	// K8s-based components - check via Helm release status
	case "storage", "seaweedfs", "prometheus", "loki", "grafana", "external-dns", "velero",
		"gateway-api", "contour", "cert-manager":
		// First check if K3s is installed
		if !cfg.SetupState.K8sInstalled {
			return false
		}
		// Check via Helm releases directly
		return isHelmReleaseInstalled(dep)
	default:
		return false
	}
}

// isHelmReleaseInstalled checks if a Helm release is installed for a given component
func isHelmReleaseInstalled(componentName string) bool {
	// Get kubeconfig
	configDir, err := config.GetConfigDir()
	if err != nil {
		return false
	}
	kubeconfigPath := filepath.Join(configDir, "kubeconfig")
	kubeconfigBytes, err := os.ReadFile(kubeconfigPath)
	if err != nil {
		return false
	}

	// Create Helm client
	helmClient, err := helm.NewClient(kubeconfigBytes, "default")
	if err != nil {
		return false
	}
	defer helmClient.Close()

	// Map component names to release names and namespaces
	releaseInfo := map[string]struct {
		name      string
		namespace string
	}{
		"storage":      {"local-path-provisioner", "kube-system"},
		"seaweedfs":    {"seaweedfs", "seaweedfs"},
		"prometheus":   {"kube-prometheus-stack", "monitoring"},
		"loki":         {"loki", "loki"},
		"grafana":      {"grafana", "grafana"},
		"external-dns": {"external-dns", "external-dns"},
		"velero":       {"velero", "velero"},
		"gateway-api":  {"gateway-api", "gateway-system"},
		"contour":      {"contour", "projectcontour"},
		"cert-manager": {"cert-manager", "cert-manager"},
	}

	info, ok := releaseInfo[componentName]
	if !ok {
		return false
	}

	// Special case for storage - check if StorageClass exists (K3s bundles local-path)
	if componentName == "storage" {
		// Storage is always available if K3s is running (bundled local-path-provisioner)
		return true
	}

	// Check for Helm release
	releases, err := helmClient.List(context.Background(), info.namespace)
	if err != nil {
		return false
	}

	for _, rel := range releases {
		if rel.Name == info.name && rel.Status == "deployed" {
			return true
		}
	}

	return false
}

// buildExternalTargetsFromStackConfig creates Prometheus external targets for
// installed infrastructure services (OpenBAO, Zot, PowerDNS) based on stack config
func buildExternalTargetsFromStackConfig(stackConfig *config.Config) []prometheus.ExternalTarget {
	var targets []prometheus.ExternalTarget

	// Check if stack config and setup state are available
	if stackConfig == nil || stackConfig.SetupState == nil {
		return targets
	}

	// OpenBAO metrics at /v1/sys/metrics?format=prometheus on port 8200
	if stackConfig.SetupState.OpenBAOInstalled {
		if addr, err := stackConfig.GetPrimaryOpenBAOAddress(); err == nil {
			targets = append(targets, prometheus.ExternalTarget{
				Name:        "openbao",
				Targets:     []string{fmt.Sprintf("%s:8200", addr)},
				MetricsPath: "/v1/sys/metrics",
				Params: map[string][]string{
					"format": {"prometheus"},
				},
			})
		}
	}

	// Zot registry metrics at /metrics on port 5000
	if stackConfig.SetupState.ZotInstalled {
		if addr, err := stackConfig.GetPrimaryZotAddress(); err == nil {
			targets = append(targets, prometheus.ExternalTarget{
				Name:        "zot",
				Targets:     []string{fmt.Sprintf("%s:5000", addr)},
				MetricsPath: "/metrics",
			})
		}
	}

	// PowerDNS metrics (native Prometheus since v4.3.0+/v4.4.0+)
	// Auth server on port 8081, Recursor on port 8082
	if stackConfig.SetupState.DNSInstalled {
		if addr, err := stackConfig.GetPrimaryDNSAddress(); err == nil {
			// PowerDNS Authoritative Server
			targets = append(targets, prometheus.ExternalTarget{
				Name:        "powerdns-auth",
				Targets:     []string{fmt.Sprintf("%s:8081", addr)},
				MetricsPath: "/metrics",
			})
			// PowerDNS Recursor
			targets = append(targets, prometheus.ExternalTarget{
				Name:        "powerdns-recursor",
				Targets:     []string{fmt.Sprintf("%s:8082", addr)},
				MetricsPath: "/metrics",
			})
		}
	}

	return targets
}
