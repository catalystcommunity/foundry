# TODO: `foundry backup local-restore`

Design for the symmetric counterpart to `foundry backup local`. **Not yet
implemented** — it must be developed and tested against a *throwaway* cluster, not
a production one, because restore is destructive and the cross-cluster path has
sharp edges (see Gotchas). This document is the implementation spec.

## Background: what `foundry backup local` does

`foundry backup local` (see `cmd/foundry/commands/backup/local.go`) streams a
Velero File System Backup — including PersistentVolume contents — to the machine
running foundry, leaving nothing lasting in the cluster:

1. Runs a local SeaweedFS S3 endpoint on `127.0.0.1`, data under
   `~/.foundry/<stack>_backups` (`localS3` in `locals3.go`).
2. Enables sshd `GatewayPorts` on one cluster node via a reversible drop-in +
   `systemd` dead-man auto-revert timer (`gatewayPortsManager` in
   `gatewayports.go`).
3. Opens a reverse SSH tunnel `nodeIP:port -> 127.0.0.1:port` over a connection
   established *after* GatewayPorts is enabled (`reverseTunnel` in
   `reversetunnel.go`).
4. Creates a temporary `BackupStorageLocation` (`foundry-local`) + credentials
   `Secret` pointing at the tunnel, waits for it to be `Available`
   (`createLocalBSL` / `waitForBSLAvailable` in `local.go`).
5. Runs the backup with `defaultVolumesToFsBackup: true` (kopia).
6. Tears everything down LIFO: delete BSL+secret -> close tunnel -> revert sshd ->
   stop local S3. The dead-man timer reverts sshd even if foundry dies.

The durable artifact is the SeaweedFS volume data in `~/.foundry/<stack>_backups`
(a kopia repository + Velero backup metadata). Restoring requires re-exposing that
store to the cluster and driving a Velero `Restore`.

## Goal

`foundry backup local-restore <backup-name>` should reconstitute a cluster (or
specific namespaces) from a local backup store produced by `foundry backup local`,
with the same "nothing lasting in the cluster, all via foundry" properties and the
same guaranteed-reversible sshd handling.

## Restore flow

Reuse the existing plumbing (`localS3`, `gatewayPortsManager`, `reverseTunnel`,
`createLocalBSL`, `waitForBSLAvailable`, `connectToHost`, `selectTunnelNode`). The
only new orchestration is "sync backups from the BSL" + "create a Restore".

1. **Preconditions**: velero installed; **node-agent deployed** (FSB restore needs
   it); the local store dir exists and is non-empty.
2. **Local S3 over existing data**: start `localS3` pointing at the *existing*
   `~/.foundry/<stack>_backups` — must **not** wipe it. `newLocalS3` already only
   `MkdirAll`s, so this is safe, but the generated S3 access/secret keys differ
   per run. That is fine: SeaweedFS S3 auth gates the endpoint, not the stored
   objects, so a fresh identity can still serve the old objects. The bucket
   (`velero`) already exists in the store; `createBucket` must tolerate
   "already exists" (today `s3.bucket.create` is idempotent enough; verify).
3. **GatewayPorts + reverse tunnel**: identical to backup (enable, fresh tunnel
   connection, dead-man timer).
4. **Recreate the BSL in ReadOnly mode**: `createLocalBSL` but with
   `spec.accessMode: ReadOnly` (add a parameter). ReadOnly prevents Velero from
   garbage-collecting or mutating the store and is the correct mode for restore.
5. **Wait for the backup to sync**: Velero's BSL controller periodically scans the
   bucket and creates `Backup` CRDs for backups it finds there (backup sync).
   After the BSL is `Available`, poll for the `Backup` named `<backup-name>` to
   appear (it will, because the backup metadata lives in the store). Optionally
   reduce the wait by setting a short `spec.backupSyncPeriod` on the BSL.
   - If the original `Backup` CR still exists but references a deleted BSL, delete
     it first so the synced one (bound to `foundry-local`) is authoritative.
6. **Create the Restore**: use `VeleroClient.CreateRestore(ctx, restoreName,
   backupName, RestoreOptions{...})` (already exists). Support
   `--namespace`/`--exclude-namespace`, `--restore-pvs`, and namespace remapping
   (`spec.namespaceMapping`) — add to `RestoreOptions` as needed.
7. **Wait for completion**: poll Restore `status.phase` (Completed / PartiallyFailed
   / Failed), mirroring `waitForBackup`.
8. **Teardown**: same LIFO as backup — delete BSL+secret, close tunnel (the fixed
   bounded `Close()`), revert sshd, stop local S3. Leave the local data intact.

## Suggested code structure

- New file `cmd/foundry/commands/backup/local_restore.go` with `LocalRestoreCommand`
  and `runLocalRestore`, mirroring `local.go`.
- Refactor the shared setup/teardown out of `local.go` into a helper, e.g.
  `type localTunnelSession struct { s3 *localS3; gp *gatewayPortsManager;
  tunnel *reverseTunnel; ... }` with `setup(ctx, ...)` and `teardown()`, so both
  `backup local` and `backup local-restore` use one well-tested path. Keep
  `createLocalBSL` parameterized by `accessMode` ("ReadWrite" | "ReadOnly").
- Register `LocalRestoreCommand` in `commands.go`.
- Tests: extend `gatewayports_test.go` / add `local_restore_test.go` for the pure
  pieces (BSL spec builder with ReadOnly, backup-sync polling logic with a fake
  dynamic client).

