# Prerequisites

This document outlines the prerequisites for deploying and managing infrastructure with Foundry.

## TL;DR - What You Actually Need

**Foundry follows a "dogfooding" principle:** The CLI should handle as much infrastructure setup as possible.

**Minimal Requirements (ONLY these are required):**
1. Base Debian 11/12 or Ubuntu 22.04/24.04 installation
2. SSH server running (openssh-server)
3. Non-root user account created
4. User password (for initial SSH connection)
5. Network connectivity (SSH accessible)

**Everything Else is Handled by Foundry:**
- ✅ sudo installation and configuration
- ✅ Container runtime (containerd + nerdctl)
- ✅ CNI networking plugins
- ✅ iptables/nftables
- ✅ Common system tools
- ✅ Time synchronization
- ✅ SSH key generation and installation

When you run `foundry host configure`, it will automatically detect and install missing components, including prompting for root password if sudo isn't configured.

---

## System Requirements

### Infrastructure Hosts

Each host in your Catalyst Stack infrastructure should meet the following requirements:

**Operating System:**
- Debian 11 or 12 (Bullseye/Bookworm)
- Ubuntu 22.04 LTS or 24.04 LTS

**Note:** Foundry currently supports Debian and Ubuntu. Other distributions may be added in future releases.

**Hardware (Minimum):**
- **CPU:** 4 cores (8+ recommended for Kubernetes nodes)
- **RAM:** 8 GB (16+ GB recommended for Kubernetes control plane nodes)
- **Disk:** 50 GB available space (more if using local storage)
- **Network:** 1 Gbps network interface

**Hardware (Production):**
- **CPU:** 8+ cores
- **RAM:** 32+ GB for control plane, 16+ GB for workers
- **Disk:** 100+ GB SSD for system, separate volumes for persistent storage
- **Network:** 10 Gbps for high-throughput workloads

### Management Machine

The machine where you run the Foundry CLI:

**Requirements:**
- **Go:** Version 1.23 or later (for building from source)
- **Network Access:** Must be able to reach infrastructure hosts via SSH
- **Disk:** Minimal space for Foundry binary and config (~100 MB)

## Required Software on Infrastructure Hosts

### SSH Server (Required)

**Requirements:**
- OpenSSH Server installed and running
- Port 22 accessible (or custom port configured in Foundry)
- Password authentication enabled (for initial setup)
- User account with password access

**Installation:**
```bash
# Debian/Ubuntu
sudo apt-get update
sudo apt-get install -y openssh-server
sudo systemctl enable --now sshd
```

**Verification:**
```bash
# Check SSH is running
systemctl status sshd

# Check SSH is accessible from management machine
nc -zv <host-ip> 22
```

**Enabling Password Authentication (if disabled):**
```bash
# Edit sshd_config
sudo vi /etc/ssh/sshd_config

# Ensure this line is present:
PasswordAuthentication yes

# Restart SSH
sudo systemctl restart sshd
```

**Note:** After initial setup, Foundry will generate and install SSH keys, so password authentication can be disabled if desired.

### Systemd (Included in Base OS)

Foundry uses systemd to manage containerized services.

**Requirements:**
- Systemd as the init system (PID 1) - included in Debian/Ubuntu by default
- Foundry will handle sudo configuration for systemd service management

**Verification:**
```bash
# Check systemd is running
systemctl --version
```

### Container Runtime (Automatically Installed)

**Foundry automatically installs a container runtime during host configuration.**

When you run `foundry host configure`, Foundry will:
1. Detect if Docker is already installed (uses it if present)
2. If not, install containerd + nerdctl + CNI plugins
3. Install iptables (uses iptables-nft on Debian 11/12)
4. Create a `docker` symlink to `nerdctl` for CLI compatibility
5. Verify the installation with a test container

**What Gets Installed:**
- **containerd:** Lightweight container runtime
- **nerdctl:** Docker-compatible CLI for containerd
- **CNI plugins:** Container networking (bridge, host, loopback, etc.)
- **iptables:** For CNI networking (uses nftables backend on modern Debian/Ubuntu)

