# Phase 2: Stack Installation - Implementation Tasks

**Goal**: Install and configure core stack components (OpenBAO, Zot, K3s, basic networking)

**Milestone**: User can deploy a working Kubernetes cluster with registry and secrets management from a single command

## Prerequisites

Phase 1 must be complete:
- ✓ CLI framework with urfave/cli v3
- ✓ Configuration system
- ✓ Secret reference parsing (with instance context)
- ✓ SSH connection management
- ✓ Host management

## Key Architectural Decisions

### Installation Order
1. **OpenBAO** (container on host) - Secrets management first
2. **Zot** (container on host) - Registry before K3s so K3s can pull from it
3. **K3s** - Kubernetes cluster configured to use Zot from the start
4. **Networking** - Contour, cert-manager (via Helm after K3s is up)

### VIP Strategy
- **Always use VIP** - Even single-node clusters use kube-vip
- No special cases for single vs multi-node
- Consistent experience regardless of cluster size

### Node Roles
- **User-specified**: If user sets `role: control-plane`, node is control-plane only
- **Default behavior**:
  - 1 node: First node is control-plane + worker
  - 2 nodes: First node is control-plane + worker, second is worker only
  - 3+ nodes: First 3 nodes are control-plane + worker, rest are workers
- No "split" roles unless user explicitly configures

## Task Breakdown

---

### 1. Component Installation Framework

**Working State**: Generic component installer interface defined

#### Tasks:
- [ ] Create `internal/component/types.go`:
  ```go
  type Component interface {
      Name() string
      Install(ctx context.Context, cfg ComponentConfig) error
      Upgrade(ctx context.Context, cfg ComponentConfig) error
      Status(ctx context.Context) (*ComponentStatus, error)
      Uninstall(ctx context.Context) error
  }

  type ComponentStatus struct {
      Installed bool
      Version   string
      Healthy   bool
      Message   string
  }

  type ComponentConfig map[string]interface{}
  ```
- [ ] Create `internal/component/registry.go` - registry of all available components
- [ ] Create `internal/component/dependency.go` - dependency resolution
- [ ] Write unit tests for dependency ordering
- [ ] Test circular dependency detection

**Test Criteria**:
- [ ] Components can be registered
- [ ] Dependencies are resolved in correct order
- [ ] Circular dependencies are detected
- [ ] Tests pass

**Files Created**:
- `internal/component/types.go`
- `internal/component/registry.go`
- `internal/component/dependency.go`
- `internal/component/*_test.go`

---

### 2. Container Runtime Helpers

**Working State**: Can interact with container runtime on remote host

#### Tasks:
- [ ] Create `internal/container/runtime.go`:
  ```go
  type Runtime interface {
      Pull(image string) error
      Run(config RunConfig) (containerID string, error)
      Stop(containerID string) error
      Remove(containerID string) error
      Inspect(containerID string) (*ContainerInfo, error)
  }

  type DockerRuntime struct {
      conn *ssh.Connection
  }

  type PodmanRuntime struct {
      conn *ssh.Connection
  }
  ```
- [ ] Implement Docker commands via SSH
- [ ] Implement Podman commands via SSH
- [ ] Auto-detect which runtime is available
- [ ] Write integration tests

**Test Criteria**:
- [ ] Can pull images
- [ ] Can run containers
- [ ] Can stop/remove containers
- [ ] Runtime detection works
- [ ] Integration tests pass

**Files Created**:
- `internal/container/runtime.go`
- `internal/container/types.go`
- `internal/container/runtime_test.go`

---

### 3. Systemd Service Management

**Working State**: Can create and manage systemd services on remote host

#### Tasks:
- [ ] Create `internal/systemd/service.go`:
  ```go
  func CreateService(conn *ssh.Connection, name string, unit UnitFile) error
  func EnableService(conn *ssh.Connection, name string) error
  func StartService(conn *ssh.Connection, name string) error
  func StopService(conn *ssh.Connection, name string) error
  func GetServiceStatus(conn *ssh.Connection, name string) (*ServiceStatus, error)
  ```
- [ ] Template systemd unit files
- [ ] Handle service enable/start
- [ ] Query service status
- [ ] Write integration tests

**Test Criteria**:
- [ ] Can create systemd unit files
- [ ] Can enable and start services
- [ ] Can query service status
- [ ] Integration tests pass

**Files Created**:
- `internal/systemd/service.go`
- `internal/systemd/types.go`
- `internal/systemd/service_test.go`

---

### 4. OpenBAO Container Installation

**Working State**: Can install OpenBAO as containerized systemd service

#### Tasks:
- [ ] Create `internal/component/openbao/install.go`:
  ```go
  func Install(conn *ssh.Connection, cfg ComponentConfig) error
  func createSystemdService(conn *ssh.Connection, version string) error
  ```
