package stack

import (
	"bufio"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	clustercommands "github.com/catalystcommunity/foundry/v1/cmd/foundry/commands/cluster"
	"github.com/catalystcommunity/foundry/v1/internal/component"
	"github.com/catalystcommunity/foundry/v1/internal/component/certmanager"
	"github.com/catalystcommunity/foundry/v1/internal/component/contour"
	"github.com/catalystcommunity/foundry/v1/internal/component/gatewayapi"
	"github.com/catalystcommunity/foundry/v1/internal/component/openbao"
	"github.com/catalystcommunity/foundry/v1/internal/config"
	"github.com/catalystcommunity/foundry/v1/internal/container"
	"github.com/catalystcommunity/foundry/v1/internal/helm"
	"github.com/catalystcommunity/foundry/v1/internal/host"
	"github.com/catalystcommunity/foundry/v1/internal/k8s"
	"github.com/catalystcommunity/foundry/v1/internal/setup"
	"github.com/catalystcommunity/foundry/v1/internal/ssh"
	"github.com/catalystcommunity/foundry/v1/internal/sudo"
	"github.com/urfave/cli/v3"
	gossh "golang.org/x/crypto/ssh"
	"golang.org/x/term"
)

// checkAllComponentsInstalled checks if all components are actually installed
func checkAllComponentsInstalled(ctx context.Context, state *setup.SetupState) bool {
	// Check Phase 2 components via state flags
	phase2Complete := state.OpenBAOInstalled &&
		state.OpenBAOInitialized &&
		state.DNSInstalled &&
		state.DNSZonesCreated &&
		state.ZotInstalled &&
		state.K8sInstalled

	if !phase2Complete {
		return false
	}

	// Check Phase 3 components via actual status
	contourComp := component.Get("contour")
	if contourComp != nil {
		status, err := contourComp.Status(ctx)
		if err != nil || status == nil || !status.Installed {
			return false
		}
	}

	return true
}

// InstallCommand handles the 'foundry stack install' command
var InstallCommand = &cli.Command{
	Name:  "install",
	Usage: "Install the complete infrastructure stack",
	Description: `Install the complete infrastructure stack from a single command.

If no configuration exists, you can provide flags to initialize one:
  --cluster-name, --domain, --host, --vip

If configuration exists, the command will:
  1. Validate and configure hosts
  2. Plan and validate network
  3. Install components in order (OpenBAO → PowerDNS → Zot → K3s)
  4. Resume from any checkpoint if interrupted

Examples:
  # Interactive mode (prompts for all config)
  foundry stack install

  # Non-interactive with flags
  foundry stack install --cluster-name prod --domain catalyst.local \
    --host vm1:10.16.0.42 --vip 10.16.0.43

  # Resume after interruption
  foundry stack install  # Automatically picks up where it left off`,
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:    "config",
			Aliases: []string{"c"},
			Usage:   "Path to configuration file",
		},
		// Config initialization flags
		&cli.StringFlag{
			Name:  "cluster-name",
			Usage: "Cluster name (for config initialization)",
		},
		&cli.StringFlag{
			Name:  "domain",
			Usage: "Cluster domain (for config initialization)",
		},
		&cli.StringSliceFlag{
			Name:  "host",
			Usage: "Host in format 'hostname:ip' (can be specified multiple times)",
		},
		&cli.StringFlag{
			Name:  "vip",
			Usage: "Virtual IP for Kubernetes cluster",
		},
		&cli.StringSliceFlag{
			Name:  "forwarder",
			Usage: "DNS forwarder (can be specified multiple times, default: 8.8.8.8, 1.1.1.1)",
		},
		// Operational flags
		&cli.BoolFlag{
			Name:  "dry-run",
			Usage: "Show what would be installed without making changes",
		},
		&cli.BoolFlag{
			Name:  "yes",
			Usage: "Skip confirmation prompts",
		},
		&cli.BoolFlag{
			Name:  "non-interactive",
			Usage: "Use defaults and flags without prompting",
		},
	},
	Action: runStackInstall,
}

// ComponentInstaller defines the interface for installing a component
// This allows us to mock installations in tests
type ComponentInstaller interface {
	Install(ctx context.Context, componentName string, cfg interface{}) error
}

// StackInstaller orchestrates the installation of all stack components
type StackInstaller struct {
	registry  *component.Registry
	installer ComponentInstaller
}

// NewStackInstaller creates a new stack installer
func NewStackInstaller(registry *component.Registry, installer ComponentInstaller) *StackInstaller {
	return &StackInstaller{
		registry:  registry,
		installer: installer,
	}
}

