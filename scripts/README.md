# scripts/test-local.sh

Local validation harness used to test the Tailscale integration PR stack (and
any component work) on a developer machine before pushing.

## Modes

| Command | What it does | Needs |
|---------|--------------|-------|
| `scripts/test-local.sh` | `go build ./...`, `go vet ./...`, and `go test` for every package **except** the Docker-gated `./test/integration/...`, plus hygiene checks (no conflict markers, no secret-leaking debug prints). | go |
| `scripts/test-local.sh --kind` | The above, then spins up a throwaway [kind](https://kind.sigs.k8s.io/) cluster and server-side dry-run-applies any rendered manifests in `$MANIFEST_DIR`. | go, kind, kubectl, Docker |
| `scripts/test-local.sh --kind --keep` | Same, but leaves the kind cluster running for manual poking. | ↑ |
| `scripts/test-local.sh --integration` | Runs the full suite **including** `./test/integration/...` (testcontainers). | go, Docker |

## Useful env vars

- `PKG=./internal/component/tailscale/...` — narrow the test run to one package.
- `CLUSTER_NAME=my-cluster` — name of the kind cluster (default `foundry-local-test`).
- `MANIFEST_DIR=/path/to/rendered/yaml` — directory of manifests for `--kind` to
  dry-run-apply against the live API server (used by the CRD/CoreDNS PRs).
- `FOUNDRY_CONFIG_DIR=$(mktemp -d)` — isolate config discovery from your real
  `~/.foundry` when running command tests directly.

## Why kind, not k3s, for local unit validation

Foundry installs against real hosts over SSH, so a full end-to-end install isn't
what a per-PR check needs. What each PR *can* verify locally is: the code builds,
unit tests pass, and — for PRs that generate Kubernetes manifests/CRDs — that
those manifests are accepted by a live API server. kind gives us that live API
server cheaply in Docker without provisioning nodes. Each PR's own "Testing"
section says which mode to run.
