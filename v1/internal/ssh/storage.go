package ssh

import (
	"fmt"
)

// KeyStorage defines the interface for storing and retrieving SSH keys
// This allows for different storage backends (OpenBAO, filesystem, etc.)
type KeyStorage interface {
	// Store saves an SSH key pair for a specific host
	Store(host string, key *KeyPair) error

	// Load retrieves an SSH key pair for a specific host
	Load(host string) (*KeyPair, error)

	// Delete removes an SSH key pair for a specific host
	Delete(host string) error

	// Exists checks if a key pair exists for a specific host
	Exists(host string) (bool, error)
}

// OpenBAOKeyStorage is a stub implementation that will store SSH keys in OpenBAO
// This is a placeholder for Phase 2 when OpenBAO integration is implemented
type OpenBAOKeyStorage struct {
	// addr is the OpenBAO server address
	addr string

	// token is the authentication token
	token string

	// basePath is the base path for storing SSH keys in OpenBAO
	// Expected format: foundry-core/ssh-keys/<hostname>
	basePath string
}

// NewOpenBAOKeyStorage creates a new OpenBAO key storage instance
func NewOpenBAOKeyStorage(addr, token string) (*OpenBAOKeyStorage, error) {
	if addr == "" {
		return nil, fmt.Errorf("OpenBAO address cannot be empty")
	}
	if token == "" {
		return nil, fmt.Errorf("OpenBAO token cannot be empty")
	}

	return &OpenBAOKeyStorage{
		addr:     addr,
		token:    token,
		basePath: "foundry-core/ssh-keys",
	}, nil
}

// Store is not yet implemented - will be implemented in Phase 2
func (s *OpenBAOKeyStorage) Store(host string, key *KeyPair) error {
	return fmt.Errorf("OpenBAO integration not yet implemented - planned for Phase 2")
}

// Load is not yet implemented - will be implemented in Phase 2
func (s *OpenBAOKeyStorage) Load(host string) (*KeyPair, error) {
	return nil, fmt.Errorf("OpenBAO integration not yet implemented - planned for Phase 2")
}

// Delete is not yet implemented - will be implemented in Phase 2
func (s *OpenBAOKeyStorage) Delete(host string) error {
	return fmt.Errorf("OpenBAO integration not yet implemented - planned for Phase 2")
}

// Exists is not yet implemented - will be implemented in Phase 2
func (s *OpenBAOKeyStorage) Exists(host string) (bool, error) {
	return false, fmt.Errorf("OpenBAO integration not yet implemented - planned for Phase 2")
}

// GetStoragePath returns the expected storage path for a given host
// This is documented for Phase 2 implementation
func (s *OpenBAOKeyStorage) GetStoragePath(host string) string {
	return fmt.Sprintf("%s/%s", s.basePath, host)
}
