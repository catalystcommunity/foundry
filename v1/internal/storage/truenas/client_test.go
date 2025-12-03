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

func TestGetSystemInfo(t *testing.T) {
	tests := []struct {
		name       string
		mockResp   interface{}
		mockErr    error
		wantErr    bool
		errMsg     string
		validateFn func(t *testing.T, method, path string)
	}{
		{
			name: "successful get",
			mockResp: SystemInfo{
				Version:       "TrueNAS-SCALE-22.12.0",
				Hostname:      "truenas.local",
				PhysicalMem:   34359738368,
				Model:         "Intel(R) Xeon(R)",
				Cores:         8,
				UptimeSeconds: 86400,
			},
			validateFn: func(t *testing.T, method, path string) {
				assert.Equal(t, "GET", method)
				assert.Equal(t, "/api/v2.0/system/info", path)
			},
		},
		{
			name:    "API error",
			mockErr: &APIError{Message: "unauthorized"},
			wantErr: true,
			errMsg:  "failed to get system info",
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

			info, err := client.GetSystemInfo()
			if tt.wantErr {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.errMsg)
				assert.Nil(t, info)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, info)
				assert.Equal(t, "TrueNAS-SCALE-22.12.0", info.Version)
			}
		})
	}
}

func TestListDisks(t *testing.T) {
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
			mockResp: []Disk{
				{Name: "sda", Serial: "ABC123", Size: 1000000000000, Pool: "tank"},
				{Name: "sdb", Serial: "DEF456", Size: 1000000000000, Pool: ""},
			},
			wantLen: 2,
		},
		{
			name:     "empty list",
			mockResp: []Disk{},
			wantLen:  0,
		},
		{
			name:    "API error",
			mockErr: &APIError{Message: "unauthorized"},
			wantErr: true,
			errMsg:  "failed to list disks",
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
						assert.Equal(t, "/api/v2.0/disk", path)
						if tt.mockErr != nil {
							return nil, tt.mockErr
						}
						return json.Marshal(tt.mockResp)
					},
				},
			}

			disks, err := client.ListDisks()
			if tt.wantErr {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.errMsg)
				assert.Nil(t, disks)
			} else {
				assert.NoError(t, err)
				assert.Len(t, disks, tt.wantLen)
			}
		})
	}
}

func TestGetUnusedDisks(t *testing.T) {
	tests := []struct {
		name        string
		mockResp    interface{}
		mockErr     error
		wantErr     bool
		errMsg      string
		wantUnused  int
	}{
		{
			name: "filters unused disks",
			mockResp: []Disk{
				{Name: "sda", Serial: "ABC123", Pool: "tank"},
				{Name: "sdb", Serial: "DEF456", Pool: ""},
				{Name: "sdc", Serial: "GHI789", Pool: ""},
				{Name: "sdd", Serial: "JKL012", Pool: "backup"},
			},
			wantUnused: 2,
		},
		{
			name: "all disks in pools",
			mockResp: []Disk{
				{Name: "sda", Serial: "ABC123", Pool: "tank"},
				{Name: "sdb", Serial: "DEF456", Pool: "backup"},
			},
			wantUnused: 0,
		},
		{
			name: "all disks unused",
			mockResp: []Disk{
				{Name: "sda", Serial: "ABC123", Pool: ""},
				{Name: "sdb", Serial: "DEF456", Pool: ""},
			},
			wantUnused: 2,
		},
		{
			name:    "API error",
			mockErr: &APIError{Message: "unauthorized"},
			wantErr: true,
			errMsg:  "failed to list disks",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := &Client{
				baseURL: "https://truenas.example.com",
				apiKey:  "test-key",
				httpClient: &mockHTTPClient{
					DoFunc: func(method, path string, body interface{}) ([]byte, error) {
						if tt.mockErr != nil {
							return nil, tt.mockErr
						}
						return json.Marshal(tt.mockResp)
					},
				},
			}

			disks, err := client.GetUnusedDisks()
			if tt.wantErr {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.errMsg)
			} else {
				assert.NoError(t, err)
				assert.Len(t, disks, tt.wantUnused)
				for _, disk := range disks {
					assert.Empty(t, disk.Pool)
				}
			}
		})
	}
}

