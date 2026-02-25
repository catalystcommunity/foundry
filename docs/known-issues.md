# Known Issues

## CoreDNS `hosts` Plugin Conflict with `coredns-custom` Overrides

**Status**: Open
**Affects**: Clusters using foundry with k3s + custom CoreDNS overrides
**Assignee**: @Soypete

### Problem

When foundry manages custom CoreDNS overrides via the `coredns-custom` ConfigMap, an override file containing a `hosts {}` plugin block will cause CoreDNS to crash and take down cluster-internal DNS.

### Root Cause

The k3s default CoreDNS `Corefile` already includes:

```
hosts /etc/coredns/NodeHosts {
  ttl 60
  reload 15s
  fallthrough
}
```

CoreDNS does not allow the `hosts` plugin to appear more than once per server block. If a custom override (e.g. `supabase.override`) also contains a `hosts {}` block, CoreDNS fails to reload with:

```
plugin/hosts: this plugin can only be used once per server block
```

This brings down DNS resolution for all pods in the cluster.

### Impact

- All pods lose cluster-internal and external DNS resolution
- Tailscale operator cannot reach `controlplane.tailscale.com` to provision new Ingress resources
- External services (e.g. Supabase/PostgreSQL at `aws-0-us-west-1.pooler.supabase.com`) become unreachable from pods
- Bot services crash and keepalive alerts fire

### Fix

**For static hostname overrides**: Add entries directly to the `NodeHosts` field of the main `coredns` ConfigMap instead of using a `hosts {}` block in a custom override:

```yaml
# kubectl edit configmap coredns -n kube-system
NodeHosts: |
  192.168.1.128 blue1
  192.168.1.11 blue2
  192.168.1.253 refurb
  52.8.172.168 aws-0-us-west-1.pooler.supabase.com
  54.177.55.191 aws-0-us-west-1.pooler.supabase.com
```

**For the custom override**: Remove any `hosts {}` blocks from `coredns-custom` ConfigMap entries and use CoreDNS `rewrite` or `template` plugins instead for custom DNS behavior.

### Related Issue: External DNS Forwarding Depends on PowerDNS Health

The default k3s CoreDNS config uses `forward . /etc/resolv.conf`, which chains through the node's resolver (systemd-resolved → PowerDNS). When PowerDNS is unhealthy (e.g. during a new node installation), all external DNS queries from pods fail with timeout — even though the node itself may have internet connectivity.

**Recommendation**: Change the CoreDNS forward directive to use public resolvers directly:

```
# In the Corefile, replace:
forward . /etc/resolv.conf

# With:
forward . 8.8.8.8 1.1.1.1
```

This ensures pods can always resolve external hostnames regardless of PowerDNS health.

### Diagnosis Commands

```bash
# Check for conflicting hosts plugin in custom overrides
kubectl get configmap coredns-custom -n kube-system -o yaml

# Check CoreDNS logs for the conflict error
kubectl logs -n kube-system -l k8s-app=kube-dns --tail=50 | grep -i "hosts\|error"

# Test external DNS from inside a pod
kubectl run -it --rm dns-test --image=busybox --restart=Never -- nslookup controlplane.tailscale.com

# Check PowerDNS health
foundry component status powerdns
```

### Patch Command

Apply the CoreDNS forward fix:

```bash
kubectl patch configmap coredns -n kube-system --type=json -p='[
  {"op": "replace", "path": "/data/Corefile", "value": ".:53 {\n    errors\n    health\n    ready\n    kubernetes cluster.local in-addr.arpa ip6.arpa {\n      pods insecure\n      fallthrough in-addr.arpa ip6.arpa\n    }\n    hosts /etc/coredns/NodeHosts {\n      ttl 60\n      reload 15s\n      fallthrough\n    }\n    prometheus :9153\n    cache 30\n    loop\n    reload\n    loadbalance\n    import /etc/coredns/custom/*.override\n    forward . 8.8.8.8 1.1.1.1\n}\nimport /etc/coredns/custom/*.server\n"}
]'
kubectl rollout restart deployment coredns -n kube-system
```
