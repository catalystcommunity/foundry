package openbaoinjector

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/catalystcommunity/foundry/v1/internal/helm"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type mockHelmClient struct {
	addRepoCalls   []helm.RepoAddOptions
	installCalls   []helm.InstallOptions
	upgradeCalls   []helm.UpgradeOptions
	uninstallCalls []helm.UninstallOptions
	listResponse   []helm.Release
	listErr        error
	addRepoErr     error
	installErr     error
	upgradeErr     error
	uninstallErr   error
}

func (m *mockHelmClient) AddRepo(ctx context.Context, opts helm.RepoAddOptions) error {
	m.addRepoCalls = append(m.addRepoCalls, opts)
	if m.addRepoErr != nil {
		return m.addRepoErr
	}
	return nil
}

func (m *mockHelmClient) Install(ctx context.Context, opts helm.InstallOptions) error {
	m.installCalls = append(m.installCalls, opts)
	if m.installErr != nil {
		return m.installErr
	}
	return nil
}

func (m *mockHelmClient) Upgrade(ctx context.Context, opts helm.UpgradeOptions) error {
	m.upgradeCalls = append(m.upgradeCalls, opts)
	if m.upgradeErr != nil {
		return m.upgradeErr
	}
	return nil
}

func (m *mockHelmClient) Uninstall(ctx context.Context, opts helm.UninstallOptions) error {
	m.uninstallCalls = append(m.uninstallCalls, opts)
	if m.uninstallErr != nil {
		return m.uninstallErr
	}
	return nil
}

func (m *mockHelmClient) List(ctx context.Context, namespace string) ([]helm.Release, error) {
	return m.listResponse, m.listErr
}

func TestInstall_Success(t *testing.T) {
	mock := &mockHelmClient{}
	cfg := &Config{
		Version:           "0.26.2",
		Namespace:         "openbao",
		ExternalVaultAddr: "http://10.0.0.1:8200",
	}

	err := Install(context.Background(), mock, nil, nil, cfg, false)

	require.NoError(t, err)
	assert.Len(t, mock.addRepoCalls, 1)
	assert.Equal(t, "openbao", mock.addRepoCalls[0].Name)
	assert.Equal(t, "https://openbao.github.io/openbao-helm", mock.addRepoCalls[0].URL)
	assert.Len(t, mock.installCalls, 1)
	assert.Equal(t, "openbao-injector", mock.installCalls[0].ReleaseName)
}

func TestInstall_NilHelmClient(t *testing.T) {
	cfg := &Config{
		Version:           "0.26.2",
		Namespace:         "openbao",
		ExternalVaultAddr: "http://10.0.0.1:8200",
	}

	err := Install(context.Background(), nil, nil, nil, cfg, false)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "helm client cannot be nil")
}

func TestInstall_NilConfig(t *testing.T) {
	mock := &mockHelmClient{}

	err := Install(context.Background(), mock, nil, nil, nil, false)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "config cannot be nil")
}

func TestInstall_AddRepoFailure(t *testing.T) {
	mock := &mockHelmClient{
		addRepoErr: errors.New("failed to add repo"),
	}
	cfg := &Config{
		Version:           "0.26.2",
		Namespace:         "openbao",
		ExternalVaultAddr: "http://10.0.0.1:8200",
	}

	err := Install(context.Background(), mock, nil, nil, cfg, false)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to add openbao helm repo")
}

func TestInstall_UpgradeExisting(t *testing.T) {
	mock := &mockHelmClient{
		listResponse: []helm.Release{
			{
				Name:       "openbao-injector",
				Status:     "deployed",
				AppVersion: "0.26.2",
			},
		},
	}
	cfg := &Config{
		Version:           "0.26.3",
		Namespace:         "openbao",
		ExternalVaultAddr: "http://10.0.0.1:8200",
	}

	err := Install(context.Background(), mock, nil, nil, cfg, false)

	require.NoError(t, err)
	assert.Len(t, mock.upgradeCalls, 1)
	assert.Equal(t, "openbao-injector", mock.upgradeCalls[0].ReleaseName)
	assert.Equal(t, "0.26.3", mock.upgradeCalls[0].Version)
	assert.Len(t, mock.installCalls, 0)
}

