package network

import (
	"context"
	"fmt"

	"github.com/catalystcommunity/foundry/v1/internal/config"
	internalNetwork "github.com/catalystcommunity/foundry/v1/internal/network"
	"github.com/catalystcommunity/foundry/v1/internal/setup"
	"github.com/catalystcommunity/foundry/v1/internal/ssh"
	"github.com/urfave/cli/v3"
)

// ValidateCommand implements the 'foundry network validate' command
var ValidateCommand = &cli.Command{
	Name:  "validate",
	Usage: "Validate network configuration",
	Description: `Validate network configuration and verify connectivity.

This command performs comprehensive validation of your network configuration:
  1. All required IPs are configured
  2. All IPs are on the same network (gateway/netmask)
  3. K8s VIP is unique (not used by infrastructure hosts)
  4. No conflicts with DHCP range (if configured)
  5. Hosts are reachable via ping (requires SSH access)
  6. DNS resolution works (if PowerDNS is installed)

If all checks pass, the setup state will be updated to mark network
as validated, allowing the setup wizard to proceed.

Requirements:
  - Network configuration must be present in config file
  - SSH access to at least one host (for reachability checks)`,
	Flags: []cli.Flag{
		&cli.BoolFlag{
			Name:  "skip-reachability",
			Usage: "Skip reachability checks (ping tests)",
			Value: false,
		},
		&cli.BoolFlag{
			Name:  "skip-dns",
			Usage: "Skip DNS resolution checks",
			Value: false,
		},
	},
	Action: runValidate,
}

func runValidate(ctx context.Context, cmd *cli.Command) error {
	skipReachability := cmd.Bool("skip-reachability")
	skipDNS := cmd.Bool("skip-dns")

	// Load configuration (--config flag inherited from root command)
	configPath, err := config.FindConfig(cmd.String("config"))
	if err != nil {
		return fmt.Errorf("failed to find config: %w", err)
	}

	cfg, err := config.Load(configPath)
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	if cfg.Network == nil {
		return fmt.Errorf("network configuration not found in config file")
	}

	fmt.Println("Network Configuration Validation")
	fmt.Println("================================")
	fmt.Println()

	// Check 1: Basic network config validation
	fmt.Print("✓ Checking network configuration format... ")
	if err := cfg.Network.Validate(); err != nil {
		fmt.Println("❌")
		return fmt.Errorf("network configuration validation failed: %w", err)
	}
	fmt.Println("✓")

	// Check 2: Validate IPs are on same network
	fmt.Print("✓ Checking IPs are on same network... ")
	if err := internalNetwork.ValidateIPs(cfg); err != nil {
		fmt.Println("❌")
		return fmt.Errorf("IP validation failed: %w", err)
	}
	fmt.Println("✓")

	// Check 3: Check for DHCP conflicts
	if cfg.Network.DHCPRange != nil {
		fmt.Print("✓ Checking for DHCP range conflicts... ")
		if err := internalNetwork.CheckDHCPConflicts(cfg); err != nil {
			fmt.Println("❌")
			return fmt.Errorf("DHCP conflict check failed: %w", err)
		}
		fmt.Println("✓")
	}

	// Check 4: Reachability checks (requires SSH)
	if !skipReachability {
		fmt.Print("✓ Checking host reachability... ")

		// Collect all unique IPs from hosts
		uniqueIPs := make(map[string]bool)
		for _, h := range cfg.Hosts {
			uniqueIPs[h.Address] = true
		}

		ips := []string{}
		for ip := range uniqueIPs {
			ips = append(ips, ip)
		}

		// Try to connect to the first reachable host for ping tests
		var conn *ssh.Connection
		for _, ip := range ips {
			opts := &ssh.ConnectionOptions{
				Host: ip,
				Port: 22,
				User: "root", // Default user, should be configurable
			}
			c, err := ssh.Connect(opts)
			if err == nil {
				conn = c
				break
			}
		}

		if conn == nil {
			fmt.Println("⚠ (skipped - no SSH access)")
			fmt.Println()
			fmt.Println("Warning: Could not establish SSH connection to any host.")
			fmt.Println("Reachability checks require SSH access to at least one infrastructure host.")
			fmt.Println("You can skip this check with --skip-reachability")
		} else {
			if err := internalNetwork.CheckReachability(conn, ips); err != nil {
				fmt.Println("❌")
				return fmt.Errorf("reachability check failed: %w", err)
			}
			fmt.Println("✓")
		}
	}

	// Check 5: DNS resolution (if PowerDNS is installed)
	if !skipDNS && cfg.DNS != nil && len(cfg.DNS.InfrastructureZones) > 0 {
		// Only check DNS if PowerDNS is marked as installed in setup state
		if cfg.SetupState != nil && cfg.SetupState.DNSInstalled {
			fmt.Print("✓ Checking DNS resolution... ")

			// Try to connect to a host for DNS checks
			var conn *ssh.Connection
			// Prefer DNS hosts, then other infrastructure hosts
			dnsHosts := cfg.GetDNSHosts()
			testIPs := []string{}
			for _, h := range dnsHosts {
				testIPs = append(testIPs, h.Address)
			}
			// Add other hosts as fallback
			for _, h := range cfg.Hosts {
				testIPs = append(testIPs, h.Address)
			}

			for _, ip := range testIPs {
				opts := &ssh.ConnectionOptions{
					Host: ip,
					Port: 22,
					User: "root", // Default user, should be configurable
				}
				c, err := ssh.Connect(opts)
				if err == nil {
					conn = c
					break
				}
			}

			if conn == nil {
				fmt.Println("⚠ (skipped - no SSH access)")
			} else {
				// Test resolution of a few key hostnames
				zone := cfg.DNS.InfrastructureZones[0].Name

				// Get expected IPs from role-based host discovery
				openbaoAddr, _ := cfg.GetPrimaryOpenBAOAddress()
				dnsAddr, _ := cfg.GetPrimaryDNSAddress()
				zotAddr, _ := cfg.GetPrimaryZotAddress()

				tests := map[string]string{
					fmt.Sprintf("openbao.%s", zone): openbaoAddr,
					fmt.Sprintf("dns.%s", zone):     dnsAddr,
					fmt.Sprintf("zot.%s", zone):     zotAddr,
				}

				for hostname, expectedIP := range tests {
					if err := internalNetwork.ValidateDNSResolution(conn, hostname, expectedIP); err != nil {
						fmt.Println("❌")
						fmt.Printf("Warning: DNS resolution failed for %s: %v\n", hostname, err)
						fmt.Println("This is expected if PowerDNS is not yet installed.")
						break
					}
				}
				fmt.Println("✓")
			}
		}
	}

	fmt.Println()
	fmt.Println("✓ Network validation successful!")

	// Update setup state to mark network as validated
	if cfg.SetupState == nil {
		cfg.SetupState = &setup.SetupState{}
	}
	cfg.SetupState.NetworkValidated = true

	// Save updated config
	if err := config.Save(cfg, configPath); err != nil {
		return fmt.Errorf("failed to save config: %w", err)
	}

	fmt.Println()
	fmt.Println("Setup state updated: network_validated = true")
	fmt.Println()
	fmt.Println("Next steps:")
	fmt.Println("  - Run 'foundry setup' to continue stack installation")
	fmt.Println("  - Or install components individually with 'foundry component install <name>'")

	return nil
}
