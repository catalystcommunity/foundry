# Host Management Guide

This guide covers managing infrastructure hosts with Foundry, including adding hosts, SSH key management, and configuration.

## Overview

Foundry manages infrastructure hosts through:
- **Host Registry**: Stores host information (hostname, address, user, etc.)
- **SSH Key Management**: Automatic key generation and installation
- **Configuration**: Basic host setup and preparation

## Adding Hosts

### Interactive Mode

The recommended way to add hosts:

```bash
foundry host add
```

You'll be prompted for:
1. **Hostname**: Friendly name for the host (e.g., `node1`, `db-server`)
2. **Address**: IP address or FQDN (e.g., `192.168.1.100`, `server.example.com`)
3. **Port**: SSH port (default: 22)
4. **User**: SSH username (e.g., `ubuntu`, `admin`)
5. **Password**: SSH password for initial authentication

### What Happens

When you add a host, Foundry:
1. ✓ Tests SSH connection with password
2. ✓ Generates an Ed25519 SSH key pair
3. ✓ Installs public key on the host (`~/.ssh/authorized_keys`)
4. ✓ Stores private key in memory (for Phase 1)
5. ✓ Adds host to the registry

```bash
$ foundry host add

Hostname: web-server
Address (IP or FQDN): 192.168.1.100
SSH User: ubuntu
Password (warning: password will be visible): ********

Testing SSH connection to ubuntu@192.168.1.100:22...
✓ SSH connection successful
Generating SSH key pair...
✓ SSH key pair generated
Installing public key on host...
✓ Public key installed

✓ Host web-server added successfully
  Address: 192.168.1.100:22
  User: ubuntu
  SSH Key: configured
```

### Non-Interactive Mode

For automation or scripts:

```bash
foundry host add web-server \
  --address 192.168.1.100 \
  --port 22 \
  --user ubuntu \
  --password mypassword \
  --non-interactive
```

**⚠️ Warning**: Password will be visible in shell history. Use with caution.

## Listing Hosts

### Basic List

```bash
foundry host list
```

Output:
```
HOSTNAME     ADDRESS          USER
--------     -------          ----
node1        192.168.1.100    ubuntu
node2        192.168.1.101    ubuntu
web-server   10.0.0.50        admin
```

### Detailed List

```bash
foundry host list --verbose
```

Output:
```
HOSTNAME     ADDRESS          PORT   USER     SSH KEY
--------     -------          ----   ----     -------
node1        192.168.1.100    22     ubuntu   configured
node2        192.168.1.101    22     ubuntu   configured
web-server   10.0.0.50        2222   admin    configured
```

### Short Alias

```bash
foundry host ls
```

## Configuring Hosts

After adding a host, configure it with basic setup:

```bash
foundry host configure <hostname>
```

### What It Does

The configure command:
1. Connects to the host using the stored SSH key
2. Updates package lists (`apt-get update`)
3. Installs common tools (curl, git, vim, htop)
4. Configures time synchronization (NTP)

```bash
$ foundry host configure web-server

Connecting to web-server...
✓ Connected

Running configuration steps on web-server...

[1/3] Updating package lists...
  ✓ Update package lists complete
[2/3] Installing common tools (curl, git, vim, htop)...
  ✓ Install common tools complete
[3/3] Configuring time synchronization...
  ✓ Configure time synchronization complete

✓ Configuration complete for web-server
```

### Configuration Options

Skip specific steps:

```bash
# Skip package updates
foundry host configure web-server --skip-update

# Skip tool installation
foundry host configure web-server --skip-tools

# Skip everything except time sync
foundry host configure web-server --skip-update --skip-tools
```

## SSH Key Management

### Key Generation

Foundry uses **Ed25519** keys because they are:
- More secure than RSA with smaller key size
- Faster to generate and use
- More resistant to side-channel attacks

Keys are automatically generated when adding a host.

### Key Storage

**Phase 1**: Keys are stored in-memory
- Lost when Foundry exits
- Suitable for development/testing
- Not persistent across sessions

**Phase 2** (planned): Keys will be stored in OpenBAO
- Persistent storage
- Encrypted at rest
- Access-controlled
- Audit logging

### Manual Key Installation

If you need to manually install a key:

```bash
# On the remote host
mkdir -p ~/.ssh
chmod 700 ~/.ssh
echo "ssh-ed25519 AAAA...your-public-key..." >> ~/.ssh/authorized_keys
chmod 600 ~/.ssh/authorized_keys
```

## Host Registry

### Registry Behavior

**Phase 1**: In-memory registry
- Hosts are stored in RAM
- Not persistent across Foundry sessions
- Suitable for testing and development

**Phase 2** (planned): Persistent registry
- Stored in configuration or OpenBAO
- Survives across sessions
- Shareable across team members

### Registry Operations

Check if a host exists:
```bash
foundry host list | grep hostname
```

Remove a host (Phase 2):
```bash
foundry host remove <hostname>
```

Update host information (Phase 2):
```bash
foundry host update <hostname> --address new-address
```

## Best Practices

### 1. Use Descriptive Hostnames

✅ Good:
```
k8s-control-1
k8s-worker-1
db-primary
cache-redis-1
```

❌ Bad:
```
server1
host2
box
```

### 2. Document Your Hosts

Keep track of hosts in a separate document:

