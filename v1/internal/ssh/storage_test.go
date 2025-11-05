package ssh

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewOpenBAOKeyStorage(t *testing.T) {
	t.Run("valid parameters", func(t *testing.T) {
		storage, err := NewOpenBAOKeyStorage("http://localhost:8200", "test-token")
		require.NoError(t, err)
		assert.NotNil(t, storage)
		assert.NotNil(t, storage.client)
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
	t.Run("successful store", func(t *testing.T) {
		// Create a mock OpenBAO server
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			assert.Equal(t, "POST", r.Method)
			assert.Equal(t, "/v1/foundry-core/data/ssh-keys/example.com", r.URL.Path)
			assert.Equal(t, "test-token", r.Header.Get("X-Vault-Token"))

			// Verify the request body
			var reqBody map[string]interface{}
			err := json.NewDecoder(r.Body).Decode(&reqBody)
			require.NoError(t, err)

			data, ok := reqBody["data"].(map[string]interface{})
			require.True(t, ok)
			assert.Contains(t, data, "private_key")
			assert.Contains(t, data, "public_key")

			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(map[string]interface{}{})
		}))
		defer server.Close()

		storage, err := NewOpenBAOKeyStorage(server.URL, "test-token")
		require.NoError(t, err)

		kp, err := GenerateKeyPair()
		require.NoError(t, err)

		err = storage.Store("example.com", kp)
		require.NoError(t, err)
	})

	t.Run("empty host", func(t *testing.T) {
		storage, err := NewOpenBAOKeyStorage("http://localhost:8200", "test-token")
		require.NoError(t, err)

		kp, err := GenerateKeyPair()
		require.NoError(t, err)

		err = storage.Store("", kp)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "host cannot be empty")
	})

	t.Run("nil key pair", func(t *testing.T) {
		storage, err := NewOpenBAOKeyStorage("http://localhost:8200", "test-token")
		require.NoError(t, err)

		err = storage.Store("example.com", nil)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "key pair cannot be nil")
	})

	t.Run("empty private key", func(t *testing.T) {
		storage, err := NewOpenBAOKeyStorage("http://localhost:8200", "test-token")
		require.NoError(t, err)

		kp := &KeyPair{
			Private: []byte{},
			Public:  []byte("test"),
		}

		err = storage.Store("example.com", kp)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "private key cannot be empty")
	})

	t.Run("empty public key", func(t *testing.T) {
		storage, err := NewOpenBAOKeyStorage("http://localhost:8200", "test-token")
		require.NoError(t, err)

		kp := &KeyPair{
			Private: []byte("test"),
			Public:  []byte{},
		}

		err = storage.Store("example.com", kp)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "public key cannot be empty")
	})
}

func TestOpenBAOKeyStorage_Load(t *testing.T) {
	t.Run("successful load", func(t *testing.T) {
		// Generate a test key pair
		testKP, err := GenerateKeyPair()
		require.NoError(t, err)

		// Encode keys as base64
		privateKeyB64 := base64.StdEncoding.EncodeToString(testKP.Private)
		publicKeyB64 := base64.StdEncoding.EncodeToString(testKP.Public)

		// Create a mock OpenBAO server
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			assert.Equal(t, "GET", r.Method)
			assert.Equal(t, "/v1/foundry-core/data/ssh-keys/example.com", r.URL.Path)
			assert.Equal(t, "test-token", r.Header.Get("X-Vault-Token"))

			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(map[string]interface{}{
				"data": map[string]interface{}{
					"data": map[string]interface{}{
						"private_key": privateKeyB64,
						"public_key":  publicKeyB64,
					},
					"metadata": map[string]interface{}{
						"version": 1,
					},
				},
			})
		}))
		defer server.Close()

		storage, err := NewOpenBAOKeyStorage(server.URL, "test-token")
		require.NoError(t, err)

		kp, err := storage.Load("example.com")
		require.NoError(t, err)
		assert.NotNil(t, kp)
		assert.Equal(t, testKP.Private, kp.Private)
		assert.Equal(t, testKP.Public, kp.Public)
	})

	t.Run("empty host", func(t *testing.T) {
		storage, err := NewOpenBAOKeyStorage("http://localhost:8200", "test-token")
		require.NoError(t, err)

		_, err = storage.Load("")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "host cannot be empty")
	})

	t.Run("secret not found", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusNotFound)
			json.NewEncoder(w).Encode(map[string]interface{}{
				"errors": []string{"secret not found"},
			})
		}))
		defer server.Close()

		storage, err := NewOpenBAOKeyStorage(server.URL, "test-token")
		require.NoError(t, err)

		_, err = storage.Load("nonexistent.com")
		require.Error(t, err)
	})

	t.Run("missing private key", func(t *testing.T) {
		publicKeyB64 := base64.StdEncoding.EncodeToString([]byte("test-public-key"))

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(map[string]interface{}{
				"data": map[string]interface{}{
					"data": map[string]interface{}{
						"public_key": publicKeyB64,
					},
				},
			})
		}))
		defer server.Close()

		storage, err := NewOpenBAOKeyStorage(server.URL, "test-token")
		require.NoError(t, err)

		_, err = storage.Load("example.com")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "private_key not found")
	})

	t.Run("missing public key", func(t *testing.T) {
		privateKeyB64 := base64.StdEncoding.EncodeToString([]byte("test-private-key"))

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(map[string]interface{}{
				"data": map[string]interface{}{
					"data": map[string]interface{}{
						"private_key": privateKeyB64,
					},
				},
			})
		}))
		defer server.Close()

		storage, err := NewOpenBAOKeyStorage(server.URL, "test-token")
		require.NoError(t, err)

		_, err = storage.Load("example.com")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "public_key not found")
	})
}

