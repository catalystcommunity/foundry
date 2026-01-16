package config

import (
	"bufio"
	"context"
	_ "embed"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/urfave/cli/v3"
)

//go:embed template.yaml
var configTemplate string

// InitCommand creates a new config file interactively
var InitCommand = &cli.Command{
	Name:  "init",
	Usage: "Create a new configuration file",
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:    "name",
			Aliases: []string{"n"},
			Usage:   "name for the config file (without .yaml extension)",
		},
		&cli.BoolFlag{
			Name:    "force",
			Aliases: []string{"f"},
			Usage:   "overwrite existing config file",
		},
		&cli.BoolFlag{
			Name:  "non-interactive",
			Usage: "use defaults without prompting (uses template with placeholders)",
		},
		&cli.StringFlag{
			Name:  "cluster-name",
			Usage: "cluster name (e.g., my-cluster)",
		},
		&cli.StringFlag{
			Name:  "primary-domain",
			Usage: "primary domain for the cluster (e.g., catalyst.local)",
		},
		&cli.StringSliceFlag{
			Name:  "kubernetes-zones",
			Usage: "additional kubernetes DNS zones (can be specified multiple times)",
		},
		&cli.StringFlag{
			Name:  "vip",
			Usage: "virtual IP for K3s API (e.g., 192.168.1.100)",
		},
		&cli.StringFlag{
			Name:  "gateway",
			Usage: "network gateway (e.g., 192.168.1.1)",
		},
		&cli.StringFlag{
			Name:  "netmask",
			Usage: "network netmask (e.g., 255.255.255.0)",
		},
	},
	Action: runInit,
}

func runInit(ctx context.Context, cmd *cli.Command) error {
	// Get home directory
	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("failed to get home directory: %w", err)
	}

	configDir := filepath.Join(home, ".foundry")

	// Ensure config directory exists
	if err := os.MkdirAll(configDir, 0755); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}

	// Determine config name
	configName := cmd.String("name")
	if configName == "" {
		if cmd.Bool("non-interactive") {
			configName = "stack"
		} else {
			configName, err = prompt("Config name", "stack")
			if err != nil {
				return err
			}
		}
	}

	// Build config file path
	configPath := filepath.Join(configDir, configName+".yaml")

	// Check if file exists
	if _, err := os.Stat(configPath); err == nil && !cmd.Bool("force") {
		return fmt.Errorf("config file already exists: %s (use --force to overwrite)", configPath)
	}

	var content string
	if cmd.Bool("non-interactive") {
		// Use template with placeholders
		content = configTemplate
	} else {
		// Prompt for configuration values (flags override prompts)
		content, err = promptForConfigValues(cmd)
		if err != nil {
			return err
		}
	}

	// Write config to file
	if err := os.WriteFile(configPath, []byte(content), 0644); err != nil {
		return fmt.Errorf("failed to write config file: %w", err)
	}

	fmt.Printf("Created config file: %s\n", configPath)
	if cmd.Bool("non-interactive") {
		fmt.Println("\nNext steps:")
		fmt.Println("  1. Edit the config file and replace placeholder values")
		fmt.Println("  2. Add your hosts with appropriate roles")
		fmt.Println("  3. Run: foundry stack install")
	} else {
		fmt.Println("\nNext steps:")
		fmt.Println("  1. Add your hosts to the config file")
		fmt.Println("  2. Run: foundry stack install")
	}
	return nil
}

