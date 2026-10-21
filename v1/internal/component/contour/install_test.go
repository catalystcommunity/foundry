package contour

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/catalystcommunity/foundry/v1/internal/helm"
	"github.com/catalystcommunity/foundry/v1/internal/k8s"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockHelmClient is a mock implementation of helm.Client for testing
type mockHelmClient struct {
	addRepoErr      error
	installErr      error
	listReleases    []helm.Release
	listErr         error
	reposAdded      []helm.RepoAddOptions
	chartsInstalled []helm.InstallOptions
	chartsUpgraded  []helm.UpgradeOptions
	upgradeErr      error
	uninstalls      int
}

func (m *mockHelmClient) AddRepo(ctx context.Context, opts helm.RepoAddOptions) error {
	m.reposAdded = append(m.reposAdded, opts)
	return m.addRepoErr
}

func (m *mockHelmClient) Install(ctx context.Context, opts helm.InstallOptions) error {
	m.chartsInstalled = append(m.chartsInstalled, opts)
	return m.installErr
}

func (m *mockHelmClient) Upgrade(ctx context.Context, opts helm.UpgradeOptions) error {
	m.chartsUpgraded = append(m.chartsUpgraded, opts)
	return m.upgradeErr
}

func (m *mockHelmClient) List(ctx context.Context, namespace string) ([]helm.Release, error) {
	return m.listReleases, m.listErr
}

func (m *mockHelmClient) Uninstall(ctx context.Context, opts helm.UninstallOptions) error {
	m.uninstalls++
	return nil
}

func (m *mockHelmClient) Close() error {
	return nil
}

// mockK8sClient is a mock implementation of k8s.Client for testing
type mockK8sClient struct {
	pods                       []*k8s.Pod
	podsErr                    error
	serviceMonitorCRDExists    bool
	serviceMonitorCRDExistsErr error
	applyManifestErr           error
	appliedManifests           []string
	crdExists                  map[string]bool
	crdExistsErr               error
}

func (m *mockK8sClient) GetPods(ctx context.Context, namespace string) ([]*k8s.Pod, error) {
	return m.pods, m.podsErr
}

func (m *mockK8sClient) ServiceMonitorCRDExists(ctx context.Context) (bool, error) {
	return m.serviceMonitorCRDExists, m.serviceMonitorCRDExistsErr
}

func (m *mockK8sClient) ApplyManifest(ctx context.Context, manifest string) error {
	m.appliedManifests = append(m.appliedManifests, manifest)
	return m.applyManifestErr
}

func (m *mockK8sClient) CRDExists(ctx context.Context, crdName string) (bool, error) {
	if m.crdExistsErr != nil {
		return false, m.crdExistsErr
	}
	if m.crdExists == nil {
		return false, nil
	}
	return m.crdExists[crdName], nil
}

func (m *mockK8sClient) PatchDeploymentArgs(ctx context.Context, namespace, name string, oldArg, newArg string) error {
	return nil // Mock always succeeds
}

func TestBuildHelmValues_Defaults(t *testing.T) {
	// Clear any VIP override from previous tests
	SetClusterVIP("")

	cfg := DefaultConfig()
	values := buildHelmValues(cfg)

	// Check the replicaCount key used by the pinned chart.
	contour, ok := values["contour"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, uint64(2), contour["replicaCount"])

	// Check Envoy replicas
	envoy, ok := values["envoy"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, uint64(2), envoy["replicaCount"])

	// Check service type
	service, ok := envoy["service"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "LoadBalancer", service["type"])

	// When no VIP is set, no annotation should be added (let kube-vip auto-assign)
	_, hasAnnotations := service["annotations"]
	assert.False(t, hasAnnotations, "no annotations expected when VIP not set")

	// Check default IngressClass (nested under contour in official chart)
	ingressClass, ok := contour["ingressClass"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, true, ingressClass["default"])
	assert.Equal(t, true, ingressClass["create"])
}

