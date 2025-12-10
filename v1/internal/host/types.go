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
	Hostname  string            // Friendly name for the host
	Address   string            // IP address or FQDN
	Port      int               // SSH port (default 22)
	User      string            // SSH user
	SSHKeySet bool              // Whether an SSH key has been configured for this host
	Roles     []string          // Component roles (openbao, dns, zot, cluster-control-plane, cluster-worker)
	State     string            // Host state (added, ssh-configured, configured)
	Labels    map[string]string // Kubernetes node labels (optional)
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

	// Validate labels if present
	for key, value := range h.Labels {
		if err := ValidateLabelKey(key); err != nil {
			return fmt.Errorf("invalid label key %q: %w", key, err)
		}
		if err := ValidateLabelValue(value); err != nil {
			return fmt.Errorf("invalid label value for key %q: %w", key, err)
		}
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

// HasLabel checks if the host has a specific label
func (h *Host) HasLabel(key string) bool {
	if h.Labels == nil {
		return false
	}
	_, ok := h.Labels[key]
	return ok
}

// GetLabel returns the value of a label, or empty string if not present
func (h *Host) GetLabel(key string) string {
	if h.Labels == nil {
		return ""
	}
	return h.Labels[key]
}

// SetLabel sets a label on the host
func (h *Host) SetLabel(key, value string) {
	if h.Labels == nil {
		h.Labels = make(map[string]string)
	}
	h.Labels[key] = value
}

// RemoveLabel removes a label from the host
func (h *Host) RemoveLabel(key string) {
	if h.Labels != nil {
		delete(h.Labels, key)
	}
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
		Labels:    nil,        // No labels by default
	}
}

// Label validation constants
const (
	labelKeyMaxLength    = 63
	labelPrefixMaxLength = 253
	labelValueMaxLength  = 63
)

// labelNameRegex matches valid label name portion (after prefix/)
// Must be 63 characters or less, beginning and ending with alphanumeric,
// with dashes (-), underscores (_), dots (.), and alphanumerics between.
var labelNameRegex = regexp.MustCompile(`^[a-zA-Z0-9]([a-zA-Z0-9._-]{0,61}[a-zA-Z0-9])?$|^[a-zA-Z0-9]$`)

// labelPrefixRegex matches valid DNS subdomain for prefix portion
var labelPrefixRegex = regexp.MustCompile(`^[a-z0-9]([-a-z0-9]*[a-z0-9])?(\.[a-z0-9]([-a-z0-9]*[a-z0-9])?)*$`)

// labelValueRegex matches valid label values (can be empty)
var labelValueRegex = regexp.MustCompile(`^([a-zA-Z0-9]([a-zA-Z0-9._-]{0,61}[a-zA-Z0-9])?)?$`)

// ValidateLabelKey validates a Kubernetes label key
// Keys have an optional prefix and a required name, separated by a slash (/).
// The name must be 63 characters or less, beginning and ending with alphanumeric.
// The prefix is optional; if specified, it must be a DNS subdomain (max 253 chars).
func ValidateLabelKey(key string) error {
	if key == "" {
		return fmt.Errorf("label key cannot be empty")
	}

	// Check for prefix/name format
	parts := strings.SplitN(key, "/", 2)
	var prefix, name string

	if len(parts) == 2 {
		prefix = parts[0]
		name = parts[1]
	} else {
		name = parts[0]
	}

	// Validate prefix if present
	if prefix != "" {
		if len(prefix) > labelPrefixMaxLength {
			return fmt.Errorf("prefix must be %d characters or less", labelPrefixMaxLength)
		}
		if !labelPrefixRegex.MatchString(prefix) {
			return fmt.Errorf("prefix must be a valid DNS subdomain")
		}
	}

	// Validate name
	if name == "" {
		return fmt.Errorf("label name cannot be empty")
	}
	if len(name) > labelKeyMaxLength {
		return fmt.Errorf("name must be %d characters or less", labelKeyMaxLength)
	}
	if !labelNameRegex.MatchString(name) {
		return fmt.Errorf("name must begin and end with alphanumeric, and contain only alphanumerics, dashes, underscores, and dots")
	}

	return nil
}

// ValidateLabelValue validates a Kubernetes label value
// Values can be empty, or up to 63 characters, beginning and ending with alphanumeric,
// with dashes (-), underscores (_), dots (.), and alphanumerics between.
func ValidateLabelValue(value string) error {
	if len(value) > labelValueMaxLength {
		return fmt.Errorf("value must be %d characters or less", labelValueMaxLength)
	}
	if value != "" && !labelValueRegex.MatchString(value) {
		return fmt.Errorf("value must begin and end with alphanumeric, and contain only alphanumerics, dashes, underscores, and dots")
	}
	return nil
}
