package truenas

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockOpenBAOClient implements OpenBAOClient for testing
type mockOpenBAOClient struct {
	secrets    map[string]map[string]interface{}
	writeErr   error
	readErr    error
	writeCalls []mockWriteCall
}

type mockWriteCall struct {
	mount string
	path  string
	data  map[string]interface{}
}

func newMockOpenBAOClient() *mockOpenBAOClient {
	return &mockOpenBAOClient{
		secrets:    make(map[string]map[string]interface{}),
		writeCalls: make([]mockWriteCall, 0),
	}
}

func (m *mockOpenBAOClient) WriteSecretV2(ctx context.Context, mount, path string, data map[string]interface{}) error {
	m.writeCalls = append(m.writeCalls, mockWriteCall{mount: mount, path: path, data: data})
	if m.writeErr != nil {
		return m.writeErr
	}
	key := mount + "/" + path
	m.secrets[key] = data
	return nil
}

func (m *mockOpenBAOClient) ReadSecretV2(ctx context.Context, mount, path string) (map[string]interface{}, error) {
	if m.readErr != nil {
		return nil, m.readErr
	}
	key := mount + "/" + path
	data, ok := m.secrets[key]
	if !ok {
		return nil, nil
	}
	return data, nil
}

func TestValidateOrPromptConfig_AllPresent(t *testing.T) {
	cfg := &InstallConfig{
		APIURL:      "https://truenas.example.com",
		APIKey:      "test-api-key",
		Interactive: false,
	}

	err := validateOrPromptConfig(cfg)

	assert.NoError(t, err)
	assert.NotNil(t, cfg.SetupConfig) // Should have default setup config
}

func TestValidateOrPromptConfig_MissingAPIURL_NonInteractive(t *testing.T) {
	cfg := &InstallConfig{
		APIKey:      "test-api-key",
		Interactive: false,
	}

	err := validateOrPromptConfig(cfg)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "TrueNAS API URL is required")
}

func TestValidateOrPromptConfig_MissingAPIKey_NonInteractive(t *testing.T) {
	cfg := &InstallConfig{
		APIURL:      "https://truenas.example.com",
		Interactive: false,
	}

	err := validateOrPromptConfig(cfg)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "TrueNAS API key is required")
}

func TestStoreTrueNASAPIKey_NewKey(t *testing.T) {
	ctx := context.Background()
	client := newMockOpenBAOClient()

	err := storeTrueNASAPIKey(ctx, client, "new-api-key")

	assert.NoError(t, err)
	require.Len(t, client.writeCalls, 1)
	assert.Equal(t, "foundry-core", client.writeCalls[0].mount)
	assert.Equal(t, "truenas", client.writeCalls[0].path)
	assert.Equal(t, "new-api-key", client.writeCalls[0].data["api_key"])
}

func TestStoreTrueNASAPIKey_ExistingMatchingKey(t *testing.T) {
	ctx := context.Background()
	client := newMockOpenBAOClient()
	client.secrets["foundry-core/truenas"] = map[string]interface{}{
		"api_key": "existing-api-key",
	}

	err := storeTrueNASAPIKey(ctx, client, "existing-api-key")

	assert.NoError(t, err)
	// Should not write if key already matches
	assert.Len(t, client.writeCalls, 0)
}

func TestStoreTrueNASAPIKey_ExistingDifferentKey(t *testing.T) {
	ctx := context.Background()
	client := newMockOpenBAOClient()
	client.secrets["foundry-core/truenas"] = map[string]interface{}{
		"api_key": "old-api-key",
	}

	err := storeTrueNASAPIKey(ctx, client, "new-api-key")

	assert.NoError(t, err)
	require.Len(t, client.writeCalls, 1)
	assert.Equal(t, "new-api-key", client.writeCalls[0].data["api_key"])
}

func TestEnsureTrueNASAPIKey_ExistingKey(t *testing.T) {
	ctx := context.Background()
	client := newMockOpenBAOClient()
	client.secrets["foundry-core/truenas"] = map[string]interface{}{
		"api_key": "existing-key",
	}

	key, err := EnsureTrueNASAPIKey(ctx, client, "")

	assert.NoError(t, err)
	assert.Equal(t, "existing-key", key)
	// Should not write since key exists
	assert.Len(t, client.writeCalls, 0)
}

