# Secret References

Foundry can read a secret value from an environment variable or from OpenBAO.
Foundry resolves secret references when it installs a component.

## Reference Format

Use this format in a configuration value:

```yaml
${secret:path/to/secret:key}
```

The path can contain letters, numbers, slashes, underscores, and hyphens. The
key can contain letters, numbers, underscores, and hyphens.

For example:

```yaml
components:
  zot:
    config:
      docker_hub_password: "${secret:zot:docker_hub_password}"
```

## Resolution Order

The component installer uses this order:

1. It checks an environment variable.
2. It checks OpenBAO when OpenBAO is available.
3. It stops with an error if neither source has the value.

The environment variable name starts with `FOUNDRY_SECRET_`. Foundry converts
the path and key to uppercase. It replaces slashes, hyphens, and colons with
underscores.

For this reference:

```text
${secret:zot:docker_hub_password}
```

use this environment variable:

```bash
export FOUNDRY_SECRET_ZOT_DOCKER_HUB_PASSWORD="replace-this-value"
```

The component installer uses the `foundry-core` OpenBAO mount. It gets the
OpenBAO address from the stack configuration. It gets the token from this
file:

```text
~/.foundry/openbao-keys/<cluster-name>/keys.json
```

## Validation

Use this command to validate the configuration structure and secret reference
syntax:

```bash
foundry config validate
```

This command does not read secret values. The component installer reads the
values when it needs them.

## Display

By default, Foundry redacts secret references in command output:

```bash
foundry config show
```

Use this option to show the reference text. This option does not show the
secret value.

```bash
foundry config show --show-secret-refs
```

## Security

- Do not store secret values in the stack configuration.
- Do not commit secret values to the repository.
- Set environment variables only for the process that needs them.
- Protect the OpenBAO key file with file-system permissions.
- Rotate a secret after an unintended disclosure.

For pod secret injection, see [Pod Secrets](./pod-secrets.md).
