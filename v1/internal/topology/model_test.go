package topology

import (
	"testing"

	"github.com/catalystcommunity/foundry/v1/internal/config"
	"github.com/catalystcommunity/foundry/v1/internal/host"
	"github.com/catalystcommunity/foundry/v1/internal/setup"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBuildIncludesNetworkVIPAndSortedHosts(t *testing.T) {
	cfg := &config.Config{
		Network: &config.NetworkConfig{Gateway: "192.0.2.1"},
		Cluster: config.ClusterConfig{VIP: "192.0.2.10"},
		Hosts: []*host.Host{
			{Hostname: "worker", Address: "192.0.2.12", Roles: []string{host.RoleClusterWorker}, State: host.StateAdded},
			{Hostname: "control", Address: "192.0.2.11", Roles: []string{host.RoleZot, host.RoleClusterControlPlane}, State: host.StateConfigured},
		},
		SetupState: &setup.SetupState{K8sInstalled: true},
	}

	model := Build(cfg)
	require.Len(t, model.Nodes, 5)
	assert.Equal(t, []string{"internet", "router", "vip", "host:control", "host:worker"}, nodeIDs(model.Nodes))
	assert.Equal(t, HealthHealthy, model.Nodes[2].Health)
	assert.Equal(t, []string{host.RoleClusterControlPlane, host.RoleZot}, model.Nodes[3].Roles)
	assert.Contains(t, model.Guidance, "router to the VIP")
	assert.Contains(t, model.Guidance, "Cloudflare Tunnel")
	assert.Contains(t, model.Edges, Edge{From: "vip", To: "host:control", Kind: "vip-member", Label: "VIP member"})
	assert.NotContains(t, model.Edges, Edge{From: "vip", To: "host:worker", Kind: "vip-member", Label: "VIP member"})
}

func TestBuildHandlesMissingOptionalNetworkState(t *testing.T) {
	model := Build(&config.Config{})
	require.Len(t, model.Nodes, 2)
	assert.Equal(t, HealthPending, model.Nodes[1].Health)
	assert.Empty(t, model.Nodes[1].Address)
}

func nodeIDs(nodes []Node) []string {
	ids := make([]string, 0, len(nodes))
	for _, node := range nodes {
		ids = append(ids, node.ID)
	}
	return ids
}
