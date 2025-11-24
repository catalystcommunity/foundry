package dns

import (
	"context"
	"fmt"
	"os"
	"strings"
	"text/tabwriter"

	"github.com/catalystcommunity/foundry/v1/internal/component/dns"
	"github.com/catalystcommunity/foundry/v1/internal/config"
	"github.com/catalystcommunity/foundry/v1/internal/host"
	"github.com/catalystcommunity/foundry/v1/internal/secrets"
	"github.com/catalystcommunity/foundry/v1/internal/ssh"
	"github.com/urfave/cli/v3"
)

// RecordCommand is the dns record subcommand
var RecordCommand = &cli.Command{
	Name:  "record",
	Usage: "Manage DNS records",
	Description: `Manage DNS records in PowerDNS zones.

Records are individual DNS entries within a zone. You can add, list, and delete
records using these commands.`,
	Commands: []*cli.Command{
		recordAddCommand,
		recordListCommand,
		recordDeleteCommand,
	},
}

var recordAddCommand = &cli.Command{
	Name:      "add",
	Usage:     "Add a DNS record to a zone",
	ArgsUsage: "<zone> <name> <type> <content>",
	Description: `Add a DNS record to a zone.

Record types: A, AAAA, CNAME, MX, TXT, NS, SOA, PTR, SRV, etc.

The record name should be fully qualified (ending with zone name).
If the name doesn't end with the zone name, it will be appended automatically.

Examples:
  foundry dns record add example.com. www.example.com. A 192.168.1.10
  foundry dns record add example.com. mail MX "10 mail.example.com."
  foundry dns record add example.com. @ TXT "v=spf1 mx ~all"
  foundry dns record add example.com. www.example.com. A 192.168.1.10 --ttl 7200`,
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:    "config",
			Aliases: []string{"c"},
			Usage:   "Path to configuration file",
			Value:   config.DefaultConfigPath(),
		},
		&cli.IntFlag{
			Name:  "ttl",
			Usage: "Time to live in seconds",
			Value: 3600,
		},
	},
	Action: runRecordAdd,
}

var recordListCommand = &cli.Command{
	Name:      "list",
	Usage:     "List all records in a zone",
	ArgsUsage: "<zone>",
	Description: `List all DNS records in a zone.

This command shows all records configured in the specified zone.

Example:
  foundry dns record list example.com.`,
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:    "config",
			Aliases: []string{"c"},
			Usage:   "Path to configuration file",
			Value:   config.DefaultConfigPath(),
		},
		&cli.StringFlag{
			Name:  "type",
			Usage: "Filter by record type (A, AAAA, CNAME, etc.)",
		},
	},
	Action: runRecordList,
}

var recordDeleteCommand = &cli.Command{
	Name:      "delete",
	Usage:     "Delete a DNS record from a zone",
	ArgsUsage: "<zone> <name> <type>",
	Description: `Delete a DNS record from a zone.

This will delete ALL records matching the specified name and type.

Examples:
  foundry dns record delete example.com. www.example.com. A
  foundry dns record delete example.com. mail.example.com. MX`,
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:    "config",
			Aliases: []string{"c"},
			Usage:   "Path to configuration file",
			Value:   config.DefaultConfigPath(),
		},
		&cli.BoolFlag{
			Name:  "yes",
			Usage: "Skip confirmation prompt",
			Value: false,
		},
	},
	Action: runRecordDelete,
}

func runRecordAdd(ctx context.Context, cmd *cli.Command) error {
	if cmd.Args().Len() < 4 {
		return fmt.Errorf("requires 4 arguments: <zone> <name> <type> <content>")
	}

	zoneName := cmd.Args().Get(0)
	recordName := cmd.Args().Get(1)
	recordType := strings.ToUpper(cmd.Args().Get(2))
	recordContent := cmd.Args().Get(3)
	configPath := cmd.String("config")
	ttl := cmd.Int("ttl")

	// Ensure zone name ends with dot
	if !strings.HasSuffix(zoneName, ".") {
		zoneName = zoneName + "."
	}

	// Ensure record name ends with dot
	if !strings.HasSuffix(recordName, ".") {
		// If it's just "@", replace with zone name
		if recordName == "@" {
			recordName = zoneName
		} else if !strings.HasSuffix(recordName, zoneName) {
			// If it doesn't end with zone name, append it
			recordName = recordName + "." + zoneName
		} else {
			recordName = recordName + "."
		}
	}

	// Load configuration
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

	// Add record
	fmt.Printf("Adding %s record %s -> %s (TTL: %d)...\n", recordType, recordName, recordContent, ttl)
	if err := client.AddRecord(zoneName, recordName, recordType, recordContent, ttl); err != nil {
		return fmt.Errorf("failed to add record: %w", err)
	}

	fmt.Printf("✓ Record added successfully\n")

	// Clear DNS cache for this zone so changes take effect immediately
	if err := clearDNSCache(cfg, zoneName); err != nil {
		// Don't fail if cache clear fails, just warn
		fmt.Printf("⚠ Warning: Failed to clear DNS cache: %v\n", err)
		fmt.Println("  DNS changes may take up to TTL seconds to propagate")
	}

	return nil
}

