package host

import "sync"

// Package-level global registry for host management
// This provides a centralized registry that can be accessed from any package
// Production code must call SetDefaultRegistry() before using host operations
var (
	defaultRegistry HostRegistry = nil
	registryMu      sync.RWMutex
)

// GetDefaultRegistry returns the global default host registry
func GetDefaultRegistry() HostRegistry {
	registryMu.RLock()
	defer registryMu.RUnlock()
	return defaultRegistry
}

// SetDefaultRegistry sets the global default host registry
// This is primarily useful for testing or custom implementations
func SetDefaultRegistry(r HostRegistry) {
	registryMu.Lock()
	defer registryMu.Unlock()
	defaultRegistry = r
}

// Get retrieves a host by hostname from the global registry
func Get(hostname string) (*Host, error) {
	return GetDefaultRegistry().Get(hostname)
}

// Add adds a new host to the global registry
func Add(host *Host) error {
	return GetDefaultRegistry().Add(host)
}

// List returns all registered hosts from the global registry
func List() ([]*Host, error) {
	return GetDefaultRegistry().List()
}

// Remove removes a host from the global registry
func Remove(hostname string) error {
	return GetDefaultRegistry().Remove(hostname)
}

// Update updates an existing host in the global registry
func Update(host *Host) error {
	return GetDefaultRegistry().Update(host)
}

// Exists checks if a host exists in the global registry
func Exists(hostname string) (bool, error) {
	return GetDefaultRegistry().Exists(hostname)
}

// ClearHosts removes all hosts from the global registry
// This is primarily useful for testing
func ClearHosts() {
	if mr, ok := defaultRegistry.(*MemoryRegistry); ok {
		mr.Clear()
	}
}
