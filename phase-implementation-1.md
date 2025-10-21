# Phase 1: Foundation - Implementation Tasks

**Goal**: Establish core CLI structure, configuration system, and SSH management

**Milestone**: User can manage configuration files, connect to hosts via SSH, and resolve secrets from multiple sources with proper instance scoping

## Task Breakdown

Each task is designed to result in a working, testable state. Tests should be written alongside implementation.

---

### 1. Project Initialization

**Working State**: Go project compiles and runs, outputs version information

#### Tasks:
- [ ] Initialize Go module: `go mod init github.com/catalystcommunity/foundry`
- [ ] Create directory structure:
  ```
  cmd/foundry/          # Main entry point
  internal/
    config/             # Config file handling
    secrets/            # Secret resolution
    ssh/                # SSH connection management
    host/               # Host operations
  pkg/                  # Public APIs (if needed)
  test/
    fixtures/           # Test data
    integration/        # Integration tests
  ```
- [ ] Create `cmd/foundry/main.go` with basic CLI scaffold using urfave/cli v3
- [ ] Implement `--version` flag that outputs version info
- [ ] Set up `Makefile` with targets: `build`, `test`, `clean`, `install`
- [ ] Create `.gitignore` with Go-specific excludes (if not comprehensive already)
- [ ] Add `LICENSE` (already exists - verify correct)
- [ ] Initialize `go.mod` with core dependencies:
  - `github.com/urfave/cli/v3`
  - `gopkg.in/yaml.v3`
  - `github.com/stretchr/testify`
  - `golang.org/x/crypto/ssh`

**Test Criteria**:
- [ ] `go build ./cmd/foundry` compiles successfully
- [ ] `./foundry --version` outputs version
- [ ] `./foundry --help` shows command structure
- [ ] `make test` runs (even with zero tests)

**Files Created**:
- `cmd/foundry/main.go`
- `Makefile`
- `go.mod`
- Updated `.gitignore` (if needed)

---

### 2. Configuration Data Structures

**Working State**: Config structs defined, can parse valid YAML into structs

#### Tasks:
- [ ] Define `internal/config/types.go` with core structs:
  ```go
  type Config struct {
      Version string         `yaml:"version"`
      Cluster ClusterConfig  `yaml:"cluster"`
      Components ComponentMap `yaml:"components"`
      Observability ObsConfig `yaml:"observability,omitempty"`
      Storage StorageConfig   `yaml:"storage,omitempty"`
  }

  type ClusterConfig struct {
      Name   string      `yaml:"name"`
      Domain string      `yaml:"domain"`
      Nodes  []NodeConfig `yaml:"nodes"`
  }

  type NodeConfig struct {
      Hostname string `yaml:"hostname"`
      Role     string `yaml:"role"` // control-plane, worker
  }

  type ComponentMap map[string]ComponentConfig

  type ComponentConfig struct {
      Version string                 `yaml:"version,omitempty"`
      Hosts   []string               `yaml:"hosts,omitempty"`
      Config  map[string]interface{} `yaml:",inline"`
  }
  ```
- [ ] Add validation methods for each struct (e.g., `Validate() error`)
- [ ] Write unit tests for validation logic
  - Valid configs pass
  - Missing required fields fail
  - Invalid roles fail
  - Invalid versions fail (if format-specific)

**Test Criteria**:
- [ ] Can unmarshal valid YAML into Config struct
- [ ] Validation catches missing required fields
- [ ] Validation catches invalid enum values
- [ ] Tests pass with >80% coverage

**Files Created**:
- `internal/config/types.go`
- `internal/config/types_test.go`
- `test/fixtures/valid-config.yaml`
- `test/fixtures/invalid-config-*.yaml`

---

### 3. Configuration File Loading

**Working State**: Can load config from file path, parse YAML, validate structure

#### Tasks:
- [ ] Implement `internal/config/loader.go`:
  ```go
  func Load(path string) (*Config, error)
  func LoadFromReader(r io.Reader) (*Config, error)
  ```
- [ ] Handle file reading errors gracefully
- [ ] Parse YAML and unmarshal into Config
- [ ] Run validation after parsing
- [ ] Return detailed errors (file not found, parse error, validation error)
- [ ] Write unit tests:
  - Load valid config succeeds
  - Load missing file returns error
  - Load invalid YAML returns parse error
  - Load invalid config returns validation error

