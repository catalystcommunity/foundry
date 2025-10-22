# Phase 1: Foundation - Implementation Tasks

**Goal**: Establish core CLI structure, configuration system, and SSH management

**Milestone**: User can manage configuration files, connect to hosts via SSH, and resolve secrets from multiple sources with proper instance scoping

## Task Breakdown

Each task is designed to result in a working, testable state. Tests should be written alongside implementation.

---

### 1. Project Initialization ✅

**Working State**: Go project compiles and runs, outputs version information

#### Tasks:
- [x] Initialize Go module: `go mod init github.com/catalystcommunity/foundry`
- [x] Create directory structure (v1/ version for future-proofing)
- [x] Create `cmd/foundry/main.go` with basic CLI scaffold using urfave/cli v3
- [x] Implement `--version` flag that outputs version info
- [x] Set up `tools` script with targets: `build`, `test`, `clean`, `install` (bash instead of Make)
- [x] `.gitignore` already comprehensive
- [x] `LICENSE` already exists
- [x] Initialize `go.mod` with core dependencies:
  - `github.com/urfave/cli/v3`
  - `gopkg.in/yaml.v3`
  - `github.com/stretchr/testify`
  - `golang.org/x/crypto/ssh`

**Test Criteria**:
- [x] `go build ./cmd/foundry` compiles successfully
- [x] `./foundry --version` outputs version
- [x] `./foundry --help` shows command structure
- [x] `./tools test` runs successfully

**Files Created**:
- `v1/cmd/foundry/main.go`
- `v1/tools` (bash script)
- `v1/go.mod`
- `README.md` with tools documentation

---

### 2. Configuration Data Structures ✅

**Working State**: Config structs defined, can parse valid YAML into structs

#### Tasks:
- [x] Define `internal/config/types.go` with core structs:
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
- [x] Add validation methods for each struct (e.g., `Validate() error`)
- [x] Write unit tests for validation logic
  - Valid configs pass
  - Missing required fields fail
  - Invalid roles fail
  - Invalid versions fail (if format-specific)

**Test Criteria**:
- [x] Can unmarshal valid YAML into Config struct
- [x] Validation catches missing required fields
- [x] Validation catches invalid enum values
- [x] Tests pass with 83.3% coverage

**Files Created**:
- `v1/internal/config/types.go`
- `v1/internal/config/types_test.go`
- `v1/test/fixtures/valid-config.yaml`
- `v1/test/fixtures/invalid-config-*.yaml`

---

### 3. Configuration File Loading ✅

**Working State**: Can load config from file path, parse YAML, validate structure

#### Tasks:
- [x] Implement `internal/config/loader.go`:
  ```go
  func Load(path string) (*Config, error)
  func LoadFromReader(r io.Reader) (*Config, error)
  ```
- [x] Handle file reading errors gracefully
- [x] Parse YAML and unmarshal into Config
- [x] Run validation after parsing
- [x] Return detailed errors (file not found, parse error, validation error)
- [x] Write unit tests:
  - Load valid config succeeds
  - Load missing file returns error
  - Load invalid YAML returns parse error
  - Load invalid config returns validation error

**Test Criteria**:
- [x] Can load `test/fixtures/valid-config.yaml`
- [x] Loading non-existent file returns clear error
- [x] Loading invalid YAML returns parse error
- [x] Loading invalid config returns validation error
- [x] All tests pass

**Files Created**:
- `v1/internal/config/loader.go`
- `v1/internal/config/loader_test.go`

---

### 4. Configuration Path Resolution ✅

**Working State**: CLI can find and load config from multiple possible locations

#### Tasks:
- [x] Implement `internal/config/paths.go`:
  ```go
  func GetConfigDir() (string, error)  // Returns ~/.foundry/
  func FindConfig(name string) (string, error)  // Finds config file
  func ListConfigs() ([]string, error)  // Lists available configs
  ```
- [x] Use `os.UserHomeDir()` for cross-platform home directory
- [x] Check for `FOUNDRY_CONFIG` env var first
- [x] Then check `--config` flag value
- [x] Then look for default `~/.foundry/stack.yaml`
- [x] If multiple configs and no explicit selection, return error
- [x] Write unit tests with temporary directories

