// Package topology builds a presentation-neutral graph of a Foundry stack.
package topology

import (
	"sort"

	"github.com/catalystcommunity/foundry/v1/internal/config"
	"github.com/catalystcommunity/foundry/v1/internal/host"
)

// Health is the summarized state of a topology node.
type Health string

const (
	HealthHealthy Health = "healthy"
	HealthWarning Health = "warning"
	HealthPending Health = "pending"
	HealthUnknown Health = "unknown"
)

// Node is one item in the stack graph.
type Node struct {
	ID          string   `json:"id"`
	Kind        string   `json:"kind"`
	Label       string   `json:"label"`
	Address     string   `json:"address,omitempty"`
	Roles       []string `json:"roles,omitempty"`
	Health      Health   `json:"health"`
	HealthLabel string   `json:"health_label"`
}

// Edge connects two graph nodes.
type Edge struct {
	From  string `json:"from"`
	To    string `json:"to"`
	Kind  string `json:"kind"`
	Label string `json:"label,omitempty"`
}

// Model is the graph rendered by the web dashboard.
type Model struct {
	Nodes    []Node `json:"nodes"`
	Edges    []Edge `json:"edges"`
	Guidance string `json:"guidance"`
}

// Build creates a deterministic topology graph from a stack configuration.
func Build(cfg *config.Config) Model {
	model := Model{
		Guidance: "To publish a service, forward only the ports you need from your router to the VIP. You can also configure a tunnel or reverse proxy, such as Cloudflare Tunnel. Foundry does not configure these proxies yet. It will support more options over time.",
		Nodes: []Node{
			{ID: "internet", Kind: "internet", Label: "Internet", Health: HealthUnknown, HealthLabel: "External network"},
		},
	}

	routerAddress := ""
	if cfg.Network != nil {
		routerAddress = cfg.Network.Gateway
	}
	model.Nodes = append(model.Nodes, Node{
		ID: "router", Kind: "router", Label: "Router", Address: routerAddress,
		Health: healthForValue(routerAddress), HealthLabel: labelForValue(routerAddress, "Configured", "Not configured"),
	})
	model.Edges = append(model.Edges, Edge{From: "internet", To: "router", Kind: "network"})

	if cfg.Cluster.VIP != "" {
		vipHealth := HealthPending
		vipLabel := "Not installed"
		if cfg.SetupState != nil && cfg.SetupState.K8sInstalled {
			vipHealth = HealthHealthy
			vipLabel = "Kubernetes installed"
		}
		model.Nodes = append(model.Nodes, Node{
			ID: "vip", Kind: "vip", Label: "Kubernetes VIP", Address: cfg.Cluster.VIP,
			Health: vipHealth, HealthLabel: vipLabel,
		})
		model.Edges = append(model.Edges, Edge{From: "router", To: "vip", Kind: "virtual", Label: "Optional forwarded traffic"})
	}

	hosts := append([]*host.Host(nil), cfg.Hosts...)
	sort.Slice(hosts, func(i, j int) bool { return hosts[i].Hostname < hosts[j].Hostname })
	for _, configuredHost := range hosts {
		if configuredHost == nil {
			continue
		}
		nodeID := "host:" + configuredHost.Hostname
		roles := append([]string(nil), configuredHost.Roles...)
		sort.Strings(roles)
		health, healthLabel := hostHealth(configuredHost)
		model.Nodes = append(model.Nodes, Node{
			ID: nodeID, Kind: "host", Label: configuredHost.Hostname, Address: configuredHost.Address,
			Roles: roles, Health: health, HealthLabel: healthLabel,
		})
		model.Edges = append(model.Edges, Edge{From: "router", To: nodeID, Kind: "network"})
		if cfg.Cluster.VIP != "" && configuredHost.HasRole(host.RoleClusterControlPlane) {
			model.Edges = append(model.Edges, Edge{From: "vip", To: nodeID, Kind: "vip-member", Label: "VIP member"})
		}
	}

	return model
}

func hostHealth(configuredHost *host.Host) (Health, string) {
	switch configuredHost.State {
	case host.StateConfigured:
		return HealthHealthy, "Configured"
	case host.StateSSHConfigured:
		return HealthWarning, "SSH configured"
	case host.StateAdded:
		return HealthPending, "Key setup pending"
	default:
		return HealthUnknown, "Health not checked"
	}
}

func healthForValue(value string) Health {
	if value == "" {
		return HealthPending
	}
	return HealthHealthy
}

func labelForValue(value, present, missing string) string {
	if value == "" {
		return missing
	}
	return present
}
