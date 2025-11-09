# Foundry

A CLI for managing Catalyst Community tech stacks and workflows.

## Status

**Phase 1 - Complete**: Foundation built with CLI structure, configuration management, secrets resolution, SSH operations, and host management.

**Next**: Phase 2 will add stack installation (OpenBAO, K3s, Zot registry)

See [implementation-tasks.md](./implementation-tasks.md) for detailed progress tracking.

## Development

This project uses a `tools` bash script instead of Make for better portability.

### Building

```bash
cd v1
./tools build        # Build the foundry binary
./tools build-static # Build a static binary (no CGO)
```

### Testing

```bash
./tools test              # Run unit tests
./tools test-integration  # Run integration tests (requires Docker)
./tools coverage          # Generate coverage report
```

### Other Commands

```bash
./tools lint    # Run linters (gofmt, go vet)
./tools clean   # Remove build artifacts
./tools install # Install to GOPATH/bin
./tools help    # Show all available commands
```

### Working with CSIL-Generated Types

Foundry uses [CSIL](https://github.com/catalystcommunity/csilgen) to define persisted data structures. When modifying configuration types, component configs, or storage formats, you'll need to update CSIL definitions and regenerate Go code.

See the [CSIL Workflow Guide](./docs/csil-workflow.md) for detailed instructions on:
- Modifying existing types
- Adding new fields (breaking vs. non-breaking)
- Regenerating Go code
- Handling breaking changes

## Project Structure

```
foundry/
├── v1/                          # Version 1 module
│   ├── cmd/foundry/            # Main entry point
│   ├── internal/               # Internal packages
│   │   ├── config/            # Configuration management
│   │   ├── secrets/           # Secret resolution
│   │   ├── ssh/               # SSH operations
│   │   └── host/              # Host management
│   ├── pkg/                   # Public APIs (if needed)
│   ├── test/                  # Test fixtures and integration tests
│   └── tools                  # Development tool script
├── DESIGN.md                  # Architecture and design decisions
├── implementation-tasks.md    # Implementation tracking
└── phase-implementation-*.md  # Detailed phase breakdowns
```

## Documentation

### Design & Planning
- [DESIGN.md](./DESIGN.md) - Architecture, philosophy, and design decisions
- [implementation-tasks.md](./implementation-tasks.md) - Implementation phases and status
- [CLAUDE.md](./CLAUDE.md) - Development best practices and guidelines

### Developer Guides
- [CSIL Workflow](./docs/csil-workflow.md) - Working with CSIL-generated types

### User Guides
- [Getting Started](./docs/getting-started.md) - Quick start guide and common commands
- [Configuration](./docs/configuration.md) - Configuration file format and management
- [Secrets](./docs/secrets.md) - Secret management with instance scoping
- [Hosts](./docs/hosts.md) - Infrastructure host management

## Quick Start

```bash
# Build Foundry
cd v1
./tools build

# Initialize a configuration
./foundry config init

# Add a host
./foundry host add

# List hosts
./foundry host list

# Configure a host
./foundry host configure <hostname>
```

See the [Getting Started Guide](./docs/getting-started.md) for more details.

## Contributing

Join the [Catalyst Community Discord](https://discord.gg/sfNb9xRjPn) to discuss and contribute.

## License

See [LICENSE](./LICENSE) file for details.