- [ ] Pull official OpenBAO container image (quay.io/openbao/openbao)
- [ ] Create systemd service that runs container:
  ```
  [Service]
  ExecStart=/usr/bin/docker run --rm --name openbao \
    -p 8200:8200 \
    -v /var/lib/openbao:/vault/data \
    quay.io/openbao/openbao:${VERSION} server -config=/vault/config/config.hcl
  ```
- [ ] Create config directory with config.hcl
- [ ] Set up data volume
- [ ] Enable and start service
- [ ] Write integration tests

**Test Criteria**:
- [ ] OpenBAO container runs as systemd service
- [ ] Service survives reboots (enabled)
- [ ] API is accessible
- [ ] Integration tests pass

**Files Created**:
- `internal/component/openbao/install.go`
- `internal/component/openbao/config.go`
- `internal/component/openbao/templates/config.hcl`
- `internal/component/openbao/install_test.go`

---

### 5. OpenBAO Initialization

**Working State**: Can initialize and unseal OpenBAO

#### Tasks:
- [ ] Create `internal/component/openbao/init.go`:
  ```go
  func Initialize(addr string) (*InitResult, error)
  func Unseal(addr string, keys []string) error
  func VerifySealed(addr string) (bool, error)
  ```
- [ ] Use OpenBAO HTTP API
- [ ] Implement initialization flow
- [ ] Capture root token and unseal keys
- [ ] Display clear instructions for storing root token securely
- [ ] Implement unseal process
- [ ] Write integration tests

**Test Criteria**:
- [ ] Can initialize OpenBAO
- [ ] Returns root token and unseal keys
- [ ] Can unseal successfully
- [ ] Integration tests pass

**Files Created**:
- `internal/component/openbao/init.go`
- `internal/component/openbao/client.go` (HTTP API client)
- `internal/component/openbao/init_test.go`

---

### 6. OpenBAO Secret Resolution Implementation

**Working State**: Can resolve secrets from OpenBAO with instance scoping

#### Tasks:
- [ ] Implement `internal/secrets/openbao.go` (replace stub from Phase 1):
  ```go
  type OpenBAOResolver struct {
      client *openbaoClient
  }

  func NewOpenBAOResolver(addr, token string) (*OpenBAOResolver, error)
  func (o *OpenBAOResolver) Resolve(ctx *ResolutionContext, ref SecretRef) (string, error)
  ```
- [ ] Use instance from context to build path: `<instance>/<path>`
- [ ] Query OpenBAO KV v2 API
- [ ] Handle missing secrets gracefully
- [ ] Cache client connection
- [ ] Write integration tests with OpenBAO container

**Test Criteria**:
- [ ] Can resolve secrets from OpenBAO
- [ ] Instance scoping works correctly
- [ ] Returns error for missing secrets
- [ ] Integration tests pass

**Files Created**:
- Updated `internal/secrets/openbao.go`
- Updated `internal/secrets/openbao_test.go`
- `test/integration/openbao_secrets_test.go`

---

### 7. OpenBAO Auth Token Management

**Working State**: Can store and retrieve Foundry's OpenBAO auth token

#### Tasks:
- [ ] Create `internal/secrets/auth.go`:
  ```go
  func StoreAuthToken(token string) error
  func LoadAuthToken() (string, error)
  func ClearAuthToken() error
  ```
- [ ] Use OS keyring for storage (e.g., `zalando/go-keyring`)
- [ ] Fallback to file with restrictive permissions if keyring unavailable
- [ ] Handle token rotation
- [ ] Write unit tests

**Test Criteria**:
- [ ] Can store token
- [ ] Can retrieve token
- [ ] Can clear token
- [ ] Fallback works if keyring unavailable
- [ ] Tests pass

**Files Created**:
- `internal/secrets/auth.go`
- `internal/secrets/auth_test.go`

---

### 8. OpenBAO SSH Key Storage Implementation

**Working State**: Can store/retrieve SSH keys in OpenBAO

#### Tasks:
- [ ] Implement `internal/ssh/storage.go` (replace stub from Phase 1):
  ```go
  type OpenBAOKeyStorage struct {
      client *openbao.Client
  }

  func (o *OpenBAOKeyStorage) Store(host string, key *KeyPair) error
  func (o *OpenBAOKeyStorage) Load(host string) (*KeyPair, error)
  ```
- [ ] Store at path: `foundry-core/ssh-keys/<hostname>`
- [ ] Store both private and public keys
- [ ] Handle concurrent access
- [ ] Write integration tests

**Test Criteria**:
- [ ] Can store SSH keys
- [ ] Can retrieve SSH keys
- [ ] Keys are stored in correct path
- [ ] Integration tests pass

