# DNS Configuration

## DNS Strategy

Foundry uses PowerDNS with a **flat namespace architecture** - one DNS zone for everything.

### Single Zone - Flat Namespace

**Architecture:**
- One DNS zone for the entire cluster (e.g., `catalyst.local`)
- No subdomains like `infra.` or `k8s.` - everything is flat
- Specific A records for infrastructure services
- Wildcard A record for all Kubernetes services

**DNS Records:**

**Infrastructure Services (specific A records):**
- `openbao.catalyst.local` → Host IP (e.g., 10.16.0.42)
- `dns.catalyst.local` → Host IP
- `zot.catalyst.local` → Host IP

**Kubernetes Services (wildcard A record):**
- `*.catalyst.local` → VIP (e.g., 10.16.0.43)
- Catches all K8s app hostnames: `grafana.catalyst.local`, `myapp.catalyst.local`, etc.
- Ingress controller (Contour) routes based on HTTP Host header

### What About cluster.local?

`cluster.local` is K8s internal DNS managed by CoreDNS. Foundry does NOT manage this - it's used for pod-to-pod communication inside Kubernetes.

## Public DNS Providers

The `external-dns` component supports these providers:

- PowerDNS
- Cloudflare
- AWS Route 53
- Google Cloud DNS
- Azure DNS
- RFC 2136

Set the provider and domain filters in the `external-dns` component
configuration. Then install the component:

```bash
foundry component install external-dns
```

## DNS Management

### Zone Operations

List zones:
```bash
foundry dns zone list
```

Create zone:
```bash
foundry dns zone create catalyst.local
```

### Record Operations

Add specific A record (infrastructure service):
```bash
foundry dns record add catalyst.local openbao A 192.168.1.42
```

Add wildcard A record (all K8s services):
```bash
foundry dns record add catalyst.local "*" A 192.168.1.43
```

List records:
```bash
foundry dns record list catalyst.local
```

### Testing

Test DNS resolution:
```bash
foundry dns test openbao.catalyst.local
foundry dns test grafana.catalyst.local
```

This queries the PowerDNS server directly to verify record configuration.

## Troubleshooting

Check PowerDNS logs:
```bash
docker logs foundry-powerdns
```

Verify API connectivity:
```bash
curl -H "X-API-Key: <api-key>" \
  http://dns.example.com:8081/api/v1/servers/localhost/zones
```

Test external resolution:
```bash
dig @10.16.0.42 openbao.catalyst.local
dig @10.16.0.42 grafana.catalyst.local
```