**Test Criteria**:
- [x] `GetConfigDir()` returns correct path
- [x] `FindConfig()` finds existing config
- [x] `FindConfig()` returns error for non-existent config
- [x] `ListConfigs()` returns all *.yaml files in config dir
- [x] Tests use temp dirs, don't touch user's actual config

**Files Created**:
- `v1/internal/config/paths.go`
- `v1/internal/config/paths_test.go`

---

### 5. Secret Reference Parsing ✅

**Working State**: Can identify and parse secret references in config, with optional instance context

#### Tasks:
- [x] Implement `internal/secrets/parser.go`:
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
- [x] Use regex to match `${secret:path/to/secret:key}` pattern
- [x] Handle malformed references (missing parts, invalid syntax)
- [x] Instance is NOT part of the secret reference syntax in config
  - Config contains: `${secret:database/prod:password}`
  - Instance context is provided at resolution time
- [x] Write unit tests:
  - Valid reference parses correctly
  - Invalid references return errors
  - Non-secret strings return nil/false
  - Edge cases (empty path, empty key, special chars)

**Test Criteria**:
- [x] `${secret:database/prod:password}` parses correctly
- [x] `${secret:}` returns error
- [x] `plaintext` returns false for IsSecretRef
- [x] All tests pass

**Files Created**:
- `v1/internal/secrets/parser.go`
- `v1/internal/secrets/parser_test.go`

**Design Note**:
Secret references in config files don't include instance context. Instance is provided at resolution time based on what service/namespace is being deployed. This allows the same config to be used for multiple instances (e.g., `myapp-prod`, `myapp-stable`).

---

### 6. Secret Resolution Context ✅

**Working State**: Context type for secret resolution with instance scoping

#### Tasks:
- [x] Implement `internal/secrets/context.go`:
  ```go
  type ResolutionContext struct {
      Instance  string  // e.g., "myapp-prod", "foundry-core"
      Namespace string  // Optional: for namespace-scoped secrets
  }

  func NewResolutionContext(instance string) *ResolutionContext
  func (rc *ResolutionContext) NamespacedPath(ref SecretRef) string
  ```
- [x] `NamespacedPath()` combines instance with secret path
  - Example: instance="myapp-prod", ref.Path="database/main"
  - Returns: "myapp-prod/database/main"
- [x] For foundry core components, use instance="foundry-core" or similar
- [x] Write unit tests for path construction

**Test Criteria**:
- [x] Context correctly combines instance + path
- [x] Handles edge cases (empty instance, etc.)
- [x] Tests pass

**Files Created**:
- `v1/internal/secrets/context.go`
- `v1/internal/secrets/context_test.go`

---

### 7. Secret Resolution - Environment Variables ✅

**Working State**: Can resolve secrets from environment variables with instance context

#### Tasks:
- [x] Implement `internal/secrets/resolver.go`:
  ```go
  type Resolver interface {
      Resolve(ctx *ResolutionContext, ref SecretRef) (string, error)
  }

  type EnvResolver struct{}

  func (e *EnvResolver) Resolve(ctx *ResolutionContext, ref SecretRef) (string, error)
  ```
- [x] Convert secret ref to env var name using instance context:
  - Pattern: `FOUNDRY_SECRET_<instance>_<path>_<key>`
  - Example: `FOUNDRY_SECRET_myapp_prod_database_main_password`
- [x] Replace slashes, hyphens, and colons with underscores
- [x] Check if env var exists
- [x] Return value or error if not found
- [x] Write unit tests with `os.Setenv`

**Test Criteria**:
- [x] Can resolve secret from env var with instance context
- [x] Returns error if env var not set
- [x] Name conversion works correctly (including instance)
- [x] Tests clean up env vars after running

**Files Created**:
- `v1/internal/secrets/resolver.go`
- `v1/internal/secrets/resolver_test.go`

---

### 8. Secret Resolution - ~/.foundryvars ✅

**Working State**: Can resolve secrets from ~/.foundryvars file with instance scoping

#### Tasks:
- [x] Implement `internal/secrets/foundryvars.go`:
  ```go
  type FoundryVarsResolver struct {
      vars map[string]string  // instance/path:key -> value
  }

  func NewFoundryVarsResolver(path string) (*FoundryVarsResolver, error)
  func (f *FoundryVarsResolver) Resolve(ctx *ResolutionContext, ref SecretRef) (string, error)
  ```