**Migration Path:**
Foundry installs iptables which uses the nftables backend on Debian 11/12. This provides compatibility with existing tooling while using modern nftables. When Kubernetes 1.33+ becomes available with GA nftables support, you can migrate to pure nftables mode.

**Manual Pre-Installation (Optional):**
If you prefer to install Docker yourself before running Foundry:
```bash
# Docker installation (optional - Foundry will do this automatically)
curl -fsSL https://get.docker.com | sh
sudo usermod -aG docker $USER
```

### sudo Access (Automatically Configured)

**Foundry automatically detects and configures sudo if needed.**

When you run `foundry host configure`, Foundry will:
1. Check if the user has sudo access
2. If not, prompt for the root password
3. Install the sudo package if missing
4. Add the user to the sudoers group
5. Configure passwordless sudo for convenience
6. Continue with the rest of host configuration

**What This Means:**
- You don't need to manually configure sudo
- You just need the root password on first setup
- After that, all Foundry operations work seamlessly

**Manual Pre-Configuration (Optional):**
If you prefer to set up sudo yourself before using Foundry:
```bash
# As root or via su
apt-get install -y sudo
usermod -aG sudo <username>

# Optional: Configure passwordless sudo
echo "<username> ALL=(ALL) NOPASSWD:ALL" > /etc/sudoers.d/<username>
chmod 0440 /etc/sudoers.d/<username>
```

## Network Requirements

### IP Addressing

**Static IP or DHCP Reservation:**
- Each infrastructure host should have a stable IP address
- Use static IP configuration OR DHCP reservation
- IP address should not change between reboots

**Available VIP:**
- You must have an available IP address for the Kubernetes cluster VIP
- This IP should NOT be assigned to any host
- It should be in the same subnet as your infrastructure hosts
- It should be outside any DHCP range to prevent conflicts

**Example Network Layout:**
```
Network: 192.168.1.0/24
Gateway: 192.168.1.1
DHCP Range: 192.168.1.100 - 192.168.1.200

Infrastructure Hosts:
- foundry-vm-01: 192.168.1.10 (static or reserved)
- foundry-vm-02: 192.168.1.11 (static or reserved)
- foundry-vm-03: 192.168.1.12 (static or reserved)

Kubernetes VIP: 192.168.1.20 (available, not assigned)
```

### DNS

**Upstream DNS Servers:**
- Your network should have functional DNS servers
- These will be used as forwarders by PowerDNS
- Typically provided by your network via DHCP or configured statically

**DNS Configuration Notes:**
- Foundry installs PowerDNS for internal service discovery
- PowerDNS forwards external queries to your existing DNS servers
- Host DNS configuration may need adjustment during installation

### Outbound Internet Access

**Required for:**
- Downloading container images (Docker Hub, ghcr.io, quay.io)
- Installing K3s (downloads installation script and binaries)
- Installing system packages via apt

**Ports:**
- HTTP (80) and HTTPS (443) for downloads
- SSH (22) for git operations (optional)

### Intra-Host Communication

**Required Ports Between Infrastructure Hosts:**

| Service | Port | Protocol | Purpose |
|---------|------|----------|---------|
| SSH | 22 | TCP | Remote management |
| OpenBAO | 8200 | TCP | Secrets management API |
| PowerDNS API | 8081 | TCP | DNS management |
| PowerDNS | 53 | TCP/UDP | DNS queries |
| Zot Registry | 5000 | TCP | Container image registry |
| K3s API | 6443 | TCP | Kubernetes API server |
| K3s Cluster | 10250 | TCP | Kubelet API |
| K3s Flannel | 8472 | UDP | VXLAN overlay network |

**Firewall Notes:**
- If you have a firewall enabled, ensure these ports are open
- Foundry does NOT currently manage firewall rules
- You may need to configure firewall rules manually

