# Using Foundry with Tailscale Networks

This guide covers deploying Foundry clusters on Tailscale overlay networks using CGNAT IP addresses (RFC 6598 Shared Address Space, 100.64.0.0/10) with automated Tailscale operator integration.

## Overview

Tailscale uses the CGNAT IP range (100.64.0.0/10) for its overlay network, which is outside the traditional RFC 1918 private IP ranges. Foundry provides two levels of Tailscale support:

1. **Basic CGNAT Support** (`allow_cgnat_vip: true`) - Enables using Tailscale IPs for cluster VIPs
2. **Automated Operator Integration** (`use_tailscale: true`) - Automatically deploys and configures the Tailscale operator

## Prerequisites

### For Basic CGNAT Support
- Tailscale installed and configured on all cluster nodes
- Nodes tagged appropriately (e.g., `tag:k8s`)
- Tailscale ACL configured to allow inter-node communication

### For Automated Operator Integration
- OAuth client credentials from Tailscale (see Setup section)
- OpenBAO configured for secret storage (recommended)
- All basic prerequisites above

## Tailscale OAuth Client Setup

The automated operator integration requires OAuth credentials:

1. **Create OAuth Client**:
   - Go to: https://login.tailscale.com/admin/settings/oauth
   - Click "Generate OAuth Client"
   - Name: `foundry-cluster-<name>`
   - Scopes (minimum required):
     - `devices:write` - Create and manage devices
     - `routes:write` - Advertise subnet routes
   - Save the `client_id` (starts with `tskey-client-`)
   - Save the `client_secret` (starts with `tskey-secret-`)

2. **Store Credentials Securely**:

   **Option A: OpenBAO (Recommended for Production)**
   ```bash
   foundry openbao write foundry-core/tailscale \
     client_id="<YOUR_CLIENT_ID>" \
     client_secret="<YOUR_CLIENT_SECRET>"
   ```

   **Option B: Literal Values (Development/Testing Only)**
   - Use credentials directly in configuration (not recommended for production)

## Required Tailscale ACL Configuration

Your Tailscale ACL must allow:
1. **Your local machine → cluster nodes** (for Foundry SSH access)
2. **Cluster nodes → cluster nodes** (for K3s cluster formation)
3. **Cluster pods → Tailscale network** (via operator)

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
    "tag:k8s": ["autogroup:admin"],
    "tag:k8s-foundry": ["autogroup:admin"]
  }
}
```

**Critical:** The second SSH rule (`tag:k8s` → `tag:k8s`) allows cluster nodes to SSH to each other, which is required for K3s agent installation on worker nodes.

## Configuration

### Single Control Plane with Basic CGNAT Support

For single control plane deployments without the operator:

```yaml
cluster:
  name: my-cluster
  primary_domain: example.local
  vip: 100.81.89.62  # Control plane's Tailscale IP
  allow_cgnat_vip: true

hosts:
  - hostname: control-plane
    address: 100.81.89.62
    user: root
  - hostname: worker-1
    address: 100.70.90.12
    user: root
```

**Why this works:**
- Single control plane means no HA failover needed
- VIP is just a stable endpoint for workers to connect to
- Using the control plane's actual IP avoids routing complexity

### High Availability with Automated Operator (Recommended)

For HA setups with automated Tailscale integration:

```yaml
cluster:
  name: my-cluster
  primary_domain: example.local
  vip: 100.81.89.100  # Dedicated VIP
  allow_cgnat_vip: true
  use_tailscale: true  # Enable automated operator

components:
  tailscale:
    # OAuth credentials from OpenBAO
    oauth_client_id: ${secret:foundry-core/tailscale:client_id}
    oauth_client_secret: ${secret:foundry-core/tailscale:client_secret}

    # Optional: Custom tags for ACL policies
    tags:
      - tag:k8s-foundry
      - tag:production

    # Optional: Additional routes to advertise
    advertise_routes:
      - 10.0.0.0/8

hosts:
  - hostname: control-1
    address: 100.81.89.62
    user: root
  - hostname: control-2
    address: 100.81.89.63
    user: root
  - hostname: worker-1
    address: 100.70.90.12
    user: root
