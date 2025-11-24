package setup

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoadState_EmptyFile(t *testing.T) {
	// Create temporary config without setup_state
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "stack.yaml")

	configContent := `version: "1.0"
cluster:
  name: test
  domain: example.com
  nodes:
    - hostname: node1
      role: control-plane
components:
  k3s:
    version: v1.28.5
`
	err := os.WriteFile(configPath, []byte(configContent), 0644)
	require.NoError(t, err)

	// Load state should return empty state
	state, err := LoadState(configPath)
	require.NoError(t, err)
	assert.NotNil(t, state)
	assert.False(t, state.NetworkPlanned)
	assert.False(t, state.OpenBAOInstalled)
	assert.False(t, state.StackComplete)
}

func TestLoadState_WithState(t *testing.T) {
	// Create temporary config with setup_state
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "stack.yaml")

	configContent := `version: "1.0"
cluster:
  name: test
  domain: example.com
  nodes:
    - hostname: node1
      role: control-plane
components:
  k3s:
    version: v1.28.5
setup_state:
  network_planned: true
  network_validated: true
  openbao_installed: false
  dns_installed: false
  dns_zones_created: false
  zot_installed: false
  k8s_installed: false
  stack_complete: false
`
	err := os.WriteFile(configPath, []byte(configContent), 0644)
	require.NoError(t, err)

	// Load state
	state, err := LoadState(configPath)
	require.NoError(t, err)
	assert.NotNil(t, state)
	assert.True(t, state.NetworkPlanned)
	assert.True(t, state.NetworkValidated)
	assert.False(t, state.OpenBAOInstalled)
	assert.False(t, state.StackComplete)
}

func TestLoadState_NonExistentFile(t *testing.T) {
	state, err := LoadState("/nonexistent/path/config.yaml")
	assert.Error(t, err)
	assert.Nil(t, state)
}

func TestLoadState_InvalidYAML(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "invalid.yaml")

	invalidContent := `this is not: valid: yaml: content`
	err := os.WriteFile(configPath, []byte(invalidContent), 0644)
	require.NoError(t, err)

	state, err := LoadState(configPath)
	assert.Error(t, err)
	assert.Nil(t, state)
}

func TestSaveState_NewState(t *testing.T) {
	// Create temporary config
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "stack.yaml")

	configContent := `version: "1.0"
cluster:
  name: test
  domain: example.com
  nodes:
    - hostname: node1
      role: control-plane
components:
  k3s:
    version: v1.28.5
`
	err := os.WriteFile(configPath, []byte(configContent), 0644)
	require.NoError(t, err)

	// Create and save state
	state := &SetupState{
		NetworkPlanned:   true,
		NetworkValidated: true,
		OpenBAOInstalled: false,
	}

	err = SaveState(configPath, state)
	require.NoError(t, err)

	// Load and verify
	loadedState, err := LoadState(configPath)
	require.NoError(t, err)
	assert.True(t, loadedState.NetworkPlanned)
	assert.True(t, loadedState.NetworkValidated)
	assert.False(t, loadedState.OpenBAOInstalled)
}

func TestSaveState_UpdateExisting(t *testing.T) {
	// Create temporary config with existing state
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "stack.yaml")

	configContent := `version: "1.0"
cluster:
  name: test
  domain: example.com
  nodes:
    - hostname: node1
      role: control-plane
components:
  k3s:
    version: v1.28.5
setup_state:
  network_planned: true
  network_validated: false
  openbao_installed: false
  dns_installed: false
  dns_zones_created: false
  zot_installed: false
  k8s_installed: false
  stack_complete: false
`
	err := os.WriteFile(configPath, []byte(configContent), 0644)
	require.NoError(t, err)

	// Update state
	state := &SetupState{
		NetworkPlanned:   true,
		NetworkValidated: true,
		OpenBAOInstalled: true,
	}

	err = SaveState(configPath, state)
	require.NoError(t, err)

	// Load and verify
	loadedState, err := LoadState(configPath)
	require.NoError(t, err)
	assert.True(t, loadedState.NetworkPlanned)
	assert.True(t, loadedState.NetworkValidated)
	assert.True(t, loadedState.OpenBAOInstalled)
	assert.False(t, loadedState.DNSInstalled)
}

func TestSaveState_NonExistentFile(t *testing.T) {
	state := &SetupState{
		NetworkPlanned: true,
	}
	err := SaveState("/nonexistent/path/config.yaml", state)
	assert.Error(t, err)
}

func TestDetermineNextStep_InitialState(t *testing.T) {
	state := &SetupState{}
	step := DetermineNextStep(state)
	assert.Equal(t, StepNetworkPlan, step)
}