**Test Criteria**:
- [ ] Can load `test/fixtures/valid-config.yaml`
- [ ] Loading non-existent file returns clear error
- [ ] Loading invalid YAML returns parse error
- [ ] Loading invalid config returns validation error
- [ ] All tests pass

**Files Created**:
- `internal/config/loader.go`
- `internal/config/loader_test.go`

---

### 4. Configuration Path Resolution

**Working State**: CLI can find and load config from multiple possible locations

#### Tasks:
- [ ] Implement `internal/config/paths.go`:
  ```go
  func GetConfigDir() (string, error)  // Returns ~/.foundry/
  func FindConfig(name string) (string, error)  // Finds config file
  func ListConfigs() ([]string, error)  // Lists available configs
  ```
- [ ] Use `os.UserHomeDir()` for cross-platform home directory
- [ ] Check for `FOUNDRY_CONFIG` env var first
- [ ] Then check `--config` flag value
- [ ] Then look for default `~/.foundry/stack.yaml`
- [ ] If multiple configs and no explicit selection, return error
- [ ] Write unit tests with temporary directories

**Test Criteria**:
- [ ] `GetConfigDir()` returns correct path
- [ ] `FindConfig()` finds existing config
- [ ] `FindConfig()` returns error for non-existent config
- [ ] `ListConfigs()` returns all *.yaml files in config dir
- [ ] Tests use temp dirs, don't touch user's actual config

**Files Created**:
- `internal/config/paths.go`
- `internal/config/paths_test.go`

---

### 5. Secret Reference Parsing

**Working State**: Can identify and parse secret references in config, with optional instance context

#### Tasks:
- [ ] Implement `internal/secrets/parser.go`:
  ```go
  type SecretRef struct {
      Path     string  // path/to/secret
      Key      string  // key_name
      Instance string  // Optional: service instance (e.g., "myapp-prod", "myapp-stable")
      Raw      string  // original ${secret:path:key}
  }

  func ParseSecretRef(s string) (*SecretRef, error)
  func IsSecretRef(s string) bool
  ```
- [ ] Use regex to match `${secret:path/to/secret:key}` pattern
- [ ] Handle malformed references (missing parts, invalid syntax)
- [ ] Instance is NOT part of the secret reference syntax in config
  - Config contains: `${secret:database/prod:password}`
  - Instance context is provided at resolution time
- [ ] Write unit tests:
  - Valid reference parses correctly
  - Invalid references return errors
  - Non-secret strings return nil/false
  - Edge cases (empty path, empty key, special chars)

**Test Criteria**:
- [ ] `${secret:database/prod:password}` parses correctly
- [ ] `${secret:}` returns error
- [ ] `plaintext` returns false for IsSecretRef
- [ ] All tests pass

**Files Created**:
- `internal/secrets/parser.go`
- `internal/secrets/parser_test.go`

**Design Note**:
Secret references in config files don't include instance context. Instance is provided at resolution time based on what service/namespace is being deployed. This allows the same config to be used for multiple instances (e.g., `myapp-prod`, `myapp-stable`).

---

### 6. Secret Resolution Context

**Working State**: Context type for secret resolution with instance scoping

#### Tasks:
- [ ] Implement `internal/secrets/context.go`:
  ```go
  type ResolutionContext struct {
      Instance  string  // e.g., "myapp-prod", "foundry-core"
      Namespace string  // Optional: for namespace-scoped secrets
  }

  func NewResolutionContext(instance string) *ResolutionContext
  func (rc *ResolutionContext) NamespacedPath(ref SecretRef) string
  ```
- [ ] `NamespacedPath()` combines instance with secret path
  - Example: instance="myapp-prod", ref.Path="database/main"
  - Returns: "myapp-prod/database/main"
- [ ] For foundry core components, use instance="foundry-core" or similar
- [ ] Write unit tests for path construction

**Test Criteria**:
- [ ] Context correctly combines instance + path
- [ ] Handles edge cases (empty instance, etc.)
- [ ] Tests pass

**Files Created**:
- `internal/secrets/context.go`
- `internal/secrets/context_test.go`

---

### 7. Secret Resolution - Environment Variables

