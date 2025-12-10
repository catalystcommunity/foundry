package certmanager

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/catalystcommunity/foundry/v1/internal/component"
	"github.com/catalystcommunity/foundry/v1/internal/helm"
	"github.com/catalystcommunity/foundry/v1/internal/k8s"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

// Mock HelmClient
type MockHelmClient struct {
	mock.Mock
}

func (m *MockHelmClient) AddRepo(ctx context.Context, opts helm.RepoAddOptions) error {
	args := m.Called(ctx, opts)
	return args.Error(0)
}

func (m *MockHelmClient) Install(ctx context.Context, opts helm.InstallOptions) error {
	args := m.Called(ctx, opts)
	return args.Error(0)
}

func (m *MockHelmClient) Upgrade(ctx context.Context, opts helm.UpgradeOptions) error {
	args := m.Called(ctx, opts)
	return args.Error(0)
}

func (m *MockHelmClient) List(ctx context.Context, namespace string) ([]helm.Release, error) {
	args := m.Called(ctx, namespace)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]helm.Release), args.Error(1)
}

func (m *MockHelmClient) Uninstall(ctx context.Context, opts helm.UninstallOptions) error {
	args := m.Called(ctx, opts)
	return args.Error(0)
}

// Mock K8sClient
type MockK8sClient struct {
	mock.Mock
}

func (m *MockK8sClient) GetPods(ctx context.Context, namespace string) ([]*k8s.Pod, error) {
	args := m.Called(ctx, namespace)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*k8s.Pod), args.Error(1)
}

func (m *MockK8sClient) GetNamespace(ctx context.Context, name string) (*k8s.Namespace, error) {
	args := m.Called(ctx, name)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*k8s.Namespace), args.Error(1)
}

func (m *MockK8sClient) CreateNamespace(ctx context.Context, name string) error {
	args := m.Called(ctx, name)
	return args.Error(0)
}

func (m *MockK8sClient) ApplyManifest(ctx context.Context, manifest string) error {
	args := m.Called(ctx, manifest)
	return args.Error(0)
}

func TestInstall_Success(t *testing.T) {
	ctx := context.Background()
	cfg := &Config{
		Namespace:   "cert-manager",
		Version:     "v1.14.2",
		InstallCRDs: true,
	}

	mockHelm := new(MockHelmClient)
	mockK8s := new(MockK8sClient)

	// Expect namespace check
	mockK8s.On("GetNamespace", ctx, "cert-manager").Return(&k8s.Namespace{Name: "cert-manager"}, nil)

	// Expect repo add
	mockHelm.On("AddRepo", ctx, helm.RepoAddOptions{
		Name: DefaultRepoName,
		URL:  DefaultRepoURL,
	}).Return(nil)

	// Expect list to check for existing release (none found)
	mockHelm.On("List", ctx, "cert-manager").Return([]helm.Release{}, nil)

	// Expect install
	mockHelm.On("Install", ctx, mock.MatchedBy(func(opts helm.InstallOptions) bool {
		return opts.ReleaseName == DefaultReleaseName &&
			opts.Chart == DefaultChartName &&
			opts.Namespace == "cert-manager" &&
			opts.Version == "v1.14.2" &&
			opts.Wait == true
	})).Return(nil)

	// Expect pod check (immediate ready)
	mockK8s.On("GetPods", ctx, "cert-manager").Return([]*k8s.Pod{
		{Name: "cert-manager-1", Ready: true},
		{Name: "cert-manager-webhook-1", Ready: true},
		{Name: "cert-manager-cainjector-1", Ready: true},
	}, nil).Once()

	componentCfg := component.ComponentConfig{
		"helm_client": mockHelm,
		"k8s_client":  mockK8s,
	}

	err := Install(ctx, cfg, componentCfg)
	assert.NoError(t, err)

	mockHelm.AssertExpectations(t)
	mockK8s.AssertExpectations(t)
}

