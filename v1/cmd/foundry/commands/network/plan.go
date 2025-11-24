package network

import (
	"context"
	"fmt"
	"os"

	"github.com/catalystcommunity/foundry/v1/internal/config"
	"github.com/catalystcommunity/foundry/v1/internal/setup"
	"github.com/urfave/cli/v3"
)

// PlanCommand implements the 'foundry network plan' command
var PlanCommand = &cli.Command{
	Name:  "plan",
	Usage: "Interactively plan network configuration",
	Description: `Interactive wizard for network planning.

This command helps you plan your network configuration by:
- Prompting for gateway, netmask, and DHCP range
- Configuring the Kubernetes VIP
- Updating the configuration file

The wizard will prompt for:
  1. Network gateway (e.g., 192.168.1.1)
  2. Network netmask (e.g., 255.255.255.0)
  3. DHCP range (optional, e.g., 192.168.1.50 - 192.168.1.200)
  4. Kubernetes VIP (must be unique, outside DHCP range)

After completing the wizard:
  1. Add your infrastructure hosts using: foundry host add <hostname> <ip>
  2. Assign roles to hosts (openbao, dns, zot, cluster-control-plane, cluster-worker)
  3. Configure static IPs or DHCP reservations for your infrastructure hosts
  4. Use 'foundry network validate' to verify the configuration`,
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:    "config",
			Aliases: []string{"c"},
			Usage:   "Path to configuration file",
			Value:   config.DefaultConfigPath(),
		},
	},
	Action: runPlan,
}

func runPlan(ctx context.Context, cmd *cli.Command) error {
	configPath := cmd.String("config")

	// Load existing config or create new one
	cfg, err := config.Load(configPath)
	if err != nil {
		// If config doesn't exist, create a new one
		if os.IsNotExist(err) {
			cfg = &config.Config{}
		} else {
			return fmt.Errorf("failed to load config: %w", err)
		}
	}

	// Initialize network config if not present
	if cfg.Network == nil {
		cfg.Network = &config.NetworkConfig{}
	}

	// Run the interactive network planning wizard
	fmt.Println("Network Planning Wizard")
	fmt.Println("======================")
	fmt.Println()
	fmt.Println("This wizard will help you plan your network configuration.")
	fmt.Println("Press Ctrl+C to cancel at any time.")
	fmt.Println()

	// Prompt for gateway
	gateway, err := promptString("Network gateway", cfg.Network.Gateway, "192.168.1.1")
	if err != nil {
		return err
	}
	cfg.Network.Gateway = gateway

	// Prompt for netmask
	netmask, err := promptString("Network netmask", cfg.Network.Netmask, "255.255.255.0")
	if err != nil {
		return err
	}
	cfg.Network.Netmask = netmask

	// Prompt for DHCP range (optional)
	fmt.Println()
	fmt.Println("DHCP Range (optional)")
	fmt.Println("If you use DHCP, specify the range to avoid IP conflicts.")
	fmt.Println("Infrastructure hosts should use static IPs or DHCP reservations outside this range.")

	useDHCP, err := promptYesNo("Configure DHCP range?", cfg.Network.DHCPRange != nil)
	if err != nil {
		return err
	}

	if useDHCP {
		if cfg.Network.DHCPRange == nil {
			cfg.Network.DHCPRange = &config.DHCPRange{}
		}

		dhcpStart, err := promptString("DHCP range start", cfg.Network.DHCPRange.Start, "192.168.1.50")
		if err != nil {
			return err
		}
		cfg.Network.DHCPRange.Start = dhcpStart

		dhcpEnd, err := promptString("DHCP range end", cfg.Network.DHCPRange.End, "192.168.1.200")
		if err != nil {
			return err
		}
		cfg.Network.DHCPRange.End = dhcpEnd
	} else {
		cfg.Network.DHCPRange = nil
	}

	// Infrastructure hosts are now managed separately via 'foundry host add'
	fmt.Println()
	fmt.Println("Infrastructure Hosts")
	fmt.Println("In this schema, hosts are managed separately from network planning.")
	fmt.Println("After completing network planning, add your hosts using:")
	fmt.Println("  foundry host add <hostname> <ip-address>")
	fmt.Println()
	fmt.Println("Then assign roles to hosts (openbao, dns, zot, cluster-control-plane, cluster-worker)")
	fmt.Println("either via config file or during cluster initialization.")
	fmt.Println()

	// Kubernetes VIP
	fmt.Println()
	fmt.Println("Kubernetes VIP")
	fmt.Println("This is a virtual IP for the Kubernetes API server (kube-vip).")
	fmt.Println("It must be unique and not used by any infrastructure host.")
	if cfg.Network.DHCPRange != nil {
		fmt.Printf("It should be outside the DHCP range (%s - %s).\n",
			cfg.Network.DHCPRange.Start, cfg.Network.DHCPRange.End)
	}
	fmt.Println()

	k8sVIP, err := promptString("Kubernetes VIP", cfg.Cluster.VIP, "192.168.1.100")
	if err != nil {
		return err
	}
	cfg.Cluster.VIP = k8sVIP

	// TrueNAS configuration is now handled via host roles
	// No separate TrueNAS-specific prompts needed

	// Validate the network configuration
	fmt.Println()
	fmt.Println("Validating network configuration...")
	if err := cfg.Network.Validate(); err != nil {
		return fmt.Errorf("network configuration validation failed: %w", err)
	}

	// Update setup state to mark network as planned
	if cfg.SetupState == nil {
		cfg.SetupState = &setup.SetupState{}
	}
	cfg.SetupState.NetworkPlanned = true

	// Save the configuration
	if err := config.Save(cfg, configPath); err != nil {
		return fmt.Errorf("failed to save configuration: %w", err)
	}

	fmt.Println()
	fmt.Println("✓ Network configuration saved to", configPath)
	fmt.Println()
	fmt.Println("Next steps:")
	fmt.Println("  1. Add your infrastructure hosts using:")
	fmt.Println("       foundry host add <hostname> <ip-address>")
	fmt.Println("  2. Assign roles to hosts in the config file or during cluster init")
	fmt.Println("  3. Configure static IPs or DHCP reservations in your router/DHCP server")
	fmt.Println("     (This is YOUR responsibility - Foundry doesn't manage host IPs)")
	fmt.Println("  4. Run 'foundry network validate' to verify the configuration")

	return nil
}

