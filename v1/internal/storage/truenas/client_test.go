package truenas

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockHTTPClient implements HTTPClient for testing
type mockHTTPClient struct {
	DoFunc func(method, path string, body interface{}) ([]byte, error)
}

func (m *mockHTTPClient) Do(method, path string, body interface{}) ([]byte, error) {
	return m.DoFunc(method, path, body)
}

func TestNewClient(t *testing.T) {
	tests := []struct {
		name    string
		apiURL  string
		apiKey  string
		wantErr bool
		errMsg  string
	}{
		{
			name:    "valid client",
			apiURL:  "https://truenas.example.com",
			apiKey:  "test-api-key",
			wantErr: false,
		},
		{
			name:    "valid client with trailing slash",
			apiURL:  "https://truenas.example.com/",
			apiKey:  "test-api-key",
			wantErr: false,
		},
		{
			name:    "empty API URL",
			apiURL:  "",
			apiKey:  "test-api-key",
			wantErr: true,
			errMsg:  "API URL cannot be empty",
		},
		{
			name:    "empty API key",
			apiURL:  "https://truenas.example.com",
			apiKey:  "",
			wantErr: true,
			errMsg:  "API key cannot be empty",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client, err := NewClient(tt.apiURL, tt.apiKey)
			if tt.wantErr {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.errMsg)
				assert.Nil(t, client)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, client)
				assert.Equal(t, "https://truenas.example.com", client.baseURL)
				assert.Equal(t, "test-api-key", client.apiKey)
			}
		})
	}
}

func TestCreateDataset(t *testing.T) {
	tests := []struct {
		name       string
		config     DatasetConfig
		mockResp   interface{}
		mockErr    error
		wantErr    bool
		errMsg     string
		validateFn func(t *testing.T, method, path string, body interface{})
	}{
		{
			name: "successful creation",
			config: DatasetConfig{
				Name: "tank/test-dataset",
				Type: "FILESYSTEM",
			},
			mockResp: Dataset{
				ID:         "tank/test-dataset",
				Name:       "test-dataset",
				Pool:       "tank",
				Type:       "FILESYSTEM",
				Mountpoint: "/mnt/tank/test-dataset",
			},
			validateFn: func(t *testing.T, method, path string, body interface{}) {
				assert.Equal(t, "POST", method)
				assert.Equal(t, "/api/v2.0/pool/dataset", path)
				cfg, ok := body.(DatasetConfig)
				require.True(t, ok)
				assert.Equal(t, "tank/test-dataset", cfg.Name)
				assert.Equal(t, "FILESYSTEM", cfg.Type)
			},
		},
		{
			name: "default type to FILESYSTEM",
			config: DatasetConfig{
				Name: "tank/test-dataset",
			},
			mockResp: Dataset{
				ID:   "tank/test-dataset",
				Name: "test-dataset",
				Type: "FILESYSTEM",
			},
			validateFn: func(t *testing.T, method, path string, body interface{}) {
				cfg, ok := body.(DatasetConfig)
				require.True(t, ok)
				assert.Equal(t, "FILESYSTEM", cfg.Type)
			},
		},
		{
			name:    "empty name",
			config:  DatasetConfig{},
			wantErr: true,
			errMsg:  "dataset name cannot be empty",
		},
		{
			name: "API error",
			config: DatasetConfig{
				Name: "tank/test-dataset",
			},
			mockErr: &APIError{Message: "dataset already exists"},
			wantErr: true,
			errMsg:  "failed to create dataset",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := &Client{
				baseURL: "https://truenas.example.com",
				apiKey:  "test-key",
				httpClient: &mockHTTPClient{
					DoFunc: func(method, path string, body interface{}) ([]byte, error) {
						if tt.validateFn != nil {
							tt.validateFn(t, method, path, body)
						}
						if tt.mockErr != nil {
							return nil, tt.mockErr
						}
						return json.Marshal(tt.mockResp)
					},
				},
			}

			dataset, err := client.CreateDataset(tt.config)
			if tt.wantErr {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.errMsg)
				assert.Nil(t, dataset)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, dataset)
			}
		})
	}
}