func TestInstall_NamespaceCreated(t *testing.T) {
	ctx := context.Background()
	cfg := &Config{
		Namespace:   "custom-ns",
		Version:     "v1.14.2",
		InstallCRDs: true,
	}

	mockHelm := new(MockHelmClient)
	mockK8s := new(MockK8sClient)

	// Namespace doesn't exist
	mockK8s.On("GetNamespace", ctx, "custom-ns").Return(nil, fmt.Errorf("not found"))
	mockK8s.On("CreateNamespace", ctx, "custom-ns").Return(nil)

	mockHelm.On("AddRepo", ctx, mock.Anything).Return(nil)
	mockHelm.On("List", ctx, "custom-ns").Return([]helm.Release{}, nil)
	mockHelm.On("Install", ctx, mock.Anything).Return(nil)
	mockK8s.On("GetPods", ctx, "custom-ns").Return([]*k8s.Pod{
		{Name: "cert-manager-1", Ready: true},
	}, nil)

	componentCfg := component.ComponentConfig{
		"helm_client": mockHelm,
		"k8s_client":  mockK8s,
	}

	err := Install(ctx, cfg, componentCfg)
	assert.NoError(t, err)

	mockK8s.AssertCalled(t, "CreateNamespace", ctx, "custom-ns")
}

func TestInstall_WithDefaultIssuer_SelfSigned(t *testing.T) {
	ctx := context.Background()
	cfg := &Config{
		Namespace:           "cert-manager",
		Version:             "v1.14.2",
		InstallCRDs:         true,
		CreateDefaultIssuer: true,
		DefaultIssuerType:   "self-signed",
	}

	mockHelm := new(MockHelmClient)
	mockK8s := new(MockK8sClient)

	mockK8s.On("GetNamespace", ctx, "cert-manager").Return(&k8s.Namespace{}, nil)
	mockHelm.On("AddRepo", ctx, mock.Anything).Return(nil)
	mockHelm.On("List", ctx, "cert-manager").Return([]helm.Release{}, nil)
	mockHelm.On("Install", ctx, mock.Anything).Return(nil)
	mockK8s.On("GetPods", ctx, "cert-manager").Return([]*k8s.Pod{
		{Name: "cert-manager-1", Ready: true},
	}, nil)

	// Expect manifest application for self-signed issuer
	mockK8s.On("ApplyManifest", ctx, mock.MatchedBy(func(manifest string) bool {
		return assert.Contains(t, manifest, "ClusterIssuer") &&
			assert.Contains(t, manifest, "selfsigned-issuer") &&
			assert.Contains(t, manifest, "selfSigned: {}")
	})).Return(nil)

	componentCfg := component.ComponentConfig{
		"helm_client": mockHelm,
		"k8s_client":  mockK8s,
	}

	err := Install(ctx, cfg, componentCfg)
	assert.NoError(t, err)

	mockK8s.AssertCalled(t, "ApplyManifest", ctx, mock.Anything)
}

func TestInstall_WithDefaultIssuer_ACME(t *testing.T) {
	ctx := context.Background()
	cfg := &Config{
		Namespace:           "cert-manager",
		Version:             "v1.14.2",
		InstallCRDs:         true,
		CreateDefaultIssuer: true,
		DefaultIssuerType:   "acme",
		ACMEEmail:           "test@example.com",
		ACMEServer:          "https://acme-staging.api.letsencrypt.org/directory",
	}

	mockHelm := new(MockHelmClient)
	mockK8s := new(MockK8sClient)

	mockK8s.On("GetNamespace", ctx, "cert-manager").Return(&k8s.Namespace{}, nil)
	mockHelm.On("AddRepo", ctx, mock.Anything).Return(nil)
	mockHelm.On("List", ctx, "cert-manager").Return([]helm.Release{}, nil)
	mockHelm.On("Install", ctx, mock.Anything).Return(nil)
	mockK8s.On("GetPods", ctx, "cert-manager").Return([]*k8s.Pod{
		{Name: "cert-manager-1", Ready: true},
	}, nil)

	mockK8s.On("ApplyManifest", ctx, mock.MatchedBy(func(manifest string) bool {
		return assert.Contains(t, manifest, "ClusterIssuer") &&
			assert.Contains(t, manifest, "letsencrypt-prod") &&
			assert.Contains(t, manifest, "test@example.com") &&
			assert.Contains(t, manifest, "https://acme-staging.api.letsencrypt.org/directory") &&
			assert.Contains(t, manifest, "http01")
	})).Return(nil)

	componentCfg := component.ComponentConfig{
		"helm_client": mockHelm,
		"k8s_client":  mockK8s,
	}

	err := Install(ctx, cfg, componentCfg)
	assert.NoError(t, err)
}

