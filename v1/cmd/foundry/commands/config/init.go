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
			Usage: "use defaults without prompting",
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
		// Prompt for configuration values
		content, err = promptForConfigValues()
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

func promptForConfigValues() (string, error) {
	fmt.Println("Creating a new Foundry stack configuration")
	fmt.Println("(Press Enter to accept defaults)")
	fmt.Println()

	clusterName, err := prompt("Cluster name", "my-cluster")
	if err != nil {
		return "", err
	}

	domain, err := prompt("Domain", "cluster.local")
	if err != nil {
		return "", err
	}

	vip, err := prompt("VIP (Virtual IP for K3s API)", "192.168.1.100")
	if err != nil {
		return "", err
	}

	gateway, err := prompt("Network gateway", "192.168.1.1")
	if err != nil {
		return "", err
	}

	netmask, err := prompt("Network netmask", "255.255.255.0")
	if err != nil {
		return "", err
	}

	// Generate YAML with user-provided values
	config := fmt.Sprintf(`# Foundry Stack Configuration
# Generated with user-provided values

cluster:
  name: %s
  domain: %s
  vip: %s

network:
  gateway: %s
  netmask: %s

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
`, clusterName, domain, vip, gateway, netmask, domain)

	return config, nil
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
