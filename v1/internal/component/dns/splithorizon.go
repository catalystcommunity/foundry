package dns

import (
	"fmt"
	"net"
	"strings"
)

// RFC1918 private IP ranges
var privateIPRanges = []string{
	"10.0.0.0/8",
	"172.16.0.0/12",
	"192.168.0.0/16",
}

// IsInternalQuery determines if a source IP is from an internal network
func IsInternalQuery(sourceIP string, internalRanges []string) (bool, error) {
	ip := net.ParseIP(sourceIP)
	if ip == nil {
		return false, fmt.Errorf("invalid IP address: %s", sourceIP)
	}

	// If custom internal ranges are provided, use them
	ranges := internalRanges
	if len(ranges) == 0 {
		// Default to RFC1918 ranges
		ranges = privateIPRanges
	}

	for _, cidr := range ranges {
		_, ipNet, err := net.ParseCIDR(cidr)
		if err != nil {
			return false, fmt.Errorf("invalid CIDR range %s: %w", cidr, err)
		}
		if ipNet.Contains(ip) {
			return true, nil
		}
	}

	return false, nil
}

// GenerateCNAMERecord creates a CNAME record for external queries
// This points to the public DDNS hostname (e.g., home.example.com)
func GenerateCNAMERecord(publicCNAME string) string {
	// Ensure CNAME ends with a dot for DNS
	if !strings.HasSuffix(publicCNAME, ".") {
		return publicCNAME + "."
	}
	return publicCNAME
}

// GenerateARecord creates an A record for internal queries
// This returns the local IP address directly
func GenerateARecord(localIP string) (string, error) {
	// Validate IP address
	ip := net.ParseIP(localIP)
	if ip == nil {
		return "", fmt.Errorf("invalid IP address: %s", localIP)
	}

	// Ensure it's an IPv4 address
	if ip.To4() == nil {
		return "", fmt.Errorf("not an IPv4 address: %s", localIP)
	}

	return ip.String(), nil
}

// SplitHorizonConfig represents the configuration for split-horizon DNS
type SplitHorizonConfig struct {
	// PublicCNAME is the DDNS hostname for external queries
	PublicCNAME string
	// LocalIP is the internal IP address for internal queries
	LocalIP string
	// InternalRanges are custom CIDR ranges to consider as internal
	// If empty, defaults to RFC1918 ranges
	InternalRanges []string
}

// DetermineRecordContent returns the appropriate record content based on query source
// For internal queries, returns A record with local IP
// For external queries, returns CNAME to public hostname
func DetermineRecordContent(sourceIP string, config SplitHorizonConfig) (recordType, content string, err error) {
	isInternal, err := IsInternalQuery(sourceIP, config.InternalRanges)
	if err != nil {
		return "", "", err
	}

	if isInternal {
		// Internal query - return A record with local IP
		aRecord, err := GenerateARecord(config.LocalIP)
		if err != nil {
			return "", "", err
		}
		return "A", aRecord, nil
	}

	// External query - return CNAME to public hostname
	return "CNAME", GenerateCNAMERecord(config.PublicCNAME), nil
}
