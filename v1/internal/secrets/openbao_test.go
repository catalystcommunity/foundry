package secrets

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/catalystcommunity/foundry/v1/internal/component/openbao"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewOpenBAOResolver(t *testing.T) {
	t.Run("valid parameters", func(t *testing.T) {
		resolver, err := NewOpenBAOResolver("https://openbao.example.com", "test-token")
		require.NoError(t, err)
		require.NotNil(t, resolver)
		assert.NotNil(t, resolver.client)
		assert.Equal(t, "secret", resolver.mount)
	})

	t.Run("empty address", func(t *testing.T) {
		resolver, err := NewOpenBAOResolver("", "test-token")
		require.Error(t, err)
		assert.Nil(t, resolver)
		assert.Contains(t, err.Error(), "address is required")
	})

	t.Run("empty token", func(t *testing.T) {
		resolver, err := NewOpenBAOResolver("https://openbao.example.com", "")
		require.Error(t, err)
		assert.Nil(t, resolver)
		assert.Contains(t, err.Error(), "token is required")
	})
}

func TestNewOpenBAOResolverWithMount(t *testing.T) {
	t.Run("valid parameters", func(t *testing.T) {
		resolver, err := NewOpenBAOResolverWithMount("https://openbao.example.com", "test-token", "custom-mount")
		require.NoError(t, err)
		require.NotNil(t, resolver)
		assert.Equal(t, "custom-mount", resolver.mount)
	})

	t.Run("empty mount", func(t *testing.T) {
		resolver, err := NewOpenBAOResolverWithMount("https://openbao.example.com", "test-token", "")
		require.Error(t, err)
		assert.Nil(t, resolver)
		assert.Contains(t, err.Error(), "mount point is required")
	})
}

func TestOpenBAOResolver_Resolve(t *testing.T) {
	t.Run("successful resolution", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			assert.Equal(t, "/v1/secret/data/myapp-prod/database/main", r.URL.Path)
			assert.Equal(t, "GET", r.Method)
			assert.Equal(t, "test-token", r.Header.Get("X-Vault-Token"))

			response := openbao.KVv2Response{}
			response.Data.Data = map[string]interface{}{
				"password": "secret123",
				"username": "admin",
			}
			response.Data.Metadata.Version = 1

			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(response)
		}))
		defer server.Close()

		resolver, err := NewOpenBAOResolver(server.URL, "test-token")
		require.NoError(t, err)

		ctx := NewResolutionContext("myapp-prod")
		ref := SecretRef{Path: "database/main", Key: "password"}

		value, err := resolver.Resolve(ctx, ref)
		require.NoError(t, err)
		assert.Equal(t, "secret123", value)
	})

	t.Run("nil context", func(t *testing.T) {
		resolver, err := NewOpenBAOResolver("https://openbao.example.com", "test-token")
		require.NoError(t, err)

		ref := SecretRef{Path: "database/main", Key: "password"}

		_, err = resolver.Resolve(nil, ref)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "context is required")
	})

	t.Run("secret not found", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusNotFound)
			w.Write([]byte(`{"errors":["secret not found"]}`))
		}))
		defer server.Close()

		resolver, err := NewOpenBAOResolver(server.URL, "test-token")
		require.NoError(t, err)

		ctx := NewResolutionContext("myapp-prod")
		ref := SecretRef{Path: "database/main", Key: "password"}

		_, err = resolver.Resolve(ctx, ref)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to read secret from OpenBAO")
	})

	t.Run("key not found in secret", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			response := openbao.KVv2Response{}
			response.Data.Data = map[string]interface{}{
				"username": "admin",
				// password key missing
			}
			response.Data.Metadata.Version = 1

			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(response)
		}))
		defer server.Close()

		resolver, err := NewOpenBAOResolver(server.URL, "test-token")
		require.NoError(t, err)

		ctx := NewResolutionContext("myapp-prod")
		ref := SecretRef{Path: "database/main", Key: "password"}

		_, err = resolver.Resolve(ctx, ref)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "key password not found in secret")
	})

	t.Run("value is not a string", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			response := openbao.KVv2Response{}
			response.Data.Data = map[string]interface{}{
				"password": 12345, // Not a string
			}
			response.Data.Metadata.Version = 1

			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(response)
		}))
		defer server.Close()

		resolver, err := NewOpenBAOResolver(server.URL, "test-token")
		require.NoError(t, err)

		ctx := NewResolutionContext("myapp-prod")
		ref := SecretRef{Path: "database/main", Key: "password"}

		_, err = resolver.Resolve(ctx, ref)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "is not a string")
	})

	t.Run("instance scoping works", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Verify the instance prefix is in the path
			assert.Equal(t, "/v1/secret/data/foundry-core/grafana/admin", r.URL.Path)

			response := openbao.KVv2Response{}
			response.Data.Data = map[string]interface{}{
				"password": "grafana123",
			}
			response.Data.Metadata.Version = 1

			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(response)
		}))
		defer server.Close()

		resolver, err := NewOpenBAOResolver(server.URL, "test-token")
		require.NoError(t, err)

		ctx := NewResolutionContext("foundry-core")
		ref := SecretRef{Path: "grafana/admin", Key: "password"}

		value, err := resolver.Resolve(ctx, ref)
		require.NoError(t, err)
		assert.Equal(t, "grafana123", value)
	})

	t.Run("custom mount point", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Verify the custom mount is used
			assert.Equal(t, "/v1/custom-kv/data/myapp/database", r.URL.Path)

			response := openbao.KVv2Response{}
			response.Data.Data = map[string]interface{}{
				"password": "secret123",
			}
			response.Data.Metadata.Version = 1

			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(response)
		}))
		defer server.Close()

		resolver, err := NewOpenBAOResolverWithMount(server.URL, "test-token", "custom-kv")
		require.NoError(t, err)

		ctx := NewResolutionContext("myapp")
		ref := SecretRef{Path: "database", Key: "password"}

		value, err := resolver.Resolve(ctx, ref)
		require.NoError(t, err)
		assert.Equal(t, "secret123", value)
	})
}

