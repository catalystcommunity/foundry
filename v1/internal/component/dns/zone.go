package dns

import (
	"fmt"
	"strings"
)

const (
	// Default TTL for DNS records (1 hour)
	DefaultTTL = 3600

	// SOA record defaults
	DefaultSOAMName   = "ns1"
	DefaultSOARName   = "hostmaster"
	DefaultSOASerial  = 1
	DefaultSOARefresh = 3600
	DefaultSOARetry   = 600
	DefaultSOAExpire  = 604800
	DefaultSOAMinTTL  = 86400
)

// ZoneType represents the type of DNS zone
type ZoneType string

const (
	// ZoneTypeNative is a master zone
	ZoneTypeNative ZoneType = "Native"
	// ZoneTypeMaster is an alias for Native (PowerDNS uses both)
	ZoneTypeMaster ZoneType = "Master"
)

// ZoneConfig represents configuration for creating a DNS zone
type ZoneConfig struct {
	// Name is the zone name (e.g., "example.com")
	Name string
	// Type is the zone type (Native/Master)
	Type ZoneType
	// IsPublic indicates if this zone should be accessible externally
	IsPublic bool
	// PublicCNAME is the DDNS hostname for external queries (required if IsPublic)
	PublicCNAME string
	// Nameservers is the list of NS records to add
	Nameservers []string
}

// Validate checks if the zone configuration is valid
func (z *ZoneConfig) Validate() error {
	if z.Name == "" {
		return fmt.Errorf("zone name cannot be empty")
	}

	// Ensure zone name ends with a dot
	if !strings.HasSuffix(z.Name, ".") {
		z.Name = z.Name + "."
	}

	// .local zones cannot be public
	if strings.HasSuffix(z.Name, ".local.") && z.IsPublic {
		return fmt.Errorf("zone %s: .local zones cannot be public", z.Name)
	}

	// Public zones must have a PublicCNAME
	if z.IsPublic && z.PublicCNAME == "" {
		return fmt.Errorf("zone %s: public zones must have a public_cname", z.Name)
	}

	// Default to Native type
	if z.Type == "" {
		z.Type = ZoneTypeNative
	}

	return nil
}

// CreateInfrastructureZone creates a DNS zone for infrastructure services
// Infrastructure zones contain static A records for core services like OpenBAO, Zot, etc.
func CreateInfrastructureZone(client *Client, config ZoneConfig) error {
	if err := config.Validate(); err != nil {
		return fmt.Errorf("invalid zone config: %w", err)
	}

	// Create the zone
	if err := client.CreateZone(config.Name, string(config.Type)); err != nil {
		return fmt.Errorf("failed to create zone %s: %w", config.Name, err)
	}

	// Add SOA record
	if err := AddSOARecord(client, config.Name); err != nil {
		return fmt.Errorf("failed to add SOA record to zone %s: %w", config.Name, err)
	}

	// Add NS records
	if len(config.Nameservers) > 0 {
		for _, ns := range config.Nameservers {
			if err := AddNSRecord(client, config.Name, ns); err != nil {
				return fmt.Errorf("failed to add NS record to zone %s: %w", config.Name, err)
			}
		}
	} else {
		// Add default NS record
		zoneName := config.Name
		if strings.HasSuffix(zoneName, ".") {
			zoneName = zoneName[:len(zoneName)-1]
		}
		defaultNS := fmt.Sprintf("%s.%s", DefaultSOAMName, zoneName)
		if err := AddNSRecord(client, config.Name, defaultNS); err != nil {
			return fmt.Errorf("failed to add default NS record to zone %s: %w", config.Name, err)
		}
	}

	return nil
}

// CreateKubernetesZone creates a DNS zone for Kubernetes services
// Kubernetes zones typically have wildcard records and are managed by External-DNS
func CreateKubernetesZone(client *Client, config ZoneConfig) error {
	if err := config.Validate(); err != nil {
		return fmt.Errorf("invalid zone config: %w", err)
	}

	// Create the zone
	if err := client.CreateZone(config.Name, string(config.Type)); err != nil {
		return fmt.Errorf("failed to create zone %s: %w", config.Name, err)
	}

	// Add SOA record
	if err := AddSOARecord(client, config.Name); err != nil {
		return fmt.Errorf("failed to add SOA record to zone %s: %w", config.Name, err)
	}

	// Add NS records
	if len(config.Nameservers) > 0 {
		for _, ns := range config.Nameservers {
			if err := AddNSRecord(client, config.Name, ns); err != nil {
				return fmt.Errorf("failed to add NS record to zone %s: %w", config.Name, err)
			}
		}
	} else {
		// Add default NS record
		zoneName := config.Name
		if strings.HasSuffix(zoneName, ".") {
			zoneName = zoneName[:len(zoneName)-1]
		}
		defaultNS := fmt.Sprintf("%s.%s", DefaultSOAMName, zoneName)
		if err := AddNSRecord(client, config.Name, defaultNS); err != nil {
			return fmt.Errorf("failed to add default NS record to zone %s: %w", config.Name, err)
		}
	}

	return nil
}

