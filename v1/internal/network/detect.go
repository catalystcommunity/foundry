package network

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/catalystcommunity/foundry/v1/internal/ssh"
)

// SSHExecutor is an interface for executing SSH commands
// This allows for easier testing with mocks
type SSHExecutor interface {
	Exec(command string) (*ssh.ExecResult, error)
}

// InterfaceInfo contains information about a network interface
type InterfaceInfo struct {
	Name      string
	MAC       string
	IP        string
	IsDefault bool
}

// DetectPrimaryInterface detects the primary network interface on the remote host
// This is typically the interface with the default route
func DetectPrimaryInterface(conn SSHExecutor) (string, error) {
	// Try to get the default route interface
	result, err := conn.Exec("ip route show default | head -n1 | awk '{print $5}'")
	if err != nil {
		return "", fmt.Errorf("failed to detect primary interface: %w", err)
	}

	if result.ExitCode != 0 {
		return "", fmt.Errorf("failed to detect primary interface: %s", result.Stderr)
	}

	iface := strings.TrimSpace(result.Stdout)
	if iface == "" {
		return "", fmt.Errorf("no default route interface found")
	}

	return iface, nil
}

// DetectMAC detects the MAC address for the specified interface
func DetectMAC(conn SSHExecutor, iface string) (string, error) {
	if iface == "" {
		return "", fmt.Errorf("interface name cannot be empty")
	}

	// Get the MAC address from /sys/class/net/<iface>/address
	cmd := fmt.Sprintf("cat /sys/class/net/%s/address", iface)
	result, err := conn.Exec(cmd)
	if err != nil {
		return "", fmt.Errorf("failed to detect MAC address: %w", err)
	}

	if result.ExitCode != 0 {
		return "", fmt.Errorf("failed to detect MAC address for %s: %s", iface, result.Stderr)
	}

	mac := strings.TrimSpace(result.Stdout)
	if mac == "" {
		return "", fmt.Errorf("no MAC address found for interface %s", iface)
	}

	// Validate MAC address format (basic check)
	macRegex := regexp.MustCompile(`^([0-9a-fA-F]{2}:){5}[0-9a-fA-F]{2}$`)
	if !macRegex.MatchString(mac) {
		return "", fmt.Errorf("invalid MAC address format: %s", mac)
	}

	return mac, nil
}

// DetectCurrentIP detects the current IP address for the specified interface
func DetectCurrentIP(conn SSHExecutor, iface string) (string, error) {
	if iface == "" {
		return "", fmt.Errorf("interface name cannot be empty")
	}

	// Get the IP address using ip addr show
	cmd := fmt.Sprintf("ip addr show %s | grep 'inet ' | head -n1 | awk '{print $2}' | cut -d'/' -f1", iface)
	result, err := conn.Exec(cmd)
	if err != nil {
		return "", fmt.Errorf("failed to detect IP address: %w", err)
	}

	if result.ExitCode != 0 {
		return "", fmt.Errorf("failed to detect IP address for %s: %s", iface, result.Stderr)
	}

	ip := strings.TrimSpace(result.Stdout)
	if ip == "" {
		return "", fmt.Errorf("no IP address found for interface %s", iface)
	}

	// Basic IPv4 validation
	ipRegex := regexp.MustCompile(`^(\d{1,3}\.){3}\d{1,3}$`)
	if !ipRegex.MatchString(ip) {
		return "", fmt.Errorf("invalid IP address format: %s", ip)
	}

	return ip, nil
}

// DetectInterface detects comprehensive interface information for the primary interface
func DetectInterface(conn SSHExecutor) (*InterfaceInfo, error) {
	// First, detect the primary interface
	iface, err := DetectPrimaryInterface(conn)
	if err != nil {
		return nil, err
	}

	// Then get MAC and IP
	mac, err := DetectMAC(conn, iface)
	if err != nil {
		return nil, fmt.Errorf("failed to detect MAC for %s: %w", iface, err)
	}

	ip, err := DetectCurrentIP(conn, iface)
	if err != nil {
		return nil, fmt.Errorf("failed to detect IP for %s: %w", iface, err)
	}

	return &InterfaceInfo{
		Name:      iface,
		MAC:       mac,
		IP:        ip,
		IsDefault: true,
	}, nil
}

// ListInterfaces lists all network interfaces on the remote host
func ListInterfaces(conn SSHExecutor) ([]*InterfaceInfo, error) {
	// Get list of interfaces (excluding loopback)
	result, err := conn.Exec("ip link show | grep '^[0-9]' | awk '{print $2}' | sed 's/:$//' | grep -v '^lo$'")
	if err != nil {
		return nil, fmt.Errorf("failed to list interfaces: %w", err)
	}

	if result.ExitCode != 0 {
		return nil, fmt.Errorf("failed to list interfaces: %s", result.Stderr)
	}

	ifaceNames := strings.Split(strings.TrimSpace(result.Stdout), "\n")
	if len(ifaceNames) == 0 || (len(ifaceNames) == 1 && ifaceNames[0] == "") {
		return nil, fmt.Errorf("no network interfaces found")
	}

	// Get the default interface name
	defaultIface, _ := DetectPrimaryInterface(conn)

	interfaces := make([]*InterfaceInfo, 0, len(ifaceNames))
	for _, name := range ifaceNames {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}

		info := &InterfaceInfo{
			Name:      name,
			IsDefault: name == defaultIface,
		}

		// Best effort to get MAC and IP
		if mac, err := DetectMAC(conn, name); err == nil {
			info.MAC = mac
		}

		if ip, err := DetectCurrentIP(conn, name); err == nil {
			info.IP = ip
		}

		interfaces = append(interfaces, info)
	}

	return interfaces, nil
}
