package setup

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// SetupState is generated from CSIL in types.gen.go
// Methods below extend the generated type with business logic

// Step represents a setup step identifier
type Step string

const (
	StepNetworkPlan     Step = "network_plan"
	StepNetworkValidate Step = "network_validate"
	StepOpenBAOInstall  Step = "openbao_install"
	StepDNSInstall      Step = "dns_install"
	StepDNSZonesCreate  Step = "dns_zones_create"
	StepZotInstall      Step = "zot_install"
	StepK8sInstall      Step = "k8s_install"
	StepComplete        Step = "complete"
)

// ConfigWithState wraps a config file with embedded setup state
type ConfigWithState struct {
	data        map[string]interface{}
	setupState  *SetupState
	configPath  string
	stateLoaded bool
}

// LoadState loads setup state from a config file
func LoadState(configPath string) (*SetupState, error) {
	data, err := os.ReadFile(configPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	var config map[string]interface{}
	if err := yaml.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("failed to parse config file: %w", err)
	}

	// Extract setup_state section if it exists
	stateData, exists := config["setup_state"]
	if !exists {
		// Return empty state if not present
		return &SetupState{}, nil
	}

	// Marshal and unmarshal to convert map to SetupState struct
	stateBytes, err := yaml.Marshal(stateData)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal setup state: %w", err)
	}

	var state SetupState
	if err := yaml.Unmarshal(stateBytes, &state); err != nil {
		return nil, fmt.Errorf("failed to unmarshal setup state: %w", err)
	}

	return &state, nil
}

// SaveState saves setup state to a config file
func SaveState(configPath string, state *SetupState) error {
	// Read existing config
	data, err := os.ReadFile(configPath)
	if err != nil {
		return fmt.Errorf("failed to read config file: %w", err)
	}

	var config map[string]interface{}
	if err := yaml.Unmarshal(data, &config); err != nil {
		return fmt.Errorf("failed to parse config file: %w", err)
	}

	// Update setup_state section
	config["setup_state"] = state

	// Write back to file
	output, err := yaml.Marshal(config)
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	if err := os.WriteFile(configPath, output, 0644); err != nil {
		return fmt.Errorf("failed to write config file: %w", err)
	}

	return nil
}

// DetermineNextStep returns the next step to execute based on current state
func DetermineNextStep(state *SetupState) Step {
	if !state.NetworkPlanned {
		return StepNetworkPlan
	}
	if !state.NetworkValidated {
		return StepNetworkValidate
	}
	if !state.OpenBAOInstalled {
		return StepOpenBAOInstall
	}
	if !state.OpenBAOInitialized {
		return StepOpenBAOInstall // Initialization happens as part of install step
	}
	if !state.DNSInstalled {
		return StepDNSInstall
	}
	if !state.DNSZonesCreated {
		return StepDNSZonesCreate
	}
	if !state.ZotInstalled {
		return StepZotInstall
	}
	if !state.K8sInstalled {
		return StepK8sInstall
	}
	if !state.StackComplete {
		return StepComplete
	}

	// All steps complete
	return StepComplete
}

// IsComplete returns true if all setup steps are complete
func (s *SetupState) IsComplete() bool {
	return s.NetworkPlanned &&
		s.NetworkValidated &&
		s.OpenBAOInstalled &&
		s.OpenBAOInitialized &&
		s.DNSInstalled &&
		s.DNSZonesCreated &&
		s.ZotInstalled &&
		s.K8sInstalled &&
		s.StackComplete
}

// Reset resets all state flags to false
func (s *SetupState) Reset() {
	s.NetworkPlanned = false
	s.NetworkValidated = false
	s.OpenBAOInstalled = false
	s.OpenBAOInitialized = false
	s.DNSInstalled = false
	s.DNSZonesCreated = false
	s.ZotInstalled = false
	s.K8sInstalled = false
	s.StackComplete = false
}