func TestCreatePool(t *testing.T) {
	tests := []struct {
		name       string
		config     PoolCreateConfig
		mockResp   interface{}
		mockErr    error
		wantErr    bool
		errMsg     string
		validateFn func(t *testing.T, method, path string, body interface{})
	}{
		{
			name: "successful creation",
			config: PoolCreateConfig{
				Name: "tank",
				Topology: PoolCreateTopology{
					Data: []PoolCreateVDev{
						{Type: "MIRROR", Disks: []string{"sda", "sdb"}},
					},
				},
			},
			mockResp: Pool{
				ID:      1,
				Name:    "tank",
				Status:  "ONLINE",
				Healthy: true,
			},
			validateFn: func(t *testing.T, method, path string, body interface{}) {
				assert.Equal(t, "POST", method)
				assert.Equal(t, "/api/v2.0/pool", path)
			},
		},
		{
			name: "empty name",
			config: PoolCreateConfig{
				Topology: PoolCreateTopology{
					Data: []PoolCreateVDev{{Type: "STRIPE", Disks: []string{"sda"}}},
				},
			},
			wantErr: true,
			errMsg:  "pool name cannot be empty",
		},
		{
			name: "no data vdevs",
			config: PoolCreateConfig{
				Name:     "tank",
				Topology: PoolCreateTopology{},
			},
			wantErr: true,
			errMsg:  "pool must have at least one data vdev",
		},
		{
			name: "API error",
			config: PoolCreateConfig{
				Name: "tank",
				Topology: PoolCreateTopology{
					Data: []PoolCreateVDev{{Type: "STRIPE", Disks: []string{"sda"}}},
				},
			},
			mockErr: &APIError{Message: "pool already exists"},
			wantErr: true,
			errMsg:  "failed to create pool",
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

			pool, err := client.CreatePool(tt.config)
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

func TestListServices(t *testing.T) {
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
			mockResp: []Service{
				{ID: 1, Service: "nfs", State: "RUNNING", Enable: true},
				{ID: 2, Service: "iscsitarget", State: "STOPPED", Enable: false},
			},
			wantLen: 2,
		},
		{
			name:     "empty list",
			mockResp: []Service{},
			wantLen:  0,
		},
		{
			name:    "API error",
			mockErr: &APIError{Message: "unauthorized"},
			wantErr: true,
			errMsg:  "failed to list services",
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
						assert.Equal(t, "/api/v2.0/service", path)
						if tt.mockErr != nil {
							return nil, tt.mockErr
						}
						return json.Marshal(tt.mockResp)
					},
				},
			}

			services, err := client.ListServices()
			if tt.wantErr {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.errMsg)
				assert.Nil(t, services)
			} else {
				assert.NoError(t, err)
				assert.Len(t, services, tt.wantLen)
			}
		})
	}
}

func TestGetService(t *testing.T) {
	tests := []struct {
		name        string
		serviceName string
		mockResp    interface{}
		mockErr     error
		wantErr     bool
		errMsg      string
	}{
		{
			name:        "successful get",
			serviceName: "nfs",
			mockResp: []Service{
				{ID: 1, Service: "nfs", State: "RUNNING", Enable: true},
				{ID: 2, Service: "iscsitarget", State: "STOPPED", Enable: false},
			},
		},
		{
			name:        "service not found",
			serviceName: "nonexistent",
			mockResp: []Service{
				{ID: 1, Service: "nfs", State: "RUNNING", Enable: true},
			},
			wantErr: true,
			errMsg:  "service \"nonexistent\" not found",
		},
		{
			name:        "API error",
			serviceName: "nfs",
			mockErr:     &APIError{Message: "unauthorized"},
			wantErr:     true,
			errMsg:      "failed to list services",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := &Client{
				baseURL: "https://truenas.example.com",
				apiKey:  "test-key",
				httpClient: &mockHTTPClient{
					DoFunc: func(method, path string, body interface{}) ([]byte, error) {
						if tt.mockErr != nil {
							return nil, tt.mockErr
						}
						return json.Marshal(tt.mockResp)
					},
				},
			}

			svc, err := client.GetService(tt.serviceName)
			if tt.wantErr {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.errMsg)
				assert.Nil(t, svc)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, svc)
				assert.Equal(t, tt.serviceName, svc.Service)
			}
		})
	}
}