```

### Using Literal Credentials (Development Only)

For development/testing without OpenBAO:

```yaml
cluster:
  name: dev-cluster
  vip: 100.81.89.100
  allow_cgnat_vip: true
  use_tailscale: true

components:
  tailscale:
    # Direct credentials (NOT recommended for production)
    oauth_client_id: tskey-client-abc123def456
    oauth_client_secret: tskey-secret-xyz789abc123
```

## What Gets Deployed with `use_tailscale: true`

When you enable automated operator integration, Foundry:

1. **Creates Namespace**: `tailscale` namespace for operator resources
2. **Installs Operator**: Deploys Tailscale operator via Helm
3. **Configures Connector**: Creates Connector CRD to advertise VIP as subnet route
4. **Enables Magic DNS**: Deploys DNSConfig CRD for Tailscale hostname resolution
5. **Patches CoreDNS**: Configures CoreDNS to forward `.ts.net` queries to Tailscale DNS

### Automatic VIP Route Advertisement

The operator automatically advertises your cluster VIP as a Tailscale subnet route, eliminating the need for manual `tailscale up --advertise-routes` commands.

Routes advertised:
- Cluster VIP (e.g., `100.81.89.100/32`)
- Any additional routes in `advertise_routes` config

## Verification

After deployment, verify the Tailscale integration:

```bash
# Check operator pods
kubectl get pods -n tailscale

# Check Connector status (shows advertised routes)
kubectl get connector -n tailscale -o yaml

# Check DNSConfig
kubectl get dnsconfig -n tailscale

# Verify VIP is reachable from workers
curl -k https://<VIP>:6443/version --max-time 5

# Test Magic DNS from a pod
kubectl run test --image=nicolaka/netshoot -it --rm -- nslookup mydevice.your-tailnet.ts.net
```

## Network Routing Considerations

### Understanding VIP Routing on Tailscale

Traditional kube-vip assumes Layer 2 networking where the VIP can "float" between nodes via ARP announcements. Tailscale is a Layer 3 overlay network where:

- **IPs are routed, not bridged** - Nodes communicate via Tailscale's WireGuard tunnels
- **No ARP** - IP routing is managed by Tailscale's coordination server
- **Explicit routes required** - Any IP that isn't a node's primary Tailscale IP needs to be advertised as a subnet route

### VIP Reachability

**With automated operator (`use_tailscale: true`):**
- Operator automatically advertises VIP via Connector CRD
- Routes update dynamically as VIP moves between control planes
- No manual intervention needed

**Without operator:**
- Single control plane: VIP = control plane IP → Always routable
- Multiple control planes: Must manually advertise VIP as subnet route

## Troubleshooting

### Operator Not Starting

**Symptom:**
```bash
kubectl get pods -n tailscale
# Shows operator pod in CrashLoopBackOff
```

**Diagnosis:**
Check operator logs:
```bash
kubectl logs -n tailscale -l app=tailscale-operator
```

**Common issues:**
- Invalid OAuth credentials
- OAuth client missing required scopes (`devices:write`, `routes:write`)
- Network connectivity to Tailscale coordination server

**Solution:**
```bash
# Verify credentials in OpenBAO
foundry openbao read foundry-core/tailscale

# Update if needed
foundry openbao write foundry-core/tailscale \
  client_id="<CORRECT_CLIENT_ID>" \
  client_secret="<CORRECT_CLIENT_SECRET>"

# Restart operator
kubectl rollout restart deployment -n tailscale tailscale-operator
```

### VIP Not Advertised

**Symptom:**
Workers can't reach VIP after operator installation.

**Diagnosis:**
```bash
# Check Connector status
kubectl get connector -n tailscale foundry-vip-connector -o yaml

# Look for status conditions
```

**Solution:**
- Verify Connector created: `kubectl get connector -n tailscale`
- Check operator logs: `kubectl logs -n tailscale -l app=tailscale-operator`
- Ensure OAuth client has `routes:write` scope
- Check Tailscale admin console for route approval requirements

### DNS Resolution Not Working

**Symptom:**
Pods can't resolve `.ts.net` hostnames.

**Diagnosis:**
```bash
# Check DNSConfig
kubectl get dnsconfig -n tailscale ts-dns -o yaml

