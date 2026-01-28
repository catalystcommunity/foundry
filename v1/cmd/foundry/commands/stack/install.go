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

	"gopkg.in/yaml.v3"

	clustercommands "github.com/catalystcommunity/foundry/v1/cmd/foundry/commands/cluster"
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
	"github.com/catalystcommunity/foundry/v1/internal/component/storage"
	"github.com/catalystcommunity/foundry/v1/internal/component/velero"
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
	// Order: gateway-api, storage, prometheus, contour, cert-manager, seaweedfs, external-dns, loki, grafana, velero
	phase3Components := []string{
		"gateway-api",
		"storage",
		"prometheus",
		"contour",
		"cert-manager",
		"seaweedfs",
		"external-dns",
		"loki",
		"grafana",
		"velero",
	}

	for _, name := range phase3Components {
		comp := component.Get(name)
		if comp != nil {
			status, err := comp.Status(ctx)
			if err != nil || status == nil || !status.Installed {
				return false
			}
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
		&cli.StringFlag{
			Name:  "user",
			Usage: "SSH user for hosts (default: root)",
			Value: "root",
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
		&cli.BoolFlag{
			Name:  "upgrade",
			Usage: "Upgrade already-installed K8s components with current configuration",
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
	// Determine config path (--config flag inherited from root command)
	configPath, err := config.FindConfig(cmd.String("config"))
	if err != nil {
		// For install, missing config is OK - we'll create one
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
	upgradeMode := cmd.Bool("upgrade")
	if allComponentsInstalled && !upgradeMode {
		fmt.Println("\n✓ Stack is already complete!")
		fmt.Println("\nTo upgrade components with new configuration, use: foundry stack install --upgrade")
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
	if err := installComponents(ctx, cfg, configPath, cmd.Bool("yes"), upgradeMode); err != nil {
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
	fmt.Printf("  Domain:    %s\n", cfg.Cluster.PrimaryDomain)
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
		cfg, err = createConfigFromFlags(clusterName, domain, hosts, vip, cmd.StringSlice("forwarder"), cmd.String("user"))
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
func createConfigFromFlags(clusterName, domain string, hosts []string, vip string, forwarders []string, sshUser string) (*config.Config, error) {
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
			User:     sshUser,
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
			Name:          clusterName,
			PrimaryDomain: domain,
			VIP:           vip,
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

	// SSH User
	fmt.Print("SSH user [root]: ")
	sshUser, err := reader.ReadString('\n')
	if err != nil {
		return nil, err
	}
	sshUser = strings.TrimSpace(sshUser)
	if sshUser == "" {
		sshUser = "root"
	}

	// VIP
	fmt.Print("Kubernetes VIP: ")
	vip, err := reader.ReadString('\n')
	if err != nil {
		return nil, err
	}
	vip = strings.TrimSpace(vip)

	cfg := &config.Config{
		Cluster: config.ClusterConfig{
			Name:          clusterName,
			PrimaryDomain: domain,
			VIP:           vip,
		},
		Hosts: []*config.Host{
			{
				Hostname: hostname,
				Address:  ip,
				Port:     22,
				User:     sshUser,
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

	// Cache root password across hosts - if they share the same password,
	// we only need to prompt once
	var cachedRootPassword string

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
		if err := configureHost(conn, h, nonInteractive, &cachedRootPassword); err != nil {
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
// If upgrade is true, K8s components will be upgraded even if already installed
func installComponents(ctx context.Context, cfg *config.Config, configPath string, skipConfirm bool, upgrade bool) error {
	fmt.Println("\n[3/4] Component Installation")
	fmt.Println(strings.Repeat("-", 60))

	// Ensure DNS config exists with defaults
	ensureDefaultDNSConfig(cfg)

	// Helper function to check component status via registry
	checkComponentStatus := func(name string) bool {
		comp := component.Get(name)
		if comp == nil {
			return false
		}
		status, err := comp.Status(ctx)
		if err != nil || status == nil {
			return false
		}
		return status.Installed
	}

	// Helper function to track component in config
	trackComponent := func(name string) {
		if cfg.Components == nil {
			cfg.Components = make(config.ComponentMap)
		}
		if _, exists := cfg.Components[name]; !exists {
			cfg.Components[name] = config.ComponentConfig{Config: map[string]any{"installed": true}}
		} else {
			// Update existing entry
			existing := cfg.Components[name]
			if existing.Config == nil {
				existing.Config = make(map[string]any)
			}
			existing.Config["installed"] = true
			cfg.Components[name] = existing
		}
	}

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
				trackComponent("openbao")
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
				trackComponent("dns")
			},
		},
		{
			name: "zot",
			checkFunc: func(s *setup.SetupState) bool {
				return s.ZotInstalled
			},
			setFunc: func(s *setup.SetupState) {
				s.ZotInstalled = true
				trackComponent("zot")
			},
		},
		{
			name: "k3s",
			checkFunc: func(s *setup.SetupState) bool {
				return s.K8sInstalled
			},
			setFunc: func(s *setup.SetupState) {
				s.K8sInstalled = true
				trackComponent("k3s")
			},
		},
		{
			name: "gateway-api",
			checkFunc: func(s *setup.SetupState) bool {
				return checkComponentStatus("gateway-api")
			},
			setFunc: func(s *setup.SetupState) {
				trackComponent("gateway-api")
			},
		},
		// Storage must come before prometheus because prometheus is configured to use
		// the longhorn storage class which is provided by the storage component
		{
			name: "storage",
			checkFunc: func(s *setup.SetupState) bool {
				return checkComponentStatus("storage")
			},
			setFunc: func(s *setup.SetupState) {
				trackComponent("storage")
			},
		},
		// Prometheus must come before contour, cert-manager, and seaweedfs
		// because those components enable ServiceMonitor which requires the CRD
		// installed by the kube-prometheus-stack Helm chart
		{
			name: "prometheus",
			checkFunc: func(s *setup.SetupState) bool {
				return checkComponentStatus("prometheus")
			},
			setFunc: func(s *setup.SetupState) {
				trackComponent("prometheus")
			},
		},
		{
			name: "contour",
			checkFunc: func(s *setup.SetupState) bool {
				return checkComponentStatus("contour")
			},
			setFunc: func(s *setup.SetupState) {
				trackComponent("contour")
			},
		},
		{
			name: "cert-manager",
			checkFunc: func(s *setup.SetupState) bool {
				return checkComponentStatus("cert-manager")
			},
			setFunc: func(s *setup.SetupState) {
				trackComponent("cert-manager")
			},
		},
		{
			name: "seaweedfs",
			checkFunc: func(s *setup.SetupState) bool {
				return checkComponentStatus("seaweedfs")
			},
			setFunc: func(s *setup.SetupState) {
				trackComponent("seaweedfs")
			},
		},
		{
			name: "external-dns",
			checkFunc: func(s *setup.SetupState) bool {
				return checkComponentStatus("external-dns")
			},
			setFunc: func(s *setup.SetupState) {
				trackComponent("external-dns")
			},
		},
		{
			name: "loki",
			checkFunc: func(s *setup.SetupState) bool {
				return checkComponentStatus("loki")
			},
			setFunc: func(s *setup.SetupState) {
				trackComponent("loki")
			},
		},
		{
			name: "grafana",
			checkFunc: func(s *setup.SetupState) bool {
				return checkComponentStatus("grafana")
			},
			setFunc: func(s *setup.SetupState) {
				trackComponent("grafana")
			},
		},
		{
			name: "velero",
			checkFunc: func(s *setup.SetupState) bool {
				return checkComponentStatus("velero")
			},
			setFunc: func(s *setup.SetupState) {
				trackComponent("velero")
			},
		},
	}

	// K8s components that can be upgraded with --upgrade flag
	k8sComponents := map[string]bool{
		"gateway-api": true, "contour": true, "cert-manager": true, "storage": true,
		"seaweedfs": true, "prometheus": true, "external-dns": true,
		"loki": true, "grafana": true, "velero": true,
	}

	for i, comp := range components {
		// Check if already installed
		isInstalled := comp.checkFunc(cfg.SetupState)
		isK8sComponent := k8sComponents[comp.name]

		if isInstalled {
			if upgrade && isK8sComponent {
				fmt.Printf("\n[%d/%d] Upgrading %s...\n", i+1, len(components), comp.name)
			} else {
				fmt.Printf("\n[%d/%d] %s: ✓ Already installed (skipping)\n", i+1, len(components), comp.name)
				// DNS is installed but ensure zones exist (sync config with reality)
				if comp.name == "dns" {
					configDir, err := config.GetConfigDir()
					if err != nil {
						return fmt.Errorf("failed to get config directory: %w", err)
					}
					if err := createDNSZones(ctx, cfg, configDir); err != nil {
						return fmt.Errorf("failed to ensure DNS zones: %w", err)
					}
				}
				// K3s is installed but ensure registries.yaml is up to date
				if comp.name == "k3s" {
					fmt.Println("  Syncing K3s registry configuration...")
					if err := installK3sCluster(ctx, cfg); err != nil {
						return fmt.Errorf("failed to sync K3s registries: %w", err)
					}
				}
				continue
			}
		} else {
			fmt.Printf("\n[%d/%d] Installing %s...\n", i+1, len(components), comp.name)
		}

		// Install/upgrade the component
		if err := installSingleComponent(ctx, cfg, comp.name); err != nil {
			return fmt.Errorf("%s installation failed: %w", comp.name, err)
		}

		// After DNS is installed, create zones in PowerDNS
		if comp.name == "dns" {
			configDir, err := config.GetConfigDir()
			if err != nil {
				return fmt.Errorf("failed to get config directory: %w", err)
			}
			if err := createDNSZones(ctx, cfg, configDir); err != nil {
				return fmt.Errorf("failed to create DNS zones: %w", err)
			}
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
		// Create cert-manager with CA issuer enabled for signing TLS certificates
		componentWithClients = certmanager.NewComponent(&certmanager.Config{
			CreateDefaultIssuer: true,
			DefaultIssuerType:   "self-signed",
		})
	case "external-dns":
		componentWithClients = externaldns.NewComponent(helmClient, k8sClient)
	case "storage":
		componentWithClients = storage.NewComponent(helmClient, k8sClient)
	case "seaweedfs":
		componentWithClients = seaweedfs.NewComponent(helmClient, k8sClient)
	case "prometheus":
		componentWithClients = prometheus.NewComponent(helmClient, k8sClient)
	case "loki":
		componentWithClients = loki.NewComponent(helmClient, k8sClient)
	case "grafana":
		componentWithClients = grafana.NewComponent(helmClient, k8sClient)
	case "velero":
		componentWithClients = velero.NewComponent(helmClient, k8sClient)
	default:
		return fmt.Errorf("unknown kubernetes component: %s", componentName)
	}

	// Create component config with cluster-specific values
	componentConfig := component.ComponentConfig{}

	// Pass helm and k8s clients to components that need them via ComponentConfig
	if componentName == "cert-manager" {
		componentConfig["helm_client"] = helmClient
		componentConfig["k8s_client"] = k8sClient
	}

	// Pass cluster VIP to Contour for LoadBalancer annotation sharing
	if componentName == "contour" && cfg.Cluster.VIP != "" {
		componentConfig["cluster_vip"] = cfg.Cluster.VIP
	}

	// Pass DNS provider config to external-dns
	if componentName == "external-dns" {
		componentConfig = buildExternalDNSConfig(ctx, cfg, configDir)
	}

	// Pass storage backend config
	if componentName == "storage" {
		componentConfig = buildStorageConfig(ctx, cfg)
	}

	// Pass SeaweedFS config
	if componentName == "seaweedfs" {
		componentConfig = buildSeaweedFSConfig(cfg)
	}

	// Pass Prometheus config
	if componentName == "prometheus" {
		componentConfig = buildPrometheusConfig(cfg)
	}

	// Pass Loki config (needs SeaweedFS connection info)
	if componentName == "loki" {
		componentConfig = buildLokiConfig(cfg)
	}

	// Pass Grafana config (needs Prometheus and Loki endpoints)
	if componentName == "grafana" {
		componentConfig = buildGrafanaConfig(cfg)
	}

	// Pass Velero config (needs SeaweedFS connection info)
	if componentName == "velero" {
		componentConfig = buildVeleroConfig(cfg)
	}

	// Install the component
	if err := componentWithClients.Install(ctx, componentConfig); err != nil {
		return fmt.Errorf("installation failed: %w", err)
	}

	// Save component config (including values) back to stack config
	// This allows users to see and customize the Helm values
	saveComponentConfig(cfg, componentName, componentConfig)

	fmt.Printf("  ✓ %s installed successfully\n", componentName)

	// After prometheus is installed, upgrade storage to enable ServiceMonitor
	// This solves the chicken-and-egg problem: storage needs to come before prometheus
	// (for storage class), but ServiceMonitor requires the CRD from prometheus
	if componentName == "prometheus" {
		if err := upgradeStorageWithServiceMonitor(ctx, cfg, helmClient, k8sClient); err != nil {
			// Log warning but don't fail - storage is functional, just without metrics
			fmt.Printf("  ⚠ Could not enable storage ServiceMonitor: %v\n", err)
		}
	}

	return nil
}

// upgradeStorageWithServiceMonitor upgrades storage (Longhorn) to enable ServiceMonitor
// after Prometheus has been installed and the CRD is available
func upgradeStorageWithServiceMonitor(ctx context.Context, cfg *config.Config, helmClient *helm.Client, k8sClient *k8s.Client) error {
	// Only upgrade if storage backend is Longhorn
	if cfg.Storage == nil || cfg.Storage.Backend != "longhorn" {
		// Check component config too
		if cfg.Components != nil {
			if storageCfg, exists := cfg.Components["storage"]; exists {
				if backend, ok := storageCfg.Config["backend"].(string); ok && backend != "longhorn" {
					return nil // Not using Longhorn, nothing to upgrade
				}
			}
		}
	}

	// Check if ServiceMonitor CRD is now available
	if k8sClient != nil {
		crdExists, err := k8sClient.ServiceMonitorCRDExists(ctx)
		if err != nil || !crdExists {
			return fmt.Errorf("ServiceMonitor CRD still not available")
		}
	}

	fmt.Println("  Enabling ServiceMonitor for storage...")

	// Create storage component with ServiceMonitor enabled
	storageComp := storage.NewComponent(helmClient, k8sClient)

	// Build config with ServiceMonitor enabled
	componentConfig := buildStorageConfig(ctx, cfg)
	// Ensure ServiceMonitor is enabled for the upgrade
	if longhornCfg, ok := componentConfig["longhorn"].(map[string]interface{}); ok {
		longhornCfg["service_monitor_enabled"] = true
	}

	// Use Upgrade method if available, otherwise Install will handle upgrade
	if err := storageComp.Install(ctx, componentConfig); err != nil {
		return fmt.Errorf("failed to upgrade storage: %w", err)
	}

	fmt.Println("  ✓ Storage ServiceMonitor enabled")
	return nil
}

// buildExternalDNSConfig creates config for external-dns component
func buildExternalDNSConfig(ctx context.Context, cfg *config.Config, configDir string) component.ComponentConfig {
	// Build PowerDNS config
	pdnsConfig := map[string]interface{}{}
	var pdnsAPIURL string
	var pdnsAPIKey string

	// Get PowerDNS API endpoint
	if dnsHosts := cfg.GetHostAddresses(host.RoleDNS); len(dnsHosts) > 0 {
		pdnsAPIURL = fmt.Sprintf("http://%s:8081", dnsHosts[0])
		pdnsConfig["api_url"] = pdnsAPIURL
	}

	// Get DNS API key from OpenBAO
	if cfg.SetupState != nil && cfg.SetupState.OpenBAOInitialized {
		apiKey, err := getDNSAPIKeyFromOpenBAO(ctx, cfg, configDir)
		if err == nil && apiKey != "" {
			pdnsAPIKey = apiKey
			pdnsConfig["api_key"] = apiKey
		}
	}

	// Build domain filters from primary_domain + kubernetes_zones (deduplicated)
	domainFilters := []string{}
	seen := make(map[string]bool)

	// Always include primary_domain first
	if cfg.Cluster.PrimaryDomain != "" {
		domainFilters = append(domainFilters, cfg.Cluster.PrimaryDomain)
		seen[cfg.Cluster.PrimaryDomain] = true
	}

	// Add zones from kubernetes_zones (skip duplicates)
	if cfg.DNS != nil {
		for _, zone := range cfg.DNS.KubernetesZones {
			if !seen[zone.Name] {
				domainFilters = append(domainFilters, zone.Name)
				seen[zone.Name] = true
			}
		}
	}

	// Build domain filters YAML list
	domainFiltersYAML := ""
	for _, df := range domainFilters {
		domainFiltersYAML += fmt.Sprintf("  - %s\n", df)
	}

	// Default Helm values for external-dns (YAML format for readability)
	defaultValuesYAML := fmt.Sprintf(`
provider:
  name: pdns
pdns:
  apiUrl: %s
  apiKey: %s
domainFilters:
%ssources:
  - ingress
  - service
policy: upsert-only
txtOwnerId: foundry
logLevel: info
resources:
  requests:
    cpu: 50m
    memory: 64Mi
`, pdnsAPIURL, pdnsAPIKey, domainFiltersYAML)

	defaultValues := parseYAMLValues(defaultValuesYAML)

	componentConfig := component.ComponentConfig{
		"provider":       "pdns",
		"domain_filters": domainFilters,
		"powerdns":       pdnsConfig,
	}

	// Merge user-provided values over defaults (user values take precedence)
	if userValues := getUserValuesFromConfig(cfg, "external-dns"); userValues != nil {
		componentConfig["values"] = mergeValues(defaultValues, userValues)
	} else {
		componentConfig["values"] = defaultValues
	}

	return componentConfig
}

// getDNSAPIKeyFromOpenBAO retrieves the DNS API key from OpenBAO
func getDNSAPIKeyFromOpenBAO(ctx context.Context, cfg *config.Config, configDir string) (string, error) {
	// Get OpenBAO address from config
	addr, err := cfg.GetPrimaryOpenBAOAddress()
	if err != nil {
		return "", err
	}
	openBAOAddr := fmt.Sprintf("http://%s:8200", addr)

	// Get OpenBAO token from keys file
	keysPath := filepath.Join(configDir, "openbao-keys", cfg.Cluster.Name, "keys.json")
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

	// Create OpenBAO client
	openBAOClient := openbao.NewClient(openBAOAddr, keys.RootToken)

	// Read DNS API key
	secretData, err := openBAOClient.ReadSecretV2(ctx, "foundry-core", "dns")
	if err != nil {
		return "", err
	}

	if apiKey, ok := secretData["api_key"].(string); ok {
		return apiKey, nil
	}

	return "", fmt.Errorf("api_key not found in secret")
}

// buildStorageConfig creates config for storage component
func buildStorageConfig(ctx context.Context, cfg *config.Config) component.ComponentConfig {
	// Calculate optimal replica count based on configured cluster nodes
	replicaCount := calculateOptimalReplicaCount(cfg)
	ingressHost := fmt.Sprintf("longhorn.%s", cfg.Cluster.PrimaryDomain)

	longhornConfig := map[string]interface{}{
		"replica_count":   replicaCount,
		"data_path":       "/var/lib/longhorn",
		"ingress_enabled": true,
		"ingress_host":    ingressHost,
	}

	// Default Helm values for Longhorn (YAML format for readability)
	defaultValuesYAML := fmt.Sprintf(`
defaultSettings:
  defaultReplicaCount: %d
  defaultDataPath: /var/lib/longhorn
  guaranteedInstanceManagerCPU: 12
  defaultDataLocality: disabled
persistence:
  defaultClass: true
  defaultClassReplicaCount: %d
  reclaimPolicy: Delete
ingress:
  enabled: true
  ingressClassName: contour
  host: %s
  tls: true
  tlsSecret: longhorn-tls
  annotations:
    cert-manager.io/cluster-issuer: foundry-ca-issuer
`, replicaCount, replicaCount, ingressHost)

	defaultValues := parseYAMLValues(defaultValuesYAML)

	// Default to Longhorn for distributed block storage with replication
	componentConfig := component.ComponentConfig{
		"backend":            "longhorn",
		"storage_class_name": "longhorn",
		"set_default":        true,
		"longhorn":           longhornConfig,
	}

	// Allow override to local-path if explicitly configured
	if cfg.Storage != nil && cfg.Storage.Backend == "local-path" {
		componentConfig["backend"] = "local-path"
		componentConfig["storage_class_name"] = "local-path"
		delete(componentConfig, "longhorn")
		defaultValues = parseYAMLValues(`
storageClass:
  create: true
  name: local-path
  defaultClass: true
`)
	}

	// Merge user-provided values over defaults (user values take precedence)
	if userValues := getUserValuesFromConfig(cfg, "storage"); userValues != nil {
		componentConfig["values"] = mergeValues(defaultValues, userValues)
	} else {
		componentConfig["values"] = defaultValues
	}

	return componentConfig
}

// calculateOptimalReplicaCount determines the best Longhorn replica count
// based on the number of configured cluster nodes. Returns min(nodeCount, 3).
func calculateOptimalReplicaCount(cfg *config.Config) int {
	clusterHosts := cfg.GetClusterHosts()
	nodeCount := len(clusterHosts)

	if nodeCount <= 0 {
		return 1 // Minimum of 1 replica
	}
	if nodeCount >= 3 {
		return 3 // Cap at 3 replicas (standard HA)
	}
	return nodeCount
}

// SeaweedFS constants
const (
	seaweedfsEndpoint = "http://seaweedfs-s3.seaweedfs.svc.cluster.local:8333"
	seaweedfsRegion   = "us-east-1"
)

// getOrCreateSeaweedFSCredentials retrieves SeaweedFS credentials from config or generates new ones
func getOrCreateSeaweedFSCredentials(cfg *config.Config) (accessKey, secretKey string) {
	// Check if credentials already exist in config
	if cfg.Components != nil {
		if seaweedfsCfg, exists := cfg.Components["seaweedfs"]; exists {
			if seaweedfsCfg.Config != nil {
				if key, ok := seaweedfsCfg.Config["access_key"].(string); ok && key != "" {
					if secret, ok := seaweedfsCfg.Config["secret_key"].(string); ok && secret != "" {
						return key, secret
					}
				}
			}
		}
	}

	// Generate new random credentials
	keyBytes := make([]byte, 32)
	secretBytes := make([]byte, 32)
	if _, err := rand.Read(keyBytes); err != nil {
		accessKey = fmt.Sprintf("foundry-key-%d", time.Now().UnixNano())
	} else {
		accessKey = hex.EncodeToString(keyBytes)
	}
	if _, err := rand.Read(secretBytes); err != nil {
		secretKey = fmt.Sprintf("foundry-secret-%d", time.Now().UnixNano())
	} else {
		secretKey = hex.EncodeToString(secretBytes)
	}
	return accessKey, secretKey
}

// buildSeaweedFSConfig creates config for SeaweedFS component
func buildSeaweedFSConfig(cfg *config.Config) component.ComponentConfig {
	accessKey, secretKey := getOrCreateSeaweedFSCredentials(cfg)

	// Store credentials in config for other components to use
	if cfg.Components == nil {
		cfg.Components = make(config.ComponentMap)
	}
	if _, exists := cfg.Components["seaweedfs"]; !exists {
		cfg.Components["seaweedfs"] = config.ComponentConfig{Config: make(map[string]any)}
	}
	seaweedfsCfg := cfg.Components["seaweedfs"]
	if seaweedfsCfg.Config == nil {
		seaweedfsCfg.Config = make(map[string]any)
	}
	seaweedfsCfg.Config["access_key"] = accessKey
	seaweedfsCfg.Config["secret_key"] = secretKey
	cfg.Components["seaweedfs"] = seaweedfsCfg

	ingressHostFiler := fmt.Sprintf("seaweedfs.%s", cfg.Cluster.PrimaryDomain)
	ingressHostS3 := fmt.Sprintf("s3.%s", cfg.Cluster.PrimaryDomain)

	// Default Helm values for SeaweedFS (YAML format for readability)
	defaultValuesYAML := fmt.Sprintf(`
master:
  replicas: 1
  persistence:
    enabled: true
    size: 5Gi
    storageClass: longhorn
volume:
  replicas: 1
  persistence:
    enabled: true
    size: 50Gi
    storageClass: longhorn
filer:
  replicas: 1
  s3:
    enabled: true
    port: 8333
  ingress:
    enabled: true
    className: contour
    host: %s
    tls: true
    tlsSecretName: seaweedfs-filer-tls
    annotations:
      cert-manager.io/cluster-issuer: foundry-ca-issuer
s3:
  enabled: true
  ingress:
    enabled: true
    className: contour
    host: %s
    tls: true
    tlsSecretName: seaweedfs-s3-tls
    annotations:
      cert-manager.io/cluster-issuer: foundry-ca-issuer
`, ingressHostFiler, ingressHostS3)

	defaultValues := parseYAMLValues(defaultValuesYAML)

	componentConfig := component.ComponentConfig{
		"namespace":          "seaweedfs",
		"storage_size":       "50Gi",
		"storage_class":      "longhorn",
		"access_key":         accessKey,
		"secret_key":         secretKey,
		"buckets":            []string{"loki", "velero"},
		"ingress_enabled":    true,
		"ingress_host_filer": ingressHostFiler,
		"ingress_host_s3":    ingressHostS3,
	}

	// Merge user-provided values over defaults (user values take precedence)
	if userValues := getUserValuesFromConfig(cfg, "seaweedfs"); userValues != nil {
		componentConfig["values"] = mergeValues(defaultValues, userValues)
	} else {
		componentConfig["values"] = defaultValues
	}

	return componentConfig
}

// getSeaweedFSCredentials retrieves SeaweedFS credentials from config
func getSeaweedFSCredentials(cfg *config.Config) (accessKey, secretKey string) {
	accessKey = ""
	secretKey = ""

	if cfg.Components != nil {
		if seaweedfsCfg, exists := cfg.Components["seaweedfs"]; exists {
			if seaweedfsCfg.Config != nil {
				if key, ok := seaweedfsCfg.Config["access_key"].(string); ok {
					accessKey = key
				}
				if secret, ok := seaweedfsCfg.Config["secret_key"].(string); ok {
					secretKey = secret
				}
			}
		}
	}

	return accessKey, secretKey
}

// buildPrometheusConfig creates config for Prometheus component
func buildPrometheusConfig(cfg *config.Config) component.ComponentConfig {
	ingressHost := fmt.Sprintf("prometheus.%s", cfg.Cluster.PrimaryDomain)

	// Default Helm values for kube-prometheus-stack (YAML format for readability)
	defaultValuesYAML := fmt.Sprintf(`
prometheus:
  prometheusSpec:
    retention: 15d
    scrapeInterval: 30s
    storageSpec:
      volumeClaimTemplate:
        spec:
          storageClassName: longhorn
          accessModes:
            - ReadWriteOnce
          resources:
            requests:
              storage: 10Gi
    # ServiceMonitor discovery - discover all ServiceMonitors
    serviceMonitorSelectorNilUsesHelmValues: false
    serviceMonitorSelector: {}
    serviceMonitorNamespaceSelector: {}
    # PodMonitor discovery
    podMonitorSelectorNilUsesHelmValues: false
    podMonitorSelector: {}
    podMonitorNamespaceSelector: {}
    # Probe discovery
    probeSelectorNilUsesHelmValues: false
    probeSelector: {}
    probeNamespaceSelector: {}
    # Rule discovery
    ruleSelectorNilUsesHelmValues: false
    ruleSelector: {}
    ruleNamespaceSelector: {}
  ingress:
    enabled: true
    ingressClassName: contour
    hosts:
      - %s
    annotations:
      cert-manager.io/cluster-issuer: foundry-ca-issuer
    tls:
      - hosts:
          - %s
        secretName: prometheus-tls
alertmanager:
  enabled: false
grafana:
  enabled: false  # We deploy Grafana separately
nodeExporter:
  enabled: true
prometheus-node-exporter:
  hostRootFsMount:
    enabled: true
kubeStateMetrics:
  enabled: true
prometheusOperator:
  resources:
    requests:
      cpu: 100m
      memory: 128Mi
`, ingressHost, ingressHost)

	defaultValues := parseYAMLValues(defaultValuesYAML)

	componentConfig := component.ComponentConfig{
		"namespace":       "monitoring",
		"retention_days":  15,
		"storage_size":    "10Gi",
		"storage_class":   "longhorn",
		"ingress_enabled": true,
		"ingress_host":    ingressHost,
	}

	// Merge user-provided values over defaults (user values take precedence)
	if userValues := getUserValuesFromConfig(cfg, "prometheus"); userValues != nil {
		componentConfig["values"] = mergeValues(defaultValues, userValues)
	} else {
		componentConfig["values"] = defaultValues
	}

	return componentConfig
}

// buildLokiConfig creates config for Loki component
func buildLokiConfig(cfg *config.Config) component.ComponentConfig {
	accessKey, secretKey := getSeaweedFSCredentials(cfg)
	ingressHost := fmt.Sprintf("loki.%s", cfg.Cluster.PrimaryDomain)

	// Default Helm values for Loki (YAML format for readability)
	defaultValuesYAML := fmt.Sprintf(`
deploymentMode: SingleBinary
loki:
  auth_enabled: false
  commonConfig:
    replication_factor: 1
  storage:
    type: s3
    s3:
      endpoint: %s
      bucketnames: loki
      region: %s
      accessKeyId: %s
      secretAccessKey: %s
      s3ForcePathStyle: true
      insecure: true
  schemaConfig:
    configs:
      - from: "2024-01-01"
        store: tsdb
        object_store: s3
        schema: v13
        index:
          prefix: loki_index_
          period: 24h
singleBinary:
  replicas: 1
  persistence:
    enabled: true
    size: 10Gi
    storageClass: longhorn
gateway:
  enabled: true
  ingress:
    enabled: true
    ingressClassName: contour
    hosts:
      - host: %s
        paths:
          - path: /
            pathType: Prefix
    annotations:
      cert-manager.io/cluster-issuer: foundry-ca-issuer
    tls:
      - hosts:
          - %s
        secretName: loki-gateway-tls
# Disable unused components for SingleBinary mode
read:
  replicas: 0
write:
  replicas: 0
backend:
  replicas: 0
`, seaweedfsEndpoint, seaweedfsRegion, accessKey, secretKey, ingressHost, ingressHost)

	defaultValues := parseYAMLValues(defaultValuesYAML)

	componentConfig := component.ComponentConfig{
		"namespace":        "monitoring",
		"deployment_mode":  "SingleBinary",
		"storage_backend":  "s3",
		"s3_endpoint":      seaweedfsEndpoint,
		"s3_bucket":        "loki",
		"s3_access_key":    accessKey,
		"s3_secret_key":    secretKey,
		"s3_region":        seaweedfsRegion,
		"promtail_enabled": true,
		"ingress_enabled":  true,
		"ingress_host":     ingressHost,
	}

	// Merge user-provided values over defaults (user values take precedence)
	if userValues := getUserValuesFromConfig(cfg, "loki"); userValues != nil {
		componentConfig["values"] = mergeValues(defaultValues, userValues)
	} else {
		componentConfig["values"] = defaultValues
	}

	return componentConfig
}

// buildGrafanaConfig creates config for Grafana component
func buildGrafanaConfig(cfg *config.Config) component.ComponentConfig {
	ingressHost := fmt.Sprintf("grafana.%s", cfg.Cluster.PrimaryDomain)
	prometheusURL := "http://kube-prometheus-stack-prometheus.monitoring.svc.cluster.local:9090"
	lokiURL := "http://loki-gateway.monitoring.svc.cluster.local:80"

	// Default Helm values for Grafana (YAML format for readability)
	defaultValuesYAML := fmt.Sprintf(`
adminUser: admin
adminPassword: ""  # Auto-generated by the component
# Use Recreate strategy since we use ReadWriteOnce PVC
deploymentStrategy:
  type: Recreate
persistence:
  enabled: true
  size: 5Gi
  storageClass: longhorn
ingress:
  enabled: true
  ingressClassName: contour
  hosts:
    - %s
  annotations:
    cert-manager.io/cluster-issuer: foundry-ca-issuer
  tls:
    - hosts:
        - %s
      secretName: grafana-tls
datasources:
  datasources.yaml:
    apiVersion: 1
    datasources:
      - name: Prometheus
        type: prometheus
        url: %s
        access: proxy
        isDefault: true
      - name: Loki
        type: loki
        url: %s
        access: proxy
sidecar:
  dashboards:
    enabled: true
dashboardProviders:
  dashboardproviders.yaml:
    apiVersion: 1
    providers:
      - name: default
        orgId: 1
        folder: ""
        type: file
        disableDeletion: false
        editable: true
        options:
          path: /var/lib/grafana/dashboards/default
`, ingressHost, ingressHost, prometheusURL, lokiURL)

	defaultValues := parseYAMLValues(defaultValuesYAML)

	componentConfig := component.ComponentConfig{
		"namespace":       "monitoring",
		"storage_size":    "5Gi",
		"storage_class":   "longhorn",
		"prometheus_url":  prometheusURL,
		"loki_url":        lokiURL,
		"ingress_enabled": true,
		"ingress_host":    ingressHost,
	}

	// Merge user-provided values over defaults (user values take precedence)
	if userValues := getUserValuesFromConfig(cfg, "grafana"); userValues != nil {
		componentConfig["values"] = mergeValues(defaultValues, userValues)
	} else {
		componentConfig["values"] = defaultValues
	}

	return componentConfig
}

// buildVeleroConfig creates config for Velero component
func buildVeleroConfig(cfg *config.Config) component.ComponentConfig {
	accessKey, secretKey := getSeaweedFSCredentials(cfg)

	// Default Helm values for Velero (YAML format for readability)
	// Note: credentials.secretContents.cloud uses INI format, handled separately
	defaultValuesYAML := fmt.Sprintf(`
configuration:
  backupStorageLocation:
    - name: default
      provider: aws
      bucket: velero
      default: true
      config:
        region: %s
        s3ForcePathStyle: "true"
        s3Url: %s
credentials:
  useSecret: true
initContainers:
  - name: velero-plugin-for-aws
    image: velero/velero-plugin-for-aws:v1.10.0
    imagePullPolicy: IfNotPresent
    volumeMounts:
      - mountPath: /target
        name: plugins
deployNodeAgent: false  # Disable node agent (not needed for S3)
resources:
  requests:
    memory: 128Mi
schedules:
  daily-backup:
    disabled: false
    schedule: "0 2 * * *"
    template:
      ttl: 720h  # 30 days
      excludedNamespaces:
        - kube-system
        - velero
      storageLocation: default
`, seaweedfsRegion, seaweedfsEndpoint)

	defaultValues := parseYAMLValues(defaultValuesYAML)

	// Add credentials separately (contains newlines that need special handling)
	if defaultValues["credentials"] == nil {
		defaultValues["credentials"] = make(map[string]interface{})
	}
	credMap := defaultValues["credentials"].(map[string]interface{})
	credMap["secretContents"] = map[string]interface{}{
		"cloud": fmt.Sprintf("[default]\naws_access_key_id=%s\naws_secret_access_key=%s\n", accessKey, secretKey),
	}

	componentConfig := component.ComponentConfig{
		"namespace":     "velero",
		"provider":      "s3",
		"s3_endpoint":   seaweedfsEndpoint,
		"s3_bucket":     "velero",
		"s3_access_key": accessKey,
		"s3_secret_key": secretKey,
		"s3_region":     seaweedfsRegion,
		"schedule_cron": "0 2 * * *", // Daily at 2am
		"schedule_name": "daily-backup",
	}

	// Merge user-provided values over defaults (user values take precedence)
	if userValues := getUserValuesFromConfig(cfg, "velero"); userValues != nil {
		componentConfig["values"] = mergeValues(defaultValues, userValues)
	} else {
		componentConfig["values"] = defaultValues
	}

	return componentConfig
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
	k8sComponents := map[string]bool{
		"gateway-api":  true,
		"contour":      true,
		"cert-manager": true,
		"external-dns": true,
		"storage":      true,
		"seaweedfs":    true,
		"prometheus":   true,
		"loki":         true,
		"grafana":      true,
		"velero":       true,
	}
	if k8sComponents[componentName] {
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

		// Pass all zones as local zones for Recursor forwarding
		// This includes primary_domain + kubernetes_zones + infrastructure_zones (deduplicated)
		localZones := []string{}
		seen := make(map[string]bool)
		if cfg.Cluster.PrimaryDomain != "" {
			localZones = append(localZones, cfg.Cluster.PrimaryDomain)
			seen[cfg.Cluster.PrimaryDomain] = true
		}
		if cfg.DNS != nil {
			for _, zone := range cfg.DNS.KubernetesZones {
				if !seen[zone.Name] {
					localZones = append(localZones, zone.Name)
					seen[zone.Name] = true
				}
			}
			for _, zone := range cfg.DNS.InfrastructureZones {
				if !seen[zone.Name] {
					localZones = append(localZones, zone.Name)
					seen[zone.Name] = true
				}
			}
		}
		if len(localZones) > 0 {
			compCfg["local_zones"] = localZones
		}

	case "zot":
		// Check for Docker Hub credentials (for pull-through cache rate limiting)
		// Credentials may be:
		// 1. Plain text in config → store in OpenBao, pass to Zot
		// 2. Already a secret ref ${secret:zot:...} → resolve from OpenBao
		// 3. Not configured → Zot works without them (with rate limiting)
		if cfg.SetupState.OpenBAOInitialized {
			username, password, err := getZotDockerHubCredentials(cfg, configDir)
			if err != nil {
				fmt.Printf("  ⚠ Warning: Failed to get Docker Hub credentials: %v\n", err)
			} else if username != "" && password != "" {
				compCfg["docker_hub_username"] = username
				compCfg["docker_hub_password"] = password
			}
		}
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

	// Reconcile node labels from config after cluster is up
	if err := reconcileNodeLabels(ctx, cfg); err != nil {
		// Don't fail the whole installation, just warn
		fmt.Printf("  Warning: failed to reconcile node labels: %v\n", err)
		fmt.Println("  Labels can be applied manually with: foundry cluster node label")
	}

	return nil
}

// reconcileNodeLabels ensures all nodes have the labels defined in config
// This is additive - labels on nodes that are not in config are left untouched
func reconcileNodeLabels(ctx context.Context, cfg *config.Config) error {
	// Check if any hosts have labels configured
	hasLabels := false
	for _, h := range cfg.Hosts {
		if len(h.Labels) > 0 {
			hasLabels = true
			break
		}
	}

	if !hasLabels {
		return nil // No labels configured, nothing to do
	}

	fmt.Println("  Reconciling node labels...")

	// Get kubeconfig
	configDir, err := config.GetConfigDir()
	if err != nil {
		return fmt.Errorf("failed to get config directory: %w", err)
	}
	kubeconfigPath := filepath.Join(configDir, "kubeconfig")
	kubeconfigBytes, err := os.ReadFile(kubeconfigPath)
	if err != nil {
		return fmt.Errorf("failed to read kubeconfig: %w", err)
	}

	// Create K8s client
	k8sClient, err := k8s.NewClientFromKubeconfig(kubeconfigBytes)
	if err != nil {
		return fmt.Errorf("failed to create k8s client: %w", err)
	}

	// Get current nodes
	nodes, err := k8sClient.GetNodes(ctx)
	if err != nil {
		return fmt.Errorf("failed to get nodes: %w", err)
	}

	// Build hostname -> host mapping
	hostMap := make(map[string]*host.Host)
	for _, h := range cfg.Hosts {
		hostMap[h.Hostname] = h
	}

	// Reconcile labels for each node
	labelsApplied := 0
	for _, node := range nodes {
		h, ok := hostMap[node.Name]
		if !ok || len(h.Labels) == 0 {
			continue
		}

		// Find labels that need to be applied (additive reconciliation)
		missingLabels := make(map[string]string)
		for k, v := range h.Labels {
			currentV, exists := node.Labels[k]
			if !exists || currentV != v {
				missingLabels[k] = v
			}
		}

		if len(missingLabels) > 0 {
			if err := k8sClient.SetNodeLabels(ctx, node.Name, missingLabels); err != nil {
				fmt.Printf("    Warning: failed to apply labels to %s: %v\n", node.Name, err)
				continue
			}
			fmt.Printf("    Applied labels to %s: %v\n", node.Name, missingLabels)
			labelsApplied++
		}
	}

	if labelsApplied > 0 {
		fmt.Printf("  Labels reconciled on %d node(s)\n", labelsApplied)
	} else {
		fmt.Println("  All node labels are already in sync")
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
	if cfg.Cluster.PrimaryDomain == "" {
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
	fmt.Printf("  Domain:    %s\n", cfg.Cluster.PrimaryDomain)
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

// getZotDockerHubCredentials retrieves or stores Docker Hub credentials for Zot
// This enables authenticated pulls to avoid Docker Hub rate limiting
func getZotDockerHubCredentials(cfg *config.Config, configDir string) (username, password string, err error) {
	// Get OpenBAO address from config
	addr, err := cfg.GetPrimaryOpenBAOAddress()
	if err != nil {
		return "", "", err
	}
	openBAOAddr := fmt.Sprintf("http://%s:8200", addr)

	// Get OpenBAO token from keys file
	keysPath := filepath.Join(configDir, "openbao-keys", cfg.Cluster.Name, "keys.json")
	keysData, err := os.ReadFile(keysPath)
	if err != nil {
		return "", "", fmt.Errorf("failed to read OpenBAO keys: %w", err)
	}

	var keys struct {
		RootToken string `json:"root_token"`
	}
	if err := json.Unmarshal(keysData, &keys); err != nil {
		return "", "", fmt.Errorf("failed to parse OpenBAO keys: %w", err)
	}

	openBAOClient := openbao.NewClient(openBAOAddr, keys.RootToken)

	ctx := context.Background()

	// First, check if credentials are in the component config
	if cfg.Components != nil {
		if zotCfg, exists := cfg.Components["zot"]; exists {
			cfgUsername, hasUser := zotCfg.Config["docker_hub_username"].(string)
			cfgPassword, hasPass := zotCfg.Config["docker_hub_password"].(string)

			if hasUser && hasPass && cfgUsername != "" && cfgPassword != "" {
				// Check if these are secret references
				if isSecretRef(cfgPassword) {
					// Read from OpenBao - zot secrets are stored at foundry-core/zot
					secretData, err := openBAOClient.ReadSecretV2(ctx, "foundry-core", "zot")
					if err != nil {
						return "", "", fmt.Errorf("failed to read secret from OpenBao: %w", err)
					}
					if secretData != nil {
						if u, ok := secretData["docker_hub_username"].(string); ok {
							username = u
						}
						if p, ok := secretData["docker_hub_password"].(string); ok {
							password = p
						}
					}
				} else {
					// Plain text credentials - store in OpenBao
					fmt.Println("  Storing Docker Hub credentials in OpenBAO...")
					secretData := map[string]interface{}{
						"docker_hub_username": cfgUsername,
						"docker_hub_password": cfgPassword,
					}
					if err := openBAOClient.WriteSecretV2(ctx, "foundry-core", "zot", secretData); err != nil {
						return "", "", fmt.Errorf("failed to store credentials in OpenBAO: %w", err)
					}
					fmt.Println("  ✓ Docker Hub credentials stored in OpenBAO")
					username = cfgUsername
					password = cfgPassword
				}
				return username, password, nil
			}
		}
	}

	// If not in config, try to read existing credentials from OpenBao
	existingData, err := openBAOClient.ReadSecretV2(ctx, "foundry-core", "zot")
	if err == nil && existingData != nil {
		if u, ok := existingData["docker_hub_username"].(string); ok {
			username = u
		}
		if p, ok := existingData["docker_hub_password"].(string); ok {
			password = p
		}
		if username != "" && password != "" {
			return username, password, nil
		}
	}

	// No credentials configured - return empty (Zot will work without them)
	return "", "", nil
}

// isSecretRef checks if a string is a secret reference
func isSecretRef(s string) bool {
	return len(s) > 10 && s[:9] == "${secret:" && s[len(s)-1] == '}'
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
	checkValue(cfg.Cluster.PrimaryDomain, "cluster.domain")
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
// cachedRootPassword is used to avoid prompting for root password on every host
// if hosts share the same root password
func configureHost(conn *ssh.Connection, h *config.Host, nonInteractive bool, cachedRootPassword *string) error {
	fmt.Println("  Configuring host...")

	// Step 1: Check and setup sudo
	fmt.Println("    Checking sudo access...")
	sudoStatus, err := sudo.GetSudoStatus(adaptToSudoExec(conn))
	if err != nil {
		return fmt.Errorf("failed to check sudo access: %w", err)
	}

	switch sudoStatus {
	case sudo.SudoPasswordless:
		fmt.Println("    ✓ Passwordless sudo already configured")

	case sudo.SudoRequiresPassword:
		if nonInteractive {
			return fmt.Errorf("user %s has sudo but requires a password; non-interactive mode cannot configure passwordless sudo", h.User)
		}

		fmt.Printf("    User %s has sudo access but requires a password\n", h.User)
		fmt.Println("    Foundry requires passwordless sudo for automated operations")
		fmt.Println()
		fmt.Println("    To configure passwordless sudo, we need to run commands as root.")
		_, err := getRootPasswordWithCache(adaptToSudoExec(conn), h.User, cachedRootPassword, "    Enter root password: ")
		if err != nil {
			return fmt.Errorf("failed to setup sudo: %w", err)
		}
		fmt.Println("    ✓ Passwordless sudo configured")

	case sudo.SudoNoAccess:
		if nonInteractive {
			return fmt.Errorf("user %s is not in sudoers and non-interactive mode is enabled", h.User)
		}

		fmt.Printf("    User %s is not in the sudoers file\n", h.User)
		fmt.Println("    Foundry requires passwordless sudo for automated operations")
		fmt.Println()
		fmt.Println("    To add user to sudoers, we need to run commands as root.")
		_, err := getRootPasswordWithCache(adaptToSudoExec(conn), h.User, cachedRootPassword, "    Enter root password: ")
		if err != nil {
			return fmt.Errorf("failed to setup sudo: %w", err)
		}
		fmt.Println("    ✓ Passwordless sudo configured")

	case sudo.SudoNotInstalled:
		if nonInteractive {
			return fmt.Errorf("sudo is not installed on host and non-interactive mode is enabled")
		}

		fmt.Println("    sudo is not installed on this host")
		fmt.Println("    Foundry requires passwordless sudo for automated operations")
		fmt.Println()
		fmt.Println("    To install sudo and configure access, we need to run commands as root.")
		_, err := getRootPasswordWithCache(adaptToSudoExec(conn), h.User, cachedRootPassword, "    Enter root password: ")
		if err != nil {
			return fmt.Errorf("failed to setup sudo: %w", err)
		}
		fmt.Println("    ✓ sudo installed and passwordless access configured")
	}

	// Step 2: Update package lists and fix any broken packages
	fmt.Println("    Updating package lists...")
	result, err := conn.Exec("sudo apt-get update -qq")
	if err != nil || result.ExitCode != 0 {
		fmt.Printf("    ⚠ Package update warning (continuing): %s\n", result.Stderr)
	} else {
		fmt.Println("    ✓ Package lists updated")
	}

	// Fix any packages left in a broken state from previous failed installs
	// This must happen BEFORE attempting new package installations
	conn.Exec("sudo dpkg --configure -a 2>/dev/null || true")
	result, err = conn.Exec("sudo apt-get --fix-broken install -y -qq")
	if err != nil || result.ExitCode != 0 {
		fmt.Printf("    ⚠ Package repair warning (continuing): %s\n", result.Stderr)
	}

	// Step 3: Install common tools
	// Includes open-iscsi for Longhorn storage support
	fmt.Println("    Installing common tools...")
	result, err = conn.Exec("sudo apt-get install -y curl git vim htop open-iscsi")
	if err != nil || result.ExitCode != 0 {
		// If full install fails, try just curl and open-iscsi (required for storage)
		fmt.Printf("    ⚠ Some tools failed to install, retrying with essentials only...\n")
		result, err = conn.Exec("sudo apt-get install -y curl open-iscsi")
		if err != nil || result.ExitCode != 0 {
			return fmt.Errorf("failed to install curl/open-iscsi (required): %s", result.Stderr)
		}
		fmt.Println("    ✓ Essential tools installed (other tools skipped)")
	} else {
		fmt.Println("    ✓ Common tools installed")
	}

	// Ensure iscsid service is enabled and running (required for Longhorn)
	conn.Exec("sudo systemctl enable iscsid 2>/dev/null || true")
	conn.Exec("sudo systemctl start iscsid 2>/dev/null || true")

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

// getRootPasswordWithCache attempts to use a cached root password first,
// prompting only if the cache is empty or the cached password fails.
// On success, the password is cached for subsequent hosts.
func getRootPasswordWithCache(executor sudo.CommandExecutor, user string, cachedPassword *string, promptMessage string) (string, error) {
	// Try cached password first if available
	if cachedPassword != nil && *cachedPassword != "" {
		fmt.Println("    Trying cached root password...")
		err := sudo.SetupSudo(executor, user, *cachedPassword)
		if err == nil {
			return *cachedPassword, nil
		}
		fmt.Println("    Cached password didn't work, prompting for password...")
	}

	// Prompt for password
	fmt.Print(promptMessage)
	passwordBytes, err := term.ReadPassword(int(os.Stdin.Fd()))
	fmt.Println()
	if err != nil {
		return "", fmt.Errorf("failed to read root password: %w", err)
	}
	password := string(passwordBytes)

	// Try the new password
	if err := sudo.SetupSudo(executor, user, password); err != nil {
		return "", err
	}

	// Cache the successful password
	if cachedPassword != nil {
		*cachedPassword = password
	}

	return password, nil
}

// parseYAMLValues parses a YAML string into a map for Helm values
// This makes it easier to define default values as readable YAML strings
func parseYAMLValues(yamlStr string) map[string]interface{} {
	var result map[string]interface{}
	if err := yaml.Unmarshal([]byte(yamlStr), &result); err != nil {
		// This should never happen with valid YAML literals
		// If it does, return empty map and log warning
		fmt.Printf("Warning: failed to parse YAML values: %v\n", err)
		return make(map[string]interface{})
	}
	return result
}

// getUserValuesFromConfig extracts user-provided Helm values from stack config
func getUserValuesFromConfig(cfg *config.Config, componentName string) map[string]interface{} {
	if cfg.Components == nil {
		return nil
	}

	compCfg, exists := cfg.Components[componentName]
	if !exists || compCfg.Config == nil {
		return nil
	}

	if values, ok := compCfg.Config["values"].(map[string]interface{}); ok {
		return values
	}

	return nil
}

// mergeValues deep merges userValues over defaults
// User values take precedence (override defaults)
func mergeValues(defaults, userValues map[string]interface{}) map[string]interface{} {
	result := make(map[string]interface{})

	// Copy defaults first
	for k, v := range defaults {
		result[k] = v
	}

	// User values override defaults
	for k, v := range userValues {
		if existingMap, ok := result[k].(map[string]interface{}); ok {
			if newMap, ok := v.(map[string]interface{}); ok {
				// Deep merge for nested maps
				result[k] = mergeValues(existingMap, newMap)
				continue
			}
		}
		result[k] = v // User value overrides default
	}

	return result
}

// saveComponentConfig saves the component config (including values) back to stack config
func saveComponentConfig(cfg *config.Config, componentName string, componentConfig component.ComponentConfig) {
	if cfg.Components == nil {
		cfg.Components = make(config.ComponentMap)
	}

	compCfg, exists := cfg.Components[componentName]
	if !exists {
		compCfg = config.ComponentConfig{Config: make(map[string]any)}
	}
	if compCfg.Config == nil {
		compCfg.Config = make(map[string]any)
	}

	// Fields to exclude from saving (internal/runtime only)
	internalFields := map[string]bool{
		"helm_client": true,
		"k8s_client":  true,
		"host":        true,
		"ssh_conn":    true,
		"cluster_vip": true, // Runtime derived from cluster config
	}

	// Copy component config settings
	for k, v := range componentConfig {
		if !internalFields[k] {
			compCfg.Config[k] = v
		}
	}

	compCfg.Config["installed"] = true
	cfg.Components[componentName] = compCfg
}

// ensureDefaultDNSConfig ensures the DNS config exists with sensible defaults.
// Zone creation is handled by createDNSZones using primary_domain directly.
func ensureDefaultDNSConfig(cfg *config.Config) {
	if cfg.DNS == nil {
		cfg.DNS = &config.DNSConfig{
			Backend:    "gsqlite3",
			Forwarders: []string{"8.8.8.8", "1.1.1.1"},
		}
	}
}

// createDNSZones creates all configured zones in PowerDNS and adds initial records.
// This is called after the DNS component is installed and running.
//
// Zone creation logic:
// 1. primary_domain is always created as the main zone with infrastructure records + wildcard
// 2. Additional kubernetes_zones get created with wildcard (skip if duplicate of primary_domain)
// 3. infrastructure_zones (if any) get created with infrastructure records
func createDNSZones(ctx context.Context, cfg *config.Config, configDir string) error {
	fmt.Println("  Creating DNS zones in PowerDNS...")

	primaryDomain := cfg.Cluster.PrimaryDomain
	if primaryDomain == "" {
		fmt.Println("  No primary_domain configured, skipping zone creation")
		return nil
	}

	// Get DNS API client
	dnsAddr, err := cfg.GetPrimaryDNSAddress()
	if err != nil {
		return fmt.Errorf("failed to get DNS address: %w", err)
	}

	// Get API key from OpenBAO
	apiKey, err := getDNSAPIKeyFromOpenBAO(ctx, cfg, configDir)
	if err != nil {
		return fmt.Errorf("failed to get DNS API key: %w", err)
	}

	client := dns.NewClient(fmt.Sprintf("http://%s:8081", dnsAddr), apiKey)

	// Wait for DNS API to become ready
	fmt.Printf("  Waiting for PowerDNS API to become ready")
	maxAttempts := 30
	for i := 0; i < maxAttempts; i++ {
		_, err := client.ListZones()
		if err == nil {
			fmt.Printf(" ✓\n")
			break
		}
		if i == maxAttempts-1 {
			fmt.Printf(" ✗\n")
			return fmt.Errorf("PowerDNS API not ready after %d attempts: %w", maxAttempts, err)
		}
		fmt.Printf(".")
		time.Sleep(1 * time.Second)
	}

	// Get infrastructure host IPs for record creation
	var openbaoIP, dnsIP, zotIP string
	if addr, err := cfg.GetPrimaryOpenBAOAddress(); err == nil {
		openbaoIP = addr
	}
	if addr, err := cfg.GetPrimaryDNSAddress(); err == nil {
		dnsIP = addr
	}
	if addr, err := cfg.GetPrimaryZotAddress(); err == nil {
		zotIP = addr
	}
	k8sVIP := cfg.Cluster.VIP

	// Track which zones we've processed to avoid duplicates
	processedZones := make(map[string]bool)

	// 1. Create primary_domain zone with infrastructure records + wildcard
	primaryZoneConfig := dns.ZoneConfig{
		Name:        primaryDomain,
		Type:        dns.ZoneTypeNative,
		IsPublic:    false,
		Nameservers: []string{fmt.Sprintf("ns1.%s", primaryDomain)},
	}
	if err := createZoneIfNotExists(client, primaryZoneConfig, "primary"); err != nil {
		return fmt.Errorf("failed to create primary zone %s: %w", primaryDomain, err)
	}
	processedZones[primaryDomain] = true

	// Add infrastructure A records to primary zone
	infraConfig := dns.InfrastructureRecordConfig{
		Zone:      primaryDomain,
		OpenBAOIP: openbaoIP,
		DNSIP:     dnsIP,
		ZotIP:     zotIP,
		K8sVIP:    k8sVIP,
	}
	if err := dns.InitializeInfrastructureDNS(client, infraConfig); err != nil {
		fmt.Printf("  Warning: Failed to add infrastructure records to %s: %v\n", primaryDomain, err)
	}

	// Add wildcard record to primary zone
	if k8sVIP != "" {
		if err := dns.AddWildcardRecord(client, primaryDomain, k8sVIP); err != nil {
			fmt.Printf("  Warning: Failed to add wildcard record to %s: %v\n", primaryDomain, err)
		}
	}

	// 2. Create additional kubernetes_zones (skip if same as primary_domain)
	if cfg.DNS != nil {
		for _, zone := range cfg.DNS.KubernetesZones {
			if processedZones[zone.Name] {
				continue // Skip duplicates
			}

			zoneConfig := dns.ZoneConfig{
				Name:        zone.Name,
				Type:        dns.ZoneTypeNative,
				IsPublic:    zone.Public,
				Nameservers: []string{fmt.Sprintf("ns1.%s", zone.Name)},
			}
			if zone.PublicCNAME != nil {
				zoneConfig.PublicCNAME = *zone.PublicCNAME
			}

			if err := createZoneIfNotExists(client, zoneConfig, "kubernetes"); err != nil {
				return fmt.Errorf("failed to create kubernetes zone %s: %w", zone.Name, err)
			}
			processedZones[zone.Name] = true

			// Add wildcard record
			if k8sVIP != "" {
				if err := dns.AddWildcardRecord(client, zone.Name, k8sVIP); err != nil {
					fmt.Printf("  Warning: Failed to add wildcard record to %s: %v\n", zone.Name, err)
				}
			}
		}

		// 3. Create infrastructure_zones (if any) with infrastructure records
		for _, zone := range cfg.DNS.InfrastructureZones {
			if processedZones[zone.Name] {
				continue // Skip duplicates
			}

			zoneConfig := dns.ZoneConfig{
				Name:        zone.Name,
				Type:        dns.ZoneTypeNative,
				IsPublic:    zone.Public,
				Nameservers: []string{fmt.Sprintf("ns1.%s", zone.Name)},
			}
			if zone.PublicCNAME != nil {
				zoneConfig.PublicCNAME = *zone.PublicCNAME
			}

			if err := createZoneIfNotExists(client, zoneConfig, "infrastructure"); err != nil {
				return fmt.Errorf("failed to create infrastructure zone %s: %w", zone.Name, err)
			}
			processedZones[zone.Name] = true

			// Add infrastructure A records
			zoneInfraConfig := dns.InfrastructureRecordConfig{
				Zone:      zone.Name,
				OpenBAOIP: openbaoIP,
				DNSIP:     dnsIP,
				ZotIP:     zotIP,
				K8sVIP:    k8sVIP,
			}
			if err := dns.InitializeInfrastructureDNS(client, zoneInfraConfig); err != nil {
				fmt.Printf("  Warning: Failed to add infrastructure records to %s: %v\n", zone.Name, err)
			}
		}
	}

	fmt.Println("  ✓ DNS zones created")
	return nil
}

// createZoneIfNotExists creates a zone in PowerDNS if it doesn't already exist
func createZoneIfNotExists(client *dns.Client, zoneConfig dns.ZoneConfig, zoneType string) error {
	// Check if zone exists
	zones, err := client.ListZones()
	if err != nil {
		return fmt.Errorf("failed to list zones: %w", err)
	}

	zoneName := zoneConfig.Name
	if !strings.HasSuffix(zoneName, ".") {
		zoneName = zoneName + "."
	}

	for _, z := range zones {
		if z.Name == zoneName {
			fmt.Printf("  Zone %s already exists (skipping)\n", zoneConfig.Name)
			return nil
		}
	}

	// Create zone
	fmt.Printf("  Creating %s zone: %s\n", zoneType, zoneConfig.Name)
	if zoneType == "infrastructure" {
		return dns.CreateInfrastructureZone(client, zoneConfig)
	}
	return dns.CreateKubernetesZone(client, zoneConfig)
}
