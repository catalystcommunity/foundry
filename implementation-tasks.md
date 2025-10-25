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
| Phase 2 | Not Started | Stack Installation - Core components | [phase-implementation-2.md](./phase-implementation-2.md) |
| Phase 3 | Not Started | Observability & Storage | [phase-implementation-3.md](./phase-implementation-3.md) |
| Phase 4 | Not Started | RBAC & Operations | [phase-implementation-4.md](./phase-implementation-4.md) |
| Phase 5 | Not Started | Polish & Documentation | [phase-implementation-5.md](./phase-implementation-5.md) |
| Phase 6 | Not Started | Service Creation (Optional) | [phase-implementation-6.md](./phase-implementation-6.md) |

## Current Phase

**Active Phase**: Phase 1 - Foundation ✅ **COMPLETE**

**Next Phase**: Phase 2 - Stack Installation (OpenBAO, K3s, Zot)

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

### Phase 2: Stack Installation ✗
- [ ] OpenBAO can be installed on remote hosts
- [ ] K3s cluster can be initialized (single-node and HA)
- [ ] Zot registry can be deployed to K3s
- [ ] K3s configured to use Zot as default registry
- [ ] Nodes can be added/removed from cluster
- [ ] Component dependencies are automatically resolved
- [ ] `foundry stack install` deploys full basic stack
- [ ] Integration tests use Kind/K3s for K8s testing
- [ ] User can deploy a working cluster with registry

### Phase 3: Observability & Storage ✗
- [ ] TrueNAS API integration complete
- [ ] CSI drivers deployed for persistent storage
- [ ] MinIO deployed when needed
- [ ] Prometheus, Loki, Grafana deployed and configured
- [ ] External-DNS manages DNS records automatically
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
go mod init github.com/catalystcommunity/foundry

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

**Last Updated**: 2025-10-21
**Current Status**: Phase 1 COMPLETE - All 27 tasks finished with comprehensive tests and documentation