// AddSOARecord adds a Start of Authority record to a zone
func AddSOARecord(client *Client, zone string) error {
	// Ensure zone ends with a dot
	if !strings.HasSuffix(zone, ".") {
		zone = zone + "."
	}

	// SOA record format: mname rname serial refresh retry expire minimum
	// Ensure zone has no trailing dot for concatenation, then add it back
	zoneName := zone
	if strings.HasSuffix(zoneName, ".") {
		zoneName = zoneName[:len(zoneName)-1]
	}
	mname := fmt.Sprintf("%s.%s.", DefaultSOAMName, zoneName)
	rname := fmt.Sprintf("%s.%s.", DefaultSOARName, zoneName)

	content := fmt.Sprintf("%s %s %d %d %d %d %d",
		mname,
		rname,
		DefaultSOASerial,
		DefaultSOARefresh,
		DefaultSOARetry,
		DefaultSOAExpire,
		DefaultSOAMinTTL,
	)

	if err := client.AddRecord(zone, zone, "SOA", content, DefaultTTL); err != nil {
		return fmt.Errorf("failed to add SOA record: %w", err)
	}

	return nil
}

// AddNSRecord adds a nameserver record to a zone
func AddNSRecord(client *Client, zone, nameserver string) error {
	// Ensure zone ends with a dot
	if !strings.HasSuffix(zone, ".") {
		zone = zone + "."
	}

	// Ensure nameserver ends with a dot
	if !strings.HasSuffix(nameserver, ".") {
		nameserver = nameserver + "."
	}

	if err := client.AddRecord(zone, zone, "NS", nameserver, DefaultTTL); err != nil {
		return fmt.Errorf("failed to add NS record: %w", err)
	}

	return nil
}

// AddWildcardRecord adds a wildcard A record to a zone
// This is useful for Kubernetes zones where all services should resolve to the same IP
func AddWildcardRecord(client *Client, zone, ip string) error {
	// Ensure zone ends with a dot
	if !strings.HasSuffix(zone, ".") {
		zone = zone + "."
	}

	// Wildcard name
	name := fmt.Sprintf("*.%s", zone)

	if err := client.AddRecord(zone, name, "A", ip, DefaultTTL); err != nil {
		return fmt.Errorf("failed to add wildcard record: %w", err)
	}

	return nil
}

// AddARecord adds an A record to a zone
func AddARecord(client *Client, zone, name, ip string) error {
	// Ensure zone ends with a dot
	if !strings.HasSuffix(zone, ".") {
		zone = zone + "."
	}

	// Build FQDN
	fqdn := name
	if !strings.HasSuffix(name, ".") {
		if strings.HasSuffix(name, zone[:len(zone)-1]) {
			// Name already includes zone
			fqdn = name + "."
		} else {
			// Append zone to name
			fqdn = fmt.Sprintf("%s.%s", name, zone)
		}
	}

	if err := client.AddRecord(zone, fqdn, "A", ip, DefaultTTL); err != nil {
		return fmt.Errorf("failed to add A record: %w", err)
	}

	return nil
}

// AddCNAMERecord adds a CNAME record to a zone
func AddCNAMERecord(client *Client, zone, name, target string) error {
	// Ensure zone ends with a dot
	if !strings.HasSuffix(zone, ".") {
		zone = zone + "."
	}

	// Build FQDN for name
	fqdn := name
	if !strings.HasSuffix(name, ".") {
		if strings.HasSuffix(name, zone[:len(zone)-1]) {
			// Name already includes zone
			fqdn = name + "."
		} else {
			// Append zone to name
			fqdn = fmt.Sprintf("%s.%s", name, zone)
		}
	}

	// Ensure target ends with a dot
	if !strings.HasSuffix(target, ".") {
		target = target + "."
	}

	if err := client.AddRecord(zone, fqdn, "CNAME", target, DefaultTTL); err != nil {
		return fmt.Errorf("failed to add CNAME record: %w", err)
	}

	return nil
}