func runRecordList(ctx context.Context, cmd *cli.Command) error {
	if cmd.Args().Len() < 1 {
		return fmt.Errorf("zone name is required")
	}

	zoneName := cmd.Args().Get(0)
	configPath := cmd.String("config")
	filterType := cmd.String("type")

	// Ensure zone name ends with dot
	if !strings.HasSuffix(zoneName, ".") {
		zoneName = zoneName + "."
	}

	// Load configuration
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

	// List records
	recordSets, err := client.ListRecords(zoneName)
	if err != nil {
		return fmt.Errorf("failed to list records: %w", err)
	}

	if len(recordSets) == 0 {
		fmt.Printf("No records found in zone %s\n", zoneName)
		return nil
	}

	// Filter by type if specified
	filteredSets := recordSets
	if filterType != "" {
		filterType = strings.ToUpper(filterType)
		filteredSets = make([]dns.RecordSet, 0)
		for _, rs := range recordSets {
			if rs.Type == filterType {
				filteredSets = append(filteredSets, rs)
			}
		}
	}

	if len(filteredSets) == 0 {
		if filterType != "" {
			fmt.Printf("No %s records found in zone %s\n", filterType, zoneName)
		} else {
			fmt.Printf("No records found in zone %s\n", zoneName)
		}
		return nil
	}

	// Display records in table format
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "NAME\tTYPE\tTTL\tCONTENT")
	fmt.Fprintln(w, "----\t----\t---\t-------")
	for _, rs := range filteredSets {
		for _, record := range rs.Records {
			disabled := ""
			if record.Disabled {
				disabled = " (disabled)"
			}
			fmt.Fprintf(w, "%s\t%s\t%d\t%s%s\n", record.Name, record.Type, record.TTL, record.Content, disabled)
		}
	}
	w.Flush()

	return nil
}

func runRecordDelete(ctx context.Context, cmd *cli.Command) error {
	if cmd.Args().Len() < 3 {
		return fmt.Errorf("requires 3 arguments: <zone> <name> <type>")
	}

	zoneName := cmd.Args().Get(0)
	recordName := cmd.Args().Get(1)
	recordType := strings.ToUpper(cmd.Args().Get(2))
	configPath := cmd.String("config")
	skipConfirm := cmd.Bool("yes")

	// Ensure zone name ends with dot
	if !strings.HasSuffix(zoneName, ".") {
		zoneName = zoneName + "."
	}

	// Ensure record name ends with dot
	if !strings.HasSuffix(recordName, ".") {
		if recordName == "@" {
			recordName = zoneName
		} else if !strings.HasSuffix(recordName, zoneName) {
			recordName = recordName + "." + zoneName
		} else {
			recordName = recordName + "."
		}
	}

	// Load configuration
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
		fmt.Printf("WARNING: This will delete ALL %s records for %s\n", recordType, recordName)
		fmt.Print("Are you sure? (yes/no): ")
		var response string
		fmt.Scanln(&response)
		if strings.ToLower(response) != "yes" {
			fmt.Println("Deletion cancelled")
			return nil
		}
	}

	// Delete record
	fmt.Printf("Deleting %s record %s...\n", recordType, recordName)
	if err := client.DeleteRecord(zoneName, recordName, recordType); err != nil {
		return fmt.Errorf("failed to delete record: %w", err)
	}

	fmt.Printf("✓ Record deleted successfully\n")

	// Clear DNS cache for this zone so changes take effect immediately
	if err := clearDNSCache(cfg, zoneName); err != nil {
		// Don't fail if cache clear fails, just warn
		fmt.Printf("⚠ Warning: Failed to clear DNS cache: %v\n", err)
		fmt.Println("  DNS changes may take up to TTL seconds to propagate")
	}

	return nil
}

// clearDNSCache clears the PowerDNS Recursor cache for a specific zone
// This ensures DNS changes take effect immediately without waiting for TTL expiration
func clearDNSCache(cfg *config.Config, zoneName string) error {
	dnsAddr, err := cfg.GetPrimaryDNSAddress()
	if err != nil {
		return fmt.Errorf("DNS host not configured")
	}

	dnsHost := dnsAddr

	// Get host from registry
	hosts, err := host.List()
	if err != nil {
		return fmt.Errorf("failed to list hosts: %w", err)
	}

	var targetHost *host.Host
	for _, h := range hosts {
		if h.Address == dnsHost {
			targetHost = h
			break
		}
	}

	if targetHost == nil {
		return fmt.Errorf("DNS host %s not found in host registry", dnsHost)
	}

	// Connect to host
	configDir, err := config.GetConfigDir()
	if err != nil {
		return fmt.Errorf("failed to get config directory: %w", err)
	}

	keyStorage, err := ssh.GetKeyStorage(configDir, cfg.Cluster.Name)
	if err != nil {
		return fmt.Errorf("failed to create key storage: %w", err)
	}

	keyPair, err := keyStorage.Load(targetHost.Hostname)
	if err != nil {
		return fmt.Errorf("failed to load SSH key: %w", err)
	}

	authMethod, err := keyPair.AuthMethod()
	if err != nil {
		return fmt.Errorf("failed to create auth method: %w", err)
	}

	conn, err := ssh.Connect(&ssh.ConnectionOptions{
		Host:       targetHost.Address,
		Port:       targetHost.Port,
		User:       targetHost.User,
		AuthMethod: authMethod,
		Timeout:    10,
	})
	if err != nil {
		return fmt.Errorf("failed to connect to DNS host: %w", err)
	}
	defer conn.Close()

	// Clear cache for the zone
	cmd := fmt.Sprintf("sudo docker exec powerdns-recursor rec_control wipe-cache %s", zoneName)
	result, err := conn.Exec(cmd)
	if err != nil {
		return fmt.Errorf("failed to execute cache clear command: %w", err)
	}

	if result.ExitCode != 0 {
		return fmt.Errorf("cache clear command failed: %s", result.Stderr)
	}

	fmt.Printf("  Cache cleared for zone %s\n", zoneName)
	return nil
}