func TestOpenBAOResolver_Integration(t *testing.T) {
	// This test uses testcontainers to spin up a real OpenBAO instance
	// Skipped by default, run with -tags=integration
	t.Skip("Integration test - requires testcontainers")

	// TODO: Add testcontainers-based integration test in a separate file
	// This would:
	// 1. Start OpenBAO container
	// 2. Initialize and unseal it
	// 3. Enable KV v2 secrets engine
	// 4. Write test secrets
	// 5. Test resolution with real OpenBAO
}

func TestResolveSecret(t *testing.T) {
	t.Run("successful resolution", func(t *testing.T) {
		// Create mock server
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			assert.Equal(t, "/v1/secret/data/foundry-core/k3s", r.URL.Path)
			assert.Equal(t, "test-token", r.Header.Get("X-Vault-Token"))

			response := openbao.KVv2Response{
				Data: struct {
					Data     map[string]interface{} `json:"data"`
					Metadata struct {
						Version int `json:"version"`
					} `json:"metadata"`
				}{
					Data: map[string]interface{}{
						"kubeconfig": "test-kubeconfig-content",
					},
					Metadata: struct {
						Version int `json:"version"`
					}{
						Version: 1,
					},
				},
			}
			json.NewEncoder(w).Encode(response)
		}))
		defer server.Close()

		resolver, err := NewOpenBAOResolver(server.URL, "test-token")
		require.NoError(t, err)

		ctx := context.Background()
		value, err := resolver.ResolveSecret(ctx, "foundry-core/k3s", "kubeconfig")
		require.NoError(t, err)
		assert.Equal(t, "test-kubeconfig-content", value)
	})

	t.Run("key not found", func(t *testing.T) {
		// Create mock server
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			response := openbao.KVv2Response{
				Data: struct {
					Data     map[string]interface{} `json:"data"`
					Metadata struct {
						Version int `json:"version"`
					} `json:"metadata"`
				}{
					Data: map[string]interface{}{
						"other-key": "value",
					},
					Metadata: struct {
						Version int `json:"version"`
					}{
						Version: 1,
					},
				},
			}
			json.NewEncoder(w).Encode(response)
		}))
		defer server.Close()

		resolver, err := NewOpenBAOResolver(server.URL, "test-token")
		require.NoError(t, err)

		ctx := context.Background()
		value, err := resolver.ResolveSecret(ctx, "path", "missing-key")
		require.Error(t, err)
		assert.Empty(t, value)
		assert.Contains(t, err.Error(), "key missing-key not found")
	})

	t.Run("non-string value", func(t *testing.T) {
		// Create mock server
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			response := openbao.KVv2Response{
				Data: struct {
					Data     map[string]interface{} `json:"data"`
					Metadata struct {
						Version int `json:"version"`
					} `json:"metadata"`
				}{
					Data: map[string]interface{}{
						"number": 123,
					},
					Metadata: struct {
						Version int `json:"version"`
					}{
						Version: 1,
					},
				},
			}
			json.NewEncoder(w).Encode(response)
		}))
		defer server.Close()

		resolver, err := NewOpenBAOResolver(server.URL, "test-token")
		require.NoError(t, err)

		ctx := context.Background()
		value, err := resolver.ResolveSecret(ctx, "path", "number")
		require.Error(t, err)
		assert.Empty(t, value)
		assert.Contains(t, err.Error(), "not a string")
	})

	t.Run("OpenBAO API error", func(t *testing.T) {
		// Create mock server that returns an error
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusNotFound)
			json.NewEncoder(w).Encode(map[string]interface{}{
				"errors": []string{"secret not found"},
			})
		}))
		defer server.Close()

		resolver, err := NewOpenBAOResolver(server.URL, "test-token")
		require.NoError(t, err)

		ctx := context.Background()
		value, err := resolver.ResolveSecret(ctx, "nonexistent/path", "key")
		require.Error(t, err)
		assert.Empty(t, value)
		assert.Contains(t, err.Error(), "failed to read secret from OpenBAO")
	})
}
