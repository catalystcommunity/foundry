package externaldns

import (
	"context"
	"errors"
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

// PowerDNS Tests

func TestStorePowerDNSCredentials_NewCredentials(t *testing.T) {
	ctx := context.Background()
	client := newMockOpenBAOClient()

	err := StorePowerDNSCredentials(ctx, client, &PowerDNSSecrets{
		APIUrl:   "http://powerdns.local:8081",
		APIKey:   "test-api-key",
		ServerID: "localhost",
	})

	assert.NoError(t, err)
	require.Len(t, client.writeCalls, 1)
	assert.Equal(t, "foundry-core", client.writeCalls[0].mount)
	assert.Equal(t, "external-dns/pdns", client.writeCalls[0].path)
	assert.Equal(t, "http://powerdns.local:8081", client.writeCalls[0].data["api_url"])
	assert.Equal(t, "test-api-key", client.writeCalls[0].data["api_key"])
	assert.Equal(t, "localhost", client.writeCalls[0].data["server_id"])
}

func TestStorePowerDNSCredentials_ExistingMatching(t *testing.T) {
	ctx := context.Background()
	client := newMockOpenBAOClient()
	client.secrets["foundry-core/external-dns/pdns"] = map[string]interface{}{
		"api_url":   "http://powerdns.local:8081",
		"api_key":   "test-api-key",
		"server_id": "localhost",
	}

	err := StorePowerDNSCredentials(ctx, client, &PowerDNSSecrets{
		APIUrl:   "http://powerdns.local:8081",
		APIKey:   "test-api-key",
		ServerID: "localhost",
	})

	assert.NoError(t, err)
	assert.Len(t, client.writeCalls, 0) // Should not write
}

func TestStorePowerDNSCredentials_ExistingDifferent(t *testing.T) {
	ctx := context.Background()
	client := newMockOpenBAOClient()
	client.secrets["foundry-core/external-dns/pdns"] = map[string]interface{}{
		"api_url": "http://powerdns.local:8081",
		"api_key": "old-api-key",
	}

	err := StorePowerDNSCredentials(ctx, client, &PowerDNSSecrets{
		APIUrl: "http://powerdns.local:8081",
		APIKey: "new-api-key",
	})

	assert.NoError(t, err)
	require.Len(t, client.writeCalls, 1)
	assert.Equal(t, "new-api-key", client.writeCalls[0].data["api_key"])
}

func TestStorePowerDNSCredentials_NilClient(t *testing.T) {
	ctx := context.Background()

	err := StorePowerDNSCredentials(ctx, nil, &PowerDNSSecrets{
		APIUrl: "http://powerdns.local:8081",
		APIKey: "test-api-key",
	})

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "OpenBAO client is required")
}

func TestStorePowerDNSCredentials_NilSecrets(t *testing.T) {
	ctx := context.Background()
	client := newMockOpenBAOClient()

	err := StorePowerDNSCredentials(ctx, client, nil)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "secrets cannot be nil")
}

func TestStorePowerDNSCredentials_MissingAPIUrl(t *testing.T) {
	ctx := context.Background()
	client := newMockOpenBAOClient()

	err := StorePowerDNSCredentials(ctx, client, &PowerDNSSecrets{
		APIKey: "test-api-key",
	})

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "api_url is required")
}

func TestStorePowerDNSCredentials_MissingAPIKey(t *testing.T) {
	ctx := context.Background()
	client := newMockOpenBAOClient()

	err := StorePowerDNSCredentials(ctx, client, &PowerDNSSecrets{
		APIUrl: "http://powerdns.local:8081",
	})

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "api_key is required")
}

func TestGetPowerDNSCredentials_Success(t *testing.T) {
	ctx := context.Background()
	client := newMockOpenBAOClient()
	client.secrets["foundry-core/external-dns/pdns"] = map[string]interface{}{
		"api_url":   "http://powerdns.local:8081",
		"api_key":   "test-api-key",
		"server_id": "localhost",
	}

	secrets, err := GetPowerDNSCredentials(ctx, client)

	assert.NoError(t, err)
	require.NotNil(t, secrets)
	assert.Equal(t, "http://powerdns.local:8081", secrets.APIUrl)
	assert.Equal(t, "test-api-key", secrets.APIKey)
	assert.Equal(t, "localhost", secrets.ServerID)
}