func TestDeleteDataset(t *testing.T) {
	tests := []struct {
		name       string
		datasetName string
		mockErr    error
		wantErr    bool
		errMsg     string
		validateFn func(t *testing.T, method, path string)
	}{
		{
			name:        "successful deletion",
			datasetName: "tank/test-dataset",
			validateFn: func(t *testing.T, method, path string) {
				assert.Equal(t, "DELETE", method)
				assert.Equal(t, "/api/v2.0/pool/dataset/id/tank/test-dataset", path)
			},
		},
		{
			name:        "empty name",
			datasetName: "",
			wantErr:     true,
			errMsg:      "dataset name cannot be empty",
		},
		{
			name:        "API error",
			datasetName: "tank/test-dataset",
			mockErr:     &APIError{Message: "dataset not found"},
			wantErr:     true,
			errMsg:      "failed to delete dataset",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := &Client{
				baseURL: "https://truenas.example.com",
				apiKey:  "test-key",
				httpClient: &mockHTTPClient{
					DoFunc: func(method, path string, body interface{}) ([]byte, error) {
						if tt.validateFn != nil {
							tt.validateFn(t, method, path)
						}
						if tt.mockErr != nil {
							return nil, tt.mockErr
						}
						return []byte(`{}`), nil
					},
				},
			}

			err := client.DeleteDataset(tt.datasetName)
			if tt.wantErr {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.errMsg)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestListDatasets(t *testing.T) {
	tests := []struct {
		name     string
		mockResp interface{}
		mockErr  error
		wantErr  bool
		errMsg   string
		wantLen  int
	}{
		{
			name: "successful list",
			mockResp: []Dataset{
				{ID: "tank/dataset1", Name: "dataset1", Pool: "tank"},
				{ID: "tank/dataset2", Name: "dataset2", Pool: "tank"},
			},
			wantLen: 2,
		},
		{
			name:     "empty list",
			mockResp: []Dataset{},
			wantLen:  0,
		},
		{
			name:    "API error",
			mockErr: &APIError{Message: "unauthorized"},
			wantErr: true,
			errMsg:  "failed to list datasets",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := &Client{
				baseURL: "https://truenas.example.com",
				apiKey:  "test-key",
				httpClient: &mockHTTPClient{
					DoFunc: func(method, path string, body interface{}) ([]byte, error) {
						assert.Equal(t, "GET", method)
						assert.Equal(t, "/api/v2.0/pool/dataset", path)
						if tt.mockErr != nil {
							return nil, tt.mockErr
						}
						return json.Marshal(tt.mockResp)
					},
				},
			}

			datasets, err := client.ListDatasets()
			if tt.wantErr {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.errMsg)
				assert.Nil(t, datasets)
			} else {
				assert.NoError(t, err)
				assert.Len(t, datasets, tt.wantLen)
			}
		})
	}
}

func TestGetDataset(t *testing.T) {
	tests := []struct {
		name        string
		datasetName string
		mockResp    interface{}
		mockErr     error
		wantErr     bool
		errMsg      string
		validateFn  func(t *testing.T, method, path string)
	}{
		{
			name:        "successful get",
			datasetName: "tank/test-dataset",
			mockResp: Dataset{
				ID:   "tank/test-dataset",
				Name: "test-dataset",
				Pool: "tank",
			},
			validateFn: func(t *testing.T, method, path string) {
				assert.Equal(t, "GET", method)
				assert.Equal(t, "/api/v2.0/pool/dataset/id/tank/test-dataset", path)
			},
		},
		{
			name:        "empty name",
			datasetName: "",
			wantErr:     true,
			errMsg:      "dataset name cannot be empty",
		},
		{
			name:        "API error",
			datasetName: "tank/test-dataset",
			mockErr:     &APIError{Message: "dataset not found"},
			wantErr:     true,
			errMsg:      "failed to get dataset",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := &Client{
				baseURL: "https://truenas.example.com",
				apiKey:  "test-key",
				httpClient: &mockHTTPClient{
					DoFunc: func(method, path string, body interface{}) ([]byte, error) {
						if tt.validateFn != nil {
							tt.validateFn(t, method, path)
						}
						if tt.mockErr != nil {
							return nil, tt.mockErr
						}
						return json.Marshal(tt.mockResp)
					},
				},
			}

			dataset, err := client.GetDataset(tt.datasetName)
			if tt.wantErr {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.errMsg)
				assert.Nil(t, dataset)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, dataset)
			}
		})
	}
}

