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

func TestClient_Initialize(t *testing.T) {
	tests := []struct {
		name       string
		shares     int
		threshold  int
		wantErr    bool
		errMsg     string
		serverFunc func(w http.ResponseWriter, r *http.Request)
	}{
		{
			name:      "successful initialization",
			shares:    5,
			threshold: 3,
			wantErr:   false,
			serverFunc: func(w http.ResponseWriter, r *http.Request) {
				assert.Equal(t, "PUT", r.Method)
				assert.Equal(t, "/v1/sys/init", r.URL.Path)

				var req InitRequest
				json.NewDecoder(r.Body).Decode(&req)
				assert.Equal(t, 5, req.SecretShares)
				assert.Equal(t, 3, req.SecretThreshold)

				resp := InitResponse{
					Keys:       []string{"key1", "key2", "key3", "key4", "key5"},
					KeysBase64: []string{"a2V5MQ==", "a2V5Mg==", "a2V5Mw==", "a2V5NA==", "a2V5NQ=="},
					RootToken:  "root-token-12345",
				}
				w.WriteHeader(http.StatusOK)
				json.NewEncoder(w).Encode(resp)
			},
		},
		{
			name:      "invalid shares (zero)",
			shares:    0,
			threshold: 1,
			wantErr:   true,
			errMsg:    "secret_shares must be at least 1",
		},
		{
			name:      "invalid threshold (zero)",
			shares:    5,
			threshold: 0,
			wantErr:   true,
			errMsg:    "secret_threshold must be at least 1",
		},
		{
			name:      "threshold exceeds shares",
			shares:    3,
			threshold: 5,
			wantErr:   true,
			errMsg:    "secret_threshold (5) cannot exceed secret_shares (3)",
		},
		{
			name:      "single share",
			shares:    1,
			threshold: 1,
			wantErr:   false,
			serverFunc: func(w http.ResponseWriter, r *http.Request) {
				resp := InitResponse{
					Keys:       []string{"key1"},
					KeysBase64: []string{"a2V5MQ=="},
					RootToken:  "root-token",
				}
				w.WriteHeader(http.StatusOK)
				json.NewEncoder(w).Encode(resp)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.serverFunc != nil {
				server := httptest.NewServer(http.HandlerFunc(tt.serverFunc))
				defer server.Close()

				client := NewClient(server.URL, "")
				resp, err := client.Initialize(context.Background(), tt.shares, tt.threshold)

				if tt.wantErr {
					require.Error(t, err)
					if tt.errMsg != "" {
						assert.Contains(t, err.Error(), tt.errMsg)
					}
				} else {
					require.NoError(t, err)
					assert.NotNil(t, resp)
					assert.Len(t, resp.Keys, tt.shares)
					assert.NotEmpty(t, resp.RootToken)
				}
			} else {
				// No server needed for validation errors
				client := NewClient("http://localhost:8200", "")
				_, err := client.Initialize(context.Background(), tt.shares, tt.threshold)
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errMsg)
			}
		})
	}
}

func TestClient_Unseal(t *testing.T) {
	tests := []struct {
		name       string
		key        string
		wantErr    bool
		errMsg     string
		serverFunc func(w http.ResponseWriter, r *http.Request)
	}{
		{
			name:    "successful unseal (still sealed)",
			key:     "test-key-1",
			wantErr: false,
			serverFunc: func(w http.ResponseWriter, r *http.Request) {
				assert.Equal(t, "PUT", r.Method)
				assert.Equal(t, "/v1/sys/unseal", r.URL.Path)

				var req UnsealRequest
				json.NewDecoder(r.Body).Decode(&req)
				assert.Equal(t, "test-key-1", req.Key)

				resp := UnsealResponse{
					Sealed:   true,
					T:        3,
					N:        5,
					Progress: 1,
				}
				w.WriteHeader(http.StatusOK)
				json.NewEncoder(w).Encode(resp)
			},
		},
		{
			name:    "successful unseal (now unsealed)",
			key:     "test-key-3",
			wantErr: false,
			serverFunc: func(w http.ResponseWriter, r *http.Request) {
				resp := UnsealResponse{
					Sealed:   false,
					T:        3,
					N:        5,
					Progress: 0,
				}
				w.WriteHeader(http.StatusOK)
				json.NewEncoder(w).Encode(resp)
			},
		},
		{
			name:    "empty key",
			key:     "",
			wantErr: true,
			errMsg:  "unseal key is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.serverFunc != nil {
				server := httptest.NewServer(http.HandlerFunc(tt.serverFunc))
				defer server.Close()

				client := NewClient(server.URL, "")
				resp, err := client.Unseal(context.Background(), tt.key)

				if tt.wantErr {
					require.Error(t, err)
					if tt.errMsg != "" {
						assert.Contains(t, err.Error(), tt.errMsg)
					}
				} else {
					require.NoError(t, err)
					assert.NotNil(t, resp)
				}
			} else {
				client := NewClient("http://localhost:8200", "")
				_, err := client.Unseal(context.Background(), tt.key)
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errMsg)
			}
		})
	}
}

