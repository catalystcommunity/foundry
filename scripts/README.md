# Local Test Script

Use `scripts/test-local.sh` to validate Foundry changes on a development
computer. The default mode does not require a cluster.

## Test Modes

| Command | Action | Required tools |
|---------|--------|----------------|
| `scripts/test-local.sh` | Build the Go module, run `go vet`, check Go formatting, run the short tests, and run hygiene checks. This mode does not test `./test/integration/...`. | Go |
| `scripts/test-local.sh --kind` | Run the default checks. Then, create a temporary kind cluster and validate the files in `$MANIFEST_DIR`. | Go, kind, kubectl, and Docker |
| `scripts/test-local.sh --kind --keep` | Run the kind test and keep the cluster after the test. | Go, kind, kubectl, and Docker |
| `scripts/test-local.sh --integration` | Build the Go module and run all integration tests. This mode uses `-tags=integration`, does not use `-short`, and uses a 60-minute package timeout. | Go and a container runtime |

The default mode follows the test job in `.reactorcide/jobs/test.yaml`. The
integration package contains two types of tests. Some tests stop when Go uses
the `-short` option. Other tests require the `integration` build tag. The
`--integration` option enables both types.
The longer timeout lets the package create and remove multiple clusters.

## Environment Variables

- Set `PKG=./internal/component/tailscale/...` to test one package pattern.
  Build, vet, and formatting checks still apply to the complete Go module.
- Set `CLUSTER_NAME=my-cluster` to change the kind cluster name. The default
  name is `foundry-local-test`.
- Set `MANIFEST_DIR=/path/to/rendered/yaml` to validate generated YAML files in
  kind mode.
## Use of kind

Foundry installs software on remote hosts through SSH. A local test does not
need a complete Foundry installation. The kind test provides a Kubernetes API
server in Docker. The script can use this API server to validate generated
manifests.
