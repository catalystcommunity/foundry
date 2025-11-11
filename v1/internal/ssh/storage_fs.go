package ssh

import (
	"fmt"
	"os"
	"path/filepath"
)

// FilesystemKeyStorage stores SSH keys in the local filesystem
// This is used for interim storage before OpenBAO is available
type FilesystemKeyStorage struct {
	// basePath is the base directory for storing SSH keys
	// Default: ~/.foundry/keys/
	basePath string
}

// NewFilesystemKeyStorage creates a new filesystem key storage instance
func NewFilesystemKeyStorage(basePath string) (*FilesystemKeyStorage, error) {
	if basePath == "" {
		return nil, fmt.Errorf("basePath cannot be empty")
	}

	// Ensure base path exists with proper permissions
	if err := os.MkdirAll(basePath, 0700); err != nil {
		return nil, fmt.Errorf("failed to create keys directory: %w", err)
	}

	return &FilesystemKeyStorage{
		basePath: basePath,
	}, nil
}

// Store saves an SSH key pair for a specific host to the filesystem
func (s *FilesystemKeyStorage) Store(host string, key *KeyPair) error {
	if host == "" {
		return fmt.Errorf("host cannot be empty")
	}
	if key == nil {
		return fmt.Errorf("key pair cannot be nil")
	}
	if len(key.Private) == 0 {
		return fmt.Errorf("private key cannot be empty")
	}
	if len(key.Public) == 0 {
		return fmt.Errorf("public key cannot be empty")
	}

	// Create host directory
	hostDir := filepath.Join(s.basePath, host)
	if err := os.MkdirAll(hostDir, 0700); err != nil {
		return fmt.Errorf("failed to create host directory: %w", err)
	}

	// Write private key
	privateKeyPath := filepath.Join(hostDir, "id_ed25519")
	if err := os.WriteFile(privateKeyPath, key.Private, 0600); err != nil {
		return fmt.Errorf("failed to write private key: %w", err)
	}

	// Write public key
	publicKeyPath := filepath.Join(hostDir, "id_ed25519.pub")
	if err := os.WriteFile(publicKeyPath, key.Public, 0644); err != nil {
		// If public key write fails, try to clean up private key
		os.Remove(privateKeyPath)
		return fmt.Errorf("failed to write public key: %w", err)
	}

	return nil
}

// Load retrieves an SSH key pair for a specific host from the filesystem
func (s *FilesystemKeyStorage) Load(host string) (*KeyPair, error) {
	if host == "" {
		return nil, fmt.Errorf("host cannot be empty")
	}

	// Read private key
	privateKeyPath := filepath.Join(s.basePath, host, "id_ed25519")
	privateKey, err := os.ReadFile(privateKeyPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("SSH key for host %s not found", host)
		}
		return nil, fmt.Errorf("failed to read private key: %w", err)
	}

	// Read public key
	publicKeyPath := filepath.Join(s.basePath, host, "id_ed25519.pub")
	publicKey, err := os.ReadFile(publicKeyPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("public key for host %s not found", host)
		}
		return nil, fmt.Errorf("failed to read public key: %w", err)
	}

	return &KeyPair{
		Private: privateKey,
		Public:  publicKey,
	}, nil
}

// Delete removes an SSH key pair for a specific host from the filesystem
func (s *FilesystemKeyStorage) Delete(host string) error {
	if host == "" {
		return fmt.Errorf("host cannot be empty")
	}

	hostDir := filepath.Join(s.basePath, host)

	// Check if directory exists
	if _, err := os.Stat(hostDir); os.IsNotExist(err) {
		return fmt.Errorf("SSH key for host %s not found", host)
	}

	// Remove entire host directory
	if err := os.RemoveAll(hostDir); err != nil {
		return fmt.Errorf("failed to delete SSH key directory: %w", err)
	}

	return nil
}

// Exists checks if an SSH key pair exists for a specific host
func (s *FilesystemKeyStorage) Exists(host string) (bool, error) {
	if host == "" {
		return false, fmt.Errorf("host cannot be empty")
	}

	privateKeyPath := filepath.Join(s.basePath, host, "id_ed25519")

	if _, err := os.Stat(privateKeyPath); err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, fmt.Errorf("failed to check if key exists: %w", err)
	}

	return true, nil
}

// GetStoragePath returns the storage path for a given host
func (s *FilesystemKeyStorage) GetStoragePath(host string) string {
	return filepath.Join(s.basePath, host)
}