func TestEnsureTrueNASAPIKey_NoExistingKey_WithProvided(t *testing.T) {
	ctx := context.Background()
	client := newMockOpenBAOClient()

	key, err := EnsureTrueNASAPIKey(ctx, client, "provided-key")

	assert.NoError(t, err)
	assert.Equal(t, "provided-key", key)
	require.Len(t, client.writeCalls, 1)
	assert.Equal(t, "provided-key", client.writeCalls[0].data["api_key"])
}

func TestEnsureTrueNASAPIKey_NoExistingKey_NoneProvided(t *testing.T) {
	ctx := context.Background()
	client := newMockOpenBAOClient()

	_, err := EnsureTrueNASAPIKey(ctx, client, "")

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "no TrueNAS API key found")
}

func TestGetTrueNASAPIKey_Success(t *testing.T) {
	ctx := context.Background()
	client := newMockOpenBAOClient()
	client.secrets["foundry-core/truenas"] = map[string]interface{}{
		"api_key": "test-api-key",
	}

	key, err := GetTrueNASAPIKey(ctx, client)

	assert.NoError(t, err)
	assert.Equal(t, "test-api-key", key)
}

func TestGetTrueNASAPIKey_NotFound(t *testing.T) {
	ctx := context.Background()
	client := newMockOpenBAOClient()

	_, err := GetTrueNASAPIKey(ctx, client)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

func TestGetTrueNASAPIKey_EmptyKey(t *testing.T) {
	ctx := context.Background()
	client := newMockOpenBAOClient()
	client.secrets["foundry-core/truenas"] = map[string]interface{}{
		"api_key": "",
	}

	_, err := GetTrueNASAPIKey(ctx, client)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "empty or invalid")
}

func TestPrepareInstall_NilConfig(t *testing.T) {
	ctx := context.Background()

	_, err := PrepareInstall(ctx, nil, nil)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "install config is required")
}

func TestPrepareInstall_SuccessWithSetup(t *testing.T) {
	// Create mock TrueNAS responses
	systemInfo := SystemInfo{Version: "22.12.0", Hostname: "truenas.local"}
	existingPool := Pool{ID: 1, Name: "tank", Status: "ONLINE", Healthy: true}
	existingDataset := Dataset{ID: "tank/k8s", Name: "k8s", Pool: "tank"}
	nfsService := Service{Service: "nfs", State: "RUNNING", Enable: true}
	iscsiService := Service{Service: "iscsitarget", State: "RUNNING", Enable: true}
	portal := ISCSIPortal{ID: 1, Tag: 1, Listen: []ISCSIPortalListen{{IP: "0.0.0.0", Port: 3260}}}
	initiator := ISCSIInitiator{ID: 1, Tag: 1, Initiators: []string{}}

	client := &Client{
		baseURL: "https://truenas.example.com",
		apiKey:  "test-api-key",
		httpClient: &mockHTTPClient{
			DoFunc: func(method, path string, body interface{}) ([]byte, error) {
				switch path {
				case "/api/v2.0/system/info":
					return json.Marshal(systemInfo)
				case "/api/v2.0/pool":
					return json.Marshal([]Pool{existingPool})
				case "/api/v2.0/pool/dataset/id/tank/k8s":
					return json.Marshal(existingDataset)
				case "/api/v2.0/service":
					return json.Marshal([]Service{nfsService, iscsiService})
				case "/api/v2.0/iscsi/portal":
					return json.Marshal([]ISCSIPortal{portal})
				case "/api/v2.0/iscsi/initiator":
					return json.Marshal([]ISCSIInitiator{initiator})
				}
				return []byte(`{}`), nil
			},
		},
	}

	// We can't use PrepareInstall directly because it creates its own client
	// Instead, test the individual components
	validator := NewValidator(client)

	// Test setup
	setupResult, err := validator.Setup(DefaultSetupConfig())
	require.NoError(t, err)
	assert.NotNil(t, setupResult)
	assert.Equal(t, "tank", setupResult.Pool.Name)

	// Test CSI config generation
	csiConfig := validator.GetCSIConfig(setupResult, DefaultSetupConfig(), "https://truenas.example.com")
	assert.Equal(t, "https://truenas.example.com", csiConfig.HTTPURL)
	assert.Equal(t, "tank", csiConfig.PoolName)
	assert.Equal(t, "tank/k8s", csiConfig.DatasetParent)
}

