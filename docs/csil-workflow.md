# Working with CSIL-Generated Types

Foundry uses [CSIL (Catalyst Schema Interface Language)](https://github.com/catalystcommunity/csilgen) to define data structures that are persisted or interchanged. This ensures type safety, validation, and enables cross-language tooling.

## What Types Are Generated?

**Generated from CSIL** (in `.gen.go` files):
- Configuration structs (stack.yaml format)
- OpenBAO storage formats (SSH keys, K3s tokens)
- Component-specific configs
- Setup state tracking

**NOT generated** (hand-written):
- Runtime-only structs (container state, SSH connections)
- External API types (Kubernetes, OpenBAO, PowerDNS APIs)
- Business logic, validation methods, helper functions

## Developer Workflow

### 1. Modifying Existing Types

When you need to change a persisted data structure:

```bash
# 1. Edit the CSIL definition
vim csil/v1/config/network-simple.csil

# 2. Regenerate Go code
cd ../csilgen
cargo build --release
./target/release/csilgen generate \
  --input ../foundry/csil/v1/config/network-simple.csil \
  --target go \
  --output ../foundry/v1/internal/config/

# 3. The generated types.gen.go file is updated
# 4. Update hand-written code if needed
cd ../foundry/v1
vim internal/config/types.go  # Add any new validation logic

# 5. Run tests
go test ./internal/config/...
```

### 2. Adding a New Field

**Non-breaking change** (optional field):

```csil
; In network-simple.csil
NetworkConfig = {
    gateway: text,
    netmask: text,
    ? description: text,  ; NEW optional field
    ; ... rest of fields
}
```

After regeneration:
- Existing YAML files load without changes
- New field is `*string` (pointer type for optional)
- No breaking changes for existing code

**Breaking change** (required field):

```csil
NetworkConfig = {
    gateway: text,
    netmask: text,
    region: text,  ; NEW required field - BREAKING!
    ; ... rest of fields
}
```

After regeneration:
- Existing YAML files will fail to load (missing required field)
- You MUST provide migration path or default value
- All code creating this type must provide the new field

### 3. Adding Validation

Add constraints in CSIL:

```csil
NetworkConfig = {
    gateway: text,
    vlan_id: uint .ge(1) .le(4094),  ; Valid VLAN range
    hosts: [* text] .size(1..100),    ; 1-100 hosts
    ; ... rest of fields
}
```

The generator creates validation methods automatically. Extend with custom logic:

```go
// In internal/config/types.go (hand-written)

// ValidateNetwork adds business logic validation beyond CSIL constraints
func (n *NetworkConfig) ValidateNetwork() error {
    // Call generated validation first
    if err := n.Validate(); err != nil {
        return err
    }

    // Add custom validation
    if !isValidIPAddress(n.Gateway) {
        return fmt.Errorf("invalid gateway IP: %s", n.Gateway)
    }

    return nil
}
```

### 4. Working with Generated Code

**DO**:
- ✅ Edit CSIL files to change type definitions
- ✅ Regenerate `.gen.go` files after CSIL changes
- ✅ Add methods to generated types in separate files (e.g., `types.go`)
- ✅ Add custom validation in hand-written files

**DON'T**:
- ❌ Edit `.gen.go` files directly (they'll be overwritten)
- ❌ Remove the import for `setup` package from generated files (manually added for now)
- ❌ Commit CSIL changes without regenerating Go code

### 5. Detecting Breaking Changes

Before making changes, check if they're breaking:

```bash
cd ../csilgen

# Test your changes
./target/release/csilgen breaking \
  --current ../foundry/csil/v1/config/network-simple.csil \
  --new /tmp/my-changes.csil
```

Exit codes:
- `0` = No breaking changes
- `1` = Breaking changes detected

**Note**: Current csilgen has some false positives in breaking change detection. Treat warnings as advisory, not absolute.

## Common Scenarios

### Scenario: Add Optional Configuration Field

```csil
; csil/v1/components/k3s.csil
K3sConfig = {
    version: text,
    vip: text,
    ? registry_mirrors: [* text],  ; NEW: Optional registry mirrors
    ; ... existing fields
}
```

```bash
# Regenerate
csilgen generate --input csil/v1/components/k3s.csil --target go \
  --output v1/internal/component/k3s/

# Test
cd v1
go test ./internal/component/k3s/...
```

### Scenario: Change Field Type (BREAKING)

```csil
; OLD: port: text
; NEW: port: uint
StorageConfig = {
    host: text,
    port: uint,  ; Changed from text to uint
}
```

**Migration path**:
1. Add new field alongside old field (with different name)
2. Update code to use new field
3. Add migration code to convert old → new
4. After transition period, remove old field

### Scenario: Add Component Type

```csil
; Create new file: csil/v1/components/postgres.csil
options {
    go_package: "github.com/catalystcommunity/foundry/v1/internal/component/postgres"
}

PostgresConfig = {
    version: text,
    hosts: [* text],
    port: uint .default(5432),
    database: text,
}
```

```bash
# Generate
csilgen generate --input csil/v1/components/postgres.csil --target go \
  --output v1/internal/component/postgres/

# Create hand-written implementation
vim v1/internal/component/postgres/postgres.go
```

## Generated File Structure

```
v1/internal/config/
├── types.gen.go          # GENERATED - DO NOT EDIT
├── types.go              # Hand-written - validation, helpers
├── loader.go             # Hand-written - load/save logic
└── validation.go         # Hand-written - custom validation
```

## CSIL Metadata Annotations

### Field Name Overrides

```csil
NetworkConfig = {
    api_key: text @go_name("APIKey"),      ; Generates: APIKey string
    k8s_vip: text @go_name("K8sVIP"),      ; Generates: K8sVIP string
}
```

### Type Overrides

```csil
Config = {
    setup_state: any @go_type("*setup.SetupState"),  ; Cross-package reference
}
```

## Troubleshooting

### "Field X not found" after regeneration

**Cause**: You renamed or removed a field in CSIL.

**Fix**: Update all code references, update test fixtures, provide migration.

### Generated code has wrong field names

**Cause**: CSIL uses snake_case, Go uses PascalCase. Acronyms need `@go_name`.

**Fix**: Add `@go_name("FieldName")` annotation to CSIL.

### Tests fail after adding optional field

**Cause**: Test struct literals need pointer values for optional fields.

**Fix**: Use helper function:
```go
func strPtr(s string) *string { return &s }

cfg := &Config{
    Version: strPtr("1.0.0"),  // Optional field
}
```

### Import error: `setup` package not found

**Cause**: Cross-package type reference needs manual import (csilgen limitation).

**Fix**: Manually add import to generated file (temporary workaround):
```go
// In types.gen.go, after package declaration
import "github.com/catalystcommunity/foundry/v1/internal/setup"
```

## Reference

- **CSIL Definitions**: `csil/v1/` directory
- **Generated Code**: `v1/internal/*/types.gen.go` files
- **CSIL Spec**: [csilgen README](https://github.com/catalystcommunity/csilgen)
- **Migration Plan**: `foundry-csil-plan.md` (historical reference)
- **CSIL Organization**: `csil/README.md`

## Getting Help

- **Catalyst Community Discord**: https://discord.gg/sfNb9xRjPn
- **Csilgen Issues**: https://github.com/catalystcommunity/csilgen/issues
- **Foundry Issues**: https://github.com/catalystcommunity/foundry/issues