func TestInstall_WithDefaultIssuer_ACME_NoEmail(t *testing.T) {
	ctx := context.Background()
	cfg := &Config{
		Namespace:           "cert-manager",
		Version:             "v1.14.2",
		InstallCRDs:         true,
		CreateDefaultIssuer: true,
		DefaultIssuerType:   "acme",
		// Missing ACMEEmail
	}

	mockHelm := new(MockHelmClient)
	mockK8s := new(MockK8sClient)

	mockK8s.On("GetNamespace", ctx, "cert-manager").Return(&k8s.Namespace{}, nil)
	mockHelm.On("AddRepo", ctx, mock.Anything).Return(nil)
	mockHelm.On("List", ctx, "cert-manager").Return([]helm.Release{}, nil)
	mockHelm.On("Install", ctx, mock.Anything).Return(nil)
	mockK8s.On("GetPods", ctx, "cert-manager").Return([]*k8s.Pod{
		{Name: "cert-manager-1", Ready: true},
	}, nil)

	componentCfg := component.ComponentConfig{
		"helm_client": mockHelm,
		"k8s_client":  mockK8s,
	}

	err := Install(ctx, cfg, componentCfg)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "acme_email is required")
}

func TestInstall_MissingHelmClient(t *testing.T) {
	ctx := context.Background()
	cfg := &Config{Namespace: "cert-manager"}
	componentCfg := component.ComponentConfig{}

	err := Install(ctx, cfg, componentCfg)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "helm_client not provided")
}

func TestInstall_MissingK8sClient(t *testing.T) {
	ctx := context.Background()
	cfg := &Config{Namespace: "cert-manager"}
	mockHelm := new(MockHelmClient)
	componentCfg := component.ComponentConfig{
		"helm_client": mockHelm,
	}

	err := Install(ctx, cfg, componentCfg)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "k8s_client not provided")
}

func TestInstall_RepoAddFails(t *testing.T) {
	ctx := context.Background()
	cfg := &Config{Namespace: "cert-manager"}

	mockHelm := new(MockHelmClient)
	mockK8s := new(MockK8sClient)

	mockK8s.On("GetNamespace", ctx, "cert-manager").Return(&k8s.Namespace{}, nil)
	mockHelm.On("AddRepo", ctx, mock.Anything).Return(fmt.Errorf("repo add failed"))

	componentCfg := component.ComponentConfig{
		"helm_client": mockHelm,
		"k8s_client":  mockK8s,
	}

	err := Install(ctx, cfg, componentCfg)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to add Jetstack Helm repo")
}

func TestInstall_HelmInstallFails(t *testing.T) {
	ctx := context.Background()
	cfg := &Config{Namespace: "cert-manager"}

	mockHelm := new(MockHelmClient)
	mockK8s := new(MockK8sClient)

	mockK8s.On("GetNamespace", ctx, "cert-manager").Return(&k8s.Namespace{}, nil)
	mockHelm.On("AddRepo", ctx, mock.Anything).Return(nil)
	mockHelm.On("List", ctx, "cert-manager").Return([]helm.Release{}, nil)
	mockHelm.On("Install", ctx, mock.Anything).Return(fmt.Errorf("install failed"))

	componentCfg := component.ComponentConfig{
		"helm_client": mockHelm,
		"k8s_client":  mockK8s,
	}

	err := Install(ctx, cfg, componentCfg)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to install cert-manager")
}