**Files Created**:
- Updated `internal/ssh/storage.go`
- Updated `internal/ssh/storage_test.go`

---

### 9. Zot Container Installation

**Working State**: Can install Zot registry as containerized systemd service

#### Tasks:
- [ ] Create `internal/component/zot/install.go`:
  ```go
  func Install(conn *ssh.Connection, cfg ComponentConfig) error
  func createConfig(cfg ComponentConfig) (string, error)
  func createSystemdService(conn *ssh.Connection, version string) error
  ```
- [ ] Pull official Zot container image (ghcr.io/project-zot/zot)
- [ ] Create Zot config file with:
  - Storage configuration (use TrueNAS mount if available)
  - Pull-through cache for Docker Hub
  - Authentication (OpenBAO OIDC later, basic auth for now)
- [ ] Create systemd service running Zot container
- [ ] Set up data directories with correct permissions
- [ ] Enable and start service
- [ ] Write integration tests

**Test Criteria**:
- [ ] Zot container runs as systemd service
- [ ] Registry API is accessible
- [ ] Can push/pull images
- [ ] Pull-through cache works
- [ ] Integration tests pass

**Files Created**:
- `internal/component/zot/install.go`
- `internal/component/zot/config.go`
- `internal/component/zot/templates/config.json`
- `internal/component/zot/install_test.go`

**Note**: Zot is installed BEFORE K3s so K3s can use it from the start

---

### 10. K3s Token Generation

**Working State**: Can generate secure tokens for K3s cluster

#### Tasks:
- [ ] Create `internal/component/k3s/tokens.go`:
  ```go
  func GenerateClusterToken() (string, error)
  func GenerateAgentToken() (string, error)
  func StoreTokens(clusterToken, agentToken string) error
  func LoadTokens() (clusterToken, agentToken string, error)
  ```
- [ ] Generate cryptographically secure random tokens
- [ ] Store in OpenBAO: `foundry-core/k3s/cluster-token` and `foundry-core/k3s/agent-token`
- [ ] Write unit tests

**Test Criteria**:
- [ ] Tokens are cryptographically secure
- [ ] Tokens are stored in OpenBAO
- [ ] Can retrieve tokens
- [ ] Tests pass

**Files Created**:
- `internal/component/k3s/tokens.go`
- `internal/component/k3s/tokens_test.go`

---

### 11. VIP Configuration

**Working State**: Can determine VIP configuration for cluster

#### Tasks:
- [ ] Create `internal/component/k3s/vip.go`:
  ```go
  func DetermineVIP(cfg ClusterConfig) (string, error)
  func DetectNetworkInterface(conn *ssh.Connection) (string, error)
  func GenerateKubeVIPManifest(vip, iface string) (string, error)
  ```
- [ ] Get VIP from config (required field)
- [ ] Auto-detect primary network interface on host
- [ ] Generate kube-vip DaemonSet manifest
- [ ] Write unit tests

**Test Criteria**:
- [ ] VIP is validated
- [ ] Network interface detection works
- [ ] Manifest generation works
- [ ] Tests pass

**Files Created**:
- `internal/component/k3s/vip.go`
- `internal/component/k3s/vip_test.go`

---

### 12. K3s Control Plane Installation

**Working State**: Can install K3s control plane with VIP

#### Tasks:
- [ ] Create `internal/component/k3s/install.go`:
  ```go
  func InstallControlPlane(conn *ssh.Connection, cfg K3sConfig) error
  func setupKubeVIP(conn *ssh.Connection, vip, iface string) error
  func getKubeconfig(conn *ssh.Connection) ([]byte, error)
  ```
- [ ] Use curl | bash pattern: `curl -sfL https://get.k3s.io | sh -s - server`
- [ ] Pass flags based on your script:
  - `--cluster-init` (for HA)
  - `--token ${CLUSTER_TOKEN}`
  - `--agent-token ${AGENT_TOKEN}`
  - `--tls-san ${VIP}`
  - `--tls-san <cluster-name>.<domain>`
  - `--disable=traefik`
  - `--disable=servicelb`
- [ ] Configure K3s to use Zot registry via `/etc/rancher/k3s/registries.yaml`
- [ ] Set up kube-vip:
  - Apply RBAC manifest
  - Apply cloud controller manifest
  - Create configmap with VIP CIDR
  - Pull kube-vip image using `ctr`
  - Generate and deploy DaemonSet manifest
- [ ] Retrieve kubeconfig from `/etc/rancher/k3s/k3s.yaml`
- [ ] Store kubeconfig in OpenBAO: `foundry-core/k3s/kubeconfig`
- [ ] Write integration tests

**Test Criteria**:
- [ ] K3s installs successfully
- [ ] VIP is accessible
- [ ] kube-vip is running
- [ ] Kubeconfig works
- [ ] Kubeconfig is in OpenBAO
- [ ] K3s uses Zot registry
- [ ] Integration tests pass