func TestStartService(t *testing.T) {
	tests := []struct {
		name        string
		serviceName string
		mockErr     error
		wantErr     bool
		errMsg      string
		validateFn  func(t *testing.T, method, path string, body interface{})
	}{
		{
			name:        "successful start",
			serviceName: "nfs",
			validateFn: func(t *testing.T, method, path string, body interface{}) {
				assert.Equal(t, "POST", method)
				assert.Equal(t, "/api/v2.0/service/start", path)
				bodyMap, ok := body.(map[string]string)
				require.True(t, ok)
				assert.Equal(t, "nfs", bodyMap["service"])
			},
		},
		{
			name:        "API error",
			serviceName: "nfs",
			mockErr:     &APIError{Message: "service failed to start"},
			wantErr:     true,
			errMsg:      "failed to start service nfs",
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
						return []byte(`true`), nil
					},
				},
			}

			err := client.StartService(tt.serviceName)
			if tt.wantErr {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.errMsg)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestStopService(t *testing.T) {
	tests := []struct {
		name        string
		serviceName string
		mockErr     error
		wantErr     bool
		errMsg      string
		validateFn  func(t *testing.T, method, path string, body interface{})
	}{
		{
			name:        "successful stop",
			serviceName: "nfs",
			validateFn: func(t *testing.T, method, path string, body interface{}) {
				assert.Equal(t, "POST", method)
				assert.Equal(t, "/api/v2.0/service/stop", path)
				bodyMap, ok := body.(map[string]string)
				require.True(t, ok)
				assert.Equal(t, "nfs", bodyMap["service"])
			},
		},
		{
			name:        "API error",
			serviceName: "nfs",
			mockErr:     &APIError{Message: "service failed to stop"},
			wantErr:     true,
			errMsg:      "failed to stop service nfs",
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
						return []byte(`true`), nil
					},
				},
			}

			err := client.StopService(tt.serviceName)
			if tt.wantErr {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.errMsg)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestEnableService(t *testing.T) {
	tests := []struct {
		name        string
		serviceName string
		mockResp    []Service
		mockErr     error
		wantErr     bool
		errMsg      string
	}{
		{
			name:        "successful enable",
			serviceName: "nfs",
			mockResp: []Service{
				{ID: 1, Service: "nfs", State: "STOPPED", Enable: false},
			},
		},
		{
			name:        "service not found",
			serviceName: "nonexistent",
			mockResp:    []Service{},
			wantErr:     true,
			errMsg:      "service \"nonexistent\" not found",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			callCount := 0
			client := &Client{
				baseURL: "https://truenas.example.com",
				apiKey:  "test-key",
				httpClient: &mockHTTPClient{
					DoFunc: func(method, path string, body interface{}) ([]byte, error) {
						callCount++
						if tt.mockErr != nil {
							return nil, tt.mockErr
						}
						// First call is to list services
						if callCount == 1 {
							return json.Marshal(tt.mockResp)
						}
						// Second call is PUT to enable
						assert.Equal(t, "PUT", method)
						return []byte(`{}`), nil
					},
				},
			}

			err := client.EnableService(tt.serviceName)
			if tt.wantErr {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.errMsg)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestEnsureServiceRunning(t *testing.T) {
	tests := []struct {
		name        string
		serviceName string
		service     Service
		wantErr     bool
		errMsg      string
		expectCalls int
	}{
		{
			name:        "already running and enabled",
			serviceName: "nfs",
			service:     Service{ID: 1, Service: "nfs", State: "RUNNING", Enable: true},
			expectCalls: 1, // Just the list call
		},
		{
			name:        "needs enable and start",
			serviceName: "nfs",
			service:     Service{ID: 1, Service: "nfs", State: "STOPPED", Enable: false},
			expectCalls: 4, // list, list for enable, put enable, start
		},
		{
			name:        "just needs start",
			serviceName: "nfs",
			service:     Service{ID: 1, Service: "nfs", State: "STOPPED", Enable: true},
			expectCalls: 2, // list, start
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			callCount := 0
			client := &Client{
				baseURL: "https://truenas.example.com",
				apiKey:  "test-key",
				httpClient: &mockHTTPClient{
					DoFunc: func(method, path string, body interface{}) ([]byte, error) {
						callCount++
						// Return service list
						if method == "GET" && path == "/api/v2.0/service" {
							return json.Marshal([]Service{tt.service})
						}
						return []byte(`{}`), nil
					},
				},
			}

			err := client.EnsureServiceRunning(tt.serviceName)
			if tt.wantErr {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.errMsg)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestListISCSIPortals(t *testing.T) {
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
			mockResp: []ISCSIPortal{
				{ID: 1, Tag: 1, Listen: []ISCSIPortalListen{{IP: "0.0.0.0", Port: 3260}}},
			},
			wantLen: 1,
		},
		{
			name:     "empty list",
			mockResp: []ISCSIPortal{},
			wantLen:  0,
		},
		{
			name:    "API error",
			mockErr: &APIError{Message: "unauthorized"},
			wantErr: true,
			errMsg:  "failed to list iSCSI portals",
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
						assert.Equal(t, "/api/v2.0/iscsi/portal", path)
						if tt.mockErr != nil {
							return nil, tt.mockErr
						}
						return json.Marshal(tt.mockResp)
					},
				},
			}

			portals, err := client.ListISCSIPortals()
			if tt.wantErr {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.errMsg)
				assert.Nil(t, portals)
			} else {
				assert.NoError(t, err)
				assert.Len(t, portals, tt.wantLen)
			}
		})
	}
}

