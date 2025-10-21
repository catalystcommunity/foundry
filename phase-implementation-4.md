# Phase 4: RBAC & Operations - Implementation Tasks

**Goal**: Multi-user management, operations tooling, and day-2 operational tasks

**Milestone**: User can manage teams, perform upgrades, and operate production clusters safely

## Prerequisites

Phase 3 must be complete:
- ✓ Full observability stack
- ✓ Storage provisioning
- ✓ Backup and restore capabilities

## High-Level Task Areas

### 1. OpenBAO OIDC Provider

**Working States**:
- [ ] OpenBAO configured as OIDC identity provider
- [ ] Can create users in OpenBAO
- [ ] Users get authentication tokens
- [ ] Tokens can be used for K8s auth

**Key Tasks**:
- Configure OpenBAO OIDC plugin
- Create authentication endpoints
- Set up token issuance
- Configure token TTLs and policies
- Create helper functions for user management
- Write integration tests

**Files**:
- `internal/component/openbao/oidc.go`
- `internal/component/openbao/users.go`

---

### 2. Kubernetes RBAC Integration

**Working States**:
- [ ] K8s configured to use OpenBAO OIDC
- [ ] Users authenticated via OIDC can access K8s
- [ ] Namespace-scoped permissions work
- [ ] Cluster-admin role can be granted

**Key Tasks**:
- Configure K8s API server for OIDC auth
- Create ClusterRole and Role templates
- Create ClusterRoleBindings
- Implement namespace-scoped permission grants
- Test with real users
- Document RBAC model

**Files**:
- `internal/k8s/rbac.go`
- `internal/k8s/oidc.go`

---

### 3. User Management

**Working States**:
- [ ] Can create users in OpenBAO
- [ ] Can grant K8s permissions to users
- [ ] Can list users and their permissions
- [ ] Can revoke user access

**Key Tasks**:
- `foundry rbac user create <username>`
- `foundry rbac user grant <username> --namespace NS --role ROLE`
- `foundry rbac user revoke <username> --namespace NS`
- `foundry rbac user list [--namespace NS]`
- Generate kubeconfig for users
- Test multi-user scenarios

**Files**:
- `cmd/foundry/commands/rbac/user.go`
- `internal/rbac/users.go`

---

### 4. Service Account Management

**Working States**:
- [ ] Can create K8s service accounts
- [ ] Can grant permissions to service accounts
- [ ] Can retrieve service account tokens
- [ ] Service accounts can be used by applications

**Key Tasks**:
- `foundry rbac serviceaccount create <name> --namespace NS`
- `foundry rbac serviceaccount grant <name> --permissions PERMS`
- `foundry rbac serviceaccount token <name>`
- Integration with OpenBAO for token storage
- Test service account access

**Files**:
- `cmd/foundry/commands/rbac/serviceaccount.go`
- `internal/rbac/serviceaccounts.go`

---

### 5. Component Upgrade System

**Working States**:
- [ ] Can check current vs available component versions
- [ ] Can upgrade components safely
- [ ] Dry-run mode shows what would change
- [ ] Rollback on failure

**Key Tasks**:
- Implement version checking for all components
- Create upgrade plan generation
- Implement dry-run for upgrades
- Handle component-specific upgrade procedures
- Test upgrade paths
- Document upgrade process

**Files**:
- `internal/component/upgrade.go`
- `internal/component/versioning.go`
- `cmd/foundry/commands/component/upgrade.go`

---

### 6. Stack Upgrade

**Working States**:
- [ ] Can upgrade entire stack
- [ ] Respects component dependencies
- [ ] Shows upgrade plan
- [ ] Confirms before proceeding
- [ ] Handles partial failures

**Key Tasks**:
- `foundry stack upgrade [--dry-run] [--yes]`
- Generate comprehensive upgrade plan
- Check for breaking changes
- Perform rolling upgrades where possible
- Verify stack health after upgrade
- Create upgrade reports

**Files**:
- `cmd/foundry/commands/stack/upgrade.go`
- `internal/stack/upgrade.go`

---

### 7. Secret Management Commands

**Working States**:
- [ ] Can set secrets interactively
- [ ] Can get secrets (with confirmation)
- [ ] Can list secret paths
- [ ] Can rotate secrets

**Key Tasks**:
- `foundry secret set <instance>/<path>:<key>` - Interactive prompt
- `foundry secret get <instance>/<path>:<key>` - With confirmation
- `foundry secret list [instance]/[path]` - List available secrets
- `foundry secret rotate <instance>/<path>:<key>` - Rotate and update services
- Handle sensitive data securely
- Audit secret access

