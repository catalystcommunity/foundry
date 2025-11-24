package dns

import (
	"context"
	"fmt"
	"net"
	"strings"
	"time"

	"github.com/catalystcommunity/foundry/v1/internal/component/dns"
	"github.com/catalystcommunity/foundry/v1/internal/config"
	"github.com/catalystcommunity/foundry/v1/internal/secrets"
	"github.com/urfave/cli/v3"
)

// TestCommand is the dns test command
var TestCommand = &cli.Command{
	Name:      "test",
	Usage:     "Test DNS resolution",
	ArgsUsage: "<hostname>",
	Description: `Test DNS resolution for a hostname.

This command queries PowerDNS to verify that DNS resolution is working correctly.
It shows:
  - Whether the hostname resolves
  - The IP address(es) returned
  - Which zone answered the query
  - Whether split-horizon DNS is working (if configured)

Examples:
  foundry dns test openbao.infraexample.com
  foundry dns test www.k8sexample.com`,
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:    "config",
			Aliases: []string{"c"},
			Usage:   "Path to configuration file",
			Value:   config.DefaultConfigPath(),
		},
		&cli.StringFlag{
			Name:  "type",
			Usage: "Record type to query (A, AAAA, CNAME, etc.)",
			Value: "A",
		},
	},
	Action: runTest,
}

func runTest(ctx context.Context, cmd *cli.Command) error {
	if cmd.Args().Len() < 1 {
		return fmt.Errorf("hostname is required")
	}

	hostname := cmd.Args().Get(0)
	configPath := cmd.String("config")
	recordType := strings.ToUpper(cmd.String("type"))

	// Ensure hostname ends with dot for FQDN
	fqdn := hostname
	if !strings.HasSuffix(fqdn, ".") {
		fqdn = fqdn + "."
	}

	// Load configuration
	cfg, err := config.Load(configPath)
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	if cfg.DNS == nil {
		return fmt.Errorf("DNS configuration not found in config file")
	}

	// Get DNS host from network config
	dnsAddr, err := cfg.GetPrimaryDNSAddress()
	if err != nil {
		return fmt.Errorf("DNS host not configured in network section")
	}
	dnsHost := dnsAddr

	fmt.Printf("Testing DNS resolution for: %s\n", hostname)
	fmt.Printf("Query type: %s\n", recordType)
	fmt.Printf("DNS server: %s\n", dnsHost)
	fmt.Println()

	// Test 1: Use system DNS resolver
	fmt.Println("Test 1: System DNS resolution")
	fmt.Println("------------------------------")

	resolver := &net.Resolver{
		PreferGo: true,
		Dial: func(ctx context.Context, network, address string) (net.Conn, error) {
			d := net.Dialer{
				Timeout: 5 * time.Second,
			}
			return d.DialContext(ctx, network, fmt.Sprintf("%s:53", dnsHost))
		},
	}

	ips, err := resolver.LookupHost(context.Background(), hostname)
	if err != nil {
		fmt.Printf("❌ Resolution failed: %v\n", err)
	} else {
		fmt.Printf("✓ Resolved to: %s\n", strings.Join(ips, ", "))
	}
	fmt.Println()

	// Test 2: Query PowerDNS API to see which zone answered
	fmt.Println("Test 2: PowerDNS API query")
	fmt.Println("--------------------------")

	// Resolve API key from secrets
	secretResolver := secrets.NewChainResolver(
		secrets.NewEnvResolver(),
	)
	resCtx := &secrets.ResolutionContext{
		Instance: "foundry-core",
	}

	// Parse the secret reference if it's in ${secret:...} format
	apiKeyStr := cfg.DNS.APIKey
	var apiKey string
	if parsed, err := secrets.ParseSecretRef(apiKeyStr); err == nil && parsed != nil {
		apiKey, err = secretResolver.Resolve(resCtx, *parsed)
		if err != nil {
			return fmt.Errorf("failed to resolve DNS API key: %w", err)
		}
	} else {
		// Not a secret reference, use as-is
		apiKey = apiKeyStr
	}

	// Create PowerDNS client
	client := dns.NewClient(fmt.Sprintf("http://%s:8081", dnsHost), apiKey)

	// Find which zone contains this hostname
	zones, err := client.ListZones()
	if err != nil {
		return fmt.Errorf("failed to list zones: %w", err)
	}

	var matchingZone *dns.Zone
	for _, zone := range zones {
		if strings.HasSuffix(fqdn, zone.Name) {
			matchingZone = &zone
			break
		}
	}

	if matchingZone == nil {
		fmt.Printf("❌ No zone found for hostname %s\n", hostname)
		fmt.Println("\nAvailable zones:")
		for _, zone := range zones {
			fmt.Printf("  - %s\n", zone.Name)
		}
		return nil
	}

	fmt.Printf("✓ Zone: %s (type: %s, serial: %d)\n", matchingZone.Name, matchingZone.Type, matchingZone.Serial)

	// List records in the zone to find our hostname
	recordSets, err := client.ListRecords(matchingZone.Name)
	if err != nil {
		return fmt.Errorf("failed to list records: %w", err)
	}

	found := false
	for _, rs := range recordSets {
		if rs.Type == recordType {
			for _, record := range rs.Records {
				if record.Name == fqdn {
					found = true
					disabled := ""
					if record.Disabled {
						disabled = " (DISABLED)"
					}
					fmt.Printf("✓ Record found: %s %s %d %s%s\n",
						record.Name, record.Type, record.TTL, record.Content, disabled)
				}
			}
		}
	}

	if !found {
		fmt.Printf("❌ No %s record found for %s in zone %s\n", recordType, fqdn, matchingZone.Name)
		fmt.Println("\nTry:")
		fmt.Printf("  foundry dns record list %s\n", matchingZone.Name)
	}

	// Test 3: Check for split-horizon configuration
	fmt.Println()
	fmt.Println("Test 3: Split-horizon DNS check")
	fmt.Println("--------------------------------")

	isPublic := false
	var publicCNAME string

	// Check DNS config for split-horizon zones
	if cfg.DNS.InfrastructureZones != nil {
		for _, zone := range cfg.DNS.InfrastructureZones {
			if strings.HasSuffix(fqdn, zone.Name+".") && zone.Public {
				isPublic = true
				if zone.PublicCNAME != nil {
					publicCNAME = *zone.PublicCNAME
				}
				break
			}
		}
	}

	if cfg.DNS.KubernetesZones != nil {
		for _, zone := range cfg.DNS.KubernetesZones {
			if strings.HasSuffix(fqdn, zone.Name+".") && zone.Public {
				isPublic = true
				if zone.PublicCNAME != nil {
					publicCNAME = *zone.PublicCNAME
				}
				break
			}
		}
	}

	if isPublic {
		fmt.Printf("✓ Split-horizon enabled for this zone\n")
		fmt.Printf("  Public CNAME: %s\n", publicCNAME)
		fmt.Println("  Internal queries: Return local IP")
		fmt.Println("  External queries: CNAME to public hostname")
	} else {
		fmt.Println("✓ No split-horizon configuration (private zone)")
	}

	return nil
}