func TestCreateISCSIPortal(t *testing.T) {
	tests := []struct {
		name       string
		config     ISCSIPortalConfig
		mockResp   interface{}
		mockErr    error
		wantErr    bool
		errMsg     string
		validateFn func(t *testing.T, method, path string, body interface{})
	}{
		{
			name: "successful creation",
			config: ISCSIPortalConfig{
				Listen: []ISCSIPortalListen{{IP: "0.0.0.0", Port: 3260}},
			},
			mockResp: ISCSIPortal{
				ID:     1,
				Tag:    1,
				Listen: []ISCSIPortalListen{{IP: "0.0.0.0", Port: 3260}},
			},
			validateFn: func(t *testing.T, method, path string, body interface{}) {
				assert.Equal(t, "POST", method)
				assert.Equal(t, "/api/v2.0/iscsi/portal", path)
			},
		},
		{
			name:    "no listen addresses",
			config:  ISCSIPortalConfig{},
			wantErr: true,
			errMsg:  "portal must have at least one listen address",
		},
		{
			name: "API error",
			config: ISCSIPortalConfig{
				Listen: []ISCSIPortalListen{{IP: "0.0.0.0", Port: 3260}},
			},
			mockErr: &APIError{Message: "portal already exists"},
			wantErr: true,
			errMsg:  "failed to create iSCSI portal",
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

			portal, err := client.CreateISCSIPortal(tt.config)
			if tt.wantErr {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.errMsg)
				assert.Nil(t, portal)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, portal)
			}
		})
	}
}

func TestListISCSIInitiators(t *testing.T) {
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
			mockResp: []ISCSIInitiator{
				{ID: 1, Tag: 1, Initiators: []string{}, Comment: "Allow all"},
			},
			wantLen: 1,
		},
		{
			name:     "empty list",
			mockResp: []ISCSIInitiator{},
			wantLen:  0,
		},
		{
			name:    "API error",
			mockErr: &APIError{Message: "unauthorized"},
			wantErr: true,
			errMsg:  "failed to list iSCSI initiators",
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
						assert.Equal(t, "/api/v2.0/iscsi/initiator", path)
						if tt.mockErr != nil {
							return nil, tt.mockErr
						}
						return json.Marshal(tt.mockResp)
					},
				},
			}

			initiators, err := client.ListISCSIInitiators()
			if tt.wantErr {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.errMsg)
				assert.Nil(t, initiators)
			} else {
				assert.NoError(t, err)
				assert.Len(t, initiators, tt.wantLen)
			}
		})
	}
}

func TestCreateISCSIInitiator(t *testing.T) {
	tests := []struct {
		name       string
		config     ISCSIInitiatorConfig
		mockResp   interface{}
		mockErr    error
		wantErr    bool
		errMsg     string
		validateFn func(t *testing.T, method, path string, body interface{})
	}{
		{
			name: "successful creation - allow all",
			config: ISCSIInitiatorConfig{
				Comment: "Allow all initiators",
			},
			mockResp: ISCSIInitiator{
				ID:         1,
				Tag:        1,
				Initiators: []string{},
				Comment:    "Allow all initiators",
			},
			validateFn: func(t *testing.T, method, path string, body interface{}) {
				assert.Equal(t, "POST", method)
				assert.Equal(t, "/api/v2.0/iscsi/initiator", path)
			},
		},
		{
			name: "successful creation - specific initiators",
			config: ISCSIInitiatorConfig{
				Initiators: []string{"iqn.2024-01.com.example:initiator1"},
				Comment:    "Specific initiator",
			},
			mockResp: ISCSIInitiator{
				ID:         1,
				Tag:        1,
				Initiators: []string{"iqn.2024-01.com.example:initiator1"},
				Comment:    "Specific initiator",
			},
		},
		{
			name:    "API error",
			config:  ISCSIInitiatorConfig{Comment: "Test"},
			mockErr: &APIError{Message: "failed"},
			wantErr: true,
			errMsg:  "failed to create iSCSI initiator",
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

			initiator, err := client.CreateISCSIInitiator(tt.config)
			if tt.wantErr {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.errMsg)
				assert.Nil(t, initiator)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, initiator)
			}
		})
	}
}

