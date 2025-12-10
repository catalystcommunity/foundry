# Storage Configuration

Foundry uses a Kubernetes-native storage architecture with Longhorn for block storage and SeaweedFS for S3-compatible object storage.

## Architecture Overview

```
┌─────────────────────────────────────────────────────────────────────┐
│  Worker Nodes                                                        │
│  - Longhorn uses directory on existing filesystem                   │
│  - Each node contributes storage capacity                           │
│  - Replicas distributed across nodes                                │
└─────────────────────────────────────────────────────────────────────┘
                              │
                              ▼
┌─────────────────────────────────────────────────────────────────────┐
│  Longhorn                                                           │
│  - Provides StorageClass for all PVCs                               │
│  - Handles replication across nodes                                 │
│  - Snapshots and backup capabilities                                │
└─────────────────────────────────────────────────────────────────────┘
                              │
                              ▼
┌─────────────────────────────────────────────────────────────────────┐
│  SeaweedFS (S3-compatible object storage)                           │
│  - Runs on Longhorn PVCs                                            │
│  - Provides S3 API for Velero, Loki, etc.                           │
│  - High performance and scalable                                    │
└─────────────────────────────────────────────────────────────────────┘
```

## Storage Backends

### Longhorn (Recommended)

Longhorn provides distributed block storage for Kubernetes with built-in replication.

**Features:**
- Distributed across nodes (no single point of failure)
- Automatic replication (configurable replica count)
- Snapshot and backup support
- No RAID required (Longhorn handles redundancy)

**Configuration:**
```yaml
storage:
  backend: longhorn
  longhorn:
    replica_count: 3
    data_path: /var/lib/longhorn
    default_data_locality: best-effort
```

### Local-Path (Simple)

K3s bundled local-path provisioner for single-node or development clusters.

**Configuration:**
```yaml
storage:
  backend: local-path
```

### NFS (External Storage)

NFS subdir provisioner for existing NFS servers.

**Configuration:**
```yaml
storage:
  backend: nfs
  nfs:
    server: nfs.example.local
    path: /exports/k8s
```

## Object Storage (SeaweedFS)

SeaweedFS provides S3-compatible object storage for services that need it:
- **Velero**: Cluster backups
- **Loki**: Log storage
- **Zot**: Container image storage (optional)

### Why SeaweedFS?

- High performance and scalable
- S3-compatible API
- Runs on Longhorn PVCs (no external dependencies)
- Designed for self-hosted environments
- Active development and community

### Configuration

```yaml
seaweedfs:
  enabled: true
  replicas: 3
  storage_size: 100Gi
  buckets:
    - velero
    - loki
```

### S3 Endpoint

Once deployed, SeaweedFS is accessible at:
```
http://seaweedfs-s3.seaweedfs.svc.cluster.local:8333
```

Services like Loki and Velero are automatically configured to use this endpoint.

## Commands

List storage configuration:
```bash
foundry storage list
```

Provision storage:
```bash
foundry storage provision
```

Create PVC:
```bash
foundry storage pvc create --name my-data --size 10Gi
```

## Disk Recommendations

**Worker Nodes:**
- Single partition with OS and data
- Longhorn uses a directory (e.g., `/var/lib/longhorn`)
- No RAID needed (Longhorn handles replication)

**Storage Nodes (Optional):**
- Dedicated storage nodes with additional disks
- Each disk mounted separately (no RAID)
- Longhorn uses each mount point

## Component Dependencies

Install order is handled automatically:

1. **Longhorn** (provides StorageClass)
2. **SeaweedFS** (uses Longhorn PVCs, provides S3)
3. **Loki** (uses SeaweedFS for log storage)
4. **Velero** (uses SeaweedFS for backup storage)

## Troubleshooting

### Check Longhorn Status

```bash
kubectl -n longhorn-system get pods
kubectl -n longhorn-system get storageclass
```

### Check SeaweedFS Status

```bash
kubectl -n seaweedfs get pods
kubectl -n seaweedfs get svc
```

### Verify S3 Connectivity

```bash
# Port-forward SeaweedFS S3 gateway
kubectl -n seaweedfs port-forward svc/seaweedfs-s3 8333:8333

# Test with aws-cli
aws --endpoint-url http://localhost:8333 s3 ls
```