**Files Created**:
- `internal/component/k3s/install.go`
- `internal/component/k3s/config.go`
- `internal/component/k3s/registry.go`
- `internal/component/k3s/install_test.go`

---

### 13. K3s Node Role Determination

**Working State**: Can determine correct role for each node

#### Tasks:
- [ ] Create `internal/component/k3s/roles.go`:
  ```go
  func DetermineNodeRoles(nodes []NodeConfig) []NodeRole
  func IsControlPlane(index int, totalNodes int, explicitRole string) bool
  func IsWorker(index int, totalNodes int, explicitRole string) bool
  ```
- [ ] Implement logic:
  - If user specifies `role: control-plane`, use it
  - Default for 1 node: control-plane + worker
  - Default for 2 nodes: node 0 is control-plane + worker, node 1 is worker
  - Default for 3+ nodes: nodes 0-2 are control-plane + worker, rest are workers
- [ ] Write unit tests for all scenarios

**Test Criteria**:
- [ ] 1-node cluster: first node is control-plane + worker
- [ ] 2-node cluster: first is both, second is worker
- [ ] 3-node cluster: first 3 are both
- [ ] 5-node cluster: first 3 are both, last 2 are workers
- [ ] Explicit roles are respected
- [ ] Tests pass

**Files Created**:
- `internal/component/k3s/roles.go`
- `internal/component/k3s/roles_test.go`

---

### 14. K3s Additional Control Plane Nodes

**Working State**: Can add additional control plane nodes to cluster

#### Tasks:
- [ ] Create `internal/component/k3s/controlplane.go`:
  ```go
  func JoinControlPlane(conn *ssh.Connection, existingCP string, tokens Tokens, cfg K3sConfig) error
  ```
- [ ] Use curl | bash with `server` mode
- [ ] Pass `--server https://<existing-cp>:6443`
- [ ] Pass same tokens and TLS SANs
- [ ] Configure registry
- [ ] Verify node joins successfully
- [ ] Write integration tests

**Test Criteria**:
- [ ] Additional control plane joins cluster
- [ ] Node is marked as control-plane
- [ ] Node is also a worker (combined role)
- [ ] Integration tests pass

**Files Created**:
- `internal/component/k3s/controlplane.go`
- `internal/component/k3s/controlplane_test.go`

---

### 15. K3s Worker Node Addition

**Working State**: Can add worker nodes to cluster

#### Tasks:
- [ ] Create `internal/component/k3s/worker.go`:
  ```go
  func JoinWorker(conn *ssh.Connection, serverURL string, agentToken string, cfg K3sConfig) error
  ```
- [ ] Use curl | bash with `agent` mode
- [ ] Pass `--server https://<vip>:6443`
- [ ] Pass agent token
- [ ] Configure registry
- [ ] Verify node joins successfully
- [ ] Write integration tests

**Test Criteria**:
- [ ] Worker joins cluster successfully
- [ ] Node appears in cluster
- [ ] Node is marked ready
- [ ] Integration tests pass

**Files Created**:
- `internal/component/k3s/worker.go`
- `internal/component/k3s/worker_test.go`

---

### 16. K8s Client

**Working State**: Can interact with K3s cluster via Kubernetes API

#### Tasks:
- [ ] Create `internal/k8s/client.go`:
  ```go
  func NewClientFromKubeconfig(kubeconfig []byte) (*Client, error)
  func NewClientFromOpenBAO(resolver *secrets.OpenBAOResolver) (*Client, error)
  func (c *Client) GetNodes() ([]Node, error)
  func (c *Client) GetPods(namespace string) ([]Pod, error)
  func (c *Client) ApplyManifest(manifest string) error
  ```
- [ ] Use `client-go` library
- [ ] Wrap common operations
- [ ] Handle authentication
- [ ] Write unit tests with fake clientset

**Test Criteria**:
- [ ] Can create client from kubeconfig
- [ ] Can create client from OpenBAO-stored kubeconfig
- [ ] Can query cluster resources
- [ ] Error handling works
- [ ] Tests pass

**Files Created**:
- `internal/k8s/client.go`
- `internal/k8s/types.go`
- `internal/k8s/client_test.go`

---

### 17. Helm Integration

**Working State**: Can deploy Helm charts to cluster

#### Tasks:
- [ ] Create `internal/helm/client.go`:
  ```go
  func NewClient(kubeconfig []byte, namespace string) (*Client, error)
  func (c *Client) AddRepo(name, url string) error
  func (c *Client) Install(release string, chart string, values map[string]interface{}) error
  func (c *Client) Upgrade(release string, chart string, values map[string]interface{}) error
  func (c *Client) Uninstall(release string) error
  func (c *Client) List() ([]Release, error)
  ```