func TestClient_UnsealWithKeys(t *testing.T) {
	tests := []struct {
		name        string
		keys        []string
		wantErr     bool
		errMsg      string
		serverCalls int
	}{
		{
			name:        "successful unseal with 3 keys",
			keys:        []string{"key1", "key2", "key3"},
			wantErr:     false,
			serverCalls: 3,
		},
		{
			name:    "no keys provided",
			keys:    []string{},
			wantErr: true,
			errMsg:  "at least one unseal key is required",
		},
		{
			name:        "unseals after 2 keys",
			keys:        []string{"key1", "key2", "key3", "key4", "key5"},
			wantErr:     false,
			serverCalls: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if len(tt.keys) > 0 && !tt.wantErr {
				callCount := 0
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					callCount++

					var resp UnsealResponse
					if callCount >= tt.serverCalls {
						// Unsealed after enough keys
						resp = UnsealResponse{
							Sealed:   false,
							T:        3,
							N:        5,
							Progress: 0,
						}
					} else {
						// Still sealed, show progress
						resp = UnsealResponse{
							Sealed:   true,
							T:        3,
							N:        5,
							Progress: callCount,
						}
					}

					w.WriteHeader(http.StatusOK)
					json.NewEncoder(w).Encode(resp)
				}))
				defer server.Close()

				client := NewClient(server.URL, "")
				err := client.UnsealWithKeys(context.Background(), tt.keys)

				require.NoError(t, err)
				assert.Equal(t, tt.serverCalls, callCount)
			} else {
				client := NewClient("http://localhost:8200", "")
				err := client.UnsealWithKeys(context.Background(), tt.keys)
				require.Error(t, err)
				if tt.errMsg != "" {
					assert.Contains(t, err.Error(), tt.errMsg)
				}
			}
		})
	}
}

func TestClient_UnsealWithKeys_InsufficientKeys(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Always return sealed with progress
		resp := UnsealResponse{
			Sealed:   true,
			T:        3,
			N:        5,
			Progress: 2,
		}
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	client := NewClient(server.URL, "")
	err := client.UnsealWithKeys(context.Background(), []string{"key1", "key2"})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "still sealed after")
}

func TestClient_ResetUnseal(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "PUT", r.Method)
		assert.Equal(t, "/v1/sys/unseal", r.URL.Path)

		var req UnsealRequest
		json.NewDecoder(r.Body).Decode(&req)
		assert.True(t, req.Reset)

		resp := UnsealResponse{
			Sealed:   true,
			T:        3,
			N:        5,
			Progress: 0,
		}
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	client := NewClient(server.URL, "")
	err := client.ResetUnseal(context.Background())
	require.NoError(t, err)
}

func TestClient_VerifySealed(t *testing.T) {
	tests := []struct {
		name     string
		sealed   bool
		expected bool
	}{
		{
			name:     "is sealed",
			sealed:   true,
			expected: true,
		},
		{
			name:     "is unsealed",
			sealed:   false,
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				statusCode := 200
				if tt.sealed {
					statusCode = 503
				}

				resp := HealthResponse{
					Initialized: true,
					Sealed:      tt.sealed,
				}
				w.WriteHeader(statusCode)
				json.NewEncoder(w).Encode(resp)
			}))
			defer server.Close()

			client := NewClient(server.URL, "")
			sealed, err := client.VerifySealed(context.Background())

			require.NoError(t, err)
			assert.Equal(t, tt.expected, sealed)
		})
	}
}

func TestClient_VerifyInitialized(t *testing.T) {
	tests := []struct {
		name        string
		initialized bool
		expected    bool
	}{
		{
			name:        "is initialized",
			initialized: true,
			expected:    true,
		},
		{
			name:        "not initialized",
			initialized: false,
			expected:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				statusCode := 200
				if !tt.initialized {
					statusCode = 501
				}

				resp := HealthResponse{
					Initialized: tt.initialized,
					Sealed:      true,
				}
				w.WriteHeader(statusCode)
				json.NewEncoder(w).Encode(resp)
			}))
			defer server.Close()

			client := NewClient(server.URL, "")
			initialized, err := client.VerifyInitialized(context.Background())

			require.NoError(t, err)
			assert.Equal(t, tt.expected, initialized)
		})
	}
}
