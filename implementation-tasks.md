# Foundry Implementation Tracking

This document provides high-level tracking of implementation phases. Each phase has detailed tasks in its own `phase-implementation-N.md` file.

## Implementation Philosophy

Each phase builds upon the previous one, with the following principles:
- Every task results in a working, testable state
- Tests are written alongside implementation (happy path + error paths)
- Integration tests use containers for all components we can run locally
- Semantic versioning and conventional commits throughout
- Clear separation of concerns in the codebase

## Phase Status Overview

| Phase | Status | Description | Milestone File |
|-------|--------|-------------|----------------|
| Phase 1 | ✅ Complete (100%) | Foundation - CLI structure, config, SSH | [phase-implementation-1.md](./phase-implementation-1.md) |
| Phase 2 | ✅ Complete (100%, 54/54 tasks) | Stack Installation - Core components | [phase-implementation-2.md](./phase-implementation-2.md) |
| Phase 3 | Not Started | Observability & Storage | [phase-implementation-3.md](./phase-implementation-3.md) |
| Phase 4 | Not Started | RBAC & Operations | [phase-implementation-4.md](./phase-implementation-4.md) |
| Phase 5 | Not Started | Polish & Documentation | [phase-implementation-5.md](./phase-implementation-5.md) |
| Phase 6 | Not Started | Service Creation (Optional) | [phase-implementation-6.md](./phase-implementation-6.md) |

## Current Phase

**Active Phase**: Phase 3 - Observability & Storage (Not Started)

**Completed Phases**:
- Phase 1 - Foundation ✅ **COMPLETE**
- Phase 2 - Stack Installation ✅ **COMPLETE**

## Phase Completion Criteria

### Phase 1: Foundation ✅ **COMPLETE**
- [x] CLI foundation established with urfave/cli v3
- [x] Config files can be validated and parsed (83.3% test coverage)
- [x] Secret resolution works from ~/.foundryvars and env vars (89.1% test coverage)
- [x] Secret reference parsing and validation complete
- [x] Instance-scoped secret resolution implemented
- [x] SSH connections can be established and managed - 62.7% coverage
- [x] Host management types and in-memory registry complete (100% coverage)
- [x] Host management CLI commands (add, configure, list) work (Tasks 23-25)
- [x] CLI commands for config operations (Tasks 19-22) - 77.3% test coverage
- [x] Integration tests created with testcontainers (Task 26)
- [x] Comprehensive documentation written (Task 27)
- [x] User can manage hosts and configuration end-to-end

**Progress**: All 27 tasks complete (100%)