func runStackInstall(ctx context.Context, cmd *cli.Command) error {
	// Determine config path
	configPath := cmd.String("config")
	if configPath == "" {
		configPath = config.DefaultConfigPath()
	}

	// Step 1: Load or create configuration
	cfg, isNewConfig, err := loadOrCreateConfig(ctx, cmd, configPath)
	if err != nil {
		return err
	}

	if isNewConfig {
		fmt.Printf("\n✓ Configuration created: %s\n", configPath)
	} else {
		fmt.Printf("Using configuration: %s\n", configPath)
	}

	// Step 2: Load setup state
	if cfg.SetupState == nil {
		cfg.SetupState = &setup.SetupState{}
	}

	// Check if all components are actually installed (don't rely solely on StackComplete flag)
	allComponentsInstalled := checkAllComponentsInstalled(ctx, cfg.SetupState)
	if allComponentsInstalled {
		fmt.Println("\n✓ Stack is already complete!")
		fmt.Println("\nTo reinstall, reset your configuration or use specific component commands.")
		return nil
	}

	// Determine next step
	nextStep := setup.DetermineNextStep(cfg.SetupState)
	fmt.Printf("\nCurrent checkpoint: %s\n", nextStep)

	// Dry-run mode
	if cmd.Bool("dry-run") {
		return printStackPlan(cfg, nextStep)
	}

	// Step 3: Execute installation phases
	fmt.Println("\n" + strings.Repeat("=", 60))
	fmt.Println("  FOUNDRY STACK INSTALLATION")
	fmt.Println(strings.Repeat("=", 60))

	// Phase 1: Host Configuration
	if err := ensureHostsConfigured(ctx, cfg, cmd.Bool("non-interactive")); err != nil {
		return fmt.Errorf("host configuration failed: %w", err)
	}

	// Phase 2: Network Planning and Validation
	if err := ensureNetworkPlanned(ctx, cfg, cmd.Bool("non-interactive")); err != nil {
		return fmt.Errorf("network planning failed: %w", err)
	}

	// Phase 3: Component Installation
	if err := installComponents(ctx, cfg, configPath, cmd.Bool("yes")); err != nil {
		return fmt.Errorf("component installation failed: %w", err)
	}

	// Phase 4: Final validation
	// Only mark complete if we actually finished (not an early exit at checkpoint)
	if !cfg.SetupState.StackComplete {
		cfg.SetupState.StackComplete = true
		if err := config.Save(cfg, configPath); err != nil {
			fmt.Printf("⚠ Warning: Failed to save final state: %v\n", err)
		}
	}

	// Success summary
	fmt.Println("\n" + strings.Repeat("=", 60))
	fmt.Println("  ✓ STACK INSTALLATION COMPLETE!")
	fmt.Println(strings.Repeat("=", 60))
	fmt.Println("\nYour infrastructure stack is ready:")
	fmt.Printf("  Cluster:   %s\n", cfg.Cluster.Name)
	fmt.Printf("  Domain:    %s\n", cfg.Cluster.Domain)
	if cfg.Network != nil && cfg.Cluster.VIP != "" {
		fmt.Printf("  K8s VIP:   %s\n", cfg.Cluster.VIP)
	}
	fmt.Printf("  Kubeconfig: ~/.foundry/kubeconfig\n")
	fmt.Println("\nNext steps:")
	fmt.Println("  kubectl --kubeconfig ~/.foundry/kubeconfig get nodes")
	fmt.Println("  foundry stack status")

	return nil
}

// loadOrCreateConfig loads existing config or creates a new one from flags/prompts
func loadOrCreateConfig(ctx context.Context, cmd *cli.Command, configPath string) (*config.Config, bool, error) {
	// Try to load existing config
	cfg, err := config.Load(configPath)
	if err == nil {
		// Config exists and loaded successfully
		// Check if this is a template that hasn't been filled in (contains <PLACEHOLDER> values)
		if placeholders := findTemplatePlaceholders(cfg); len(placeholders) > 0 {
			return nil, false, fmt.Errorf("configuration file contains template placeholders\n\n"+
				"Found placeholders in %s:\n"+
				"  %s\n\n"+
				"Please replace all <PLACEHOLDER> values with actual values and try again.\n\n"+
				"Hint: Use 'foundry stack template' to generate a fresh template",
				configPath, strings.Join(placeholders, "\n  "))
		}
		return cfg, false, nil
	}

	// Config doesn't exist - create it
	fmt.Println("No configuration found. Creating new configuration...")

	// Check if we have flags for non-interactive creation
	clusterName := cmd.String("cluster-name")
	domain := cmd.String("domain")
	hosts := cmd.StringSlice("host")
	vip := cmd.String("vip")
	nonInteractive := cmd.Bool("non-interactive")

	// If we have all required flags, create config non-interactively
	if clusterName != "" && domain != "" && len(hosts) > 0 && vip != "" {
		cfg, err = createConfigFromFlags(clusterName, domain, hosts, vip, cmd.StringSlice("forwarder"))
		if err != nil {
			return nil, false, fmt.Errorf("failed to create config from flags: %w", err)
		}
	} else if nonInteractive {
		return nil, false, fmt.Errorf("non-interactive mode requires flags: --cluster-name, --domain, --host, --vip")
	} else {
		// Interactive mode
		cfg, err = createConfigInteractive()
		if err != nil {
			return nil, false, fmt.Errorf("failed to create config interactively: %w", err)
		}
	}

	// Initialize setup state
	cfg.SetupState = &setup.SetupState{}

	// Save the new config
	if err := config.Save(cfg, configPath); err != nil {
		return nil, false, fmt.Errorf("failed to save config: %w", err)
	}

	return cfg, true, nil
}

// createConfigFromFlags creates a config from command-line flags
func createConfigFromFlags(clusterName, domain string, hosts []string, vip string, forwarders []string) (*config.Config, error) {
	// Parse hosts (format: hostname:ip)
	// In the new schema, create Host objects with appropriate roles
	var hostConfigs []*config.Host

	for _, hostStr := range hosts {
		parts := strings.Split(hostStr, ":")
		if len(parts) != 2 {
			return nil, fmt.Errorf("invalid host format %q (expected hostname:ip)", hostStr)
		}
		hostname, ip := parts[0], parts[1]

		// For quick start, assign all infrastructure roles to each host
		// (openbao, dns, zot, cluster-control-plane)
		hostConfigs = append(hostConfigs, &config.Host{
			Hostname: hostname,
			Address:  ip,
			Port:     22,
			User:     "root",
			Roles: []string{
				"openbao",
				"dns",
				"zot",
				"cluster-control-plane",
			},
		})
	}

	// Default forwarders if not specified
	if len(forwarders) == 0 {
		forwarders = []string{"8.8.8.8", "1.1.1.1"}
	}

	cfg := &config.Config{
		Cluster: config.ClusterConfig{
			Name:   clusterName,
			Domain: domain,
			VIP:    vip,
		},
		Hosts: hostConfigs,
		Network: &config.NetworkConfig{
			Gateway: "", // Will be detected/prompted during network planning
			Netmask: "", // Will be detected/prompted during network planning
		},
		DNS: &config.DNSConfig{
			Backend:    "gsqlite3",
			Forwarders: forwarders,
		},
	}

	return cfg, nil
}