## Gotchas (the reasons to test off-cluster first)

- **kopia repo password (the big one for cross-cluster).** Velero FSB stores PV
  data in a kopia repository whose password lives in the `velero-repo-credentials`
  Secret in the `velero` namespace. Same-cluster restore "just works" because that
  secret is unchanged. **On a different/rebuilt cluster the repo is unreadable
  unless that secret matches the one used at backup time.** `local-restore` (or a
  companion step) must let the user supply/restore the original
  `velero-repo-credentials` before restoring volume data. Document and handle this
  explicitly; consider capturing that secret into the local store at backup time.
- **BackupRepository CRs**: Velero may need to (re)discover the `BackupRepository`
  for the namespace/repo; on a fresh cluster these are recreated, but they depend
  on the repo password above.
- **ReadOnly BSL**: restore must use a ReadOnly BSL so Velero doesn't try to write
  or GC the local store.
- **Backup sync timing**: don't assume the `Backup` CR exists immediately; wait for
  sync (or trigger it).
- **node-agent must be present** on the target cluster for FSB restore.
- **StorageClass / PV provisioning**: restored PVCs need a working StorageClass on
  the target; for cross-cluster, the class name must exist or be remapped.
- **Tunnel longevity**: restores of large volumes are long; rely on the fixed
  bounded `reverseTunnel.Close()` and the keepalive, and set a generous `--timeout`.
- **Idempotency**: re-running must tolerate an existing `foundry-local` BSL/secret
  and existing bucket.

## Testing plan (separate throwaway cluster)

1. Stand up a fresh foundry stack on a disposable cluster.
2. Create a couple of namespaces with small PVCs holding known data.
3. `foundry backup local` -> confirm local store populated.
4. Destroy the workloads/namespaces (and ideally rebuild the cluster to exercise
   the cross-cluster path, copying `velero-repo-credentials`).
5. `foundry backup local-restore <name>` -> confirm namespaces, objects, and
   **PV data contents** come back; confirm full teardown and `gatewayports no`.
6. Verify cross-cluster restore explicitly, since that is the path with the kopia
   password gotcha.

## Open questions

- Should `backup local` capture `velero-repo-credentials` into the local store
  automatically, so a restore is self-contained? (Leaning yes — store it alongside
  the backup, with appropriate file permissions.)
- Namespace remapping UX for restoring into a differently-named environment.
- Whether to offer a `--keep` "leave the BSL/tunnel up" mode for manual
  `velero restore` debugging, as `backup local` does.

---

# TODO: `foundry stack export` / `foundry stack import` (shareable credential bundle)

**Not yet implemented.** Package the sensitive bits of `~/.foundry/` into a single
gzipped tarball so a stack can be handed to another operator, and import it on the
other end. **Secure transfer of the tarball is explicitly out of scope** for this
CLI — we just produce/consume the artifact; how it's shared (age/gpg/Vault transit/
USB) is the operator's responsibility. We will, however, warn loudly that the
bundle contains secrets.

## What goes in the bundle

Selectable via `--include` (repeatable) or `--include all`:
- `kubeconfig` — `~/.foundry/kubeconfig` (contains client cert/key → admin access).
- `openbao` — `~/.foundry/openbao-keys/<cluster>/keys.json` (unseal keys + root
  token). **The most sensitive item.**
- `hostkeys` — `~/.foundry/keys/` (per-host SSH private keys).
- `config` — the stack config `~/.foundry/<stack>.yaml` (may itself contain inline
  secrets until they're migrated to OpenBAO; redact or include as-is with a warning).
- `all` — everything above.

Deliberately excluded by default: `*_backups/` (huge), `bin/` (re-downloadable),
`foundrybak/`.

## Command design

- `foundry stack export --include all -o <stack>-bundle.tar.gz`
  - Build a tar.gz containing only the selected files under a top-level dir named
    for the stack, plus a `manifest.json` (stack name, foundry version, contents
    list, created-at passed in — remember `Date.now()` is unavailable in workflow
    scripts but fine in normal Go; use `time.Now()`).
  - Preserve/record file modes so import can restore `0600` on keys/kubeconfig.
  - Refuse to write a world-readable bundle; create the output `0600`.
  - Print a prominent warning listing the secrets included.
- `foundry stack import <bundle.tar.gz> [--target ~/.foundry] [--force]`
  - Validate the manifest; show what will be written and to where.
  - Extract into the target, restoring file modes (force `0600` on
    keys/kubeconfig/openbao regardless of what's in the tar).
  - Refuse to overwrite existing files unless `--force`; never follow symlinks or
    write outside the target (zip-slip protection: reject `..`/absolute paths).

## Gotchas / notes

- **Permissions**: always re-apply restrictive modes on import; never trust the
  tar's modes for secret files.
- **Path safety**: sanitize entry names (no absolute paths, no `..`) — classic
  tar-extraction vulnerability.
- **Cluster name coupling**: openbao-keys and dashboards dirs are keyed by stack
  name; importing under a different stack name needs remapping (document or
  support `--as <stackname>`).
- **Partial bundles**: importing a bundle that lacks `config` should still work
  (e.g. share only kubeconfig); don't assume all parts are present.
- **No encryption in-tool** (by design). Consider documenting a recommended
  `age`/`gpg` wrapper command in the help text so users don't ship plaintext.