### Phase 2: Stack Installation ✅ **COMPLETE** (54/54 tasks, 100%)
- [x] Network planning utilities work (MAC detection, IP validation, DHCP guidance) (93.9% coverage)
- [x] Network planning commands (`foundry network plan/detect-macs/validate`) fully functional
- [x] Setup wizard (`foundry setup`) works with state tracking and resume capability (85.7% coverage)
- [x] Setup state management tracks progress and enables resume (90.9% coverage)
- [x] Network and DNS configuration types with validation (85.1% coverage)
- [x] Component installation framework with registry and dependency resolution (98.0% coverage)
- [x] Container runtime helpers (Docker/Podman support via SSH) (93.7% coverage)
- [x] Systemd service management (87.2% coverage)
- [x] OpenBAO can be installed as container on infrastructure hosts (82.9% coverage)
- [x] OpenBAO initialization and unseal works (82.9% coverage)
- [x] OpenBAO secret resolution from KV v2 API (85.8% coverage)
- [x] OpenBAO auth token management with OS keyring + file fallback (85.8% coverage)
- [x] PowerDNS can be installed as container on infrastructure hosts (57.4% coverage)
- [x] PowerDNS HTTP API client works for zone and record management (57.4% coverage)
- [x] Split-horizon DNS logic implemented (68.1% coverage)
- [x] DNS zones can be created (infrastructure and kubernetes) (68.1% coverage)
- [x] Infrastructure DNS initialization (openbao, dns, zot, truenas, k8s A records) (69.9% coverage)
- [x] Kubernetes DNS initialization (wildcard record for ingress) (69.9% coverage)
- [x] DNS management commands (zone list/create/delete, record add/list/delete, dns test) (CLI with tests)
- [x] OpenBAO SSH key storage implementation (85-100% per-function coverage)
- [x] Zot registry can be installed as container on infrastructure hosts (90.2% coverage)
- [x] Zot configured with pull-through cache for Docker Hub (part of config generation)
- [x] Zot configured with optional TrueNAS storage backend (part of installation logic)
- [x] K3s token generation for cluster and agent tokens (91.7% coverage)
- [x] VIP configuration validation and kube-vip manifest generation (95.0% coverage)
- [x] K3s control plane installation with VIP, DNS, and registry configuration (92.2% coverage)
- [x] Component registry and CLI commands (82.3% coverage for commands, 70.0% for registry)
- [x] `foundry component list` command works
- [x] `foundry component status <name>` command works
- [x] `foundry component install <name>` command works with dependency checking
- [x] K8s client implemented with client-go (82.1% coverage)
- [x] Helm integration complete with SDK wrapper (75.0% coverage)
- [x] Contour ingress controller deployed via Helm (90.8% coverage)
- [x] cert-manager deployed via Helm with ClusterIssuer support (93.3% coverage)
- [x] `foundry cluster init` command works with dry-run mode (33.8% coverage)
- [x] `foundry cluster node list` command works (100% coverage for core logic)
- [x] `foundry cluster status` command works (100% coverage for health analysis)
- [x] `foundry stack install` command works with dry-run mode (51.9% coverage, 9 tests)
- [x] `foundry stack status` command works with health checks (100% coverage for core logic)
- [x] `foundry stack validate` command works with comprehensive validation (100% coverage on validation logic, 8 test suites, 30+ test cases)
- [x] Component dependencies are automatically resolved via dependency resolution system
- [x] TrueNAS API client complete (96.8% coverage, datasets, NFS shares, pools, ping)
- [x] OpenBAO integration test complete (Task 42.1 - container lifecycle, secrets, SSH keys, resolver, auth tokens - 6 test scenarios, all passing)
- [x] PowerDNS integration test complete (Task 42.2 - 9 test scenarios: zone creation, record management, infrastructure/kubernetes DNS initialization, zone/record deletion - all passing)
- [x] Zot integration test complete (Task 42.3 - 7 test scenarios: health, catalog, manifest upload/retrieval, tags, deletion - all passing)
- [x] K3s integration test complete (Task 42.4 - Kind-based cluster, kubeconfig retrieval/storage, K8s client from OpenBAO, health checks, node operations, token storage - 8 test scenarios, all passing)
- [x] Helm integration test complete (Task 42.5 - Kind cluster, repo add, chart install/upgrade/uninstall, release listing, namespace creation, error handling - 8 test scenarios, all passing)
- [x] Full stack integration test complete (Task 42.6 - All 4 phases: OpenBAO + PowerDNS + Zot + K3s + Contour + cert-manager, 18-step end-to-end workflow, all passing in 101s)
- [x] Documentation created for Phase 2 (Task 43 - installation.md, components.md, dns.md, storage.md)
- [ ] Integration tests run in CI pipeline
- [x] User can deploy a working cluster with DNS, registry, and secrets management (validated via integration tests)

### Phase 3: Observability & Storage ✗
- [x] TrueNAS API integration complete (96.8% coverage)
- [ ] CSI drivers deployed for persistent storage
- [ ] MinIO deployed when needed
- [ ] Prometheus, Loki, Grafana deployed and configured
- [ ] External-DNS manages DNS records automatically (using PowerDNS API)
- [ ] Velero configured for backups
- [ ] Stack status command shows all component health
- [ ] Integration tests verify storage and observability
- [ ] User has full observability and storage capabilities

### Phase 4: RBAC & Operations ✗
- [ ] Users can be created in OpenBAO
- [ ] OIDC provider configured for K8s auth
- [ ] K8s RBAC integrated with OpenBAO
- [ ] Service accounts can be created
- [ ] Components can be upgraded safely
- [ ] Stack upgrades work with dry-run
- [ ] ArgoCD can be deployed (optional)
- [ ] Integration tests cover RBAC scenarios
- [ ] User can manage multi-user environments

### Phase 5: Polish & Documentation ✗
- [ ] Comprehensive user guide written
- [ ] Operator guide for troubleshooting
- [ ] Interactive wizards for complex operations
- [ ] Shell completion for bash/zsh/fish
- [ ] Binary releases for Linux/macOS/Windows
- [ ] Migration guide from manual setups
- [ ] Error messages are clear and actionable
- [ ] Project is ready for public use

### Phase 6: Service Creation (Optional) ✗
- [ ] Copier templates created for Go, Python, Rust, JS
- [ ] `foundry service create` generates working projects
- [ ] `foundry tool create` generates CLI tools
- [ ] Template upgrades preserve user changes
- [ ] Generated services include Helm charts
- [ ] CI/CD templates work out of the box
- [ ] Grafana dashboards auto-generated
- [ ] User can scaffold and deploy new services in <1 hour

## Development Guidelines

### Testing Requirements
- Write tests for every feature (happy path + error paths)
- Mock only third-party APIs we can't run locally
- Use testcontainers for OpenBAO, databases, etc.
- Use Kind for Kubernetes integration tests
- Ensure all tests pass before marking tasks complete

### Code Organization
- Follow separation of concerns
- Keep command handlers thin (delegate to packages)
- Shared logic goes in `internal/` packages
- External-facing APIs in `pkg/` if needed
- Configuration in `~/.foundry/` directory

