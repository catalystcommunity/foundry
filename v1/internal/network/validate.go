package network

import (
	"fmt"
	"net"
	"strings"

	"github.com/catalystcommunity/foundry/v1/internal/config"
)

// ValidateIPs validates IP addresses in the network configuration
func ValidateIPs(cfg *config.NetworkConfig) error {
	if cfg == nil {
		return fmt.Errorf("network config is nil")
	}

	// Gateway and netmask are already validated by config.Validate()
	// But we can do additional checks here

	// Validate that gateway is on the same network as other IPs
	network, err := GetNetworkCIDR(cfg.Gateway, cfg.Netmask)
	if err != nil {
		return fmt.Errorf("failed to calculate network CIDR: %w", err)
	}

	// Validate all infrastructure IPs are on the same network
	allIPs := []string{cfg.K8sVIP}
	allIPs = append(allIPs, cfg.OpenBAOHosts...)
	allIPs = append(allIPs, cfg.DNSHosts...)
	allIPs = append(allIPs, cfg.ZotHosts...)
	allIPs = append(allIPs, cfg.TrueNASHosts...)

	for _, ipStr := range allIPs {
		ip := net.ParseIP(ipStr)
		if ip == nil {
			return fmt.Errorf("invalid IP address: %s", ipStr)
		}

		if !network.Contains(ip) {
			return fmt.Errorf("IP %s is not in network %s", ipStr, network.String())
		}
	}

	return nil
}

// CheckReachability checks if the given IPs are reachable via ping
func CheckReachability(conn SSHExecutor, ips []string) error {
	if len(ips) == 0 {
		return nil
	}

	unreachable := []string{}
	for _, ip := range ips {
		// Use ping with count=1 and timeout=2 seconds
		cmd := fmt.Sprintf("ping -c 1 -W 2 %s > /dev/null 2>&1", ip)
		result, err := conn.Exec(cmd)
		if err != nil {
			return fmt.Errorf("failed to ping %s: %w", ip, err)
		}

		if result.ExitCode != 0 {
			unreachable = append(unreachable, ip)
		}
	}

	if len(unreachable) > 0 {
		return fmt.Errorf("unreachable IPs: %s", strings.Join(unreachable, ", "))
	}

	return nil
}

// CheckDHCPConflicts checks if any infrastructure IPs fall within the DHCP range
func CheckDHCPConflicts(cfg *config.NetworkConfig) error {
	if cfg == nil {
		return fmt.Errorf("network config is nil")
	}

	// If no DHCP range is configured, no conflicts possible
	if cfg.DHCPRange == nil {
		return nil
	}

	dhcpStart := net.ParseIP(cfg.DHCPRange.Start)
	dhcpEnd := net.ParseIP(cfg.DHCPRange.End)

	if dhcpStart == nil || dhcpEnd == nil {
		return fmt.Errorf("invalid DHCP range")
	}

	// Collect all infrastructure IPs
	allIPs := []string{cfg.K8sVIP}
	allIPs = append(allIPs, cfg.OpenBAOHosts...)
	allIPs = append(allIPs, cfg.DNSHosts...)
	allIPs = append(allIPs, cfg.ZotHosts...)
	allIPs = append(allIPs, cfg.TrueNASHosts...)

	conflicts := []string{}
	for _, ipStr := range allIPs {
		ip := net.ParseIP(ipStr)
		if ip == nil {
			continue
		}

		if isIPInRange(ip, dhcpStart, dhcpEnd) {
			conflicts = append(conflicts, ipStr)
		}
	}

	if len(conflicts) > 0 {
		return fmt.Errorf("infrastructure IPs within DHCP range (%s - %s): %s",
			cfg.DHCPRange.Start, cfg.DHCPRange.End, strings.Join(conflicts, ", "))
	}

	return nil
}

// ValidateDNSResolution validates that a hostname resolves to the expected IP
// This is used after PowerDNS is installed to verify DNS configuration
func ValidateDNSResolution(conn SSHExecutor, hostname string, expectedIP string) error {
	if hostname == "" {
		return fmt.Errorf("hostname is required")
	}

	if expectedIP == "" {
		return fmt.Errorf("expected IP is required")
	}

	// Use dig to query DNS (more reliable than nslookup)
	// If dig is not available, fall back to getent hosts
	cmd := fmt.Sprintf("dig +short %s || getent hosts %s | awk '{print $1}'", hostname, hostname)
	result, err := conn.Exec(cmd)
	if err != nil {
		return fmt.Errorf("failed to resolve %s: %w", hostname, err)
	}

	if result.ExitCode != 0 {
		return fmt.Errorf("failed to resolve %s: %s", hostname, result.Stderr)
	}

	resolvedIP := strings.TrimSpace(result.Stdout)
	if resolvedIP == "" {
		return fmt.Errorf("hostname %s did not resolve to any IP", hostname)
	}

	// Handle multiple IPs (take the first one)
	if strings.Contains(resolvedIP, "\n") {
		resolvedIP = strings.Split(resolvedIP, "\n")[0]
	}

	if resolvedIP != expectedIP {
		return fmt.Errorf("hostname %s resolved to %s, expected %s", hostname, resolvedIP, expectedIP)
	}

	return nil
}

// GetNetworkCIDR calculates the network CIDR from gateway and netmask
func GetNetworkCIDR(gateway, netmask string) (*net.IPNet, error) {
	gatewayIP := net.ParseIP(gateway)
	if gatewayIP == nil {
		return nil, fmt.Errorf("invalid gateway IP: %s", gateway)
	}

	netmaskIP := net.ParseIP(netmask)
	if netmaskIP == nil {
		return nil, fmt.Errorf("invalid netmask: %s", netmask)
	}

	// Convert netmask to IPMask
	mask := net.IPMask(netmaskIP.To4())
	if mask == nil {
		return nil, fmt.Errorf("invalid netmask format: %s", netmask)
	}

	// Create IPNet from gateway and mask
	return &net.IPNet{
		IP:   gatewayIP.Mask(mask),
		Mask: mask,
	}, nil
}

// isIPInRange checks if an IP is within the range [start, end]
func isIPInRange(ip, start, end net.IP) bool {
	// Convert to 4-byte representation for comparison
	ip4 := ip.To4()
	start4 := start.To4()
	end4 := end.To4()

	if ip4 == nil || start4 == nil || end4 == nil {
		return false
	}

	// Convert to uint32 for comparison
	ipInt := ipToUint32(ip4)
	startInt := ipToUint32(start4)
	endInt := ipToUint32(end4)

	return ipInt >= startInt && ipInt <= endInt
}

// ipToUint32 converts an IPv4 address to uint32
func ipToUint32(ip net.IP) uint32 {
	ip = ip.To4()
	if ip == nil {
		return 0
	}
	return uint32(ip[0])<<24 | uint32(ip[1])<<16 | uint32(ip[2])<<8 | uint32(ip[3])
}
