package k3s

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDetermineNodeRoles_EmptyNodes(t *testing.T) {
	roles, err := DetermineNodeRoles([]NodeConfig{})
	assert.Error(t, err)
	assert.Nil(t, roles)
	assert.Contains(t, err.Error(), "no nodes provided")
}

func TestDetermineNodeRoles_SingleNode(t *testing.T) {
	nodes := []NodeConfig{
		{Hostname: "node1"},
	}

	roles, err := DetermineNodeRoles(nodes)
	require.NoError(t, err)
	require.Len(t, roles, 1)

	// Single node should be both control-plane and worker
	assert.Equal(t, "node1", roles[0].Hostname)
	assert.Equal(t, RoleBoth, roles[0].Role)
	assert.True(t, roles[0].IsControlPlane)
	assert.True(t, roles[0].IsWorker)
	assert.False(t, roles[0].ExplicitlySet)
}

func TestDetermineNodeRoles_TwoNodes(t *testing.T) {
	nodes := []NodeConfig{
		{Hostname: "node1"},
		{Hostname: "node2"},
	}

	roles, err := DetermineNodeRoles(nodes)
	require.NoError(t, err)
	require.Len(t, roles, 2)

	// First node: both control-plane and worker
	assert.Equal(t, "node1", roles[0].Hostname)
	assert.Equal(t, RoleBoth, roles[0].Role)
	assert.True(t, roles[0].IsControlPlane)
	assert.True(t, roles[0].IsWorker)
	assert.False(t, roles[0].ExplicitlySet)

	// Second node: worker only
	assert.Equal(t, "node2", roles[1].Hostname)
	assert.Equal(t, RoleWorker, roles[1].Role)
	assert.False(t, roles[1].IsControlPlane)
	assert.True(t, roles[1].IsWorker)
	assert.False(t, roles[1].ExplicitlySet)
}

func TestDetermineNodeRoles_ThreeNodes(t *testing.T) {
	nodes := []NodeConfig{
		{Hostname: "node1"},
		{Hostname: "node2"},
		{Hostname: "node3"},
	}

	roles, err := DetermineNodeRoles(nodes)
	require.NoError(t, err)
	require.Len(t, roles, 3)

	// All three nodes should be both control-plane and worker
	for i, role := range roles {
		assert.Equal(t, nodes[i].Hostname, role.Hostname)
		assert.Equal(t, RoleBoth, role.Role)
		assert.True(t, role.IsControlPlane)
		assert.True(t, role.IsWorker)
		assert.False(t, role.ExplicitlySet)
	}
}

func TestDetermineNodeRoles_FiveNodes(t *testing.T) {
	nodes := []NodeConfig{
		{Hostname: "node1"},
		{Hostname: "node2"},
		{Hostname: "node3"},
		{Hostname: "node4"},
		{Hostname: "node5"},
	}

	roles, err := DetermineNodeRoles(nodes)
	require.NoError(t, err)
	require.Len(t, roles, 5)

	// First 3 nodes: both control-plane and worker
	for i := 0; i < 3; i++ {
		assert.Equal(t, nodes[i].Hostname, roles[i].Hostname)
		assert.Equal(t, RoleBoth, roles[i].Role)
		assert.True(t, roles[i].IsControlPlane)
		assert.True(t, roles[i].IsWorker)
		assert.False(t, roles[i].ExplicitlySet)
	}

	// Last 2 nodes: worker only
	for i := 3; i < 5; i++ {
		assert.Equal(t, nodes[i].Hostname, roles[i].Hostname)
		assert.Equal(t, RoleWorker, roles[i].Role)
		assert.False(t, roles[i].IsControlPlane)
		assert.True(t, roles[i].IsWorker)
		assert.False(t, roles[i].ExplicitlySet)
	}
}

