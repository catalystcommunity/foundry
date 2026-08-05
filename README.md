# Foundry

A CLI for managing Catalyst Community tech stacks and workflows.

## Status

Foundry is under active development. The CLI can manage configuration, hosts,
stack installation, Kubernetes components, storage, backups, DNS, and
observability services.

## Development

This project uses the `v1/tools` script for development commands.

### Building

```bash
cd v1
./tools build        # Build the foundry binary
./tools build-static # Build a static binary (no CGO)
```

### Testing

```bash
scripts/test-local.sh               # Run fast validation
scripts/test-local.sh --integration # Run integration tests
```

See the [Testing Guide](./docs/testing.md) for kind and package-specific modes.

### Other Commands

```bash
./tools lint    # Run linters (gofmt, go vet)
./tools clean   # Remove build artifacts
./tools install # Install to GOPATH/bin
./tools help    # Show all available commands
```

### Working with CSIL-Generated Types

Foundry uses [CSIL](https://github.com/catalystcommunity/csilgen) to define
persistent data structures. Update the CSIL definitions before you regenerate
Go types.

See the [CSIL Workflow Guide](./docs/csil-workflow.md) for these tasks:
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
│   ├── test/                  # Test fixtures and integration tests
│   └── tools                  # Development tool script
├── DESIGN.md                  # Architecture and design decisions
├── docs/                       # User and developer guides
└── scripts/                    # Local test scripts
```

## Documentation

### Design & Planning

- [DESIGN.md](./DESIGN.md) - Architecture, philosophy, and design decisions
- [CLAUDE.md](./CLAUDE.md) - Development best practices and guidelines

### Developer Guides

- [CSIL Workflow](./docs/csil-workflow.md) - Working with CSIL-generated types

### User Guides

- [Getting Started](./docs/getting-started.md) - Quick start guide and common commands
- [Configuration](./docs/configuration.md) - Configuration file format and management
- [Secrets](./docs/secrets.md) - Secret reference and resolution behavior
- [Hosts](./docs/hosts.md) - Infrastructure host management
- [Gateway Controller](./docs/gateway-controller.md) - Route-driven L4 (TCP/TLS) listeners on the cluster VIP

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