func TestListISCSITargets(t *testing.T) {
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
			mockResp: []ISCSITarget{
				{ID: 1, Name: "iqn.2024-01.com.example:target1", Mode: "ISCSI"},
			},
			wantLen: 1,
		},
		{
			name:     "empty list",
			mockResp: []ISCSITarget{},
			wantLen:  0,
		},
		{
			name:    "API error",
			mockErr: &APIError{Message: "unauthorized"},
			wantErr: true,
			errMsg:  "failed to list iSCSI targets",
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
						assert.Equal(t, "/api/v2.0/iscsi/target", path)
						if tt.mockErr != nil {
							return nil, tt.mockErr
						}
						return json.Marshal(tt.mockResp)
					},
				},
			}

			targets, err := client.ListISCSITargets()
			if tt.wantErr {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.errMsg)
				assert.Nil(t, targets)
			} else {
				assert.NoError(t, err)
				assert.Len(t, targets, tt.wantLen)
			}
		})
	}
}

func TestCreateISCSITarget(t *testing.T) {
	tests := []struct {
		name       string
		config     ISCSITargetConfig
		mockResp   interface{}
		mockErr    error
		wantErr    bool
		errMsg     string
		validateFn func(t *testing.T, method, path string, body interface{})
	}{
		{
			name: "successful creation",
			config: ISCSITargetConfig{
				Name: "target1",
				Mode: "ISCSI",
			},
			mockResp: ISCSITarget{
				ID:   1,
				Name: "target1",
				Mode: "ISCSI",
			},
			validateFn: func(t *testing.T, method, path string, body interface{}) {
				assert.Equal(t, "POST", method)
				assert.Equal(t, "/api/v2.0/iscsi/target", path)
			},
		},
		{
			name:    "empty name",
			config:  ISCSITargetConfig{},
			wantErr: true,
			errMsg:  "target name cannot be empty",
		},
		{
			name:    "API error",
			config:  ISCSITargetConfig{Name: "target1"},
			mockErr: &APIError{Message: "target exists"},
			wantErr: true,
			errMsg:  "failed to create iSCSI target",
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

			target, err := client.CreateISCSITarget(tt.config)
			if tt.wantErr {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.errMsg)
				assert.Nil(t, target)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, target)
			}
		})
	}
}

func TestDeleteISCSITarget(t *testing.T) {
	tests := []struct {
		name       string
		targetID   int
		mockErr    error
		wantErr    bool
		errMsg     string
		validateFn func(t *testing.T, method, path string)
	}{
		{
			name:     "successful deletion",
			targetID: 1,
			validateFn: func(t *testing.T, method, path string) {
				assert.Equal(t, "DELETE", method)
				assert.Equal(t, "/api/v2.0/iscsi/target/id/1", path)
			},
		},
		{
			name:     "API error",
			targetID: 1,
			mockErr:  &APIError{Message: "target not found"},
			wantErr:  true,
			errMsg:   "failed to delete iSCSI target",
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
						return []byte(`true`), nil
					},
				},
			}

			err := client.DeleteISCSITarget(tt.targetID)
			if tt.wantErr {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.errMsg)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestListISCSIExtents(t *testing.T) {
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
			mockResp: []ISCSIExtent{
				{ID: 1, Name: "extent1", Type: "DISK", Enabled: true},
			},
			wantLen: 1,
		},
		{
			name:     "empty list",
			mockResp: []ISCSIExtent{},
			wantLen:  0,
		},
		{
			name:    "API error",
			mockErr: &APIError{Message: "unauthorized"},
			wantErr: true,
			errMsg:  "failed to list iSCSI extents",
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
						assert.Equal(t, "/api/v2.0/iscsi/extent", path)
						if tt.mockErr != nil {
							return nil, tt.mockErr
						}
						return json.Marshal(tt.mockResp)
					},
				},
			}

			extents, err := client.ListISCSIExtents()
			if tt.wantErr {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.errMsg)
				assert.Nil(t, extents)
			} else {
				assert.NoError(t, err)
				assert.Len(t, extents, tt.wantLen)
			}
		})
	}
}