- [ ] Use Helm SDK (not CLI wrapper)
- [ ] Support custom values
- [ ] Handle release already exists
- [ ] Write unit tests

**Test Criteria**:
- [ ] Can add repos
- [ ] Can install charts
- [ ] Can upgrade charts
- [ ] Can list releases
- [ ] Tests pass

**Files Created**:
- `internal/helm/client.go`
- `internal/helm/types.go`
- `internal/helm/client_test.go`

---

### 18. Contour Ingress Controller

**Working State**: Can deploy Contour ingress controller via Helm

#### Tasks:
- [ ] Create `internal/component/contour/install.go`:
  ```go
  func Install(helmClient *helm.Client, k8sClient *k8s.Client, cfg ComponentConfig) error
  ```
- [ ] Add Contour Helm repo
- [ ] Deploy Contour chart
- [ ] Configure for bare metal (use kube-vip cloud provider)
- [ ] Set default IngressClass
- [ ] Verify deployment
- [ ] Write integration tests

**Test Criteria**:
- [ ] Contour deploys successfully
- [ ] Envoy pods are running
- [ ] Can create HTTPProxy resources
- [ ] LoadBalancer service gets VIP from kube-vip
- [ ] Integration tests pass

**Files Created**:
- `internal/component/contour/install.go`
- `internal/component/contour/install_test.go`

---

### 19. cert-manager Deployment

**Working State**: Can deploy cert-manager via Helm

#### Tasks:
- [ ] Create `internal/component/certmanager/install.go`:
  ```go
  func Install(helmClient *helm.Client, k8sClient *k8s.Client, cfg ComponentConfig) error
  func CreateClusterIssuer(k8sClient *k8s.Client, issuerType string, config IssuerConfig) error
  ```
- [ ] Add cert-manager Helm repo
- [ ] Deploy cert-manager chart (includes CRDs)
- [ ] Wait for cert-manager to be ready
- [ ] Create default ClusterIssuer (self-signed for now)
- [ ] Write integration tests

**Test Criteria**:
- [ ] cert-manager deploys successfully
- [ ] CRDs are installed
- [ ] Can create ClusterIssuer
- [ ] Can issue certificates
- [ ] Integration tests pass

**Files Created**:
- `internal/component/certmanager/install.go`
- `internal/component/certmanager/issuer.go`
- `internal/component/certmanager/install_test.go`

---

### 20. Storage Configuration - TrueNAS API Client

**Working State**: Can interact with TrueNAS API

#### Tasks:
- [ ] Create `internal/storage/truenas/client.go`:
  ```go
  func NewClient(apiURL string, apiKey string) (*Client, error)
  func (c *Client) CreateDataset(name string, config DatasetConfig) error
  func (c *Client) DeleteDataset(name string) error
  func (c *Client) ListDatasets() ([]Dataset, error)
  func (c *Client) CreateNFSShare(path string, config NFSConfig) error
  ```
- [ ] Implement TrueNAS API operations
- [ ] Handle authentication
- [ ] Error handling
- [ ] Write unit tests (mocked API)

**Test Criteria**:
- [ ] Can create datasets
- [ ] Can list datasets
- [ ] Can create NFS shares
- [ ] Tests pass with mocked API

**Files Created**:
- `internal/storage/truenas/client.go`
- `internal/storage/truenas/types.go`
- `internal/storage/truenas/client_test.go`

**Note**: Full CSI integration comes in Phase 3

---

### 21. CLI Command: `foundry component install <name>`

**Working State**: Can install individual components

#### Tasks:
- [ ] Create `cmd/foundry/commands/component/install.go`
- [ ] Look up component by name in registry
- [ ] Resolve dependencies
- [ ] Load config from stack.yaml
- [ ] Resolve secrets with appropriate instance context (e.g., `foundry-core`)
- [ ] Execute installation
- [ ] Show progress
- [ ] Handle errors gracefully
- [ ] Write integration tests

**Test Criteria**:
- [ ] Can install OpenBAO
- [ ] Can install Zot
- [ ] Can install K3s
- [ ] Dependencies are enforced (Zot before K3s)
- [ ] Integration tests pass

**Files Created**:
- `cmd/foundry/commands/component/install.go`
- `cmd/foundry/commands/component/install_test.go`

---

### 22. CLI Command: `foundry component status <name>`

**Working State**: Can check status of installed components

#### Tasks:
- [ ] Create `cmd/foundry/commands/component/status.go`
- [ ] Query component status
- [ ] Display version, health, message
- [ ] Handle component not installed
- [ ] Write tests

**Test Criteria**:
- [ ] Shows correct status for installed components
- [ ] Shows correct status for not-installed components
- [ ] Output is clear and formatted
- [ ] Tests pass