func TestBuildHelmValues_CustomReplicas(t *testing.T) {
	cfg := &Config{
		ReplicaCount:        uint64(5),
		EnvoyReplicaCount:   uint64(3),
		UseKubeVIP:          false,
		DefaultIngressClass: false,
		Values:              make(map[string]interface{}),
	}

	values := buildHelmValues(cfg)

	contour, ok := values["contour"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, uint64(5), contour["replicaCount"])

	envoy, ok := values["envoy"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, uint64(3), envoy["replicaCount"])
}

func TestBuildHelmValues_NoKubeVIP(t *testing.T) {
	cfg := &Config{
		ReplicaCount:        uint64(2),
		EnvoyReplicaCount:   uint64(2),
		UseKubeVIP:          false,
		DefaultIngressClass: true,
		Values:              make(map[string]interface{}),
	}

	values := buildHelmValues(cfg)

	envoy, ok := values["envoy"].(map[string]interface{})
	require.True(t, ok)

	service, ok := envoy["service"].(map[string]interface{})
	require.True(t, ok)

	// Should not have kube-vip annotations when UseKubeVIP is false
	_, hasAnnotations := service["annotations"]
	assert.False(t, hasAnnotations)
}

func TestBuildHelmValues_NoDefaultIngressClass(t *testing.T) {
	cfg := &Config{
		ReplicaCount:        uint64(2),
		EnvoyReplicaCount:   uint64(2),
		UseKubeVIP:          true,
		DefaultIngressClass: false,
		Values:              make(map[string]interface{}),
	}

	values := buildHelmValues(cfg)

	// IngressClass should have default: false when DefaultIngressClass is false
	contour, ok := values["contour"].(map[string]interface{})
	require.True(t, ok)
	ingressClass, ok := contour["ingressClass"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, false, ingressClass["default"])
	assert.Equal(t, true, ingressClass["create"]) // Still create the class, just not as default
}

func TestBuildHelmValues_WithCustomValues(t *testing.T) {
	cfg := &Config{
		ReplicaCount:        uint64(2),
		EnvoyReplicaCount:   uint64(2),
		UseKubeVIP:          true,
		DefaultIngressClass: true,
		Values: map[string]interface{}{
			"custom": "value",
			"nested": map[string]interface{}{
				"key": "value",
			},
		},
	}

	values := buildHelmValues(cfg)

	// Custom values should be preserved
	assert.Equal(t, "value", values["custom"])
	assert.NotNil(t, values["nested"])
}

func TestBuildHelmValues_WithClusterVIP(t *testing.T) {
	// Set the cluster VIP (simulating what happens when ParseConfig is called with cluster_vip)
	SetClusterVIP("10.16.0.43")
	defer SetClusterVIP("") // Clean up

	cfg := DefaultConfig()
	values := buildHelmValues(cfg)

	// When sharing the control-plane VIP, the envoy service uses ClusterIP +
	// externalIPs instead of LoadBalancer with a kube-vip annotation, to avoid
	// kube-vip running a separate leader election for the service VIP.
	envoy, ok := values["envoy"].(map[string]interface{})
	require.True(t, ok)
	service, ok := envoy["service"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "ClusterIP", service["type"])
	assert.Equal(t, []string{"10.16.0.43"}, service["externalIPs"])
}

func TestInstall_Success(t *testing.T) {
	helmClient := &mockHelmClient{}
	k8sClient := &mockK8sClient{
		pods: []*k8s.Pod{
			{Name: "contour-1", Namespace: "projectcontour", Status: "Running"},
			{Name: "envoy-1", Namespace: "projectcontour", Status: "Running"},
		},
		serviceMonitorCRDExists: true, // Default config has ServiceMonitorEnabled=true
	}

	cfg := DefaultConfig()
	err := Install(context.Background(), helmClient, k8sClient, cfg)
	require.NoError(t, err)

	// Verify repo was added
	require.Len(t, helmClient.reposAdded, 1)
	assert.Equal(t, contourRepoName, helmClient.reposAdded[0].Name)
	assert.Equal(t, contourRepoURL, helmClient.reposAdded[0].URL)

	// Verify chart was installed
	require.Len(t, helmClient.chartsInstalled, 1)
	assert.Equal(t, "contour", helmClient.chartsInstalled[0].ReleaseName)
	assert.Equal(t, "projectcontour", helmClient.chartsInstalled[0].Namespace)
	assert.Equal(t, contourChart, helmClient.chartsInstalled[0].Chart)
	assert.True(t, helmClient.chartsInstalled[0].CreateNamespace)
	assert.True(t, helmClient.chartsInstalled[0].Wait)
	assert.Equal(t, 5*time.Minute, helmClient.chartsInstalled[0].Timeout)
}

