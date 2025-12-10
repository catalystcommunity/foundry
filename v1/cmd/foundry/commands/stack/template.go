package stack

import (
	"context"
	"fmt"
	"os"

	"github.com/catalystcommunity/foundry/v1/internal/config"
	"github.com/urfave/cli/v3"
)

// TemplateCommand generates a template stack.yaml configuration file
var TemplateCommand = &cli.Command{
	Name:  "template",
	Usage: "Generate a template stack configuration file",
	Description: `Generates a template stack.yaml file with placeholder values.

The template file includes:
  - Placeholder values for all required fields (e.g., <CLUSTER_NAME>, <VM_IP>)
  - Comments explaining what each field is for
  - A 'template: true' flag that must be removed before installation

Workflow:
  1. foundry stack template          # Generate template
  2. Edit ~/.foundry/stack.yaml      # Replace placeholders
  3. Remove 'template: true' line    # Mark as ready
  4. foundry stack install           # Install using template values

The template provides a complete configuration structure without prompting,
making it easier to script installations and test the stack install command.`,
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:    "output",
			Aliases: []string{"o"},
			Usage:   "Output file path (default: ~/.foundry/stack.yaml)",
		},
		&cli.BoolFlag{
			Name:  "force",
			Usage: "Overwrite existing configuration file",
		},
	},
	Action: runStackTemplate,
}

func runStackTemplate(ctx context.Context, cmd *cli.Command) error {
	// Determine output path
	outputPath := cmd.String("output")
	if outputPath == "" {
		outputPath = config.DefaultConfigPath()
	}

	force := cmd.Bool("force")

	// Check if file already exists
	if _, err := os.Stat(outputPath); err == nil && !force {
		return fmt.Errorf("configuration file already exists at %s\n\nUse --force to overwrite", outputPath)
	}

	// Generate template content
	templateContent := generateTemplateYAML()

	// Ensure directory exists
	configDir, err := config.GetConfigDir()
	if err != nil {
		return fmt.Errorf("failed to get config directory: %w", err)
	}
	if err := os.MkdirAll(configDir, 0700); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}

	// Write template to file
	if err := os.WriteFile(outputPath, []byte(templateContent), 0600); err != nil {
		return fmt.Errorf("failed to write template file: %w", err)
	}

	fmt.Printf("✓ Template configuration created: %s\n\n", outputPath)
	fmt.Println("Next steps:")
	fmt.Println("  1. Edit the template file and replace all <PLACEHOLDER> values")
	fmt.Println("  2. Add hosts: foundry host add <hostname> <ip>")
	fmt.Println("  3. Run: foundry stack install")
	fmt.Println()
	fmt.Printf("To edit: vim %s\n", outputPath)

	return nil
}

func generateTemplateYAML() string {
	return `# Foundry Stack Configuration Template
#
# Replace all <PLACEHOLDER> values with your actual values.
# The stack install command will check for any remaining placeholders and prevent
# installation until all values are filled in.

# Cluster Configuration
cluster:
  # Cluster name - used for namespacing in OpenBAO and resource naming
  name: <CLUSTER_NAME>  # Example: "prod-cluster" or "dev-k8s"

  # Cluster domain - used for DNS zone creation
  domain: <CLUSTER_DOMAIN>  # Example: "catalyst.local" or "k8s.example.com"

  # Kubernetes VIP (Virtual IP for cluster API access)
  # This IP will be managed by kube-vip and should be different from host IPs
  vip: <K8S_VIP>  # Example: "10.16.0.43"

# Infrastructure Hosts
# Each host can have multiple roles assigned
# Roles: openbao, dns, zot, cluster-control-plane, cluster-worker
hosts:
  - hostname: <HOSTNAME>        # Example: "vm1" or "node1.example.com"
    address: <VM_IP>            # Example: "10.16.0.42"
    port: 22                    # SSH port
    user: <SSH_USER>            # Example: "root" or "ubuntu"
    roles:                      # Roles this host will serve
      - openbao                 # OpenBAO secrets management
      - dns                     # PowerDNS server
      - zot                     # Zot container registry
      - cluster-control-plane   # Kubernetes control plane node
    state: added                # Initial state (managed by foundry)

  # Add more hosts for HA or multi-node setups:
  # - hostname: <HOSTNAME2>
  #   address: <VM_IP2>
  #   port: 22
  #   user: <SSH_USER>
  #   roles:
  #     - cluster-worker        # Worker-only node
  #   state: added

# Network Configuration
network:
  # Network settings for your infrastructure
  gateway: <GATEWAY_IP>      # Example: "10.16.0.1" or "192.168.1.1"
  netmask: <NETMASK>         # Example: "255.255.0.0" or "255.255.255.0"

# DNS Configuration
dns:
  # DNS backend (currently only gsqlite3 is supported)
  backend: gsqlite3

  # API key (auto-generated and stored in OpenBAO during install)
  api_key: ${secret:dns:api_key}

  # DNS forwarders for external queries
  forwarders:
    - 8.8.8.8    # Google DNS
    - 1.1.1.1    # Cloudflare DNS

  # Infrastructure DNS zone (for openbao, zot, etc.)
  infrastructure_zones:
    - name: <CLUSTER_DOMAIN>  # Same as cluster.domain above
      public: false           # Internal-only zone

  # Kubernetes service DNS is handled by CoreDNS inside the cluster
  # External public zone example (optional - for exposing services publicly):
  # kubernetes_zones:
  #   - name: myk8spublicdomain.com
  #     public: true
  #     public_cname: my-ingress-loadbalancer.example.com

# Storage Configuration (optional - for Longhorn backend)
# Uncomment and fill in to use Longhorn distributed storage instead of local-path.
#
# storage:
#   backend: longhorn  # Options: local-path, nfs, longhorn
#   longhorn:
#     replica_count: 3
#     data_path: /var/lib/longhorn

# Component Configuration (minimal - auto-installed during stack install)
components:
  openbao:
    config: {}
  zot:
    config: {}
  k3s:
    config: {}

# Setup State (do not modify - managed by foundry)
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
`
}
