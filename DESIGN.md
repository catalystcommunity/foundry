# Foundry Design Document

## Project Overview

### Vision

Foundry is a single-binary CLI tool that serves two primary purposes:

1. **Stack Setup & Management**: Deploy and manage the complete Catalyst Community tech stack on self-hosted infrastructure
2. **Workflow Tooling**: Provide streamlined workflows for creating, deploying, and operating services within that stack

The goal is to give developers the simplest way to manage their infrastructure without maintenance burden, allowing them to focus on code and version control rather than infrastructure complexity.

### Core Philosophy

- **Sane Defaults, Advanced Configuration**: Prioritize simplicity through intelligent defaults; power users can configure deeply
- **Modular & Composable**: Deep module-style interactions that build context as you narrow down operations
- **Interactive by Default, Automatable by Trust**: Humans in the loop by default; automation available for any operation the user trusts
- **Stateless by Design**: No local state files - query actual infrastructure state on demand
- **Location Agnostic**: Run from anywhere with network access (laptop, bastion host, control plane VM)
- **Container Everything**: All services run in containers (except TrueNAS and K3s itself)
- **Protocol Over Hardware**: Services reference each other via URLs and protocols, not infrastructure details
- **Scales Infinitely**: From 1 developer on a single server to 1000+ developers across thousands of nodes; distributed by design

### Target Users

The core user archetype: **Developers who want extremely professionally robust side projects without operational fuss, that can translate to production-ready workloads with ease.**

This includes:
- Solo developers building side projects that need production-grade infrastructure
- Small teams (2-10 developers) managing their own infrastructure
- Growing organizations (10-100+ developers) that want self-hosted control
- Anyone scaling from laptop → single server → multi-node cluster → distributed infrastructure
- Teams that want to avoid vendor lock-in and operational complexity

## Core Principles

### 1. No Infrastructure State Files

Unlike Terraform, Foundry maintains no local state about what's deployed where. Instead:
- Configuration files describe desired state (versions, settings)
- Actual state is queried from infrastructure (SSH commands, API calls, version endpoints)
- Reconciliation happens by comparing config to reality

### 2. Secrets Are Never in Config

Secrets management follows strict rules:
- Configs contain **references** to secrets, never values
- Resolution order: Environment variables → `~/.foundryvars` → OpenBAO
- OpenBAO is the source of truth, but can be overridden for flexibility
- Environment variables allow alternative secret mechanisms in any environment
- Initial bootstrap: Interactive prompts, then stored in OpenBAO

### 3. Idempotency as Default

Commands should be safe to run multiple times:
- Installing already-installed components is a no-op
- Upgrades check current version before proceeding
- Destructive operations require confirmation (unless forced)
- Exception: Where idempotency is impossible, document and warn

### 4. Namespacing by Context

All Foundry-managed resources use consistent namespacing:
- Core services: `foundry-*` prefix (OpenBAO paths, K8s namespaces, storage groups)
- User services: No coupling between service and namespace - deploy same service to multiple namespaces (e.g., `myapp-toy`, `myapp-stable`)
- Namespaces provide isolation for multi-instance deployments
- Clear separation for management, RBAC, and organization

### 5. Progressive Disclosure

Start simple, reveal complexity only when needed:
- `foundry stack install` - one command to set up everything
- `foundry component install openbao --version 2.1.0` - granular control when desired
- Interactive prompts guide through complex setups
- Dry-run mode available for all operational commands

## Architecture

### Technology Stack

**Language**: Go
- Single binary compilation
- Excellent SSH and systems programming libraries
- Fast, statically typed, cross-platform
- Strong Kubernetes ecosystem

**Core Libraries**:
- **urfave/cli**: Command structure and execution (simpler than Cobra, more complete than docopt)
- **gopkg.in/yaml.v3**: YAML parsing and configuration management
- **survey** or **bubbletea**: Interactive prompts and TUI elements
- **golang.org/x/crypto/ssh**: SSH operations and key management
- **client-go**: Kubernetes API interaction
- **Helm SDK**: Helm chart deployment

### Binary Distribution

- Single statically-linked binary
- Cross-compile for Linux, macOS, Windows
- Target infrastructure: Linux only (Debian/Ubuntu initially)
- CLI runs anywhere, manages services on Linux servers