// promptString prompts for a string value with an optional default
func promptString(prompt, current, defaultValue string) (string, error) {
	if current != "" {
		fmt.Printf("%s [%s]: ", prompt, current)
	} else if defaultValue != "" {
		fmt.Printf("%s [%s]: ", prompt, defaultValue)
	} else {
		fmt.Printf("%s: ", prompt)
	}

	var input string
	if _, err := fmt.Scanln(&input); err != nil {
		// If user just pressed Enter, use current or default
		if err.Error() == "unexpected newline" {
			if current != "" {
				return current, nil
			}
			if defaultValue != "" {
				return defaultValue, nil
			}
			return "", fmt.Errorf("value is required")
		}
		return "", err
	}

	if input == "" {
		if current != "" {
			return current, nil
		}
		if defaultValue != "" {
			return defaultValue, nil
		}
		return "", fmt.Errorf("value is required")
	}

	return input, nil
}

// promptYesNo prompts for a yes/no answer
func promptYesNo(prompt string, defaultValue bool) (bool, error) {
	var defaultStr string
	if defaultValue {
		defaultStr = "Y/n"
	} else {
		defaultStr = "y/N"
	}

	fmt.Printf("%s [%s]: ", prompt, defaultStr)

	var input string
	if _, err := fmt.Scanln(&input); err != nil {
		// If user just pressed Enter, use default
		if err.Error() == "unexpected newline" {
			return defaultValue, nil
		}
		return false, err
	}

	if input == "" {
		return defaultValue, nil
	}

	switch input {
	case "y", "Y", "yes", "Yes", "YES":
		return true, nil
	case "n", "N", "no", "No", "NO":
		return false, nil
	default:
		return false, fmt.Errorf("invalid input: %s (expected y/n)", input)
	}
}
