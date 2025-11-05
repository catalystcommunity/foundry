package openbao

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewClient(t *testing.T) {
	client := NewClient("http://localhost:8200", "test-token")
	assert.NotNil(t, client)
	assert.Equal(t, "http://localhost:8200", client.baseURL)
	assert.Equal(t, "test-token", client.token)
	assert.NotNil(t, client.httpClient)
}

func TestClient_Health(t *testing.T) {
	tests := []struct {
		name           string
		statusCode     int
		responseBody   HealthResponse
		expectedHealth *HealthResponse
		wantErr        bool
	}{
		{
			name:       "initialized, unsealed, and active",
			statusCode: 200,
			responseBody: HealthResponse{
				Initialized: true,
				Sealed:      false,
				Standby:     false,
				Version:     "2.0.0",
				ClusterName: "test-cluster",
				ClusterID:   "test-id",
			},
			expectedHealth: &HealthResponse{
				Initialized: true,
				Sealed:      false,
				Standby:     false,
				Version:     "2.0.0",
				ClusterName: "test-cluster",
				ClusterID:   "test-id",
				StatusCode:  200,
			},
			wantErr: false,
		},
		{
			name:       "sealed",
			statusCode: 503,
			responseBody: HealthResponse{
				Initialized: true,
				Sealed:      true,
				Standby:     false,
				Version:     "2.0.0",
			},
			expectedHealth: &HealthResponse{
				Initialized: true,
				Sealed:      true,
				Standby:     false,
				Version:     "2.0.0",
				StatusCode:  503,
			},
			wantErr: false,
		},
		{
			name:       "not initialized",
			statusCode: 501,
			responseBody: HealthResponse{
				Initialized: false,
				Sealed:      true,
				Standby:     false,
			},
			expectedHealth: &HealthResponse{
				Initialized: false,
				Sealed:      true,
				Standby:     false,
				StatusCode:  501,
			},
			wantErr: false,
		},
		{
			name:       "standby",
			statusCode: 429,
			responseBody: HealthResponse{
				Initialized: true,
				Sealed:      false,
				Standby:     true,
				Version:     "2.0.0",
			},
			expectedHealth: &HealthResponse{
				Initialized: true,
				Sealed:      false,
				Standby:     true,
				Version:     "2.0.0",
				StatusCode:  429,
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				assert.Equal(t, "/v1/sys/health", r.URL.Path)
				assert.Equal(t, "GET", r.Method)

				w.WriteHeader(tt.statusCode)
				json.NewEncoder(w).Encode(tt.responseBody)
			}))
			defer server.Close()

			client := NewClient(server.URL, "")
			health, err := client.Health(context.Background())

			if tt.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.expectedHealth, health)
			}
		})
	}
}

func TestHealthResponse_IsReady(t *testing.T) {
	tests := []struct {
		name     string
		response *HealthResponse
		expected bool
	}{
		{
			name: "ready",
			response: &HealthResponse{
				Initialized: true,
				Sealed:      false,
				StatusCode:  200,
			},
			expected: true,
		},
		{
			name: "sealed",
			response: &HealthResponse{
				Initialized: true,
				Sealed:      true,
				StatusCode:  503,
			},
			expected: false,
		},
		{
			name: "not initialized",
			response: &HealthResponse{
				Initialized: false,
				Sealed:      true,
				StatusCode:  501,
			},
			expected: false,
		},
		{
			name: "standby",
			response: &HealthResponse{
				Initialized: true,
				Sealed:      false,
				StatusCode:  429,
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, tt.response.IsReady())
		})
	}
}

func TestHealthResponse_IsSealed(t *testing.T) {
	tests := []struct {
		name     string
		response *HealthResponse
		expected bool
	}{
		{
			name:     "sealed",
			response: &HealthResponse{Sealed: true},
			expected: true,
		},
		{
			name:     "unsealed",
			response: &HealthResponse{Sealed: false},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, tt.response.IsSealed())
		})
	}
}

func TestHealthResponse_IsInitialized(t *testing.T) {
	tests := []struct {
		name     string
		response *HealthResponse
		expected bool
	}{
		{
			name:     "initialized",
			response: &HealthResponse{Initialized: true},
			expected: true,
		},
		{
			name:     "not initialized",
			response: &HealthResponse{Initialized: false},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, tt.response.IsInitialized())
		})
	}
}

func TestClient_doRequest(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify token header
		assert.Equal(t, "test-token", r.Header.Get("X-Vault-Token"))

		// Verify content type for POST
		if r.Method == "POST" || r.Method == "PUT" {
			assert.Equal(t, "application/json", r.Header.Get("Content-Type"))
		}

		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status":"ok"}`))
	}))
	defer server.Close()

	client := NewClient(server.URL, "test-token")

	t.Run("GET request", func(t *testing.T) {
		resp, err := client.doRequest(context.Background(), "GET", "/v1/test", nil)
		require.NoError(t, err)
		assert.Equal(t, http.StatusOK, resp.StatusCode)
		resp.Body.Close()
	})

	t.Run("POST request with body", func(t *testing.T) {
		body := map[string]string{"key": "value"}
		resp, err := client.doRequest(context.Background(), "POST", "/v1/test", body)
		require.NoError(t, err)
		assert.Equal(t, http.StatusOK, resp.StatusCode)
		resp.Body.Close()
	})
}

