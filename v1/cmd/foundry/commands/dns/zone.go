package dns

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"text/tabwriter"

	"github.com/catalystcommunity/foundry/v1/internal/component/dns"
	"github.com/catalystcommunity/foundry/v1/internal/config"
	"github.com/catalystcommunity/foundry/v1/internal/secrets"
	"github.com/catalystcommunity/foundry/v1/internal/setup"
	"github.com/urfave/cli/v3"
)

// ZoneCommand is the dns zone subcommand
var ZoneCommand = &cli.Command{
	Name:  "zone",
	Usage: "Manage DNS zones",
	Description: `Manage DNS zones in PowerDNS.

Zones are DNS domains managed by PowerDNS. You can create, list, and delete
zones using these commands.`,
	Commands: []*cli.Command{
		zoneListCommand,
		zoneCreateCommand,
		zoneDeleteCommand,
	},
}

var zoneListCommand = &cli.Command{
	Name:  "list",
	Usage: "List all DNS zones",
	Description: `List all DNS zones in PowerDNS.

This command connects to PowerDNS and lists all configured zones.

Example:
  foundry dns zone list`,
	Action: runZoneList,
}

var zoneCreateCommand = &cli.Command{
	Name:      "create",
	Usage:     "Create a new DNS zone",
	ArgsUsage: "<zone-name>",
	Description: `Create a new DNS zone in PowerDNS.

The zone name should be a fully qualified domain name (e.g., example.com).

Zone types:
  - Native (default): PowerDNS acts as master with no replication
  - Master: Zone can be transferred to slaves
  - Slave: Zone is replicated from a master

By default, zones are added to kubernetes_zones in the config (for external-dns).
Use --infrastructure to add to infrastructure_zones instead (for static infra records).

For split-horizon public zones, use --public with --public-cname to specify
the DDNS hostname for external queries.

Examples:
  foundry dns zone create example.com
  foundry dns zone create example.com --type Master
  foundry dns zone create infra.example.com --infrastructure
  foundry dns zone create example.com --public --public-cname home.example.com`,
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:  "type",
			Usage: "Zone type (Native, Master, Slave)",
			Value: "Native",
		},
		&cli.BoolFlag{
			Name:  "public",
			Usage: "Enable split-horizon DNS for public access",
			Value: false,
		},
		&cli.StringFlag{
			Name:  "public-cname",
			Usage: "DDNS hostname for public queries (required if --public)",
		},
		&cli.BoolFlag{
			Name:  "infrastructure",
			Usage: "Add to infrastructure_zones instead of kubernetes_zones",
			Value: false,
		},
		&cli.BoolFlag{
			Name:  "no-wildcard",
			Usage: "Skip adding wildcard record (for kubernetes zones)",
			Value: false,
		},
	},
	Action: runZoneCreate,
}

var zoneDeleteCommand = &cli.Command{
	Name:      "delete",
	Usage:     "Delete a DNS zone",
	ArgsUsage: "<zone-name>",
	Description: `Delete a DNS zone from PowerDNS.

WARNING: This will delete the zone and ALL records within it.
This action cannot be undone.

Example:
  foundry dns zone delete example.com`,
	Flags: []cli.Flag{
		&cli.BoolFlag{
			Name:  "yes",
			Usage: "Skip confirmation prompt",
			Value: false,
		},
	},
	Action: runZoneDelete,
}