func TestDetermineNodeRoles_ExplicitRoles(t *testing.T) {
	tests := []struct {
		name         string
		nodes        []NodeConfig
		expectedErr  bool
		validateFunc func(t *testing.T, roles []DeterminedRole)
	}{
		{
			name: "explicit control-plane only",
			nodes: []NodeConfig{
				{Hostname: "node1", ExplicitRole: "control-plane"},
			},
			expectedErr: false,
			validateFunc: func(t *testing.T, roles []DeterminedRole) {
				assert.Equal(t, RoleControlPlane, roles[0].Role)
				assert.True(t, roles[0].IsControlPlane)
				assert.False(t, roles[0].IsWorker)
				assert.True(t, roles[0].ExplicitlySet)
			},
		},
		{
			name: "explicit worker only",
			nodes: []NodeConfig{
				{Hostname: "node1", ExplicitRole: "worker"},
			},
			expectedErr: false,
			validateFunc: func(t *testing.T, roles []DeterminedRole) {
				assert.Equal(t, RoleWorker, roles[0].Role)
				assert.False(t, roles[0].IsControlPlane)
				assert.True(t, roles[0].IsWorker)
				assert.True(t, roles[0].ExplicitlySet)
			},
		},
		{
			name: "explicit both",
			nodes: []NodeConfig{
				{Hostname: "node1", ExplicitRole: "both"},
			},
			expectedErr: false,
			validateFunc: func(t *testing.T, roles []DeterminedRole) {
				assert.Equal(t, RoleBoth, roles[0].Role)
				assert.True(t, roles[0].IsControlPlane)
				assert.True(t, roles[0].IsWorker)
				assert.True(t, roles[0].ExplicitlySet)
			},
		},
		{
			name: "invalid role",
			nodes: []NodeConfig{
				{Hostname: "node1", ExplicitRole: "invalid"},
			},
			expectedErr: true,
		},
		{
			name: "mixed explicit and auto",
			nodes: []NodeConfig{
				{Hostname: "node1", ExplicitRole: "control-plane"},
				{Hostname: "node2"}, // Auto-determined
				{Hostname: "node3", ExplicitRole: "worker"},
			},
			expectedErr: false,
			validateFunc: func(t *testing.T, roles []DeterminedRole) {
				// node1: explicit control-plane
				assert.Equal(t, RoleControlPlane, roles[0].Role)
				assert.True(t, roles[0].IsControlPlane)
				assert.False(t, roles[0].IsWorker)
				assert.True(t, roles[0].ExplicitlySet)

				// node2: auto-determined (would be "both" in 3-node cluster)
				assert.Equal(t, RoleBoth, roles[1].Role)
				assert.True(t, roles[1].IsControlPlane)
				assert.True(t, roles[1].IsWorker)
				assert.False(t, roles[1].ExplicitlySet)

				// node3: explicit worker
				assert.Equal(t, RoleWorker, roles[2].Role)
				assert.False(t, roles[2].IsControlPlane)
				assert.True(t, roles[2].IsWorker)
				assert.True(t, roles[2].ExplicitlySet)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			roles, err := DetermineNodeRoles(tt.nodes)
			if tt.expectedErr {
				assert.Error(t, err)
				return
			}
			require.NoError(t, err)
			require.Len(t, roles, len(tt.nodes))
			if tt.validateFunc != nil {
				tt.validateFunc(t, roles)
			}
		})
	}
}

