package truenas

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDefaultSetupConfig(t *testing.T) {
	cfg := DefaultSetupConfig()

	assert.Equal(t, "tank", cfg.PoolName)
	assert.Equal(t, "k8s", cfg.DatasetName)
	assert.True(t, cfg.EnableNFS)
	assert.True(t, cfg.EnableISCSI)
	assert.Equal(t, "0.0.0.0", cfg.ISCSIPortalIP)
	assert.Equal(t, 3260, cfg.ISCSIPortalPort)
	assert.Equal(t, "MIRROR", cfg.VDevType)
	assert.Equal(t, 2, cfg.MinDisksForMirror)
}

func TestValidateConnection(t *testing.T) {
	tests := []struct {
		name     string
		mockResp interface{}
		mockErr  error
		wantErr  bool
		errMsg   string
	}{
		{
			name: "successful connection",
			mockResp: SystemInfo{
				Version:  "TrueNAS-SCALE-22.12.0",
				Hostname: "truenas.local",
			},
		},
		{
			name:    "connection failed",
			mockErr: &APIError{Message: "connection refused"},
			wantErr: true,
			errMsg:  "failed to connect to TrueNAS",
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

			validator := NewValidator(client)
			info, err := validator.ValidateConnection()

			if tt.wantErr {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.errMsg)
				assert.Nil(t, info)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, info)
			}
		})
	}
}

func TestEnsurePool_ExistingPool(t *testing.T) {
	existingPool := Pool{
		ID:      1,
		Name:    "tank",
		Status:  "ONLINE",
		Healthy: true,
	}

	client := &Client{
		baseURL: "https://truenas.example.com",
		apiKey:  "test-key",
		httpClient: &mockHTTPClient{
			DoFunc: func(method, path string, body interface{}) ([]byte, error) {
				if path == "/api/v2.0/pool" {
					return json.Marshal([]Pool{existingPool})
				}
				return nil, nil
			},
		},
	}

	validator := NewValidator(client)
	cfg := &SetupConfig{PoolName: "tank"}

	pool, created, err := validator.ensurePool(cfg)

	assert.NoError(t, err)
	assert.NotNil(t, pool)
	assert.False(t, created)
	assert.Equal(t, "tank", pool.Name)
}

func TestEnsurePool_CreateNew(t *testing.T) {
	unusedDisks := []Disk{
		{Name: "sda", Serial: "ABC123", Pool: ""},
		{Name: "sdb", Serial: "DEF456", Pool: ""},
	}
	newPool := Pool{
		ID:      1,
		Name:    "tank",
		Status:  "ONLINE",
		Healthy: true,
	}

	callCount := 0
	client := &Client{
		baseURL: "https://truenas.example.com",
		apiKey:  "test-key",
		httpClient: &mockHTTPClient{
			DoFunc: func(method, path string, body interface{}) ([]byte, error) {
				callCount++
				if path == "/api/v2.0/pool" && method == "GET" {
					return json.Marshal([]Pool{}) // No existing pools
				}
				if path == "/api/v2.0/disk" && method == "GET" {
					return json.Marshal(unusedDisks)
				}
				if path == "/api/v2.0/pool" && method == "POST" {
					return json.Marshal(newPool)
				}
				return nil, nil
			},
		},
	}

	validator := NewValidator(client)
	cfg := &SetupConfig{
		PoolName:          "tank",
		VDevType:          "MIRROR",
		MinDisksForMirror: 2,
	}

	pool, created, err := validator.ensurePool(cfg)

	assert.NoError(t, err)
	assert.NotNil(t, pool)
	assert.True(t, created)
	assert.Equal(t, "tank", pool.Name)
}

func TestEnsurePool_NoDisks(t *testing.T) {
	client := &Client{
		baseURL: "https://truenas.example.com",
		apiKey:  "test-key",
		httpClient: &mockHTTPClient{
			DoFunc: func(method, path string, body interface{}) ([]byte, error) {
				if path == "/api/v2.0/pool" && method == "GET" {
					return json.Marshal([]Pool{}) // No existing pools
				}
				if path == "/api/v2.0/disk" && method == "GET" {
					return json.Marshal([]Disk{}) // No disks
				}
				return nil, nil
			},
		},
	}

	validator := NewValidator(client)
	cfg := DefaultSetupConfig()

	pool, created, err := validator.ensurePool(cfg)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "no unused disks available")
	assert.Nil(t, pool)
	assert.False(t, created)
}