func TestGetPowerDNSCredentials_NotFound(t *testing.T) {
	ctx := context.Background()
	client := newMockOpenBAOClient()

	_, err := GetPowerDNSCredentials(ctx, client)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

func TestGetPowerDNSCredentials_NilClient(t *testing.T) {
	ctx := context.Background()

	_, err := GetPowerDNSCredentials(ctx, nil)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "OpenBAO client is required")
}

// Cloudflare Tests

func TestStoreCloudflareCredentials_NewCredentials(t *testing.T) {
	ctx := context.Background()
	client := newMockOpenBAOClient()

	err := StoreCloudflareCredentials(ctx, client, &CloudflareSecrets{
		APIToken: "cf-api-token",
	})

	assert.NoError(t, err)
	require.Len(t, client.writeCalls, 1)
	assert.Equal(t, "foundry-core", client.writeCalls[0].mount)
	assert.Equal(t, "external-dns/cloudflare", client.writeCalls[0].path)
	assert.Equal(t, "cf-api-token", client.writeCalls[0].data["api_token"])
}

func TestStoreCloudflareCredentials_ExistingMatching(t *testing.T) {
	ctx := context.Background()
	client := newMockOpenBAOClient()
	client.secrets["foundry-core/external-dns/cloudflare"] = map[string]interface{}{
		"api_token": "cf-api-token",
	}

	err := StoreCloudflareCredentials(ctx, client, &CloudflareSecrets{
		APIToken: "cf-api-token",
	})

	assert.NoError(t, err)
	assert.Len(t, client.writeCalls, 0)
}

func TestStoreCloudflareCredentials_NilClient(t *testing.T) {
	ctx := context.Background()

	err := StoreCloudflareCredentials(ctx, nil, &CloudflareSecrets{
		APIToken: "cf-api-token",
	})

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "OpenBAO client is required")
}

func TestStoreCloudflareCredentials_MissingAPIToken(t *testing.T) {
	ctx := context.Background()
	client := newMockOpenBAOClient()

	err := StoreCloudflareCredentials(ctx, client, &CloudflareSecrets{})

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "api_token is required")
}

func TestGetCloudflareCredentials_Success(t *testing.T) {
	ctx := context.Background()
	client := newMockOpenBAOClient()
	client.secrets["foundry-core/external-dns/cloudflare"] = map[string]interface{}{
		"api_token": "cf-api-token",
	}

	secrets, err := GetCloudflareCredentials(ctx, client)

	assert.NoError(t, err)
	require.NotNil(t, secrets)
	assert.Equal(t, "cf-api-token", secrets.APIToken)
}

func TestGetCloudflareCredentials_NotFound(t *testing.T) {
	ctx := context.Background()
	client := newMockOpenBAOClient()

	_, err := GetCloudflareCredentials(ctx, client)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

// RFC2136 Tests

func TestStoreRFC2136Credentials_NewCredentials(t *testing.T) {
	ctx := context.Background()
	client := newMockOpenBAOClient()

	err := StoreRFC2136Credentials(ctx, client, &RFC2136Secrets{
		Host:          "ns.example.com",
		Port:          53,
		Zone:          "example.com.",
		TSIGKeyName:   "tsig-key",
		TSIGSecret:    "secret-value",
		TSIGSecretAlg: "hmac-sha256",
	})

	assert.NoError(t, err)
	require.Len(t, client.writeCalls, 1)
	assert.Equal(t, "foundry-core", client.writeCalls[0].mount)
	assert.Equal(t, "external-dns/rfc2136", client.writeCalls[0].path)
	assert.Equal(t, "ns.example.com", client.writeCalls[0].data["host"])
	assert.Equal(t, 53, client.writeCalls[0].data["port"])
	assert.Equal(t, "example.com.", client.writeCalls[0].data["zone"])
	assert.Equal(t, "tsig-key", client.writeCalls[0].data["tsig_key_name"])
	assert.Equal(t, "secret-value", client.writeCalls[0].data["tsig_secret"])
	assert.Equal(t, "hmac-sha256", client.writeCalls[0].data["tsig_secret_alg"])
}

func TestStoreRFC2136Credentials_MinimalCredentials(t *testing.T) {
	ctx := context.Background()
	client := newMockOpenBAOClient()

	err := StoreRFC2136Credentials(ctx, client, &RFC2136Secrets{
		Host: "ns.example.com",
	})

	assert.NoError(t, err)
	require.Len(t, client.writeCalls, 1)
	assert.Equal(t, "ns.example.com", client.writeCalls[0].data["host"])
	// Optional fields should not be present
	_, hasPort := client.writeCalls[0].data["port"]
	assert.False(t, hasPort)
}

func TestStoreRFC2136Credentials_NilClient(t *testing.T) {
	ctx := context.Background()

	err := StoreRFC2136Credentials(ctx, nil, &RFC2136Secrets{
		Host: "ns.example.com",
	})

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "OpenBAO client is required")
}