func runZoneList(ctx context.Context, cmd *cli.Command) error {
	// Load configuration (--config flag inherited from root command)
	configPath, err := config.FindConfig(cmd.String("config"))
	if err != nil {
		return fmt.Errorf("failed to find config: %w", err)
	}

	cfg, err := config.Load(configPath)
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	if cfg.DNS == nil {
		return fmt.Errorf("DNS configuration not found in config file")
	}

	// Resolve API key from secrets (including OpenBAO)
	resolver, resCtx, err := buildSecretResolver(cfg)
	if err != nil {
		return fmt.Errorf("failed to setup secret resolver: %w", err)
	}

	// Parse the secret reference if it's in ${secret:...} format
	apiKeyStr := cfg.DNS.APIKey
	var apiKey string
	if parsed, err := secrets.ParseSecretRef(apiKeyStr); err == nil && parsed != nil {
		apiKey, err = resolver.Resolve(resCtx, *parsed)
		if err != nil {
			return fmt.Errorf("failed to resolve DNS API key: %w", err)
		}
	} else {
		// Not a secret reference, use as-is
		apiKey = apiKeyStr
	}

	// Get DNS host from network config
	dnsAddr, err := cfg.GetPrimaryDNSAddress()
	if err != nil {
		return fmt.Errorf("DNS host not configured in network section")
	}
	dnsHost := dnsAddr

	// Create PowerDNS client
	client := dns.NewClient(fmt.Sprintf("http://%s:8081", dnsHost), apiKey)

	// List zones
	zones, err := client.ListZones()
	if err != nil {
		return fmt.Errorf("failed to list zones: %w", err)
	}

	if len(zones) == 0 {
		fmt.Println("No DNS zones configured")
		return nil
	}

	// Display zones in table format
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "NAME\tTYPE\tSERIAL")
	fmt.Fprintln(w, "----\t----\t------")
	for _, zone := range zones {
		fmt.Fprintf(w, "%s\t%s\t%d\n", zone.Name, zone.Type, zone.Serial)
	}
	w.Flush()

	return nil
}