## Access Requirements

### Initial Access

**For First-Time Setup:**
- SSH username and password for each infrastructure host
- Non-root user account (Foundry will configure sudo automatically)
- Root password (needed if sudo is not yet installed)
- Password authentication must be enabled on SSH

**After Initial Setup:**
- Foundry generates SSH key pairs automatically
- SSH keys are installed on infrastructure hosts
- SSH keys are stored in `~/.foundry/keys/` initially
- SSH keys are migrated to OpenBAO after it's installed
- Password authentication can be disabled if desired

### SSH Key Lifecycle

Foundry manages SSH keys through multiple stages:

1. **Initial Setup** (`foundry host add`):
   - Generates SSH key pair
   - Installs public key on host via password authentication
   - Stores key pair in `~/.foundry/keys/<hostname>/`

2. **Re-Installation** (`foundry host sync-keys`):
   - Re-installs existing keys after VM reset/reimaging
   - Uses password authentication to bootstrap key-based auth
   - Useful for recovery scenarios

3. **Migration to OpenBAO** (automatic after OpenBAO installation):
   - Keys are migrated from filesystem to OpenBAO
   - Future operations retrieve keys from OpenBAO
   - Provides secure, centralized key storage

## Storage Components

### Longhorn (Recommended)

Foundry uses Longhorn for distributed block storage by default.

**Requirements:**
- At least 1 worker node with available disk space
- `/var/lib/longhorn` directory (or configured data path)

**Features:**
- No external storage required
- Automatic replication across nodes
- Snapshot and backup support

### SeaweedFS (S3-Compatible Storage)

SeaweedFS provides S3-compatible object storage for Loki and Velero.

**Requirements:**
- Runs on Longhorn PVCs
- No external dependencies

**Notes:**
- Automatically installed as part of the stack
- Used by Loki for log storage and Velero for backups

## Building Foundry from Source

### Install Go

**Debian/Ubuntu:**
```bash
# Download and install Go 1.23
wget https://go.dev/dl/go1.23.0.linux-amd64.tar.gz
sudo rm -rf /usr/local/go
sudo tar -C /usr/local -xzf go1.23.0.linux-amd64.tar.gz

# Add to PATH
echo 'export PATH=$PATH:/usr/local/go/bin' >> ~/.bashrc
source ~/.bashrc

# Verify
go version
```

### Clone and Build Foundry

```bash
# Clone the repository
git clone https://github.com/catalystcommunity/foundry.git
cd foundry/v1

# Build the CLI
go build -o foundry ./cmd/foundry

# Optional: Move to PATH
sudo mv foundry /usr/local/bin/

# Verify
foundry --version
```

## Host Ownership Model

### What Foundry Manages

Foundry assumes **full management** of infrastructure hosts. This means:

- **sudo Configuration:** Foundry installs and configures sudo automatically
- **Container Runtime:** Foundry installs containerd/nerdctl if not present
- **System Tools:** Foundry installs curl, git, vim, htop, iptables
- **SSH Keys:** Foundry generates SSH keys and manages access
- **DNS Configuration:** Foundry may configure DNS settings to use PowerDNS
- **Systemd Services:** Foundry creates and manages systemd units for components
- **Container Workloads:** Foundry manages containers for OpenBAO, PowerDNS, Zot, etc.
- **Kubernetes Cluster:** Foundry installs and configures K3s
- **Networking:** Foundry configures Kubernetes networking (CNI, kube-vip)

### What Foundry Does NOT Manage

Foundry does NOT modify:

- **Base OS Installation:** You must install Debian/Ubuntu yourself
- **Network Routes:** IP routing remains unchanged
- **Firewall Rules:** Host firewalls are not configured (manual setup required)
- **User Accounts:** Does not create or modify users (except SSH key installation)

### Additional Software

Users can install additional software on infrastructure hosts, but it must:

- Not conflict with Foundry-managed services
- Not use ports required by Foundry components
- Fit within Foundry's operational expectations