func TestStoreRFC2136Credentials_MissingHost(t *testing.T) {
	ctx := context.Background()
	client := newMockOpenBAOClient()

	err := StoreRFC2136Credentials(ctx, client, &RFC2136Secrets{})

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "host is required")
}

func TestGetRFC2136Credentials_Success(t *testing.T) {
	ctx := context.Background()
	client := newMockOpenBAOClient()
	client.secrets["foundry-core/external-dns/rfc2136"] = map[string]interface{}{
		"host":            "ns.example.com",
		"port":            float64(53),
		"zone":            "example.com.",
		"tsig_key_name":   "tsig-key",
		"tsig_secret":     "secret-value",
		"tsig_secret_alg": "hmac-sha256",
	}

	secrets, err := GetRFC2136Credentials(ctx, client)

	assert.NoError(t, err)
	require.NotNil(t, secrets)
	assert.Equal(t, "ns.example.com", secrets.Host)
	assert.Equal(t, 53, secrets.Port)
	assert.Equal(t, "example.com.", secrets.Zone)
	assert.Equal(t, "tsig-key", secrets.TSIGKeyName)
	assert.Equal(t, "secret-value", secrets.TSIGSecret)
	assert.Equal(t, "hmac-sha256", secrets.TSIGSecretAlg)
}

func TestGetRFC2136Credentials_NotFound(t *testing.T) {
	ctx := context.Background()
	client := newMockOpenBAOClient()

	_, err := GetRFC2136Credentials(ctx, client)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

// StoreProviderCredentials Tests

func TestStoreProviderCredentials_PowerDNS(t *testing.T) {
	ctx := context.Background()
	client := newMockOpenBAOClient()

	cfg := &Config{
		Provider: ProviderPowerDNS,
		PowerDNS: &PowerDNSConfig{
			APIUrl:   "http://powerdns.local:8081",
			APIKey:   "api-key",
			ServerID: "localhost",
		},
	}

	err := StoreProviderCredentials(ctx, client, cfg)

	assert.NoError(t, err)
	require.Len(t, client.writeCalls, 1)
	assert.Equal(t, "external-dns/pdns", client.writeCalls[0].path)
}

func TestStoreProviderCredentials_Cloudflare(t *testing.T) {
	ctx := context.Background()
	client := newMockOpenBAOClient()

	cfg := &Config{
		Provider: ProviderCloudflare,
		Cloudflare: &CloudflareConfig{
			APIToken: "cf-token",
		},
	}

	err := StoreProviderCredentials(ctx, client, cfg)

	assert.NoError(t, err)
	require.Len(t, client.writeCalls, 1)
	assert.Equal(t, "external-dns/cloudflare", client.writeCalls[0].path)
}

func TestStoreProviderCredentials_RFC2136(t *testing.T) {
	ctx := context.Background()
	client := newMockOpenBAOClient()

	cfg := &Config{
		Provider: ProviderRFC2136,
		RFC2136: &RFC2136Config{
			Host: "ns.example.com",
			Port: 53,
		},
	}

	err := StoreProviderCredentials(ctx, client, cfg)

	assert.NoError(t, err)
	require.Len(t, client.writeCalls, 1)
	assert.Equal(t, "external-dns/rfc2136", client.writeCalls[0].path)
}

func TestStoreProviderCredentials_None(t *testing.T) {
	ctx := context.Background()
	client := newMockOpenBAOClient()

	cfg := &Config{
		Provider: ProviderNone,
	}

	err := StoreProviderCredentials(ctx, client, cfg)

	assert.NoError(t, err)
	assert.Len(t, client.writeCalls, 0)
}

func TestStoreProviderCredentials_NilClient(t *testing.T) {
	ctx := context.Background()

	cfg := &Config{
		Provider: ProviderPowerDNS,
	}

	err := StoreProviderCredentials(ctx, nil, cfg)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "OpenBAO client is required")
}

