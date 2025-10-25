package ssh

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewOpenBAOKeyStorage(t *testing.T) {
	t.Run("valid parameters", func(t *testing.T) {
		storage, err := NewOpenBAOKeyStorage("http://localhost:8200", "test-token")
		require.NoError(t, err)
		assert.NotNil(t, storage)
		assert.Equal(t, "http://localhost:8200", storage.addr)
		assert.Equal(t, "test-token", storage.token)
		assert.Equal(t, "foundry-core/ssh-keys", storage.basePath)
	})

	t.Run("empty address", func(t *testing.T) {
		_, err := NewOpenBAOKeyStorage("", "test-token")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "OpenBAO address cannot be empty")
	})

	t.Run("empty token", func(t *testing.T) {
		_, err := NewOpenBAOKeyStorage("http://localhost:8200", "")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "OpenBAO token cannot be empty")
	})
}

func TestOpenBAOKeyStorage_Store(t *testing.T) {
	storage, err := NewOpenBAOKeyStorage("http://localhost:8200", "test-token")
	require.NoError(t, err)

	kp, err := GenerateKeyPair()
	require.NoError(t, err)

	err = storage.Store("example.com", kp)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "OpenBAO integration not yet implemented")
}

func TestOpenBAOKeyStorage_Load(t *testing.T) {
	storage, err := NewOpenBAOKeyStorage("http://localhost:8200", "test-token")
	require.NoError(t, err)

	_, err = storage.Load("example.com")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "OpenBAO integration not yet implemented")
}

func TestOpenBAOKeyStorage_Delete(t *testing.T) {
	storage, err := NewOpenBAOKeyStorage("http://localhost:8200", "test-token")
	require.NoError(t, err)

	err = storage.Delete("example.com")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "OpenBAO integration not yet implemented")
}

func TestOpenBAOKeyStorage_Exists(t *testing.T) {
	storage, err := NewOpenBAOKeyStorage("http://localhost:8200", "test-token")
	require.NoError(t, err)

	exists, err := storage.Exists("example.com")
	require.Error(t, err)
	assert.False(t, exists)
	assert.Contains(t, err.Error(), "OpenBAO integration not yet implemented")
}

func TestOpenBAOKeyStorage_GetStoragePath(t *testing.T) {
	storage, err := NewOpenBAOKeyStorage("http://localhost:8200", "test-token")
	require.NoError(t, err)

	tests := []struct {
		name string
		host string
		want string
	}{
		{
			name: "simple hostname",
			host: "example.com",
			want: "foundry-core/ssh-keys/example.com",
		},
		{
			name: "IP address",
			host: "192.168.1.100",
			want: "foundry-core/ssh-keys/192.168.1.100",
		},
		{
			name: "hostname with subdomain",
			host: "server1.internal.example.com",
			want: "foundry-core/ssh-keys/server1.internal.example.com",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := storage.GetStoragePath(tt.host)
			assert.Equal(t, tt.want, path)
		})
	}
}

func TestKeyStorage_Interface(t *testing.T) {
	// Verify that OpenBAOKeyStorage implements KeyStorage interface
	var _ KeyStorage = (*OpenBAOKeyStorage)(nil)
}