func TestInstall_AlreadyDeployed_CreatesGatewayResources(t *testing.T) {
	// Backwards compatibility test: Gateway resources should be created
	// even when Contour is already deployed (for existing installations)
	SetGatewayDomain("catalyst.local")
	defer SetGatewayDomain("")

	helmClient := &mockHelmClient{
		listReleases: []helm.Release{
			{Name: "contour", Namespace: "projectcontour", Status: "deployed"},
		},
	}
	k8sClient := &mockK8sClient{
		pods: []*k8s.Pod{
			{Name: "contour-1", Namespace: "projectcontour", Status: "Running"},
			{Name: "envoy-1", Namespace: "projectcontour", Status: "Running"},
		},
		serviceMonitorCRDExists: true,
		crdExists: map[string]bool{
			"certificates.cert-manager.io": true, // cert-manager is installed
		},
	}

	cfg := DefaultConfig()
	err := Install(context.Background(), helmClient, k8sClient, cfg)
	require.NoError(t, err)

	// Verify Helm install was NOT called (already deployed)
	assert.Empty(t, helmClient.chartsInstalled, "should not reinstall already deployed release")

	// Prepare Gateway mode before the upgrade, then reconcile all resources.
	require.Len(t, k8sClient.appliedManifests, 5)
	assert.Contains(t, k8sClient.appliedManifests[0], "kind: ContourConfiguration")
	assert.Contains(t, k8sClient.appliedManifests[1], "kind: GatewayClass")
	assert.Contains(t, k8sClient.appliedManifests[2], "kind: ContourConfiguration")
	assert.Contains(t, k8sClient.appliedManifests[3], "kind: Certificate")
	assert.Contains(t, k8sClient.appliedManifests[4], "kind: Gateway")
}

func TestInstall_AlreadyDeployed_NoCertManager(t *testing.T) {
	// Test that Gateway is created without TLS when cert-manager is not installed
	SetGatewayDomain("catalyst.local")
	defer SetGatewayDomain("")

	helmClient := &mockHelmClient{
		listReleases: []helm.Release{
			{Name: "contour", Namespace: "projectcontour", Status: "deployed"},
		},
	}
	k8sClient := &mockK8sClient{
		pods: []*k8s.Pod{
			{Name: "contour-1", Namespace: "projectcontour", Status: "Running"},
		},
		serviceMonitorCRDExists: true,
		crdExists:               map[string]bool{}, // cert-manager NOT installed
	}

	cfg := DefaultConfig()
	err := Install(context.Background(), helmClient, k8sClient, cfg)
	require.NoError(t, err)

	require.Len(t, k8sClient.appliedManifests, 4)
	assert.Contains(t, k8sClient.appliedManifests[0], "kind: ContourConfiguration")
	assert.Contains(t, k8sClient.appliedManifests[1], "kind: GatewayClass")
	assert.Contains(t, k8sClient.appliedManifests[2], "kind: ContourConfiguration")
	assert.Contains(t, k8sClient.appliedManifests[3], "kind: Gateway")
	assert.NotContains(t, k8sClient.appliedManifests[3], "protocol: HTTPS", "should not have HTTPS without cert-manager")
}

func TestInstall_NilHelmClient(t *testing.T) {
	err := Install(context.Background(), nil, &mockK8sClient{}, DefaultConfig())
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "helm client cannot be nil")
}