func TestEnsureDataset_Existing(t *testing.T) {
	existingDataset := Dataset{
		ID:         "tank/k8s",
		Name:       "k8s",
		Pool:       "tank",
		Type:       "FILESYSTEM",
		Mountpoint: "/mnt/tank/k8s",
	}

	client := &Client{
		baseURL: "https://truenas.example.com",
		apiKey:  "test-key",
		httpClient: &mockHTTPClient{
			DoFunc: func(method, path string, body interface{}) ([]byte, error) {
				return json.Marshal(existingDataset)
			},
		},
	}

	validator := NewValidator(client)
	dataset, created, err := validator.ensureDataset("tank", "k8s")

	assert.NoError(t, err)
	assert.NotNil(t, dataset)
	assert.False(t, created)
	assert.Equal(t, "tank/k8s", dataset.ID)
}

func TestEnsureDataset_CreateNew(t *testing.T) {
	newDataset := Dataset{
		ID:         "tank/k8s",
		Name:       "k8s",
		Pool:       "tank",
		Type:       "FILESYSTEM",
		Mountpoint: "/mnt/tank/k8s",
	}

	callCount := 0
	client := &Client{
		baseURL: "https://truenas.example.com",
		apiKey:  "test-key",
		httpClient: &mockHTTPClient{
			DoFunc: func(method, path string, body interface{}) ([]byte, error) {
				callCount++
				if callCount == 1 {
					// First call: GET returns not found
					return nil, &APIError{Message: "dataset not found"}
				}
				// Second call: POST creates dataset
				return json.Marshal(newDataset)
			},
		},
	}

	validator := NewValidator(client)
	dataset, created, err := validator.ensureDataset("tank", "k8s")

	assert.NoError(t, err)
	assert.NotNil(t, dataset)
	assert.True(t, created)
	assert.Equal(t, "tank/k8s", dataset.ID)
}

func TestValidator_EnsureServiceRunning(t *testing.T) {
	tests := []struct {
		name           string
		serviceName    string
		initialState   string
		initialEnabled bool
		wantStarted    bool
	}{
		{
			name:           "already running",
			serviceName:    "nfs",
			initialState:   "RUNNING",
			initialEnabled: true,
			wantStarted:    false,
		},
		{
			name:           "needs start",
			serviceName:    "nfs",
			initialState:   "STOPPED",
			initialEnabled: true,
			wantStarted:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			service := Service{
				ID:      1,
				Service: tt.serviceName,
				State:   tt.initialState,
				Enable:  tt.initialEnabled,
			}

			client := &Client{
				baseURL: "https://truenas.example.com",
				apiKey:  "test-key",
				httpClient: &mockHTTPClient{
					DoFunc: func(method, path string, body interface{}) ([]byte, error) {
						if method == "GET" && path == "/api/v2.0/service" {
							return json.Marshal([]Service{service})
						}
						return []byte(`true`), nil
					},
				},
			}

			validator := NewValidator(client)
			started, err := validator.ensureServiceRunning(tt.serviceName)

			assert.NoError(t, err)
			assert.Equal(t, tt.wantStarted, started)
		})
	}
}

func TestEnsureISCSIPortal_Existing(t *testing.T) {
	existingPortal := ISCSIPortal{
		ID:     1,
		Tag:    1,
		Listen: []ISCSIPortalListen{{IP: "0.0.0.0", Port: 3260}},
	}

	client := &Client{
		baseURL: "https://truenas.example.com",
		apiKey:  "test-key",
		httpClient: &mockHTTPClient{
			DoFunc: func(method, path string, body interface{}) ([]byte, error) {
				return json.Marshal([]ISCSIPortal{existingPortal})
			},
		},
	}

	validator := NewValidator(client)
	portal, err := validator.ensureISCSIPortal("0.0.0.0", 3260)

	assert.NoError(t, err)
	assert.NotNil(t, portal)
	assert.Equal(t, 1, portal.ID)
}

func TestEnsureISCSIPortal_CreateNew(t *testing.T) {
	newPortal := ISCSIPortal{
		ID:     1,
		Tag:    1,
		Listen: []ISCSIPortalListen{{IP: "0.0.0.0", Port: 3260}},
	}

	callCount := 0
	client := &Client{
		baseURL: "https://truenas.example.com",
		apiKey:  "test-key",
		httpClient: &mockHTTPClient{
			DoFunc: func(method, path string, body interface{}) ([]byte, error) {
				callCount++
				if callCount == 1 {
					return json.Marshal([]ISCSIPortal{}) // No existing portals
				}
				return json.Marshal(newPortal)
			},
		},
	}

	validator := NewValidator(client)
	portal, err := validator.ensureISCSIPortal("0.0.0.0", 3260)

	assert.NoError(t, err)
	assert.NotNil(t, portal)
}

