package host

import (
	"fmt"
	"net"
	"regexp"
	"strings"
)

// Valid host roles
const (
	RoleOpenBAO             = "openbao"
	RoleDNS                 = "dns"
	RoleZot                 = "zot"
	RoleClusterControlPlane = "cluster-control-plane"
	RoleClusterWorker       = "cluster-worker"
)

// Valid host states
const (
	StateAdded         = "added"          // Host added to registry, SSH key generated
	StateSSHConfigured = "ssh-configured" // SSH key installed, sudo configured
	StateConfigured    = "configured"     // All assigned roles installed and healthy
)

// ValidRoles returns the list of all valid role strings
func ValidRoles() []string {
	return []string{
		RoleOpenBAO,
		RoleDNS,
		RoleZot,
		RoleClusterControlPlane,
		RoleClusterWorker,
	}
}

// ValidStates returns the list of all valid state strings
func ValidStates() []string {
	return []string{
		StateAdded,
		StateSSHConfigured,
		StateConfigured,
	}
}

// Host represents a managed host in the infrastructure
type Host struct {
	Hostname  string   // Friendly name for the host
	Address   string   // IP address or FQDN
	Port      int      // SSH port (default 22)
	User      string   // SSH user
	SSHKeySet bool     // Whether an SSH key has been configured for this host
	Roles     []string // Component roles (openbao, dns, zot, cluster-control-plane, cluster-worker)
	State     string   // Host state (added, ssh-configured, configured)
}

// HostRegistry defines the interface for managing hosts
type HostRegistry interface {
	// Add adds a new host to the registry
	Add(host *Host) error

	// Get retrieves a host by hostname
	Get(hostname string) (*Host, error)

	// List returns all registered hosts
	List() ([]*Host, error)

	// Remove removes a host from the registry
	Remove(hostname string) error

	// Update updates an existing host
	Update(host *Host) error

	// Exists checks if a host exists in the registry
	Exists(hostname string) (bool, error)
}

// Validate validates the host configuration
func (h *Host) Validate() error {
	if h.Hostname == "" {
		return fmt.Errorf("hostname cannot be empty")
	}

	// Validate hostname format (alphanumeric, hyphens, dots)
	if !isValidHostname(h.Hostname) {
		return fmt.Errorf("invalid hostname format: %s", h.Hostname)
	}

	if h.Address == "" {
		return fmt.Errorf("address cannot be empty")
	}

	// Validate address is either a valid IP or hostname
	if !isValidAddress(h.Address) {
		return fmt.Errorf("invalid address format: %s", h.Address)
	}

	if h.Port <= 0 || h.Port > 65535 {
		return fmt.Errorf("port must be between 1 and 65535, got %d", h.Port)
	}

	if h.User == "" {
		return fmt.Errorf("user cannot be empty")
	}

	// Validate roles
	validRoles := map[string]bool{
		RoleOpenBAO:             true,
		RoleDNS:                 true,
		RoleZot:                 true,
		RoleClusterControlPlane: true,
		RoleClusterWorker:       true,
	}
	for _, role := range h.Roles {
		if !validRoles[role] {
			return fmt.Errorf("invalid role: %s (valid roles: %s)", role, strings.Join(ValidRoles(), ", "))
		}
	}

	// Validate state
	validStates := map[string]bool{
		StateAdded:         true,
		StateSSHConfigured: true,
		StateConfigured:    true,
	}
	if h.State != "" && !validStates[h.State] {
		return fmt.Errorf("invalid state: %s (valid states: %s)", h.State, strings.Join(ValidStates(), ", "))
	}

	return nil
}

// HasRole checks if the host has a specific role
func (h *Host) HasRole(role string) bool {
	for _, r := range h.Roles {
		if r == role {
			return true
		}
	}
	return false
}

// AddRole adds a role to the host if not already present
func (h *Host) AddRole(role string) {
	if !h.HasRole(role) {
		h.Roles = append(h.Roles, role)
	}
}

// RemoveRole removes a role from the host
func (h *Host) RemoveRole(role string) {
	roles := make([]string, 0, len(h.Roles))
	for _, r := range h.Roles {
		if r != role {
			roles = append(roles, r)
		}
	}
	h.Roles = roles
}

// isValidHostname checks if a hostname is valid
// Allows alphanumeric characters, hyphens, and dots
func isValidHostname(hostname string) bool {
	// Hostname can contain letters, numbers, hyphens, and dots
	// Must start with alphanumeric, end with alphanumeric
	// Cannot have consecutive dots
	pattern := `^[a-zA-Z0-9]([a-zA-Z0-9\-]{0,61}[a-zA-Z0-9])?(\.[a-zA-Z0-9]([a-zA-Z0-9\-]{0,61}[a-zA-Z0-9])?)*$`
	matched, _ := regexp.MatchString(pattern, hostname)
	return matched
}

// isValidAddress checks if an address is a valid IP address or hostname
func isValidAddress(address string) bool {
	// Check if it's a valid IP address
	if net.ParseIP(address) != nil {
		return true
	}

	// Check if it's a valid hostname
	return isValidHostname(address)
}

// String returns a string representation of the host
func (h *Host) String() string {
	keyStatus := "no key"
	if h.SSHKeySet {
		keyStatus = "key set"
	}
	return fmt.Sprintf("%s (%s@%s:%d) [%s]", h.Hostname, h.User, h.Address, h.Port, keyStatus)
}

// DefaultHost creates a host with default values
func DefaultHost(hostname, address, user string) *Host {
	return &Host{
		Hostname:  hostname,
		Address:   address,
		Port:      22,
		User:      user,
		SSHKeySet: false,
		Roles:     []string{}, // Empty by default, set during host add
		State:     "added",    // Initial state
	}
}