**Files Created**:
- `cmd/foundry/commands/component/status.go`
- `cmd/foundry/commands/component/status_test.go`

---

### 23. CLI Command: `foundry component list`

**Working State**: Can list all available components

#### Tasks:
- [ ] Create `cmd/foundry/commands/component/list.go`
- [ ] Query component registry
- [ ] Show which are installed
- [ ] Show versions (installed vs available)
- [ ] Write tests

**Test Criteria**:
- [ ] Lists all components
- [ ] Shows installation status
- [ ] Shows versions
- [ ] Tests pass

**Files Created**:
- `cmd/foundry/commands/component/list.go`
- `cmd/foundry/commands/component/list_test.go`

---

### 24. CLI Command: `foundry cluster init`

**Working State**: Can initialize K3s cluster from config

#### Tasks:
- [ ] Create `cmd/foundry/commands/cluster/init.go`
- [ ] Load cluster config
- [ ] Determine node roles using role determination logic
- [ ] Generate cluster and agent tokens
- [ ] Install control plane nodes (one or more based on config)
- [ ] Install worker nodes (if any)
- [ ] Set up kube-vip
- [ ] Configure K3s to use Zot
- [ ] Store kubeconfig in OpenBAO
- [ ] Verify cluster is healthy
- [ ] Write integration tests

**Test Criteria**:
- [ ] Single-node cluster initializes with VIP
- [ ] Multi-node cluster initializes correctly
- [ ] Node roles are correct
- [ ] Kubeconfig is in OpenBAO
- [ ] Integration tests pass

**Files Created**:
- `cmd/foundry/commands/cluster/init.go`
- `cmd/foundry/commands/cluster/init_test.go`

---

### 25. CLI Command: `foundry cluster node add <hostname>`

**Working State**: Can add nodes to existing cluster

#### Tasks:
- [ ] Create `cmd/foundry/commands/cluster/node_add.go`
- [ ] Look up host in registry
- [ ] Determine if node should be control-plane, worker, or both
- [ ] Load tokens from OpenBAO
- [ ] Join node to cluster (control-plane or worker)
- [ ] Verify node appears in cluster
- [ ] Write integration tests

**Test Criteria**:
- [ ] Can add worker nodes
- [ ] Can add control-plane nodes
- [ ] Node roles are correct
- [ ] Integration tests pass

**Files Created**:
- `cmd/foundry/commands/cluster/node_add.go`
- `cmd/foundry/commands/cluster/node_add_test.go`

---

### 26. CLI Command: `foundry cluster node remove <hostname>`

**Working State**: Can remove nodes from cluster

#### Tasks:
- [ ] Create `cmd/foundry/commands/cluster/node_remove.go`
- [ ] Get kubeconfig from OpenBAO
- [ ] Drain node (kubectl drain)
- [ ] Delete node from cluster (kubectl delete node)
- [ ] Uninstall K3s from host
- [ ] Write integration tests

**Test Criteria**:
- [ ] Node is drained successfully
- [ ] Node is removed from cluster
- [ ] K3s is uninstalled from host
- [ ] Integration tests pass

**Files Created**:
- `cmd/foundry/commands/cluster/node_remove.go`
- `cmd/foundry/commands/cluster/node_remove_test.go`

---

### 27. CLI Command: `foundry cluster node list`

**Working State**: Can list all cluster nodes

#### Tasks:
- [ ] Create `cmd/foundry/commands/cluster/node_list.go`
- [ ] Get kubeconfig from OpenBAO
- [ ] Query Kubernetes API
- [ ] Display table with node info (name, roles, status, version)
- [ ] Write tests

**Test Criteria**:
- [ ] Lists nodes correctly
- [ ] Shows node roles (control-plane, worker, or both)
- [ ] Shows node status
- [ ] Tests pass

**Files Created**:
- `cmd/foundry/commands/cluster/node_list.go`
- `cmd/foundry/commands/cluster/node_list_test.go`

---

### 28. CLI Command: `foundry cluster status`

**Working State**: Can show overall cluster status

#### Tasks:
- [ ] Create `cmd/foundry/commands/cluster/status.go`
- [ ] Get kubeconfig from OpenBAO
- [ ] Query cluster health
- [ ] Show control plane node count
- [ ] Show worker node count
- [ ] Show VIP status
- [ ] Show version info
- [ ] Write tests

**Test Criteria**:
- [ ] Shows cluster status correctly
- [ ] Detects unhealthy cluster
- [ ] Shows VIP information
- [ ] Tests pass

**Files Created**:
- `cmd/foundry/commands/cluster/status.go`
- `cmd/foundry/commands/cluster/status_test.go`

---

### 29. CLI Command: `foundry stack install`

