# DNS Configuration

## DNS Strategy

Foundry uses PowerDNS to manage two DNS zones:

### Infrastructure Zone

Contains A records for all infrastructure components:
- `openbao.<domain>` - OpenBAO secrets management
- `dns.<domain>` - PowerDNS server
- `zot.<domain>` - Zot OCI registry
- `truenas.<domain>` - TrueNAS storage (if configured)
- `k8s.<domain>` - Kubernetes API server VIP

### Kubernetes Zone

Contains a wildcard record for ingress:
- `*.<k8s-domain>` - Routes to Contour ingress controller

This allows services like `myapp.k8s.example.com` to automatically route through the ingress.

## Split-Horizon DNS

PowerDNS can serve different responses based on query source:

- **Internal queries** (RFC1918 IPs): Return internal IP addresses
- **External queries**: Return public CNAME or IP addresses

This is configured during setup based on whether your infrastructure is publicly accessible.

## DNS Delegation

For external access, delegate your DNS zones to PowerDNS:

```
; In your domain registrar or DNS provider
infra.example.com.  NS  your-ddns-hostname.
k8s.example.com.    NS  your-ddns-hostname.
```

If using a dynamic DNS hostname, PowerDNS will be accessible at that address.

## DNS Management

### Zone Operations

List zones:
```bash
foundry dns zone list
```

Create zone:
```bash
foundry dns zone create example.com
```

### Record Operations

Add A record:
```bash
foundry dns record add example.com myhost A 192.168.1.100
```

List records:
```bash
foundry dns record list example.com
```

### Testing

Test DNS resolution:
```bash
foundry dns test example.com
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
dig @dns.example.com example.com
```