func TestDetermineNextStep_NetworkPlanned(t *testing.T) {
	state := &SetupState{
		NetworkPlanned: true,
	}
	step := DetermineNextStep(state)
	assert.Equal(t, StepNetworkValidate, step)
}

func TestDetermineNextStep_NetworkValidated(t *testing.T) {
	state := &SetupState{
		NetworkPlanned:   true,
		NetworkValidated: true,
	}
	step := DetermineNextStep(state)
	assert.Equal(t, StepOpenBAOInstall, step)
}

func TestDetermineNextStep_OpenBAOInstalled(t *testing.T) {
	state := &SetupState{
		NetworkPlanned:     true,
		NetworkValidated:   true,
		OpenBAOInstalled:   true,
		OpenBAOInitialized: true,
	}
	step := DetermineNextStep(state)
	assert.Equal(t, StepDNSInstall, step)
}

func TestDetermineNextStep_DNSInstalled(t *testing.T) {
	state := &SetupState{
		NetworkPlanned:     true,
		NetworkValidated:   true,
		OpenBAOInstalled:   true,
		OpenBAOInitialized: true,
		DNSInstalled:       true,
	}
	step := DetermineNextStep(state)
	assert.Equal(t, StepDNSZonesCreate, step)
}

func TestDetermineNextStep_DNSZonesCreated(t *testing.T) {
	state := &SetupState{
		NetworkPlanned:     true,
		NetworkValidated:   true,
		OpenBAOInstalled:   true,
		OpenBAOInitialized: true,
		DNSInstalled:       true,
		DNSZonesCreated:    true,
	}
	step := DetermineNextStep(state)
	assert.Equal(t, StepZotInstall, step)
}

func TestDetermineNextStep_ZotInstalled(t *testing.T) {
	state := &SetupState{
		NetworkPlanned:     true,
		NetworkValidated:   true,
		OpenBAOInstalled:   true,
		OpenBAOInitialized: true,
		DNSInstalled:       true,
		DNSZonesCreated:    true,
		ZotInstalled:       true,
	}
	step := DetermineNextStep(state)
	assert.Equal(t, StepK8sInstall, step)
}

func TestDetermineNextStep_K8sInstalled(t *testing.T) {
	state := &SetupState{
		NetworkPlanned:     true,
		NetworkValidated:   true,
		OpenBAOInstalled:   true,
		OpenBAOInitialized: true,
		DNSInstalled:       true,
		DNSZonesCreated:    true,
		ZotInstalled:       true,
		K8sInstalled:       true,
	}
	step := DetermineNextStep(state)
	assert.Equal(t, StepComplete, step)
}

func TestDetermineNextStep_AllComplete(t *testing.T) {
	state := &SetupState{
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
	step := DetermineNextStep(state)
	assert.Equal(t, StepComplete, step)
}

func TestIsComplete_False(t *testing.T) {
	state := &SetupState{
		NetworkPlanned:   true,
		NetworkValidated: true,
		OpenBAOInstalled: false,
	}
	assert.False(t, state.IsComplete())
}

func TestIsComplete_True(t *testing.T) {
	state := &SetupState{
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
	assert.True(t, state.IsComplete())
}

func TestReset(t *testing.T) {
	state := &SetupState{
		NetworkPlanned:   true,
		NetworkValidated: true,
		OpenBAOInstalled: true,
		DNSInstalled:     true,
		DNSZonesCreated:  true,
		ZotInstalled:     true,
		K8sInstalled:     true,
		StackComplete:    true,
	}

	state.Reset()

	assert.False(t, state.NetworkPlanned)
	assert.False(t, state.NetworkValidated)
	assert.False(t, state.OpenBAOInstalled)
	assert.False(t, state.DNSInstalled)
	assert.False(t, state.DNSZonesCreated)
	assert.False(t, state.ZotInstalled)
	assert.False(t, state.K8sInstalled)
	assert.False(t, state.StackComplete)
}

func TestSaveAndLoad_RoundTrip(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "stack.yaml")

	// Create initial config
	configContent := `version: "1.0"
cluster:
  name: test
  domain: example.com
  nodes:
    - hostname: node1
      role: control-plane
components:
  k3s:
    version: v1.28.5
`
	err := os.WriteFile(configPath, []byte(configContent), 0644)
	require.NoError(t, err)

	// Save state multiple times with different values
	states := []*SetupState{
		{NetworkPlanned: true},
		{NetworkPlanned: true, NetworkValidated: true},
		{NetworkPlanned: true, NetworkValidated: true, OpenBAOInstalled: true},
	}

	for _, expectedState := range states {
		err = SaveState(configPath, expectedState)
		require.NoError(t, err)

		loadedState, err := LoadState(configPath)
		require.NoError(t, err)
		assert.Equal(t, expectedState, loadedState)
	}
}