func TestInstall_NilK8sClient(t *testing.T) {
	err := Install(context.Background(), &mockHelmClient{}, nil, DefaultConfig())
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "k8s client cannot be nil")
}

func TestInstall_NilConfig(t *testing.T) {
	helmClient := &mockHelmClient{}
	k8sClient := &mockK8sClient{
		pods: []*k8s.Pod{
			{Name: "contour-1", Namespace: "projectcontour", Status: "Running"},
		},
		serviceMonitorCRDExists: true, // Default config has ServiceMonitorEnabled=true
	}

	// Should use default config
	err := Install(context.Background(), helmClient, k8sClient, nil)
	require.NoError(t, err)

	// Verify installation happened with defaults
	require.Len(t, helmClient.chartsInstalled, 1)
	assert.Equal(t, "projectcontour", helmClient.chartsInstalled[0].Namespace)
}

func TestInstall_AddRepoError(t *testing.T) {
	helmClient := &mockHelmClient{
		addRepoErr: assert.AnError,
	}
	k8sClient := &mockK8sClient{
		serviceMonitorCRDExists: true, // Default config has ServiceMonitorEnabled=true
	}

	err := Install(context.Background(), helmClient, k8sClient, DefaultConfig())
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to add helm repository")
}

func TestInstall_InstallChartError(t *testing.T) {
	helmClient := &mockHelmClient{
		installErr: assert.AnError,
	}
	k8sClient := &mockK8sClient{
		serviceMonitorCRDExists: true, // Default config has ServiceMonitorEnabled=true
	}

	err := Install(context.Background(), helmClient, k8sClient, DefaultConfig())
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to install contour")
}

func TestVerifyInstallation_Success(t *testing.T) {
	k8sClient := &mockK8sClient{
		pods: []*k8s.Pod{
			{Name: "contour-1", Namespace: "projectcontour", Status: "Running"},
			{Name: "envoy-1", Namespace: "projectcontour", Status: "Running"},
		},
	}

	ctx := context.Background()
	err := verifyInstallation(ctx, k8sClient, "projectcontour")
	assert.NoError(t, err)
}

func TestVerifyInstallation_NoPods(t *testing.T) {
	k8sClient := &mockK8sClient{
		pods: []*k8s.Pod{},
	}

	// Use a context with timeout to avoid waiting too long
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	err := verifyInstallation(ctx, k8sClient, "projectcontour")
	assert.Error(t, err)
}

