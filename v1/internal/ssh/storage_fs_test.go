package ssh

import (
	"os"
	"path/filepath"
	"testing"
)

func TestFilesystemKeyStorage_NewFilesystemKeyStorage(t *testing.T) {
	tests := []struct {
		name      string
		basePath  string
		wantErr   bool
		errString string
	}{
		{
			name:     "valid path",
			basePath: t.TempDir(),
			wantErr:  false,
		},
		{
			name:      "empty path",
			basePath:  "",
			wantErr:   true,
			errString: "basePath cannot be empty",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			storage, err := NewFilesystemKeyStorage(tt.basePath)

			if tt.wantErr {
				if err == nil {
					t.Error("expected error, got nil")
				} else if tt.errString != "" && err.Error() != tt.errString {
					t.Errorf("expected error %q, got %q", tt.errString, err.Error())
				}
				return
			}

			if err != nil {
				t.Errorf("unexpected error: %v", err)
			}
			if storage == nil {
				t.Error("expected storage instance, got nil")
			}

			// Verify directory was created
			if _, err := os.Stat(tt.basePath); os.IsNotExist(err) {
				t.Error("base path directory was not created")
			}
		})
	}
}

func TestFilesystemKeyStorage_Store(t *testing.T) {
	basePath := t.TempDir()
	storage, err := NewFilesystemKeyStorage(basePath)
	if err != nil {
		t.Fatalf("failed to create storage: %v", err)
	}

	// Generate a test key pair
	keyPair, err := GenerateKeyPair()
	if err != nil {
		t.Fatalf("failed to generate key pair: %v", err)
	}

	tests := []struct {
		name      string
		host      string
		key       *KeyPair
		wantErr   bool
		errString string
	}{
		{
			name:    "valid key pair",
			host:    "example.com",
			key:     keyPair,
			wantErr: false,
		},
		{
			name:      "empty host",
			host:      "",
			key:       keyPair,
			wantErr:   true,
			errString: "host cannot be empty",
		},
		{
			name:      "nil key pair",
			host:      "example.com",
			key:       nil,
			wantErr:   true,
			errString: "key pair cannot be nil",
		},
		{
			name: "empty private key",
			host: "example.com",
			key: &KeyPair{
				Private: []byte{},
				Public:  []byte("public-key"),
			},
			wantErr:   true,
			errString: "private key cannot be empty",
		},
		{
			name: "empty public key",
			host: "example.com",
			key: &KeyPair{
				Private: []byte("private-key"),
				Public:  []byte{},
			},
			wantErr:   true,
			errString: "public key cannot be empty",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := storage.Store(tt.host, tt.key)

			if tt.wantErr {
				if err == nil {
					t.Error("expected error, got nil")
				} else if tt.errString != "" && err.Error() != tt.errString {
					t.Errorf("expected error %q, got %q", tt.errString, err.Error())
				}
				return
			}

			if err != nil {
				t.Errorf("unexpected error: %v", err)
			}

			// Verify files were created with correct permissions
			hostDir := filepath.Join(basePath, tt.host)
			privateKeyPath := filepath.Join(hostDir, "id_ed25519")
			publicKeyPath := filepath.Join(hostDir, "id_ed25519.pub")

			// Check private key
			privateInfo, err := os.Stat(privateKeyPath)
			if err != nil {
				t.Errorf("private key file not created: %v", err)
			} else if privateInfo.Mode().Perm() != 0600 {
				t.Errorf("private key permissions = %o, want 0600", privateInfo.Mode().Perm())
			}

			// Check public key
			_, err = os.Stat(publicKeyPath)
			if err != nil {
				t.Errorf("public key file not created: %v", err)
			}
		})
	}
}

func TestFilesystemKeyStorage_Load(t *testing.T) {
	basePath := t.TempDir()
	storage, err := NewFilesystemKeyStorage(basePath)
	if err != nil {
		t.Fatalf("failed to create storage: %v", err)
	}

	// Generate and store a test key pair
	keyPair, err := GenerateKeyPair()
	if err != nil {
		t.Fatalf("failed to generate key pair: %v", err)
	}

	host := "example.com"
	if err := storage.Store(host, keyPair); err != nil {
		t.Fatalf("failed to store key pair: %v", err)
	}

	tests := []struct {
		name      string
		host      string
		wantErr   bool
		errString string
	}{
		{
			name:    "existing key",
			host:    host,
			wantErr: false,
		},
		{
			name:      "non-existent key",
			host:      "nonexistent.com",
			wantErr:   true,
			errString: "SSH key for host nonexistent.com not found",
		},
		{
			name:      "empty host",
			host:      "",
			wantErr:   true,
			errString: "host cannot be empty",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			loaded, err := storage.Load(tt.host)

			if tt.wantErr {
				if err == nil {
					t.Error("expected error, got nil")
				} else if tt.errString != "" && err.Error() != tt.errString {
					t.Errorf("expected error %q, got %q", tt.errString, err.Error())
				}
				return
			}

			if err != nil {
				t.Errorf("unexpected error: %v", err)
			}

			if loaded == nil {
				t.Error("expected key pair, got nil")
				return
			}

			// Verify loaded key matches stored key
			if string(loaded.Private) != string(keyPair.Private) {
				t.Error("loaded private key does not match stored key")
			}
			if string(loaded.Public) != string(keyPair.Public) {
				t.Error("loaded public key does not match stored key")
			}
		})
	}
}