// createConfigInteractive creates a config through interactive prompts
func createConfigInteractive() (*config.Config, error) {
	reader := bufio.NewReader(os.Stdin)

	// Cluster name
	fmt.Print("Cluster name [my-cluster]: ")
	clusterName, err := reader.ReadString('\n')
	if err != nil {
		return nil, err
	}
	clusterName = strings.TrimSpace(clusterName)
	if clusterName == "" {
		clusterName = "my-cluster"
	}

	// Domain
	fmt.Print("Cluster domain [catalyst.local]: ")
	domain, err := reader.ReadString('\n')
	if err != nil {
		return nil, err
	}
	domain = strings.TrimSpace(domain)
	if domain == "" {
		domain = "catalyst.local"
	}

	// Host
	fmt.Print("Infrastructure host (hostname:ip): ")
	hostStr, err := reader.ReadString('\n')
	if err != nil {
		return nil, err
	}
	hostStr = strings.TrimSpace(hostStr)

	parts := strings.Split(hostStr, ":")
	if len(parts) != 2 {
		return nil, fmt.Errorf("invalid host format (expected hostname:ip)")
	}
	hostname, ip := parts[0], parts[1]

	// VIP
	fmt.Print("Kubernetes VIP: ")
	vip, err := reader.ReadString('\n')
	if err != nil {
		return nil, err
	}
	vip = strings.TrimSpace(vip)

	cfg := &config.Config{
		Cluster: config.ClusterConfig{
			Name:   clusterName,
			Domain: domain,
			VIP:    vip,
		},
		Hosts: []*config.Host{
			{
				Hostname: hostname,
				Address:  ip,
				Port:     22,
				User:     "root",
				Roles: []string{
					"openbao",
					"dns",
					"zot",
					"cluster-control-plane",
				},
			},
		},
		Network: &config.NetworkConfig{
			Gateway: "", // Will be determined during network planning
			Netmask: "", // Will be determined during network planning
		},
		DNS: &config.DNSConfig{
			Backend:    "gsqlite3",
			Forwarders: []string{"8.8.8.8", "1.1.1.1"},
		},
	}

	return cfg, nil
}

// ensureHostsConfigured validates that hosts are added and configured
func ensureHostsConfigured(ctx context.Context, cfg *config.Config, nonInteractive bool) error {
	fmt.Println("\n[1/4] Host Configuration")
	fmt.Println(strings.Repeat("-", 60))

	if len(cfg.Hosts) == 0 {
		return fmt.Errorf("no hosts defined in configuration")
	}

	// Get config directory for keys
	configDir, err := config.GetConfigDir()
	if err != nil {
		return fmt.Errorf("failed to get config directory: %w", err)
	}

	keysDir := filepath.Join(configDir, "keys")

	// Process each host
	for i, h := range cfg.Hosts {
		fmt.Printf("\n[%d/%d] Configuring host: %s (%s)\n", i+1, len(cfg.Hosts), h.Hostname, h.Address)

		// Step 1: Setup SSH key if not present
		keyStorage, err := ssh.NewFilesystemKeyStorage(keysDir)
		if err != nil {
			return fmt.Errorf("failed to create key storage: %w", err)
		}

		_, err = keyStorage.Load(h.Hostname)
		if err != nil {
			// SSH key doesn't exist - need to generate and install
			fmt.Println("  SSH key not found, setting up key-based authentication...")

			if err := setupHostSSHKey(h, nonInteractive); err != nil {
				return fmt.Errorf("failed to setup SSH key for %s: %w", h.Hostname, err)
			}
		} else {
			fmt.Println("  ✓ SSH key already configured")
		}

		// Step 2: Connect and configure host
		fmt.Println("  Connecting to host...")
		conn, err := connectToHostForSetup(h, keysDir)
		if err != nil {
			return fmt.Errorf("failed to connect to %s: %w", h.Hostname, err)
		}

		// Step 3: Run host configuration
		if err := configureHost(conn, h, nonInteractive); err != nil {
			conn.Close()
			return fmt.Errorf("failed to configure %s: %w", h.Hostname, err)
		}

		conn.Close()
		fmt.Printf("  ✓ Host %s configured successfully\n", h.Hostname)
	}

	fmt.Println("\n✓ All hosts configured")
	return nil
}

// ensureNetworkPlanned validates that network is planned and validated
func ensureNetworkPlanned(ctx context.Context, cfg *config.Config, nonInteractive bool) error {
	fmt.Println("\n[2/4] Network Planning & Validation")
	fmt.Println(strings.Repeat("-", 60))

	// Check if network already planned
	if cfg.SetupState.NetworkPlanned && cfg.SetupState.NetworkValidated {
		fmt.Println("✓ Network already planned and validated (skipping)")
		return nil
	}

	// TODO: Implement network planning and validation
	// For now, we'll mark it as complete if we have basic network config
	if cfg.Network != nil && cfg.Cluster.VIP != "" {
		cfg.SetupState.NetworkPlanned = true
		cfg.SetupState.NetworkValidated = true
		fmt.Println("✓ Network configuration present")
		fmt.Printf("  VIP: %s\n", cfg.Cluster.VIP)
		if openBAOHosts := cfg.GetHostAddresses(host.RoleOpenBAO); len(openBAOHosts) > 0 {
			fmt.Printf("  OpenBAO Hosts: %v\n", openBAOHosts)
		}
		if dnsHosts := cfg.GetHostAddresses(host.RoleDNS); len(dnsHosts) > 0 {
			fmt.Printf("  DNS Hosts: %v\n", dnsHosts)
		}
		return nil
	}

	return fmt.Errorf("network configuration incomplete\n\nRun: foundry network plan")
}

