# Using Foundry with Tailscale Networks

This guide covers deploying Foundry clusters on Tailscale overlay networks using CGNAT IP addresses (RFC 6598 Shared Address Space, 100.64.0.0/10).

## Overview

Tailscale uses the CGNAT IP range (100.64.0.0/10) for its overlay network, which is outside the traditional RFC 1918 private IP ranges. By default, Foundry's VIP validation only accepts RFC 1918 addresses. The `allow_cgnat_vip` configuration flag enables support for Tailscale and similar overlay networks.

## Prerequisites

- Tailscale installed and configured on all cluster nodes
- Nodes tagged appropriately (e.g., `tag:k8s`)
- Tailscale ACL configured to allow inter-node communication

## Required Tailscale ACL Configuration

Your Tailscale ACL must allow:
1. **Your local machine → cluster nodes** (for Foundry SSH access)
2. **Cluster nodes → cluster nodes** (for K3s cluster formation)

### Example ACL

```json
{
  "acls": [
    {
      "action": "accept",
      "src": ["*"],
      "dst": ["*:*"]
    }
  ],
  "ssh": [
    {
      "action": "accept",
      "src": ["autogroup:members"],
      "dst": ["tag:k8s"],
      "users": ["root", "ubuntu"]
    },
    {
      "action": "accept",
      "src": ["tag:k8s"],
      "dst": ["tag:k8s"],
      "users": ["root"]
    }
  ],
  "tagOwners": {
    "tag:k8s": ["autogroup:admin"]
  }
}
```

**Critical:** The second SSH rule (`tag:k8s` → `tag:k8s`) allows cluster nodes to SSH to each other, which is required for K3s agent installation on worker nodes.

## Configuration

### Single Control Plane Setup

**IMPORTANT:** The VIP must always be a separate, dedicated IP address that is not assigned to any host. This is required because kube-vip manages the VIP through ARP advertisements, and having the VIP match a host's actual IP can cause network conflicts and packet loss. Foundry enforces this: `allow_cgnat_vip` widens the accepted range to CGNAT, but the VIP still may not equal any host's address.

For Tailscale deployments, use a CGNAT IP in the 100.64.0.0/10 range that:
- Is NOT assigned to any of your cluster nodes
- Is within your Tailscale network's IP range
- Will be advertised as a subnet route by the Tailscale operator

### Setup Steps

For both single and multi-control-plane setups, the process is the same.

#### Step 1: Configure Foundry with a Dedicated VIP

Choose a CGNAT IP for your VIP that is NOT assigned to any node:

```yaml
cluster:
  name: my-cluster
  primary_domain: example.local
  vip: 100.81.89.100  # Dedicated VIP (not assigned to any host)
  allow_cgnat_vip: true

hosts:
  - hostname: control-plane
    address: 100.81.89.62  # Control plane's Tailscale IP
    user: root
  - hostname: worker-1
    address: 100.70.90.12
    user: root
  - hostname: worker-2
    address: 100.125.196.1
    user: root
```

**Important:** The VIP must be different from any host's IP address. You must advertise the VIP as a subnet route from the control plane:

```bash
# On the control plane node
tailscale up --advertise-routes=100.81.89.100/32
```

Then approve the route in the Tailscale admin console.

### High Availability (Multi-Control-Plane) Setup

For HA setups with multiple control planes, you need to make the VIP routable via Tailscale:

#### Option 1: Tailscale Subnet Routes

Advertise the VIP as a subnet route from the active control plane:

```bash
# On the control plane node
tailscale up --advertise-routes=100.81.89.100/32
```

Then approve the route in the Tailscale admin console.

```yaml
cluster:
  name: my-cluster
  primary_domain: example.local
  vip: 100.81.89.100  # Dedicated VIP
  allow_cgnat_vip: true
```

**Note:** kube-vip will manage the VIP assignment, but you need to ensure the route is advertised from whichever node currently holds the VIP.

#### Option 2: Tailscale Operator (Recommended for HA)

The Tailscale Operator integration will be available in a future Foundry release. This will provide:
- Automatic operator installation on control planes
- Automated VIP subnet route management
- Support for cross-pod network policies via Tailscale ACLs

For now, use Option 1 (Subnet Routes) for HA setups.

## Network Routing Considerations