func TestFilesystemKeyStorage_Delete(t *testing.T) {
	basePath := t.TempDir()
	storage, err := NewFilesystemKeyStorage(basePath)
	if err != nil {
		t.Fatalf("failed to create storage: %v", err)
	}

	// Generate and store a test key pair
	keyPair, err := GenerateKeyPair()
	if err != nil {
		t.Fatalf("failed to generate key pair: %v", err)
	}

	host := "example.com"
	if err := storage.Store(host, keyPair); err != nil {
		t.Fatalf("failed to store key pair: %v", err)
	}

	tests := []struct {
		name      string
		host      string
		wantErr   bool
		errString string
	}{
		{
			name:    "existing key",
			host:    host,
			wantErr: false,
		},
		{
			name:      "non-existent key",
			host:      "nonexistent.com",
			wantErr:   true,
			errString: "SSH key for host nonexistent.com not found",
		},
		{
			name:      "empty host",
			host:      "",
			wantErr:   true,
			errString: "host cannot be empty",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := storage.Delete(tt.host)

			if tt.wantErr {
				if err == nil {
					t.Error("expected error, got nil")
				} else if tt.errString != "" && err.Error() != tt.errString {
					t.Errorf("expected error %q, got %q", tt.errString, err.Error())
				}
				return
			}

			if err != nil {
				t.Errorf("unexpected error: %v", err)
			}

			// Verify directory was deleted
			hostDir := filepath.Join(basePath, tt.host)
			if _, err := os.Stat(hostDir); !os.IsNotExist(err) {
				t.Error("host directory still exists after delete")
			}
		})
	}
}

func TestFilesystemKeyStorage_Exists(t *testing.T) {
	basePath := t.TempDir()
	storage, err := NewFilesystemKeyStorage(basePath)
	if err != nil {
		t.Fatalf("failed to create storage: %v", err)
	}

	// Generate and store a test key pair
	keyPair, err := GenerateKeyPair()
	if err != nil {
		t.Fatalf("failed to generate key pair: %v", err)
	}

	host := "example.com"
	if err := storage.Store(host, keyPair); err != nil {
		t.Fatalf("failed to store key pair: %v", err)
	}

	tests := []struct {
		name       string
		host       string
		wantExists bool
		wantErr    bool
		errString  string
	}{
		{
			name:       "existing key",
			host:       host,
			wantExists: true,
			wantErr:    false,
		},
		{
			name:       "non-existent key",
			host:       "nonexistent.com",
			wantExists: false,
			wantErr:    false,
		},
		{
			name:      "empty host",
			host:      "",
			wantErr:   true,
			errString: "host cannot be empty",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			exists, err := storage.Exists(tt.host)

			if tt.wantErr {
				if err == nil {
					t.Error("expected error, got nil")
				} else if tt.errString != "" && err.Error() != tt.errString {
					t.Errorf("expected error %q, got %q", tt.errString, err.Error())
				}
				return
			}

			if err != nil {
				t.Errorf("unexpected error: %v", err)
			}

			if exists != tt.wantExists {
				t.Errorf("exists = %v, want %v", exists, tt.wantExists)
			}
		})
	}
}

func TestFilesystemKeyStorage_GetStoragePath(t *testing.T) {
	basePath := t.TempDir()
	storage, err := NewFilesystemKeyStorage(basePath)
	if err != nil {
		t.Fatalf("failed to create storage: %v", err)
	}

	host := "example.com"
	expectedPath := filepath.Join(basePath, host)

	path := storage.GetStoragePath(host)
	if path != expectedPath {
		t.Errorf("GetStoragePath() = %q, want %q", path, expectedPath)
	}
}

func TestFilesystemKeyStorage_RoundTrip(t *testing.T) {
	basePath := t.TempDir()
	storage, err := NewFilesystemKeyStorage(basePath)
	if err != nil {
		t.Fatalf("failed to create storage: %v", err)
	}

	// Generate a key pair
	keyPair, err := GenerateKeyPair()
	if err != nil {
		t.Fatalf("failed to generate key pair: %v", err)
	}

	host := "roundtrip.test"

	// Store
	if err := storage.Store(host, keyPair); err != nil {
		t.Fatalf("failed to store key pair: %v", err)
	}

	// Verify exists
	exists, err := storage.Exists(host)
	if err != nil {
		t.Fatalf("failed to check if key exists: %v", err)
	}
	if !exists {
		t.Fatal("key should exist after storing")
	}

	// Load
	loaded, err := storage.Load(host)
	if err != nil {
		t.Fatalf("failed to load key pair: %v", err)
	}

	// Verify content matches
	if string(loaded.Private) != string(keyPair.Private) {
		t.Error("loaded private key does not match original")
	}
	if string(loaded.Public) != string(keyPair.Public) {
		t.Error("loaded public key does not match original")
	}

	// Delete
	if err := storage.Delete(host); err != nil {
		t.Fatalf("failed to delete key pair: %v", err)
	}

	// Verify no longer exists
	exists, err = storage.Exists(host)
	if err != nil {
		t.Fatalf("failed to check if key exists: %v", err)
	}
	if exists {
		t.Fatal("key should not exist after deletion")
	}
}