**Files**:
- `cmd/foundry/commands/secret/*.go`
- `internal/secrets/management.go`

---

### 8. ArgoCD (Optional Component)

**Working States**:
- [ ] ArgoCD can be deployed
- [ ] Configured to use Zot registry
- [ ] RBAC integrated with OpenBAO
- [ ] Application templates available

**Key Tasks**:
- Deploy ArgoCD via Helm
- Configure registry integration
- Set up OIDC auth with OpenBAO
- Create application templates
- Document GitOps workflow

**Files**:
- `internal/component/argocd/install.go`
- `internal/component/argocd/apps.go`

---

### 9. Health Checks & Diagnostics

**Working States**:
- [ ] Can run health checks on stack
- [ ] Can diagnose common issues
- [ ] Can generate diagnostic reports

**Key Tasks**:
- `foundry stack health` - Comprehensive health check
- `foundry diagnose` - Run diagnostics and suggest fixes
- `foundry stack report` - Generate status report
- Check network connectivity
- Check resource usage
- Identify misconfigurations

**Files**:
- `cmd/foundry/commands/stack/health.go`
- `cmd/foundry/commands/diagnose.go`
- `internal/diagnostics/*.go`

---

### 10. Dry-Run Mode

**Working States**:
- [ ] All destructive operations support --dry-run
- [ ] Dry-run shows what would change
- [ ] No actual changes made in dry-run

**Key Tasks**:
- Add --dry-run flag to all applicable commands
- Implement dry-run logic for each operation
- Show clear output of planned changes
- Test dry-run accuracy

**Files**:
- Updates to all command files

---

### 11. CLI Commands Summary

**New Commands**:
```bash
# RBAC
foundry rbac user create <username>
foundry rbac user grant <username> --namespace NS --role ROLE
foundry rbac user revoke <username> --namespace NS
foundry rbac user list [--namespace NS]
foundry rbac serviceaccount create <name> --namespace NS
foundry rbac serviceaccount grant <name> --permissions PERMS

# Secrets
foundry secret set <instance>/<path>:<key>
foundry secret get <instance>/<path>:<key>
foundry secret list [instance]/[path]
foundry secret rotate <instance>/<path>:<key>

# Upgrades
foundry component upgrade <name> [--dry-run]
foundry stack upgrade [--dry-run] [--yes]

# Operations
foundry stack health
foundry diagnose
foundry stack report

# ArgoCD (optional)
foundry component install argocd
```

---

### 12. Integration Tests

**Test Scenarios**:
- [ ] Create user and grant permissions
- [ ] User can access assigned namespaces
- [ ] User cannot access other namespaces
- [ ] Service accounts work correctly
- [ ] Component upgrades succeed
- [ ] Stack upgrade succeeds
- [ ] Secret rotation works
- [ ] Dry-run doesn't make changes

**Files**:
- `test/integration/phase4_rbac_test.go`
- `test/integration/phase4_upgrade_test.go`
- `test/integration/phase4_secrets_test.go`

---

### 13. Documentation

**Documents to Create/Update**:
- [ ] `docs/rbac.md` - User and service account management
- [ ] `docs/upgrades.md` - Upgrade procedures and best practices
- [ ] `docs/operations.md` - Day-2 operations guide
- [ ] `docs/secrets-management.md` - Secret lifecycle management
- [ ] `docs/troubleshooting.md` - Common issues and solutions
- [ ] `docs/multi-user.md` - Multi-user setup guide

---

## Phase 4 Completion Criteria

- [ ] Users can be created and managed
- [ ] K8s RBAC integrated with OpenBAO
- [ ] Service accounts work correctly
- [ ] Components can be upgraded safely
- [ ] Stack upgrades work end-to-end
- [ ] Secret management is complete
- [ ] ArgoCD deploys successfully (if tested)
- [ ] Health checks and diagnostics work
- [ ] Dry-run mode works for all operations
- [ ] All tests pass
- [ ] Documentation complete

## Manual Verification

```bash
# Create user
foundry rbac user create alice --namespace default --role edit

# Grant additional permissions
foundry rbac user grant alice --namespace production --role view

# List users
foundry rbac user list

# Upgrade component
foundry component upgrade prometheus --dry-run
foundry component upgrade prometheus

# Upgrade stack
foundry stack upgrade --dry-run
foundry stack upgrade --yes

# Check health
foundry stack health

# Manage secrets
foundry secret set myapp-prod/database/main:password
foundry secret list myapp-prod
foundry secret rotate myapp-prod/database/main:password
```

---

**Estimated Working States**: ~25 testable states
**Estimated LOC**: ~3500-5000 lines
**Timeline**: Not time-bound