### Understanding VIP Routing on Tailscale

Traditional kube-vip assumes Layer 2 networking where the VIP can "float" between nodes via ARP announcements. Tailscale is a Layer 3 overlay network where:

- **IPs are routed, not bridged** - Nodes communicate via Tailscale's WireGuard tunnels
- **No ARP** - IP routing is managed by Tailscale's coordination server
- **Explicit routes required** - Any IP that isn't a node's primary Tailscale IP needs to be advertised as a subnet route

### VIP Reachability

For worker nodes to reach the VIP:

**Single control plane:**
- VIP = control plane IP → Always routable (it's the node's primary IP)

**Multiple control planes:**
- VIP = dedicated IP → Must be advertised as subnet route
- Route must be updated when VIP moves between control planes
- Tailscale operator can automate this

## Troubleshooting

### Workers Can't Join Cluster

**Symptom:**
```
Failed to validate connection to cluster at https://100.81.89.100:6443:
failed to get CA certs: context deadline exceeded
```

**Diagnosis:**
Worker nodes cannot reach the VIP. Check:

```bash
# On a worker node
curl -k https://<VIP>:6443/version --max-time 5

# If it times out, the VIP is not routable
```

**Solution:**
- Single control plane: Advertise VIP as subnet route from control plane
- Multi control plane: Advertise VIP as subnet route from active control plane

### SSH Connection Refused Between Nodes

**Symptom:**
```
tailscale: tailnet policy does not permit you to SSH to this node
```

**Diagnosis:**
Tailscale ACL doesn't allow SSH between cluster nodes.

**Solution:**
Add SSH rule allowing `tag:k8s` → `tag:k8s` as shown in the ACL example above.

### VIP Assigned But Not Reachable

**Symptom:**
- `ip addr show` on control plane shows VIP assigned
- Workers still can't reach it

**Diagnosis:**
VIP is assigned to the local interface but not advertised to Tailscale.

**Solution:**
```bash
# On control plane
tailscale up --advertise-routes=<VIP>/32

# Then approve in Tailscale admin console
```

## Validation Checklist

Before deploying:

- [ ] All nodes have Tailscale installed and connected
- [ ] Nodes are tagged appropriately (e.g., `tag:k8s`)
- [ ] Tailscale ACL allows SSH from your machine to nodes
- [ ] Tailscale ACL allows SSH between nodes (`tag:k8s` → `tag:k8s`)
- [ ] For HA setups: VIP subnet route is configured and approved
- [ ] `allow_cgnat_vip: true` is set in cluster config
- [ ] Workers can reach the VIP: `curl -k https://<VIP>:6443/version`

## Roadmap

Future enhancements planned for Tailscale integration:

1. **Tailscale Operator Integration**
   - Automatic operator installation on control planes
   - Automated VIP subnet route management
   - Support for cross-pod network policies via Tailscale ACLs

2. **Multi-Cluster Mesh**
   - Connect multiple Foundry clusters via Tailscale
   - Cross-cluster service discovery
   - Unified network policy across clusters

3. **GitOps for Tailscale ACLs**
   - Version control for network policies
   - CI/CD automation for ACL updates
   - Integration with Foundry stack management

## Testing (local)

The CGNAT VIP validation added here is covered by unit tests and needs no
cluster. From the repo root:

```bash
# Build, vet, and run all unit tests (excludes the Docker integration suite)
scripts/test-local.sh

# Just the VIP / k3s validation this PR touches
PKG=./internal/component/k3s/... scripts/test-local.sh
```

`scripts/test-local.sh --kind` spins up a throwaway kind cluster; later PRs in
the Tailscale stack use it to dry-run-apply their generated manifests against a
live API server. See `scripts/README.md` for all modes.

## References

- [RFC 6598 - Shared Address Space (CGNAT)](https://www.rfc-editor.org/rfc/rfc6598)
- [Tailscale ACL Documentation](https://tailscale.com/kb/1018/acls/)
- [Tailscale Subnet Routes](https://tailscale.com/kb/1019/subnets/)
- [kube-vip Documentation](https://kube-vip.io/)

## Contributing

Found an issue or have suggestions for Tailscale integration? Please open an issue on the [Foundry GitHub repository](https://github.com/catalystcommunity/foundry).