```markdown
# Infrastructure Hosts

## Kubernetes Cluster
- k8s-control-1: 192.168.1.100 - Control plane
- k8s-worker-1: 192.168.1.101 - Worker node
- k8s-worker-2: 192.168.1.102 - Worker node

## Databases
- db-primary: 192.168.1.200 - PostgreSQL primary
- db-replica: 192.168.1.201 - PostgreSQL replica
```

### 3. Use SSH Config for Complex Setups

For jump hosts or complex SSH setups, use `~/.ssh/config`:

```
# ~/.ssh/config
Host k8s-control-1
    HostName 192.168.1.100
    User ubuntu
    ProxyJump bastion

Host bastion
    HostName bastion.example.com
    User admin
```

Then add to Foundry:
```bash
foundry host add k8s-control-1 --address k8s-control-1
```

### 4. Test Connectivity First

Before adding a host to Foundry:

```bash
# Test network connectivity
ping 192.168.1.100

# Test SSH port
nc -zv 192.168.1.100 22

# Test SSH login
ssh ubuntu@192.168.1.100
```

### 5. Keep Passwords Secure

Never store passwords in:
- Shell history
- Scripts committed to git
- Shared documents

Use password managers or prompt for passwords interactively.

## Troubleshooting

### "Connection refused"

Check if SSH is running:
```bash
# On the remote host
sudo systemctl status sshd
sudo systemctl start sshd
```

Check firewall:
```bash
# Allow SSH through firewall
sudo ufw allow 22/tcp
```

### "Permission denied (publickey)"

Check key installation:
```bash
# On remote host
cat ~/.ssh/authorized_keys
# Should contain the public key

# Check permissions
ls -la ~/.ssh/
# .ssh should be 700
# authorized_keys should be 600
```

Re-add the host if needed:
```bash
# Remove old entry from authorized_keys on remote host
# Then add again with Foundry
foundry host add <hostname>
```

### "Host key verification failed"

Clear old host key:
```bash
ssh-keygen -R <hostname>
ssh-keygen -R <ip-address>
```

### "Connection timeout"

Check network:
```bash
# Test connectivity
ping <host>

# Test SSH port
nc -zv <host> 22

# Check if host is on VPN/private network
```

### "apt-get: command not found"

The configure command assumes Debian/Ubuntu. For other distributions:
- RHEL/CentOS: Modify commands to use `yum` or `dnf`
- Alpine: Modify commands to use `apk`

Phase 2 will add distribution detection and appropriate package managers.

## Advanced Usage

### Custom SSH Ports

```bash
foundry host add custom-server \
  --address 192.168.1.100 \
  --port 2222 \
  --user admin \
  --password mypassword \
  --non-interactive
```

### Jump Hosts / Bastion Hosts

Phase 1 doesn't directly support jump hosts. Use SSH config as a workaround:

```
# ~/.ssh/config
Host internal-server
    HostName 10.0.0.100
    User ubuntu
    ProxyJump bastion.example.com
```

Phase 2 will add native jump host support.

### Using Existing Keys

Phase 1 always generates new keys. Phase 2 will support:
- Using existing SSH keys
- Importing keys from files
- Sharing keys across hosts

## Integration with Configuration

Hosts added to the registry can be referenced in configurations:

```yaml
# config.yaml
cluster:
  name: my-cluster
  nodes:
    - hostname: k8s-control-1  # Must match host registry
      role: control-plane
    - hostname: k8s-worker-1   # Must match host registry
      role: worker
```

Foundry will use the stored SSH keys to connect to these hosts during deployment.

## Security Considerations

1. **SSH Key Security**
   - Ed25519 keys are secure and modern
   - Keys are not password-protected (for automation)
   - Phase 2 will add key encryption

2. **Password Security**
   - Passwords are used only once (for initial setup)
   - Not stored by Foundry
   - Consider using SSH keys from the start if possible

3. **Host Access**
   - Limit SSH access to specific IPs
   - Use firewall rules
   - Disable password authentication after key setup
   - Enable fail2ban for brute force protection

4. **Audit Logging**
   - Phase 2 will add audit logs for host access
   - Track who accessed which hosts
   - Monitor for suspicious activity

## Examples

### Add Multiple Hosts

```bash
#!/bin/bash
# add-k8s-cluster.sh

HOSTS=(
  "k8s-control-1:192.168.1.100"
  "k8s-worker-1:192.168.1.101"
  "k8s-worker-2:192.168.1.102"
)

for host in "${HOSTS[@]}"; do
  IFS=: read -r name ip <<< "$host"

  foundry host add "$name" \
    --address "$ip" \
    --user ubuntu \
    --password "$SSH_PASSWORD" \
    --non-interactive
done
```

### Configure All Hosts

```bash
# Get list of hosts and configure each
foundry host list --verbose | tail -n +3 | awk '{print $1}' | while read hostname; do
  echo "Configuring $hostname..."
  foundry host configure "$hostname"
done
```

### Verify All Hosts

```bash
#!/bin/bash
# verify-hosts.sh

foundry host list --verbose | tail -n +3 | awk '{print $1}' | while read hostname; do
  echo "Testing $hostname..."

  if ssh -o ConnectTimeout=5 -o BatchMode=yes "$hostname" "echo 'OK'" &>/dev/null; then
    echo "  ✓ $hostname is reachable"
  else
    echo "  ✗ $hostname is NOT reachable"
  fi
done
```

## Next Steps

- Read the [Getting Started Guide](./getting-started.md)
- Read the [Configuration Guide](./configuration.md)
- Read the [Secrets Guide](./secrets.md)
- Review the integration tests in `test/integration/`