func promptForConfigValues(cmd *cli.Command) (string, error) {
	// Check if all required flags are provided (fully non-interactive)
	allFlagsProvided := cmd.String("cluster-name") != "" &&
		cmd.String("primary-domain") != "" &&
		cmd.String("vip") != "" &&
		cmd.String("gateway") != "" &&
		cmd.String("netmask") != ""

	if !allFlagsProvided {
		fmt.Println("Creating a new Foundry stack configuration")
		fmt.Println("(Press Enter to accept defaults)")
		fmt.Println()
	}

	clusterName, err := promptOrFlag(cmd, "cluster-name", "Cluster name", "my-cluster")
	if err != nil {
		return "", err
	}

	primaryDomain, err := promptOrFlag(cmd, "primary-domain", "Primary domain", "catalyst.local")
	if err != nil {
		return "", err
	}

	// Collect kubernetes zones - primary domain is always first
	kubernetesZones := []string{primaryDomain}

	// Add any zones from flags
	flagZones := cmd.StringSlice("kubernetes-zones")
	if len(flagZones) > 0 {
		kubernetesZones = append(kubernetesZones, flagZones...)
	} else if !allFlagsProvided {
		// Only prompt for additional zones if not all flags were provided
		fmt.Println()
		fmt.Println("You can add additional domains for Kubernetes services.")
		fmt.Println("These will be configured as DNS zones where external-dns can create records.")
		fmt.Println()

		for {
			addMore, err := prompt("Add another Kubernetes domain? (y/n)", "n")
			if err != nil {
				return "", err
			}
			if strings.ToLower(addMore) != "y" && strings.ToLower(addMore) != "yes" {
				break
			}

			additionalDomain, err := prompt("Additional domain", "")
			if err != nil {
				return "", err
			}
			if additionalDomain != "" {
				kubernetesZones = append(kubernetesZones, additionalDomain)
				fmt.Printf("  Added: %s\n", additionalDomain)
			}
		}
	}

	vip, err := promptOrFlag(cmd, "vip", "VIP (Virtual IP for K3s API)", "192.168.1.100")
	if err != nil {
		return "", err
	}

	gateway, err := promptOrFlag(cmd, "gateway", "Network gateway", "192.168.1.1")
	if err != nil {
		return "", err
	}

	netmask, err := promptOrFlag(cmd, "netmask", "Network netmask", "255.255.255.0")
	if err != nil {
		return "", err
	}

	// Format zones as YAML arrays with proper structure
	infrastructureZonesYAML := formatYAMLZoneArray([]string{primaryDomain})
	kubernetesZonesYAML := formatYAMLZoneArray(kubernetesZones)

	// Generate YAML with user-provided values
	config := fmt.Sprintf(`# Foundry Stack Configuration
# Generated with user-provided values

cluster:
  name: %s
  primary_domain: %s
  vip: %s

network:
  gateway: %s
  netmask: %s

dns:
  backend: sqlite
  api_key: ${secret:foundry-core/dns:api_key}
  forwarders:
    - 1.1.1.1
    - 8.8.8.8
  # Infrastructure zones get A records for openbao, dns, zot, k8s
  infrastructure_zones:
%s
  # Kubernetes zones get wildcard records and are managed by external-dns
  kubernetes_zones:
%s

# Add your infrastructure hosts below
# Each host needs roles assigned from:
#   - cluster-control-plane: K3s control plane
#   - cluster-worker: K3s worker
#   - openbao: Secrets management
#   - dns: PowerDNS server
#   - zot: Container registry
hosts:
  # Example single-node configuration:
  # - hostname: node1.%s
  #   address: 192.168.1.20
  #   port: 22
  #   user: foundry
  #   roles:
  #     - cluster-control-plane
  #     - openbao
  #     - dns
  #     - zot
  #   state: new

components:
  openbao:
    version: latest

  k3s:
    version: latest
    ha: false  # Set to true for multi-control-plane

  zot:
    version: latest

# Optional: Observability configuration (Phase 3)
# observability:
#   prometheus:
#     retention: 30d
#   loki:
#     retention: 90d

# Optional: Storage configuration (Phase 3)
# storage:
#   backend: longhorn  # Options: local-path, nfs, longhorn
#   longhorn:
#     replica_count: 3
#     data_path: /var/lib/longhorn

setup_state:
  network_planned: false
  network_validated: false
  openbao_installed: false
  openbao_initialized: false
  dns_installed: false
  dns_zones_created: false
  zot_installed: false
  k8s_installed: false
  stack_complete: false
`, clusterName, primaryDomain, vip, gateway, netmask, infrastructureZonesYAML, kubernetesZonesYAML, primaryDomain)

	return config, nil
}

// formatYAMLZoneArray formats a slice of zone names as indented YAML zone objects
func formatYAMLZoneArray(zones []string) string {
	var lines []string
	for _, zone := range zones {
		lines = append(lines, fmt.Sprintf("    - name: %s\n      public: false", zone))
	}
	return strings.Join(lines, "\n")
}

func prompt(question, defaultValue string) (string, error) {
	reader := bufio.NewReader(os.Stdin)
	fmt.Printf("%s [%s]: ", question, defaultValue)

	input, err := reader.ReadString('\n')
	if err != nil {
		return "", fmt.Errorf("failed to read input: %w", err)
	}

	input = strings.TrimSpace(input)
	if input == "" {
		return defaultValue, nil
	}

	return input, nil
}

// promptOrFlag returns the flag value if set, otherwise prompts the user
func promptOrFlag(cmd *cli.Command, flagName, question, defaultValue string) (string, error) {
	if value := cmd.String(flagName); value != "" {
		return value, nil
	}
	return prompt(question, defaultValue)
}
