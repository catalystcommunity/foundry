package grafana

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

func TestStoreGrafanaCredentials_NewCredentials(t *testing.T) {
	ctx := context.Background()
	client := newMockOpenBAOClient()

	err := StoreGrafanaCredentials(ctx, client, "admin", "test-password")

	assert.NoError(t, err)
	require.Len(t, client.writeCalls, 1)
	assert.Equal(t, "foundry-core", client.writeCalls[0].mount)
	assert.Equal(t, "grafana", client.writeCalls[0].path)
	assert.Equal(t, "test-password", client.writeCalls[0].data["admin_password"])
	assert.Equal(t, "admin", client.writeCalls[0].data["admin_user"])
}

func TestStoreGrafanaCredentials_ExistingMatchingCredentials(t *testing.T) {
	ctx := context.Background()
	client := newMockOpenBAOClient()
	client.secrets["foundry-core/grafana"] = map[string]interface{}{
		"admin_password": "existing-password",
		"admin_user":     "admin",
	}

	err := StoreGrafanaCredentials(ctx, client, "admin", "existing-password")

	assert.NoError(t, err)
	// Should not write if credentials already match
	assert.Len(t, client.writeCalls, 0)
}

func TestStoreGrafanaCredentials_ExistingDifferentCredentials(t *testing.T) {
	ctx := context.Background()
	client := newMockOpenBAOClient()
	client.secrets["foundry-core/grafana"] = map[string]interface{}{
		"admin_password": "old-password",
		"admin_user":     "admin",
	}

	err := StoreGrafanaCredentials(ctx, client, "admin", "new-password")

	assert.NoError(t, err)
	require.Len(t, client.writeCalls, 1)
	assert.Equal(t, "new-password", client.writeCalls[0].data["admin_password"])
}

func TestStoreGrafanaCredentials_NilClient(t *testing.T) {
	ctx := context.Background()

	err := StoreGrafanaCredentials(ctx, nil, "admin", "password")

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "OpenBAO client is required")
}

func TestStoreGrafanaCredentials_EmptyPassword(t *testing.T) {
	ctx := context.Background()
	client := newMockOpenBAOClient()

	err := StoreGrafanaCredentials(ctx, client, "admin", "")

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "password cannot be empty")
}

func TestStoreGrafanaCredentials_WriteError(t *testing.T) {
	ctx := context.Background()
	client := newMockOpenBAOClient()
	client.writeErr = errors.New("write failed")

	err := StoreGrafanaCredentials(ctx, client, "admin", "password")

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "write failed")
}

func TestGetGrafanaCredentials_Success(t *testing.T) {
	ctx := context.Background()
	client := newMockOpenBAOClient()
	client.secrets["foundry-core/grafana"] = map[string]interface{}{
		"admin_password": "test-password",
		"admin_user":     "customuser",
	}

	username, password, err := GetGrafanaCredentials(ctx, client)

	assert.NoError(t, err)
	assert.Equal(t, "test-password", password)
	assert.Equal(t, "customuser", username)
}

func TestGetGrafanaCredentials_DefaultUsername(t *testing.T) {
	ctx := context.Background()
	client := newMockOpenBAOClient()
	client.secrets["foundry-core/grafana"] = map[string]interface{}{
		"admin_password": "test-password",
	}

	username, password, err := GetGrafanaCredentials(ctx, client)

	assert.NoError(t, err)
	assert.Equal(t, "test-password", password)
	assert.Equal(t, "admin", username)
}

func TestGetGrafanaCredentials_NotFound(t *testing.T) {
	ctx := context.Background()
	client := newMockOpenBAOClient()

	_, _, err := GetGrafanaCredentials(ctx, client)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

func TestGetGrafanaCredentials_EmptyPassword(t *testing.T) {
	ctx := context.Background()
	client := newMockOpenBAOClient()
	client.secrets["foundry-core/grafana"] = map[string]interface{}{
		"admin_password": "",
		"admin_user":     "admin",
	}

	_, _, err := GetGrafanaCredentials(ctx, client)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not found or empty")
}

func TestGetGrafanaCredentials_NilClient(t *testing.T) {
	ctx := context.Background()

	_, _, err := GetGrafanaCredentials(ctx, nil)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "OpenBAO client is required")
}

func TestGetGrafanaCredentials_ReadError(t *testing.T) {
	ctx := context.Background()
	client := newMockOpenBAOClient()
	client.readErr = errors.New("read failed")

	_, _, err := GetGrafanaCredentials(ctx, client)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "read failed")
}

func TestGetGrafanaPassword_Success(t *testing.T) {
	ctx := context.Background()
	client := newMockOpenBAOClient()
	client.secrets["foundry-core/grafana"] = map[string]interface{}{
		"admin_password": "test-password",
		"admin_user":     "admin",
	}

	password, err := GetGrafanaPassword(ctx, client)

	assert.NoError(t, err)
	assert.Equal(t, "test-password", password)
}

func TestEnsureGrafanaCredentials_ExistingCredentials(t *testing.T) {
	ctx := context.Background()
	client := newMockOpenBAOClient()
	client.secrets["foundry-core/grafana"] = map[string]interface{}{
		"admin_password": "existing-password",
		"admin_user":     "existinguser",
	}

	username, password, err := EnsureGrafanaCredentials(ctx, client, "", "")

	assert.NoError(t, err)
	assert.Equal(t, "existing-password", password)
	assert.Equal(t, "existinguser", username)
	// Should not write since credentials exist
	assert.Len(t, client.writeCalls, 0)
}

func TestEnsureGrafanaCredentials_NoExistingWithProvided(t *testing.T) {
	ctx := context.Background()
	client := newMockOpenBAOClient()

	username, password, err := EnsureGrafanaCredentials(ctx, client, "customuser", "provided-password")

	assert.NoError(t, err)
	assert.Equal(t, "provided-password", password)
	assert.Equal(t, "customuser", username)
	require.Len(t, client.writeCalls, 1)
	assert.Equal(t, "provided-password", client.writeCalls[0].data["admin_password"])
	assert.Equal(t, "customuser", client.writeCalls[0].data["admin_user"])
}

func TestEnsureGrafanaCredentials_NoExistingGeneratesPassword(t *testing.T) {
	ctx := context.Background()
	client := newMockOpenBAOClient()

	username, password, err := EnsureGrafanaCredentials(ctx, client, "", "")

	assert.NoError(t, err)
	assert.NotEmpty(t, password)
	assert.Len(t, password, 32) // Generated password should be 32 chars
	assert.Equal(t, "admin", username)
	require.Len(t, client.writeCalls, 1)
}

func TestEnsureGrafanaCredentials_NilClient(t *testing.T) {
	ctx := context.Background()

	_, _, err := EnsureGrafanaCredentials(ctx, nil, "", "")

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "OpenBAO client is required")
}

func TestGenerateSecurePassword(t *testing.T) {
	password1, err := generateSecurePassword(32)
	assert.NoError(t, err)
	assert.Len(t, password1, 32)

	password2, err := generateSecurePassword(32)
	assert.NoError(t, err)
	assert.Len(t, password2, 32)

	// Passwords should be different
	assert.NotEqual(t, password1, password2)
}

func TestGenerateSecurePassword_DifferentLengths(t *testing.T) {
	tests := []int{8, 16, 24, 32, 64}

	for _, length := range tests {
		password, err := generateSecurePassword(length)
		assert.NoError(t, err)
		assert.Len(t, password, length, "password length should be %d", length)
	}
}