### Execution Model

Foundry is location-agnostic:
- Can run from developer laptop, bastion host, CI/CD pipeline, control plane VM
- Requires network access to managed infrastructure
- SSH-based remote execution for most operations
- Direct protocol access where available (K8s API, OpenBAO API, etc.)

## Configuration System

### Stack Configuration

**Location**: `~/.foundry/` directory (OS-appropriate user home)

**Structure**:
```yaml
# ~/.foundry/stack.yaml
version: "1.0"
cluster:
  name: production
  domain: example.com
  nodes:
    - hostname: node1.example.com
      role: control-plane
    - hostname: node2.example.com
      role: worker

components:
  openbao:
    version: "2.1.0"  # Pin version, or omit for latest
    hosts:
      - openbao.example.com  # Runs on VM, not k8s

  k3s:
    version: "v1.28.5+k3s1"
    ha: true

  zot:
    version: "2.0.0"
    storage:
      backend: truenas
      path: ${secret:foundry/zot:storage_path}

  grafana:
    # version omitted - use latest
    admin_password: ${secret:foundry/grafana:admin_password}

observability:
  prometheus:
    retention: 30d
  loki:
    retention: 90d

storage:
  truenas:
    api_url: https://truenas.example.com
    api_key: ${secret:foundry/truenas:api_key}
```

**Multi-Config Support**:
- Multiple config files allowed: `~/.foundry/prod.yaml`, `~/.foundry/staging.yaml`
- Must be explicitly selected via `--config` flag or `FOUNDRY_CONFIG` env var
- No implicit default if multiple configs exist (prevents accidents)
- Flag overrides env var

### Per-Service Configuration

User-created services have their own configuration:
- Template-generated config in repository
- References stack resources by URL/DNS
- Version-controlled with the service
- Upgradeable via template updates

### Secret Reference Syntax

**Format**: `${secret:path/to/secret:key}`

**Examples**:
```yaml
database_password: ${secret:database/prod:password}
api_token: ${secret:external/github:api_token}
tls_cert: ${secret:certs/wildcard:cert}
```

**Context & Namespacing**:
Secret paths are automatically scoped to their service context. When a service references `${secret:database/prod:password}`, Foundry resolves it within that service's namespace (e.g., `myservice-stable/database/prod:password` in OpenBAO). Users don't need to include the namespace prefix in their service configs.

**Resolution Order**:
1. Check environment variable matching the full namespaced path (e.g., `FOUNDRY_SECRET_myservice_stable_database_prod_password`)
2. Check `~/.foundryvars` for override
3. Query OpenBAO at the namespaced path
4. Error if not found (no silent failures)

### Local Development Overrides

**File**: `~/.foundryvars`

**Format**:
```bash
# Map namespaced OpenBAO paths to local values
# Format: namespace/path/to/secret:key=value
myservice-stable/database/prod:password=my_local_dev_password
myservice-toy/database/prod:password=toy_password
foundry/truenas:api_key=local_truenas_key
external/github:api_token=ghp_localdevtoken123
```

**Behavior**:
- Allows local development without OpenBAO access
- Not recommended for production, but available for those accepting the security risks
- Should be in `.gitignore` (never committed)

## Secrets Management

### OpenBAO Integration

**Bootstrap Problem**: OpenBAO must be installed before K8s, but K8s components need secrets.