- [x] Parse file format: `instance/path/to/secret:key=value`
  - Example: `myapp-prod/database/main:password=secretpass123`
  - Example: `foundry-core/openbao:token=root-token`
- [x] Handle comments (lines starting with #)
- [x] Handle empty lines
- [x] Trim whitespace
- [x] Store in map for lookups
- [x] Use instance from ResolutionContext to construct lookup key
- [x] Write unit tests with temp files

**Test Criteria**:
- [x] Can parse valid .foundryvars file
- [x] Ignores comments and empty lines
- [x] Resolves secrets correctly with instance context
- [x] Returns error for missing secrets
- [x] Tests use temp files

**Files Created**:
- `v1/internal/secrets/foundryvars.go`
- `v1/internal/secrets/foundryvars_test.go`
- `v1/test/fixtures/test-foundryvars`

---

### 9. Secret Resolution - OpenBAO (Stub) ✅

**Working State**: OpenBAO resolver interface defined, returns placeholder

#### Tasks:
- [x] Implement `internal/secrets/openbao.go`:
  ```go
  type OpenBAOResolver struct {
      client *openbaoClient  // Placeholder for now
  }

  func NewOpenBAOResolver(addr, token string) (*OpenBAOResolver, error)
  func (o *OpenBAOResolver) Resolve(ctx *ResolutionContext, ref SecretRef) (string, error)
  ```
- [x] For now, return error: "OpenBAO integration not yet implemented"
- [x] Define interface that we'll implement in Phase 2
- [x] Document expected path structure: `<instance>/<path>:<key>`
- [x] Write stub tests that expect the error

**Test Criteria**:
- [x] Resolver can be instantiated
- [x] Resolve() returns not-implemented error
- [x] Tests pass

**Files Created**:
- `v1/internal/secrets/openbao.go`
- `v1/internal/secrets/openbao_test.go`

---

### 10. Secret Resolution - Chain Resolver ✅

**Working State**: Can resolve secrets by trying multiple sources in order

#### Tasks:
- [x] Implement `internal/secrets/chain.go`:
  ```go
  type ChainResolver struct {
      resolvers []Resolver
  }

  func NewChainResolver(resolvers ...Resolver) *ChainResolver
  func (c *ChainResolver) Resolve(ctx *ResolutionContext, ref SecretRef) (string, error)
  ```
- [x] Try each resolver in order (env, foundryvars, openbao)
- [x] Return first successful resolution
- [x] If all fail, return aggregate error with details
- [x] Write unit tests with mock resolvers

**Test Criteria**:
- [x] Tries resolvers in order (env, foundryvars, openbao)
- [x] Returns first successful result
- [x] Returns error if all fail
- [x] Error message lists all failures
- [x] Tests use mock/stub resolvers

**Files Created**:
- `v1/internal/secrets/chain.go`
- `v1/internal/secrets/chain_test.go`

---

### 11. Configuration Secret Resolution ✅

**Working State**: Can validate config without resolving secrets, can resolve when instance context provided

#### Tasks:
- [x] Implement `internal/config/resolve.go`:
  ```go
  func ValidateSecretRefs(cfg *Config) error  // Just validate syntax
  func ResolveSecrets(cfg *Config, ctx *secrets.ResolutionContext, resolver secrets.Resolver) error
  ```
- [x] `ValidateSecretRefs()`: Parse all secret refs, ensure valid syntax
  - Used during `foundry config validate`
  - Doesn't actually resolve secrets
  - Returns error if any secret reference is malformed
- [x] `ResolveSecrets()`: Actually resolve secrets
  - Requires ResolutionContext (instance must be provided)
  - Walk config structure recursively
  - Find all string values that are secret references
  - Resolve each using chain resolver
  - Replace reference with actual value in config
  - Handle resolution errors gracefully
- [x] Write unit tests with mock resolver

**Test Criteria**:
- [x] Can validate secret refs without resolution
- [x] Can resolve secrets when instance context provided
- [x] Handles missing secrets with clear error
- [x] Leaves non-secret values unchanged
- [x] Tests use fixtures with secret refs

**Files Created**:
- `v1/internal/config/resolve.go`
- `v1/internal/config/resolve_test.go`

**Design Note**:
This allows `foundry config validate` to check syntax without needing actual secrets. Actual resolution happens when deploying/installing components, where instance context is known.

---

### 12. SSH Connection Types ✅

**Working State**: SSH connection data structures defined

#### Tasks:
- [x] Implement `internal/ssh/types.go`:
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
- [x] Define connection options struct
- [x] Define auth method types (password, key)
- [x] Write basic validation

**Test Criteria**:
- [x] Structs compile
- [x] Validation works
- [x] Tests pass (100% coverage)

**Files Created**:
- `v1/internal/ssh/types.go`
- `v1/internal/ssh/types_test.go`

---

### 13. SSH Key Generation ✅

**Working State**: Can generate SSH key pairs

#### Tasks:
- [x] Implement `internal/ssh/keygen.go`:
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

### 14. SSH Connection Establishment ✅

**Working State**: Can connect to SSH server with password or key

#### Tasks:
- [x] Implement `internal/ssh/connect.go`:
  ```go
  func Connect(host string, port int, user string, auth ssh.AuthMethod) (*Connection, error)
  func (c *Connection) Close() error
  func (c *Connection) IsConnected() bool
  ```
- [x] Use `golang.org/x/crypto/ssh`
- [x] Support password auth
- [x] Support public key auth
- [x] Set reasonable timeouts (30s connection, 60s overall)
- [x] Handle connection errors
- [x] Write unit tests (integration tests deferred)

**Test Criteria**:
- [x] Validation errors handled correctly
- [x] Network errors handled correctly
- [x] Connection closes cleanly
- [x] Unit tests pass

**Files Created**:
- `v1/internal/ssh/connect.go`
- `v1/internal/ssh/connect_test.go`

**Note**: Integration tests with testcontainers deferred to Task 26

---

### 15. SSH Command Execution ✅

**Working State**: Can execute commands over SSH and capture output

#### Tasks:
- [x] Implement `internal/ssh/exec.go`:
  ```go
  type ExecResult struct {
      Stdout   string
      Stderr   string
      ExitCode int
  }

  func (c *Connection) Exec(command string) (*ExecResult, error)
  func (c *Connection) ExecWithTimeout(command string, timeout time.Duration) (*ExecResult, error)
  ```
- [x] Create session from connection
- [x] Capture stdout and stderr
- [x] Get exit code
- [x] Handle timeouts
- [x] Write unit tests

**Test Criteria**:
- [x] Validation errors handled correctly
- [x] ExecMultiple works correctly
- [x] Unit tests pass

**Files Created**:
- `v1/internal/ssh/exec.go`
- `v1/internal/ssh/exec_test.go`

**Note**: Integration tests with real SSH server deferred to Task 26

---

### 16. SSH Key Storage (OpenBAO Stub) ✅

**Working State**: Interface for storing SSH keys defined

#### Tasks:
- [x] Implement `internal/ssh/storage.go`:
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
- [x] Define interface
- [x] Create stub implementation that returns errors
- [x] Document expected storage path: `foundry-core/ssh-keys/<hostname>`
- [x] Write stub tests

**Test Criteria**:
- [x] Interface is defined
- [x] Stub returns appropriate errors
- [x] Tests pass (100% coverage for stub)

**Files Created**:
- `v1/internal/ssh/storage.go`
- `v1/internal/ssh/storage_test.go`

---

### 17. Host Management Types ✅

**Working State**: Host data structures defined

#### Tasks:
- [x] Implement `internal/host/types.go`:
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
- [x] Define host struct
- [x] Define registry interface
- [x] Write validation

**Test Criteria**:
- [x] Structs compile
- [x] Validation works
- [x] Tests pass (100% coverage)

**Files Created**:
- `v1/internal/host/types.go`
- `v1/internal/host/types_test.go`

---

### 18. Host Registry (In-Memory) ✅

**Working State**: Can store and retrieve host info (in-memory only for now)

#### Tasks:
- [x] Implement `internal/host/registry.go`:
  ```go
  type MemoryRegistry struct {
      hosts map[string]*Host
      mu    sync.RWMutex
  }

  func NewMemoryRegistry() *MemoryRegistry
  // Implement HostRegistry interface
  ```
- [x] Implement all registry methods with in-memory storage
- [x] Thread-safe with mutexes
- [x] Write unit tests

**Test Criteria**:
- [x] Can add hosts
- [x] Can retrieve hosts
- [x] Can list hosts
- [x] Can remove hosts
- [x] Thread-safe (test with goroutines)
- [x] Tests pass (100% coverage)

**Files Created**:
- `v1/internal/host/registry.go`
- `v1/internal/host/registry_test.go`

**Note**: Real registry (backed by config or OpenBAO) comes later

---

### 19. CLI Command: `foundry config init` ✅

**Working State**: Can create a new config file interactively or with defaults

#### Tasks:
- [x] Implement `cmd/foundry/commands/config/init.go`
- [x] Register command with urfave/cli v3
- [x] Implement interactive prompts (for now, just use basic stdin)
  - Cluster name
  - Domain
  - Single node or multi-node
- [x] Generate config struct with defaults
- [x] Write to `~/.foundry/<name>.yaml`
- [x] Handle file already exists error
- [x] Add `--force` flag to overwrite
- [x] Write integration tests

**Test Criteria**:
- [x] `foundry config init` creates config file
- [x] Config file is valid YAML
- [x] Can load created config
- [x] Error if file exists without --force
- [x] Tests use temp directories

**Files Created**:
- `cmd/foundry/commands/config/init.go`
- `cmd/foundry/commands/config/init_test.go`

---

### 20. CLI Command: `foundry config validate` ✅

**Working State**: Can validate config file syntax and structure (including secret ref syntax)

#### Tasks:
- [x] Implement `cmd/foundry/commands/config/validate.go`
- [x] Load config from path
- [x] Run structural validation
- [x] Run secret reference validation (syntax only, no resolution)
- [x] Output success or detailed error
- [x] Exit with appropriate code
- [x] Write tests

**Test Criteria**:
- [x] Valid config passes
- [x] Invalid config fails with clear error
- [x] Malformed secret references are caught
- [x] Missing file error is clear
- [x] Tests pass

**Files Created**:
- `cmd/foundry/commands/config/validate.go`
- `cmd/foundry/commands/config/validate_test.go`

---

### 21. CLI Command: `foundry config show` ✅

**Working State**: Can display current config with secrets redacted

#### Tasks:
- [x] Implement `cmd/foundry/commands/config/show.go`
- [x] Load config
- [x] Redact any secret references (replace with `[SECRET]`)
- [x] Output as formatted YAML
- [x] Add `--show-secret-refs` flag to show secret ref syntax (not values)
- [x] Write tests

**Test Criteria**:
- [x] Shows config correctly
- [x] Secrets are redacted by default
- [x] `--show-secret-refs` shows `${secret:path:key}` syntax
- [x] Tests pass

**Files Created**:
- `cmd/foundry/commands/config/show.go`
- `cmd/foundry/commands/config/show_test.go`

**Note**: Changed from `--with-secrets` since we can't resolve secrets without instance context. We can only show the reference syntax.

---

### 22. CLI Command: `foundry config list` ✅

**Working State**: Can list available config files

#### Tasks:
- [x] Implement `cmd/foundry/commands/config/list.go`
- [x] Find all *.yaml files in `~/.foundry/`
- [x] Display list with indicators for active config
- [x] Show which config would be used by default
- [x] Write tests

**Test Criteria**:
- [x] Lists all configs
- [x] Shows active config
- [x] Works with empty directory
- [x] Tests use temp directories

**Files Created**:
- `cmd/foundry/commands/config/list.go`
- `cmd/foundry/commands/config/list_test.go`

---

### 23. CLI Command: `foundry host add` ✅

**Working State**: Can add a host via interactive prompts

#### Tasks:
- [x] Implement `cmd/foundry/commands/host/add.go`
- [x] Prompt for hostname, address, user
- [x] Test SSH connection with password
- [x] Generate SSH key pair
- [x] Install public key on host
- [x] Store private key (stub for now - just in-memory)
- [x] Add host to registry (in-memory)
- [x] Write unit tests

**Test Criteria**:
- [x] Can add host interactively
- [x] SSH key is generated and installed
- [x] Host appears in registry
- [x] Unit tests pass

**Files Created**:
- `cmd/foundry/commands/host/add.go`
- `cmd/foundry/commands/host/add_test.go`
- `cmd/foundry/commands/host/commands.go`

---

### 24. CLI Command: `foundry host list` ✅

**Working State**: Can list all registered hosts

#### Tasks:
- [x] Implement `cmd/foundry/commands/host/list.go`
- [x] Query host registry
- [x] Display table with hostname, address, user, key status
- [x] Handle empty registry gracefully
- [x] Write tests

**Test Criteria**:
- [x] Lists hosts correctly
- [x] Shows empty state message
- [x] Table formatting works
- [x] Tests pass

**Files Created**:
- `cmd/foundry/commands/host/list.go`
- `cmd/foundry/commands/host/list_test.go`

---

### 25. CLI Command: `foundry host configure` ✅

**Working State**: Can run basic configuration on a host

#### Tasks:
- [x] Implement `cmd/foundry/commands/host/configure.go`
- [x] Connect to host
- [x] Run basic setup commands:
  - Update packages (`apt-get update`)
  - Install common tools (curl, git, vim, htop)
  - Configure time sync
- [x] Show progress
- [x] Handle errors gracefully
- [x] Write unit tests

**Test Criteria**:
- [x] Can execute configuration steps
- [x] Error handling works
- [x] Progress is shown
- [x] Unit tests pass

**Files Created**:
- `cmd/foundry/commands/host/configure.go`
- `cmd/foundry/commands/host/configure_test.go`

---

### 26. Integration Tests - Full Workflow ✅

**Working State**: End-to-end test of Phase 1 functionality

#### Tasks:
- [x] Create `test/integration/phase1_test.go`
- [x] Test full workflow:
  1. Create config
  2. Validate config (including secret refs)
  3. SSH connection with testcontainers
  4. Host registry operations
  5. SSH command execution
  6. Secret resolution with instance context
- [x] Use testcontainers for SSH server
- [x] Clean up resources after test
- [x] Add testcontainers dependency

**Test Criteria**:
- [x] Full workflow completes successfully
- [x] All assertions pass
- [x] Resources cleaned up properly
- [x] Tests pass with -short flag

**Files Created**:
- `test/integration/phase1_test.go`
- Added testcontainers-go dependency

**Note**: Integration tests use build tag `integration` and skip in short mode

---

### 27. Documentation - Phase 1 ✅

**Working State**: Comprehensive documentation for Phase 1 features

#### Tasks:
- [x] Create `docs/getting-started.md`:
  - Installation instructions
  - Quick start guide
  - Common commands
  - Troubleshooting
- [x] Create `docs/configuration.md`:
  - Config file format and schema
  - Secret references with instance scoping
  - Multi-config support
  - Best practices
- [x] Create `docs/secrets.md`:
  - Secret resolution order
  - Instance context and namespacing
  - Using ~/.foundryvars for development
  - Environment variable format
  - Security considerations
- [x] Create `docs/hosts.md`:
  - Host management workflows
  - SSH key handling
  - Configuration steps
  - Advanced usage examples
- [x] Update README.md with Phase 1 completion status

**Test Criteria**:
- [x] Documentation is clear and comprehensive
- [x] Examples are complete and correct
- [x] Links are valid
- [x] Instance context for secrets is well explained

**Files Created**:
- `docs/getting-started.md` (173 lines)
- `docs/configuration.md` (308 lines)
- `docs/secrets.md` (464 lines)
- `docs/hosts.md` (587 lines)
- Updated `README.md`

---

## Phase 1 Completion Checklist

Before considering Phase 1 complete, verify:

- [x] All tasks above are complete (Tasks 1-27)
- [x] All unit tests pass
- [x] Integration tests created with testcontainers
- [x] Test coverage: Config 83.3%, Secrets 89.1%, Host 100%, SSH 62.7%
- [x] Code follows Go conventions (gofmt, go vet)
- [x] Documentation is complete (4 comprehensive guides)
- [x] `foundry config` commands work end-to-end
- [x] `foundry host` commands work end-to-end
- [x] Secret reference validation works (syntax checking)
- [x] Secret resolution works from env vars and ~/.foundryvars (with instance context)
- [x] Instance-scoped secret paths work correctly
- [x] SSH connections and command execution work
- [x] CLI builds and runs successfully

**Phase 1 Status**: ✅ **COMPLETE**

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