func TestPrepareInstall_SkipSetup(t *testing.T) {
	openBAOClient := newMockOpenBAOClient()

	// Create mock TrueNAS responses for validation
	systemInfo := SystemInfo{Version: "22.12.0", Hostname: "truenas.local"}
	existingPool := Pool{ID: 1, Name: "tank", Status: "ONLINE", Healthy: true}
	existingDataset := Dataset{ID: "tank/k8s", Name: "k8s", Pool: "tank"}
	nfsService := Service{Service: "nfs", State: "RUNNING", Enable: true}
	iscsiService := Service{Service: "iscsitarget", State: "RUNNING", Enable: true}
	portal := ISCSIPortal{ID: 1, Tag: 1, Listen: []ISCSIPortalListen{{IP: "0.0.0.0", Port: 3260}}}
	initiator := ISCSIInitiator{ID: 1, Tag: 1, Initiators: []string{}}

	// Create a mock HTTP client factory for testing
	mockHTTP := &mockHTTPClient{
		DoFunc: func(method, path string, body interface{}) ([]byte, error) {
			switch path {
			case "/api/v2.0/system/info":
				return json.Marshal(systemInfo)
			case "/api/v2.0/pool":
				return json.Marshal([]Pool{existingPool})
			case "/api/v2.0/pool/dataset/id/tank/k8s":
				return json.Marshal(existingDataset)
			case "/api/v2.0/service":
				return json.Marshal([]Service{nfsService, iscsiService})
			case "/api/v2.0/iscsi/portal":
				return json.Marshal([]ISCSIPortal{portal})
			case "/api/v2.0/iscsi/initiator":
				return json.Marshal([]ISCSIInitiator{initiator})
			}
			return []byte(`{}`), nil
		},
	}

	// Test validate requirements directly (since PrepareInstall creates its own client)
	client := &Client{
		baseURL:    "https://truenas.example.com",
		apiKey:     "test-api-key",
		httpClient: mockHTTP,
	}

	validator := NewValidator(client)
	err := validator.ValidateRequirements(DefaultSetupConfig())
	require.NoError(t, err)

	// Test API key storage
	ctx := context.Background()
	err = storeTrueNASAPIKey(ctx, openBAOClient, "test-api-key")
	require.NoError(t, err)
	require.Len(t, openBAOClient.writeCalls, 1)
	assert.Equal(t, "foundry-core", openBAOClient.writeCalls[0].mount)
	assert.Equal(t, "truenas", openBAOClient.writeCalls[0].path)
}

func TestInstallConfig_Defaults(t *testing.T) {
	cfg := &InstallConfig{
		APIURL: "https://truenas.example.com",
		APIKey: "test-key",
	}

	err := validateOrPromptConfig(cfg)

	assert.NoError(t, err)
	assert.NotNil(t, cfg.SetupConfig)
	assert.Equal(t, "tank", cfg.SetupConfig.PoolName)
	assert.Equal(t, "k8s", cfg.SetupConfig.DatasetName)
	assert.True(t, cfg.SetupConfig.EnableNFS)
	assert.True(t, cfg.SetupConfig.EnableISCSI)
}

func TestInstallConfig_CustomSetupConfig(t *testing.T) {
	cfg := &InstallConfig{
		APIURL: "https://truenas.example.com",
		APIKey: "test-key",
		SetupConfig: &SetupConfig{
			PoolName:    "storage",
			DatasetName: "kubernetes",
			EnableNFS:   true,
			EnableISCSI: false,
		},
	}

	err := validateOrPromptConfig(cfg)

	assert.NoError(t, err)
	assert.Equal(t, "storage", cfg.SetupConfig.PoolName)
	assert.Equal(t, "kubernetes", cfg.SetupConfig.DatasetName)
	assert.True(t, cfg.SetupConfig.EnableNFS)
	assert.False(t, cfg.SetupConfig.EnableISCSI)
}
