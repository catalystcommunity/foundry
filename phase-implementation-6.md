# Phase 6: Service Creation (Optional) - Implementation Tasks

**Goal**: Scaffold and deploy new services quickly with best practices baked in

**Milestone**: User can create and deploy a new service to their cluster in <1 hour

**Status**: OPTIONAL - Core stack is fully functional without this phase. This will also absolutely need a lot more fleshing out for templates, as we are more prescriptive than this boilerplate state.

## Prerequisites

Phases 1-5 must be complete:
- ✓ Full stack deployed and operational
- ✓ RBAC and operations working
- ✓ Documentation complete
- ✓ Project is production-ready

## Important Note

This phase is **optional** and serves a specific subset of users who want opinionated service scaffolding. Many users will be perfectly happy deploying their own services using standard Kubernetes/Helm workflows.

**Do not implement this phase until**:
1. Phases 1-5 are complete and battle-tested
2. There is demonstrated user demand
3. Resources are available to maintain templates
4. TodPunk has revised it and approved the end goals.

## High-Level Task Areas

### 1. Copier Integration

**Working States**:
- [ ] Can invoke Copier templates
- [ ] Templates can be versioned and upgraded
- [ ] User changes are preserved during upgrades

**Key Tasks**:
- Integrate Copier as a library or CLI wrapper
- Create template repository structure
- Implement template versioning
- Handle template updates without losing user changes
- Test upgrade scenarios

**Files**:
- `internal/template/copier.go`
- `internal/template/upgrade.go`

---

### 2. Go Service Template

**Template Structure**:
```
foundry-template-go/
├── copier.yml                    # Copier configuration
├── {{ service_name }}/
│   ├── cmd/
│   │   ├── server/               # HTTP server (if service)
│   │   ├── cli/                  # CLI tool
│   │   └── migrate/              # Database migrations (optional)
│   ├── internal/                 # Private application code
│   │   ├── api/                  # API handlers
│   │   ├── service/              # Business logic
│   │   └── repository/           # Data access
│   ├── pkg/                      # Public library code
│   ├── .foundry/                 # Managed by Foundry (not user-editable)
│   │   ├── Dockerfile
│   │   ├── helm/                 # Helm chart
│   │   ├── grafana/              # Dashboards
│   │   └── ci/                   # GitHub Actions
│   ├── configs/                  # Configuration files
│   ├── test/                     # Tests
│   ├── go.mod
│   ├── go.sum
│   ├── Makefile
│   └── README.md
```

**Key Tasks**:
- Create complete Go project template
- Include HTTP server with health/ready/metrics endpoints
- Include CLI tool scaffolding
- Include library structure
- Dockerfile with multi-stage builds
- Helm chart with Foundry conventions
- Grafana dashboard template
- Prometheus ServiceMonitor
- GitHub Actions workflow (test, build, push, deploy)
- OpenBAO client initialization
- Structured logging setup
- Config loading with secret resolution

**Template Repository**:
- `https://github.com/catalystcommunity/foundry-template-go`

---

### 3. Python Service Template

**Similar structure to Go template**

**Key Differences**:
- Poetry or pip for dependencies
- FastAPI or Flask for HTTP server
- Click for CLI tool
- pytest for testing
- Dockerfile optimized for Python

**Template Repository**:
- `https://github.com/catalystcommunity/foundry-template-python`

---

### 4. Rust Service Template

**Similar structure to Go template**

**Key Differences**:
- Cargo for build system
- Actix or Axum for HTTP server
- Clap for CLI tool
- Dockerfile optimized for Rust (build caching)

**Template Repository**:
- `https://github.com/catalystcommunity/foundry-template-rust`

---

### 5. JavaScript/TypeScript Service Template

**Similar structure to Go template**

**Key Differences**:
- npm or yarn for dependencies
- Express or Fastify for HTTP server
- Commander for CLI tool
- Jest for testing
- Node.js Dockerfile

**Template Repository**:
- `https://github.com/catalystcommunity/foundry-template-js`

---

### 6. CLI Command: `foundry service create`

**Working State**: Can scaffold a new service from template

**Usage**:
```bash
foundry service create myapp --lang go
foundry service create myapp --lang python --no-server --cli-only
foundry service create myapp --lang rust --template-version v1.2.0
```

**Key Tasks**:
- `foundry service create <name> --lang <go|python|rust|js>`
- Interactive prompts for configuration:
  - Service type (HTTP server, CLI tool, library, or all)
  - Database (none, PostgreSQL, MySQL, etc.)
  - Message queue (none, RabbitMQ, NATS, etc.)
  - Deployment namespace
  - Domain name
- Clone and customize template
- Create namespace in OpenBAO for secrets
- Generate initial secret paths
- Initialize git repository
- Run initial tests

**Files**:
- `cmd/foundry/commands/service/create.go`
- `internal/service/scaffold.go`

---

### 7. CLI Command: `foundry service upgrade-template`

**Working State**: Can upgrade service scaffolding without losing user changes

**Usage**:
```bash
foundry service upgrade-template ./myapp
foundry service upgrade-template ./myapp --to-version v1.3.0
```

**Key Tasks**:
- Detect current template version
- Fetch new template version
- Apply changes only to `.foundry/` managed directory
- Detect conflicts in hybrid files
- Show upgrade summary
- Test upgrade scenarios

