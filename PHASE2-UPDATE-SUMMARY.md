# Phase 2 Implementation Update Summary

This document outlines the changes needed to `phase-implementation-2.md` based on the approved DNS and network planning proposal.

## Major Changes

### 1. Update Installation Order Section

**OLD:**
```
1. OpenBAO (container on host) - Secrets management first
2. Zot (container on host) - Registry before K3s so K3s can pull from it
3. K3s - Kubernetes cluster configured to use Zot from the start
4. Networking - Contour, cert-manager (via Helm after K3s is up)
```

**NEW:**
```
1. Network Planning & Validation - IP allocation, MAC detection, DHCP guidance
2. OpenBAO (container on host) - Secrets management first
3. PowerDNS (container on host) - DNS server for infrastructure and K8s
4. DNS Zones - Infrastructure (infraexample.com) and Kubernetes (k8sexample.com)
5. Zot (container on host) - Registry before K3s
6. K3s - Kubernetes cluster configured to use PowerDNS and Zot
7. Networking - Contour, cert-manager (via Helm after K3s is up)
```

### 2. Insert New Tasks (0.x series)

Insert BEFORE existing Task 1:

- **Task 0.1**: Setup State Management
- **Task 0.2**: Setup Wizard Framework
- **Task 0.3**: Network Configuration Types (update internal/config/types.go)
- **Task 0.4**: Network Detection Utilities
- **Task 0.5**: Network Validation
- **Task 0.6**: Network Planning Commands

### 3. Insert PowerDNS Tasks (after current Task 7)

New tasks to insert after OpenBAO Auth Token Management:

- **Task 8**: PowerDNS Container Installation
- **Task 9**: PowerDNS Client (HTTP API)
- **Task 10**: Split-Horizon DNS Logic
- **Task 11**: DNS Zone Management
- **Task 12**: Infrastructure DNS Initialization
- **Task 13**: Kubernetes DNS Initialization
- **Task 14**: DNS Management Commands

### 4. Update Task Numbering

After inserting 0.1-0.6 (6 tasks) and 8-14 (7 tasks), all subsequent tasks need renumbering:
- Old Task 1 → New Task 1 (no change)
- Old Task 2 → New Task 2 (no change)
- ...
- Old Task 8 → New Task 15
- Old Task 9 → New Task 16
- ...
- Old Task 36 → New Task 49

Total tasks: 6 (new 0.x) + 7 (existing 1-7) + 7 (new 8-14) + 29 (old 8-36 renumbered) = 49 tasks

### 5. Update Specific Task Content

**Update Task 4 (OpenBAO Installation)**:
- Change from using `hosts` config to `network.openbao_hosts[0]`
- Add DNS record creation (will be done in PowerDNS tasks, just note it)

**Update K3s Tasks (renumbered)**:
- Add PowerDNS DNS configuration
- Update nodes to point to `network.dns_hosts[0]` for DNS
- Update K8s VIP reference to use `network.k8s_vip`

**Update stack install task (final task)**:
- Update order to include network planning, PowerDNS, DNS zones

### 6. Fix DNS Naming Throughout

Search and replace:
- `infra.example.com` → `infraexample.com`
- `k8s.example.com` → `k8sexample.com`
- `zot.infra.example.com` → `zot.infraexample.com`
- `openbao.infra.example.com` → `openbao.infraexample.com`
- etc.

NO nested subdomains - single level only!

### 7. Update Phase Completion Checklist

Add items for:
- [ ] Network planning works
- [ ] PowerDNS installs and runs
- [ ] DNS zones created correctly
- [ ] Split-horizon DNS works
- [ ] `foundry setup` wizard tracks state and resumes

## Task Count Impact

- **Before**: 36 tasks
- **After**: 49 tasks (+13 tasks, +36%)

## Files Impacted

New files to create:
- `internal/setup/state.go`
- `internal/setup/wizard.go`
- `internal/network/detect.go`
- `internal/network/validate.go`
- `internal/component/dns/install.go`
- `internal/component/dns/client.go`
- `internal/component/dns/zone.go`
- `internal/component/dns/splithorizon.go`
- `cmd/foundry/commands/setup/*.go`
- `cmd/foundry/commands/network/*.go`
- `cmd/foundry/commands/dns/*.go`

Updated files:
- `internal/config/types.go` (add NetworkConfig, DNSConfig, remove version field)

## Next Steps

1. Apply these changes systematically to phase-implementation-2.md
2. Update implementation-tasks.md Phase 2 completion criteria
3. Commit changes with descriptive message

---

**Status**: Summary complete, ready to apply changes