func TestWaitForCertManager_Timeout(t *testing.T) {
	// Create a context with a very short timeout for testing
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	mockK8s := new(MockK8sClient)

	// Return no pods to simulate timeout
	mockK8s.On("GetPods", ctx, "cert-manager").Return([]*k8s.Pod{}, nil).Maybe()

	// This should timeout quickly due to the context deadline
	err := waitForCertManager(ctx, mockK8s, "cert-manager")
	assert.Error(t, err)
}

func TestWaitForCertManager_EventuallyReady(t *testing.T) {
	ctx := context.Background()
	mockK8s := new(MockK8sClient)

	// First call: not ready
	mockK8s.On("GetPods", ctx, "cert-manager").Return([]*k8s.Pod{
		{Name: "cert-manager-1", Ready: false},
	}, nil).Once()

	// Second call: ready
	mockK8s.On("GetPods", ctx, "cert-manager").Return([]*k8s.Pod{
		{Name: "cert-manager-1", Ready: true},
	}, nil).Once()

	err := waitForCertManager(ctx, mockK8s, "cert-manager")
	assert.NoError(t, err)
}

func TestEnsureNamespace_Exists(t *testing.T) {
	ctx := context.Background()
	mockK8s := new(MockK8sClient)

	mockK8s.On("GetNamespace", ctx, "test-ns").Return(&k8s.Namespace{Name: "test-ns"}, nil)

	err := ensureNamespace(ctx, mockK8s, "test-ns")
	assert.NoError(t, err)
	mockK8s.AssertNotCalled(t, "CreateNamespace", mock.Anything, mock.Anything)
}

func TestEnsureNamespace_Created(t *testing.T) {
	ctx := context.Background()
	mockK8s := new(MockK8sClient)

	mockK8s.On("GetNamespace", ctx, "test-ns").Return(nil, fmt.Errorf("not found"))
	mockK8s.On("CreateNamespace", ctx, "test-ns").Return(nil)

	err := ensureNamespace(ctx, mockK8s, "test-ns")
	assert.NoError(t, err)
	mockK8s.AssertCalled(t, "CreateNamespace", ctx, "test-ns")
}

func TestGenerateSelfSignedIssuer(t *testing.T) {
	manifest := generateSelfSignedIssuer()
	assert.Contains(t, manifest, "kind: ClusterIssuer")
	assert.Contains(t, manifest, "name: selfsigned-issuer")
	assert.Contains(t, manifest, "selfSigned: {}")
}

func TestGenerateACMEIssuer(t *testing.T) {
	manifest := generateACMEIssuer("test@example.com", "https://acme.example.com/directory")
	assert.Contains(t, manifest, "kind: ClusterIssuer")
	assert.Contains(t, manifest, "name: letsencrypt-prod")
	assert.Contains(t, manifest, "test@example.com")
	assert.Contains(t, manifest, "https://acme.example.com/directory")
	assert.Contains(t, manifest, "http01")
	assert.Contains(t, manifest, "class: contour")
}

func TestGetStatus(t *testing.T) {
	ctx := context.Background()
	cfg := &Config{
		Namespace: "cert-manager",
		Version:   "v1.14.2",
	}

	status, err := GetStatus(ctx, cfg)
	require.NoError(t, err)
	require.NotNil(t, status)
	assert.True(t, status.Installed)
	assert.Equal(t, "v1.14.2", status.Version)
	assert.True(t, status.Healthy)
}

func TestUninstall(t *testing.T) {
	ctx := context.Background()
	cfg := &Config{Namespace: "cert-manager"}

	err := Uninstall(ctx, cfg)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not yet implemented")
}