func TestInstall_ReplaceFailed(t *testing.T) {
	mock := &mockHelmClient{
		listResponse: []helm.Release{
			{
				Name:       "openbao-injector",
				Status:     "failed",
				AppVersion: "0.26.2",
			},
		},
	}
	cfg := &Config{
		Version:           "0.26.3",
		Namespace:         "openbao",
		ExternalVaultAddr: "http://10.0.0.1:8200",
	}

	err := Install(context.Background(), mock, nil, nil, cfg, false)

	require.NoError(t, err)
	assert.Len(t, mock.uninstallCalls, 1)
	assert.Len(t, mock.installCalls, 1)
}

func TestInstall_ReplaceFailedUninstallError(t *testing.T) {
	mock := &mockHelmClient{
		listResponse: []helm.Release{
			{
				Name:       "openbao-injector",
				Status:     "failed",
				AppVersion: "0.26.2",
			},
		},
		uninstallErr: errors.New("failed to uninstall"),
	}
	cfg := &Config{
		Version:           "0.26.3",
		Namespace:         "openbao",
		ExternalVaultAddr: "http://10.0.0.1:8200",
	}

	err := Install(context.Background(), mock, nil, nil, cfg, false)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to remove existing release")
}

func TestInstall_InstallFailure(t *testing.T) {
	mock := &mockHelmClient{
		installErr: errors.New("failed to install"),
	}
	cfg := &Config{
		Version:           "0.26.2",
		Namespace:         "openbao",
		ExternalVaultAddr: "http://10.0.0.1:8200",
	}

	err := Install(context.Background(), mock, nil, nil, cfg, false)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to install openbao injector")
}

func TestBuildHelmValues(t *testing.T) {
	cfg := &Config{
		Version:           "0.26.2",
		Namespace:         "openbao",
		ExternalVaultAddr: "http://10.0.0.1:8200",
	}

	values := buildHelmValues(cfg)

	serverConfig, ok := values["server"].(map[string]interface{})
	require.True(t, ok, "server should be a map")
	assert.Equal(t, false, serverConfig["enabled"])

	injectorConfig, ok := values["injector"].(map[string]interface{})
	require.True(t, ok, "injector should be a map")
	assert.Equal(t, true, injectorConfig["enabled"])
	assert.Equal(t, "http://10.0.0.1:8200", injectorConfig["externalVaultAddr"])
}

func TestComponent_Install(t *testing.T) {
	tests := []struct {
		name        string
		cfg         map[string]interface{}
		setupMock   func(*mockHelmClient)
		wantErr     bool
		errContains string
	}{
		{
			name: "successful install",
			cfg: map[string]interface{}{
				"external_vault_addr": "http://10.0.0.1:8200",
			},
			setupMock: func(m *mockHelmClient) {
				m.listResponse = []helm.Release{}
			},
			wantErr: false,
		},
		{
			name:        "missing external_vault_addr",
			cfg:         map[string]interface{}{},
			setupMock:   func(m *mockHelmClient) {},
			wantErr:     true,
			errContains: "external_vault_addr is required",
		},
		{
			name: "helm client error",
			cfg: map[string]interface{}{
				"external_vault_addr": "http://10.0.0.1:8200",
			},
			setupMock: func(m *mockHelmClient) {
				m.addRepoErr = fmt.Errorf("repo error")
			},
			wantErr:     true,
			errContains: "failed to add openbao helm repo",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mock := &mockHelmClient{}
			tt.setupMock(mock)

			comp := NewComponent(mock, nil, nil)
			err := comp.Install(context.Background(), tt.cfg)

			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errContains)
			} else {
				require.NoError(t, err)
			}
		})
	}
}