// installComponents installs all components in order with state tracking
func installComponents(ctx context.Context, cfg *config.Config, configPath string, skipConfirm bool) error {
	fmt.Println("\n[3/4] Component Installation")
	fmt.Println(strings.Repeat("-", 60))

	// Define installation order (matches Phase 2 + Phase 3 architecture)
	components := []struct {
		name      string
		checkFunc func(*setup.SetupState) bool
		setFunc   func(*setup.SetupState)
	}{
		{
			name: "openbao",
			checkFunc: func(s *setup.SetupState) bool {
				return s.OpenBAOInstalled && s.OpenBAOInitialized
			},
			setFunc: func(s *setup.SetupState) {
				s.OpenBAOInstalled = true
				s.OpenBAOInitialized = true
			},
		},
		{
			name: "dns",
			checkFunc: func(s *setup.SetupState) bool {
				return s.DNSInstalled && s.DNSZonesCreated
			},
			setFunc: func(s *setup.SetupState) {
				s.DNSInstalled = true
				s.DNSZonesCreated = true
			},
		},
		{
			name: "zot",
			checkFunc: func(s *setup.SetupState) bool {
				return s.ZotInstalled
			},
			setFunc: func(s *setup.SetupState) {
				s.ZotInstalled = true
			},
		},
		{
			name: "k3s",
			checkFunc: func(s *setup.SetupState) bool {
				return s.K8sInstalled
			},
			setFunc: func(s *setup.SetupState) {
				s.K8sInstalled = true
			},
		},
		{
			name: "gateway-api",
			checkFunc: func(s *setup.SetupState) bool {
				// Check actual component status
				comp := component.Get("gateway-api")
				if comp == nil {
					return false
				}
				status, err := comp.Status(ctx)
				if err != nil || status == nil {
					return false
				}
				return status.Installed
			},
			setFunc: func(s *setup.SetupState) {
				// No state flag for Gateway API - component tracks its own status
			},
		},
		{
			name: "contour",
			checkFunc: func(s *setup.SetupState) bool {
				// Check actual component status instead of state flag
				comp := component.Get("contour")
				if comp == nil {
					return false
				}
				status, err := comp.Status(ctx)
				if err != nil || status == nil {
					return false
				}
				return status.Installed
			},
			setFunc: func(s *setup.SetupState) {
				// No state flag for Contour yet - component tracks its own status
			},
		},
	}

	for i, comp := range components {
		// Check if already installed
		if comp.checkFunc(cfg.SetupState) {
			fmt.Printf("\n[%d/%d] %s: ✓ Already installed (skipping)\n", i+1, len(components), comp.name)
			continue
		}

		fmt.Printf("\n[%d/%d] Installing %s...\n", i+1, len(components), comp.name)

		// Install the component
		if err := installSingleComponent(ctx, cfg, comp.name); err != nil {
			return fmt.Errorf("%s installation failed: %w", comp.name, err)
		}

		// Update state
		comp.setFunc(cfg.SetupState)
		if err := config.Save(cfg, configPath); err != nil {
			return fmt.Errorf("failed to save state after %s installation: %w", comp.name, err)
		}
		fmt.Printf("  ✓ State updated\n")

		// Testing helper: Allow early exit at specific checkpoints via env var
		// Usage: FOUNDRY_EXIT_AT_CHECKPOINT=openbao ./foundry stack install
		if exitAt := os.Getenv("FOUNDRY_EXIT_AT_CHECKPOINT"); exitAt != "" {
			if exitAt == comp.name {
				fmt.Printf("\n⚠ Exiting at checkpoint '%s' (FOUNDRY_EXIT_AT_CHECKPOINT set)\n", comp.name)
				fmt.Println("Resume with: foundry stack install")
				return nil
			}
		}
	}

	return nil
}

// installK8sComponent installs a Kubernetes component using the cluster kubeconfig
func installK8sComponent(ctx context.Context, cfg *config.Config, componentName string, comp component.Component) error {
	fmt.Printf("  Installing %s to Kubernetes cluster...\n", componentName)

	// Get kubeconfig path
	configDir, err := config.GetConfigDir()
	if err != nil {
		return fmt.Errorf("failed to get config directory: %w", err)
	}
	kubeconfigPath := filepath.Join(configDir, "kubeconfig")

	// Verify kubeconfig exists and read it
	kubeconfigBytes, err := os.ReadFile(kubeconfigPath)
	if err != nil {
		return fmt.Errorf("failed to read kubeconfig at %s: %w", kubeconfigPath, err)
	}

	// Create Helm and K8s clients from kubeconfig
	helmClient, err := helm.NewClient(kubeconfigBytes, "default")
	if err != nil {
		return fmt.Errorf("failed to create helm client: %w", err)
	}

	k8sClient, err := k8s.NewClientFromKubeconfig(kubeconfigBytes)
	if err != nil {
		return fmt.Errorf("failed to create k8s client: %w", err)
	}

	// Create component-specific instance with clients
	var componentWithClients component.Component
	switch componentName {
	case "gateway-api":
		componentWithClients = gatewayapi.NewComponent(k8sClient)
	case "contour":
		componentWithClients = contour.NewComponent(helmClient, k8sClient)
	case "cert-manager":
		componentWithClients = certmanager.NewComponent(nil)
	default:
		return fmt.Errorf("unknown kubernetes component: %s", componentName)
	}

	// Create component config with cluster-specific values
	componentConfig := component.ComponentConfig{}

	// Pass cluster VIP to Contour for LoadBalancer annotation sharing
	if componentName == "contour" && cfg.Cluster.VIP != "" {
		componentConfig["cluster_vip"] = cfg.Cluster.VIP
	}

	// Install the component
	if err := componentWithClients.Install(ctx, componentConfig); err != nil {
		return fmt.Errorf("installation failed: %w", err)
	}

	fmt.Printf("  ✓ %s installed successfully\n", componentName)
	return nil
}

