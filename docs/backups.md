# Backups

Foundry uses Velero for cluster backup and restore operations, with SeaweedFS providing S3-compatible storage.

## Architecture

```
┌─────────────────────────────────────────────────────────────────────┐
│  Velero                                                              │
│  - Cluster state backups                                            │
│  - Scheduled backups                                                │
│  - Disaster recovery                                                │
└─────────────────────────────────────────────────────────────────────┘
                              │
                              ▼
┌─────────────────────────────────────────────────────────────────────┐
│  SeaweedFS S3 API                                                    │
│  - Backup storage bucket                                            │
│  - Replicated across nodes via Longhorn                             │
└─────────────────────────────────────────────────────────────────────┘
```

## What Gets Backed Up

Velero backs up:
- Kubernetes resources (Deployments, Services, ConfigMaps, Secrets, etc.)
- Persistent Volume data (via Longhorn snapshots or restic)
- Custom Resource Definitions and instances

## Configuration

```yaml
velero:
  provider: s3
  s3_endpoint: http://seaweedfs-s3.seaweedfs.svc.cluster.local:8333
  s3_bucket: velero
  s3_region: us-east-1
  backup_retention_days: 30
  schedule_cron: "0 2 * * *"  # Daily at 2 AM
  schedule_excluded_namespaces:
    - kube-system
    - velero
```

## Commands

### Create Backup

```bash
# Create a manual backup
foundry backup create

# Create a named backup
foundry backup create --name pre-upgrade-backup

# Backup specific namespaces
foundry backup create --namespaces default,production
```

### List Backups

```bash
foundry backup list
```

Output:
```
NAME                          STATUS      CREATED                  EXPIRES
daily-backup-20241201020000   Completed   2024-12-01 02:00:00     2024-12-31
daily-backup-20241130020000   Completed   2024-11-30 02:00:00     2024-12-30
pre-upgrade-backup            Completed   2024-11-29 14:30:00     2024-12-29
```

### Restore from Backup

```bash
# Restore entire backup
foundry backup restore daily-backup-20241201020000

# Restore specific namespaces
foundry backup restore daily-backup-20241201020000 --namespaces default

# Restore to different namespace
foundry backup restore daily-backup-20241201020000 --namespace-mappings old-ns:new-ns
```

### Schedule Backups

```bash
# Create a daily backup schedule
foundry backup schedule --cron "0 2 * * *"

# Create weekly backup schedule
foundry backup schedule --cron "0 3 * * 0" --name weekly-backup

# List schedules
foundry backup schedule list
```

### Delete Backup

```bash
foundry backup delete pre-upgrade-backup
```

## Scheduled Backups

By default, Foundry creates a daily backup schedule that runs at 2 AM.

**Default Schedule:**
- Name: `daily-backup`
- Cron: `0 2 * * *` (daily at 2 AM)
- Retention: 30 days
- Excluded namespaces: `kube-system`, `velero`

### Customize Schedule

Edit the Velero configuration in your stack.yaml:

```yaml
velero:
  schedule_name: daily-backup
  schedule_cron: "0 2 * * *"
  backup_retention_days: 30
  schedule_excluded_namespaces:
    - kube-system
    - velero
    - monitoring  # Add more exclusions
```

## Disaster Recovery

### Complete Cluster Restore

If you need to restore to a new cluster:

1. Install Foundry on new cluster with same SeaweedFS storage configuration
2. Wait for Velero to connect to backup storage
3. List available backups:
   ```bash
   foundry backup list
   ```
4. Restore the most recent backup:
   ```bash
   foundry backup restore <backup-name>
   ```

### Partial Restore

Restore only specific resources:

```bash
# Restore only deployments
velero restore create --from-backup daily-backup --include-resources deployments

# Restore specific namespace
velero restore create --from-backup daily-backup --include-namespaces production

# Restore with label selector
velero restore create --from-backup daily-backup --selector app=critical
```

## Backup Storage

Backups are stored in SeaweedFS, which runs on Longhorn PVCs:

- **Bucket**: `velero` (created automatically)
- **Path style**: Enabled (required for S3-compatible storage)
- **Region**: `us-east-1` (default)

### Verify Storage

```bash
# Check SeaweedFS S3 connectivity
kubectl -n seaweedfs port-forward svc/seaweedfs-s3 8333:8333

# List buckets
aws --endpoint-url http://localhost:8333 s3 ls

# List backup contents
aws --endpoint-url http://localhost:8333 s3 ls s3://velero/
```

## Volume Snapshots

For persistent volume data, Velero can use:

1. **Longhorn snapshots** (recommended): Native volume snapshots via CSI
2. **Restic/Kopia**: File-level backups for any storage class

### Enable CSI Snapshots

Longhorn includes VolumeSnapshotClass support. Velero uses this automatically when available:

```yaml
velero:
  snapshots_enabled: true
  csi_snapshot_timeout: 10m
```

## Troubleshooting

### Check Velero Status

```bash
kubectl -n velero get pods
kubectl -n velero logs deployment/velero
```

### Check Backup Status

```bash
# Get backup details
velero backup describe <backup-name>

# Get backup logs
velero backup logs <backup-name>
```

### Check Restore Status

```bash
# Get restore details
velero restore describe <restore-name>

# Get restore logs
velero restore logs <restore-name>
```

### Common Issues

**Backup stuck in "InProgress":**
1. Check Velero logs: `kubectl -n velero logs deployment/velero`
2. Verify S3 connectivity
3. Check for resource errors in backup describe output

**Restore fails:**
1. Check restore logs for specific errors
2. Verify target namespace doesn't have conflicting resources
3. Check RBAC permissions

**S3 connection errors:**
1. Verify SeaweedFS is running: `kubectl -n seaweedfs get pods`
2. Check credentials in velero secret: `kubectl -n velero get secret cloud-credentials -o yaml`
3. Test S3 connectivity from velero pod

### Verify Backup Integrity

```bash
# List backup contents
velero backup describe <backup-name> --details

# Check for warnings/errors
velero backup describe <backup-name> | grep -E "(Warning|Error)"
```

## Best Practices

1. **Test restores regularly**: Don't wait for a disaster to test your backups
2. **Monitor backup jobs**: Set up alerts for failed backups
3. **Exclude unnecessary namespaces**: Don't backup system namespaces that will be recreated
4. **Document restore procedures**: Keep runbooks updated
5. **Offsite backups**: Consider replicating SeaweedFS to external storage for true disaster recovery
