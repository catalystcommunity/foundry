# Getting Started with Foundry

Foundry is a CLI tool for managing Catalyst Community tech stacks. This guide will help you get started with the basics.

## Installation

### From Source

```bash
git clone https://github.com/catalystcommunity/foundry.git
cd foundry/v1
go build -o foundry ./cmd/foundry
sudo mv foundry /usr/local/bin/
```

### Verify Installation

```bash
foundry --version
```

## Quick Start

### 1. Initialize a Configuration

Create your first Foundry configuration:

```bash
foundry config init
```

This will interactively prompt you for:
- Cluster name
- Domain name
- Initial node configuration

The configuration will be saved to `~/.foundry/stack.yaml`.

### 2. Validate Your Configuration

```bash
foundry config validate
```

This validates:
- YAML syntax
- Required fields
- Secret reference syntax
- Node configuration

### 3. Add a Host

Add a remote host to your infrastructure:

```bash
foundry host add
```

You'll be prompted for:
- Hostname (friendly name)
- Address (IP or FQDN)
- SSH user
- SSH password (for initial setup)

Foundry will:
1. Test the SSH connection
2. Generate an SSH key pair
3. Install the public key on the host
4. Store the host in the registry

### 4. List Your Hosts

```bash
foundry host list
```

For detailed information:

```bash
foundry host list --verbose
```

### 5. Configure a Host

Run basic configuration on a host:

```bash
foundry host configure <hostname>
```

This will:
- Update package lists
- Install common tools (curl, git, vim, htop)
- Configure time synchronization

## Configuration Directory

Foundry stores configuration in `~/.foundry/`:

```
~/.foundry/
├── stack.yaml           # Your stack configuration
├── other-config.yaml    # Additional configurations
└── ...
```

## Secret Management

For development, Foundry can read secrets from `~/.foundryvars`:

```bash
# Create a .foundryvars file in your home directory
cat > ~/.foundryvars <<EOF
# Format: instance/path:key=value
myapp-prod/database/main:password=secret123
foundry-core/openbao:token=root-token
EOF

chmod 600 ~/.foundryvars
```

See the [Secrets Guide](./secrets.md) for more details.

## Next Steps

- Read the [Configuration Guide](./configuration.md) to learn about config file structure
- Read the [Secrets Guide](./secrets.md) to understand secret management
- Read the [Hosts Guide](./hosts.md) for advanced host management

## Common Commands

```bash
# Configuration management
foundry config init                  # Create new config
foundry config validate              # Validate config
foundry config show                  # Display current config
foundry config list                  # List all configs

# Host management
foundry host add                     # Add a new host
foundry host list                    # List all hosts
foundry host list --verbose          # List hosts with details
foundry host configure <hostname>    # Configure a host

# General
foundry --help                       # Show help
foundry --version                    # Show version
```

## Troubleshooting

### SSH Connection Issues

If you have trouble connecting to a host:

1. Verify the host is reachable: `ping <address>`
2. Verify SSH is running: `nc -zv <address> 22`
3. Verify credentials are correct
4. Check firewall rules

### Configuration Issues

If config validation fails:

1. Check YAML syntax
2. Verify all required fields are present
3. Check secret reference syntax: `${secret:path:key}`

### Getting Help

- Check the documentation in the `docs/` directory
- Run commands with `--help` flag
- Join the [Catalyst Community Discord](https://discord.gg/sfNb9xRjPn)
