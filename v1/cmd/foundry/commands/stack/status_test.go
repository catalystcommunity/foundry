package stack

import (
	"context"
	"errors"
	"testing"

	"github.com/catalystcommunity/foundry/v1/internal/component"
	"github.com/stretchr/testify/assert"
)

// mockComponentForStatus is a mock component for status testing
type mockComponentForStatus struct {
	name         string
	status       *component.ComponentStatus
	statusErr    error
	dependencies []string
}

func (m *mockComponentForStatus) Name() string {
	return m.name
}

func (m *mockComponentForStatus) Install(ctx context.Context, cfg component.ComponentConfig) error {
	return nil
}

func (m *mockComponentForStatus) Upgrade(ctx context.Context, cfg component.ComponentConfig) error {
	return nil
}

func (m *mockComponentForStatus) Status(ctx context.Context) (*component.ComponentStatus, error) {
	if m.statusErr != nil {
		return nil, m.statusErr
	}
	return m.status, nil
}

func (m *mockComponentForStatus) Uninstall(ctx context.Context) error {
	return nil
}

func (m *mockComponentForStatus) Dependencies() []string {
	return m.dependencies
}

func TestCalculateOverallHealth(t *testing.T) {
	tests := []struct {
		name     string
		statuses map[string]*component.ComponentStatus
		want     StackHealth
	}{
		{
			name: "all components healthy",
			statuses: map[string]*component.ComponentStatus{
				"comp1": {Installed: true, Healthy: true},
				"comp2": {Installed: true, Healthy: true},
				"comp3": {Installed: true, Healthy: true},
			},
			want: StackHealth{
				TotalComponents:     3,
				InstalledComponents: 3,
				HealthyComponents:   3,
				UnhealthyComponents: 0,
				NotInstalledCount:   0,
				OverallHealthy:      true,
			},
		},
		{
			name: "some components unhealthy",
			statuses: map[string]*component.ComponentStatus{
				"comp1": {Installed: true, Healthy: true},
				"comp2": {Installed: true, Healthy: false},
				"comp3": {Installed: true, Healthy: true},
			},
			want: StackHealth{
				TotalComponents:     3,
				InstalledComponents: 3,
				HealthyComponents:   2,
				UnhealthyComponents: 1,
				NotInstalledCount:   0,
				OverallHealthy:      false,
			},
		},
		{
			name: "some components not installed",
			statuses: map[string]*component.ComponentStatus{
				"comp1": {Installed: true, Healthy: true},
				"comp2": {Installed: false, Healthy: false},
				"comp3": {Installed: true, Healthy: true},
			},
			want: StackHealth{
				TotalComponents:     3,
				InstalledComponents: 2,
				HealthyComponents:   2,
				UnhealthyComponents: 0,
				NotInstalledCount:   1,
				OverallHealthy:      true,
			},
		},
		{
			name: "no components installed",
			statuses: map[string]*component.ComponentStatus{
				"comp1": {Installed: false, Healthy: false},
				"comp2": {Installed: false, Healthy: false},
				"comp3": {Installed: false, Healthy: false},
			},
			want: StackHealth{
				TotalComponents:     3,
				InstalledComponents: 0,
				HealthyComponents:   0,
				UnhealthyComponents: 0,
				NotInstalledCount:   3,
				OverallHealthy:      false,
			},
		},
		{
			name: "mixed state - installed but unhealthy and not installed",
			statuses: map[string]*component.ComponentStatus{
				"comp1": {Installed: true, Healthy: true},
				"comp2": {Installed: true, Healthy: false},
				"comp3": {Installed: false, Healthy: false},
			},
			want: StackHealth{
				TotalComponents:     3,
				InstalledComponents: 2,
				HealthyComponents:   1,
				UnhealthyComponents: 1,
				NotInstalledCount:   1,
				OverallHealthy:      false,
			},
		},
		{
			name:     "empty statuses",
			statuses: map[string]*component.ComponentStatus{},
			want: StackHealth{
				TotalComponents:     0,
				InstalledComponents: 0,
				HealthyComponents:   0,
				UnhealthyComponents: 0,
				NotInstalledCount:   0,
				OverallHealthy:      false,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := calculateOverallHealth(tt.statuses)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestDisplayStatusTable(t *testing.T) {
	tests := []struct {
		name     string
		order    []string
		statuses map[string]*component.ComponentStatus
	}{
		{
			name:  "all components healthy",
			order: []string{"openbao", "dns", "zot"},
			statuses: map[string]*component.ComponentStatus{
				"openbao": {
					Installed: true,
					Healthy:   true,
					Version:   "1.0.0",
					Message:   "running",
				},
				"dns": {
					Installed: true,
					Healthy:   true,
					Version:   "49",
					Message:   "running",
				},
				"zot": {
					Installed: true,
					Healthy:   true,
					Version:   "2.0.0",
					Message:   "running",
				},
			},
		},
		{
			name:  "some components not installed",
			order: []string{"openbao", "dns", "zot"},
			statuses: map[string]*component.ComponentStatus{
				"openbao": {
					Installed: true,
					Healthy:   true,
					Version:   "1.0.0",
					Message:   "running",
				},
				"dns": {
					Installed: false,
					Healthy:   false,
					Version:   "",
					Message:   "not installed",
				},
				"zot": {
					Installed: false,
					Healthy:   false,
					Version:   "",
					Message:   "not installed",
				},
			},
		},
		{
			name:  "some components unhealthy",
			order: []string{"openbao", "dns", "zot"},
			statuses: map[string]*component.ComponentStatus{
				"openbao": {
					Installed: true,
					Healthy:   true,
					Version:   "1.0.0",
					Message:   "running",
				},
				"dns": {
					Installed: true,
					Healthy:   false,
					Version:   "49",
					Message:   "service not responding",
				},
				"zot": {
					Installed: true,
					Healthy:   true,
					Version:   "2.0.0",
					Message:   "running",
				},
			},
		},
		{
			name:  "long message truncation",
			order: []string{"openbao"},
			statuses: map[string]*component.ComponentStatus{
				"openbao": {
					Installed: true,
					Healthy:   false,
					Version:   "1.0.0",
					Message:   "this is a very long error message that should be truncated to fit within the display width constraints",
				},
			},
		},
		{
			name:     "nil status entry (should skip)",
			order:    []string{"openbao", "dns"},
			statuses: map[string]*component.ComponentStatus{
				"openbao": {
					Installed: true,
					Healthy:   true,
					Version:   "1.0.0",
					Message:   "running",
				},
				"dns": nil,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// This test just ensures the function doesn't panic
			// In a real scenario, we'd capture stdout and verify output
			assert.NotPanics(t, func() {
				displayStatusTable(tt.order, tt.statuses)
			})
		})
	}
}

func TestDisplayOverallHealth(t *testing.T) {
	tests := []struct {
		name   string
		health StackHealth
	}{
		{
			name: "all healthy",
			health: StackHealth{
				TotalComponents:     3,
				InstalledComponents: 3,
				HealthyComponents:   3,
				UnhealthyComponents: 0,
				NotInstalledCount:   0,
				OverallHealthy:      true,
			},
		},
		{
			name: "some unhealthy",
			health: StackHealth{
				TotalComponents:     3,
				InstalledComponents: 3,
				HealthyComponents:   2,
				UnhealthyComponents: 1,
				NotInstalledCount:   0,
				OverallHealthy:      false,
			},
		},
		{
			name: "none installed",
			health: StackHealth{
				TotalComponents:     3,
				InstalledComponents: 0,
				HealthyComponents:   0,
				UnhealthyComponents: 0,
				NotInstalledCount:   3,
				OverallHealthy:      false,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// This test just ensures the function doesn't panic
			assert.NotPanics(t, func() {
				displayOverallHealth(tt.health)
			})
		})
	}
}

func TestRunStackStatus_ComponentStatusError(t *testing.T) {
	// Test that status errors are handled gracefully
	statuses := map[string]*component.ComponentStatus{
		"comp1": {
			Installed: true,
			Healthy:   true,
			Version:   "1.0.0",
			Message:   "running",
		},
		// Simulate a component that returns an error
		"comp2": {
			Installed: false,
			Healthy:   false,
			Message:   "error querying status: connection refused",
		},
	}

	health := calculateOverallHealth(statuses)
	assert.Equal(t, 2, health.TotalComponents)
	assert.Equal(t, 1, health.InstalledComponents)
	assert.Equal(t, 1, health.NotInstalledCount)
	assert.True(t, health.OverallHealthy) // Only installed component is healthy
}

func TestRunStackStatus_EmptyComponentList(t *testing.T) {
	statuses := map[string]*component.ComponentStatus{}
	health := calculateOverallHealth(statuses)

	assert.Equal(t, 0, health.TotalComponents)
	assert.Equal(t, 0, health.InstalledComponents)
	assert.Equal(t, 0, health.HealthyComponents)
	assert.Equal(t, 0, health.UnhealthyComponents)
	assert.Equal(t, 0, health.NotInstalledCount)
	assert.False(t, health.OverallHealthy)
}

func TestRunStackStatus_AllComponentsUnhealthy(t *testing.T) {
	statuses := map[string]*component.ComponentStatus{
		"comp1": {Installed: true, Healthy: false, Message: "error 1"},
		"comp2": {Installed: true, Healthy: false, Message: "error 2"},
		"comp3": {Installed: true, Healthy: false, Message: "error 3"},
	}

	health := calculateOverallHealth(statuses)
	assert.Equal(t, 3, health.TotalComponents)
	assert.Equal(t, 3, health.InstalledComponents)
	assert.Equal(t, 0, health.HealthyComponents)
	assert.Equal(t, 3, health.UnhealthyComponents)
	assert.False(t, health.OverallHealthy)
}

func TestComponentStatusQuery(t *testing.T) {
	ctx := context.Background()

	tests := []struct {
		name      string
		comp      *mockComponentForStatus
		wantError bool
	}{
		{
			name: "component returns healthy status",
			comp: &mockComponentForStatus{
				name: "test-comp",
				status: &component.ComponentStatus{
					Installed: true,
					Healthy:   true,
					Version:   "1.0.0",
					Message:   "running",
				},
			},
			wantError: false,
		},
		{
			name: "component returns unhealthy status",
			comp: &mockComponentForStatus{
				name: "test-comp",
				status: &component.ComponentStatus{
					Installed: true,
					Healthy:   false,
					Version:   "1.0.0",
					Message:   "degraded",
				},
			},
			wantError: false,
		},
		{
			name: "component returns not installed status",
			comp: &mockComponentForStatus{
				name: "test-comp",
				status: &component.ComponentStatus{
					Installed: false,
					Healthy:   false,
					Version:   "",
					Message:   "not found",
				},
			},
			wantError: false,
		},
		{
			name: "component returns error",
			comp: &mockComponentForStatus{
				name:      "test-comp",
				statusErr: errors.New("connection failed"),
			},
			wantError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			status, err := tt.comp.Status(ctx)
			if tt.wantError {
				assert.Error(t, err)
				assert.Nil(t, status)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, status)
				assert.Equal(t, tt.comp.status, status)
			}
		})
	}
}