# Check CoreDNS ConfigMap
kubectl get configmap -n kube-system coredns -o yaml

# Should have ts.net:53 forwarding block
```

**Solution:**
```bash
# Verify CoreDNS was patched
kubectl get configmap -n kube-system coredns -o yaml | grep -A 4 "ts.net:53"

# If missing, check installer logs
foundry logs
```

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
- Check operator is running: `kubectl get pods -n tailscale`
- Verify Connector shows route advertised: `kubectl get connector -n tailscale -o yaml`
- Check Tailscale admin console for pending route approvals
- Ensure `use_tailscale: true` is set in cluster config

### SSH Connection Refused Between Nodes

**Symptom:**
```
tailscale: tailnet policy does not permit you to SSH to this node
```

**Diagnosis:**
Tailscale ACL doesn't allow SSH between cluster nodes.

**Solution:**
Add SSH rule allowing `tag:k8s` → `tag:k8s` as shown in the ACL example above.

## Validation Checklist

Before deploying:

- [ ] All nodes have Tailscale installed and connected
- [ ] Nodes are tagged appropriately (e.g., `tag:k8s`)
- [ ] Tailscale ACL allows SSH from your machine to nodes
- [ ] Tailscale ACL allows SSH between nodes (`tag:k8s` → `tag:k8s`)
- [ ] OAuth client created with required scopes
- [ ] Credentials stored in OpenBAO or config
- [ ] `allow_cgnat_vip: true` is set in cluster config
- [ ] For automated integration: `use_tailscale: true` is set

After deploying:

- [ ] Operator pods running: `kubectl get pods -n tailscale`
- [ ] Connector created: `kubectl get connector -n tailscale`
- [ ] VIP route visible in Tailscale admin console
- [ ] Workers can reach VIP: `curl -k https://<VIP>:6443/version`
- [ ] DNS resolution works: `kubectl run test --image=nicolaka/netshoot -it --rm -- nslookup <device>.ts.net`

## Advanced Configuration

### Custom Operator Image

```yaml
components:
  tailscale:
    oauth_client_id: ${secret:foundry-core/tailscale:client_id}
    oauth_client_secret: ${secret:foundry-core/tailscale:client_secret}
    operator_image: custom-registry.com/tailscale-operator:v1.2.3
```

### Additional Subnet Routes

Advertise additional routes beyond the VIP:

```yaml
components:
  tailscale:
    oauth_client_id: ${secret:foundry-core/tailscale:client_id}
    oauth_client_secret: ${secret:foundry-core/tailscale:client_secret}
    advertise_routes:
      - 10.0.0.0/8      # Private network
      - 172.16.0.0/12   # Another subnet
```

### Custom ACL Tags

```yaml
components:
  tailscale:
    oauth_client_id: ${secret:foundry-core/tailscale:client_id}
    oauth_client_secret: ${secret:foundry-core/tailscale:client_secret}
    tags:
      - tag:k8s-foundry
      - tag:production
      - tag:us-west
```

Ensure these tags are defined in your Tailscale ACL `tagOwners`.

## Roadmap

Future enhancements planned for Tailscale integration:

1. **Multi-Cluster Mesh**
   - Connect multiple Foundry clusters via Tailscale
   - Cross-cluster service discovery
   - Unified network policy across clusters

2. **GitOps for Tailscale ACLs**
   - Version control for network policies
   - CI/CD automation for ACL updates
   - Integration with Foundry stack management

3. **Pod-to-Pod Tailscale Networking**
   - Direct Tailscale connectivity for pods
   - Per-pod Tailscale identities
   - Fine-grained ACL policies at pod level

## References

- [RFC 6598 - Shared Address Space (CGNAT)](https://www.rfc-editor.org/rfc/rfc6598)
- [Tailscale ACL Documentation](https://tailscale.com/kb/1018/acls/)
- [Tailscale Subnet Routes](https://tailscale.com/kb/1019/subnets/)
- [Tailscale Kubernetes Operator](https://tailscale.com/kb/1236/kubernetes-operator/)
- [kube-vip Documentation](https://kube-vip.io/)

## Contributing

Found an issue or have suggestions for Tailscale integration? Please open an issue on the [Foundry GitHub repository](https://github.com/catalystcommunity/foundry).
