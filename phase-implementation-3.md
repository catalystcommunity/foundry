# Phase 3: Observability & Storage - Implementation Tasks

**Goal**: Complete storage integration and deploy full observability stack

**Milestone**: User has production-ready monitoring, logging, and persistent storage

## Prerequisites

Phase 2 must be complete:
- ✓ K3s cluster running with VIP
- ✓ Helm client working
- ✓ Contour and cert-manager deployed
- ✓ TrueNAS API client implemented
- ✓ OpenBAO and Zot installed

## High-Level Task Areas

### 1. Storage Integration

**Working States**:
- [ ] TrueNAS CSI driver deployed to K3s
- [ ] Can provision PVCs backed by TrueNAS
- [ ] StorageClass configured and set as default
- [ ] MinIO deployed (if TrueNAS doesn't provide S3)
- [ ] S3-compatible storage available for backups

**Key Tasks**:
- Deploy TrueNAS CSI driver (democratic-csi or similar)
- Configure StorageClass for NFS or iSCSI
- Test PVC provisioning and mounting
- Deploy MinIO via Helm if needed
- Configure MinIO with TrueNAS-backed PVC
- Create S3 buckets for various components
- Document storage architecture decisions

**Files**:
- `internal/component/truenas/csi.go`
- `internal/component/minio/install.go`
- `cmd/foundry/commands/storage/provision.go`

---

### 2. Prometheus

**Working States**:
- [ ] Prometheus deployed via kube-prometheus-stack Helm chart
- [ ] ServiceMonitors configured for auto-discovery
- [ ] Scraping K3s, OpenBAO, Zot metrics
- [ ] PVC for Prometheus TSDB
- [ ] Retention configured per stack.yaml

**Key Tasks**:
- Deploy kube-prometheus-stack Helm chart
- Configure Prometheus retention
- Set up service discovery for `foundry-*` namespaces
- Create ServiceMonitors for core components
- Configure PVC for storage
- Set up alerting rules (basic)
- Test metric collection

**Files**:
- `internal/component/prometheus/install.go`
- `internal/component/prometheus/servicemonitors.go`

---

### 3. Loki

**Working States**:
- [ ] Loki deployed via loki-stack Helm chart
- [ ] Collecting logs from all pods
- [ ] S3-compatible storage for log retention
- [ ] Retention configured per stack.yaml

**Key Tasks**:
- Deploy loki-stack Helm chart
- Configure S3 backend (MinIO or TrueNAS S3)
- Set up retention policies
- Configure promtail for log collection
- Test log ingestion and querying

**Files**:
- `internal/component/loki/install.go`
- `internal/component/loki/config.go`

---

### 4. Grafana

**Working States**:
- [ ] Grafana deployed via Helm
- [ ] Prometheus and Loki configured as data sources
- [ ] Admin password stored in OpenBAO
- [ ] OIDC auth via OpenBAO (optional for now, required in Phase 4)
- [ ] Default dashboards provisioned
- [ ] Accessible via Ingress

**Key Tasks**:
- Deploy Grafana Helm chart
- Configure data sources (Prometheus, Loki)
- Resolve admin password from OpenBAO (`foundry-core/grafana:admin_password`)
- Create Ingress with TLS
- Provision default dashboards (K3s, Prometheus, Loki)
- Test dashboard access

**Files**:
- `internal/component/grafana/install.go`
- `internal/component/grafana/dashboards.go`
- `internal/component/grafana/datasources.go`

---

### 5. External-DNS

**Working States**:
- [ ] External-DNS deployed via Helm
- [ ] Automatically creates DNS records for Ingress resources
- [ ] Configured for appropriate DNS provider

**Key Tasks**:
- Deploy External-DNS Helm chart
- Configure DNS provider (e.g., Cloudflare, RFC2136, etc.)
- Store DNS provider credentials in OpenBAO
- Test automatic DNS record creation
- Document supported DNS providers

**Files**:
- `internal/component/externaldns/install.go`
- `internal/component/externaldns/providers.go`

---

### 6. Velero

**Working States**:
- [ ] Velero deployed via Helm
- [ ] S3 backend configured (MinIO or TrueNAS S3)
- [ ] Can create cluster backups
- [ ] Can restore from backups
- [ ] Scheduled backup configured

**Key Tasks**:
- Deploy Velero Helm chart
- Configure S3-compatible storage backend
- Set up backup schedules
- Test backup and restore
- Create backup policies

**Files**:
- `internal/component/velero/install.go`
- `internal/component/velero/backup.go`

---

### 7. CLI Commands

**Commands to Implement**:

```bash
# Storage
foundry storage provision <name> --size 10Gi
foundry storage pvc list [--namespace NS]

# Backup
foundry backup create [--name NAME]
foundry backup list
foundry backup restore <name>
foundry backup schedule --cron "0 2 * * *"

# Observability
foundry dashboard open  # Opens Grafana
foundry logs <pod>      # Quick log access via Loki
foundry metrics <service>  # Query Prometheus
```

**Files**:
- `cmd/foundry/commands/storage/provision.go`
- `cmd/foundry/commands/backup/*.go`
- `cmd/foundry/commands/dashboard.go`
- `cmd/foundry/commands/logs.go`

---

### 8. Integration Tests

**Test Scenarios**:
- [ ] Deploy PVC and mount to pod
- [ ] Backup cluster and restore
- [ ] Metrics are scraped and visible in Grafana
- [ ] Logs are collected and queryable in Loki
- [ ] DNS records are created automatically

**Files**:
- `test/integration/phase3_storage_test.go`
- `test/integration/phase3_observability_test.go`
- `test/integration/phase3_backup_test.go`

---

### 9. Documentation

**Documents to Create/Update**:
- [ ] `docs/storage.md` - Full CSI driver setup, PVC usage
- [ ] `docs/observability.md` - Prometheus, Loki, Grafana usage
- [ ] `docs/backups.md` - Velero usage, restore procedures
- [ ] `docs/dns.md` - External-DNS configuration

---

## Phase 3 Completion Criteria

- [ ] All components deploy successfully
- [ ] Storage provisioning works
- [ ] Metrics are collected and visible
- [ ] Logs are collected and queryable
- [ ] Grafana dashboards show system health
- [ ] Backups can be created and restored
- [ ] DNS records are managed automatically
- [ ] All tests pass
- [ ] Documentation complete

## Manual Verification

```bash
# Install observability stack
foundry component install prometheus
foundry component install loki
foundry component install grafana
foundry component install velero

# Test storage
foundry storage provision test-pvc --size 1Gi
kubectl get pvc

# Test backup
foundry backup create test-backup
foundry backup list
foundry backup restore test-backup

# Access Grafana
foundry dashboard open
```

---

**Estimated Working States**: ~20 testable states
**Estimated LOC**: ~3000-4000 lines
**Timeline**: Not time-bound