**Working State**: Can resolve secrets from environment variables with instance context

#### Tasks:
- [ ] Implement `internal/secrets/resolver.go`:
  ```go
  type Resolver interface {
      Resolve(ctx *ResolutionContext, ref SecretRef) (string, error)
  }

  type EnvResolver struct{}

  func (e *EnvResolver) Resolve(ctx *ResolutionContext, ref SecretRef) (string, error)
  ```
- [ ] Convert secret ref to env var name using instance context:
  - Pattern: `FOUNDRY_SECRET_<instance>_<path>_<key>`
  - Example: `FOUNDRY_SECRET_myapp_prod_database_main_password`
- [ ] Replace slashes, hyphens, and colons with underscores
- [ ] Check if env var exists
- [ ] Return value or error if not found
- [ ] Write unit tests with `os.Setenv`

**Test Criteria**:
- [ ] Can resolve secret from env var with instance context
- [ ] Returns error if env var not set
- [ ] Name conversion works correctly (including instance)
- [ ] Tests clean up env vars after running

**Files Created**:
- `internal/secrets/resolver.go`
- `internal/secrets/resolver_test.go`

---

### 8. Secret Resolution - ~/.foundryvars

**Working State**: Can resolve secrets from ~/.foundryvars file with instance scoping

#### Tasks:
- [ ] Implement `internal/secrets/foundryvars.go`:
  ```go
  type FoundryVarsResolver struct {
      vars map[string]string  // instance/path:key -> value
  }

  func NewFoundryVarsResolver(path string) (*FoundryVarsResolver, error)
  func (f *FoundryVarsResolver) Resolve(ctx *ResolutionContext, ref SecretRef) (string, error)
  ```
- [ ] Parse file format: `instance/path/to/secret:key=value`
  - Example: `myapp-prod/database/main:password=secretpass123`
  - Example: `foundry-core/openbao:token=root-token`
