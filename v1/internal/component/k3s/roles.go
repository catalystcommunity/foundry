package k3s

import "fmt"

// NodeRole represents the role(s) a node can have in the cluster
type NodeRole string

const (
	// RoleControlPlane indicates the node is part of the control plane only
	RoleControlPlane NodeRole = "control-plane"
	// RoleWorker indicates the node is a worker only
	RoleWorker NodeRole = "worker"
	// RoleBoth indicates the node is both control-plane and worker
	RoleBoth NodeRole = "both"
)

// NodeConfig represents the configuration for a single node
type NodeConfig struct {
	Hostname     string
	ExplicitRole string // User-specified role (empty string means auto-determine)
}

// DeterminedRole represents a node with its determined role
type DeterminedRole struct {
	Hostname        string
	Role            NodeRole
	IsControlPlane  bool
	IsWorker        bool
	ExplicitlySet   bool // True if role was explicitly set by user
}

// DetermineNodeRoles determines the role for each node based on cluster size and explicit configuration
// Returns a slice of DeterminedRole structs with role information for each node
func DetermineNodeRoles(nodes []NodeConfig) ([]DeterminedRole, error) {
	if len(nodes) == 0 {
		return nil, fmt.Errorf("no nodes provided")
	}

	roles := make([]DeterminedRole, len(nodes))

	for i, node := range nodes {
		role := DeterminedRole{
			Hostname: node.Hostname,
		}

		// If user explicitly specified a role, use it
		if node.ExplicitRole != "" {
			role.ExplicitlySet = true
			switch node.ExplicitRole {
			case "control-plane":
				role.Role = RoleControlPlane
				role.IsControlPlane = true
				role.IsWorker = false
			case "worker":
				role.Role = RoleWorker
				role.IsControlPlane = false
				role.IsWorker = true
			case "both":
				role.Role = RoleBoth
				role.IsControlPlane = true
				role.IsWorker = true
			default:
				return nil, fmt.Errorf("invalid role '%s' for node %s (must be 'control-plane', 'worker', or 'both')", node.ExplicitRole, node.Hostname)
			}
		} else {
			// Auto-determine role based on position and cluster size
			role.ExplicitlySet = false
			nodeRole := determineDefaultRole(i, len(nodes))
			role.Role = nodeRole
			role.IsControlPlane = (nodeRole == RoleControlPlane || nodeRole == RoleBoth)
			role.IsWorker = (nodeRole == RoleWorker || nodeRole == RoleBoth)
		}

		roles[i] = role
	}

	return roles, nil
}

// determineDefaultRole determines the default role for a node based on its index and total cluster size
func determineDefaultRole(index int, totalNodes int) NodeRole {
	switch totalNodes {
	case 1:
		// Single node: control-plane + worker
		return RoleBoth
	case 2:
		// Two nodes: first is both, second is worker
		if index == 0 {
			return RoleBoth
		}
		return RoleWorker
	default:
		// 3+ nodes: first 3 are both, rest are workers
		if index < 3 {
			return RoleBoth
		}
		return RoleWorker
	}
}

// IsControlPlane returns true if the node should be part of the control plane
func IsControlPlane(index int, totalNodes int, explicitRole string) bool {
	if explicitRole != "" {
		return explicitRole == "control-plane" || explicitRole == "both"
	}
	role := determineDefaultRole(index, totalNodes)
	return role == RoleControlPlane || role == RoleBoth
}

// IsWorker returns true if the node should be a worker
func IsWorker(index int, totalNodes int, explicitRole string) bool {
	if explicitRole != "" {
		return explicitRole == "worker" || explicitRole == "both"
	}
	role := determineDefaultRole(index, totalNodes)
	return role == RoleWorker || role == RoleBoth
}