func TestIsControlPlane(t *testing.T) {
	tests := []struct {
		name         string
		index        int
		totalNodes   int
		explicitRole string
		expected     bool
	}{
		// 1-node cluster
		{"1-node auto", 0, 1, "", true},
		{"1-node explicit control-plane", 0, 1, "control-plane", true},
		{"1-node explicit worker", 0, 1, "worker", false},
		{"1-node explicit both", 0, 1, "both", true},

		// 2-node cluster
		{"2-node first auto", 0, 2, "", true},
		{"2-node second auto", 1, 2, "", false},
		{"2-node first explicit worker", 0, 2, "worker", false},
		{"2-node second explicit control-plane", 1, 2, "control-plane", true},

		// 3-node cluster
		{"3-node all auto", 0, 3, "", true},
		{"3-node all auto", 1, 3, "", true},
		{"3-node all auto", 2, 3, "", true},

		// 5-node cluster
		{"5-node first auto", 0, 5, "", true},
		{"5-node second auto", 1, 5, "", true},
		{"5-node third auto", 2, 5, "", true},
		{"5-node fourth auto", 3, 5, "", false},
		{"5-node fifth auto", 4, 5, "", false},
		{"5-node fourth explicit control-plane", 3, 5, "control-plane", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := IsControlPlane(tt.index, tt.totalNodes, tt.explicitRole)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestIsWorker(t *testing.T) {
	tests := []struct {
		name         string
		index        int
		totalNodes   int
		explicitRole string
		expected     bool
	}{
		// 1-node cluster
		{"1-node auto", 0, 1, "", true},
		{"1-node explicit control-plane", 0, 1, "control-plane", false},
		{"1-node explicit worker", 0, 1, "worker", true},
		{"1-node explicit both", 0, 1, "both", true},

		// 2-node cluster
		{"2-node first auto", 0, 2, "", true},
		{"2-node second auto", 1, 2, "", true},
		{"2-node first explicit control-plane", 0, 2, "control-plane", false},
		{"2-node second explicit both", 1, 2, "both", true},

		// 3-node cluster
		{"3-node all auto", 0, 3, "", true},
		{"3-node all auto", 1, 3, "", true},
		{"3-node all auto", 2, 3, "", true},

		// 5-node cluster
		{"5-node first auto", 0, 5, "", true},
		{"5-node second auto", 1, 5, "", true},
		{"5-node third auto", 2, 5, "", true},
		{"5-node fourth auto", 3, 5, "", true},
		{"5-node fifth auto", 4, 5, "", true},
		{"5-node first explicit control-plane", 0, 5, "control-plane", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := IsWorker(tt.index, tt.totalNodes, tt.explicitRole)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestDetermineDefaultRole(t *testing.T) {
	tests := []struct {
		name       string
		index      int
		totalNodes int
		expected   NodeRole
	}{
		// 1-node cluster
		{"1-node", 0, 1, RoleBoth},

		// 2-node cluster
		{"2-node first", 0, 2, RoleBoth},
		{"2-node second", 1, 2, RoleWorker},

		// 3-node cluster
		{"3-node first", 0, 3, RoleBoth},
		{"3-node second", 1, 3, RoleBoth},
		{"3-node third", 2, 3, RoleBoth},

		// 4-node cluster
		{"4-node first", 0, 4, RoleBoth},
		{"4-node second", 1, 4, RoleBoth},
		{"4-node third", 2, 4, RoleBoth},
		{"4-node fourth", 3, 4, RoleWorker},

		// 10-node cluster
		{"10-node first", 0, 10, RoleBoth},
		{"10-node second", 1, 10, RoleBoth},
		{"10-node third", 2, 10, RoleBoth},
		{"10-node fourth", 3, 10, RoleWorker},
		{"10-node tenth", 9, 10, RoleWorker},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := determineDefaultRole(tt.index, tt.totalNodes)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// Edge case tests
func TestDetermineNodeRoles_EdgeCases(t *testing.T) {
	t.Run("all nodes explicitly set to same role", func(t *testing.T) {
		nodes := []NodeConfig{
			{Hostname: "node1", ExplicitRole: "worker"},
			{Hostname: "node2", ExplicitRole: "worker"},
			{Hostname: "node3", ExplicitRole: "worker"},
		}

		roles, err := DetermineNodeRoles(nodes)
		require.NoError(t, err)

		// All nodes should be workers as explicitly set
		for _, role := range roles {
			assert.Equal(t, RoleWorker, role.Role)
			assert.False(t, role.IsControlPlane)
			assert.True(t, role.IsWorker)
			assert.True(t, role.ExplicitlySet)
		}
	})

	t.Run("all nodes explicitly set to control-plane", func(t *testing.T) {
		nodes := []NodeConfig{
			{Hostname: "node1", ExplicitRole: "control-plane"},
			{Hostname: "node2", ExplicitRole: "control-plane"},
		}

		roles, err := DetermineNodeRoles(nodes)
		require.NoError(t, err)

		// All nodes should be control-plane as explicitly set
		for _, role := range roles {
			assert.Equal(t, RoleControlPlane, role.Role)
			assert.True(t, role.IsControlPlane)
			assert.False(t, role.IsWorker)
			assert.True(t, role.ExplicitlySet)
		}
	})

	t.Run("large cluster", func(t *testing.T) {
		nodes := make([]NodeConfig, 100)
		for i := 0; i < 100; i++ {
			nodes[i] = NodeConfig{Hostname: "node" + string(rune(i))}
		}

		roles, err := DetermineNodeRoles(nodes)
		require.NoError(t, err)
		require.Len(t, roles, 100)

		// First 3 should be both
		for i := 0; i < 3; i++ {
			assert.Equal(t, RoleBoth, roles[i].Role)
			assert.True(t, roles[i].IsControlPlane)
			assert.True(t, roles[i].IsWorker)
		}

		// Rest should be workers
		for i := 3; i < 100; i++ {
			assert.Equal(t, RoleWorker, roles[i].Role)
			assert.False(t, roles[i].IsControlPlane)
			assert.True(t, roles[i].IsWorker)
		}
	})
}