**Example Compatible Software:**
- Monitoring agents (Prometheus node_exporter, etc.)
- Backup tools
- Custom logging agents

**Example Incompatible Software:**
- Another DNS server on port 53
- Another container registry on port 5000
- Services that modify systemd configuration for Foundry-managed units

## Validation

Before proceeding with Foundry installation, verify your environment:

### Pre-Flight Checklist

**Required (Manual Setup):**
- [ ] Hosts meet minimum hardware requirements
- [ ] Debian 11/12 or Ubuntu 22.04/24.04 installed with systemd
- [ ] SSH server is running and accessible
- [ ] Password authentication is enabled for SSH
- [ ] Non-root user account created
- [ ] User password available
- [ ] Hosts have static IP or DHCP reservation
- [ ] Available VIP for Kubernetes cluster
- [ ] Upstream DNS servers are accessible
- [ ] Outbound internet access works
- [ ] Required ports are open (or firewall is disabled)
- [ ] Go 1.23+ is installed on management machine (for building Foundry)

**Automatic (Foundry Handles):**
- [ ] sudo configured (Foundry will prompt for root password if needed)
- [ ] Container runtime installed (Foundry installs containerd + nerdctl)
- [ ] Common tools installed (Foundry installs curl, git, vim, htop)
- [ ] SSH keys generated and installed (Foundry handles during `host add`)

### Quick Validation Script

Run this on each infrastructure host to check basic requirements:

```bash
#!/bin/bash
echo "=== Foundry Prerequisites Check ==="
echo
echo "This script checks ONLY the manual prerequisites."
echo "Foundry will handle sudo, container runtime, and tools automatically."
echo

echo "OS: $(cat /etc/os-release | grep PRETTY_NAME | cut -d'"' -f2)"
echo "Kernel: $(uname -r)"
echo "Systemd: $(systemctl --version | head -1)"
echo

echo -n "SSH Server: "
systemctl is-active sshd || systemctl is-active ssh || echo "NOT RUNNING - REQUIRED"
echo

echo "Network:"
ip addr show | grep "inet " | grep -v "127.0.0.1"
echo

echo "DNS Servers:"
cat /etc/resolv.conf | grep nameserver
echo

echo "=== Automatic Setup (Foundry Handles These) ==="
echo

echo -n "Container Runtime: "
if command -v docker &> /dev/null; then
    echo "Docker found (Foundry will use it)"
elif command -v nerdctl &> /dev/null; then
    echo "nerdctl found (Foundry will use it)"
else
    echo "Not found (Foundry will install containerd + nerdctl)"
fi

echo -n "sudo Access: "
if sudo -n true 2>/dev/null; then
    echo "Configured (Foundry will use it)"
elif command -v sudo &> /dev/null; then
    echo "Installed but requires password (Foundry will configure)"
else
    echo "Not installed (Foundry will install and configure)"
fi

echo
echo "=== End Prerequisites Check ==="
```

## Getting Help

If you encounter issues with prerequisites:

1. Review the [Getting Started Guide](getting-started.md)
2. Check the [Configuration Guide](configuration.md)
3. Review validation test plan in `phase2-validation.md` (project root)
4. Join the [Catalyst Community Discord](https://discord.gg/sfNb9xRjPn)
5. Open an issue on [GitHub](https://github.com/catalystcommunity/foundry/issues)

## Next Steps

Once prerequisites are met:

1. **Build Foundry** - Follow the build instructions above
2. **Initialize Configuration** - Run `foundry config init`
3. **Add Infrastructure Hosts** - Run `foundry host add`
4. **Configure Hosts** - Run `foundry host configure <hostname>` (handles sudo, container runtime, etc.)
5. **Plan Network** - Run `foundry network plan`
6. **Validate Network** - Run `foundry network validate`
7. **Install Components** - Start with `foundry component install openbao`

See [Getting Started](getting-started.md) for detailed instructions.
