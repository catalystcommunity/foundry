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
│   │   ├── core.csil            # Config, NetworkConfig, DNSConfig
│   │   ├── cluster.csil         # ClusterConfig, NodeConfig
│   │   ├── components.csil      # ComponentConfig, ComponentMap
│   │   ├── observability.csil   # Observability configs
│   │   └── storage.csil         # Storage configs
│   ├── openbao/                 # OpenBAO storage formats
│   │   ├── ssh-keys.csil        # SSH key storage format
│   │   └── k3s-tokens.csil      # K3s token storage format
│   ├── components/              # Component-specific config types
│   │   ├── openbao.csil         # OpenBAO component config
│   │   ├── k3s.csil             # K3s component config
│   │   ├── dns.csil             # DNS component config
│   │   ├── zot.csil             # Zot component config
│   │   ├── contour.csil         # Contour component config
│   │   └── certmanager.csil     # cert-manager component config
│   ├── generated-configs/       # Config files we generate for other tools
│   │   └── zot-config.csil      # Zot's config.json format
│   └── setup/                   # Setup wizard state
│       └── state.csil           # SetupState
└── README.md                    # This file
```

## Code Generation

### Generate Go Types

From the `csilgen` directory:

```bash
# Generate from a single CSIL file
csilgen generate \
  --input ../foundry/csil/v1/config/core.csil \
  --target go \
  --output ../foundry/v1/internal/config/generated/

# Generated code goes in <package>/generated/ subdirectory
```

### Generated Code Structure

```
v1/internal/config/
├── generated/              # Generated from CSIL
│   ├── types.go           # Generated structs
│   └── .generated         # Marker file (DO NOT EDIT)
├── loader.go              # Hand-written (uses generated types)
└── validation.go          # Hand-written (business logic)
```

## Versioning Strategy

### Version Directories

- `csil/v1/` - Version 1 definitions (current)
- `csil/v2/` - Version 2 definitions (future)

### Breaking Changes

Check for breaking changes between versions:

```bash
csilgen breaking \
  --current csil/v1/config/core.csil \
  --new csil/v2/config/core.csil
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

### Migration Process

1. Create new version directory: `csil/v2/`
2. Copy and modify CSIL files
3. Run breaking change detection
4. Generate code for both versions
5. Migrate hand-written code incrementally

## Current Parser Limitations

The current csilgen parser (as of Phase 2) has some limitations:

1. **No `.default()` constraints** - Document defaults in comments instead
2. **No optional field syntax (`?`)** - All fields are required; use pointers in Go for optionals
3. **No constraint operators** - `.size()`, `.ge()`, etc. not fully supported
4. **Inline map syntax limited** - Extract `{* text => Type}` to top-level type aliases
5. **Reserved keywords** - "service" is reserved; use alternatives like "svc"

**Workarounds**:
- Document default values in comments
- Use generated constructors in Go for defaults
- Define map types separately (e.g., `SubPathMap = {* text => SubPath}`)

## Common Tasks

### Adding a New Field

1. Add field to CSIL file:
   ```csil
   Config = {
       existing_field: text,
       new_field: text,  ; New optional field
   }
   ```

2. Regenerate Go code:
   ```bash
   csilgen generate --input csil/v1/config/core.csil --target go --output v1/internal/config/generated/
   ```

3. Update hand-written code to use new field

4. Run tests: `go test ./...`

### Modifying Field Types

1. Check for breaking changes first
2. Update CSIL definition
3. Regenerate code
4. Update hand-written code
5. Test thoroughly

### Adding a New Type

1. Create CSIL file in appropriate directory
2. Define type with full documentation
3. Generate Go code
4. Create hand-written integration code
5. Add tests

## Integration with Foundry

Generated types are integrated via type aliases and imports:

```go
// v1/internal/config/types.go
package config

import "github.com/catalystcommunity/foundry/v1/internal/config/generated"

// Config is generated from CSIL definitions
type Config = generated.Config

// Additional hand-written validation
func (c *Config) ValidateCrossField() error {
    // Complex business logic not in CSIL
}
```

## Links

- [Foundry DESIGN.md](../DESIGN.md) - Overall architecture
- [Foundry CSIL Plan](../foundry-csil-plan.md) - Migration plan and status
- [csilgen README](../../csilgen/README.md) - csilgen usage and documentation

## Contributing

When modifying CSIL definitions:

1. **Document changes** - Update comments to explain field purposes
2. **Check for breaking changes** - Run `csilgen breaking` before major changes
3. **Regenerate code** - Always regenerate after CSIL changes
4. **Test thoroughly** - Ensure all tests pass after regeneration
5. **Update migration plan** - Mark tasks complete in `foundry-csil-plan.md`
