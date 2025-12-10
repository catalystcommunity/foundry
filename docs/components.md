# Components

Foundry manages the following core components for your infrastructure stack.

## OpenBAO

**Purpose**: Secrets management and secure storage

OpenBAO stores sensitive data including:
- API keys (PowerDNS, etc.)
- SSH keys
- Kubernetes tokens
- Service credentials

**Deployment**: Container on infrastructure host
**Default Port**: 8200

## PowerDNS

**Purpose**: Authoritative DNS server with API

PowerDNS provides:
- Infrastructure zone (openbao, dns, zot, k8s nodes)
- Kubernetes zone (wildcard for ingress)
- Split-horizon DNS support
- HTTP API for dynamic record management

**Deployment**: Container on infrastructure host
**Default Ports**: 53 (DNS), 8081 (API)

## Zot

**Purpose**: OCI registry for container images

Zot provides:
- Private container registry
- Pull-through cache for Docker Hub
- Kubernetes image source

**Deployment**: Container on infrastructure host
**Default Port**: 5000

## K3s

**Purpose**: Lightweight Kubernetes distribution

K3s provides:
- Kubernetes cluster (control plane + workers)
- kube-vip for HA control plane VIP
- Pre-configured with Zot registry
- DNS integration for service discovery

**Deployment**: Native install on cluster nodes
**Default Port**: 6443 (API server)

### kube-vip

**Purpose**: Virtual IP for HA control plane

Provides a single stable IP for the Kubernetes API server across multiple control plane nodes.

## Contour

**Purpose**: Ingress controller for Kubernetes

Contour provides:
- HTTP/HTTPS ingress routing
- TLS termination
- Envoy-based proxy
- Gateway API support

**Deployment**: Helm chart in Kubernetes

## cert-manager

**Purpose**: Automatic TLS certificate management

cert-manager provides:
- Certificate issuance and renewal
- Let's Encrypt integration
- Internal CA support

**Deployment**: Helm chart in Kubernetes

## Storage Components

### Longhorn

**Purpose**: Distributed block storage for Kubernetes

Longhorn provides:
- StorageClass for PersistentVolumeClaims
- Automatic replication across nodes
- Snapshot and backup capabilities
- No RAID required (handles redundancy)

**Deployment**: Helm chart in Kubernetes
**Namespace**: longhorn-system

### SeaweedFS

**Purpose**: S3-compatible object storage

SeaweedFS provides:
- S3 API for Velero backups
- S3 API for Loki log storage
- High performance and scalable
- Runs on Longhorn PVCs

**Deployment**: Helm chart in Kubernetes
**Namespace**: seaweedfs
**Default Port**: 8333 (S3 API)

## Observability Components

### Prometheus

**Purpose**: Metrics collection and alerting

Prometheus provides:
- Time-series metrics database
- Service discovery for Kubernetes
- Alerting rules
- PromQL query language

**Deployment**: Helm chart (kube-prometheus-stack)
**Namespace**: monitoring

### Loki

**Purpose**: Log aggregation

Loki provides:
- Centralized log storage
- LogQL query language
- Integration with Grafana
- S3 backend via SeaweedFS

**Deployment**: Helm chart in Kubernetes
**Namespace**: loki

### Grafana

**Purpose**: Observability dashboards

Grafana provides:
- Unified dashboards for metrics and logs
- Pre-configured Prometheus and Loki data sources
- Alerting integration

**Deployment**: Helm chart in Kubernetes
**Namespace**: monitoring

## Backup Components

### Velero

**Purpose**: Kubernetes backup and restore

Velero provides:
- Cluster backup and disaster recovery
- PVC snapshots
- Scheduled backups
- S3 backend via SeaweedFS

**Deployment**: Helm chart in Kubernetes
**Namespace**: velero

## DNS Components

### External-DNS

**Purpose**: Automatic DNS record management

External-DNS provides:
- Automatic DNS record creation for Ingress resources
- PowerDNS integration
- Cloudflare and other provider support

**Deployment**: Helm chart in Kubernetes
**Namespace**: external-dns
