package ssh

import (
	"context"
	"encoding/base64"
	"fmt"
	"strings"

	"github.com/catalystcommunity/foundry/v1/internal/component/openbao"
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

// OpenBAOKeyStorage stores SSH keys in OpenBAO KV v2 secrets engine
type OpenBAOKeyStorage struct {
	// client is the OpenBAO API client
	client *openbao.Client

	// basePath is the base path for storing SSH keys in OpenBAO
	// Expected format: foundry-core/ssh-keys/<hostname>
	// The path is split into mount (foundry-core) and path (ssh-keys/<hostname>)
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

	client := openbao.NewClient(addr, token)

	return &OpenBAOKeyStorage{
		client:   client,
		basePath: "foundry-core/ssh-keys",
	}, nil
}

// Store saves an SSH key pair for a specific host in OpenBAO
// Keys are base64-encoded before storage for safe transport
func (s *OpenBAOKeyStorage) Store(host string, key *KeyPair) error {
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

	// Parse the base path to get mount and path
	// Format: foundry-core/ssh-keys/<hostname>
	mount, path := s.parsePath(host)

	// Base64 encode the keys for safe storage
	data := map[string]interface{}{
		"private_key": base64.StdEncoding.EncodeToString(key.Private),
		"public_key":  base64.StdEncoding.EncodeToString(key.Public),
	}

	ctx := context.Background()
	if err := s.client.WriteSecretV2(ctx, mount, path, data); err != nil {
		return fmt.Errorf("failed to store SSH key in OpenBAO: %w", err)
	}

	return nil
}

// Load retrieves an SSH key pair for a specific host from OpenBAO
func (s *OpenBAOKeyStorage) Load(host string) (*KeyPair, error) {
	if host == "" {
		return nil, fmt.Errorf("host cannot be empty")
	}

	// Parse the base path to get mount and path
	mount, path := s.parsePath(host)

	ctx := context.Background()
	data, err := s.client.ReadSecretV2(ctx, mount, path)
	if err != nil {
		return nil, fmt.Errorf("failed to load SSH key from OpenBAO: %w", err)
	}

	// Extract and decode the keys
	privateKeyStr, ok := data["private_key"].(string)
	if !ok {
		return nil, fmt.Errorf("private_key not found or invalid type in secret")
	}

	publicKeyStr, ok := data["public_key"].(string)
	if !ok {
		return nil, fmt.Errorf("public_key not found or invalid type in secret")
	}

	// Decode from base64
	privateKey, err := base64.StdEncoding.DecodeString(privateKeyStr)
	if err != nil {
		return nil, fmt.Errorf("failed to decode private key: %w", err)
	}

	publicKey, err := base64.StdEncoding.DecodeString(publicKeyStr)
	if err != nil {
		return nil, fmt.Errorf("failed to decode public key: %w", err)
	}

	return &KeyPair{
		Private: privateKey,
		Public:  publicKey,
	}, nil
}

// Delete removes an SSH key pair for a specific host from OpenBAO
func (s *OpenBAOKeyStorage) Delete(host string) error {
	if host == "" {
		return fmt.Errorf("host cannot be empty")
	}

	// Parse the base path to get mount and path
	mount, path := s.parsePath(host)

	ctx := context.Background()
	if err := s.client.DeleteSecretV2(ctx, mount, path); err != nil {
		return fmt.Errorf("failed to delete SSH key from OpenBAO: %w", err)
	}

	return nil
}

// Exists checks if an SSH key pair exists for a specific host in OpenBAO
func (s *OpenBAOKeyStorage) Exists(host string) (bool, error) {
	if host == "" {
		return false, fmt.Errorf("host cannot be empty")
	}

	// Try to load the key - if it succeeds, it exists
	_, err := s.Load(host)
	if err != nil {
		// Check if the error indicates the secret doesn't exist
		if strings.Contains(err.Error(), "secret not found") ||
		   strings.Contains(err.Error(), "404") {
			return false, nil
		}
		// Other errors are real errors
		return false, fmt.Errorf("failed to check if SSH key exists: %w", err)
	}

	return true, nil
}

// GetStoragePath returns the expected storage path for a given host
// This is documented for Phase 2 implementation
func (s *OpenBAOKeyStorage) GetStoragePath(host string) string {
	return fmt.Sprintf("%s/%s", s.basePath, host)
}

// parsePath splits the storage path into mount and path components
// For example: "foundry-core/ssh-keys/example.com" -> ("foundry-core", "ssh-keys/example.com")
func (s *OpenBAOKeyStorage) parsePath(host string) (mount, path string) {
	// The basePath is "foundry-core/ssh-keys"
	// We need to split it into mount (foundry-core) and path (ssh-keys/<hostname>)
	parts := strings.SplitN(s.basePath, "/", 2)
	mount = parts[0]

	if len(parts) > 1 {
		path = fmt.Sprintf("%s/%s", parts[1], host)
	} else {
		path = host
	}

	return mount, path
}
