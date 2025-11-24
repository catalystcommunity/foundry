package setup

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/catalystcommunity/foundry/v1/internal/config"
	"github.com/catalystcommunity/foundry/v1/internal/setup"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockExecutor implements StepExecutor for testing
type mockExecutor struct {
	executed     bool
	validated    bool
	shouldFail   bool
	validateFail bool
	description  string
}

func (m *mockExecutor) Execute(ctx context.Context, cfg *config.Config) error {
	m.executed = true
	if m.shouldFail {
		return assert.AnError
	}
	return nil
}

func (m *mockExecutor) Validate(ctx context.Context, cfg *config.Config) error {
	m.validated = true
	if m.validateFail {
		return assert.AnError
	}
	return nil
}

func (m *mockExecutor) Description() string {
	if m.description != "" {
		return m.description
	}
	return "Mock step"
}

func createTestConfig(t *testing.T) string {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "stack.yaml")

	configContent := `cluster:
  name: test
  domain: example.com
  nodes:
    - hostname: node1
      role: control-plane
components:
  k3s:
    tag: v1.28.5
network:
  gateway: 192.168.1.1
  netmask: 255.255.255.0
  k8s_vip: 192.168.1.100
  openbao_hosts:
    - 192.168.1.10
  dns_hosts:
    - 192.168.1.10
  zot_hosts:
    - 192.168.1.10
dns:
  infrastructure_zones:
    - name: infra.example.com
      public: true
      public_cname: home.example.com
  kubernetes_zones:
    - name: k8s.example.com
      public: true
      public_cname: home.example.com
  backend: sqlite
  api_key: test-key
`
	err := os.WriteFile(configPath, []byte(configContent), 0644)
	require.NoError(t, err)

	return configPath
}

func TestNewWizard(t *testing.T) {
	configPath := createTestConfig(t)

	wizard, err := NewWizard(configPath)
	require.NoError(t, err)
	assert.NotNil(t, wizard)
	assert.Equal(t, configPath, wizard.configPath)
	assert.NotNil(t, wizard.config)
	assert.NotNil(t, wizard.state)
	assert.NotNil(t, wizard.executors)
}

func TestNewWizard_InvalidConfig(t *testing.T) {
	wizard, err := NewWizard("/nonexistent/config.yaml")
	assert.Error(t, err)
	assert.Nil(t, wizard)
	assert.Contains(t, err.Error(), "failed to load config")
}

func TestWizard_UpdateState(t *testing.T) {
	configPath := createTestConfig(t)
	wizard, err := NewWizard(configPath)
	require.NoError(t, err)

	tests := []struct {
		step     setup.Step
		checkFn  func(*setup.SetupState) bool
		stepName string
	}{
		{setup.StepNetworkPlan, func(s *setup.SetupState) bool { return s.NetworkPlanned }, "NetworkPlanned"},
		{setup.StepNetworkValidate, func(s *setup.SetupState) bool { return s.NetworkValidated }, "NetworkValidated"},
		{setup.StepOpenBAOInstall, func(s *setup.SetupState) bool { return s.OpenBAOInstalled }, "OpenBAOInstalled"},
		{setup.StepDNSInstall, func(s *setup.SetupState) bool { return s.DNSInstalled }, "DNSInstalled"},
		{setup.StepDNSZonesCreate, func(s *setup.SetupState) bool { return s.DNSZonesCreated }, "DNSZonesCreated"},
		{setup.StepZotInstall, func(s *setup.SetupState) bool { return s.ZotInstalled }, "ZotInstalled"},
		{setup.StepK8sInstall, func(s *setup.SetupState) bool { return s.K8sInstalled }, "K8sInstalled"},
		{setup.StepComplete, func(s *setup.SetupState) bool { return s.StackComplete }, "StackComplete"},
	}

	for _, tt := range tests {
		t.Run(string(tt.step), func(t *testing.T) {
			wizard.state.Reset()
			err := wizard.updateState(tt.step)
			assert.NoError(t, err)
			assert.True(t, tt.checkFn(wizard.state), "%s should be true", tt.stepName)
		})
	}
}