- [ ] Handle comments (lines starting with #)
- [ ] Handle empty lines
- [ ] Trim whitespace
- [ ] Store in map for lookups
- [ ] Use instance from ResolutionContext to construct lookup key
- [ ] Write unit tests with temp files

**Test Criteria**:
- [ ] Can parse valid .foundryvars file
- [ ] Ignores comments and empty lines
- [ ] Resolves secrets correctly with instance context
- [ ] Returns error for missing secrets
- [ ] Tests use temp files

**Files Created**:
- `internal/secrets/foundryvars.go`
- `internal/secrets/foundryvars_test.go`
- `test/fixtures/test-foundryvars`

---

### 9. Secret Resolution - OpenBAO (Stub)

**Working State**: OpenBAO resolver interface defined, returns placeholder

#### Tasks:
- [ ] Implement `internal/secrets/openbao.go`:
  ```go
  type OpenBAOResolver struct {
      client *openbaoClient  // Placeholder for now
  }

  func NewOpenBAOResolver(addr, token string) (*OpenBAOResolver, error)
  func (o *OpenBAOResolver) Resolve(ctx *ResolutionContext, ref SecretRef) (string, error)
  ```
- [ ] For now, return error: "OpenBAO integration not yet implemented"
- [ ] Define interface that we'll implement in Phase 2
- [ ] Document expected path structure: `<instance>/<path>:<key>`
- [ ] Write stub tests that expect the error

**Test Criteria**:
- [ ] Resolver can be instantiated
- [ ] Resolve() returns not-implemented error
- [ ] Tests pass

**Files Created**:
- `internal/secrets/openbao.go`
- `internal/secrets/openbao_test.go`

---

### 10. Secret Resolution - Chain Resolver

**Working State**: Can resolve secrets by trying multiple sources in order

#### Tasks:
- [ ] Implement `internal/secrets/chain.go`:
  ```go
  type ChainResolver struct {
      resolvers []Resolver
  }

  func NewChainResolver(resolvers ...Resolver) *ChainResolver
  func (c *ChainResolver) Resolve(ctx *ResolutionContext, ref SecretRef) (string, error)
  ```
- [ ] Try each resolver in order (env, foundryvars, openbao)
- [ ] Return first successful resolution
- [ ] If all fail, return aggregate error with details
- [ ] Write unit tests with mock resolvers

**Test Criteria**:
- [ ] Tries resolvers in order (env, foundryvars, openbao)
- [ ] Returns first successful result
- [ ] Returns error if all fail
- [ ] Error message lists all failures
- [ ] Tests use mock/stub resolvers

**Files Created**:
- `internal/secrets/chain.go`
- `internal/secrets/chain_test.go`

---

### 11. Configuration Secret Resolution

**Working State**: Can validate config without resolving secrets, can resolve when instance context provided

#### Tasks:
- [ ] Implement `internal/config/resolve.go`:
  ```go
  func ValidateSecretRefs(cfg *Config) error  // Just validate syntax
  func ResolveSecrets(cfg *Config, ctx *secrets.ResolutionContext, resolver secrets.Resolver) error
  ```
- [ ] `ValidateSecretRefs()`: Parse all secret refs, ensure valid syntax
  - Used during `foundry config validate`
  - Doesn't actually resolve secrets
  - Returns error if any secret reference is malformed
- [ ] `ResolveSecrets()`: Actually resolve secrets
  - Requires ResolutionContext (instance must be provided)
  - Walk config structure recursively
  - Find all string values that are secret references
  - Resolve each using chain resolver
  - Replace reference with actual value in config
  - Handle resolution errors gracefully
- [ ] Write unit tests with mock resolver

**Test Criteria**:
- [ ] Can validate secret refs without resolution
- [ ] Can resolve secrets when instance context provided
- [ ] Handles missing secrets with clear error
- [ ] Leaves non-secret values unchanged
- [ ] Tests use fixtures with secret refs

**Files Created**:
- `internal/config/resolve.go`
- `internal/config/resolve_test.go`

**Design Note**:
This allows `foundry config validate` to check syntax without needing actual secrets. Actual resolution happens when deploying/installing components, where instance context is known.

---

### 12. SSH Connection Types

**Working State**: SSH connection data structures defined

#### Tasks:
- [ ] Implement `internal/ssh/types.go`:
  ```go
  type Connection struct {
      Host       string
      Port       int
      User       string
      AuthMethod ssh.AuthMethod
      client     *ssh.Client
  }

  type KeyPair struct {
      Private []byte
      Public  []byte
  }
  ```
- [ ] Define connection options struct
- [ ] Define auth method types (password, key)
- [ ] Write basic validation

**Test Criteria**:
- [ ] Structs compile
- [ ] Validation works
- [ ] Tests pass

**Files Created**:
- `internal/ssh/types.go`
- `internal/ssh/types_test.go`

---

### 13. SSH Key Generation

**Working State**: Can generate SSH key pairs

#### Tasks:
- [ ] Implement `internal/ssh/keygen.go`:
  ```go
  func GenerateKeyPair(bits int) (*KeyPair, error)
  func (kp *KeyPair) PublicKeyString() string  // OpenSSH format
  func (kp *KeyPair) PrivateKeyPEM() ([]byte, error)
  ```
- [ ] Use `crypto/ed25519` (modern, secure, fast)
- [ ] Generate key pair
- [ ] Encode private key as PEM
- [ ] Format public key as OpenSSH authorized_keys format
- [ ] Write unit tests

**Test Criteria**:
- [ ] Can generate valid key pair
- [ ] Public key is in correct format
- [ ] Private key is valid PEM
- [ ] Tests verify key validity

**Files Created**:
- `internal/ssh/keygen.go`
- `internal/ssh/keygen_test.go`

---

### 14. SSH Connection Establishment

**Working State**: Can connect to SSH server with password or key

#### Tasks:
- [ ] Implement `internal/ssh/connect.go`:
  ```go
  func Connect(host string, port int, user string, auth ssh.AuthMethod) (*Connection, error)
  func (c *Connection) Close() error
  func (c *Connection) IsConnected() bool
  ```
- [ ] Use `golang.org/x/crypto/ssh`
- [ ] Support password auth
- [ ] Support public key auth
- [ ] Set reasonable timeouts (30s connection, 60s overall)
- [ ] Handle connection errors
- [ ] Write integration tests with SSH container

**Test Criteria**:
- [ ] Can connect to SSH server (using testcontainers)
- [ ] Connection fails with bad credentials
- [ ] Connection closes cleanly
- [ ] Integration tests pass

**Files Created**:
- `internal/ssh/connect.go`
- `internal/ssh/connect_test.go`
- `test/integration/ssh_test.go`

**Dependencies**:
- Requires testcontainers with SSH server image

---

### 15. SSH Command Execution

**Working State**: Can execute commands over SSH and capture output

#### Tasks:
- [ ] Implement `internal/ssh/exec.go`:
  ```go
  type ExecResult struct {
      Stdout   string
      Stderr   string
      ExitCode int
  }

  func (c *Connection) Exec(command string) (*ExecResult, error)
  func (c *Connection) ExecWithTimeout(command string, timeout time.Duration) (*ExecResult, error)
  ```
- [ ] Create session from connection
- [ ] Capture stdout and stderr
- [ ] Get exit code
- [ ] Handle timeouts
- [ ] Write integration tests

**Test Criteria**:
- [ ] Can execute simple commands (`echo hello`)
- [ ] Captures stdout correctly
- [ ] Captures stderr correctly
- [ ] Returns correct exit code
- [ ] Timeout works
- [ ] Integration tests pass

**Files Created**:
- `internal/ssh/exec.go`
- `internal/ssh/exec_test.go`

---

### 16. SSH Key Storage (OpenBAO Stub)

**Working State**: Interface for storing SSH keys defined

#### Tasks:
- [ ] Implement `internal/ssh/storage.go`:
  ```go
  type KeyStorage interface {
      Store(host string, key *KeyPair) error
      Load(host string) (*KeyPair, error)
      Delete(host string) error
  }

  type OpenBAOKeyStorage struct {
      // Stub for now
  }
  ```
- [ ] Define interface
- [ ] Create stub implementation that returns errors
- [ ] Document expected storage path: `foundry-core/ssh-keys/<hostname>`
- [ ] Write stub tests

**Test Criteria**:
- [ ] Interface is defined
- [ ] Stub returns appropriate errors
- [ ] Tests pass

**Files Created**:
- `internal/ssh/storage.go`
- `internal/ssh/storage_test.go`

---

### 17. Host Management Types

**Working State**: Host data structures defined

#### Tasks:
- [ ] Implement `internal/host/types.go`:
  ```go
  type Host struct {
      Hostname  string
      Address   string
      Port      int
      User      string
      SSHKeySet bool
  }

  type HostRegistry interface {
      Add(host *Host) error
      Get(hostname string) (*Host, error)
      List() ([]*Host, error)
      Remove(hostname string) error
  }
  ```
- [ ] Define host struct
- [ ] Define registry interface
- [ ] Write validation

**Test Criteria**:
- [ ] Structs compile
- [ ] Validation works
- [ ] Tests pass

**Files Created**:
- `internal/host/types.go`
- `internal/host/types_test.go`

---

### 18. Host Registry (In-Memory Stub)

**Working State**: Can store and retrieve host info (in-memory only for now)

#### Tasks:
- [ ] Implement `internal/host/registry.go`:
  ```go
  type MemoryRegistry struct {
      hosts map[string]*Host
      mu    sync.RWMutex
  }

  func NewMemoryRegistry() *MemoryRegistry
  // Implement HostRegistry interface
  ```
- [ ] Implement all registry methods with in-memory storage
- [ ] Thread-safe with mutexes
- [ ] Write unit tests

**Test Criteria**:
- [ ] Can add hosts
- [ ] Can retrieve hosts
- [ ] Can list hosts
- [ ] Can remove hosts
- [ ] Thread-safe (test with goroutines)
- [ ] Tests pass

**Files Created**:
- `internal/host/registry.go`
- `internal/host/registry_test.go`

**Note**: Real registry (backed by config or OpenBAO) comes later

---

### 19. CLI Command: `foundry config init`

**Working State**: Can create a new config file interactively or with defaults

#### Tasks:
- [ ] Implement `cmd/foundry/commands/config/init.go`
- [ ] Register command with urfave/cli v3
- [ ] Implement interactive prompts (for now, just use basic stdin)
  - Cluster name
  - Domain
  - Single node or multi-node
- [ ] Generate config struct with defaults
- [ ] Write to `~/.foundry/<name>.yaml`
- [ ] Handle file already exists error
- [ ] Add `--force` flag to overwrite
- [ ] Write integration tests

**Test Criteria**:
- [ ] `foundry config init` creates config file
- [ ] Config file is valid YAML
- [ ] Can load created config
- [ ] Error if file exists without --force
- [ ] Tests use temp directories

**Files Created**:
- `cmd/foundry/commands/config/init.go`
- `cmd/foundry/commands/config/init_test.go`

---

### 20. CLI Command: `foundry config validate`

**Working State**: Can validate config file syntax and structure (including secret ref syntax)

#### Tasks:
- [ ] Implement `cmd/foundry/commands/config/validate.go`
- [ ] Load config from path
- [ ] Run structural validation
- [ ] Run secret reference validation (syntax only, no resolution)
- [ ] Output success or detailed error
- [ ] Exit with appropriate code
- [ ] Write tests

**Test Criteria**:
- [ ] Valid config passes
- [ ] Invalid config fails with clear error
- [ ] Malformed secret references are caught
- [ ] Missing file error is clear
- [ ] Tests pass

**Files Created**:
- `cmd/foundry/commands/config/validate.go`
- `cmd/foundry/commands/config/validate_test.go`

---

### 21. CLI Command: `foundry config show`

**Working State**: Can display current config with secrets redacted

#### Tasks:
- [ ] Implement `cmd/foundry/commands/config/show.go`
- [ ] Load config
- [ ] Redact any secret references (replace with `[SECRET]`)
- [ ] Output as formatted YAML
- [ ] Add `--show-secret-refs` flag to show secret ref syntax (not values)
- [ ] Write tests

**Test Criteria**:
- [ ] Shows config correctly
- [ ] Secrets are redacted by default
- [ ] `--show-secret-refs` shows `${secret:path:key}` syntax
- [ ] Tests pass

**Files Created**:
- `cmd/foundry/commands/config/show.go`
- `cmd/foundry/commands/config/show_test.go`

**Note**: Changed from `--with-secrets` since we can't resolve secrets without instance context. We can only show the reference syntax.

---

### 22. CLI Command: `foundry config list`

**Working State**: Can list available config files

#### Tasks:
- [ ] Implement `cmd/foundry/commands/config/list.go`
- [ ] Find all *.yaml files in `~/.foundry/`
- [ ] Display list with indicators for active config
- [ ] Show which config would be used by default
- [ ] Write tests

**Test Criteria**:
- [ ] Lists all configs
- [ ] Shows active config
- [ ] Works with empty directory
- [ ] Tests use temp directories

**Files Created**:
- `cmd/foundry/commands/config/list.go`
- `cmd/foundry/commands/config/list_test.go`

---

### 23. CLI Command: `foundry host add`

**Working State**: Can add a host via interactive prompts

#### Tasks:
- [ ] Implement `cmd/foundry/commands/host/add.go`
- [ ] Prompt for hostname, address, user
- [ ] Test SSH connection with password
- [ ] Generate SSH key pair
- [ ] Install public key on host
- [ ] Store private key (stub for now - just in-memory)
- [ ] Add host to registry (in-memory)
- [ ] Write integration tests (with SSH container)

**Test Criteria**:
- [ ] Can add host interactively
- [ ] SSH key is generated and installed
- [ ] Host appears in registry
- [ ] Integration tests pass

**Files Created**:
- `cmd/foundry/commands/host/add.go`
- `cmd/foundry/commands/host/add_test.go`

---

### 24. CLI Command: `foundry host list`

**Working State**: Can list all registered hosts

#### Tasks:
- [ ] Implement `cmd/foundry/commands/host/list.go`
- [ ] Query host registry
- [ ] Display table with hostname, address, user, key status
- [ ] Handle empty registry gracefully
- [ ] Write tests

**Test Criteria**:
- [ ] Lists hosts correctly
- [ ] Shows empty state message
- [ ] Table formatting works
- [ ] Tests pass

**Files Created**:
- `cmd/foundry/commands/host/list.go`
- `cmd/foundry/commands/host/list_test.go`

---

### 25. CLI Command: `foundry host configure`

**Working State**: Can run basic configuration on a host

#### Tasks:
- [ ] Implement `cmd/foundry/commands/host/configure.go`
- [ ] Connect to host
- [ ] Run basic setup commands:
  - Update packages (`apt-get update`)
  - Install common tools (curl, git, vim, etc.)
  - Configure time sync if needed
- [ ] Show progress
- [ ] Handle errors gracefully
- [ ] Write integration tests

**Test Criteria**:
- [ ] Can execute configuration steps
- [ ] Error handling works
- [ ] Progress is shown
- [ ] Integration tests pass

**Files Created**:
- `cmd/foundry/commands/host/configure.go`
- `cmd/foundry/commands/host/configure_test.go`

---

### 26. Integration Tests - Full Workflow

**Working State**: End-to-end test of Phase 1 functionality

#### Tasks:
- [ ] Create `test/integration/phase1_test.go`
- [ ] Test full workflow:
  1. Create config
  2. Validate config (including secret refs)
  3. Add host (with SSH container)
  4. List hosts
  5. Configure host
- [ ] Use testcontainers for all external services
- [ ] Clean up resources after test
- [ ] Add to CI pipeline

**Test Criteria**:
- [ ] Full workflow completes successfully
- [ ] All assertions pass
- [ ] No resource leaks
- [ ] Test runs in CI

**Files Created**:
- `test/integration/phase1_test.go`
- `.github/workflows/test.yml` (if using GitHub Actions)

---

### 27. Documentation - Phase 1

**Working State**: Basic documentation for Phase 1 features

#### Tasks:
- [ ] Create `docs/getting-started.md`:
  - Installation
  - First config
  - Adding hosts
- [ ] Create `docs/configuration.md`:
  - Config file format
  - Secret references with instance scoping
  - Multi-config support
- [ ] Create `docs/secrets.md`:
  - Secret resolution order
  - Instance context and namespacing
  - Using ~/.foundryvars for development
- [ ] Create `docs/hosts.md`:
  - Host management
  - SSH key handling
  - Configuration steps
- [ ] Update README.md with Phase 1 status

**Test Criteria**:
- [ ] Documentation is clear
- [ ] Examples work
- [ ] Links are valid
- [ ] Instance context for secrets is well explained

**Files Created**:
- `docs/getting-started.md`
- `docs/configuration.md`
- `docs/secrets.md`
- `docs/hosts.md`
- Updated `README.md`

---

## Phase 1 Completion Checklist

Before considering Phase 1 complete, verify:

- [ ] All tasks above are complete
- [ ] All unit tests pass
- [ ] All integration tests pass
- [ ] Test coverage is >80%
- [ ] Code follows Go conventions (gofmt, golint)
- [ ] Documentation is complete
- [ ] `foundry config` commands work end-to-end
- [ ] `foundry host` commands work end-to-end
- [ ] Secret reference validation works (syntax checking)
- [ ] Secret resolution works from env vars and ~/.foundryvars (with instance context)
- [ ] Instance-scoped secret paths work correctly
- [ ] SSH connections and command execution work
- [ ] Manual end-to-end test successful:
  ```bash
  foundry config init
  foundry config validate
  foundry host add <test-vm>
  foundry host list
  foundry host configure <test-vm>
  ```

## Key Design Decisions - Phase 1

### Instance Context for Secrets
- Secret references in config: `${secret:database/prod:password}`
- Resolution requires instance context: `myapp-prod/database/prod:password`
- This allows same config to be used for multiple instances
- Validation can check syntax without resolution
- Actual resolution happens when instance is known (during deployment)

### No `foundry host ssh` Command
- Users can use their own SSH clients
- Foundry manages keys and connection info
- Focus on automation, not manual access

### urfave/cli v3
- Using latest version for modern CLI features
- Check v3 docs for any API changes from v2

## Dependencies for Phase 2

Phase 1 provides the foundation for Phase 2:
- Configuration system ✓
- Secret resolution (partial - OpenBAO stub) ✓
- Instance-scoped secret paths ✓
- SSH management ✓
- Host registry ✓
- CLI framework ✓

Phase 2 will implement:
- OpenBAO installation and integration
- Real OpenBAO secret resolution
- K3s cluster management
- Component deployment system

---

**Estimated Working States**: 26 testable states (removed `foundry host ssh`)
**Estimated LOC**: ~3000-4000 lines (including tests)
**Timeline**: Not time-bound - proceed at natural pace
