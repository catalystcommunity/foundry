# Storage Configuration

## TrueNAS Integration

Foundry integrates with TrueNAS for network storage capabilities.

### Features

- **Dataset Management**: Create and manage ZFS datasets
- **NFS Shares**: Configure NFS exports for container and Kubernetes storage
- **Pool Information**: Query available storage pools
- **API Integration**: Automated configuration via TrueNAS API

### Use Cases

#### Zot Registry Storage

Configure Zot to use TrueNAS NFS storage for container images:

1. TrueNAS creates a dataset and NFS share
2. Zot mounts the NFS share
3. Container images stored on TrueNAS

This provides:
- Persistent storage for container images
- High-capacity backend storage
- ZFS snapshot and replication capabilities

#### Kubernetes Persistent Volumes

TrueNAS can provide NFS-backed persistent volumes for Kubernetes workloads.

### Configuration

TrueNAS configuration is stored in your Foundry config:

```yaml
storage:
  truenas:
    host: truenas.example.com
    api_key: ${SECRET:secret/foundry-core/truenas:api_key}
    api_version: v2.0
```

API keys are stored in OpenBAO for security.

### Commands

List storage pools:
```bash
foundry storage list
```

Test TrueNAS connectivity:
```bash
foundry storage test
```

### API Operations

The TrueNAS API client supports:

- **Datasets**: Create, list, query datasets
- **NFS Shares**: Create, list, query NFS exports
- **Pools**: List available storage pools
- **Health**: Ping and version checks

For advanced operations, see the internal API client at `internal/truenas/client.go`.

## Future Storage Support

Foundry is designed to support additional storage backends in future phases:
- Rook/Ceph for distributed storage
- Local persistent volumes
- Other NFS/SMB providers
