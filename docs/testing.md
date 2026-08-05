# Testing

Use the repository test script for local validation. Run commands from the
repository root.

## Fast Validation

```bash
scripts/test-local.sh
```

This command performs these checks:

1. It builds all Go packages.
2. It runs `go vet` on all Go packages.
3. It checks Go formatting.
4. It runs short tests outside `v1/test/integration`.
5. It checks for conflict markers and unsafe debug output.

Use `PKG` to run one package pattern:

```bash
PKG=./internal/config/... scripts/test-local.sh
```

The build, vet, formatting, and hygiene checks still apply to the complete Go
module.

## Integration Validation

```bash
scripts/test-local.sh --integration
```

This mode runs all packages with the `integration` build tag. It does not use
the `-short` option. It uses a 60-minute package timeout. A container runtime
must be available.

## Kubernetes Manifest Validation

```bash
MANIFEST_DIR=/path/to/rendered/yaml scripts/test-local.sh --kind
```

This mode runs the fast checks. It then creates a temporary kind cluster and
applies the YAML files from `MANIFEST_DIR`. Install Go, Docker, kind, and
kubectl before you use this mode.

Use `--keep` to keep the cluster after the test:

```bash
scripts/test-local.sh --kind --keep
```

## Test Isolation

Go tests that write user files must use `t.TempDir()` or `t.Setenv()`. Do not
set one shared configuration directory for the complete test suite. Some tests
must control that directory independently.

For the complete option reference, see [the local test script](../scripts/README.md).