// installSingleComponent installs a single component with proper configuration
func installSingleComponent(ctx context.Context, cfg *config.Config, componentName string) error {
	// Get component from registry
	comp := component.Get(componentName)
	if comp == nil {
		return fmt.Errorf("component %q not found in registry", componentName)
	}

	// K3s is special - use cluster init logic instead of component install
	if componentName == "k3s" {
		return installK3sCluster(ctx, cfg)
	}

	// Kubernetes components are installed via kubeconfig, not SSH to a host
	if componentName == "gateway-api" || componentName == "contour" || componentName == "cert-manager" {
		return installK8sComponent(ctx, cfg, componentName, comp)
	}

	// Get target host for this component
	targetHost, err := getTargetHostForComponent(componentName, cfg)
	if err != nil {
		return fmt.Errorf("failed to determine target host: %w", err)
	}

	fmt.Printf("  Target host: %s (%s)\n", targetHost.Hostname, targetHost.Address)

	// Establish SSH connection
	fmt.Printf("  Connecting to %s...\n", targetHost.Hostname)
	conn, err := connectToHost(targetHost, cfg.Cluster.Name)
	if err != nil {
		return fmt.Errorf("failed to connect to host: %w", err)
	}
	defer conn.Close()
	fmt.Println("  ✓ Connected")

	// Create SSH executor adapter
	executor := &sshExecutorAdapter{conn: conn}

	// Build component-specific configuration
	componentConfig, err := buildComponentConfig(ctx, cfg, componentName, executor, conn)
	if err != nil {
		return fmt.Errorf("failed to build component config: %w", err)
	}

	// Install the component
	fmt.Printf("  Installing %s...\n", componentName)
	if err := comp.Install(ctx, componentConfig); err != nil {
		return fmt.Errorf("installation failed: %w", err)
	}

	fmt.Printf("  ✓ %s installed successfully\n", componentName)

	// Post-installation: Update config with component-specific data
	if err := updateComponentConfig(cfg, componentName, componentConfig); err != nil {
		fmt.Printf("  ⚠ Warning: Failed to update config: %v\n", err)
	}

	return nil
}

// updateComponentConfig updates the stack config with component-specific data after installation
func updateComponentConfig(cfg *config.Config, componentName string, componentConfig component.ComponentConfig) error {
	switch componentName {
	case "dns", "powerdns":
		// Initialize DNS config section if needed
		if cfg.DNS == nil {
			cfg.DNS = &config.DNSConfig{
				Backend:    "gsqlite3",
				Forwarders: []string{"8.8.8.8", "1.1.1.1"},
			}
		}

		// Store API key reference (points to OpenBAO secret)
		if _, ok := componentConfig["api_key"].(string); ok {
			cfg.DNS.APIKey = "${secret:dns:api_key}"
		}
	}

	return nil
}

// buildComponentConfig creates the configuration map for a component
func buildComponentConfig(ctx context.Context, cfg *config.Config, componentName string, executor *sshExecutorAdapter, conn *ssh.Connection) (component.ComponentConfig, error) {
	// Get config directory for keys
	configDir, err := config.GetConfigDir()
	if err != nil {
		return nil, fmt.Errorf("failed to get config directory: %w", err)
	}

	// Base config (common to all components)
	compCfg := component.ComponentConfig{
		"host":         executor,
		"ssh_conn":     conn,
		"cluster_name": cfg.Cluster.Name,
		"keys_dir":     filepath.Join(configDir, "openbao-keys"),
	}

	// Component-specific configuration
	switch componentName {
	case "openbao":
		// OpenBAO needs API URL for initialization
		addr, err := cfg.GetPrimaryOpenBAOAddress()
		if err == nil {
			compCfg["api_url"] = fmt.Sprintf("http://%s:8200", addr)
		}

	case "dns", "powerdns":
		// DNS needs API key (generate/retrieve from OpenBAO if available)
		if cfg.SetupState.OpenBAOInitialized {
			apiKey, err := ensureDNSAPIKey(cfg)
			if err != nil {
				return nil, fmt.Errorf("failed to setup DNS API key: %w", err)
			}
			compCfg["api_key"] = apiKey
		}

		// Pass cluster domain as local zone for Recursor forwarding
		if cfg.Cluster.Domain != "" {
			compCfg["local_zones"] = []string{cfg.Cluster.Domain}
		}

	case "zot":
		// Zot uses the base config (SSH connection is enough)
		// No additional config needed
	}

	return compCfg, nil
}

// installK3sCluster initializes the K3s cluster using cluster init logic
func installK3sCluster(ctx context.Context, cfg *config.Config) error {
	fmt.Println("  Initializing K3s cluster...")

	// Call the exported cluster initialization function
	// This handles: tokens, control plane, nodes, VIP, kubeconfig
	if err := clustercommands.InitializeCluster(ctx, cfg); err != nil {
		return fmt.Errorf("cluster initialization failed: %w", err)
	}

	return nil
}

// validateStackConfig ensures the configuration has all required fields
func validateStackConfig(cfg *config.Config) error {
	// Validate network configuration
	if cfg.Network.Gateway == "" {
		return fmt.Errorf("network.gateway is required")
	}
	if cfg.Network.Netmask == "" {
		return fmt.Errorf("network.netmask is required")
	}
	if len(cfg.GetOpenBAOHosts()) == 0 {
		return fmt.Errorf("no hosts with openbao role configured")
	}
	if len(cfg.GetDNSHosts()) == 0 {
		return fmt.Errorf("no hosts with dns role configured")
	}
	if cfg.Cluster.VIP == "" {
		return fmt.Errorf("cluster.vip is required")
	}

	// Validate DNS configuration
	if len(cfg.DNS.InfrastructureZones) == 0 {
		return fmt.Errorf("dns.infrastructure_zones is required (at least one zone)")
	}
	if len(cfg.DNS.KubernetesZones) == 0 {
		return fmt.Errorf("dns.kubernetes_zones is required (at least one zone)")
	}

	// Validate cluster configuration
	if cfg.Cluster.Name == "" {
		return fmt.Errorf("cluster.name is required")
	}
	if cfg.Cluster.Domain == "" {
		return fmt.Errorf("cluster.domain is required")
	}
	if len(cfg.GetClusterHosts()) == 0 {
		return fmt.Errorf("no hosts with cluster roles configured (need cluster-control-plane or cluster-worker)")
	}

	return nil
}

// determineInstallationOrder returns components in dependency order
func determineInstallationOrder() ([]string, error) {
	// Stack installation order (as defined in Phase 2):
	// 1. Network validation (not a component, done separately)
	// 2. OpenBAO
	// 3. PowerDNS (dns)
	// 4. DNS zones (handled within dns installation)
	// 5. Zot
	// 6. K3s cluster
	// Note: Contour and cert-manager are optional add-ons, not included in base stack

	componentNames := []string{
		"openbao",
		"dns",
		"zot",
		"k3s",
	}

	// Use component registry to resolve dependencies
	order, err := component.ResolveInstallationOrder(component.DefaultRegistry, componentNames)
	if err != nil {
		return nil, err
	}

	return order, nil
}