func runZoneCreate(ctx context.Context, cmd *cli.Command) error {
	if cmd.Args().Len() < 1 {
		return fmt.Errorf("zone name is required")
	}

	zoneNameArg := cmd.Args().Get(0)
	zoneType := cmd.String("type")
	isPublic := cmd.Bool("public")
	publicCNAME := cmd.String("public-cname")
	isInfrastructure := cmd.Bool("infrastructure")
	noWildcard := cmd.Bool("no-wildcard")

	// Keep original name for config (without trailing dot)
	zoneNameForConfig := strings.TrimSuffix(zoneNameArg, ".")

	// Add trailing dot for PowerDNS API
	zoneName := zoneNameArg
	if !strings.HasSuffix(zoneName, ".") {
		zoneName = zoneName + "."
	}

	// Validate public zone requirements
	if isPublic && publicCNAME == "" {
		return fmt.Errorf("--public-cname is required when --public is set")
	}

	// Check if .local zone is being made public (not allowed)
	if isPublic && strings.HasSuffix(zoneName, ".local.") {
		return fmt.Errorf(".local zones cannot be public")
	}

	// Load configuration (--config flag inherited from root command)
	configPath, err := config.FindConfig(cmd.String("config"))
	if err != nil {
		return fmt.Errorf("failed to find config: %w", err)
	}

	cfg, err := config.Load(configPath)
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	if cfg.DNS == nil {
		cfg.DNS = &config.DNSConfig{
			Backend:    "gsqlite3",
			Forwarders: []string{"8.8.8.8", "1.1.1.1"},
		}
	}

	// Resolve API key from secrets (including OpenBAO)
	resolver, resCtx, err := buildSecretResolver(cfg)
	if err != nil {
		return fmt.Errorf("failed to setup secret resolver: %w", err)
	}

	// Parse the secret reference if it's in ${secret:...} format
	apiKeyStr := cfg.DNS.APIKey
	var apiKey string
	if parsed, err := secrets.ParseSecretRef(apiKeyStr); err == nil && parsed != nil {
		apiKey, err = resolver.Resolve(resCtx, *parsed)
		if err != nil {
			return fmt.Errorf("failed to resolve DNS API key: %w", err)
		}
	} else {
		// Not a secret reference, use as-is
		apiKey = apiKeyStr
	}

	// Get DNS host from network config
	dnsAddr, err := cfg.GetPrimaryDNSAddress()
	if err != nil {
		return fmt.Errorf("DNS host not configured in network section")
	}
	dnsHost := dnsAddr

	// Create PowerDNS client
	client := dns.NewClient(fmt.Sprintf("http://%s:8081", dnsHost), apiKey)

	// Check if zone already exists in PowerDNS
	zones, err := client.ListZones()
	if err != nil {
		return fmt.Errorf("failed to list zones: %w", err)
	}

	zoneExists := false
	for _, z := range zones {
		if z.Name == zoneName {
			zoneExists = true
			break
		}
	}

	// Create zone in PowerDNS if it doesn't exist
	if !zoneExists {
		fmt.Printf("Creating zone %s (type: %s)...\n", zoneName, zoneType)
		if err := client.CreateZone(zoneName, zoneType); err != nil {
			return fmt.Errorf("failed to create zone: %w", err)
		}
		fmt.Printf("✓ Zone %s created in PowerDNS\n", zoneName)
	} else {
		fmt.Printf("Zone %s already exists in PowerDNS\n", zoneName)
	}

	// Add zone to config if not already present
	newZone := config.DNSZone{
		Name:   zoneNameForConfig,
		Public: isPublic,
	}
	if isPublic && publicCNAME != "" {
		newZone.PublicCNAME = &publicCNAME
	}

	configUpdated := false
	if isInfrastructure {
		// Add to infrastructure_zones
		found := false
		for _, z := range cfg.DNS.InfrastructureZones {
			if z.Name == zoneNameForConfig {
				found = true
				break
			}
		}
		if !found {
			cfg.DNS.InfrastructureZones = append(cfg.DNS.InfrastructureZones, newZone)
			configUpdated = true
			fmt.Printf("✓ Added %s to infrastructure_zones\n", zoneNameForConfig)
		}

		// Add infrastructure records
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

		infraConfig := dns.InfrastructureRecordConfig{
			Zone:      zoneNameForConfig,
			OpenBAOIP: openbaoIP,
			DNSIP:     dnsIP,
			ZotIP:     zotIP,
			K8sVIP:    k8sVIP,
		}
		if err := dns.InitializeInfrastructureDNS(client, infraConfig); err != nil {
			fmt.Printf("  Warning: Failed to add infrastructure records: %v\n", err)
		} else {
			fmt.Println("✓ Infrastructure records added")
		}
	} else {
		// Add to kubernetes_zones
		found := false
		for _, z := range cfg.DNS.KubernetesZones {
			if z.Name == zoneNameForConfig {
				found = true
				break
			}
		}
		if !found {
			cfg.DNS.KubernetesZones = append(cfg.DNS.KubernetesZones, newZone)
			configUpdated = true
			fmt.Printf("✓ Added %s to kubernetes_zones\n", zoneNameForConfig)
		}

		// Add wildcard record pointing to k8s VIP
		if !noWildcard && cfg.Cluster.VIP != "" {
			if err := dns.AddWildcardRecord(client, zoneNameForConfig, cfg.Cluster.VIP); err != nil {
				fmt.Printf("  Warning: Failed to add wildcard record: %v\n", err)
			} else {
				fmt.Printf("✓ Wildcard record added (*.%s -> %s)\n", zoneNameForConfig, cfg.Cluster.VIP)
			}
		}
	}

	// Save config if updated
	if configUpdated {
		if err := config.Save(cfg, configPath); err != nil {
			return fmt.Errorf("failed to save config: %w", err)
		}
		fmt.Println("✓ Config updated")
	}

	if isPublic {
		fmt.Printf("  Public access: enabled (CNAME: %s)\n", publicCNAME)
		fmt.Println("  Note: Split-horizon configuration must be set up separately")
	}

	// Update setup state to mark DNS zones as created
	if err := updateDNSZonesState(configPath); err != nil {
		fmt.Printf("⚠ Warning: Failed to update setup state: %v\n", err)
	}

	return nil
}

