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

<<<<<<< HEAD
### VIP Requirements

**IMPORTANT:** The VIP must always be a separate, dedicated IP address that is not assigned to any host. This is required because kube-vip manages the VIP through ARP advertisements, and having the VIP match a host's actual IP can cause network conflicts and packet loss.

For Tailscale deployments, use a CGNAT IP in the 100.64.0.0/10 range that:
- Is NOT assigned to any of your cluster nodes
- Is within your Tailscale network's IP range
- Will be advertised as a subnet route by the Tailscale operator

### Setup Steps

For both single and multi-control-plane setups, the process is the same:

#### Step 1: Configure Foundry with a Dedicated VIP

Choose a CGNAT IP for your VIP that is NOT assigned to any node:
=======
### Single Control Plane Setup

For single control plane deployments, use a dedicated VIP address that is routable via Tailscale:
>>>>>>> 9edde40 (feat: add CGNAT VIP support and Tailscale integration)

```yaml
cluster:
  name: my-cluster
  primary_domain: example.local
<<<<<<< HEAD
  vip: 100.81.89.100  # Dedicated VIP (not a node IP!)
=======
  vip: 100.81.89.100  # Dedicated VIP (not assigned to any host)
>>>>>>> 9edde40 (feat: add CGNAT VIP support and Tailscale integration)
  allow_cgnat_vip: true

hosts:
  - hostname: control-plane
<<<<<<< HEAD
    address: 100.81.89.62  # Different from VIP
=======
    address: 100.81.89.62  # Control plane's Tailscale IP
>>>>>>> 9edde40 (feat: add CGNAT VIP support and Tailscale integration)
    user: root
  - hostname: worker-1
    address: 100.70.90.12
    user: root
  - hostname: worker-2
    address: 100.125.196.1
    user: root
```

<<<<<<< HEAD
#### Step 2: Deploy the Cluster

Run Foundry to deploy K3s and kube-vip:

```bash
foundry stack install
```

#### Step 3: Install Tailscale Operator

After the cluster is running, install the Tailscale operator to manage subnet route advertisements:

```bash
# Add Tailscale Helm repository
helm repo add tailscale https://pkgs.tailscale.com/helmcharts
helm repo update

# Install the operator
helm install tailscale-operator tailscale/tailscale-operator \
  --namespace=tailscale \
  --create-namespace \
  --set-string oauth.clientId=${TS_OAUTH_CLIENT_ID} \
  --set-string oauth.clientSecret=${TS_OAUTH_CLIENT_SECRET} \
  --wait
```

**Prerequisites:**
- Create OAuth credentials in Tailscale admin console: https://tailscale.com/kb/1236/kubernetes-operator
- Set environment variables `TS_OAUTH_CLIENT_ID` and `TS_OAUTH_CLIENT_SECRET`

#### Step 4: Configure ProxyClass for VIP Route

Create a ProxyClass to advertise the VIP as a subnet route:

```yaml
apiVersion: tailscale.com/v1alpha1
kind: ProxyClass
metadata:
  name: vip-advertiser
spec:
  statefulSet:
    labels:
      tailscale-vip: "true"
    pod:
      tailscaleContainer:
        env:
          - name: TS_ROUTES
            value: "100.81.89.100/32"
```

Apply it:

```bash
kubectl apply -f proxyclass.yaml
```

This will automatically advertise your VIP subnet route to Tailscale, making it reachable from all nodes.
=======
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
>>>>>>> 9edde40 (feat: add CGNAT VIP support and Tailscale integration)

## Network Routing Considerations

### Understanding VIP Routing on Tailscale

Traditional kube-vip assumes Layer 2 networking where the VIP can "float" between nodes via ARP announcements. Tailscale is a Layer 3 overlay network where:

- **IPs are routed, not bridged** - Nodes communicate via Tailscale's WireGuard tunnels
- **No ARP** - IP routing is managed by Tailscale's coordination server
- **Explicit routes required** - Any IP that isn't a node's primary Tailscale IP needs to be advertised as a subnet route

### VIP Reachability

For worker nodes to reach the VIP:

<<<<<<< HEAD
**All deployments (single or multi-control-plane):**
- VIP must be a dedicated IP, separate from any node's IP
- VIP must be advertised as a subnet route via Tailscale
- Tailscale operator automates route management as VIP moves between nodes
- kube-vip handles VIP assignment and failover via ARP (local to each node)
=======
**Single control plane:**
- VIP = control plane IP → Always routable (it's the node's primary IP)

**Multiple control planes:**
- VIP = dedicated IP → Must be advertised as subnet route
- Route must be updated when VIP moves between control planes
- Tailscale operator can automate this
>>>>>>> 9edde40 (feat: add CGNAT VIP support and Tailscale integration)

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
<<<<<<< HEAD
- Ensure VIP is advertised as a subnet route via Tailscale operator
- Verify ProxyClass is configured correctly with the VIP route
- Check that the route is approved in Tailscale admin console
=======
- Single control plane: Advertise VIP as subnet route from control plane
- Multi control plane: Advertise VIP as subnet route from active control plane
>>>>>>> 9edde40 (feat: add CGNAT VIP support and Tailscale integration)

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
<<<<<<< HEAD
# Check if Tailscale operator is running
kubectl get pods -n tailscale

# Check if ProxyClass is configured
kubectl get proxyclass

# Verify the route is being advertised
kubectl logs -n tailscale -l tailscale-vip=true

# If operator is not installed, install it following Step 3 above
=======
# On control plane
tailscale up --advertise-routes=<VIP>/32

# Then approve in Tailscale admin console
>>>>>>> 9edde40 (feat: add CGNAT VIP support and Tailscale integration)
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

<<<<<<< HEAD
1. **Automated Tailscale Operator Installation**
   - Automatic operator installation during cluster setup
   - Auto-generated OAuth credentials integration
   - Automated ProxyClass configuration
=======
1. **Tailscale Operator Integration**
   - Automatic operator installation on control planes
   - Automated VIP subnet route management
   - Support for cross-pod network policies via Tailscale ACLs
>>>>>>> 9edde40 (feat: add CGNAT VIP support and Tailscale integration)

2. **Multi-Cluster Mesh**
   - Connect multiple Foundry clusters via Tailscale
   - Cross-cluster service discovery
   - Unified network policy across clusters

3. **GitOps for Tailscale ACLs**
   - Version control for network policies
   - CI/CD automation for ACL updates
   - Integration with Foundry stack management

## References

- [RFC 6598 - Shared Address Space (CGNAT)](https://www.rfc-editor.org/rfc/rfc6598)
- [Tailscale ACL Documentation](https://tailscale.com/kb/1018/acls/)
- [Tailscale Subnet Routes](https://tailscale.com/kb/1019/subnets/)
- [kube-vip Documentation](https://kube-vip.io/)

## Contributing

Found an issue or have suggestions for Tailscale integration? Please open an issue on the [Foundry GitHub repository](https://github.com/catalystcommunity/foundry).