**Working State**: Can install entire stack from config in one command

#### Tasks:
- [ ] Create `cmd/foundry/commands/stack/install.go`
- [ ] Load config
- [ ] Determine component installation order:
  1. OpenBAO (on designated host)
  2. Initialize OpenBAO
  3. Zot registry (on designated host, before K3s)
  4. TrueNAS storage configuration (if configured)
  5. K3s cluster (with Zot registry already configured)
  6. Contour ingress
  7. cert-manager
- [ ] Resolve secrets with `foundry-core` instance context
- [ ] Show overall progress
- [ ] Handle partial failures (ask user to continue or abort)
- [ ] Verify all components healthy
- [ ] Write integration tests

**Test Criteria**:
- [ ] Full stack installs successfully
- [ ] Installation order is correct (Zot before K3s)
- [ ] K3s is configured to use Zot from the start
- [ ] All components are healthy
- [ ] Integration tests pass

**Files Created**:
- `cmd/foundry/commands/stack/install.go`
- `cmd/foundry/commands/stack/install_test.go`

---

### 30. CLI Command: `foundry stack status`

**Working State**: Can show status of all stack components

#### Tasks:
- [ ] Create `cmd/foundry/commands/stack/status.go`
- [ ] Query status of all components
- [ ] Display table with component status
- [ ] Show overall health indicator
- [ ] Write tests

**Test Criteria**:
- [ ] Shows all component statuses
- [ ] Overall health is accurate
- [ ] Table formatting works
- [ ] Tests pass

**Files Created**:
- `cmd/foundry/commands/stack/status.go`
- `cmd/foundry/commands/stack/status_test.go`

---

### 31. CLI Command: `foundry stack validate`

**Working State**: Can validate stack config before installation

#### Tasks:
- [ ] Create `cmd/foundry/commands/stack/validate.go`
- [ ] Load config
- [ ] Validate structure
- [ ] Check secret references (syntax only)
- [ ] Verify component dependencies
- [ ] Verify VIP is configured
- [ ] Check hosts are reachable
- [ ] Write tests

**Test Criteria**:
- [ ] Valid config passes all checks
- [ ] Missing VIP is detected
- [ ] Invalid config shows clear errors
- [ ] Unreachable hosts are detected
- [ ] Tests pass

**Files Created**:
- `cmd/foundry/commands/stack/validate.go`
- `cmd/foundry/commands/stack/validate_test.go`

---

### 32. CLI Command: `foundry storage configure`

**Working State**: Can configure TrueNAS storage backend

#### Tasks:
- [ ] Create `cmd/foundry/commands/storage/configure.go`
- [ ] Interactive prompts for TrueNAS API URL and key
- [ ] Test connection to TrueNAS
- [ ] Create base dataset for Foundry if needed
- [ ] Update config file with TrueNAS settings
- [ ] Store API key in OpenBAO: `foundry-core/truenas:api_key`
- [ ] Write integration tests

**Test Criteria**:
- [ ] Can configure TrueNAS
- [ ] Connection test works
- [ ] Credentials stored correctly in OpenBAO
- [ ] Config file updated
- [ ] Integration tests pass

**Files Created**:
- `cmd/foundry/commands/storage/configure.go`
- `cmd/foundry/commands/storage/configure_test.go`

---

### 33. CLI Command: `foundry storage list`

**Working State**: Can list configured storage backends

#### Tasks:
- [ ] Create `cmd/foundry/commands/storage/list.go`
- [ ] Query configured storage backends from config
- [ ] Show type, status, available space
- [ ] Write tests

**Test Criteria**:
- [ ] Lists storage backends
- [ ] Shows status correctly
- [ ] Tests pass

**Files Created**:
- `cmd/foundry/commands/storage/list.go`
- `cmd/foundry/commands/storage/list_test.go`

---

### 34. CLI Command: `foundry storage test`

**Working State**: Can test storage backend connectivity

#### Tasks:
- [ ] Create `cmd/foundry/commands/storage/test.go`
- [ ] Test TrueNAS API connection
- [ ] Test dataset creation/deletion (create temp dataset)
- [ ] Show results
- [ ] Write tests

**Test Criteria**:
- [ ] Can test TrueNAS connection
- [ ] Reports success/failure clearly
- [ ] Cleans up test resources
- [ ] Tests pass

**Files Created**:
- `cmd/foundry/commands/storage/test.go`
- `cmd/foundry/commands/storage/test_test.go`

---

### 35. Integration Tests - Phase 2 Full Workflow

**Working State**: End-to-end test of Phase 2 functionality