func runZoneDelete(ctx context.Context, cmd *cli.Command) error {
	if cmd.Args().Len() < 1 {
		return fmt.Errorf("zone name is required")
	}

	zoneName := cmd.Args().Get(0)
	skipConfirm := cmd.Bool("yes")

	// Validate zone name
	if !strings.HasSuffix(zoneName, ".") {
		zoneName = zoneName + "."
	}

	// Load configuration (--config flag inherited from root command)
	configPath, err := config.FindConfig(cmd.String("config"))
	if err != nil {
		return fmt.Errorf("failed to find config: %w", err)
	}

	cfg, err := config.Load(configPath)
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	if cfg.DNS == nil {
		return fmt.Errorf("DNS configuration not found in config file")
	}

	// Resolve API key from secrets (including OpenBAO)
	resolver, resCtx, err := buildSecretResolver(cfg)
	if err != nil {
		return fmt.Errorf("failed to setup secret resolver: %w", err)
	}

	// Parse the secret reference if it's in ${secret:...} format
	apiKeyStr := cfg.DNS.APIKey
	var apiKey string
	if parsed, err := secrets.ParseSecretRef(apiKeyStr); err == nil && parsed != nil {
		apiKey, err = resolver.Resolve(resCtx, *parsed)
		if err != nil {
			return fmt.Errorf("failed to resolve DNS API key: %w", err)
		}
	} else {
		// Not a secret reference, use as-is
		apiKey = apiKeyStr
	}

	// Get DNS host from network config
	dnsAddr, err := cfg.GetPrimaryDNSAddress()
	if err != nil {
		return fmt.Errorf("DNS host not configured in network section")
	}
	dnsHost := dnsAddr

	// Create PowerDNS client
	client := dns.NewClient(fmt.Sprintf("http://%s:8081", dnsHost), apiKey)

	// Confirm deletion unless --yes flag is set
	if !skipConfirm {
		fmt.Printf("WARNING: This will delete zone %s and ALL its records.\n", zoneName)
		fmt.Print("Are you sure? (yes/no): ")
		var response string
		fmt.Scanln(&response)
		if strings.ToLower(response) != "yes" {
			fmt.Println("Deletion cancelled")
			return nil
		}
	}

	// Delete zone
	fmt.Printf("Deleting zone %s...\n", zoneName)
	if err := client.DeleteZone(zoneName); err != nil {
		return fmt.Errorf("failed to delete zone: %w", err)
	}

	fmt.Printf("✓ Zone %s deleted successfully\n", zoneName)

	return nil
}

// buildSecretResolver creates a secret resolver chain that includes OpenBAO if available
func buildSecretResolver(cfg *config.Config) (*secrets.ChainResolver, *secrets.ResolutionContext, error) {
	// Try to get OpenBAO address and token
	var openBAOAddr, openBAOToken string

	if addr, err := cfg.GetPrimaryOpenBAOAddress(); err == nil {
		openBAOAddr = fmt.Sprintf("http://%s:8200", addr)

		// Try to read OpenBAO token from keys file
		configDir, err := config.GetConfigDir()
		if err == nil {
			keysPath := filepath.Join(configDir, "openbao-keys", cfg.Cluster.Name, "keys.json")
			if keysData, err := os.ReadFile(keysPath); err == nil {
				var keys struct {
					RootToken string `json:"root_token"`
				}
				if err := json.Unmarshal(keysData, &keys); err == nil {
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

// updateDNSZonesState updates the setup state to mark DNS zones as created
func updateDNSZonesState(configPath string) error {
	// Load current state
	state, err := setup.LoadState(configPath)
	if err != nil {
		return fmt.Errorf("failed to load setup state: %w", err)
	}

	// Update the dns_zones_created flag
	state.DNSZonesCreated = true

	// Save the updated state
	if err := setup.SaveState(configPath, state); err != nil {
		return fmt.Errorf("failed to save setup state: %w", err)
	}

	return nil
}
