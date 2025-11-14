# Proposal: PowerDNS Integration and Static IP Planning

**STATUS: PARTIALLY DEPRECATED** - This proposal has been implemented with modifications:
- ✅ PowerDNS integration complete
- ❌ DNS zone strategy changed to **flat namespace** (single zone, no `infra.` or `k8s.` subdomains)
- ❌ Split-horizon DNS deferred to Phase 3
- See `DESIGN.md` for current architecture

## Summary

Introduce PowerDNS-auth as a core infrastructure component with split-horizon DNS support, enabling seamless access to infrastructure both locally and remotely. Add a progressive setup workflow to guide users through network planning and stack installation.

**Note:** The multi-zone approach described in this document has been replaced with a simpler flat namespace architecture.

## Key Decisions

### DNS Strategy: Split-Horizon with Public Delegation

PowerDNS acts as the authoritative DNS server for user's domains, supporting both local and public resolution:

**Pattern:**
1. User owns public domain (e.g., `example.com`)
2. User has Dynamic DNS updating a hostname (e.g., `home.example.com` → router's public IP)
3. User delegates subdomain to their PowerDNS via NS record:
   - `infraexample.com` NS → `home.example.com`
   - `k8sexample.com` NS → `home.example.com`
4. Router port-forwards DNS (53) → PowerDNS (192.168.1.10:53)
5. PowerDNS responds differently based on query source:
   - **Internal queries**: Return local IPs (192.168.1.x)
   - **External queries**: Return CNAME to public DDNS hostname

**Benefits:**
- Single configuration works everywhere
- `docker pull zot.infraexample.com/myimage:latest` works at home (fast local) or coffee shop (via public IP)
- No need to reconfigure tools when mobile
- True split-horizon DNS

### DNS Zone Types

**Infrastructure Zones:**
- Purpose: Infrastructure components (OpenBAO, PowerDNS, Zot, TrueNAS, K8s VIP)
- Examples: `infraexample.com` (public), `infra.local` (private only)
- Records: Static A records for infrastructure
- Managed by: Foundry (during component installation)

**Kubernetes Zones:**
- Purpose: K8s services and ingresses
- Examples: `k8sexample.com` (public), `k8s.local` (private only)
- Records: Dynamic A records for services, wildcard support
- Managed by: External-DNS (Phase 3)

**`.local` TLD Rule:**
- **Always private only** - Never publicly accessible
- No split-horizon configuration
- Use when you never want external access

**Public Domain Recommendation:**
- Use split-horizon for same zone everywhere
- Don't create separate private + public zones for same infrastructure
- Example: Use `infraexample.com` (split-horizon), not both `infra.local` + `infra-public.com`

### DNS Resolution Examples

#### Infrastructure Zone (infraexample.com - split-horizon)

**Internal query** (from 192.168.1.x):
```dns
Query: zot.infraexample.com
Answer: A 192.168.1.10  (local IP, fast)
```

**External query** (from internet):
```dns
Query: zot.infraexample.com
  → Public DNS: NS home.example.com
  → Router (203.0.113.42) forwards to PowerDNS
Answer: CNAME home.example.com
  → Resolves to: A 203.0.113.42 (router's public IP)
  → Router forwards port 5000 → 192.168.1.10:5000
```

#### Kubernetes Zone (k8sexample.com - split-horizon)

**Internal query**:
```dns
Query: grafana.k8sexample.com
Answer: A 192.168.1.100  (K8s VIP)
```

**External query**:
```dns
Query: grafana.k8sexample.com
  → Public DNS: NS home.example.com
  → Router forwards to PowerDNS
Answer: CNAME home.example.com
  → Resolves to: A 203.0.113.42
  → Router forwards port 443 → 192.168.1.100:443
  → Contour routes based on Host header
```

#### Private-Only Zone (infra.local)

**Internal query**:
```dns
Query: openbao.infra.local
Answer: A 192.168.1.10
```

**External query**:
```dns
Query: openbao.infra.local
Answer: NXDOMAIN  (.local zones are never public)
```

### User Application Domains (Optional)

Users can CNAME their domains to K8s services:

```dns
# In user's public DNS (Cloudflare, Route53, etc.):
myapp.com.  CNAME  myapp.k8sexample.com.

# PowerDNS manages myapp.k8sexample.com automatically
# myapp.com follows via CNAME
```

PowerDNS is **not** authoritative for `myapp.com` - user manages that in their DNS provider.

### Required Static/Reserved IPs

**Minimum Deployment:**
1. **Infrastructure host**: OpenBAO + PowerDNS + Zot containers (e.g., 192.168.1.10)
2. **K3s VIP**: Virtual IP for Kubernetes API (e.g., 192.168.1.100) - **must be unique**

**With TrueNAS:**
3. **TrueNAS**: Separate device/VM (e.g., 192.168.1.15)

**Multi-IP Support:**
- Config supports lists of IPs for each component
- Phase 2: Only first IP is used
- Phase 4+: Multiple IPs enable HA configurations

### Infrastructure VIP Naming

K3s VIP is part of infrastructure zone:
- `k8s.infraexample.com` → 192.168.1.100 (K8s VIP)
- All K8s services use k8s zone: `*.k8sexample.com` → 192.168.1.100

### PowerDNS Configuration

- **Authoritative** for configured zones (infrastructure + k8s)
- **Recursive resolver** for other queries (forwards to 8.8.8.8, 1.1.1.1, etc.)
- **Backend**: SQLite (simple) or PostgreSQL (HA in Phase 4+)
- **Image tag**: PowerDNS uses unusual tagging (e.g., `49` = all 4.9.x versions)

### Progressive Setup Workflow

Primary UX: `foundry setup` - stateful wizard that guides through installation

**Features:**
- Tracks progress in config file (`_setup_state`)
- Resumes where it left off if interrupted
- Validates each step before proceeding
- Shows clear progress and next steps

**Individual commands still available** for advanced users who prefer granular control.

## Configuration Schema

```yaml
# ~/.foundry/stack.yaml

network:
  # Network configuration for IP planning
  gateway: 192.168.1.1
  netmask: 255.255.255.0

  # Optional: DHCP range (enables conflict validation)
  dhcp_range:
    start: 192.168.1.50
    end: 192.168.1.200

  # Required static/reserved IPs (can be hostnames or IPs)
  # Lists support HA in future phases (Phase 2 uses first entry only)
  openbao_hosts:
    - 192.168.1.10

  dns_hosts:
    - 192.168.1.10  # Can be same as OpenBAO

  zot_hosts:
    - 192.168.1.10  # Typically same host

  # Optional: TrueNAS (separate device)
  truenas_hosts:
    - 192.168.1.15

  # Required: K3s VIP (must be unique, different from all above)
  k8s_vip: 192.168.1.100

  # Optional: Explicit K3s node IPs (otherwise DHCP is fine)
  k8s_node_ips:
    node1.example.local: 192.168.1.21
    node2.example.local: 192.168.1.22

dns:
  # Infrastructure zones (can be multiple)
  # Use .local for private only, or public domain for split-horizon
  infrastructure_zones:
    - name: infraexample.com
      public: true  # Enable split-horizon
      public_cname: home.example.com  # DDNS hostname for external queries

  # Kubernetes zones (can be multiple)
  kubernetes_zones:
    - name: k8sexample.com
      public: true
      public_cname: home.example.com

  # Upstream forwarders for recursive queries
  forwarders:
    - 8.8.8.8
    - 1.1.1.1

  # PowerDNS backend
  backend: sqlite  # or postgresql for HA later

  # API key stored in OpenBAO
  api_key: ${secret:foundry-core/dns:api_key}

cluster:
  name: production
  domain: example.com  # Base domain
  k8s_vip: 192.168.1.100  # Must match network.k8s_vip

  nodes:
    - hostname: node1.example.com
      role: control-plane
    - hostname: node2.example.com
      role: worker

components:
  openbao:
    # Runs on network.openbao_hosts[0]
    # DNS: openbao.infraexample.com → 192.168.1.10

  dns:
    # Runs on network.dns_hosts[0]
    # DNS: dns.infraexample.com → 192.168.1.10
    image_tag: "49"  # PowerDNS version (4.9.x)

  zot:
    # Runs on network.zot_hosts[0]
    # DNS: zot.infraexample.com → 192.168.1.10

  k3s:
    tag: "v1.28.5+k3s1"

storage:
  truenas:
    # Optional - only if using TrueNAS
    # DNS: truenas.infraexample.com → 192.168.1.15
    api_url: https://truenas.infraexample.com
    api_key: ${secret:foundry-core/truenas:api_key}

# Setup state (managed automatically by 'foundry setup')
_setup_state:
  network_planned: true
  network_validated: true
  openbao_installed: true
  dns_installed: true
  dns_zones_created: true
  zot_installed: false
  k8s_installed: false
  stack_complete: false
```

### Configuration Examples

**Example 1: Public split-horizon setup**
```yaml
dns:
  infrastructure_zones:
    - name: infraexample.com
      public: true
      public_cname: home.example.com

  kubernetes_zones:
    - name: k8sexample.com
      public: true
      public_cname: home.example.com
```

Works everywhere - same config at home and coffee shop.

**Example 2: Private-only setup**
```yaml
dns:
  infrastructure_zones:
    - name: infra.local  # .local = private only, no public config needed

  kubernetes_zones:
    - name: k8s.local
```

Only accessible on local network, no external access.

**Example 3: Mixed public/private**
```yaml
dns:
  infrastructure_zones:
    - name: infra.local  # Private (OpenBAO stays internal)

  kubernetes_zones:
    - name: k8sexample.com  # Public (K8s apps accessible externally)
      public: true
      public_cname: home.example.com
```

Infrastructure private, K8s services public.

**Example 4: Multiple zones for different purposes**
```yaml
dns:
  infrastructure_zones:
    - name: infraexample.com
      public: true
      public_cname: home.example.com

  kubernetes_zones:
    - name: apps.example.com  # Customer-facing apps
      public: true
      public_cname: home.example.com
    - name: internal.example.com  # Internal tools
      public: false  # Block external access
```

Fine-grained control over what's publicly accessible.

### Validation Rules

**Required:**
- `network.openbao_hosts` must have at least one entry
- `network.dns_hosts` must have at least one entry
- `network.k8s_vip` must be set
- `network.k8s_vip` must not appear in any host list (unique IP)
- `dns.infrastructure_zones` must have at least one zone
- `dns.kubernetes_zones` must have at least one zone
- Infrastructure and k8s zones must not overlap
- If zone name ends in `.local`, `public` must be `false` (or omitted)
- If zone has `public: true`, `public_cname` must be set

**Recommended:**
- `network.dhcp_range` should be set (enables conflict validation)
- All static IPs should be outside DHCP range
- Use public domains with split-horizon rather than separate private + public zones
- Use `.local` only if you never want external access

## DNS Zone Record Examples

### Infrastructure Zone (infraexample.com)

```dns
infraexample.com.              SOA  dns.infraexample.com. ...
infraexample.com.              NS   dns.infraexample.com.

openbao.infraexample.com.      A    192.168.1.10   ; Internal
dns.infraexample.com.          A    192.168.1.10   ; Internal
zot.infraexample.com.          A    192.168.1.10   ; Internal
truenas.infraexample.com.      A    192.168.1.15   ; Internal
k8s.infraexample.com.          A    192.168.1.100  ; Internal (VIP)

; External queries get CNAME (if public: true)
*.infraexample.com.            CNAME home.example.com.  ; External only
```

### Kubernetes Zone (k8sexample.com)

```dns
k8sexample.com.                SOA  dns.infraexample.com. ...
k8sexample.com.                NS   dns.infraexample.com.

; Managed by External-DNS (Phase 3)
grafana.k8sexample.com.        A    192.168.1.100  ; Internal
prometheus.k8sexample.com.     A    192.168.1.100  ; Internal
myapp.k8sexample.com.          A    192.168.1.100  ; Internal

; Wildcard for convenience
*.k8sexample.com.              A    192.168.1.100  ; Internal

; External queries get CNAME (if public: true)
*.k8sexample.com.              CNAME home.example.com.  ; External only
```

### Private Zone (infra.local)

```dns
infra.local.                    SOA  dns.infra.local. ...
infra.local.                    NS   dns.infra.local.

openbao.infra.local.            A    192.168.1.10
dns.infra.local.                A    192.168.1.10
zot.infra.local.                A    192.168.1.10
k8s.infra.local.                A    192.168.1.100

; .local zones never respond to external queries (NXDOMAIN)
```

## CLI Commands

### Primary Command: `foundry setup`

```bash
foundry setup [--config PATH] [--resume] [--reset]
  # Interactive wizard that guides through entire stack setup
  # - Reads current state from config (_setup_state)
  # - Shows progress visualization
  # - Validates each step before proceeding
  # - Automatically resumes if interrupted
  # - Updates state as it progresses

  --resume    # Resume from last checkpoint (default behavior)
  --reset     # Start over from beginning
```

### Network Planning

```bash
foundry network plan [--config PATH]
  # Interactive network planning wizard
  # - Prompts for gateway, subnet, DHCP range
  # - Suggests IP allocations
  # - Detects MACs via SSH
  # - Shows DHCP reservation requirements
  # - Updates config file
  # - Sets _setup_state.network_planned = true

foundry network detect-macs [--config PATH]
  # Connect to configured hosts
  # Detect primary interface and MAC address
  # Show current IP vs desired IP
  # Output MAC→IP mappings for DHCP reservations

foundry network validate [--config PATH]
  # Validate network configuration:
  # - All required IPs are set
  # - K8s VIP is unique
  # - IPs outside DHCP range (if configured)
  # - Hosts are reachable
  # - DNS resolution works (if PowerDNS installed)
  # - Sets _setup_state.network_validated = true
```

### DNS Management

```bash
foundry dns zone list
  # List all zones in PowerDNS

foundry dns zone create <zone-name> [--type NATIVE] [--public] [--public-cname HOSTNAME]
  # Create new zone
  # --public: Enable split-horizon
  # --public-cname: DDNS hostname for external queries

foundry dns zone delete <zone-name>
  # Delete zone and all records

foundry dns record add <zone> <name> <type> <value> [--ttl 3600]
  # Add record to zone
  # Types: A, AAAA, CNAME, MX, TXT, NS, etc.

foundry dns record list <zone>
  # List all records in zone

foundry dns record delete <zone> <name> <type>
  # Delete specific record

foundry dns test <hostname>
  # Query PowerDNS to verify resolution
  # Shows which zone answered
  # Shows if split-horizon is working
```

### Component Installation

```bash
foundry component install <name> [--config PATH]
  # Install individual component
  # Updates _setup_state.<name>_installed = true

foundry component status <name>
  # Show component health and version

foundry component list
  # List all available components with status
```

### Stack Operations

```bash
foundry stack install [--config PATH] [--yes]
  # Non-interactive full installation
  # For automation / advanced users
  # Runs same logic as 'foundry setup' but without prompts

foundry stack validate [--config PATH]
  # Validate entire configuration
  # Network, DNS zones, component configs, secrets

foundry stack status [--config PATH]
  # Show status of all components
  # Overall health indicator
```

## Setup Workflow Example

### First-Time User with Public Access

```bash
$ foundry setup

Foundry Setup Wizard
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

No configuration found. Let's create one!

Step 1: Network Planning
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

What is your network gateway? 192.168.1.1
What is your subnet mask? 255.255.255.0
Do you have a DHCP server? (y/n) y
What is your DHCP range? (format: start-end) 192.168.1.50-192.168.1.200

✓ Detected DHCP range: 192.168.1.50 - 192.168.1.200

How many infrastructure hosts do you have?
  1. Single host (all services on one machine) [recommended]
  2. Separate hosts (OpenBAO, DNS, Zot on different machines)
> 1

Suggested IPs (outside DHCP range):
  Infrastructure host: 192.168.1.10
  K3s VIP: 192.168.1.100

Accept these? (y/n) y

Will you be using TrueNAS? (y/n) y
TrueNAS IP address? 192.168.1.15

Step 2: DNS Configuration
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

Do you want to access your infrastructure from outside your local network? (y/n) y

Great! You'll need:
  1. A domain you own (e.g., example.com)
  2. Dynamic DNS configured on your router

What domain do you want for infrastructure services?
  Examples: infraexample.com, core.example.com
  Use .local for private-only: infra.local
> infraexample.com

What domain do you want for Kubernetes services?
  Examples: k8sexample.com, apps.example.com
  Use .local for private-only: k8s.local
> k8sexample.com

What is your Dynamic DNS hostname?
  (This should already be configured on your router)
  Example: home.example.com, myrouter.duckdns.org
> home.example.com

✓ Split-horizon DNS will be configured:
  - Internal queries: Return local IPs (192.168.1.x)
  - External queries: Return CNAME to home.example.com

⚠ Required DNS configuration (do this in your DNS provider):
  Add NS records:
    infraexample.com NS home.example.com
    k8sexample.com NS home.example.com

⚠ Required router configuration:
  Port forwarding:
    53 (DNS) → 192.168.1.10:53
    80 (HTTP) → 192.168.1.100:80
    443 (HTTPS) → 192.168.1.100:443
    5000 (Zot) → 192.168.1.10:5000

Have you configured the NS records in your DNS provider? (y/n)
> n

No problem! You can do this later. We'll continue with local setup.
Split-horizon DNS will work once you complete the DNS delegation.

Upstream DNS servers? (comma-separated, or Enter for 8.8.8.8,1.1.1.1)
>

Step 3: Cluster Configuration
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

Cluster name? production
How many Kubernetes nodes? 2

Node 1 hostname? node1.example.com
Node 1 role? (control-plane/worker) [control-plane]

Node 2 hostname? node2.example.com
Node 2 role? (control-plane/worker) [worker]

Step 4: Infrastructure Host Detection
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

Infrastructure host (for OpenBAO, DNS, Zot):
Hostname or current IP? 192.168.1.75

Connecting to 192.168.1.75...
✓ Connected successfully
✓ Interface: eth0
✓ MAC: 52:54:00:12:34:56
✓ Current IP: 192.168.1.75 (DHCP)

This host needs IP: 192.168.1.10

Configure using DHCP reservation or static IP?
  1. DHCP reservation (recommended)
  2. Static IP configuration
  3. I'll do it manually
> 1

Step 5: DHCP Reservation
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

Add this DHCP reservation to your router:
  MAC: 52:54:00:12:34:56 → IP: 192.168.1.10

After configuring:
  1. Reboot the host
  2. Run: foundry setup

Configuration saved to ~/.foundry/stack.yaml

Press Enter when ready to continue, or Ctrl+C to exit...

# User configures router, reboots host, returns

$ foundry setup

Foundry Setup Wizard
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

Welcome back! Resuming setup...

Validating network configuration...
✓ Infrastructure host has correct IP (192.168.1.10)
✓ All IPs are outside DHCP range
✓ K8s VIP is unique

Progress: ▓▓░░░░░░░░░░░░░░░░░░ 10%

✓ Network planned
✓ Network validated
⧗ OpenBAO installation (next step)

Next Step: Install OpenBAO
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

OpenBAO will be installed on:
  Host: 192.168.1.10
  DNS: openbao.infraexample.com

Continue? (Y/n)

Installing OpenBAO...
✓ Container image pulled
✓ Systemd service created
✓ OpenBAO started

Initializing OpenBAO...
✓ Initialized (5 unseal keys, threshold 3)

━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
⚠ IMPORTANT: Save these securely in your password manager!
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

Unseal Key 1: AbCd1234...
Unseal Key 2: EfGh5678...
Unseal Key 3: IjKl9012...
Unseal Key 4: MnOp3456...
Unseal Key 5: QrSt7890...

Root Token: hvs.UvWxYz...

Press Enter after saving these values...

Unsealing OpenBAO...
✓ Unsealed successfully

Storing Foundry auth token...
✓ Token stored securely

Progress: ▓▓▓▓░░░░░░░░░░░░░░░░ 20%

✓ Network planned
✓ Network validated
✓ OpenBAO installed
⧗ PowerDNS installation (next step)

Next Step: Install PowerDNS
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

PowerDNS will be installed on:
  Host: 192.168.1.10 (same as OpenBAO)
  DNS: dns.infraexample.com

Continue? (Y/n)

Installing PowerDNS...
✓ Container image pulled (powerdns-auth-49)
✓ Configuration created (SQLite backend)
✓ API key generated and stored in OpenBAO
✓ Systemd service created
✓ PowerDNS started
✓ API is accessible

Progress: ▓▓▓▓▓▓░░░░░░░░░░░░░░ 30%

✓ Network planned
✓ Network validated
✓ OpenBAO installed
✓ PowerDNS installed
⧗ DNS zones creation (next step)

Next Step: Create DNS Zones
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

Creating zones:
  Infrastructure: infraexample.com (split-horizon)
  Kubernetes: k8sexample.com (split-horizon)

Continue? (Y/n)

Creating infrastructure zone: infraexample.com...
✓ Zone created
✓ SOA and NS records added
✓ A records added:
  - openbao.infraexample.com → 192.168.1.10
  - dns.infraexample.com → 192.168.1.10
  - zot.infraexample.com → 192.168.1.10
  - truenas.infraexample.com → 192.168.1.15
  - k8s.infraexample.com → 192.168.1.100
✓ Split-horizon CNAME configured for external queries

Creating kubernetes zone: k8sexample.com...
✓ Zone created
✓ SOA and NS records added
✓ Wildcard configured: *.k8sexample.com → 192.168.1.100
✓ Split-horizon CNAME configured for external queries

Testing DNS resolution...
✓ openbao.infraexample.com → 192.168.1.10
✓ zot.infraexample.com → 192.168.1.10
✓ k8s.infraexample.com → 192.168.1.100

Progress: ▓▓▓▓▓▓▓▓░░░░░░░░░░░░ 40%

# ... continues through Zot, K3s, Contour, cert-manager ...

Progress: ▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓ 100%

✓ Network planned
✓ Network validated
✓ OpenBAO installed
✓ PowerDNS installed
✓ DNS zones created
✓ Zot installed
✓ K3s cluster initialized
✓ Networking components installed
✓ Stack validated

━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
🎉 Stack installation complete!
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

Your infrastructure is ready:
  OpenBAO:  https://openbao.infraexample.com
  Zot:      https://zot.infraexample.com
  K8s API:  https://k8s.infraexample.com:6443

Local access: ✓ Working
External access: ⚠ Complete DNS delegation first

To enable external access:
  1. Add NS records in your DNS provider:
       infraexample.com NS home.example.com
       k8sexample.com NS home.example.com
  2. Configure router port forwarding (see setup notes)
  3. Test: dig zot.infraexample.com

Next steps:
  - Deploy your first application
  - Configure External-DNS (Phase 3)
  - Set up observability (Grafana, Prometheus, Loki)

Run 'foundry stack status' to see component health.
```

## Implementation Changes

### Phase 1 (Foundation) - Minor Update

**Update Task 19: `foundry config init`**
- Remove `version` field from generated config
- Add network planning prompts
- Add DNS zone configuration with split-horizon support
- Add public CNAME configuration
- Add `.local` TLD validation
- Add MAC detection
- Generate config with `_setup_state` section

### Phase 2 (Stack Installation) - Significant Changes

#### New Tasks (0.x series):

**Task 0.1: Setup State Management**
- `internal/setup/state.go`
- Track setup progress in config file (`_setup_state`)
- Load/save state
- Determine next step based on state
- Resume capability

**Task 0.2: Setup Wizard Framework**
- `cmd/foundry/commands/setup/wizard.go`
- Interactive TUI for step-by-step setup
- Progress visualization (progress bar, checkmarks)
- Step validation before proceeding
- Checkpoint/resume logic
- Clear error messages and guidance

**Task 0.3: Network Configuration Types**
- Update `internal/config/types.go`
- Remove `version` field
- Add `NetworkConfig` with multi-IP list support
- Add `DNSConfig` with zone list and split-horizon fields
- Validation for IP uniqueness (K8s VIP must not be in host lists)
- Validation for zone separation (infrastructure vs k8s)
- Validation for `.local` TLD (must be private only)
- Validation for split-horizon requirements (public zones need public_cname)

**Task 0.4: Network Detection Utilities**
- `internal/network/detect.go`
- Interface detection via SSH
- MAC address detection
- Current IP detection
- Network topology detection

**Task 0.5: Network Validation**
- `internal/network/validate.go`
- IP reachability checks
- DHCP conflict detection
- DNS resolution validation (query PowerDNS if installed)
- Multi-IP validation (use first, validate others)

**Task 0.6: Network Planning Commands**
- `cmd/foundry/commands/network/plan.go`
- `cmd/foundry/commands/network/detect_macs.go`
- `cmd/foundry/commands/network/validate.go`
- Integration with setup wizard
- DHCP reservation guidance (no router-specific configs)

#### PowerDNS Tasks (8.x series):

**Task 8: PowerDNS Container Installation**
- `internal/component/dns/install.go`
- Pull PowerDNS image (use tag from config, e.g., "49")
- Generate secure API key → store in OpenBAO: `foundry-core/dns:api_key`
- Create PowerDNS config:
  - SQLite backend (simple file-based)
  - Forwarders configuration (8.8.8.8, 1.1.1.1, custom)
  - API enabled
  - Recursive resolver enabled
- Create systemd service on dns_hosts[0]
- Health check (verify API accessibility)

**Task 9: PowerDNS Client**
- `internal/component/dns/client.go`
- HTTP API client for PowerDNS
- Authentication with API key
- Zone operations (create, delete, list)
- Record operations (add, delete, list, update)
- Error handling and retries

**Task 10: Split-Horizon DNS Logic**
- `internal/component/dns/splithorizon.go`
- Detect query source (internal vs external)
- Internal network detection (RFC1918 ranges, custom ranges)
- CNAME generation for external queries
- A record generation for internal queries
- `.local` TLD handling (always private, never CNAME)

**Task 11: DNS Zone Management**
- `internal/component/dns/zone.go`
- Create infrastructure zones (from config list)
- Create kubernetes zones (from config list)
- Add SOA and NS records
- Wildcard record support for k8s zones
- Split-horizon configuration per zone
- Zone validation

**Task 12: Infrastructure DNS Initialization**
- Create all infrastructure zones from config
- For each zone, add A records:
  - `openbao.<zone>` → openbao_hosts[0]
  - `dns.<zone>` → dns_hosts[0]
  - `zot.<zone>` → zot_hosts[0]
  - `truenas.<zone>` → truenas_hosts[0] (if configured)
  - `k8s.<zone>` → k8s_vip
- If zone has `public: true`:
  - Configure split-horizon CNAME to `public_cname`
- Verify local DNS resolution (query from setup host)

**Task 13: Kubernetes DNS Initialization**
- Create all kubernetes zones from config
- Add wildcard: `*.<zone>` → k8s_vip (for internal queries)
- If zone has `public: true`:
  - Configure split-horizon CNAME to `public_cname`
- Leave zone empty for External-DNS to populate (Phase 3)
- Verify resolution

**Task 14: DNS Management Commands**
- `cmd/foundry/commands/dns/zone.go` - zone list/create/delete
- `cmd/foundry/commands/dns/record.go` - record add/list/delete
- `cmd/foundry/commands/dns/test.go` - test DNS resolution
- Split-horizon testing (show internal vs external response)

#### Modified Tasks:

**Update Task 4: OpenBAO Installation** (renumbered)
- Use openbao_hosts[0] from config (first in list)
- Validate host has correct IP before install
- Create DNS entries in all infrastructure zones
- Verify DNS resolution

**Update Task X: Zot Installation** (renumbered)
- Use zot_hosts[0] from config
- Create DNS entries in all infrastructure zones
- Configure Zot to use FQDN (e.g., zot.infraexample.com)

**Update Task Y: K3s Installation** (renumbered)
- Configure K3s nodes to use dns_hosts[0] for DNS resolution
- Update `/etc/resolv.conf` on nodes to point to PowerDNS
- Validate k8s_vip matches config
- Create DNS entries: k8s.<infra-zone> → k8s_vip
- Verify VIP is accessible

**Update Task Z: `foundry stack install`** (renumbered)
- Make non-interactive (for automation/CI)
- Call same installation logic as setup wizard
- Skip prompts, fail fast on errors
- Update order:
  1. Validate network configuration
  2. OpenBAO
  3. PowerDNS
  4. Infrastructure DNS zones
  5. Kubernetes DNS zones
  6. Zot
  7. K3s
  8. Contour, cert-manager

**Update Task AA: `foundry stack validate`** (renumbered)
- Network validation (IPs, reachability)
- DNS zone validation (multi-zone support)
- Split-horizon configuration validation
- `.local` TLD validation
- IP uniqueness checks (K8s VIP)
- Public CNAME validation (if public zones configured)

### Phase 3 (Observability & Storage) - Minor Changes

**External-DNS Installation**
- Configure External-DNS to use PowerDNS API
- Point to kubernetes zones only (not infrastructure zones)
- Use API key from OpenBAO: `foundry-core/dns:api_key`
- Configure source: `service,ingress` (K8s resources to watch)
- Verify dynamic record creation in k8s zones
- Test split-horizon: internal query vs external query

**Documentation Updates**
- Document DNS delegation setup (NS records)
- Document router port forwarding requirements
- Document split-horizon DNS concept
- Troubleshooting guide for DNS issues
- Examples of different zone configurations

## Testing Strategy

### Unit Tests
- Setup state management (save/load/resume)
- Network config validation (multi-IP lists)
- DNS zone validation (multi-zone, split-horizon)
- `.local` TLD validation
- IP uniqueness validation
- MAC address parsing
- Split-horizon logic (internal vs external query detection)

### Integration Tests
- Mock SSH for MAC detection
- Mock PowerDNS API for zone/record operations
- Multi-zone creation and validation
- Split-horizon DNS responses
- State checkpoint/resume workflow
- Setup wizard flow (full end-to-end)

### Manual Testing
- [ ] Complete `foundry setup` from start to finish
- [ ] Interrupt and resume at each checkpoint
- [ ] Single-host configuration (all services on one IP)
- [ ] Configuration with TrueNAS
- [ ] Multiple infrastructure zones
- [ ] Multiple kubernetes zones
- [ ] Private-only setup (`.local` zones)
- [ ] Public split-horizon setup
- [ ] DNS resolution from internal network (get local IPs)
- [ ] DNS resolution from external (get CNAME, if delegation configured)
- [ ] Port forwarding and external access
- [ ] Mobile device (laptop) works both at home and externally

## Public DNS Delegation Setup (User's Responsibility)

Users need to configure these external to Foundry:

### 1. Dynamic DNS (DDNS)
Configure on router or using service:
- **Router's DDNS**: Many routers support DDNS providers (DynDNS, No-IP, DuckDNS)
- **External service**: Run DDNS client on infrastructure host

Result: `home.example.com` → router's public IP (updated automatically)

### 2. DNS Delegation
In user's DNS provider (Cloudflare, Route53, etc.):
```
infraexample.com.  NS  home.example.com.
k8sexample.com.    NS  home.example.com.
```

Result: DNS queries for these subdomains go to PowerDNS (via router)

### 3. Router Port Forwarding
```
External Port → Internal IP:Port
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
53 (DNS)     → 192.168.1.10:53   (PowerDNS)
80 (HTTP)    → 192.168.1.100:80  (K8s Ingress)
443 (HTTPS)  → 192.168.1.100:443 (K8s Ingress)
5000 (Zot)   → 192.168.1.10:5000 (Zot Registry)
```

### Testing External Access

```bash
# From outside your network (mobile data, VPN, friend's house):

# Test DNS delegation
$ dig zot.infraexample.com
; Should return CNAME to home.example.com, then A record with public IP

# Test HTTP access
$ curl https://zot.infraexample.com
; Should reach your Zot instance via router port forwarding
```

## Summary

### Key Features

1. **Split-Horizon DNS** - Same hostnames work locally (fast) and externally (via public IP)
2. **DNS Delegation** - PowerDNS is authoritative for user's subdomains
3. **`.local` TLD Rule** - Always private, never public (clear semantics)
4. **Progressive Setup** - `foundry setup` wizard guides through entire process
5. **Stateful Resume** - Interrupted setups resume where they left off
6. **Multi-Zone Support** - Flexible zone configuration for different use cases
7. **Public CNAME** - External queries get CNAME to DDNS hostname
8. **No Router Lock-In** - No router-specific configs, user handles DDNS + port forwarding

### Files to Update

- `DESIGN.md` - Architecture, DNS zones, split-horizon, setup workflow
- `phase-implementation-2.md` - Insert 0.x and 8.x tasks, update existing tasks
- `implementation-tasks.md` - Update Phase 2 criteria, add DNS requirements
- `internal/config/types.go` - Remove version, add network/dns config with split-horizon

### Estimated Impact

- **Phase 2**: +20 tasks (0.1-0.6, 8-14) = ~56 total tasks
- **Additional LOC**: ~4000-5000 (setup wizard, DNS, split-horizon logic, network planning)
- **Timeline**: +50% for Phase 2 (acceptable for significant UX and functionality improvement)

---

**Ready for implementation approval.**
