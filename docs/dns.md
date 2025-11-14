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
- `truenas.catalyst.local` → TrueNAS IP (if configured)

**Kubernetes Services (wildcard A record):**
- `*.catalyst.local` → VIP (e.g., 10.16.0.43)
- Catches all K8s app hostnames: `grafana.catalyst.local`, `myapp.catalyst.local`, etc.
- Ingress controller (Contour) routes based on HTTP Host header

### What About cluster.local?

`cluster.local` is K8s internal DNS managed by CoreDNS. Foundry does NOT manage this - it's used for pod-to-pod communication inside Kubernetes.

## Split-Horizon DNS (Phase 3 - Future)

PowerDNS supports split-horizon DNS for public domains:

- **Internal queries** (RFC1918 IPs): Return internal IP addresses
- **External queries**: Return CNAME to user's DDNS hostname or use third-party DNS

**Note:** Split-horizon DNS is planned for Phase 3 with External-DNS integration. Phase 2 focuses on local network DNS resolution only.

## Public Domain Support (Phase 3 - Future)

For external access to your services, you have two options:

**Option 1: PowerDNS with DNS Delegation**
```
; In your domain registrar or DNS provider
mycompany.com.  NS  your-ddns-hostname.
```

**Option 2: Third-Party DNS Provider**
- Use Cloudflare, Route53, or any DNS provider
- External-DNS can manage records via their APIs
- Local queries still use PowerDNS

Phase 2 validation focuses on local network DNS only.

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
curl -H "X-API-Key: $(foundry config get dns.api_key)" \
  http://dns.example.com:8081/api/v1/servers/localhost/zones
```

Test external resolution:
```bash
dig @10.16.0.42 openbao.catalyst.local
dig @10.16.0.42 grafana.catalyst.local
```
