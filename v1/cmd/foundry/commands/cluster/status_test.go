package cluster

import (
	"testing"

	"github.com/catalystcommunity/foundry/v1/internal/k8s"
	"github.com/stretchr/testify/assert"
)

func TestNewStatusCommand(t *testing.T) {
	cmd := NewStatusCommand()

	assert.NotNil(t, cmd)
	assert.Equal(t, "status", cmd.Name)
	assert.NotEmpty(t, cmd.Usage)
	assert.NotNil(t, cmd.Action)
}

func TestCalculateClusterHealth(t *testing.T) {
	tests := []struct {
		name           string
		nodes          []*k8s.Node
		expectedHealth *ClusterHealth
	}{
		{
			name:  "empty cluster",
			nodes: []*k8s.Node{},
			expectedHealth: &ClusterHealth{
				TotalNodes:     0,
				OverallHealthy: false,
				HealthMessage:  "No nodes found in cluster",
			},
		},
		{
			name: "single healthy control-plane+worker node",
			nodes: []*k8s.Node{
				{
					Name:    "node1",
					Status:  "Ready",
					Roles:   []string{"control-plane", "worker"},
					Version: "v1.28.5+k3s1",
				},
			},
			expectedHealth: &ClusterHealth{
				TotalNodes:        1,
				ControlPlaneNodes: 1,
				WorkerNodes:       1,
				ReadyNodes:        1,
				NotReadyNodes:     0,
				Version:           "v1.28.5+k3s1",
				OverallHealthy:    true,
				HealthMessage:     "All nodes ready",
			},
		},
		{
			name: "three node cluster - all healthy",
			nodes: []*k8s.Node{
				{
					Name:    "node1",
					Status:  "Ready",
					Roles:   []string{"control-plane", "worker"},
					Version: "v1.28.5+k3s1",
				},
				{
					Name:    "node2",
					Status:  "Ready",
					Roles:   []string{"control-plane", "worker"},
					Version: "v1.28.5+k3s1",
				},
				{
					Name:    "node3",
					Status:  "Ready",
					Roles:   []string{"worker"},
					Version: "v1.28.5+k3s1",
				},
			},
			expectedHealth: &ClusterHealth{
				TotalNodes:        3,
				ControlPlaneNodes: 2,
				WorkerNodes:       3,
				ReadyNodes:        3,
				NotReadyNodes:     0,
				Version:           "v1.28.5+k3s1",
				OverallHealthy:    true,
				HealthMessage:     "All nodes ready",
			},
		},
		{
			name: "cluster with not ready node",
			nodes: []*k8s.Node{
				{
					Name:    "node1",
					Status:  "Ready",
					Roles:   []string{"control-plane", "worker"},
					Version: "v1.28.5+k3s1",
				},
				{
					Name:    "node2",
					Status:  "NotReady",
					Roles:   []string{"worker"},
					Version: "v1.28.5+k3s1",
				},
			},
			expectedHealth: &ClusterHealth{
				TotalNodes:        2,
				ControlPlaneNodes: 1,
				WorkerNodes:       2,
				ReadyNodes:        1,
				NotReadyNodes:     1,
				Version:           "v1.28.5+k3s1",
				OverallHealthy:    false,
				HealthMessage:     "1 node(s) not ready",
			},
		},
		{
			name: "cluster with only worker nodes (no control plane)",
			nodes: []*k8s.Node{
				{
					Name:    "node1",
					Status:  "Ready",
					Roles:   []string{"worker"},
					Version: "v1.28.5+k3s1",
				},
			},
			expectedHealth: &ClusterHealth{
				TotalNodes:        1,
				ControlPlaneNodes: 0,
				WorkerNodes:       1,
				ReadyNodes:        1,
				NotReadyNodes:     0,
				Version:           "v1.28.5+k3s1",
				OverallHealthy:    false,
				HealthMessage:     "No control plane nodes found",
			},
		},
		{
			name: "five node cluster - mixed",
			nodes: []*k8s.Node{
				{
					Name:    "node1",
					Status:  "Ready",
					Roles:   []string{"control-plane", "worker"},
					Version: "v1.28.5+k3s1",
				},
				{
					Name:    "node2",
					Status:  "Ready",
					Roles:   []string{"control-plane", "worker"},
					Version: "v1.28.5+k3s1",
				},
				{
					Name:    "node3",
					Status:  "Ready",
					Roles:   []string{"control-plane", "worker"},
					Version: "v1.28.5+k3s1",
				},
				{
					Name:    "node4",
					Status:  "Ready",
					Roles:   []string{"worker"},
					Version: "v1.28.5+k3s1",
				},
				{
					Name:    "node5",
					Status:  "NotReady",
					Roles:   []string{"worker"},
					Version: "v1.28.5+k3s1",
				},
			},
			expectedHealth: &ClusterHealth{
				TotalNodes:        5,
				ControlPlaneNodes: 3,
				WorkerNodes:       5,
				ReadyNodes:        4,
				NotReadyNodes:     1,
				Version:           "v1.28.5+k3s1",
				OverallHealthy:    false,
				HealthMessage:     "1 node(s) not ready",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			health := calculateClusterHealth(tt.nodes)

			assert.Equal(t, tt.expectedHealth.TotalNodes, health.TotalNodes)
			assert.Equal(t, tt.expectedHealth.ControlPlaneNodes, health.ControlPlaneNodes)
			assert.Equal(t, tt.expectedHealth.WorkerNodes, health.WorkerNodes)
			assert.Equal(t, tt.expectedHealth.ReadyNodes, health.ReadyNodes)
			assert.Equal(t, tt.expectedHealth.NotReadyNodes, health.NotReadyNodes)
			assert.Equal(t, tt.expectedHealth.Version, health.Version)
			assert.Equal(t, tt.expectedHealth.OverallHealthy, health.OverallHealthy)
			assert.Equal(t, tt.expectedHealth.HealthMessage, health.HealthMessage)
		})
	}
}

func TestDisplayClusterStatus(t *testing.T) {
	// This test just ensures displayClusterStatus doesn't panic
	// Actual output formatting is tested manually
	health := &ClusterHealth{
		TotalNodes:        3,
		ControlPlaneNodes: 1,
		WorkerNodes:       3,
		ReadyNodes:        3,
		NotReadyNodes:     0,
		Version:           "v1.28.5+k3s1",
		OverallHealthy:    true,
		HealthMessage:     "All nodes ready",
	}

	// Should not panic
	assert.NotPanics(t, func() {
		displayClusterStatus(health)
	})
}