func TestCreateISCSIExtent(t *testing.T) {
	tests := []struct {
		name       string
		config     ISCSIExtentConfig
		mockResp   interface{}
		mockErr    error
		wantErr    bool
		errMsg     string
		validateFn func(t *testing.T, method, path string, body interface{})
	}{
		{
			name: "successful creation - disk",
			config: ISCSIExtentConfig{
				Name: "extent1",
				Type: "DISK",
				Disk: "zvol/tank/iscsi/vol1",
			},
			mockResp: ISCSIExtent{
				ID:      1,
				Name:    "extent1",
				Type:    "DISK",
				Disk:    "zvol/tank/iscsi/vol1",
				Enabled: true,
			},
			validateFn: func(t *testing.T, method, path string, body interface{}) {
				assert.Equal(t, "POST", method)
				assert.Equal(t, "/api/v2.0/iscsi/extent", path)
			},
		},
		{
			name:    "empty name",
			config:  ISCSIExtentConfig{Type: "DISK"},
			wantErr: true,
			errMsg:  "extent name cannot be empty",
		},
		{
			name:    "API error",
			config:  ISCSIExtentConfig{Name: "extent1", Type: "DISK"},
			mockErr: &APIError{Message: "extent exists"},
			wantErr: true,
			errMsg:  "failed to create iSCSI extent",
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

			extent, err := client.CreateISCSIExtent(tt.config)
			if tt.wantErr {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.errMsg)
				assert.Nil(t, extent)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, extent)
			}
		})
	}
}

func TestDeleteISCSIExtent(t *testing.T) {
	tests := []struct {
		name       string
		extentID   int
		removeFile bool
		mockErr    error
		wantErr    bool
		errMsg     string
		validateFn func(t *testing.T, method, path string, body interface{})
	}{
		{
			name:       "successful deletion without file removal",
			extentID:   1,
			removeFile: false,
			validateFn: func(t *testing.T, method, path string, body interface{}) {
				assert.Equal(t, "DELETE", method)
				assert.Equal(t, "/api/v2.0/iscsi/extent/id/1", path)
				bodyMap, ok := body.(map[string]bool)
				require.True(t, ok)
				assert.False(t, bodyMap["remove"])
			},
		},
		{
			name:       "successful deletion with file removal",
			extentID:   1,
			removeFile: true,
			validateFn: func(t *testing.T, method, path string, body interface{}) {
				bodyMap, ok := body.(map[string]bool)
				require.True(t, ok)
				assert.True(t, bodyMap["remove"])
			},
		},
		{
			name:     "API error",
			extentID: 1,
			mockErr:  &APIError{Message: "extent not found"},
			wantErr:  true,
			errMsg:   "failed to delete iSCSI extent",
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
						return []byte(`true`), nil
					},
				},
			}

			err := client.DeleteISCSIExtent(tt.extentID, tt.removeFile)
			if tt.wantErr {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.errMsg)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestListISCSITargetExtents(t *testing.T) {
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
			mockResp: []ISCSITargetExtent{
				{ID: 1, Target: 1, Extent: 1, LUNId: 0},
			},
			wantLen: 1,
		},
		{
			name:     "empty list",
			mockResp: []ISCSITargetExtent{},
			wantLen:  0,
		},
		{
			name:    "API error",
			mockErr: &APIError{Message: "unauthorized"},
			wantErr: true,
			errMsg:  "failed to list iSCSI target extents",
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
						assert.Equal(t, "/api/v2.0/iscsi/targetextent", path)
						if tt.mockErr != nil {
							return nil, tt.mockErr
						}
						return json.Marshal(tt.mockResp)
					},
				},
			}

			mappings, err := client.ListISCSITargetExtents()
			if tt.wantErr {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.errMsg)
				assert.Nil(t, mappings)
			} else {
				assert.NoError(t, err)
				assert.Len(t, mappings, tt.wantLen)
			}
		})
	}
}