func TestStoreProviderCredentials_NilConfig(t *testing.T) {
	ctx := context.Background()
	client := newMockOpenBAOClient()

	err := StoreProviderCredentials(ctx, client, nil)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "config is required")
}

func TestStoreProviderCredentials_MissingProviderConfig(t *testing.T) {
	ctx := context.Background()
	client := newMockOpenBAOClient()

	// PowerDNS provider without PowerDNS config
	cfg := &Config{
		Provider: ProviderPowerDNS,
	}

	err := StoreProviderCredentials(ctx, client, cfg)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "PowerDNS config is required")
}

// LoadProviderCredentials Tests

func TestLoadProviderCredentials_PowerDNS(t *testing.T) {
	ctx := context.Background()
	client := newMockOpenBAOClient()
	client.secrets["foundry-core/external-dns/pdns"] = map[string]interface{}{
		"api_url":   "http://powerdns.local:8081",
		"api_key":   "api-key",
		"server_id": "localhost",
	}

	cfg := &Config{
		Provider: ProviderPowerDNS,
	}

	err := LoadProviderCredentials(ctx, client, cfg)

	assert.NoError(t, err)
	require.NotNil(t, cfg.PowerDNS)
	assert.Equal(t, "http://powerdns.local:8081", cfg.PowerDNS.APIUrl)
	assert.Equal(t, "api-key", cfg.PowerDNS.APIKey)
	assert.Equal(t, "localhost", cfg.PowerDNS.ServerID)
}

func TestLoadProviderCredentials_Cloudflare(t *testing.T) {
	ctx := context.Background()
	client := newMockOpenBAOClient()
	client.secrets["foundry-core/external-dns/cloudflare"] = map[string]interface{}{
		"api_token": "cf-token",
	}

	cfg := &Config{
		Provider: ProviderCloudflare,
	}

	err := LoadProviderCredentials(ctx, client, cfg)

	assert.NoError(t, err)
	require.NotNil(t, cfg.Cloudflare)
	assert.Equal(t, "cf-token", cfg.Cloudflare.APIToken)
}

func TestLoadProviderCredentials_RFC2136(t *testing.T) {
	ctx := context.Background()
	client := newMockOpenBAOClient()
	client.secrets["foundry-core/external-dns/rfc2136"] = map[string]interface{}{
		"host":            "ns.example.com",
		"port":            float64(53),
		"zone":            "example.com.",
		"tsig_key_name":   "tsig-key",
		"tsig_secret":     "secret",
		"tsig_secret_alg": "hmac-sha256",
	}

	cfg := &Config{
		Provider: ProviderRFC2136,
	}

	err := LoadProviderCredentials(ctx, client, cfg)

	assert.NoError(t, err)
	require.NotNil(t, cfg.RFC2136)
	assert.Equal(t, "ns.example.com", cfg.RFC2136.Host)
	assert.Equal(t, 53, cfg.RFC2136.Port)
	assert.Equal(t, "example.com.", cfg.RFC2136.Zone)
	assert.Equal(t, "tsig-key", cfg.RFC2136.TSIGKeyName)
	assert.Equal(t, "secret", cfg.RFC2136.TSIGSecret)
	assert.Equal(t, "hmac-sha256", cfg.RFC2136.TSIGSecretAlg)
}

func TestLoadProviderCredentials_None(t *testing.T) {
	ctx := context.Background()
	client := newMockOpenBAOClient()

	cfg := &Config{
		Provider: ProviderNone,
	}

	err := LoadProviderCredentials(ctx, client, cfg)

	assert.NoError(t, err)
}

func TestLoadProviderCredentials_NotFound(t *testing.T) {
	ctx := context.Background()
	client := newMockOpenBAOClient()

	cfg := &Config{
		Provider: ProviderPowerDNS,
	}

	err := LoadProviderCredentials(ctx, client, cfg)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

func TestLoadProviderCredentials_WriteError(t *testing.T) {
	ctx := context.Background()
	client := newMockOpenBAOClient()
	client.readErr = errors.New("read failed")

	cfg := &Config{
		Provider: ProviderPowerDNS,
	}

	err := LoadProviderCredentials(ctx, client, cfg)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "read failed")
}