### Commit Practices
- Use semantic versioning
- Follow conventional commits
- Each commit should be a working state
- Test before committing

### Configuration
- No .env files (use YAML configs)
- Secrets never in config (use ${secret:path:key} references)
- Validate all configs on load

## Quick Reference

### Starting Phase 1
```bash
# Set up Go project
go mod init github.com/catalystcommunity/foundry/v1

# Install key dependencies
go get github.com/urfave/cli/v2
go get gopkg.in/yaml.v3
go get github.com/stretchr/testify

# See phase-implementation-1.md for first tasks
```

### Running Tests
```bash
# Unit tests
go test ./...

# Integration tests
go test -tags=integration ./...

# With coverage
go test -cover ./...
```

### Build
```bash
# Development build
go build -o foundry ./cmd/foundry

# Production build (static binary)
CGO_ENABLED=0 go build -ldflags="-s -w" -o foundry ./cmd/foundry
```

## Notes

- Phases are not time-bound; proceed at natural pace
- Each phase file contains detailed task breakdown
- Update phase status as work progresses
- Mark milestones complete when all criteria met
- Refer to DESIGN.md for architectural decisions

---

**Last Updated**: 2025-11-08
**Current Status**: Phase 2 ✅ **COMPLETE** - 54/54 tasks complete (100%, All tasks 0.1-43)
  - Setup State Management (90.9% coverage)
  - Setup Wizard Framework (85.7% coverage)
  - Network & DNS Configuration Types (85.1% coverage)
  - Network Detection Utilities (98.6% coverage)
  - Network Validation (93.9% coverage)
  - Network Planning Commands (CLI with tests)
  - Component Installation Framework (98.0% coverage)
  - Container Runtime Helpers (93.7% coverage)
  - Systemd Service Management (87.2% coverage)
  - OpenBAO Container Installation (82.9% coverage)
  - OpenBAO Initialization & Unseal (82.9% coverage)
  - OpenBAO Secret Resolution (85.8% coverage)
  - OpenBAO Auth Token Management (85.8% coverage)
  - PowerDNS Container Installation (57.4% coverage)
  - PowerDNS HTTP API Client (57.4% coverage)
  - Split-Horizon DNS Logic (68.1% coverage)
  - DNS Zone Management (68.1% coverage)
  - Infrastructure DNS Initialization (69.9% coverage)
  - Kubernetes DNS Initialization (69.9% coverage)
  - DNS Management Commands (CLI with tests)
  - OpenBAO SSH Key Storage Implementation (85-100% per-function coverage)
  - Zot Container Installation (90.2% coverage, pull-through cache, TrueNAS support)
  - K3s Token Generation (91.7% coverage, 25 test cases)
  - VIP Configuration (95.0% coverage, 13 test functions, 58 test cases)
  - K3s Control Plane Installation (92.2% coverage, comprehensive tests)
  - K3s Node Role Determination (100% coverage, 50+ test cases)
  - K8s Client (82.1% coverage, comprehensive tests with fake clientset)
  - Helm Integration (75.0% coverage, 27 tests)
  - Contour Ingress Controller (90.8% coverage, 21 tests)
  - cert-manager Deployment (93.3% coverage, 31 tests)
  - Component Registry (70.0% coverage, 4 test suites)
  - Component CLI Commands (82.3% coverage, 3 command test suites)
  - Cluster Init Command (33.8% coverage, dry-run mode complete)
  - Cluster Node List Command (100% coverage for core logic)
  - Cluster Status Command (100% coverage for health analysis, 93.8% for display)
  - Stack Install Command (51.9% coverage, dry-run mode complete, 9 tests)
  - Stack Status Command (100% coverage for core logic, comprehensive health checks)
  - Stack Validate Command (100% coverage on validation logic, 85.7% on dependency validation, 8 test suites, 30+ test cases)
  - TrueNAS API Client (96.8% coverage, datasets, NFS shares, pools, ping endpoint)
  - Storage Configure Command (all tests passing, interactive prompts, connection testing)
  - Storage List Command (all tests passing, pool formatting, secret resolution)
  - Storage Test Command (all tests passing, full test mode with dataset creation/deletion)
  - OpenBAO Integration Test (6 test scenarios - health, secrets, SSH keys, resolver, auth tokens, deletion - all passing)
  - PowerDNS Integration Test (9 test scenarios - all passing)
  - Zot Integration Test (7 test scenarios - all passing)
  - K3s Integration Test (8 test scenarios - Kind cluster, kubeconfig, OpenBAO storage, health checks, node ops - all passing)
  - Helm Integration Test (8 test scenarios - Kind cluster, repo add, chart install/upgrade/uninstall, release listing, namespace creation, error handling - all passing)