func TestCreateISCSITargetExtent(t *testing.T) {
	tests := []struct {
		name       string
		config     ISCSITargetExtentConfig
		mockResp   interface{}
		mockErr    error
		wantErr    bool
		errMsg     string
		validateFn func(t *testing.T, method, path string, body interface{})
	}{
		{
			name: "successful creation",
			config: ISCSITargetExtentConfig{
				Target: 1,
				Extent: 1,
				LUNId:  0,
			},
			mockResp: ISCSITargetExtent{
				ID:     1,
				Target: 1,
				Extent: 1,
				LUNId:  0,
			},
			validateFn: func(t *testing.T, method, path string, body interface{}) {
				assert.Equal(t, "POST", method)
				assert.Equal(t, "/api/v2.0/iscsi/targetextent", path)
			},
		},
		{
			name:    "API error",
			config:  ISCSITargetExtentConfig{Target: 1, Extent: 1},
			mockErr: &APIError{Message: "mapping exists"},
			wantErr: true,
			errMsg:  "failed to create iSCSI target extent",
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

			mapping, err := client.CreateISCSITargetExtent(tt.config)
			if tt.wantErr {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.errMsg)
				assert.Nil(t, mapping)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, mapping)
			}
		})
	}
}

func TestDeleteISCSITargetExtent(t *testing.T) {
	tests := []struct {
		name       string
		mappingID  int
		mockErr    error
		wantErr    bool
		errMsg     string
		validateFn func(t *testing.T, method, path string)
	}{
		{
			name:      "successful deletion",
			mappingID: 1,
			validateFn: func(t *testing.T, method, path string) {
				assert.Equal(t, "DELETE", method)
				assert.Equal(t, "/api/v2.0/iscsi/targetextent/id/1", path)
			},
		},
		{
			name:      "API error",
			mappingID: 1,
			mockErr:   &APIError{Message: "mapping not found"},
			wantErr:   true,
			errMsg:    "failed to delete iSCSI target extent",
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
						return []byte(`true`), nil
					},
				},
			}

			err := client.DeleteISCSITargetExtent(tt.mappingID)
			if tt.wantErr {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.errMsg)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestGetISCSIGlobalConfig(t *testing.T) {
	tests := []struct {
		name       string
		mockResp   interface{}
		mockErr    error
		wantErr    bool
		errMsg     string
		validateFn func(t *testing.T, method, path string)
	}{
		{
			name: "successful get",
			mockResp: ISCSIGlobalConfig{
				Basename:    "iqn.2024-01.com.example",
				ISNSServers: []string{},
			},
			validateFn: func(t *testing.T, method, path string) {
				assert.Equal(t, "GET", method)
				assert.Equal(t, "/api/v2.0/iscsi/global", path)
			},
		},
		{
			name:    "API error",
			mockErr: &APIError{Message: "unauthorized"},
			wantErr: true,
			errMsg:  "failed to get iSCSI global config",
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

			config, err := client.GetISCSIGlobalConfig()
			if tt.wantErr {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.errMsg)
				assert.Nil(t, config)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, config)
			}
		})
	}
}

// JSON error tests for new methods
func TestGetSystemInfo_JSONError(t *testing.T) {
	client := &Client{
		baseURL: "https://truenas.example.com",
		apiKey:  "test-key",
		httpClient: &mockHTTPClient{
			DoFunc: func(method, path string, body interface{}) ([]byte, error) {
				return []byte(`{invalid json}`), nil
			},
		},
	}

	info, err := client.GetSystemInfo()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to parse response")
	assert.Nil(t, info)
}

func TestListDisks_JSONError(t *testing.T) {
	client := &Client{
		baseURL: "https://truenas.example.com",
		apiKey:  "test-key",
		httpClient: &mockHTTPClient{
			DoFunc: func(method, path string, body interface{}) ([]byte, error) {
				return []byte(`{invalid json}`), nil
			},
		},
	}

	disks, err := client.ListDisks()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to parse response")
	assert.Nil(t, disks)
}