func TestCreateNFSShare(t *testing.T) {
	tests := []struct {
		name       string
		config     NFSConfig
		mockResp   interface{}
		mockErr    error
		wantErr    bool
		errMsg     string
		validateFn func(t *testing.T, method, path string, body interface{})
	}{
		{
			name: "successful creation",
			config: NFSConfig{
				Path:     "/mnt/tank/share",
				Comment:  "Test share",
				Networks: []string{"192.168.1.0/24"},
			},
			mockResp: NFSShare{
				ID:       1,
				Path:     "/mnt/tank/share",
				Comment:  "Test share",
				Networks: []string{"192.168.1.0/24"},
				Enabled:  true,
			},
			validateFn: func(t *testing.T, method, path string, body interface{}) {
				assert.Equal(t, "POST", method)
				assert.Equal(t, "/api/v2.0/sharing/nfs", path)
				cfg, ok := body.(NFSConfig)
				require.True(t, ok)
				assert.Equal(t, "/mnt/tank/share", cfg.Path)
			},
		},
		{
			name:    "empty path",
			config:  NFSConfig{},
			wantErr: true,
			errMsg:  "NFS share path cannot be empty",
		},
		{
			name: "API error",
			config: NFSConfig{
				Path: "/mnt/tank/share",
			},
			mockErr: &APIError{Message: "share already exists"},
			wantErr: true,
			errMsg:  "failed to create NFS share",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := &Client{
				baseURL: "https://truenas.example.com",
				apiKey:  "test-key",
				httpClient: &mockHTTPClient{
					DoFunc: func(method, path string, body interface{}) ([]byte, error) {
						if tt.validateFn != nil {
							tt.validateFn(t, method, path, body)
						}
						if tt.mockErr != nil {
							return nil, tt.mockErr
						}
						return json.Marshal(tt.mockResp)
					},
				},
			}

			share, err := client.CreateNFSShare(tt.config)
			if tt.wantErr {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.errMsg)
				assert.Nil(t, share)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, share)
			}
		})
	}
}

func TestDeleteNFSShare(t *testing.T) {
	tests := []struct {
		name       string
		shareID    int
		mockErr    error
		wantErr    bool
		errMsg     string
		validateFn func(t *testing.T, method, path string)
	}{
		{
			name:    "successful deletion",
			shareID: 1,
			validateFn: func(t *testing.T, method, path string) {
				assert.Equal(t, "DELETE", method)
				assert.Equal(t, "/api/v2.0/sharing/nfs/id/1", path)
			},
		},
		{
			name:    "invalid ID - zero",
			shareID: 0,
			wantErr: true,
			errMsg:  "invalid NFS share ID: 0",
		},
		{
			name:    "invalid ID - negative",
			shareID: -1,
			wantErr: true,
			errMsg:  "invalid NFS share ID: -1",
		},
		{
			name:    "API error",
			shareID: 1,
			mockErr: &APIError{Message: "share not found"},
			wantErr: true,
			errMsg:  "failed to delete NFS share",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := &Client{
				baseURL: "https://truenas.example.com",
				apiKey:  "test-key",
				httpClient: &mockHTTPClient{
					DoFunc: func(method, path string, body interface{}) ([]byte, error) {
						if tt.validateFn != nil {
							tt.validateFn(t, method, path)
						}
						if tt.mockErr != nil {
							return nil, tt.mockErr
						}
						return []byte(`{}`), nil
					},
				},
			}

			err := client.DeleteNFSShare(tt.shareID)
			if tt.wantErr {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.errMsg)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestListNFSShares(t *testing.T) {
	tests := []struct {
		name     string
		mockResp interface{}
		mockErr  error
		wantErr  bool
		errMsg   string
		wantLen  int
	}{
		{
			name: "successful list",
			mockResp: []NFSShare{
				{ID: 1, Path: "/mnt/tank/share1", Enabled: true},
				{ID: 2, Path: "/mnt/tank/share2", Enabled: false},
			},
			wantLen: 2,
		},
		{
			name:     "empty list",
			mockResp: []NFSShare{},
			wantLen:  0,
		},
		{
			name:    "API error",
			mockErr: &APIError{Message: "unauthorized"},
			wantErr: true,
			errMsg:  "failed to list NFS shares",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := &Client{
				baseURL: "https://truenas.example.com",
				apiKey:  "test-key",
				httpClient: &mockHTTPClient{
					DoFunc: func(method, path string, body interface{}) ([]byte, error) {
						assert.Equal(t, "GET", method)
						assert.Equal(t, "/api/v2.0/sharing/nfs", path)
						if tt.mockErr != nil {
							return nil, tt.mockErr
						}
						return json.Marshal(tt.mockResp)
					},
				},
			}

			shares, err := client.ListNFSShares()
			if tt.wantErr {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.errMsg)
				assert.Nil(t, shares)
			} else {
				assert.NoError(t, err)
				assert.Len(t, shares, tt.wantLen)
			}
		})
	}
}

func TestGetNFSShare(t *testing.T) {
	tests := []struct {
		name       string
		shareID    int
		mockResp   interface{}
		mockErr    error
		wantErr    bool
		errMsg     string
		validateFn func(t *testing.T, method, path string)
	}{
		{
			name:    "successful get",
			shareID: 1,
			mockResp: NFSShare{
				ID:      1,
				Path:    "/mnt/tank/share",
				Enabled: true,
			},
			validateFn: func(t *testing.T, method, path string) {
				assert.Equal(t, "GET", method)
				assert.Equal(t, "/api/v2.0/sharing/nfs/id/1", path)
			},
		},
		{
			name:    "invalid ID - zero",
			shareID: 0,
			wantErr: true,
			errMsg:  "invalid NFS share ID: 0",
		},
		{
			name:    "invalid ID - negative",
			shareID: -1,
			wantErr: true,
			errMsg:  "invalid NFS share ID: -1",
		},
		{
			name:    "API error",
			shareID: 1,
			mockErr: &APIError{Message: "share not found"},
			wantErr: true,
			errMsg:  "failed to get NFS share",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := &Client{
				baseURL: "https://truenas.example.com",
				apiKey:  "test-key",
				httpClient: &mockHTTPClient{
					DoFunc: func(method, path string, body interface{}) ([]byte, error) {
						if tt.validateFn != nil {
							tt.validateFn(t, method, path)
						}
						if tt.mockErr != nil {
							return nil, tt.mockErr
						}
						return json.Marshal(tt.mockResp)
					},
				},
			}

			share, err := client.GetNFSShare(tt.shareID)
			if tt.wantErr {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.errMsg)
				assert.Nil(t, share)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, share)
			}
		})
	}
}

