# Secrets Management Guide

Foundry provides a flexible secret management system with support for multiple sources and instance-scoped secrets.

## Overview

Foundry resolves secrets from multiple sources in order:
1. Environment variables
2. `~/.foundryvars` file
3. OpenBAO (planned for Phase 2)

Secrets are resolved at deployment time, not during configuration validation.

## Secret Reference Format

In configuration files, reference secrets using:

```yaml
${secret:path/to/secret:key}
```

**Components:**
- `path/to/secret`: The path to the secret (can have multiple levels)
- `key`: The specific key within that secret

**Examples:**
```yaml
${secret:database/main:password}
${secret:api/github:token}
${secret:ssl/wildcard:cert}
```

## Instance Context

Secrets are scoped to instances, allowing the same configuration to be used for multiple deployments (e.g., production, staging).

### How It Works

When Foundry resolves a secret:
1. Config contains: `${secret:database/main:password}`
2. Instance context is provided: `myapp-prod`
3. Foundry looks for: `myapp-prod/database/main:password`

This allows:
```bash
# Same config, different secrets
foundry deploy myapp --instance myapp-prod
foundry deploy myapp --instance myapp-staging
```

## Secret Sources

### 1. Environment Variables

Set secrets as environment variables:

```bash
# Format: FOUNDRY_SECRET_<instance>_<path>_<key>
# Slashes, hyphens, and colons become underscores

export FOUNDRY_SECRET_myapp_prod_database_main_password="prod-secret-123"
export FOUNDRY_SECRET_myapp_staging_database_main_password="staging-secret-456"
```

**Example:**
- Instance: `myapp-prod`
- Path: `database/main`
- Key: `password`
- Env var: `FOUNDRY_SECRET_myapp_prod_database_main_password`

### 2. .foundryvars File

Create `~/.foundryvars` for local development:

```bash
# Format: instance/path:key=value
# One secret per line
# Comments start with #

# Production secrets
myapp-prod/database/main:password=prod-secret-123
myapp-prod/api/github:token=ghp_prod_token

# Staging secrets
myapp-stable/database/main:password=staging-secret-456
myapp-stable/api/github:token=ghp_staging_token

# Foundry core secrets
foundry-core/openbao:token=root-token
```

**Security:**
```bash
# Set restrictive permissions
chmod 600 ~/.foundryvars

# Never commit to git
echo ".foundryvars" >> ~/.gitignore
```

### 3. OpenBAO (Phase 2)

OpenBAO integration is planned for Phase 2. It will follow the same instance-scoped path structure:

```
openbao kv put myapp-prod/database/main password=secret123
openbao kv put myapp-staging/database/main password=different-secret
```

## Resolution Order

Foundry tries each source in order and returns the first match:

1. **Environment Variables** - Highest priority
2. **~/.foundryvars** - Development/local use
3. **OpenBAO** - Production use (Phase 2)

If all sources fail, deployment fails with a clear error message.

## Configuration Validation

During `foundry config validate`, Foundry:
- ✅ Validates secret reference **syntax**
- ✅ Ensures references are well-formed
- ❌ Does **NOT** resolve actual secrets

This allows you to validate configs without having secrets available.

```bash
# This validates syntax only
foundry config validate

# Output:
# ✓ Config structure valid
# ✓ Secret references syntax valid
# ✓ Configuration is valid
```

## Secret Resolution

Secrets are resolved during deployment when instance context is known:

```bash
# During deployment, instance context is provided
foundry deploy myapp --instance myapp-prod

# Foundry resolves:
# ${secret:database/main:password}
# → myapp-prod/database/main:password
# → "actual-secret-value"
```

## Best Practices

### 1. Use Descriptive Paths

Organize secrets with clear paths:

```
✅ Good:
  database/postgres:password
  database/postgres:username
  api/github:token
  api/slack:webhook

❌ Bad:
  db:pass
  secret1:value
  prod:key
```

### 2. Never Commit Secrets

```bash
# Add to .gitignore
echo ".foundryvars" >> ~/.gitignore
echo "*.env" >> ~/.gitignore

# Check before committing
git diff
grep -r "password" .
```

### 3. Use Instance Scoping

Separate secrets by instance:

```bash
# ~/.foundryvars
myapp-prod/database/main:password=prod-secret
myapp-staging/database/main:password=staging-secret
myapp-dev/database/main:password=dev-secret
```

