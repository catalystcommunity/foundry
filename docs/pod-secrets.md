# Injecting OpenBao Secrets into Pods

This guide explains how to inject secrets from OpenBao into Kubernetes pods using the OpenBao agent injector.

## Prerequisites

The `openbao-injector` component must be installed:

```bash
foundry component install openbao-injector
```

This deploys the OpenBao agent injector and registers a `MutatingWebhookConfiguration` that intercepts pod creation. Pods annotated with `vault.hashicorp.com/agent-inject: "true"` automatically receive a sidecar that mounts secrets from OpenBao before the main container starts.

## How It Works

1. You annotate a pod/deployment/cronjob with the secrets it needs
2. The injector sidecar fetches those secrets from OpenBao at pod startup
3. Secrets are written as files to `/vault/secrets/`
4. Your container sources those files to load them as environment variables

## OpenBao Setup

### Enable Kubernetes Auth

The injector authenticates pods against OpenBao using the Kubernetes service account JWT. You have two options:

#### Option 1: Automatic (Recommended)

Add `configure_k8s_auth: true` to your stack config when installing the injector:

```yaml
components:
  openbao-injector:
    configure_k8s_auth: true
    k8s_auth_roles:
      - role_name: "my-app"
        service_account_name: "default"
        service_account_namespace: "my-namespace"
        policies: "my-app"
        ttl: "1h"
```

This automatically:
- Creates a `vault-reviewer` ServiceAccount in `kube-system`
- Binds it to `system:auth-delegator` ClusterRole
- Enables Kubernetes auth in OpenBao
- Configures it with `disable_iss_validation=true` (required for in-cluster JWT issuer)
- Creates the roles you specify

#### Option 2: Manual

```bash
vault auth enable kubernetes

vault write auth/kubernetes/config \
  kubernetes_host="https://$(kubectl get svc kubernetes -o jsonpath='{.spec.clusterIP}'):443" \
  disable_iss_validation=true
```

> **Note**: `kubernetes_host` must use the ClusterIP (e.g., `https://10.43.0.1:443`), not a Tailscale hostname. The injector runs on Kubernetes nodes where the ClusterIP is reachable, but Tailscale addresses may not be routable from the host node.

### Create a Policy

Define what secrets a pod is allowed to read:

```bash
vault policy write my-app - <<EOF
path "secret/data/apps/my-app" {
  capabilities = ["read"]
}
EOF
```

### Create a Role

Bind the policy to a Kubernetes service account:

```bash
vault write auth/kubernetes/role/my-app \
  bound_service_account_names=default \
  bound_service_account_namespaces=my-namespace \
  policies=my-app \
  ttl=1h
```

## Annotating Pods

Add annotations to your pod spec to request secret injection. The injector creates one file per secret at `/vault/secrets/<name>`.

### Basic Example

```yaml
spec:
  template:
    metadata:
      annotations:
        vault.hashicorp.com/agent-inject: "true"
        vault.hashicorp.com/role: "my-app"

        # Inject secret/data/apps/my-app as /vault/secrets/my-app
        vault.hashicorp.com/agent-inject-secret-my-app: "secret/data/apps/my-app"

        # Template the file as shell exports so the container can source it
        vault.hashicorp.com/agent-inject-template-my-app: |
          {{- with secret "secret/data/apps/my-app" -}}
          export DB_PASSWORD="{{ .Data.data.password }}"
          export API_KEY="{{ .Data.data.api_key }}"
          {{- end }}
```

### Container Entrypoint

Because the secrets are shell files (not environment variables), the container must source them before starting:

```yaml
containers:
  - name: my-app
    command: ["/bin/sh", "-c"]
    args: [". /vault/secrets/my-app && exec my-binary"]
```

> **Note**: Use `.` (dot) not `source` — slim/alpine images use `dash` as `/bin/sh` which does not support `source`.

## Helm Chart Pattern

For a Helm chart with multiple secrets (e.g. reddit-watcher):

```yaml
# In your cronjob/deployment template
metadata:
  annotations:
    vault.hashicorp.com/agent-inject: "true"
    vault.hashicorp.com/role: "{{ .Values.vault.role }}"

    vault.hashicorp.com/agent-inject-secret-db: "{{ .Values.vault.dbPath }}"
    vault.hashicorp.com/agent-inject-template-db: |
      {{`{{- with secret "`}}{{ .Values.vault.dbPath }}{{`" -}}`}}
      {{`export POSTGRES_URL="{{ .Data.data.postgres_url }}"`}}
      {{`{{- end }}`}}

    vault.hashicorp.com/agent-inject-secret-api: "{{ .Values.vault.apiPath }}"
    vault.hashicorp.com/agent-inject-template-api: |
      {{`{{- with secret "`}}{{ .Values.vault.apiPath }}{{`" -}}`}}
      {{`export API_KEY="{{ .Data.data.key }}"`}}
      {{`{{- end }}`}}

spec:
  containers:
    - name: my-app
      command: ["/bin/sh", "-c"]
      args: [". /vault/secrets/db && . /vault/secrets/api && exec python -m main"]
```

## Storing Secrets in OpenBao

```bash
# Store secrets (run from your local machine with VAULT_ADDR set)
vault kv put secret/apps/my-app \
  password="my-db-password" \
  api_key="my-api-key"

# Add a key to an existing secret without overwriting others
vault kv patch secret/apps/my-app new_key="new-value"

# Using 1Password to inject the value at write time
op run -- vault kv patch secret/apps/my-app \
  postgres_url="op://pedro/POSTGRES_URL/credential"
```

## Troubleshooting

### `/vault/secrets/<name>: No such file`

The injector sidecar didn't run. Check:

1. Is `openbao-injector` installed?
   ```bash
   foundry component status openbao-injector
   kubectl get mutatingwebhookconfigurations | grep openbao
   ```

2. Does the pod have `vault.hashicorp.com/agent-inject: "true"` annotation?

3. Is the pod in a namespace the webhook targets? By default it targets all namespaces.

### `permission denied` fetching secrets

The Kubernetes auth role doesn't bind to this pod's service account or namespace:

```bash
# Verify the role bindings
vault read auth/kubernetes/role/my-app
```

Ensure `bound_service_account_namespaces` includes the pod's namespace.

### `source: not found`

Using `source` instead of `.` in a `dash`-based image. Replace:
```sh
# Wrong (bash only)
source /vault/secrets/my-app

# Correct (POSIX sh / dash compatible)
. /vault/secrets/my-app
```
