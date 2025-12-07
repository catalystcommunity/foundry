# Configuration Guide

This guide explains Foundry's configuration file format and how to manage configurations.

## Configuration File Format

Foundry uses YAML configuration files stored in `~/.foundry/`. Here's a complete example:

```yaml
version: v1

cluster:
  name: my-cluster
  domain: example.local
  nodes:
    - hostname: node1
      role: control-plane
    - hostname: node2
      role: worker
    - hostname: node3
      role: worker

components:
  openbao:
    version: "2.0.0"
    hosts:
      - node1
    config:
      address: "https://vault.example.local"

  k3s:
    version: "v1.28.5+k3s1"
    hosts:
      - node1
      - node2
      - node3
    config:
      cluster_init: true
      tls_san:
        - "k8s.example.local"

  zot:
    version: "v2.0.0"
    hosts:
      - node1
    config:
      storage_path: "/var/lib/zot"
      http_port: 5000

observability:
  prometheus:
    version: "v2.45.0"
    retention: "30d"
  loki:
    version: "v2.9.0"
    retention: "7d"
  grafana:
    version: "v10.0.0"

storage:
  backend: longhorn  # Options: local-path, longhorn, nfs
  longhorn:
    replica_count: 3
    data_path: /var/lib/longhorn

# S3-compatible object storage (Garage)
garage:
  enabled: true
  replicas: 3
  buckets:
    - velero
    - loki
```

## Configuration Schema

### Top-Level Fields

- `version` (required): Configuration version (currently `v1`)
- `cluster` (required): Cluster configuration
- `components` (required): Component installations
- `observability` (optional): Observability stack configuration
- `storage` (optional): Storage configuration

### Cluster Configuration

```yaml
cluster:
  name: string        # Cluster name (required)
  domain: string      # Base domain (required)
  nodes: []           # Node list (required)
```

### Node Configuration

```yaml
nodes:
  - hostname: string  # Node hostname (required)
    role: string      # Node role: control-plane, worker (required)
```

Valid roles:
- `control-plane`: Kubernetes control plane node
- `worker`: Kubernetes worker node

### Component Configuration

Components are defined as a map where the key is the component name:

```yaml
components:
  component-name:
    version: string              # Component version (optional)
    hosts: []string              # Target hosts (optional)
    config: map[string]any       # Component-specific config (optional)
```

Any additional fields in the component are treated as configuration.

## Secret References

Foundry supports secret references in configuration values using the format:

```yaml
${secret:path/to/secret:key}
```

Example:

```yaml
components:
  database:
    config:
      password: "${secret:database/main:password}"
      api_key: "${secret:database/main:api_key}"
```

**Important Notes:**
- Secret references are validated for syntax during `foundry config validate`
- Actual secret resolution happens during deployment when instance context is known
- See the [Secrets Guide](./secrets.md) for more details on secret management

## Multi-Config Support

Foundry supports multiple configuration files:

```bash
# Create multiple configs
foundry config init --name production
foundry config init --name staging
foundry config init --name development

# List all configs
foundry config list

# Use a specific config
foundry --config ~/.foundry/production.yaml config show

# Or set environment variable
export FOUNDRY_CONFIG=~/.foundry/production.yaml
foundry config show
```

## Configuration Commands

### Initialize a New Config

```bash
# Interactive mode
foundry config init

# With name
foundry config init --name production

# Non-interactive with defaults
foundry config init --name dev --non-interactive

# Force overwrite existing
foundry config init --name prod --force
```

### Validate Configuration

```bash
# Validate default config
foundry config validate

# Validate specific config
foundry config validate ~/.foundry/production.yaml

# Or use the --config flag
foundry --config ~/.foundry/production.yaml config validate
```

Validation checks:
- YAML syntax
- Required fields
- Valid enum values (e.g., node roles)
- Secret reference syntax
- Node references in components

### Show Configuration

```bash
# Show with secrets redacted (default)
foundry config show

# Show secret reference syntax (not values)
foundry config show --show-secret-refs
```

### List Configurations

```bash
foundry config list
```

Shows:
- All configuration files in `~/.foundry/`
- Which config is active (via `--config` flag or `FOUNDRY_CONFIG`)
- Default config if none specified

## Best Practices

### 1. Use Semantic Versioning

```yaml
components:
  openbao:
    version: "2.0.0"  # Use specific versions
```

### 2. Organize by Environment

```bash
~/.foundry/
├── production.yaml
├── staging.yaml
└── development.yaml
```

### 3. Never Store Secrets in Config

❌ **Wrong:**
```yaml
components:
  database:
    password: "actual-password-here"  # NEVER do this
```

✅ **Correct:**
```yaml
components:
  database:
    password: "${secret:database/main:password}"
```

### 4. Document Component Configs

Add comments to explain non-obvious configuration:

```yaml
components:
  k3s:
    version: "v1.28.5+k3s1"
    config:
      # Required for HA control plane
      cluster_init: true
      # Add all possible control plane IPs
      tls_san:
        - "k8s.example.local"
        - "192.168.1.10"
```

### 5. Validate Before Deployment

Always validate configuration before using it:

```bash
foundry config validate && foundry stack install
```

## Configuration Inheritance

Currently, Foundry does not support configuration inheritance or includes. Each configuration file must be self-contained.

This may be added in future versions.

## Migration

When migrating from manual setups, create a config that matches your existing infrastructure:

```bash
# 1. Create base config
foundry config init --name migration

# 2. Edit to match existing setup
vim ~/.foundry/migration.yaml

# 3. Validate
foundry config validate ~/.foundry/migration.yaml

# 4. Add existing hosts
foundry host add node1
foundry host add node2
```

## Troubleshooting

### "Config not found" Error

- Check file exists: `ls -la ~/.foundry/`
- Check file name is correct
- Use absolute path: `foundry config validate ~/.foundry/stack.yaml`

### "Invalid YAML" Error

- Check YAML syntax with: `yamllint ~/.foundry/stack.yaml`
- Ensure proper indentation (use spaces, not tabs)
- Ensure strings with special chars are quoted

### "Invalid secret reference" Error

- Check format: `${secret:path:key}`
- Ensure no spaces in the reference
- Path should use forward slashes: `database/main`

## Examples

See `test/fixtures/` directory for example configurations:

```bash
cd v1/test/fixtures/
ls -la *.yaml
```

## Next Steps

- Learn about [Secret Management](./secrets.md)
- Learn about [Host Management](./hosts.md)
- Review the [Getting Started Guide](./getting-started.md)