func TestEnsureISCSIInitiator_Existing(t *testing.T) {
	existingInitiator := ISCSIInitiator{
		ID:         1,
		Tag:        1,
		Initiators: []string{}, // Allow all
		Comment:    "Allow all",
	}

	client := &Client{
		baseURL: "https://truenas.example.com",
		apiKey:  "test-key",
		httpClient: &mockHTTPClient{
			DoFunc: func(method, path string, body interface{}) ([]byte, error) {
				return json.Marshal([]ISCSIInitiator{existingInitiator})
			},
		},
	}

	validator := NewValidator(client)
	initiator, err := validator.ensureISCSIInitiator()

	assert.NoError(t, err)
	assert.NotNil(t, initiator)
	assert.Equal(t, 1, initiator.ID)
}

func TestEnsureISCSIInitiator_CreateNew(t *testing.T) {
	newInitiator := ISCSIInitiator{
		ID:         1,
		Tag:        1,
		Initiators: []string{},
		Comment:    "Foundry CSI - allow all initiators",
	}

	callCount := 0
	client := &Client{
		baseURL: "https://truenas.example.com",
		apiKey:  "test-key",
		httpClient: &mockHTTPClient{
			DoFunc: func(method, path string, body interface{}) ([]byte, error) {
				callCount++
				if callCount == 1 {
					// Existing initiator has specific IQNs (not allow all)
					return json.Marshal([]ISCSIInitiator{
						{ID: 1, Initiators: []string{"iqn.example.com:specific"}},
					})
				}
				return json.Marshal(newInitiator)
			},
		},
	}

	validator := NewValidator(client)
	initiator, err := validator.ensureISCSIInitiator()

	assert.NoError(t, err)
	assert.NotNil(t, initiator)
}

func TestValidateRequirements_Success(t *testing.T) {
	client := &Client{
		baseURL: "https://truenas.example.com",
		apiKey:  "test-key",
		httpClient: &mockHTTPClient{
			DoFunc: func(method, path string, body interface{}) ([]byte, error) {
				switch path {
				case "/api/v2.0/system/info":
					return json.Marshal(SystemInfo{Version: "22.12.0"})
				case "/api/v2.0/pool":
					return json.Marshal([]Pool{{Name: "tank", Status: "ONLINE"}})
				case "/api/v2.0/pool/dataset/id/tank/k8s":
					return json.Marshal(Dataset{ID: "tank/k8s"})
				case "/api/v2.0/service":
					return json.Marshal([]Service{
						{Service: "nfs", State: "RUNNING"},
						{Service: "iscsitarget", State: "RUNNING"},
					})
				case "/api/v2.0/iscsi/portal":
					return json.Marshal([]ISCSIPortal{{ID: 1}})
				case "/api/v2.0/iscsi/initiator":
					return json.Marshal([]ISCSIInitiator{{ID: 1}})
				}
				return []byte(`{}`), nil
			},
		},
	}

	validator := NewValidator(client)
	err := validator.ValidateRequirements(DefaultSetupConfig())

	assert.NoError(t, err)
}

func TestValidateRequirements_NoPools(t *testing.T) {
	client := &Client{
		baseURL: "https://truenas.example.com",
		apiKey:  "test-key",
		httpClient: &mockHTTPClient{
			DoFunc: func(method, path string, body interface{}) ([]byte, error) {
				switch path {
				case "/api/v2.0/system/info":
					return json.Marshal(SystemInfo{Version: "22.12.0"})
				case "/api/v2.0/pool":
					return json.Marshal([]Pool{})
				}
				return []byte(`{}`), nil
			},
		},
	}

	validator := NewValidator(client)
	err := validator.ValidateRequirements(DefaultSetupConfig())

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "no storage pools found")
}

func TestValidateRequirements_NFSNotRunning(t *testing.T) {
	client := &Client{
		baseURL: "https://truenas.example.com",
		apiKey:  "test-key",
		httpClient: &mockHTTPClient{
			DoFunc: func(method, path string, body interface{}) ([]byte, error) {
				switch path {
				case "/api/v2.0/system/info":
					return json.Marshal(SystemInfo{Version: "22.12.0"})
				case "/api/v2.0/pool":
					return json.Marshal([]Pool{{Name: "tank", Status: "ONLINE"}})
				case "/api/v2.0/pool/dataset/id/tank/k8s":
					return json.Marshal(Dataset{ID: "tank/k8s"})
				case "/api/v2.0/service":
					return json.Marshal([]Service{
						{Service: "nfs", State: "STOPPED"},
						{Service: "iscsitarget", State: "RUNNING"},
					})
				}
				return []byte(`{}`), nil
			},
		},
	}

	validator := NewValidator(client)
	err := validator.ValidateRequirements(DefaultSetupConfig())

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "NFS service is not running")
}