func TestListPools(t *testing.T) {
	tests := []struct {
		name     string
		mockResp interface{}
		mockErr  error
		wantErr  bool
		errMsg   string
		wantLen  int
	}{
		{
			name: "successful list",
			mockResp: []Pool{
				{ID: 1, Name: "tank", Status: "ONLINE", Healthy: true},
				{ID: 2, Name: "backup", Status: "ONLINE", Healthy: true},
			},
			wantLen: 2,
		},
		{
			name:     "empty list",
			mockResp: []Pool{},
			wantLen:  0,
		},
		{
			name:    "API error",
			mockErr: &APIError{Message: "unauthorized"},
			wantErr: true,
			errMsg:  "failed to list pools",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := &Client{
				baseURL: "https://truenas.example.com",
				apiKey:  "test-key",
				httpClient: &mockHTTPClient{
					DoFunc: func(method, path string, body interface{}) ([]byte, error) {
						assert.Equal(t, "GET", method)
						assert.Equal(t, "/api/v2.0/pool", path)
						if tt.mockErr != nil {
							return nil, tt.mockErr
						}
						return json.Marshal(tt.mockResp)
					},
				},
			}

			pools, err := client.ListPools()
			if tt.wantErr {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.errMsg)
				assert.Nil(t, pools)
			} else {
				assert.NoError(t, err)
				assert.Len(t, pools, tt.wantLen)
			}
		})
	}
}

func TestGetPool(t *testing.T) {
	tests := []struct {
		name       string
		poolID     int
		mockResp   interface{}
		mockErr    error
		wantErr    bool
		errMsg     string
		validateFn func(t *testing.T, method, path string)
	}{
		{
			name:   "successful get",
			poolID: 1,
			mockResp: Pool{
				ID:      1,
				Name:    "tank",
				Status:  "ONLINE",
				Healthy: true,
			},
			validateFn: func(t *testing.T, method, path string) {
				assert.Equal(t, "GET", method)
				assert.Equal(t, "/api/v2.0/pool/id/1", path)
			},
		},
		{
			name:    "invalid ID - zero",
			poolID:  0,
			wantErr: true,
			errMsg:  "invalid pool ID: 0",
		},
		{
			name:    "invalid ID - negative",
			poolID:  -1,
			wantErr: true,
			errMsg:  "invalid pool ID: -1",
		},
		{
			name:    "API error",
			poolID:  1,
			mockErr: &APIError{Message: "pool not found"},
			wantErr: true,
			errMsg:  "failed to get pool",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := &Client{
				baseURL: "https://truenas.example.com",
				apiKey:  "test-key",
				httpClient: &mockHTTPClient{
					DoFunc: func(method, path string, body interface{}) ([]byte, error) {
						if tt.validateFn != nil {
							tt.validateFn(t, method, path)
						}
						if tt.mockErr != nil {
							return nil, tt.mockErr
						}
						return json.Marshal(tt.mockResp)
					},
				},
			}

			pool, err := client.GetPool(tt.poolID)
			if tt.wantErr {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.errMsg)
				assert.Nil(t, pool)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, pool)
			}
		})
	}
}

