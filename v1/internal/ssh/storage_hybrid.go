package ssh

import (
	"fmt"
	"path/filepath"
)

// HybridKeyStorage provides automatic migration from filesystem to OpenBAO
// It tries OpenBAO first, falls back to filesystem, and migrates keys on load
type HybridKeyStorage struct {
	openbao    *OpenBAOKeyStorage
	filesystem *FilesystemKeyStorage
}

// NewHybridKeyStorage creates a hybrid storage that handles migration automatically
func NewHybridKeyStorage(openbao *OpenBAOKeyStorage, configDir string) (*HybridKeyStorage, error) {
	keysDir := filepath.Join(configDir, "keys")
	filesystem, err := NewFilesystemKeyStorage(keysDir)
	if err != nil {
		return nil, fmt.Errorf("failed to create filesystem storage: %w", err)
	}

	return &HybridKeyStorage{
		openbao:    openbao,
		filesystem: filesystem,
	}, nil
}

// Store saves to both OpenBAO and filesystem for redundancy
func (h *HybridKeyStorage) Store(host string, key *KeyPair) error {
	// Store in OpenBAO first (primary)
	if err := h.openbao.Store(host, key); err != nil {
		return fmt.Errorf("failed to store in OpenBAO: %w", err)
	}

	// Also store in filesystem as backup
	if err := h.filesystem.Store(host, key); err != nil {
		// Non-fatal - log but continue
		// In production, this would use a logger
		_ = err
	}

	return nil
}

// Load attempts to load from OpenBAO, falls back to filesystem with auto-migration
func (h *HybridKeyStorage) Load(host string) (*KeyPair, error) {
	// Try OpenBAO first
	key, err := h.openbao.Load(host)
	if err == nil {
		return key, nil
	}

	// OpenBAO failed, try filesystem
	key, fsErr := h.filesystem.Load(host)
	if fsErr != nil {
		// Both failed - return original OpenBAO error
		return nil, err
	}

	// Found in filesystem! Migrate to OpenBAO automatically
	if migrateErr := h.openbao.Store(host, key); migrateErr != nil {
		// Migration failed, but we have the key from filesystem
		// Log the error but return the key
		// In production, this would use a logger
		_ = migrateErr
	}

	return key, nil
}

// Delete removes from both storages
func (h *HybridKeyStorage) Delete(host string) error {
	// Delete from both, collecting errors
	var err1, err2 error

	err1 = h.openbao.Delete(host)
	err2 = h.filesystem.Delete(host)

	if err1 != nil && err2 != nil {
		return fmt.Errorf("failed to delete from both storages: openbao: %v, filesystem: %v", err1, err2)
	}
	if err1 != nil {
		return fmt.Errorf("failed to delete from OpenBAO: %w", err1)
	}
	if err2 != nil {
		return fmt.Errorf("failed to delete from filesystem: %w", err2)
	}

	return nil
}

// Exists checks if key exists in either storage
func (h *HybridKeyStorage) Exists(host string) (bool, error) {
	// Check OpenBAO first
	exists, err := h.openbao.Exists(host)
	if err == nil && exists {
		return true, nil
	}

	// Check filesystem
	return h.filesystem.Exists(host)
}