**Solution**:
1. Install OpenBAO on dedicated VM(s) first
2. Interactive initialization (generate root token)
3. Store root token securely (user's password manager initially)
4. Foundry stores its own auth token in OS keyring
5. Subsequent operations use stored token

**Secret Operations**:
- `foundry secret set <path>`: Interactive prompt for value, store in OpenBAO
- `foundry secret get <path>`: Retrieve and display (with confirmation)
- `foundry secret list [path]`: List available secret paths
- Auto-create namespaced paths for new services

### SSH Key Management

**Workflow**:
1. Initial connection: Password-based SSH (interactive prompt)
2. Generate unique SSH key pair for this specific host
3. Install public key on target host
4. Store private key in OpenBAO at `foundry/ssh-keys/<hostname>` (unless config override specifies not to use OpenBAO)
5. Subsequent connections use stored key

**Security**:
- Every host gets a unique SSH key (whether managing 1 host or 1000)
- Keys never stored locally by default - always in OpenBAO
- Local storage only if configuration explicitly overrides OpenBAO usage

### RBAC Integration

Users and service accounts are managed in OpenBAO:
- `foundry rbac user create <username>` → Creates OpenBAO entity
- Permissions tied to K8s namespaces via OIDC integration
- Requires the executing user to have cluster-admin permissions; will check and warn if insufficient
- Read-only, admin, or custom roles per namespace

## Command Structure

### Design Pattern

**Noun-Verb Structure**: Build context as you narrow down operations.

```bash
foundry <noun> <verb> [arguments] [flags]
```

**Examples**:
- `foundry stack install` - Stack is the noun, install is the verb
- `foundry cluster node add` - Cluster is the context, node is the sub-noun, add is the verb
- `foundry secret get path/to/secret` - Secret is the noun, get is the verb

### Command Reference

#### Stack Management

Orchestrates the entire stack installation and lifecycle.

```bash
foundry stack install [--config PATH] [--dry-run]
  # Install entire stack from config
  # Handles dependency ordering automatically

foundry stack upgrade [--dry-run]
  # Upgrade all components to versions in config
  # Prompts before each component unless --yes

foundry stack status
  # Query and display status of all components

foundry stack validate
  # Validate config file without making changes
```

#### Component Management

Individual services: OpenBAO, Zot, Grafana, K3s, etc.

```bash
foundry component install <name> [--version X] [--dry-run]
  # Install a single component

foundry component upgrade <name> [--dry-run]
  # Upgrade component to version in config
  # Checks current version, prompts for confirmation

foundry component status <name>
  # Query component health and version

foundry component list
  # List all available components
```

#### Cluster Operations

Kubernetes cluster management (K3s-based).

```bash
foundry cluster init [--single-node] [--config PATH]
  # Initialize new K3s cluster
  # Single-node or HA based on config

foundry cluster node add <hostname> [--interactive]
  # Add worker or control-plane node to cluster
  # Interactive prompts for SSH, role, etc.

foundry cluster node remove <hostname>
  # Drain and remove node from cluster

foundry cluster node list
  # List all cluster nodes with status

foundry cluster status
  # Overall cluster health and version info
```

#### Host Operations

Management of non-K8s VMs (for OpenBAO, bastion hosts, etc.).

```bash
foundry host add <hostname> [--interactive]
  # Register a new host
  # Sets up SSH, installs base packages

foundry host configure <hostname>
  # (Re)configure host: users, sudoers, SSH keys, packages

foundry host list
  # List all registered hosts

foundry host ssh <hostname>
  # Quick SSH helper using stored credentials
```

#### Service/Tool Creation

Generate new user services or CLI tools from templates.

```bash
foundry service create <name> --lang <go|python|rust|js>
  # Scaffold new service from Copier template
  # Auto-wires observability, secrets, Helm chart

foundry service upgrade-template <name>
  # Update service scaffolding from latest template
  # Minimizes merge conflicts via managed/user file separation

foundry tool create <name> --lang <go|python|rust|js>
  # Scaffold new CLI tool (no Helm chart)
```

#### RBAC Management

User and service account management tied to OpenBAO.

```bash
foundry rbac user create <username> [--namespace NS] [--role ROLE]
  # Create user in OpenBAO, grant K8s permissions
  # Requires cluster-admin permissions

foundry rbac user grant <username> --namespace NS --permissions PERMS
  # Grant additional permissions to existing user

foundry rbac serviceaccount create <name> --namespace NS
  # Create service account for application use

foundry rbac list [--namespace NS]
  # List users and service accounts
```

#### Secrets Management

OpenBAO secret operations.

```bash
foundry secret get <path>
  # Retrieve secret value (prompts for confirmation)

foundry secret set <path> [--interactive]
  # Store secret in OpenBAO
  # Interactive prompt for value (hidden input)

foundry secret list [path]
  # List available secrets at path
```

#### Storage Management

Storage backend configuration (TrueNAS, future cloud providers).

```bash
foundry storage configure [--interactive]
  # Configure storage backend
  # Interactive wizard for TrueNAS API, credentials, etc.

foundry storage list
  # List configured storage backends and volumes

foundry storage test
  # Verify connectivity and permissions
```

#### Config Management

Stack configuration file operations.

```bash
foundry config init [--interactive]
  # Create new stack config file
  # Interactive wizard for common settings

foundry config validate
  # Validate config against schema

foundry config show
  # Display current effective config (with secrets redacted)

foundry config list
  # Show available config files in ~/.foundry/
```

## Bootstrap & Installation Flow

### New Stack Setup (Greenfield)

**Prerequisites**:
- Fresh Debian/Ubuntu server(s) with OpenSSH (can be a single VM or multiple)
- Non-root admin user created on each
- Network connectivity from wherever Foundry is run
- DNS configuration (TBD - requirements to be determined)

**Workflow**:

```bash
# 1. Initialize config
foundry config init --interactive
  # Prompts for:
  # - Cluster name and domain
  # - Host address(es) - single server or multiple
  # - Which components to install
  # Creates ~/.foundry/<name>.yaml

# 2. Install stack
foundry stack install --config ~/.foundry/<name>.yaml
  # Execution order:
  # a. Verify SSH access to all hosts (prompt for passwords)
  # b. Generate and install SSH keys
  # c. Install OpenBAO on designated host(s)
  # d. Initialize OpenBAO (interactive root token generation)
  # e. Store Foundry's OpenBAO auth token
  # f. Install K3s (control plane first, then workers if multi-node)
  # g. Install Zot (OCI registry)
  # h. Configure K3s to use Zot as registry and pull-through cache
  # i. Install remaining components (Grafana, Loki, etc.)
  # j. Configure storage backends
  # k. Verify all components healthy

# 3. Create first user
foundry rbac user create admin --role cluster-admin
  # Prompts for password
  # Stores in OpenBAO
  # Creates K8s RBAC bindings
```

### Component Installation Order

Foundry automatically handles dependencies:

1. **OpenBAO** (on VM, not K8s)
   - Required first for all other secrets
   - Can be HA setup on multiple VMs

2. **K3s Cluster**
   - Control plane node(s) with optional VIP for HA
   - Worker nodes (if multi-node)
   - Requires OpenBAO for bootstrap secrets

3. **Zot** (OCI registry)
   - Deployed to K8s
   - K3s configured to use Zot as registry
   - Pull-through cache configuration
   - Required before other containerized components

4. **Storage**
   - Storage backends (TrueNAS integration)
   - CSI drivers
   - MinIO (if TrueNAS doesn't provide S3-compatible storage)

5. **Networking**
   - Contour (Ingress controller)
   - Cert-manager (TLS automation)
   - External-DNS (automatic DNS management)

6. **Observability**
   - Prometheus
   - Loki
   - Grafana

7. **Backup & Recovery**
   - Velero (using MinIO or TrueNAS S3 backend)

8. **CI/CD** (optional)
   - ArgoCD

### Adding a New Service

Once the stack is running:

```bash
foundry service create myapp --lang go
  # Generates:
  # - Go project structure
  # - Dockerfile
  # - Helm chart with Foundry conventions
  # - Grafana dashboard template
  # - Prometheus metrics endpoint
  # - OpenBAO secret paths
  # - GitHub Actions / CI pipeline template

cd myapp
# ... develop your service ...

# Deploy to cluster
helm upgrade --install myapp ./helm \
  --namespace myapp \
  --create-namespace
```

## Remote Execution & SSH

### Connection Management

**SSH Library**: `golang.org/x/crypto/ssh`

**Features**:
- Connection pooling (reuse connections within a command)
- Timeout handling
- Known hosts verification
- Agent forwarding support (optional)

### Multi-Host Orchestration

For commands that affect multiple hosts:
- Parallel execution where possible (independent operations)
- Serial execution where required (dependencies)
- Progress bars for long-running operations
- Aggregate error reporting

**Example**: `foundry cluster node add` on 3 nodes
1. Verify SSH access (parallel)
2. Install prerequisites (parallel)
3. Join to cluster (serial, one at a time to avoid split-brain)

### Error Handling

- SSH failures: Retry with exponential backoff
- Command failures: Show stdout/stderr, exit code
- Partial failures in multi-host: Continue or abort (user choice)
- Rollback: Best-effort revert for multi-step operations

## Service Creation & Templates

### Copier Integration

**Template Repository**: One template per language (e.g., `catalystcommunity/foundry-template-go`)

**Features**:
- Version-tagged templates
- Upgradeable scaffolding
- User-editable vs managed file distinction
- Optional directory inclusion (service, CLI, library components)

**Template Structure**:
- Every service includes: service code, CLI tool, and library
- CLI tools and libraries can exist independently (without service component)
- Single Copier template with conditional directory generation

### File Segregation

To enable template upgrades without merge conflicts:

**Managed Files** (can be auto-updated):
- `.foundry/` directory (scaffolding, CI, Helm base)
- `Dockerfile`
- Metrics endpoint boilerplate
- Health check endpoints
- Base test utilities (e.g., DataUtils for test data generation)

**User Files** (never overwritten):
- Application logic (`cmd/`, `internal/`, `pkg/`)
- Custom config files
- User-extended test code

**Hybrid Files** (merge markers):
- `main.go` or equivalent (initial template, then user-owned)
- Helm `values.yaml` (base template, user adds custom values)

### Template Contents

Every service template includes:

**Code**:
- HTTP server with health/ready/metrics endpoints (if service)
- CLI tool scaffolding
- Library structure
- OpenBAO client initialization
- Config loading (from files + secret resolution)
- Structured logging setup
- Error handling patterns

**Infrastructure** (for services):
- Dockerfile (multi-stage, optimized)
- Helm chart (Deployment, Service, Ingress, ConfigMap)
- Grafana dashboard JSON (basic metrics)
- Prometheus ServiceMonitor

**CI/CD**:
- Workflow for test, build, push to Zot, deploy via Helm
- Pre-commit hooks

**Documentation**:
- README template
- API documentation setup (optional)

### Target Languages

Initial template support planned for:
- Go
- Python
- Rust
- JavaScript/TypeScript

**Note**: Service creation is future work (later implementation phases). Details may evolve.

## Stack Components

### OpenBAO

**Purpose**: Secrets management and identity provider

**Deployment**: Dedicated VM(s), not in K8s (bootstrap requirement)

**Installation**:
- Download binary, install as systemd service
- Initialize and unseal
- Configure OIDC provider for K8s auth
- Set up namespaced secret paths

**Configuration**:
```yaml
components:
  openbao:
    version: "2.1.0"
    hosts:
      - openbao1.example.com
      - openbao2.example.com  # HA setup
    storage: postgresql  # or consul, raft
```

### K3s (Kubernetes)

**Purpose**: Container orchestration

**Why K3s**: Lightweight, simple, single-binary, perfect for self-hosted

**Installation**:
- Control plane: `curl -sfL https://get.k3s.io | sh -` (with config)
- Workers: Join with token from control plane
- Store kubeconfig in OpenBAO

**Configuration**:
```yaml
components:
  k3s:
    version: "v1.28.5+k3s1"
    ha: true  # Multi control-plane with VIP
    vip: 192.168.1.100  # Virtual IP for HA
    disable:
      - traefik  # We use Contour
```

### Zot (OCI Registry)

**Purpose**: Container image storage

**Why Zot**: Lightweight, OCI-native, minimal resource usage

**Deployment**: Helm chart in K8s

**Configuration**:
```yaml
components:
  zot:
    version: "2.0.0"
    storage:
      backend: truenas
      size: 500Gi
    auth: openbao  # OIDC via OpenBAO
```

### Grafana / Loki / Prometheus

**Purpose**: Observability stack

**Deployment**: Helm charts (kube-prometheus-stack, loki-stack)

**Auto-Configuration**:
- Service discovery for all `foundry-*` namespaced services
- Automatic dashboard provisioning for templated services
- Log aggregation from all pods
- Alerting rules for common failures

**Configuration**:
```yaml
components:
  grafana:
    admin_password: ${secret:foundry/grafana:admin_password}
    plugins:
      - grafana-piechart-panel

  prometheus:
    retention: 30d
    storage:
      size: 100Gi

  loki:
    retention: 90d
    storage:
      size: 200Gi
```

### Contour (Ingress)

**Purpose**: Ingress controller (HTTP/HTTPS routing)

**Why Contour**: Envoy-based, simple, GitOps-friendly

**Deployment**: Helm chart

**Features**:
- Automatic TLS via cert-manager
- HTTPProxy CRD for advanced routing
- Integration with External-DNS

### External-DNS

**Purpose**: Automatic DNS record management

**Deployment**: Helm chart in K8s

**Features**:
- Automatically creates/updates DNS records for Ingress resources
- Supports multiple DNS providers
- Integrates with Contour for automatic service DNS

### Velero

**Purpose**: Kubernetes backup and restore

**Deployment**: Helm chart in K8s

**Features**:
- Cluster backup and disaster recovery
- PVC snapshots
- Scheduled backups
- S3-compatible object storage backend (MinIO or TrueNAS)

### MinIO (Conditional)

**Purpose**: S3-compatible object storage

**Deployment**: Helm chart in K8s (only if TrueNAS doesn't provide S3-compatible storage)

**Usage**:
- Velero backup storage
- General object storage for services
- PVC-backed (potentially by TrueNAS)

**Configuration**:
```yaml
components:
  minio:
    deploy: auto  # Deploy if TrueNAS S3 unavailable
    storage:
      size: 1Ti
      backend: truenas  # PVC backed by TrueNAS if available
```

### ArgoCD (Optional)

**Purpose**: GitOps continuous delivery

**Deployment**: Helm chart

**Integration**:
- Auto-configure for Zot registry
- RBAC via OpenBAO
- Application templates for new services

### TrueNAS (External)

**Purpose**: Persistent storage backend

**Integration**:
- Not installed by Foundry (assumed pre-existing)
- `foundry storage configure` sets up API access
- Creates datasets/zvols for PVC provisioning
- CSI driver installation in K8s

## Testing Strategy

Following project best practices:

### Testing Philosophy

- **Mock only what we can't run locally**: Third-party vendor APIs (e.g., VoIP providers, cloud-only services)
- **Run everything else in containers**: OpenBAO, K8s (via Kind), databases, etc.
- **Prefer integration over unit tests**: Exercising actual services provides better signal
- **Trade-off**: Reduced parallel test capability is acceptable for higher confidence

### Unit Tests

- Minimal unit tests for pure logic (config parsing, path manipulation, etc.)
- Mock only third-party APIs that cannot be run locally
- Test secret resolution logic
- Test template generation

**Framework**: Go's built-in `testing` package + `testify` for assertions

### Integration Tests

- Container-based test environments (Docker Compose or Skaffold)
- Spin up OpenBAO in container with base configuration
- Kind (Kubernetes in Docker) for K8s integration tests
- Test full command workflows against real components
- No mocking of components we can run locally

**Test Containers**: Use `testcontainers-go` library

### CI/CD

- Run all tests on every PR
- Require 100% pass rate before merge
- Coverage reporting (aim for >80%)
- Automated integration test environments

## Implementation Phases

**Note**: Phases are not time-bound. Development proceeds at its own pace based on need and priority.

### Phase 1: Foundation

**Goals**: Core CLI structure, config system, SSH management

**Deliverables**:
- [ ] CLI scaffolding with urfave/cli
- [ ] Config file parsing (YAML)
- [ ] Secret reference resolution (OpenBAO + ~/.foundryvars)
- [ ] SSH connection management and key storage
- [ ] `foundry config` commands
- [ ] `foundry host` commands (add, configure, list, ssh)
- [ ] Unit tests for all of the above

### Phase 2: Stack Installation

**Goals**: Install core components

**Deliverables**:
- [ ] `foundry component install openbao`
- [ ] OpenBAO initialization and auth
- [ ] `foundry storage configure` for TrueNAS (early setup for Zot)
- [ ] `foundry component install zot`
- [ ] Configure Zot with TrueNAS storage backend
- [ ] `foundry cluster init` (K3s setup with Zot registry integration)
- [ ] `foundry cluster node add/remove`
- [ ] `foundry component install` for Contour, cert-manager
- [ ] Component dependency resolution and ordering
- [ ] `foundry stack install` orchestration
- [ ] Integration tests with Kind/K3s

### Phase 3: Observability & Storage

**Goals**: Complete the core stack

**Deliverables**:
- [ ] Complete TrueNAS integration (CSI drivers, PVC provisioning)
- [ ] MinIO installation (if needed)
- [ ] `foundry component install` for Prometheus, Loki, Grafana
- [ ] External-DNS installation
- [ ] Velero installation and backup configuration
- [ ] Grafana dashboards for core components
- [ ] `foundry stack status` with health checks

### Phase 4: RBAC & Operational Commands

**Goals**: User management and day-2 operations

**Deliverables**:
- [ ] `foundry rbac` commands (user, serviceaccount, grant)
- [ ] OpenBAO OIDC provider setup
- [ ] K8s RBAC integration
- [ ] `foundry stack upgrade` with dry-run
- [ ] `foundry component upgrade`
- [ ] Backup and restore commands
- [ ] ArgoCD installation (optional component)

### Phase 5: Polish & Documentation

**Goals**: Production-ready for broader use

**Deliverables**:
- [ ] Comprehensive documentation (user guide, operator guide)
- [ ] Error message improvements and help text
- [ ] Interactive wizards for complex operations
- [ ] Shell completion (bash, zsh, fish)
- [ ] Binary releases for Linux, macOS, Windows
- [ ] Migration guide from manual setups

### Phase 6: Service Creation (Future)

**Goals**: Service/tool scaffolding and templates

**Note**: This phase is optional and serves a specific subset of users. The core stack is fully functional without these templates.

**Deliverables**:
- [ ] Copier template integration
- [ ] `foundry service create` for Go, Python, Rust, JS
- [ ] `foundry tool create`
- [ ] Template upgrade mechanism
- [ ] Helm chart generation with Foundry conventions
- [ ] Grafana dashboard templates
- [ ] CI/CD pipeline templates
- [ ] Documentation for service development workflow

## Future Enhancements

- Cloud provider support (AWS, GCP, Azure storage backends)
- Additional K8s distributions (K0s, Talos)
- Multi-cluster management (federated secrets, service mesh)
- Backup and disaster recovery automation
- Cost tracking and resource optimization
- Plugin system for community extensions
- Web UI for status dashboards
- Terraform provider for Foundry resources
- Ansible integration for hybrid workflows

## Design Decisions & Rationale

### Why Not Terraform?

Terraform manages infrastructure. Foundry manages things deployed on infrastructure.

The tools are complementary by design:
- Terraform provisions VMs, networks, storage
- Foundry deploys and manages services on those VMs

Terraform isn't appropriate for what we're doing. We're not managing infrastructure state; we're managing service deployment and configuration.

### Why Not Ansible?

Ansible is closer to our needs, but:
- YAML playbooks become unwieldy for complex logic
- Inventory management is separate from execution
- Not a single binary (requires Python environment)
- Module ecosystem is vast but inconsistent

Foundry uses SSH like Ansible, but with a more focused CLI UX.

### Why K3s Over Vanilla K8s?

- Single binary, easier to manage
- Lower resource footprint (perfect for self-hosted)
- Batteries-included (local storage, simple setup)
- Production-ready
- Upgrades are simpler
- Targeting local infrastructure use case for now
- Easily adaptable to use existing K8s deployments with future adjustments

### Why OpenBAO Over HashiCorp Vault?

- Open source (Vault is now BSL-licensed)
- Community governance (LF project)
- API-compatible with Vault (easy migration)
- Self-hosted friendly (no licensing concerns)

### Why Zot Over Docker Registry / Harbor?

- Minimal resource usage
- OCI-native (not just Docker images)
- Simple to operate
- Harbor is too heavy for our tastes

## Success Metrics

How do we know Foundry is successful?

**For Users**:
- Time to first deployment: <30 minutes from bare VMs to running stack
- Learning curve: Junior dev can create and deploy a new service in <1 hour
- Operational burden: <2 hours/month to maintain a production stack

**For the Project**:
- Adoption: Multiple users with different use cases doing production deployments

If people are using it, we've won.

---

**Version**: 0.1.0
**Last Updated**: 2025-10-19
**Status**: Draft - Ready for Implementation