func TestGetCSIConfig(t *testing.T) {
	client := &Client{
		baseURL: "https://truenas.example.com",
		apiKey:  "test-api-key",
	}

	result := &SetupResult{
		Pool: &Pool{
			ID:   1,
			Name: "tank",
		},
		Dataset: &Dataset{
			ID: "tank/k8s",
		},
		ISCSIPortal: &ISCSIPortal{
			ID:     1,
			Tag:    1,
			Listen: []ISCSIPortalListen{{IP: "0.0.0.0", Port: 3260}},
		},
		ISCSIInitiator: &ISCSIInitiator{
			ID:  1,
			Tag: 1,
		},
	}

	cfg := DefaultSetupConfig()
	validator := NewValidator(client)

	csiConfig := validator.GetCSIConfig(result, cfg, "https://192.168.1.100")

	assert.Equal(t, "https://192.168.1.100", csiConfig.HTTPURL)
	assert.Equal(t, "test-api-key", csiConfig.APIKey)
	assert.Equal(t, "tank", csiConfig.PoolName)
	assert.Equal(t, "tank/k8s", csiConfig.DatasetParent)
	assert.Equal(t, "192.168.1.100", csiConfig.NFSShareHost)
	assert.Equal(t, "192.168.1.100:3260", csiConfig.ISCSIPortal)
	assert.Equal(t, 1, csiConfig.ISCSITargetPortalGroup)
	assert.Equal(t, 1, csiConfig.ISCSIInitiatorGroup)
}

func TestExtractHostFromURL(t *testing.T) {
	tests := []struct {
		url      string
		expected string
	}{
		{"https://192.168.1.100", "192.168.1.100"},
		{"http://192.168.1.100", "192.168.1.100"},
		{"https://192.168.1.100:443", "192.168.1.100"},
		{"http://192.168.1.100:80", "192.168.1.100"},
		{"https://truenas.local", "truenas.local"},
		{"https://truenas.local/api", "truenas.local"},
		{"https://truenas.local:8080/api", "truenas.local"},
	}

	for _, tt := range tests {
		t.Run(tt.url, func(t *testing.T) {
			result := extractHostFromURL(tt.url)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestCreatePool_VDevTypeAdjustment(t *testing.T) {
	tests := []struct {
		name         string
		diskCount    int
		vdevType     string
		expectedType string
	}{
		{"single disk becomes STRIPE", 1, "MIRROR", "STRIPE"},
		{"two disks stay MIRROR", 2, "MIRROR", "MIRROR"},
		{"two disks RAIDZ1 becomes MIRROR", 2, "RAIDZ1", "MIRROR"},
		{"three disks RAIDZ1 stays", 3, "RAIDZ1", "RAIDZ1"},
		{"three disks RAIDZ2 becomes RAIDZ1", 3, "RAIDZ2", "RAIDZ1"},
		{"four disks RAIDZ2 stays", 4, "RAIDZ2", "RAIDZ2"},
		{"four disks RAIDZ3 becomes RAIDZ2", 4, "RAIDZ3", "RAIDZ2"},
		{"five disks RAIDZ3 stays", 5, "RAIDZ3", "RAIDZ3"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create disk list
			disks := make([]Disk, tt.diskCount)
			for i := 0; i < tt.diskCount; i++ {
				disks[i] = Disk{Name: string(rune('a' + i))}
			}

			var capturedConfig PoolCreateConfig
			client := &Client{
				baseURL: "https://truenas.example.com",
				apiKey:  "test-key",
				httpClient: &mockHTTPClient{
					DoFunc: func(method, path string, body interface{}) ([]byte, error) {
						if path == "/api/v2.0/disk" && method == "GET" {
							return json.Marshal(disks)
						}
						if path == "/api/v2.0/pool" && method == "POST" {
							// Capture the config
							data, _ := json.Marshal(body)
							json.Unmarshal(data, &capturedConfig)
							return json.Marshal(Pool{ID: 1, Name: "tank"})
						}
						return nil, nil
					},
				},
			}

			validator := NewValidator(client)
			cfg := &SetupConfig{
				PoolName:          "tank",
				VDevType:          tt.vdevType,
				MinDisksForMirror: 2,
			}

			_, err := validator.createPool(cfg)

			require.NoError(t, err)
			require.Len(t, capturedConfig.Topology.Data, 1)
			assert.Equal(t, tt.expectedType, capturedConfig.Topology.Data[0].Type)
		})
	}
}
