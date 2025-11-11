package host

import (
	"fmt"
	"sort"
	"sync"
)

// ConfigRegistry is a config-file-backed implementation of HostRegistry
// It persists hosts to a YAML config file
type ConfigRegistry struct {
	configPath string
	loader     ConfigLoader
	mu         sync.RWMutex
}

// ConfigLoader defines the interface for loading/saving config with hosts
type ConfigLoader interface {
	LoadHosts() ([]*Host, error)
	SaveHosts(hosts []*Host) error
}

// NewConfigRegistry creates a new config-backed host registry
func NewConfigRegistry(configPath string, loader ConfigLoader) *ConfigRegistry {
	return &ConfigRegistry{
		configPath: configPath,
		loader:     loader,
	}
}

// Add adds a new host to the registry and persists to config
func (r *ConfigRegistry) Add(host *Host) error {
	if host == nil {
		return fmt.Errorf("host cannot be nil")
	}

	if err := host.Validate(); err != nil {
		return fmt.Errorf("invalid host: %w", err)
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	// Load current hosts
	hosts, err := r.loader.LoadHosts()
	if err != nil {
		return fmt.Errorf("failed to load hosts: %w", err)
	}

	// Check if host already exists
	for _, h := range hosts {
		if h.Hostname == host.Hostname {
			return fmt.Errorf("host with hostname %s already exists", host.Hostname)
		}
	}

	// Add new host
	hostCopy := *host
	hosts = append(hosts, &hostCopy)

	// Save to config
	if err := r.loader.SaveHosts(hosts); err != nil {
		return fmt.Errorf("failed to save hosts: %w", err)
	}

	return nil
}

// Get retrieves a host by hostname
func (r *ConfigRegistry) Get(hostname string) (*Host, error) {
	if hostname == "" {
		return nil, fmt.Errorf("hostname cannot be empty")
	}

	r.mu.RLock()
	defer r.mu.RUnlock()

	hosts, err := r.loader.LoadHosts()
	if err != nil {
		return nil, fmt.Errorf("failed to load hosts: %w", err)
	}

	for _, h := range hosts {
		if h.Hostname == hostname {
			hostCopy := *h
			return &hostCopy, nil
		}
	}

	return nil, fmt.Errorf("host %s not found", hostname)
}

// List returns all registered hosts
func (r *ConfigRegistry) List() ([]*Host, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	hosts, err := r.loader.LoadHosts()
	if err != nil {
		return nil, fmt.Errorf("failed to load hosts: %w", err)
	}

	// Create copies to prevent external modifications
	hostsCopy := make([]*Host, len(hosts))
	for i, h := range hosts {
		hostCopy := *h
		hostsCopy[i] = &hostCopy
	}

	// Sort by hostname for consistent ordering
	sort.Slice(hostsCopy, func(i, j int) bool {
		return hostsCopy[i].Hostname < hostsCopy[j].Hostname
	})

	return hostsCopy, nil
}

// Remove removes a host from the registry
func (r *ConfigRegistry) Remove(hostname string) error {
	if hostname == "" {
		return fmt.Errorf("hostname cannot be empty")
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	hosts, err := r.loader.LoadHosts()
	if err != nil {
		return fmt.Errorf("failed to load hosts: %w", err)
	}

	// Find and remove host
	found := false
	newHosts := make([]*Host, 0, len(hosts))
	for _, h := range hosts {
		if h.Hostname == hostname {
			found = true
			continue
		}
		newHosts = append(newHosts, h)
	}

	if !found {
		return fmt.Errorf("host %s not found", hostname)
	}

	// Save updated list
	if err := r.loader.SaveHosts(newHosts); err != nil {
		return fmt.Errorf("failed to save hosts: %w", err)
	}

	return nil
}

// Update updates an existing host
func (r *ConfigRegistry) Update(host *Host) error {
	if host == nil {
		return fmt.Errorf("host cannot be nil")
	}

	if err := host.Validate(); err != nil {
		return fmt.Errorf("invalid host: %w", err)
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	hosts, err := r.loader.LoadHosts()
	if err != nil {
		return fmt.Errorf("failed to load hosts: %w", err)
	}

	// Find and update host
	found := false
	for i, h := range hosts {
		if h.Hostname == host.Hostname {
			hostCopy := *host
			hosts[i] = &hostCopy
			found = true
			break
		}
	}

	if !found {
		return fmt.Errorf("host %s not found", host.Hostname)
	}

	// Save updated list
	if err := r.loader.SaveHosts(hosts); err != nil {
		return fmt.Errorf("failed to save hosts: %w", err)
	}

	return nil
}

// Exists checks if a host exists in the registry
func (r *ConfigRegistry) Exists(hostname string) (bool, error) {
	if hostname == "" {
		return false, fmt.Errorf("hostname cannot be empty")
	}

	r.mu.RLock()
	defer r.mu.RUnlock()

	hosts, err := r.loader.LoadHosts()
	if err != nil {
		return false, fmt.Errorf("failed to load hosts: %w", err)
	}

	for _, h := range hosts {
		if h.Hostname == hostname {
			return true, nil
		}
	}

	return false, nil
}