// InfrastructureRecordConfig defines the infrastructure services for DNS records
type InfrastructureRecordConfig struct {
	// Zone is the DNS zone name (e.g., "infraexample.com")
	Zone string
	// OpenBAOIP is the IP address for OpenBAO service
	OpenBAOIP string
	// DNSIP is the IP address for PowerDNS service
	DNSIP string
	// ZotIP is the IP address for Zot registry service
	ZotIP string
	// K8sVIP is the virtual IP for Kubernetes API
	K8sVIP string
	// IsPublic indicates if split-horizon is configured
	IsPublic bool
	// PublicCNAME is the DDNS hostname for external queries (required if IsPublic)
	PublicCNAME string
}

// Validate checks if the infrastructure record configuration is valid
func (i *InfrastructureRecordConfig) Validate() error {
	if i.Zone == "" {
		return fmt.Errorf("zone is required")
	}
	if i.OpenBAOIP == "" {
		return fmt.Errorf("openbao_ip is required")
	}
	if i.DNSIP == "" {
		return fmt.Errorf("dns_ip is required")
	}
	if i.ZotIP == "" {
		return fmt.Errorf("zot_ip is required")
	}
	if i.K8sVIP == "" {
		return fmt.Errorf("k8s_vip is required")
	}

	// Ensure zone ends with a dot
	if !strings.HasSuffix(i.Zone, ".") {
		i.Zone = i.Zone + "."
	}

	return nil
}

// InitializeInfrastructureDNS creates all infrastructure DNS records
// This includes A records for: openbao, dns, zot, and k8s
// For public zones with split-horizon, CNAME records are also added for external access
func InitializeInfrastructureDNS(client *Client, config InfrastructureRecordConfig) error {
	if err := config.Validate(); err != nil {
		return fmt.Errorf("invalid infrastructure config: %w", err)
	}

	// Add A record for OpenBAO: openbao.<zone> -> OpenBAO IP
	if err := AddARecord(client, config.Zone, "openbao", config.OpenBAOIP); err != nil {
		return fmt.Errorf("failed to add openbao A record: %w", err)
	}

	// Add A record for DNS: dns.<zone> -> DNS IP
	if err := AddARecord(client, config.Zone, "dns", config.DNSIP); err != nil {
		return fmt.Errorf("failed to add dns A record: %w", err)
	}

	// Add A record for Zot: zot.<zone> -> Zot IP
	if err := AddARecord(client, config.Zone, "zot", config.ZotIP); err != nil {
		return fmt.Errorf("failed to add zot A record: %w", err)
	}

	// Add A record for K8s: k8s.<zone> -> K8s VIP
	if err := AddARecord(client, config.Zone, "k8s", config.K8sVIP); err != nil {
		return fmt.Errorf("failed to add k8s A record: %w", err)
	}

	// If public zone with split-horizon, add CNAME records for external access
	// Note: Split-horizon logic (internal vs external response) is handled by PowerDNS
	// configuration, not by duplicate records. The CNAME records here serve as fallback
	// or for documentation purposes. Actual split-horizon is configured via PowerDNS
	// recursor settings and zones.
	//
	// For now, we're just adding the A records. Full split-horizon implementation
	// will be handled by PowerDNS configuration in the install.go logic.

	return nil
}

// KubernetesZoneConfig defines configuration for initializing a Kubernetes DNS zone
type KubernetesZoneConfig struct {
	// Zone is the DNS zone name (e.g., "k8sexample.com")
	Zone string
	// K8sVIP is the virtual IP for Kubernetes ingress
	K8sVIP string
	// IsPublic indicates if split-horizon is configured
	IsPublic bool
	// PublicCNAME is the DDNS hostname for external queries (required if IsPublic)
	PublicCNAME string
}

// Validate checks if the kubernetes zone configuration is valid
func (k *KubernetesZoneConfig) Validate() error {
	if k.Zone == "" {
		return fmt.Errorf("zone is required")
	}
	if k.K8sVIP == "" {
		return fmt.Errorf("k8s_vip is required")
	}

	// Ensure zone ends with a dot
	if !strings.HasSuffix(k.Zone, ".") {
		k.Zone = k.Zone + "."
	}

	return nil
}

// InitializeKubernetesDNS creates the Kubernetes DNS zone with wildcard record
// This zone will be populated by External-DNS in Phase 3
func InitializeKubernetesDNS(client *Client, config KubernetesZoneConfig) error {
	if err := config.Validate(); err != nil {
		return fmt.Errorf("invalid kubernetes zone config: %w", err)
	}

	// Add wildcard A record: *.<zone> -> K8s VIP
	// This ensures all kubernetes services resolve to the ingress VIP
	if err := AddWildcardRecord(client, config.Zone, config.K8sVIP); err != nil {
		return fmt.Errorf("failed to add wildcard record: %w", err)
	}

	// External-DNS will populate this zone with specific service records in Phase 3

	return nil
}
