# Getting Started with Foundry

Foundry is a CLI tool for deploying a complete Kubernetes-based infrastructure stack on Debian/Ubuntu hosts.

## Prerequisites

Before you begin, ensure you have:

**On each infrastructure host:**
- Debian 11/12 or Ubuntu 22.04/24.04 installed
- SSH server running with password authentication enabled
- A non-root user account (or root access)
- Static IP address (or DHCP reservation)

**On your network:**
- An available VIP (Virtual IP) for Kubernetes that won't conflict with other IPs
- Outbound internet access for downloading packages and container images

**On your management machine:**
- Go 1.23+ installed (for building Foundry from source)
- Network access to your infrastructure hosts via SSH

See the [Prerequisites Guide](./prerequisites.md) for detailed requirements and validation scripts.

## Installation

```bash
git clone https://github.com/catalystcommunity/foundry.git
cd foundry/v1
go build -o foundry ./cmd/foundry
sudo mv foundry /usr/local/bin/

# Verify
foundry --version
```

## Quick Start: Single Host

The fastest way to get started is a single-host deployment where one machine runs everything.

**What you need:**
- One Debian/Ubuntu host with SSH access
- A VIP address (unused IP in the same subnet)

**Run one command:**

```bash
foundry stack install
```

**You'll be prompted for:**
1. Cluster name (default: `my-cluster`)
2. Domain (default: `catalyst.local`)
3. Infrastructure host in `hostname:ip` format (e.g., `node1:192.168.1.10`)
4. SSH user (default: `root`)
5. Kubernetes VIP (e.g., `192.168.1.20`)

**During installation, you'll also be prompted for:**
- SSH password for the specified user (used once to install SSH key)
- Root password (only if the SSH user doesn't have sudo access)

The installation takes 10-20 minutes and installs:
- OpenBAO (secrets management)
- PowerDNS (internal DNS)
- Zot (container registry)
- K3s (Kubernetes)
- Gateway API + Contour (ingress)
- Cert-Manager (certificates)
- Longhorn (storage)
- SeaweedFS (S3-compatible storage)
- Prometheus + Loki + Grafana (observability)
- Velero (backups)

**After installation:**

```bash
# Check cluster status
kubectl --kubeconfig ~/.foundry/kubeconfig get nodes

# View stack status
foundry stack status
```

## Quick Start: Multi-Host

For production deployments, you'll want multiple hosts with different roles.

### Option A: Interactive Setup

```bash
# Initialize config (creates ~/.foundry/stack.yaml)
foundry config init

# Add each host with specific roles
foundry host add node1 --address 192.168.1.10 --user myuser \
  --roles openbao,dns,zot,cluster-control-plane

foundry host add node2 --address 192.168.1.11 --user myuser \
  --roles cluster-worker

foundry host add node3 --address 192.168.1.12 --user myuser \
  --roles cluster-worker

# Install the stack
foundry stack install
```

### Option B: Edit Configuration File

```bash
# Initialize config
foundry config init

# Edit ~/.foundry/stack.yaml to add your hosts
```

Example multi-host configuration:

```yaml
cluster:
  name: production
  domain: catalyst.local
  vip: 192.168.1.20

hosts:
  - hostname: control1
    address: 192.168.1.10
    port: 22
    user: myuser
    roles:
      - openbao
      - dns
      - zot
      - cluster-control-plane

  - hostname: worker1
    address: 192.168.1.11
    port: 22
    user: myuser
    roles:
      - cluster-worker

  - hostname: worker2
    address: 192.168.1.12
    port: 22
    user: myuser
    roles:
      - cluster-worker
```

Then run:

```bash
foundry stack install
```

### Available Roles

| Role | Description |
|------|-------------|
| `openbao` | OpenBAO secrets management server |
| `dns` | PowerDNS server for internal DNS |
| `zot` | Container image registry |
| `cluster-control-plane` | Kubernetes control plane node |
| `cluster-worker` | Kubernetes worker node |

## Accessing Services

After installation, services are accessible via DNS names on the `catalyst.local` domain (or your configured domain).

**To access services in your browser:**

1. Configure your machine to use the PowerDNS server for DNS resolution
2. Point your DNS to the host with the `dns` role

**Available services:**
- `grafana.catalyst.local` - Observability dashboards
- `prometheus.catalyst.local` - Metrics
- `openbao.catalyst.local` - Secrets management UI
- `zot.catalyst.local` - Container registry

**DNS Resolution:**
- Infrastructure services (`openbao`, `dns`, `zot`) resolve to their host IPs
- All other `*.catalyst.local` names resolve to the VIP (handled by Kubernetes ingress)

## Configuration Directory

Foundry stores all state in `~/.foundry/`:

```
~/.foundry/
├── stack.yaml      # Stack configuration
├── kubeconfig      # Kubernetes access config
└── keys/           # SSH keys (migrated to OpenBAO after install)
```

## Resuming Installation

If installation is interrupted, simply run `foundry stack install` again. Foundry tracks progress and resumes from the last checkpoint.

Check current progress:

```bash
foundry config show
```

## Common Commands

```bash
# Stack management
foundry stack install          # Install complete stack
foundry stack status           # Show installation status

# Configuration
foundry config init            # Create new configuration
foundry config validate        # Validate configuration
foundry config show            # Display current config

# Host management
foundry host add               # Add a new host
foundry host list              # List all hosts
foundry host configure <name>  # Configure a specific host

# Components
foundry component list         # List available components
foundry component status       # Show component status
```

## Troubleshooting

### SSH Connection Issues

```bash
# Verify host is reachable
ping <host-ip>

# Verify SSH is running
nc -zv <host-ip> 22

# Test SSH connection manually
ssh user@host-ip
```

### DNS Not Resolving

1. Verify PowerDNS is running: `foundry component status dns`
2. Ensure your machine uses the DNS server IP for resolution
3. Test with: `dig @<dns-server-ip> grafana.catalyst.local`

### Installation Stuck

1. Check progress: `foundry config show`
2. Re-run: `foundry stack install` (resumes from checkpoint)
3. Check logs on the host if a specific component fails

## Next Steps

- [Configuration Guide](./configuration.md) - Detailed configuration options
- [DNS Guide](./dns.md) - PowerDNS configuration
- [Storage Guide](./storage.md) - Longhorn and SeaweedFS
- [Observability Guide](./observability.md) - Prometheus, Loki, Grafana
- [Backups Guide](./backups.md) - Velero backup configuration

## Getting Help

- Join the [Catalyst Community Discord](https://discord.gg/sfNb9xRjPn)
- Report issues on [GitHub](https://github.com/catalystcommunity/foundry/issues)
