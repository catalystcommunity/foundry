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

See `validateK8sVIPUniqueness()` in v1/internal/config/types.go and "Understanding VIP Routing on Tailscale" below for details.

For Tailscale deployments, use a CGNAT IP in the 100.64.0.0/10 range that:
- Is NOT assigned to any of your cluster nodes
- Is within your Tailscale network's IP range
- Will be advertised as a subnet route by the Tailscale operator

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

#### Option 2: Tailscale Operator (Not Available)

The Tailscale Operator integration will be available in a future Foundry release. This will provide:
- Automatic operator installation on control planes
- Automated VIP subnet route management
- Support for cross-pod network policies via Tailscale ACLs

For now, use Option 1 (Subnet Routes) for HA setups.

## Tailscale Ingress

Foundry does not install the Tailscale Kubernetes Operator. Thus, Foundry does
not support the Tailscale Ingress controller. Do not set
`ingressClassName: tailscale` in a Foundry stack. Use the current Foundry
Contour configuration, or manage the Tailscale Operator outside Foundry.

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

## Tailscale Operator Status

Foundry does not install the Tailscale Kubernetes Operator. Foundry supports a
manual subnet route for the cluster VIP. The following functions are not
available:

| Function | Status |
|----------|--------|
| Access to the API server through an operator proxy | Not available |
| Service access through a Tailscale IngressClass | Not available |
| Public service access through Funnel | Not available |
| Pod access to services in the tailnet | Not available |
| Automatic subnet routes | Not available |
| Cluster exit nodes | Not available |
| Connections between clusters | Not available |
| Load distribution between clusters | Not available |

## Roadmap

The plan includes the following work:

1. **Tailscale Operator Integration**
   - Install the operator on control-plane hosts.
   - Manage the VIP subnet route.
   - Apply Tailscale ACLs to traffic between pods.

2. **Multi-Cluster Mesh**
   - Connect Foundry clusters through Tailscale.
   - Find services in other clusters.
   - Apply one network policy to multiple clusters.

3. **GitOps for Tailscale ACLs**
   - Store network policies in version control.
   - Use CI/CD to update ACLs.
   - Manage ACLs through Foundry stack operations.

## References

- [RFC 6598 - Shared Address Space (CGNAT)](https://www.rfc-editor.org/rfc/rfc6598)
- [Tailscale ACL Documentation](https://tailscale.com/kb/1018/acls/)
- [Tailscale Subnet Routes](https://tailscale.com/kb/1019/subnets/)
- [kube-vip Documentation](https://kube-vip.io/)

## Contributing

Found an issue or have suggestions for Tailscale integration? Please open an issue on the [Foundry GitHub repository](https://github.com/catalystcommunity/foundry).