func TestPing(t *testing.T) {
	tests := []struct {
		name       string
		mockErr    error
		wantErr    bool
		errMsg     string
		validateFn func(t *testing.T, method, path string)
	}{
		{
			name: "successful ping",
			validateFn: func(t *testing.T, method, path string) {
				assert.Equal(t, "GET", method)
				assert.Equal(t, "/api/v2.0/system/info", path)
			},
		},
		{
			name:    "ping failure",
			mockErr: fmt.Errorf("connection refused"),
			wantErr: true,
			errMsg:  "ping failed",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := &Client{
				baseURL: "https://truenas.example.com",
				apiKey:  "test-key",
				httpClient: &mockHTTPClient{
					DoFunc: func(method, path string, body interface{}) ([]byte, error) {
						if tt.validateFn != nil {
							tt.validateFn(t, method, path)
						}
						if tt.mockErr != nil {
							return nil, tt.mockErr
						}
						return []byte(`{"version": "13.0"}`), nil
					},
				},
			}

			err := client.Ping()
			if tt.wantErr {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.errMsg)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestCreateDataset_JSONError(t *testing.T) {
	client := &Client{
		baseURL: "https://truenas.example.com",
		apiKey:  "test-key",
		httpClient: &mockHTTPClient{
			DoFunc: func(method, path string, body interface{}) ([]byte, error) {
				// Return invalid JSON
				return []byte(`{invalid json}`), nil
			},
		},
	}

	dataset, err := client.CreateDataset(DatasetConfig{Name: "tank/test"})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to parse response")
	assert.Nil(t, dataset)
}

func TestListDatasets_JSONError(t *testing.T) {
	client := &Client{
		baseURL: "https://truenas.example.com",
		apiKey:  "test-key",
		httpClient: &mockHTTPClient{
			DoFunc: func(method, path string, body interface{}) ([]byte, error) {
				// Return invalid JSON
				return []byte(`{invalid json}`), nil
			},
		},
	}

	datasets, err := client.ListDatasets()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to parse response")
	assert.Nil(t, datasets)
}

func TestGetDataset_JSONError(t *testing.T) {
	client := &Client{
		baseURL: "https://truenas.example.com",
		apiKey:  "test-key",
		httpClient: &mockHTTPClient{
			DoFunc: func(method, path string, body interface{}) ([]byte, error) {
				// Return invalid JSON
				return []byte(`{invalid json}`), nil
			},
		},
	}

	dataset, err := client.GetDataset("tank/test")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to parse response")
	assert.Nil(t, dataset)
}

func TestCreateNFSShare_JSONError(t *testing.T) {
	client := &Client{
		baseURL: "https://truenas.example.com",
		apiKey:  "test-key",
		httpClient: &mockHTTPClient{
			DoFunc: func(method, path string, body interface{}) ([]byte, error) {
				// Return invalid JSON
				return []byte(`{invalid json}`), nil
			},
		},
	}

	share, err := client.CreateNFSShare(NFSConfig{Path: "/mnt/tank/share"})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to parse response")
	assert.Nil(t, share)
}

func TestListNFSShares_JSONError(t *testing.T) {
	client := &Client{
		baseURL: "https://truenas.example.com",
		apiKey:  "test-key",
		httpClient: &mockHTTPClient{
			DoFunc: func(method, path string, body interface{}) ([]byte, error) {
				// Return invalid JSON
				return []byte(`{invalid json}`), nil
			},
		},
	}

	shares, err := client.ListNFSShares()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to parse response")
	assert.Nil(t, shares)
}

func TestGetNFSShare_JSONError(t *testing.T) {
	client := &Client{
		baseURL: "https://truenas.example.com",
		apiKey:  "test-key",
		httpClient: &mockHTTPClient{
			DoFunc: func(method, path string, body interface{}) ([]byte, error) {
				// Return invalid JSON
				return []byte(`{invalid json}`), nil
			},
		},
	}

	share, err := client.GetNFSShare(1)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to parse response")
	assert.Nil(t, share)
}

func TestListPools_JSONError(t *testing.T) {
	client := &Client{
		baseURL: "https://truenas.example.com",
		apiKey:  "test-key",
		httpClient: &mockHTTPClient{
			DoFunc: func(method, path string, body interface{}) ([]byte, error) {
				// Return invalid JSON
				return []byte(`{invalid json}`), nil
			},
		},
	}

	pools, err := client.ListPools()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to parse response")
	assert.Nil(t, pools)
}

func TestGetPool_JSONError(t *testing.T) {
	client := &Client{
		baseURL: "https://truenas.example.com",
		apiKey:  "test-key",
		httpClient: &mockHTTPClient{
			DoFunc: func(method, path string, body interface{}) ([]byte, error) {
				// Return invalid JSON
				return []byte(`{invalid json}`), nil
			},
		},
	}

	pool, err := client.GetPool(1)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to parse response")
	assert.Nil(t, pool)
}

func TestAPIError_Error(t *testing.T) {
	tests := []struct {
		name    string
		apiErr  APIError
		wantMsg string
	}{
		{
			name: "message takes precedence",
			apiErr: APIError{
				ErrorMsg: "generic error",
				Message:  "detailed message",
			},
			wantMsg: "detailed message",
		},
		{
			name: "fallback to error field",
			apiErr: APIError{
				ErrorMsg: "generic error",
			},
			wantMsg: "generic error",
		},
		{
			name:    "empty error",
			apiErr:  APIError{},
			wantMsg: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.wantMsg, tt.apiErr.Error())
		})
	}
}

// Integration-style tests using real HTTP server to test defaultHTTPClient
func TestDefaultHTTPClient_Integration(t *testing.T) {
	t.Run("successful request", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			assert.Equal(t, "Bearer test-api-key", r.Header.Get("Authorization"))
			assert.Equal(t, "application/json", r.Header.Get("Content-Type"))
			assert.Equal(t, "/api/v2.0/pool/dataset", r.URL.Path)

			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode([]Dataset{
				{ID: "tank/test", Name: "test"},
			})
		}))
		defer server.Close()

		client, err := NewClient(server.URL, "test-api-key")
		require.NoError(t, err)

		datasets, err := client.ListDatasets()
		assert.NoError(t, err)
		assert.Len(t, datasets, 1)
		assert.Equal(t, "tank/test", datasets[0].ID)
	})

	t.Run("API error response", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(APIError{
				Message: "invalid request",
			})
		}))
		defer server.Close()

		client, err := NewClient(server.URL, "test-api-key")
		require.NoError(t, err)

		datasets, err := client.ListDatasets()
		assert.Error(t, err)
		assert.Nil(t, datasets)
		assert.Contains(t, err.Error(), "invalid request")
	})

	t.Run("non-JSON error response", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
			w.Write([]byte("Internal Server Error"))
		}))
		defer server.Close()

		client, err := NewClient(server.URL, "test-api-key")
		require.NoError(t, err)

		datasets, err := client.ListDatasets()
		assert.Error(t, err)
		assert.Nil(t, datasets)
		assert.Contains(t, err.Error(), "500")
	})

	t.Run("POST request with body", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			assert.Equal(t, "POST", r.Method)
			assert.Equal(t, "/api/v2.0/pool/dataset", r.URL.Path)

			var cfg DatasetConfig
			err := json.NewDecoder(r.Body).Decode(&cfg)
			require.NoError(t, err)
			assert.Equal(t, "tank/new-dataset", cfg.Name)

			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(Dataset{
				ID:   "tank/new-dataset",
				Name: "new-dataset",
			})
		}))
		defer server.Close()

		client, err := NewClient(server.URL, "test-api-key")
		require.NoError(t, err)

		dataset, err := client.CreateDataset(DatasetConfig{
			Name: "tank/new-dataset",
			Type: "FILESYSTEM",
		})
		assert.NoError(t, err)
		assert.NotNil(t, dataset)
		assert.Equal(t, "tank/new-dataset", dataset.ID)
	})

	t.Run("ping endpoint", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			assert.Equal(t, "GET", r.Method)
			assert.Equal(t, "/api/v2.0/system/info", r.URL.Path)

			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{"version": "13.0"}`))
		}))
		defer server.Close()

		client, err := NewClient(server.URL, "test-api-key")
		require.NoError(t, err)

		err = client.Ping()
		assert.NoError(t, err)
	})
}
