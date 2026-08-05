package ssh

import (
	"fmt"
	"path/filepath"
)

// HybridKeyStorage keeps OpenBAO as the primary store and the file system as a fallback.
type HybridKeyStorage struct {
	openbao    *OpenBAOKeyStorage
	filesystem *FilesystemKeyStorage
}

// NewHybridKeyStorage creates storage that migrates a local key when it is loaded.
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

// Store saves to OpenBAO and keeps a best-effort local fallback.
func (h *HybridKeyStorage) Store(host string, key *KeyPair) error {
	if err := h.openbao.Store(host, key); err != nil {
		return fmt.Errorf("failed to store in OpenBAO: %w", err)
	}

	_ = h.filesystem.Store(host, key)

	return nil
}

// Load reads OpenBAO first and copies a local fallback to OpenBAO when necessary.
func (h *HybridKeyStorage) Load(host string) (*KeyPair, error) {
	key, err := h.openbao.Load(host)
	if err == nil {
		return key, nil
	}

	key, fsErr := h.filesystem.Load(host)
	if fsErr != nil {
		return nil, err
	}

	_ = h.openbao.Store(host, key)

	return key, nil
}

// Delete removes a key from both stores.
func (h *HybridKeyStorage) Delete(host string) error {
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

// Exists reports whether either store contains the key.
func (h *HybridKeyStorage) Exists(host string) (bool, error) {
	exists, err := h.openbao.Exists(host)
	if err == nil && exists {
		return true, nil
	}

	return h.filesystem.Exists(host)
}