func TestOpenBAOKeyStorage_Delete(t *testing.T) {
	t.Run("successful delete", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			assert.Equal(t, "DELETE", r.Method)
			assert.Equal(t, "/v1/foundry-core/data/ssh-keys/example.com", r.URL.Path)
			assert.Equal(t, "test-token", r.Header.Get("X-Vault-Token"))

			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(map[string]interface{}{})
		}))
		defer server.Close()

		storage, err := NewOpenBAOKeyStorage(server.URL, "test-token")
		require.NoError(t, err)

		err = storage.Delete("example.com")
		require.NoError(t, err)
	})

	t.Run("empty host", func(t *testing.T) {
		storage, err := NewOpenBAOKeyStorage("http://localhost:8200", "test-token")
		require.NoError(t, err)

		err = storage.Delete("")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "host cannot be empty")
	})
}

func TestOpenBAOKeyStorage_Exists(t *testing.T) {
	t.Run("exists - key found", func(t *testing.T) {
		// Generate a test key pair
		testKP, err := GenerateKeyPair()
		require.NoError(t, err)

		// Encode keys as base64
		privateKeyB64 := base64.StdEncoding.EncodeToString(testKP.Private)
		publicKeyB64 := base64.StdEncoding.EncodeToString(testKP.Public)

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(map[string]interface{}{
				"data": map[string]interface{}{
					"data": map[string]interface{}{
						"private_key": privateKeyB64,
						"public_key":  publicKeyB64,
					},
				},
			})
		}))
		defer server.Close()

		storage, err := NewOpenBAOKeyStorage(server.URL, "test-token")
		require.NoError(t, err)

		exists, err := storage.Exists("example.com")
		require.NoError(t, err)
		assert.True(t, exists)
	})

	t.Run("does not exist - 404", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusNotFound)
			json.NewEncoder(w).Encode(map[string]interface{}{
				"errors": []string{"secret not found"},
			})
		}))
		defer server.Close()

		storage, err := NewOpenBAOKeyStorage(server.URL, "test-token")
		require.NoError(t, err)

		exists, err := storage.Exists("nonexistent.com")
		require.NoError(t, err)
		assert.False(t, exists)
	})

	t.Run("empty host", func(t *testing.T) {
		storage, err := NewOpenBAOKeyStorage("http://localhost:8200", "test-token")
		require.NoError(t, err)

		exists, err := storage.Exists("")
		require.Error(t, err)
		assert.False(t, exists)
		assert.Contains(t, err.Error(), "host cannot be empty")
	})
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

func TestOpenBAOKeyStorage_parsePath(t *testing.T) {
	storage, err := NewOpenBAOKeyStorage("http://localhost:8200", "test-token")
	require.NoError(t, err)

	tests := []struct {
		name         string
		host         string
		expectedMount string
		expectedPath string
	}{
		{
			name:         "simple hostname",
			host:         "example.com",
			expectedMount: "foundry-core",
			expectedPath: "ssh-keys/example.com",
		},
		{
			name:         "IP address",
			host:         "192.168.1.100",
			expectedMount: "foundry-core",
			expectedPath: "ssh-keys/192.168.1.100",
		},
		{
			name:         "hostname with subdomain",
			host:         "server1.internal.example.com",
			expectedMount: "foundry-core",
			expectedPath: "ssh-keys/server1.internal.example.com",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mount, path := storage.parsePath(tt.host)
			assert.Equal(t, tt.expectedMount, mount)
			assert.Equal(t, tt.expectedPath, path)
		})
	}
}

func TestKeyStorage_Interface(t *testing.T) {
	// Verify that OpenBAOKeyStorage implements KeyStorage interface
	var _ KeyStorage = (*OpenBAOKeyStorage)(nil)
}