### 4. Rotate Secrets Regularly

```bash
# Update secret in all sources
vi ~/.foundryvars
export FOUNDRY_SECRET_myapp_prod_api_token="new-token"

# Redeploy to apply
foundry deploy myapp --instance myapp-prod
```

### 5. Use Environment Variables in CI/CD

For CI/CD pipelines, use environment variables:

```yaml
# .github/workflows/deploy.yml
env:
  FOUNDRY_SECRET_myapp_prod_database_password: ${{ secrets.DB_PASSWORD }}
  FOUNDRY_SECRET_myapp_prod_api_token: ${{ secrets.API_TOKEN }}
```

## Development Workflow

### Local Development

Use `~/.foundryvars` for local development:

```bash
# 1. Create secrets file
cat > ~/.foundryvars <<EOF
myapp-dev/database/main:password=dev-password
myapp-dev/api/key:value=dev-api-key
EOF

chmod 600 ~/.foundryvars

# 2. Test secret resolution
foundry deploy myapp --instance myapp-dev --dry-run
```

### CI/CD

Use environment variables in CI/CD:

```bash
# In CI/CD environment
export FOUNDRY_SECRET_myapp_prod_database_password="${DB_PASSWORD}"
export FOUNDRY_SECRET_myapp_prod_api_token="${API_TOKEN}"

foundry deploy myapp --instance myapp-prod
```

### Production

Use OpenBAO (Phase 2):

```bash
# Store in OpenBAO
openbao kv put myapp-prod/database/main password="${DB_PASSWORD}"

# Foundry automatically retrieves from OpenBAO
foundry deploy myapp --instance myapp-prod
```

## Examples

### Database Configuration

```yaml
# config.yaml
components:
  postgres:
    config:
      host: "db.example.com"
      port: 5432
      username: "${secret:database/postgres:username}"
      password: "${secret:database/postgres:password}"
      database: "${secret:database/postgres:dbname}"
```

```bash
# ~/.foundryvars
myapp-prod/database/postgres:username=dbuser
myapp-prod/database/postgres:password=secure-password
myapp-prod/database/postgres:dbname=myapp_production
```

### API Keys

```yaml
# config.yaml
components:
  api-service:
    config:
      github_token: "${secret:api/github:token}"
      slack_webhook: "${secret:api/slack:webhook}"
      stripe_key: "${secret:api/stripe:secret_key}"
```

```bash
# ~/.foundryvars
myapp-prod/api/github:token=ghp_xxxxxxxxxxxxx
myapp-prod/api/slack:webhook=https://hooks.slack.com/xxxxx
myapp-prod/api/stripe:secret_key=sk_live_xxxxxx
```

### TLS Certificates

```yaml
# config.yaml
components:
  ingress:
    config:
      tls_cert: "${secret:tls/wildcard:cert}"
      tls_key: "${secret:tls/wildcard:key}"
```

## Troubleshooting

### "Secret not found" Error

Check resolution order:

```bash
# 1. Check environment variables
env | grep FOUNDRY_SECRET

# 2. Check .foundryvars
cat ~/.foundryvars | grep "path:key"

# 3. Verify instance context matches
# If deploying with --instance myapp-prod
# Secret must be: myapp-prod/path:key
```

### Wrong Secret Value

Ensure instance context is correct:

```bash
# Check which instance you're deploying
foundry deploy myapp --instance myapp-prod

# Ensure secrets match instance
# myapp-prod/database:password  ← Correct
# myapp-staging/database:password  ← Wrong instance
```

### Permission Denied on .foundryvars

```bash
# Fix permissions
chmod 600 ~/.foundryvars

# Verify
ls -la ~/.foundryvars
# Should show: -rw------- (600)
```

## Security Considerations

1. **Never commit secrets to git**
2. **Use restrictive permissions** (`chmod 600` for `.foundryvars`)
3. **Rotate secrets regularly**
4. **Use different secrets per environment**
5. **Use OpenBAO for production** (when available)
6. **Audit secret access**

## Future Enhancements

Phase 2 will add:
- ✅ OpenBAO integration
- ✅ Dynamic secret rotation
- ✅ Secret encryption at rest
- ✅ Audit logging for secret access
- ✅ Role-based access control

## Next Steps

- Read the [Configuration Guide](./configuration.md)
- Read the [Getting Started Guide](./getting-started.md)
- Review example configs in `test/fixtures/`
