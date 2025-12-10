# Getting Started with Foundry

Foundry is a CLI tool for managing Catalyst Community tech stacks. This guide will help you get started with the basics.

## Before You Begin

**Prerequisites:** Foundry requires specific infrastructure and software to be in place before you begin. Please review the [Prerequisites Guide](./prerequisites.md) to ensure your environment is ready.

**Important Notes:**
- Foundry assumes **full management** of infrastructure hosts
- Foundry may configure DNS settings, systemd services, and container workloads
- SSH keys are generated automatically and stored securely in OpenBAO after bootstrap
- The bootstrap process tracks state to enable resuming from any checkpoint

## Installation

### From Source

**Requirements:** Go 1.23 or later ([installation instructions](./prerequisites.md#install-go))

```bash
git clone https://github.com/catalystcommunity/foundry.git
cd foundry
go build -o foundry ./cmd/foundry
sudo mv foundry /usr/local/bin/
```

**Note:** You can also keep the binary in the project directory and run it as `./foundry` if you prefer not to install system-wide.

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

**SSH Key Lifecycle:**
- Keys are initially generated and used immediately for connections
- After OpenBAO is installed and initialized, keys are migrated to OpenBAO
- All future operations retrieve keys from OpenBAO for secure storage
- This bootstrap → secure storage transition is handled automatically

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

### Bootstrap State Tracking

Foundry tracks installation progress in the `_setup_state` section of your config:

```yaml
_setup_state:
  network_planned: false
  network_validated: false
  openbao_installed: false
  dns_installed: false
  dns_zones_created: false
  zot_installed: false
  k8s_installed: false
  stack_complete: false
```

This enables:
- **Resumability:** Stop and resume installation at any checkpoint
- **Safety:** Prevent duplicate installations
- **Clarity:** See exactly where you are in the bootstrap process

You can view your current state with: `foundry config show`

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

### Learning Resources

- Read the [Configuration Guide](./configuration.md) to learn about config file structure
- Read the [Secrets Guide](./secrets.md) to understand secret management
- Review [Component Documentation](./components.md) to understand the stack components
- Learn about [DNS Configuration](./dns.md) for PowerDNS setup
- Explore [Storage Configuration](./storage.md) for Longhorn and SeaweedFS setup

### Validation Testing

If you're validating a new Foundry installation or contributing to development:

- Review `phase2-validation.md` in the project root for detailed testing procedures
- This includes step-by-step validation of the complete bootstrap process

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

### Bootstrap and Interim State Issues

If you encounter issues during the bootstrap process:

**SSH Keys Not Working:**
- Check if OpenBAO is installed: `foundry component status openbao`
- If OpenBAO is not yet installed, keys are stored in-memory only
- After OpenBAO is available, keys will be migrated automatically

**Installation Stuck:**
- Check `_setup_state` in your config: `foundry config show`
- You can resume from any checkpoint by re-running the failed command
- State tracking prevents duplicate installations

**Reset and Start Over:**
- Use `scripts/reset-local-state.sh` to clean local state
- Reset VMs via Proxmox UI to restore to clean state
- This is safe and designed for testing/validation

**DNS Configuration:**
- Foundry installs PowerDNS for internal service discovery
- PowerDNS forwards external queries to your existing DNS
- Host DNS configuration may need adjustment (documented during validation)

### Getting Help

- Check the documentation in the `docs/` directory
- Review [Prerequisites](./prerequisites.md) for environment setup
- Review `phase2-validation.md` for detailed bootstrap validation
- Run commands with `--help` flag
- Join the [Catalyst Community Discord](https://discord.gg/sfNb9xRjPn)
- Report issues on [GitHub](https://github.com/catalystcommunity/foundry/issues)