func TestWizard_UpdateState_UnknownStep(t *testing.T) {
	configPath := createTestConfig(t)
	wizard, err := NewWizard(configPath)
	require.NoError(t, err)

	err = wizard.updateState(setup.Step("unknown"))
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "unknown step")
}

func TestWizard_ExecuteStep_NoExecutor(t *testing.T) {
	configPath := createTestConfig(t)
	wizard, err := NewWizard(configPath)
	require.NoError(t, err)

	ctx := context.Background()

	// Execute step with no registered executor (should not fail, just skip)
	err = wizard.executeStep(ctx, setup.StepNetworkPlan)
	assert.NoError(t, err)
}

func TestWizard_ExecuteStep_WithExecutor(t *testing.T) {
	configPath := createTestConfig(t)
	wizard, err := NewWizard(configPath)
	require.NoError(t, err)

	ctx := context.Background()

	// Register mock executor
	mockExec := &mockExecutor{description: "Testing network plan"}
	wizard.executors[setup.StepNetworkPlan] = mockExec

	// Execute step
	err = wizard.executeStep(ctx, setup.StepNetworkPlan)
	assert.NoError(t, err)
	assert.True(t, mockExec.validated, "Validate should be called")
	assert.True(t, mockExec.executed, "Execute should be called")
}

func TestWizard_ExecuteStep_ValidationFails(t *testing.T) {
	configPath := createTestConfig(t)
	wizard, err := NewWizard(configPath)
	require.NoError(t, err)

	ctx := context.Background()

	// Register mock executor that fails validation
	mockExec := &mockExecutor{validateFail: true}
	wizard.executors[setup.StepNetworkPlan] = mockExec

	// Execute step should fail
	err = wizard.executeStep(ctx, setup.StepNetworkPlan)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "validation failed")
	assert.True(t, mockExec.validated)
	assert.False(t, mockExec.executed, "Execute should not be called if validation fails")
}

func TestWizard_ExecuteStep_ExecutionFails(t *testing.T) {
	configPath := createTestConfig(t)
	wizard, err := NewWizard(configPath)
	require.NoError(t, err)

	ctx := context.Background()

	// Register mock executor that fails execution
	mockExec := &mockExecutor{shouldFail: true}
	wizard.executors[setup.StepNetworkPlan] = mockExec

	// Execute step should fail
	err = wizard.executeStep(ctx, setup.StepNetworkPlan)
	assert.Error(t, err)
	assert.True(t, mockExec.validated)
	assert.True(t, mockExec.executed)
}

func TestWizard_StepName(t *testing.T) {
	configPath := createTestConfig(t)
	wizard, err := NewWizard(configPath)
	require.NoError(t, err)

	tests := []struct {
		step     setup.Step
		expected string
	}{
		{setup.StepNetworkPlan, "Network Planning"},
		{setup.StepNetworkValidate, "Network Validation"},
		{setup.StepOpenBAOInstall, "OpenBAO Installation"},
		{setup.StepDNSInstall, "DNS (PowerDNS) Installation"},
		{setup.StepDNSZonesCreate, "DNS Zones Creation"},
		{setup.StepZotInstall, "Zot Registry Installation"},
		{setup.StepK8sInstall, "Kubernetes Cluster Setup"},
		{setup.StepComplete, "Setup Complete"},
		{setup.Step("unknown"), "unknown"},
	}

	for _, tt := range tests {
		t.Run(string(tt.step), func(t *testing.T) {
			name := wizard.stepName(tt.step)
			assert.Equal(t, tt.expected, name)
		})
	}
}

