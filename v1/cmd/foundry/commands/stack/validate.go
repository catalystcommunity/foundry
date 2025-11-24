package stack

import (
	"context"
	"fmt"

	"github.com/catalystcommunity/foundry/v1/internal/component"
	"github.com/catalystcommunity/foundry/v1/internal/config"
	"github.com/catalystcommunity/foundry/v1/internal/network"
	"github.com/urfave/cli/v3"
)

// ValidateCommand handles the 'foundry stack validate' command
var ValidateCommand = &cli.Command{
	Name:  "validate",
	Usage: "Validate stack configuration without installing",
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:    "config",
			Aliases: []string{"c"},
			Usage:   "Path to configuration file",
		},
	},
	Action: runStackValidate,
}

func runStackValidate(ctx context.Context, cmd *cli.Command) error {
	// Load configuration
	configPath := cmd.String("config")
	if configPath == "" {
		configPath = config.DefaultConfigPath()
	}

	cfg, err := config.Load(configPath)
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	fmt.Println("Validating stack configuration...")
	fmt.Println()

	// Run all validation checks
	validations := []struct {
		name string
		fn   func(*config.Config) error
	}{
		{"Configuration structure", validateConfigStructure},
		{"Secret references syntax", validateSecretReferences},
		{"Network configuration", validateNetworkConfig},
		{"DNS configuration", validateDNSConfig},
		{"VIP configuration", validateVIPConfig},
		{"Cluster configuration", validateClusterConfig},
		{"Component dependencies", validateComponentDependencies},
	}

	for _, v := range validations {
		if err := v.fn(cfg); err != nil {
			fmt.Printf("✗ %s failed: %v\n", v.name, err)
			return err
		}
		fmt.Printf("✓ %s passed\n", v.name)
	}

	fmt.Println()
	fmt.Println("✓ All validation checks passed")
	fmt.Println()
	fmt.Println("Your stack configuration is valid and ready for installation.")
	fmt.Println("Run 'foundry stack install' to deploy the stack.")

	return nil
}

// validateConfigStructure validates the basic config structure
func validateConfigStructure(cfg *config.Config) error {
	// Use the built-in Validate method which checks all struct validations
	if err := cfg.Validate(); err != nil {
		return err
	}
	return nil
}

// validateSecretReferences validates all secret reference syntax
func validateSecretReferences(cfg *config.Config) error {
	if err := config.ValidateSecretRefs(cfg); err != nil {
		return fmt.Errorf("invalid secret reference: %w", err)
	}
	return nil
}

// validateNetworkConfig performs network-specific validations
func validateNetworkConfig(cfg *config.Config) error {
	if cfg.Network == nil {
		return fmt.Errorf("network configuration is required")
	}

	// Validate IP addresses are on the same network
	if err := network.ValidateIPs(cfg); err != nil {
		return err
	}

	// Check for DHCP conflicts
	if err := network.CheckDHCPConflicts(cfg); err != nil {
		return err
	}

	return nil
}

// validateDNSConfig performs DNS-specific validations
func validateDNSConfig(cfg *config.Config) error {
	if cfg.DNS == nil {
		return fmt.Errorf("dns configuration is required")
	}

	// DNS.Validate() is already called by cfg.Validate(), but we can add
	// additional checks here if needed

	// Verify at least one infrastructure zone
	if len(cfg.DNS.InfrastructureZones) == 0 {
		return fmt.Errorf("at least one infrastructure zone is required")
	}

	// Verify at least one kubernetes zone
	if len(cfg.DNS.KubernetesZones) == 0 {
		return fmt.Errorf("at least one kubernetes zone is required")
	}

	// Check that public zones all have the same public_cname (if multiple)
	var publicCNAME string
	allZones := append(cfg.DNS.InfrastructureZones, cfg.DNS.KubernetesZones...)
	for _, zone := range allZones {
		if zone.Public && zone.PublicCNAME != nil {
			if publicCNAME == "" {
				publicCNAME = *zone.PublicCNAME
			} else if *zone.PublicCNAME != publicCNAME {
				return fmt.Errorf("all public zones must have the same public_cname (found %q and %q)",
					publicCNAME, *zone.PublicCNAME)
			}
		}
	}

	return nil
}

// validateVIPConfig validates VIP configuration
func validateVIPConfig(cfg *config.Config) error {
	if cfg.Network == nil {
		return fmt.Errorf("network configuration is required")
	}

	// VIP is already validated in Network.Validate() and validateK8sVIPUniqueness()
	// but we can add additional checks here

	// Ensure VIP is set
	if cfg.Cluster.VIP == "" {
		return fmt.Errorf("cluster.vip is required")
	}

	return nil
}

// validateClusterConfig validates cluster configuration
func validateClusterConfig(cfg *config.Config) error {
	// Cluster.Validate() is already called by cfg.Validate()
	// Additional checks can be added here

	// Ensure at least one host with cluster role is defined
	clusterHosts := cfg.GetClusterHosts()
	if len(clusterHosts) == 0 {
		return fmt.Errorf("at least one host with cluster role (cluster-control-plane or cluster-worker) is required")
	}

	// Verify at least one control-plane host
	controlPlaneHosts := cfg.GetHostsByRole("cluster-control-plane")
	if len(controlPlaneHosts) == 0 {
		return fmt.Errorf("at least one host with cluster-control-plane role is required")
	}

	return nil
}

// validateComponentDependencies validates component dependencies can be resolved
func validateComponentDependencies(cfg *config.Config) error {
	// Get the installation order to verify dependencies can be resolved
	componentNames := []string{
		"openbao",
		"dns",
		"zot",
		"k3s",
		"contour",
		"certmanager",
	}

	order, err := component.ResolveInstallationOrder(component.DefaultRegistry, componentNames)
	if err != nil {
		return fmt.Errorf("dependency resolution failed: %w", err)
	}

	// Verify we got all components in the order
	if len(order) != len(componentNames) {
		return fmt.Errorf("dependency resolution incomplete: expected %d components, got %d",
			len(componentNames), len(order))
	}

	return nil
}