func TestCreatePool_JSONError(t *testing.T) {
	client := &Client{
		baseURL: "https://truenas.example.com",
		apiKey:  "test-key",
		httpClient: &mockHTTPClient{
			DoFunc: func(method, path string, body interface{}) ([]byte, error) {
				return []byte(`{invalid json}`), nil
			},
		},
	}

	pool, err := client.CreatePool(PoolCreateConfig{
		Name: "tank",
		Topology: PoolCreateTopology{
			Data: []PoolCreateVDev{{Type: "STRIPE", Disks: []string{"sda"}}},
		},
	})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to parse response")
	assert.Nil(t, pool)
}

func TestListServices_JSONError(t *testing.T) {
	client := &Client{
		baseURL: "https://truenas.example.com",
		apiKey:  "test-key",
		httpClient: &mockHTTPClient{
			DoFunc: func(method, path string, body interface{}) ([]byte, error) {
				return []byte(`{invalid json}`), nil
			},
		},
	}

	services, err := client.ListServices()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to parse response")
	assert.Nil(t, services)
}

func TestListISCSIPortals_JSONError(t *testing.T) {
	client := &Client{
		baseURL: "https://truenas.example.com",
		apiKey:  "test-key",
		httpClient: &mockHTTPClient{
			DoFunc: func(method, path string, body interface{}) ([]byte, error) {
				return []byte(`{invalid json}`), nil
			},
		},
	}

	portals, err := client.ListISCSIPortals()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to parse response")
	assert.Nil(t, portals)
}

func TestCreateISCSIPortal_JSONError(t *testing.T) {
	client := &Client{
		baseURL: "https://truenas.example.com",
		apiKey:  "test-key",
		httpClient: &mockHTTPClient{
			DoFunc: func(method, path string, body interface{}) ([]byte, error) {
				return []byte(`{invalid json}`), nil
			},
		},
	}

	portal, err := client.CreateISCSIPortal(ISCSIPortalConfig{
		Listen: []ISCSIPortalListen{{IP: "0.0.0.0", Port: 3260}},
	})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to parse response")
	assert.Nil(t, portal)
}

func TestCreateISCSIInitiator_JSONError(t *testing.T) {
	client := &Client{
		baseURL: "https://truenas.example.com",
		apiKey:  "test-key",
		httpClient: &mockHTTPClient{
			DoFunc: func(method, path string, body interface{}) ([]byte, error) {
				return []byte(`{invalid json}`), nil
			},
		},
	}

	initiator, err := client.CreateISCSIInitiator(ISCSIInitiatorConfig{Comment: "Test"})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to parse response")
	assert.Nil(t, initiator)
}

func TestCreateISCSITarget_JSONError(t *testing.T) {
	client := &Client{
		baseURL: "https://truenas.example.com",
		apiKey:  "test-key",
		httpClient: &mockHTTPClient{
			DoFunc: func(method, path string, body interface{}) ([]byte, error) {
				return []byte(`{invalid json}`), nil
			},
		},
	}

	target, err := client.CreateISCSITarget(ISCSITargetConfig{Name: "target1"})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to parse response")
	assert.Nil(t, target)
}

func TestCreateISCSIExtent_JSONError(t *testing.T) {
	client := &Client{
		baseURL: "https://truenas.example.com",
		apiKey:  "test-key",
		httpClient: &mockHTTPClient{
			DoFunc: func(method, path string, body interface{}) ([]byte, error) {
				return []byte(`{invalid json}`), nil
			},
		},
	}

	extent, err := client.CreateISCSIExtent(ISCSIExtentConfig{Name: "extent1", Type: "DISK"})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to parse response")
	assert.Nil(t, extent)
}

func TestCreateISCSITargetExtent_JSONError(t *testing.T) {
	client := &Client{
		baseURL: "https://truenas.example.com",
		apiKey:  "test-key",
		httpClient: &mockHTTPClient{
			DoFunc: func(method, path string, body interface{}) ([]byte, error) {
				return []byte(`{invalid json}`), nil
			},
		},
	}

	mapping, err := client.CreateISCSITargetExtent(ISCSITargetExtentConfig{Target: 1, Extent: 1})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to parse response")
	assert.Nil(t, mapping)
}

func TestGetISCSIGlobalConfig_JSONError(t *testing.T) {
	client := &Client{
		baseURL: "https://truenas.example.com",
		apiKey:  "test-key",
		httpClient: &mockHTTPClient{
			DoFunc: func(method, path string, body interface{}) ([]byte, error) {
				return []byte(`{invalid json}`), nil
			},
		},
	}

	config, err := client.GetISCSIGlobalConfig()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to parse response")
	assert.Nil(t, config)
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
