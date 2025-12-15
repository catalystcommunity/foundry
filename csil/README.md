# Foundry CSIL Definitions

This directory contains CSIL (Common Schema Intermediate Language) definitions for all persisted and interchanged data structures in Foundry.

## Purpose

CSIL provides a single source of truth for data structure definitions, enabling:

1. **Breaking change detection** - Automated detection of incompatible changes across versions
2. **Cross-language code generation** - Generate types for Go, TypeScript, Python, Rust, etc.
3. **Formal documentation** - Machine-readable specifications of data formats
4. **Compile-time validation** - Type safety and constraint checking

## What's Defined in CSIL

**IN SCOPE** (Migrated to CSIL):
- YAML configuration files (`stack.yaml`)
- OpenBAO storage formats (SSH keys, K3s tokens)
- Generated config files for other tools (Zot `config.json`, K3s `registries.yaml`)
- Component-specific configurations

**OUT OF SCOPE** (NOT in CSIL):
- External API types (Kubernetes API, OpenBAO API responses, PowerDNS API)
- Runtime-only structs (never persisted or interchanged)

## Directory Structure

```
csil/
├── v1/                          # Version 1 definitions
│   ├── config/                  # YAML configuration types
│   │   └── network-simple.csil  # Config, NetworkConfig, DNSConfig, etc.
│   ├── openbao/                 # OpenBAO storage formats
│   │   └── ssh-keys.csil        # SSH key storage format
│   ├── components/              # Component-specific config types
│   │   ├── openbao.csil         # OpenBAO component config
│   │   ├── k3s.csil             # K3s component config
│   │   ├── dns.csil             # DNS component config
│   │   ├── zot.csil             # Zot component config
│   │   ├── contour.csil         # Contour component config
│   │   └── certmanager.csil     # cert-manager component config
│   └── setup/                   # Setup wizard state
│       └── state.csil           # SetupState
└── README.md                    # This file
```

## Code Generation

### Generate All Go Types (Single Command)

From the foundry repository root:

```bash
csilgen generate --input csil/v1/ --target go --output v1/
```

This single command:
- Processes all CSIL files in `csil/v1/`
- Uses `go_module` and `go_package` options to determine output paths
- Generates `types.gen.go` in each target package directory

### How Package Routing Works

Each CSIL file specifies its target Go package via options:

```csil
options {
    go_module: "github.com/catalystcommunity/foundry/v1",
    go_package: "github.com/catalystcommunity/foundry/v1/internal/config"
}
```

The Go generator strips `go_module` from `go_package` to get the relative output path.
For example: `go_package` minus `go_module` = `internal/config` -> outputs to `v1/internal/config/types.gen.go`

### Generated Code Structure

```
v1/internal/
├── config/
│   ├── types.gen.go        # Generated from csil/v1/config/network-simple.csil
│   ├── types.go            # Hand-written (validation, type aliases)
│   └── loader.go           # Hand-written (uses generated types)
├── setup/
│   └── types.gen.go        # Generated from csil/v1/setup/state.csil
├── ssh/
│   └── types.gen.go        # Generated from csil/v1/openbao/ssh-keys.csil
└── component/
    ├── certmanager/
    │   └── types.gen.go    # Generated from csil/v1/components/certmanager.csil
    ├── contour/
    │   └── types.gen.go    # Generated from csil/v1/components/contour.csil
    ├── dns/
    │   └── types.gen.go    # Generated from csil/v1/components/dns.csil
    ├── k3s/
    │   └── types.gen.go    # Generated from csil/v1/components/k3s.csil
    ├── openbao/
    │   └── types.gen.go    # Generated from csil/v1/components/openbao.csil
    └── zot/
        └── types.gen.go    # Generated from csil/v1/components/zot.csil
```

## CSIL File Requirements

Each CSIL file that generates Go code must have:

1. **`go_module`** - The Go module path (from `go.mod`)
2. **`go_package`** - The full Go package path for generated types

Example:
```csil
options {
    go_module: "github.com/catalystcommunity/foundry/v1",
    go_package: "github.com/catalystcommunity/foundry/v1/internal/component/dns",
    go_imports: ["github.com/catalystcommunity/foundry/v1/internal/host"]  ; optional
}

Config = {
    version: text,
    namespace: text
}
```

### External Type References

For types defined in other packages, use `@go_type` annotation:

```csil
Config = {
    hosts: [* any] @go_type("[]*host.Host"),
    setup_state: any @go_type("*setup.SetupState")
}
```

Note: Type aliases to external packages (e.g., `type Host = host.Host`) must be defined
in hand-written Go files, as CSIL cannot express cross-package type aliases.

## Versioning Strategy

### Version Directories

- `csil/v1/` - Version 1 definitions (current)
- `csil/v2/` - Version 2 definitions (future)

### Breaking Changes

Check for breaking changes between versions:

```bash
csilgen breaking \
  --current csil/v1/config/network-simple.csil \
  --new csil/v2/config/network-simple.csil
```

**Breaking changes include**:
- Removing required fields
- Changing field types
- Adding new required fields
- Renaming fields

**Non-breaking changes include**:
- Adding optional fields
- Adding new types
- Documentation updates

## Common Tasks

### Regenerate All Types

```bash
csilgen generate --input csil/v1/ --target go --output v1/
```

### Adding a New Field

1. Add field to CSIL file:
   ```csil
   Config = {
       existing_field: text,
       ? new_field: text  ; Optional field
   }
   ```

2. Regenerate: `csilgen generate --input csil/v1/ --target go --output v1/`

3. Update hand-written code if needed

4. Run tests: `go test ./...`

### Adding a New Component

1. Create CSIL file: `csil/v1/components/newcomponent.csil`

2. Add options block with `go_module` and `go_package`

3. Define types

4. Regenerate all types

5. Create hand-written integration code in the new package

## Links

- [Foundry DESIGN.md](../DESIGN.md) - Overall architecture
- [csilgen README](https://github.com/catalystcommunity/csilgen) - csilgen usage and documentation