**Files**:
- `cmd/foundry/commands/service/upgrade_template.go`
- `internal/service/upgrade.go`

---

### 8. CLI Command: `foundry tool create`

**Working State**: Can scaffold a CLI tool (no server component)

**Usage**:
```bash
foundry tool create mytool --lang go
```

**Key Tasks**:
- Similar to `service create` but only CLI + library
- No Helm chart
- No Dockerfile (optional)
- Focused on tool development

**Files**:
- `cmd/foundry/commands/tool/create.go`

---

### 9. Helm Chart Conventions

**Foundry Helm Chart Standards**:

All generated Helm charts include:
- Deployment with resource limits
- Service (if HTTP server)
- Ingress (if external access needed)
- ConfigMap for non-secret config
- ServiceMonitor for Prometheus
- Secret references resolved from OpenBAO
- Health check and readiness probe configuration
- Rolling update strategy
- Pod disruption budget (for production)

**Files**:
- Template charts in each template repository

---

### 10. CI/CD Pipeline Templates

**GitHub Actions Workflow** (example):
```yaml
name: CI/CD

on:
  push:
    branches: [main]
  pull_request:

jobs:
  test:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v3
      - name: Run tests
        run: make test

  build:
    needs: test
    runs-on: ubuntu-latest
    steps:
      - name: Build and push to Zot
        run: |
          docker build -t zot.example.com/myapp:${{ github.sha }} .
          docker push zot.example.com/myapp:${{ github.sha }}

  deploy:
    needs: build
    runs-on: ubuntu-latest
    steps:
      - name: Deploy via Helm
        run: |
          helm upgrade --install myapp ./helm \
            --set image.tag=${{ github.sha }}
```

**Key Tasks**:
- Create workflow templates for:
  - GitHub Actions
  - GitLab CI
  - Jenkins (optional)
- Include test, build, push, deploy stages
- Integration with Zot registry
- Deployment to K8s cluster
- Rollback on failure

**Files**:
- Templates include `.github/workflows/*.yml`

---

### 11. Database Migration Support

**Working States**:
- [ ] Can generate migration scaffolding
- [ ] Can run migrations
- [ ] Migrations integrated with deployment

**Key Tasks**:
- Generate migration tools (e.g., golang-migrate, Alembic, Flyway)
- `foundry service migrate create <name>`
- `foundry service migrate up`
- `foundry service migrate down`
- Integration with Helm chart (init containers)

**Files**:
- `cmd/foundry/commands/service/migrate.go`
- Migration tooling in templates

---

### 12. Template Documentation

**Documents to Create**:
- [ ] `docs/service-creation.md` - Service creation guide
- [ ] `docs/templates.md` - Template structure and customization
- [ ] `docs/deployment.md` - Deploying generated services
- [ ] Language-specific guides for each template

---

### 13. Integration Tests

**Test Scenarios**:
- [ ] Create Go service and deploy
- [ ] Create Python service and deploy
- [ ] Create CLI tool
- [ ] Upgrade template without losing changes
- [ ] Generated service passes all tests
- [ ] Generated Helm chart deploys successfully

**Files**:
- `test/integration/phase6_service_test.go`

---

## Phase 6 Completion Criteria

- [ ] All template repositories created and tested
- [ ] `foundry service create` works for all languages
- [ ] `foundry service upgrade-template` preserves user changes
- [ ] `foundry tool create` works
- [ ] Generated services include all best practices:
  - Health checks
  - Metrics
  - Logging
  - Secret management
  - CI/CD
- [ ] Generated Helm charts follow Foundry conventions
- [ ] Documentation complete
- [ ] Integration tests pass

## Manual Verification

```bash
# Create a new Go service
foundry service create myapp --lang go

cd myapp

# Run tests
make test

# Build
make build

# Deploy to cluster
helm upgrade --install myapp ./helm --namespace myapp --create-namespace

# Verify deployment
kubectl get pods -n myapp
kubectl logs -n myapp deployment/myapp

# Create Python tool
foundry tool create mytool --lang python
cd mytool
make test
```

---

## Design Philosophy

**Opinionated but Flexible**:
- Templates encode best practices
- Users can customize as needed
- Managed vs user files clearly separated

**Upgrade-Friendly**:
- Template upgrades don't break user code
- Conflicts are detected and reported
- Users opt-in to upgrades

**Production-Ready from Day One**:
- Health checks, metrics, logging built-in
- Secrets managed properly
- CI/CD ready
- Deployment ready

**Language-Idiomatic**:
- Each template follows language best practices
- Standard project layouts
- Standard tooling (go mod, cargo, poetry, npm)

---

**Estimated Working States**: ~40 testable states (across all languages)
**Estimated LOC**: ~5000-10000 lines (mostly template code)
**Timeline**: Not time-bound - only implement if there's demand
**Maintenance**: Requires ongoing template maintenance as languages and best practices evolve

## Before Starting Phase 6

**Ask these questions**:
1. Are Phases 1-5 complete and stable?
2. Are there users requesting this feature?
3. Is there capacity to maintain multiple language templates?
4. Would this effort be better spent on other features?

If all answers are "yes", proceed. Otherwise, defer Phase 6.