#### Tasks:
- [ ] Create `test/integration/phase2_test.go`
- [ ] Test full workflow:
  1. Create stack config with VIP
  2. Validate config
  3. Install OpenBAO (container)
  4. Initialize and unseal OpenBAO
  5. Configure storage (if test TrueNAS available)
  6. Install Zot (container)
  7. Test Zot registry
  8. Install K3s cluster (with VIP)
  9. Verify K3s uses Zot
  10. Install Contour
  11. Install cert-manager
  12. Verify full stack status
  13. Deploy test workload
- [ ] Use testcontainers where appropriate
- [ ] Use Kind for K8s testing if full VM testing not available
- [ ] Clean up resources after test
- [ ] Add to CI pipeline

**Test Criteria**:
- [ ] Full workflow completes successfully
- [ ] All components are healthy
- [ ] VIP works correctly
- [ ] Can deploy and access workload
- [ ] Tests run in CI

**Files Created**:
- `test/integration/phase2_test.go`

---

### 36. Documentation - Phase 2

**Working State**: Documentation for Phase 2 features

#### Tasks:
- [ ] Create `docs/installation.md`:
  - Prerequisites (host requirements, network)
  - VIP configuration
  - Stack installation walkthrough
  - Component installation individually
  - Troubleshooting
- [ ] Create `docs/components.md`:
  - OpenBAO (container deployment, initialization)
  - Zot registry (container deployment, pull-through cache)
  - K3s cluster (installation, node roles, VIP)
  - kube-vip setup
  - Contour ingress
  - cert-manager
- [ ] Create `docs/storage.md`:
  - TrueNAS integration
  - Storage configuration
  - Dataset management
- [ ] Update `docs/secrets.md`:
  - OpenBAO secret storage
  - Instance contexts for core components
  - Kubeconfig storage
- [ ] Create `docs/architecture.md`:
  - Deployment order and rationale (Zot before K3s)
  - VIP strategy
  - Node roles strategy
- [ ] Update README.md with Phase 2 status

**Test Criteria**:
- [ ] Documentation is clear and accurate
- [ ] Examples work
- [ ] Troubleshooting section is helpful
- [ ] VIP and node roles are well explained

**Files Created**:
- `docs/installation.md`
- `docs/components.md`
- `docs/storage.md`
- `docs/architecture.md`
- Updated `docs/secrets.md`
- Updated `README.md`

---

## Phase 2 Completion Checklist

Before considering Phase 2 complete, verify:

- [ ] All tasks above are complete
- [ ] All unit tests pass
- [ ] All integration tests pass
- [ ] Test coverage is >80%
- [ ] OpenBAO installs as container and initializes successfully
- [ ] Zot installs as container and works as registry + pull-through cache
- [ ] K3s cluster can be created with VIP (single-node and multi-node)
- [ ] K3s uses Zot from initial installation
- [ ] Node roles are correctly determined
- [ ] Control plane nodes can be both control-plane AND worker
- [ ] kube-vip works correctly (even on single node)
- [ ] Kubeconfig is stored in OpenBAO
- [ ] Contour ingress controller works
- [ ] cert-manager can issue certificates
- [ ] Secret resolution from OpenBAO works with instance context
- [ ] `foundry stack install` deploys working stack in correct order
- [ ] Manual end-to-end test successful:
  ```bash
  foundry config init
  foundry stack validate
  foundry stack install
  foundry stack status
  foundry cluster node list
  foundry component list
  # Deploy test workload
  kubectl run nginx --image=nginx
  kubectl get pods
  ```

## Key Design Principles - Phase 2

### Why Zot Before K3s?
- K3s needs to pull images from somewhere
- By installing Zot first, K3s can use it from the start
- Pull-through cache reduces external dependencies
- Consistent with "container everything" philosophy

### Why VIP on Single Node?
- No special cases - same experience for 1 or 100 nodes
- Makes it easy to add nodes later
- Production-ready from the start
- Consistent kubeconfig (always points to VIP)

### Why Combined Control-Plane+Worker Roles?
- Efficient resource usage
- Simpler for small deployments
- Can still scale to pure control-plane nodes if user wants
- Based on user's existing scripts and experience

## Dependencies for Phase 3

Phase 2 provides:
- ✓ Component installation framework
- ✓ OpenBAO integration (container-based)
- ✓ Zot registry (container-based, before K3s)
- ✓ K3s cluster management (with VIP always)
- ✓ Helm deployment capability
- ✓ Basic networking (Contour, cert-manager)
- ✓ Storage configuration (TrueNAS API)
- ✓ Kubeconfig in OpenBAO

Phase 3 will add:
- Full storage integration (CSI drivers, PVC provisioning)
- MinIO deployment (if needed)
- Observability stack (Prometheus, Loki, Grafana)
- External-DNS
- Velero backups

---

**Estimated Working States**: 36 testable states
**Estimated LOC**: ~6000-8000 lines (including tests)
**Timeline**: Not time-bound - proceed at natural pace
