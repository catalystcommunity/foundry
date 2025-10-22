package host

import (
	"fmt"
	"sort"
	"sync"
)

// MemoryRegistry is an in-memory implementation of HostRegistry
// This is suitable for Phase 1 development and testing
// In later phases, this may be replaced with persistent storage
type MemoryRegistry struct {
	hosts map[string]*Host
	mu    sync.RWMutex
}

// NewMemoryRegistry creates a new in-memory host registry
func NewMemoryRegistry() *MemoryRegistry {
	return &MemoryRegistry{
		hosts: make(map[string]*Host),
	}
}

// Add adds a new host to the registry
func (r *MemoryRegistry) Add(host *Host) error {
	if host == nil {
		return fmt.Errorf("host cannot be nil")
	}

	if err := host.Validate(); err != nil {
		return fmt.Errorf("invalid host: %w", err)
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	// Check if host already exists
	if _, exists := r.hosts[host.Hostname]; exists {
		return fmt.Errorf("host with hostname %s already exists", host.Hostname)
	}

	// Create a copy to avoid external modifications
	hostCopy := *host
	r.hosts[host.Hostname] = &hostCopy

	return nil
}

// Get retrieves a host by hostname
func (r *MemoryRegistry) Get(hostname string) (*Host, error) {
	if hostname == "" {
		return nil, fmt.Errorf("hostname cannot be empty")
	}

	r.mu.RLock()
	defer r.mu.RUnlock()

	host, exists := r.hosts[hostname]
	if !exists {
		return nil, fmt.Errorf("host %s not found", hostname)
	}

	// Return a copy to prevent external modifications
	hostCopy := *host
	return &hostCopy, nil
}

// List returns all registered hosts
func (r *MemoryRegistry) List() ([]*Host, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	hosts := make([]*Host, 0, len(r.hosts))
	for _, host := range r.hosts {
		// Create copies to prevent external modifications
		hostCopy := *host
		hosts = append(hosts, &hostCopy)
	}

	// Sort by hostname for consistent ordering
	sort.Slice(hosts, func(i, j int) bool {
		return hosts[i].Hostname < hosts[j].Hostname
	})

	return hosts, nil
}

// Remove removes a host from the registry
func (r *MemoryRegistry) Remove(hostname string) error {
	if hostname == "" {
		return fmt.Errorf("hostname cannot be empty")
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.hosts[hostname]; !exists {
		return fmt.Errorf("host %s not found", hostname)
	}

	delete(r.hosts, hostname)
	return nil
}

// Update updates an existing host
func (r *MemoryRegistry) Update(host *Host) error {
	if host == nil {
		return fmt.Errorf("host cannot be nil")
	}

	if err := host.Validate(); err != nil {
		return fmt.Errorf("invalid host: %w", err)
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.hosts[host.Hostname]; !exists {
		return fmt.Errorf("host %s not found", host.Hostname)
	}

	// Create a copy to avoid external modifications
	hostCopy := *host
	r.hosts[host.Hostname] = &hostCopy

	return nil
}

// Exists checks if a host exists in the registry
func (r *MemoryRegistry) Exists(hostname string) (bool, error) {
	if hostname == "" {
		return false, fmt.Errorf("hostname cannot be empty")
	}

	r.mu.RLock()
	defer r.mu.RUnlock()

	_, exists := r.hosts[hostname]
	return exists, nil
}

// Count returns the number of hosts in the registry
func (r *MemoryRegistry) Count() int {
	r.mu.RLock()
	defer r.mu.RUnlock()

	return len(r.hosts)
}

// Clear removes all hosts from the registry
// This is primarily useful for testing
func (r *MemoryRegistry) Clear() {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.hosts = make(map[string]*Host)
}