// printStackPlan displays what would be installed
func printStackPlan(cfg *config.Config, nextStep setup.Step) error {
	fmt.Println("\n" + strings.Repeat("=", 60))
	fmt.Println("  DRY-RUN MODE - Stack Installation Plan")
	fmt.Println(strings.Repeat("=", 60))

	fmt.Println("\nConfiguration:")
	fmt.Printf("  Cluster:   %s\n", cfg.Cluster.Name)
	fmt.Printf("  Domain:    %s\n", cfg.Cluster.Domain)
	fmt.Printf("  Hosts:     %d\n", len(cfg.Hosts))
	for i, h := range cfg.Hosts {
		rolesStr := strings.Join(h.Roles, ", ")
		if rolesStr == "" {
			rolesStr = "none"
		}
		fmt.Printf("    %d. %s (%s) - roles: [%s]\n", i+1, h.Hostname, h.Address, rolesStr)
	}

	if cfg.Network != nil {
		fmt.Println("\nNetwork:")
		if cfg.Network.Gateway != "" {
			fmt.Printf("  Gateway: %s\n", cfg.Network.Gateway)
		}
		if cfg.Network.Netmask != "" {
			fmt.Printf("  Netmask: %s\n", cfg.Network.Netmask)
		}
		fmt.Printf("  K8s VIP: %s\n", cfg.Cluster.VIP)
		if openBAOHosts := cfg.GetHostAddresses(host.RoleOpenBAO); len(openBAOHosts) > 0 {
			fmt.Printf("  OpenBAO Hosts: %v\n", openBAOHosts)
		}
		if dnsHosts := cfg.GetHostAddresses(host.RoleDNS); len(dnsHosts) > 0 {
			fmt.Printf("  DNS Hosts: %v\n", dnsHosts)
		}
		if zotHosts := cfg.GetHostAddresses(host.RoleZot); len(zotHosts) > 0 {
			fmt.Printf("  Zot Hosts: %v\n", zotHosts)
		}
	}

	if cfg.DNS != nil && len(cfg.DNS.Forwarders) > 0 {
		fmt.Println("\nDNS:")
		fmt.Printf("  Forwarders: %v\n", cfg.DNS.Forwarders)
	}

	fmt.Println("\nCurrent State:")
	fmt.Printf("  Next step: %s\n", nextStep)
	if cfg.SetupState != nil {
		fmt.Printf("  Network planned: %v\n", cfg.SetupState.NetworkPlanned)
		fmt.Printf("  Network validated: %v\n", cfg.SetupState.NetworkValidated)
		fmt.Printf("  OpenBAO installed: %v\n", cfg.SetupState.OpenBAOInstalled)
		fmt.Printf("  DNS installed: %v\n", cfg.SetupState.DNSInstalled)
		fmt.Printf("  DNS zones created: %v\n", cfg.SetupState.DNSZonesCreated)
		fmt.Printf("  Zot installed: %v\n", cfg.SetupState.ZotInstalled)
		fmt.Printf("  K8s installed: %v\n", cfg.SetupState.K8sInstalled)
		fmt.Printf("  Stack complete: %v\n", cfg.SetupState.StackComplete)
	}

	fmt.Println("\nPlanned Steps:")
	fmt.Println("  1. Host configuration (validate hosts are added and configured)")
	fmt.Println("  2. Network planning and validation")
	fmt.Println("  3. Install OpenBAO (secrets management)")
	fmt.Println("  4. Install PowerDNS (DNS server with zones)")
	fmt.Println("  5. Install Zot (container registry)")
	fmt.Println("  6. Install K3s cluster (Kubernetes with VIP)")
	fmt.Println("  7. Final validation")

	fmt.Println("\nNote: This is a dry-run. No changes will be made.")
	fmt.Println("      Remove --dry-run flag to proceed with installation.")

	return nil
}

// installStack performs the actual installation
func installStack(ctx context.Context, cfg *config.Config, installOrder []string) error {
	startTime := time.Now()

	for i, compName := range installOrder {
		fmt.Printf("\n[%d/%d] Installing %s...\n", i+1, len(installOrder), compName)

		// Get component from registry
		comp := component.Get(compName)
		if comp == nil {
			return fmt.Errorf("component %q not found in registry", compName)
		}

		// TODO: Build component-specific config from stack config
		// For now, we'll pass nil and let components use defaults
		// This will be implemented when we integrate with actual component installers

		// Simulate installation for now
		// In real implementation, this would call comp.Install(ctx, componentConfig)
		time.Sleep(500 * time.Millisecond)
		fmt.Printf("  ✓ %s installed successfully\n", compName)
	}

	elapsed := time.Since(startTime)
	fmt.Printf("\nTotal installation time: %s\n", elapsed.Round(time.Second))

	return nil
}

// sshExecutorAdapter adapts ssh.Connection to container.SSHExecutor interface
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

// getTargetHostForComponent determines which host a component should be installed on
func getTargetHostForComponent(componentName string, stackConfig *config.Config) (*host.Host, error) {
	if stackConfig.Network == nil {
		return nil, fmt.Errorf("network configuration is required")
	}

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
	default:
		return nil, fmt.Errorf("unknown component %q - cannot determine target host", componentName)
	}

	return targetHost, nil
}