func TestReadResponse(t *testing.T) {
	t.Run("successful response", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(map[string]string{"key": "value"})
		}))
		defer server.Close()

		resp, err := http.Get(server.URL)
		require.NoError(t, err)

		var result map[string]string
		err = readResponse(resp, &result)
		require.NoError(t, err)
		assert.Equal(t, "value", result["key"])
	})

	t.Run("error response", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusBadRequest)
			w.Write([]byte("bad request"))
		}))
		defer server.Close()

		resp, err := http.Get(server.URL)
		require.NoError(t, err)

		var result map[string]string
		err = readResponse(resp, &result)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "unexpected status code 400")
	})

	t.Run("empty response", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		}))
		defer server.Close()

		resp, err := http.Get(server.URL)
		require.NoError(t, err)

		var result map[string]string
		err = readResponse(resp, &result)
		require.NoError(t, err)
	})
}

func TestClient_ReadSecretV2(t *testing.T) {
	t.Run("successful read", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			assert.Equal(t, "/v1/secret/data/myapp/database", r.URL.Path)
			assert.Equal(t, "GET", r.Method)
			assert.Equal(t, "test-token", r.Header.Get("X-Vault-Token"))

			response := KVv2Response{}
			response.Data.Data = map[string]interface{}{
				"password": "secret123",
				"username": "admin",
			}
			response.Data.Metadata.Version = 1

			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(response)
		}))
		defer server.Close()

		client := NewClient(server.URL, "test-token")
		data, err := client.ReadSecretV2(context.Background(), "secret", "myapp/database")

		require.NoError(t, err)
		assert.Equal(t, "secret123", data["password"])
		assert.Equal(t, "admin", data["username"])
	})

	t.Run("secret not found", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusNotFound)
			w.Write([]byte(`{"errors":["secret not found"]}`))
		}))
		defer server.Close()

		client := NewClient(server.URL, "test-token")
		_, err := client.ReadSecretV2(context.Background(), "secret", "nonexistent")

		require.Error(t, err)
		assert.Contains(t, err.Error(), "unexpected status code 404")
	})

	t.Run("empty data", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			response := KVv2Response{}
			// response.Data.Data is nil

			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(response)
		}))
		defer server.Close()

		client := NewClient(server.URL, "test-token")
		_, err := client.ReadSecretV2(context.Background(), "secret", "empty")

		require.Error(t, err)
		assert.Contains(t, err.Error(), "secret not found at path")
	})
}

func TestClient_WriteSecretV2(t *testing.T) {
	t.Run("successful write", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			assert.Equal(t, "/v1/secret/data/myapp/database", r.URL.Path)
			assert.Equal(t, "POST", r.Method)
			assert.Equal(t, "test-token", r.Header.Get("X-Vault-Token"))
			assert.Equal(t, "application/json", r.Header.Get("Content-Type"))

			// Verify request body
			var body map[string]interface{}
			err := json.NewDecoder(r.Body).Decode(&body)
			require.NoError(t, err)

			data, ok := body["data"].(map[string]interface{})
			require.True(t, ok)
			assert.Equal(t, "secret123", data["password"])
			assert.Equal(t, "admin", data["username"])

			w.WriteHeader(http.StatusOK)
		}))
		defer server.Close()

		client := NewClient(server.URL, "test-token")
		secretData := map[string]interface{}{
			"password": "secret123",
			"username": "admin",
		}

		err := client.WriteSecretV2(context.Background(), "secret", "myapp/database", secretData)
		require.NoError(t, err)
	})

	t.Run("write error", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusForbidden)
			w.Write([]byte(`{"errors":["permission denied"]}`))
		}))
		defer server.Close()

		client := NewClient(server.URL, "test-token")
		secretData := map[string]interface{}{
			"password": "secret123",
		}

		err := client.WriteSecretV2(context.Background(), "secret", "denied", secretData)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "unexpected status code 403")
	})
}

func TestClient_DeleteSecretV2(t *testing.T) {
	t.Run("successful delete", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			assert.Equal(t, "DELETE", r.Method)
			assert.Equal(t, "/v1/secret/data/myapp/database", r.URL.Path)
			assert.Equal(t, "test-token", r.Header.Get("X-Vault-Token"))

			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(map[string]interface{}{})
		}))
		defer server.Close()

		client := NewClient(server.URL, "test-token")
		err := client.DeleteSecretV2(context.Background(), "secret", "myapp/database")
		require.NoError(t, err)
	})

	t.Run("delete non-existent secret", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusNotFound)
			json.NewEncoder(w).Encode(map[string]interface{}{
				"errors": []string{"secret not found"},
			})
		}))
		defer server.Close()

		client := NewClient(server.URL, "test-token")
		err := client.DeleteSecretV2(context.Background(), "secret", "nonexistent")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "unexpected status code 404")
	})

	t.Run("delete with permission denied", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusForbidden)
			json.NewEncoder(w).Encode(map[string]interface{}{
				"errors": []string{"permission denied"},
			})
		}))
		defer server.Close()

		client := NewClient(server.URL, "test-token")
		err := client.DeleteSecretV2(context.Background(), "secret", "denied")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "unexpected status code 403")
	})
}