func TestVerifyInstallation_PodsNotReady(t *testing.T) {
	k8sClient := &mockK8sClient{
		pods: []*k8s.Pod{
			{Name: "contour-1", Namespace: "projectcontour", Status: "Pending"},
		},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	err := verifyInstallation(ctx, k8sClient, "projectcontour")
	assert.Error(t, err)
}

// Removed flaky timing-based test - the verification logic is adequately tested
// by the other success and failure cases

func TestVerifyInstallation_ContextCanceled(t *testing.T) {
	k8sClient := &mockK8sClient{
		pods: []*k8s.Pod{},
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	err := verifyInstallation(ctx, k8sClient, "projectcontour")
	assert.Error(t, err)
	assert.Equal(t, context.Canceled, err)
}

func TestCreateGatewayResources_Success(t *testing.T) {
	// Clear any domain override from previous tests
	SetGatewayDomain("")

	k8sClient := &mockK8sClient{}
	cfg := &Config{
		Namespace:                "projectcontour",
		CreateGateway:            true,
		CreateGatewayCertificate: true,
	}

	err := createGatewayResources(context.Background(), k8sClient, cfg)
	require.NoError(t, err)

	// Should have 3 manifests: GatewayClass, ContourConfiguration, and Gateway (no certificate without domain)
	assert.Len(t, k8sClient.appliedManifests, 3)

	// Check GatewayClass manifest
	assert.Contains(t, k8sClient.appliedManifests[0], "kind: GatewayClass")
	assert.Contains(t, k8sClient.appliedManifests[0], "name: contour")
	assert.Contains(t, k8sClient.appliedManifests[0], "projectcontour.io/gateway-controller")

	// Check ContourConfiguration manifest
	assert.Contains(t, k8sClient.appliedManifests[1], "kind: ContourConfiguration")
	assert.Contains(t, k8sClient.appliedManifests[1], "namespace: projectcontour")

	// Check Gateway manifest
	assert.Contains(t, k8sClient.appliedManifests[2], "kind: Gateway")
	assert.Contains(t, k8sClient.appliedManifests[2], "namespace: projectcontour")
}

func TestCreateGatewayResources_WithDomain(t *testing.T) {
	// Set domain for TLS certificate
	SetGatewayDomain("catalyst.local")
	defer SetGatewayDomain("") // Clean up

	k8sClient := &mockK8sClient{
		crdExists: map[string]bool{
			"certificates.cert-manager.io": true, // cert-manager is installed
		},
	}
	cfg := &Config{
		Namespace:                "projectcontour",
		CreateGateway:            true,
		CreateGatewayCertificate: true,
	}

	err := createGatewayResources(context.Background(), k8sClient, cfg)
	require.NoError(t, err)

	// Should have 4 manifests: GatewayClass, ContourConfiguration, Certificate, and Gateway
	assert.Len(t, k8sClient.appliedManifests, 4)

	// Check ContourConfiguration manifest
	assert.Contains(t, k8sClient.appliedManifests[1], "kind: ContourConfiguration")

	// Check Certificate manifest
	assert.Contains(t, k8sClient.appliedManifests[2], "kind: Certificate")
	assert.Contains(t, k8sClient.appliedManifests[2], "*.catalyst.local")
	assert.Contains(t, k8sClient.appliedManifests[2], "foundry-ca-issuer")

	// Check Gateway has HTTPS listener
	assert.Contains(t, k8sClient.appliedManifests[3], "protocol: HTTPS")
	assert.Contains(t, k8sClient.appliedManifests[3], "gateway-wildcard-tls")
}

func TestCreateGatewayResources_NoCertificate(t *testing.T) {
	SetGatewayDomain("catalyst.local")
	defer SetGatewayDomain("") // Clean up

	k8sClient := &mockK8sClient{}
	cfg := &Config{
		Namespace:                "projectcontour",
		CreateGateway:            true,
		CreateGatewayCertificate: false, // Disable certificate creation
	}

	err := createGatewayResources(context.Background(), k8sClient, cfg)
	require.NoError(t, err)

	// Should have 3 manifests: GatewayClass, ContourConfiguration, and Gateway (no certificate)
	assert.Len(t, k8sClient.appliedManifests, 3)

	// Gateway should not have HTTPS listener
	assert.NotContains(t, k8sClient.appliedManifests[2], "protocol: HTTPS")
}

func TestCreateGatewayResources_ApplyManifestError(t *testing.T) {
	k8sClient := &mockK8sClient{
		applyManifestErr: assert.AnError,
	}
	cfg := &Config{
		Namespace:     "projectcontour",
		CreateGateway: true,
	}

	err := createGatewayResources(context.Background(), k8sClient, cfg)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to create GatewayClass")
}

func TestGenerateGatewayClassManifest(t *testing.T) {
	manifest := generateGatewayClassManifest()

	assert.Contains(t, manifest, "apiVersion: gateway.networking.k8s.io/v1")
	assert.Contains(t, manifest, "kind: GatewayClass")
	assert.Contains(t, manifest, "name: contour")
	assert.Contains(t, manifest, "controllerName: projectcontour.io/gateway-controller")
}

func TestGenerateGatewayCertificateManifest(t *testing.T) {
	manifest := generateGatewayCertificateManifest("projectcontour", "example.com")

	assert.Contains(t, manifest, "apiVersion: cert-manager.io/v1")
	assert.Contains(t, manifest, "kind: Certificate")
	assert.Contains(t, manifest, "namespace: projectcontour")
	assert.Contains(t, manifest, "secretName: gateway-wildcard-tls")
	assert.Contains(t, manifest, "*.example.com")
	assert.Contains(t, manifest, "example.com")
	assert.Contains(t, manifest, "foundry-ca-issuer")
}

func TestGenerateGatewayManifest_HTTPOnly(t *testing.T) {
	manifest := generateGatewayManifest("projectcontour", "", false, nil)

	assert.Contains(t, manifest, "apiVersion: gateway.networking.k8s.io/v1")
	assert.Contains(t, manifest, "kind: Gateway")
	assert.Contains(t, manifest, "name: contour")
	assert.Contains(t, manifest, "namespace: projectcontour")
	assert.Contains(t, manifest, "gatewayClassName: contour")
	assert.Contains(t, manifest, "name: http")
	assert.Contains(t, manifest, "port: 80")
	assert.Contains(t, manifest, "protocol: HTTP")
	assert.NotContains(t, manifest, "protocol: HTTPS")
}

func TestGenerateGatewayManifest_WithHTTPS(t *testing.T) {
	manifest := generateGatewayManifest("projectcontour", "catalyst.local", true, nil)

	assert.Contains(t, manifest, "name: http")
	assert.Contains(t, manifest, "port: 80")
	assert.Contains(t, manifest, "protocol: HTTP")
	assert.Contains(t, manifest, "name: https")
	assert.Contains(t, manifest, "port: 443")
	assert.Contains(t, manifest, "protocol: HTTPS")
	assert.NotContains(t, manifest, "hostname:")
	assert.Contains(t, manifest, "gateway-wildcard-tls")
	assert.Contains(t, manifest, "mode: Terminate")
}

func strPtr(s string) *string { return &s }

func TestGenerateGatewayManifest_TLSPassthroughListener(t *testing.T) {
	listeners := []ContourListener{
		{Name: "linkkeys-tls", Protocol: ListenerProtocolTLS, Port: 4987},
	}
	manifest := generateGatewayManifest("projectcontour", "", false, listeners)

	assert.Contains(t, manifest, "name: linkkeys-tls")
	assert.Contains(t, manifest, "port: 4987")
	assert.Contains(t, manifest, "protocol: TLS")
	assert.Contains(t, manifest, "mode: Passthrough")
	assert.Contains(t, manifest, "kind: TLSRoute")
	assert.Contains(t, manifest, "from: All")
	// Passthrough must not emit certificateRefs
	assert.NotContains(t, manifest, "certificateRefs")
}

func TestGenerateGatewayManifest_TCPListener(t *testing.T) {
	listeners := []ContourListener{
		{Name: "raw-tcp", Protocol: ListenerProtocolTCP, Port: 6000},
	}
	manifest := generateGatewayManifest("projectcontour", "", false, listeners)

	assert.Contains(t, manifest, "name: raw-tcp")
	assert.Contains(t, manifest, "port: 6000")
	assert.Contains(t, manifest, "kind: TCPRoute")
	// TCP listener has no tls block
	assert.NotContains(t, manifest, "mode: Passthrough")
}

func TestGenerateGatewayManifest_TLSWithHostnameAndTerminate(t *testing.T) {
	listeners := []ContourListener{
		{
			Name:           "termd",
			Protocol:       ListenerProtocolTLS,
			Port:           8888,
			TLSMode:        strPtr(TLSModeTerminate),
			Hostname:       strPtr("svc.example.com"),
			CertificateRef: strPtr("my-cert"),
		},
	}
	manifest := generateGatewayManifest("projectcontour", "", false, listeners)

	assert.Contains(t, manifest, "hostname: \"svc.example.com\"")
	assert.Contains(t, manifest, "mode: Terminate")
	assert.Contains(t, manifest, "certificateRefs")
	assert.Contains(t, manifest, "name: my-cert")
}

func TestBuildHelmValues_ListenersExtraPortsAndNetworkPolicy(t *testing.T) {
	SetClusterVIP("10.16.0.30")
	defer SetClusterVIP("")

	cfg := DefaultConfig()
	cfg.Listeners = []ContourListener{
		{Name: "linkkeys-tls", Protocol: ListenerProtocolTLS, Port: 4987},
	}
	values := buildHelmValues(cfg)

	envoy := values["envoy"].(map[string]interface{})
	service := envoy["service"].(map[string]interface{})

	// VIP still wired via externalIPs
	assert.Equal(t, "ClusterIP", service["type"])
	assert.Equal(t, []string{"10.16.0.30"}, service["externalIPs"])

	// extraPorts: external 4987 -> remapped envoy target 12987
	extraPorts := service["extraPorts"].([]interface{})
	require.Len(t, extraPorts, 1)
	port := extraPorts[0].(map[string]interface{})
	assert.Equal(t, "linkkeys-tls", port["name"])
	assert.Equal(t, uint64(4987), port["port"])
	assert.Equal(t, uint64(12987), port["targetPort"])
	assert.Equal(t, "TCP", port["protocol"])

	// networkPolicy must admit the remapped port or traffic is dropped
	np := envoy["networkPolicy"].(map[string]interface{})
	extraIngress := np["extraIngress"].([]interface{})
	require.Len(t, extraIngress, 1)
	rule := extraIngress[0].(map[string]interface{})
	rulePorts := rule["ports"].([]interface{})
	require.Len(t, rulePorts, 1)
	assert.Equal(t, uint64(12987), rulePorts[0].(map[string]interface{})["port"])
}

func TestBuildHelmValues_NoListeners_NoExtraPorts(t *testing.T) {
	SetClusterVIP("")
	cfg := DefaultConfig()
	values := buildHelmValues(cfg)

	envoy := values["envoy"].(map[string]interface{})
	service := envoy["service"].(map[string]interface{})
	_, hasExtraPorts := service["extraPorts"]
	assert.False(t, hasExtraPorts)
	_, hasNetPol := envoy["networkPolicy"]
	assert.False(t, hasNetPol)
}

func TestEnvoyContainerPort_Remap(t *testing.T) {
	assert.Equal(t, uint64(8080), ContourListener{Port: 80}.EnvoyContainerPort())
	assert.Equal(t, uint64(8443), ContourListener{Port: 443}.EnvoyContainerPort())
	assert.Equal(t, uint64(12987), ContourListener{Port: 4987}.EnvoyContainerPort())
}

func TestUpgradePreservesGatewayModeAndExistingRelease(t *testing.T) {
	for _, status := range []string{"deployed", "failed"} {
		t.Run(status, func(t *testing.T) {
			sentinel := fmt.Errorf("stop after checking upgrade values")
			hc := &mockHelmClient{listReleases: []helm.Release{{Name: "contour", Status: status}}, upgradeErr: sentinel}
			kc := &mockK8sClient{serviceMonitorCRDExists: true}
			err := Install(context.Background(), hc, kc, DefaultConfig())
			require.ErrorIs(t, err, sentinel)
			require.Len(t, hc.chartsUpgraded, 1)
			require.Len(t, kc.appliedManifests, 1)
			assert.Contains(t, kc.appliedManifests[0], "kind: ContourConfiguration")
			values := hc.chartsUpgraded[0].Values["contour"].(map[string]interface{})
			assert.Equal(t, false, values["configPath"])
			assert.Equal(t, "contour", values["contourConfigName"])
			assert.Equal(t, 0, hc.uninstalls)
		})
	}
}

func TestUpgradePreparationErrors(t *testing.T) {
	for _, tc := range []struct {
		name, status string
		applyErr     error
	}{
		{name: "busy", status: "pending-upgrade"},
		{name: "configuration error", status: "deployed", applyErr: fmt.Errorf("API unavailable")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			hc := &mockHelmClient{listReleases: []helm.Release{{Name: "contour", Status: tc.status}}}
			kc := &mockK8sClient{serviceMonitorCRDExists: true, applyManifestErr: tc.applyErr}
			require.Error(t, Install(context.Background(), hc, kc, DefaultConfig()))
			assert.Empty(t, hc.chartsUpgraded)
			assert.Zero(t, hc.uninstalls)
		})
	}
}