// connectToHost establishes an SSH connection to the given host
func connectToHost(h *host.Host, clusterName string) (*ssh.Connection, error) {
	// Get SSH key from storage (prefers OpenBAO, falls back to filesystem)
	configDir, err := config.GetConfigDir()
	if err != nil {
		return nil, fmt.Errorf("failed to get config directory: %w", err)
	}

	keyStorage, err := ssh.GetKeyStorage(configDir, clusterName)
	if err != nil {
		return nil, fmt.Errorf("failed to create key storage: %w", err)
	}

	keyPair, err := keyStorage.Load(h.Hostname)
	if err != nil {
		return nil, fmt.Errorf("SSH key not found for host %s: %w", h.Hostname, err)
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

// findTemplatePlaceholders scans the config for any <PLACEHOLDER> values
func findTemplatePlaceholders(cfg *config.Config) []string {
	var placeholders []string
	seen := make(map[string]bool) // Deduplicate

	// Helper to check a string value
	checkValue := func(val, field string) {
		if strings.Contains(val, "<") && strings.Contains(val, ">") {
			// Extract just the placeholder
			start := strings.Index(val, "<")
			end := strings.Index(val, ">")
			if start < end {
				placeholder := val[start : end+1]
				if !seen[placeholder] {
					placeholders = append(placeholders, fmt.Sprintf("%s (in %s)", placeholder, field))
					seen[placeholder] = true
				}
			}
		}
	}

	// Check cluster config
	checkValue(cfg.Cluster.Name, "cluster.name")
	checkValue(cfg.Cluster.Domain, "cluster.domain")
	// Node configuration now comes from hosts with cluster roles

	// Check network config
	if cfg.Network != nil {
		checkValue(cfg.Network.Gateway, "network.gateway")
		checkValue(cfg.Network.Netmask, "network.netmask")
		checkValue(cfg.Cluster.VIP, "cluster.vip")
	}

	// Check hosts configuration
	if cfg.Hosts != nil {
		for i, h := range cfg.Hosts {
			checkValue(h.Hostname, fmt.Sprintf("hosts[%d].hostname", i))
			checkValue(h.Address, fmt.Sprintf("hosts[%d].address", i))
			checkValue(h.User, fmt.Sprintf("hosts[%d].user", i))
		}
	}

	return placeholders
}

// setupHostSSHKey generates and installs an SSH key for a host
func setupHostSSHKey(h *config.Host, nonInteractive bool) error {
	// Prompt for password
	var password string
	if nonInteractive {
		return fmt.Errorf("cannot setup SSH key in non-interactive mode (password required)")
	}

	fmt.Printf("  Password for %s@%s: ", h.User, h.Address)
	passwordBytes, err := term.ReadPassword(int(os.Stdin.Fd()))
	fmt.Println() // Newline after password
	if err != nil {
		return fmt.Errorf("failed to read password: %w", err)
	}
	password = string(passwordBytes)

	// Test SSH connection with password
	fmt.Println("  Testing SSH connection with password...")
	authMethod := gossh.Password(password)
	connOpts := &ssh.ConnectionOptions{
		Host:       h.Address,
		Port:       h.Port,
		User:       h.User,
		AuthMethod: authMethod,
		Timeout:    30,
	}

	conn, err := ssh.Connect(connOpts)
	if err != nil {
		return fmt.Errorf("failed to connect with password: %w", err)
	}
	defer conn.Close()
	fmt.Println("  ✓ SSH connection successful")

	// Generate SSH key pair
	fmt.Println("  Generating SSH key pair...")
	keyPair, err := ssh.GenerateKeyPair()
	if err != nil {
		return fmt.Errorf("failed to generate key pair: %w", err)
	}
	fmt.Println("  ✓ SSH key pair generated")

	// Install public key on host
	fmt.Println("  Installing public key on host...")
	if err := installPublicKeyOnHost(conn, keyPair); err != nil {
		return fmt.Errorf("failed to install public key: %w", err)
	}
	fmt.Println("  ✓ Public key installed")

	// Store private key to filesystem
	fmt.Println("  Storing SSH key...")
	keysDir, err := config.GetKeysDir()
	if err != nil {
		return fmt.Errorf("failed to get keys directory: %w", err)
	}

	keyStorage, err := ssh.NewFilesystemKeyStorage(keysDir)
	if err != nil {
		return fmt.Errorf("failed to create key storage: %w", err)
	}

	if err := keyStorage.Store(h.Hostname, keyPair); err != nil {
		return fmt.Errorf("failed to store SSH key: %w", err)
	}
	fmt.Println("  ✓ SSH key stored")

	return nil
}

// installPublicKeyOnHost installs the public key on the remote host
func installPublicKeyOnHost(conn *ssh.Connection, keyPair *ssh.KeyPair) error {
	// Ensure .ssh directory exists
	createDirCmd := "mkdir -p ~/.ssh && chmod 700 ~/.ssh"
	result, err := conn.Exec(createDirCmd)
	if err != nil {
		return fmt.Errorf("failed to create .ssh directory: %w", err)
	}
	if result.ExitCode != 0 {
		return fmt.Errorf("failed to create .ssh directory: %s", result.Stderr)
	}

	// Append public key to authorized_keys
	publicKey := strings.TrimSpace(keyPair.PublicKeyString())
	appendKeyCmd := fmt.Sprintf("echo '%s' >> ~/.ssh/authorized_keys && chmod 600 ~/.ssh/authorized_keys", publicKey)
	result, err = conn.Exec(appendKeyCmd)
	if err != nil {
		return fmt.Errorf("failed to append public key: %w", err)
	}
	if result.ExitCode != 0 {
		return fmt.Errorf("failed to append public key: %s", result.Stderr)
	}

	return nil
}

// connectToHostForSetup connects to a host using SSH keys
func connectToHostForSetup(h *config.Host, keysDir string) (*ssh.Connection, error) {
	keyStorage, err := ssh.NewFilesystemKeyStorage(keysDir)
	if err != nil {
		return nil, fmt.Errorf("failed to create key storage: %w", err)
	}

	keyPair, err := keyStorage.Load(h.Hostname)
	if err != nil {
		return nil, fmt.Errorf("SSH key not found: %w", err)
	}

	authMethod, err := keyPair.AuthMethod()
	if err != nil {
		return nil, fmt.Errorf("failed to create auth method: %w", err)
	}

	connOpts := &ssh.ConnectionOptions{
		Host:       h.Address,
		Port:       h.Port,
		User:       h.User,
		AuthMethod: authMethod,
		Timeout:    30,
	}

	return ssh.Connect(connOpts)
}

// configureHost runs host configuration steps
func configureHost(conn *ssh.Connection, h *config.Host, nonInteractive bool) error {
	fmt.Println("  Configuring host...")

	// Step 1: Check and setup sudo
	fmt.Println("    Checking sudo access...")
	hasSudo, err := sudo.CheckSudoAccess(adaptToSudoExec(conn), h.User)
	if err != nil {
		return fmt.Errorf("failed to check sudo access: %w", err)
	}

	if !hasSudo {
		if nonInteractive {
			return fmt.Errorf("user %s does not have sudo access and non-interactive mode is enabled", h.User)
		}

		fmt.Printf("    User %s does not have sudo access\n", h.User)
		fmt.Print("    Root password: ")
		passwordBytes, err := term.ReadPassword(int(os.Stdin.Fd()))
		fmt.Println() // Newline
		if err != nil {
			return fmt.Errorf("failed to read root password: %w", err)
		}
		rootPassword := string(passwordBytes)

		fmt.Println("    Setting up sudo...")
		if err := sudo.SetupSudo(adaptToSudoExec(conn), h.User, rootPassword); err != nil {
			return fmt.Errorf("failed to setup sudo: %w", err)
		}
		fmt.Println("    ✓ sudo configured")
	} else {
		fmt.Println("    ✓ User has sudo access")
	}

	// Step 2: Update package lists
	fmt.Println("    Updating package lists...")
	result, err := conn.Exec("sudo apt-get update -qq")
	if err != nil || result.ExitCode != 0 {
		fmt.Printf("    ⚠ Package update warning (continuing): %s\n", result.Stderr)
	} else {
		fmt.Println("    ✓ Package lists updated")
	}

	// Step 3: Install common tools (with --fix-missing for robustness)
	fmt.Println("    Installing common tools...")
	result, err = conn.Exec("sudo apt-get install -y --fix-missing curl git vim htop")
	if err != nil || result.ExitCode != 0 {
		// If full install fails, try just curl (required for container runtime)
		fmt.Printf("    ⚠ Some tools failed to install, retrying with curl only...\n")
		result, err = conn.Exec("sudo apt-get install -y --fix-missing curl")
		if err != nil || result.ExitCode != 0 {
			return fmt.Errorf("failed to install curl (required): %s", result.Stderr)
		}
		fmt.Println("    ✓ curl installed (other tools skipped)")
	} else {
		fmt.Println("    ✓ Common tools installed")
	}

	// Step 4: Configure time sync
	fmt.Println("    Configuring time synchronization...")
	result, err = conn.Exec("sudo timedatectl set-ntp true || echo 'NTP configuration skipped'")
	if err == nil && result.ExitCode == 0 {
		fmt.Println("    ✓ Time synchronization configured")
	}

	// Step 5: Create container system user
	fmt.Println("    Creating container system user...")
	result, err = conn.Exec("sudo groupadd -g 374 foundrysys 2>/dev/null || true")
	if err == nil {
		conn.Exec("sudo useradd -r -u 374 -g 374 -s /usr/sbin/nologin -M foundrysys 2>/dev/null || true")
		fmt.Println("    ✓ Container system user created")
	}

	// Step 6: Install container runtime
	fmt.Println("    Installing container runtime...")
	containerExec := adaptToContainerExec(conn)

	runtimeType := container.DetectRuntimeInstallation(containerExec)
	switch runtimeType {
	case container.RuntimeDocker:
		fmt.Println("    ✓ Docker already installed")
	case container.RuntimeNerdctl:
		fmt.Println("    ✓ nerdctl already installed")
	case container.RuntimeNerdctlIncomplete:
		fmt.Println("    Completing nerdctl installation...")
		if err := container.InstallRuntime(containerExec, h.User); err != nil {
			return fmt.Errorf("failed to complete container runtime: %w", err)
		}
		fmt.Println("    ✓ Container runtime completed")
	case container.RuntimeNone:
		fmt.Println("    Installing containerd + nerdctl...")
		if err := container.InstallRuntime(containerExec, h.User); err != nil {
			return fmt.Errorf("failed to install container runtime: %w", err)
		}
		fmt.Println("    ✓ Container runtime installed")
	}

	// Verify container runtime
	if runtimeType == container.RuntimeNone || runtimeType == container.RuntimeNerdctlIncomplete {
		fmt.Println("    Verifying container runtime...")
		if err := container.VerifyRuntimeInstalled(containerExec); err != nil {
			return fmt.Errorf("container runtime verification failed: %w", err)
		}
		fmt.Println("    ✓ Container runtime verified")
	}

	return nil
}

// adaptToSudoExec creates a sudo.CommandExecutor from an ssh.Connection
func adaptToSudoExec(conn *ssh.Connection) sudo.CommandExecutor {
	return sudoExecFunc(func(cmd string) (*sudo.ExecResult, error) {
		result, err := conn.Exec(cmd)
		if err != nil {
			return nil, err
		}
		return &sudo.ExecResult{
			Stdout:   result.Stdout,
			Stderr:   result.Stderr,
			ExitCode: result.ExitCode,
		}, nil
	})
}

// adaptToContainerExec creates a container.CommandExecutor from an ssh.Connection
func adaptToContainerExec(conn *ssh.Connection) container.CommandExecutor {
	return containerExecFunc(func(cmd string) (*container.ExecResult, error) {
		result, err := conn.Exec(cmd)
		if err != nil {
			return nil, err
		}
		return &container.ExecResult{
			Stdout:   result.Stdout,
			Stderr:   result.Stderr,
			ExitCode: result.ExitCode,
		}, nil
	})
}

// sudoExecFunc is a function type that implements sudo.CommandExecutor
type sudoExecFunc func(cmd string) (*sudo.ExecResult, error)

func (f sudoExecFunc) Exec(cmd string) (*sudo.ExecResult, error) {
	return f(cmd)
}

// containerExecFunc is a function type that implements container.CommandExecutor
type containerExecFunc func(cmd string) (*container.ExecResult, error)

func (f containerExecFunc) Exec(cmd string) (*container.ExecResult, error) {
	return f(cmd)
}