func TestWizard_Reset(t *testing.T) {
	configPath := createTestConfig(t)

	// Set some state
	state := &setup.SetupState{
		NetworkPlanned:   true,
		NetworkValidated: true,
		OpenBAOInstalled: true,
	}
	err := setup.SaveState(configPath, state)
	require.NoError(t, err)

	// Create wizard and reset
	wizard, err := NewWizard(configPath)
	require.NoError(t, err)
	assert.True(t, wizard.state.NetworkPlanned)

	err = wizard.Reset()
	assert.NoError(t, err)

	// Reload state to verify it was saved
	loadedState, err := setup.LoadState(configPath)
	require.NoError(t, err)
	assert.False(t, loadedState.NetworkPlanned)
	assert.False(t, loadedState.NetworkValidated)
	assert.False(t, loadedState.OpenBAOInstalled)
}

func TestWizard_Run_AlreadyComplete(t *testing.T) {
	configPath := createTestConfig(t)

	// Set state to complete
	state := &setup.SetupState{
		NetworkPlanned:     true,
		NetworkValidated:   true,
		OpenBAOInstalled:   true,
		OpenBAOInitialized: true,
		DNSInstalled:       true,
		DNSZonesCreated:    true,
		ZotInstalled:       true,
		K8sInstalled:       true,
		StackComplete:      true,
	}
	err := setup.SaveState(configPath, state)
	require.NoError(t, err)

	wizard, err := NewWizard(configPath)
	require.NoError(t, err)

	ctx := context.Background()
	err = wizard.Run(ctx)
	assert.NoError(t, err)
}

func TestWizard_Run_Cancellation(t *testing.T) {
	configPath := createTestConfig(t)
	wizard, err := NewWizard(configPath)
	require.NoError(t, err)

	// Use a pre-cancelled context
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	err = wizard.Run(ctx)
	assert.Error(t, err)
	assert.Equal(t, context.Canceled, err)

	// Verify state was saved
	loadedState, err := setup.LoadState(configPath)
	require.NoError(t, err)
	assert.NotNil(t, loadedState)
}

func TestWizard_Run_SingleStep(t *testing.T) {
	configPath := createTestConfig(t)
	wizard, err := NewWizard(configPath)
	require.NoError(t, err)

	// Set state to nearly complete (only network plan remaining)
	wizard.state = &setup.SetupState{
		NetworkPlanned:     false, // This step will run
		NetworkValidated:   true,
		OpenBAOInstalled:   true,
		OpenBAOInitialized: true,
		DNSInstalled:       true,
		DNSZonesCreated:    true,
		ZotInstalled:       true,
		K8sInstalled:       true,
		StackComplete:      false,
	}
	err = setup.SaveState(configPath, wizard.state)
	require.NoError(t, err)

	// Register executor for network plan
	mockExec := &mockExecutor{}
	wizard.executors[setup.StepNetworkPlan] = mockExec

	// Reload to get the state we just set
	wizard, err = NewWizard(configPath)
	require.NoError(t, err)
	wizard.executors[setup.StepNetworkPlan] = mockExec

	ctx := context.Background()
	// Note: This will try to execute all remaining steps, not just one
	// Since most are already complete, it should complete quickly
	err = wizard.Run(ctx)
	assert.NoError(t, err)
}

func TestWizard_ShowProgress(t *testing.T) {
	configPath := createTestConfig(t)
	wizard, err := NewWizard(configPath)
	require.NoError(t, err)

	// Set partial state
	wizard.state.NetworkPlanned = true
	wizard.state.NetworkValidated = true

	// Should not panic
	wizard.showProgress()
}

func TestWizard_ShowWelcome(t *testing.T) {
	configPath := createTestConfig(t)
	wizard, err := NewWizard(configPath)
	require.NoError(t, err)

	// Should not panic
	wizard.showWelcome()
}

func TestWizard_ShowCompletion(t *testing.T) {
	configPath := createTestConfig(t)
	wizard, err := NewWizard(configPath)
	require.NoError(t, err)

	// Should not panic
	wizard.showCompletion()
}
