#!/usr/bin/env bash
#
# Local validation for Foundry.
#
# Each pull request identifies the required test mode. The default mode runs
# the build and the unit tests. Use --kind to create a temporary kind cluster.
# The script can apply generated manifests to the cluster API server.
#
# Usage:
#   scripts/test-local.sh                # build + unit tests (default, no cluster)
#   scripts/test-local.sh --kind         # test manifests in a kind cluster
#   scripts/test-local.sh --kind --keep  # keep the kind cluster after the test
#   scripts/test-local.sh --integration  # run the container integration tests
#   PKG=./internal/component/tailscale/... scripts/test-local.sh   # test one package
#
# The default mode does not run the integration suite in ./test/integration/....
# The integration suite requires a container runtime. Use --kind to test
# manifests. Use --integration to run the container integration tests.
#
# Requirements: Go. The optional modes also require the tools that they use.
set -euo pipefail

CLUSTER_NAME="${CLUSTER_NAME:-foundry-local-test}"
# The default mode tests all packages except the integration package.
PKG="${PKG:-}"
DO_KIND=0
DO_INTEGRATION=0
KEEP=0

for arg in "$@"; do
  case "$arg" in
    --kind) DO_KIND=1 ;;
    --integration) DO_INTEGRATION=1 ;;
    --keep) KEEP=1 ;;
    -h|--help) grep '^#' "$0" | sed 's/^# \{0,1\}//'; exit 0 ;;
    *) echo "unknown arg: $arg" >&2; exit 2 ;;
  esac
done

# Find the repository root and the Go module directory.
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
MODDIR="$ROOT/v1"

step() { printf '\n\033[1;34m==> %s\033[0m\n' "$1"; }
ok()   { printf '\033[1;32m✓ %s\033[0m\n' "$1"; }

# Select the test packages. PKG selects one package pattern. Integration mode
# selects all packages. Default mode excludes the integration package.
TEST_PKGS=()
if [[ -n "$PKG" ]]; then
  TEST_PKGS=("$PKG")
elif [[ "$DO_INTEGRATION" -eq 1 ]]; then
  TEST_PKGS=("./...")
else
  while IFS= read -r pkg; do
    TEST_PKGS+=("$pkg")
  done < <(cd "$MODDIR" && go list ./... | grep -v '/test/integration')
fi

step "go build"
( cd "$MODDIR" && go build ./... )
ok "build"

step "go vet"
( cd "$MODDIR" && go vet ./... )
ok "vet"

step "gofmt check"
if [[ -n "$(cd "$MODDIR" && gofmt -l .)" ]]; then
  echo "unformatted files found:" >&2
  cd "$MODDIR" && gofmt -l . >&2
  exit 1
fi
ok "gofmt"

# Integration mode does not use -short. It also uses -tags=integration and a
# 60-minute package timeout. The integration package creates multiple clusters.
# Omitting -short enables untagged integration tests. The build tag enables
# tagged component and stack integration tests. See docs/testing.md.
if [[ "$DO_INTEGRATION" -eq 1 ]]; then
  step "go test (integration mode, -tags=integration)"
  ( cd "$MODDIR" && go test -timeout=60m -tags=integration "${TEST_PKGS[@]}" )
  ok "integration tests"
else
  step "go test -short (fast mode)"
  ( cd "$MODDIR" && go test -short "${TEST_PKGS[@]}" )
  ok "unit tests"
fi

# Check for conflict markers and debug messages that can show secrets.
step "hygiene checks (conflict markers / secret-leaking debug prints)"
# Do not check a line that contains only equal signs. Markdown can use this line
# below a heading.
if git -C "$ROOT" grep -nE '^(<<<<<<<|>>>>>>>) ' -- '*.go' '*.md' >/dev/null 2>&1; then
  echo "found merge-conflict markers:" >&2
  git -C "$ROOT" grep -nE '^(<<<<<<<|>>>>>>>) ' -- '*.go' '*.md' >&2
  exit 1
fi
if git -C "$ROOT" grep -nE 'DEBUG:.*(client_id|clientId|secret|token)' -- '*.go' >/dev/null 2>&1; then
  echo "found debug print that may leak a secret:" >&2
  git -C "$ROOT" grep -nE 'DEBUG:.*(client_id|clientId|secret|token)' -- '*.go' >&2
  exit 1
fi
ok "hygiene"

if [[ "$DO_KIND" -eq 0 ]]; then
  echo
  if [[ "$DO_INTEGRATION" -eq 1 ]]; then
    ok "all integration checks passed"
  else
    ok "all fast checks passed"
  fi
  exit 0
fi

# Run the kind cluster test.
command -v kind >/dev/null    || { echo "kind not found on PATH" >&2; exit 1; }
command -v kubectl >/dev/null || { echo "kubectl not found on PATH" >&2; exit 1; }

cleanup() {
  if [[ "$KEEP" -eq 1 ]]; then
    echo "leaving cluster '$CLUSTER_NAME' running (--keep)"
    return
  fi
  step "deleting kind cluster '$CLUSTER_NAME'"
  kind delete cluster --name "$CLUSTER_NAME" >/dev/null 2>&1 || true
}
trap cleanup EXIT

step "creating kind cluster '$CLUSTER_NAME'"
if ! kind get clusters 2>/dev/null | grep -qx "$CLUSTER_NAME"; then
  kind create cluster --name "$CLUSTER_NAME" --wait 60s
fi
kubectl cluster-info --context "kind-$CLUSTER_NAME" >/dev/null
ok "cluster up"

# Apply each manifest to the API server as a server-side dry run. Set
# MANIFEST_DIR to the directory that contains the generated YAML files. If this
# variable is empty, the script tests only the cluster.
MANIFEST_DIR="${MANIFEST_DIR:-}"
if [[ -n "$MANIFEST_DIR" && -d "$MANIFEST_DIR" ]]; then
  step "server-side dry-run apply of manifests in $MANIFEST_DIR"
  shopt -s nullglob
  for f in "$MANIFEST_DIR"/*.yaml "$MANIFEST_DIR"/*.yml; do
    echo "  applying $f"
    kubectl apply --dry-run=server -f "$f" --context "kind-$CLUSTER_NAME"
  done
  ok "manifests validate against live API server"
else
  echo "MANIFEST_DIR is not set; the manifest test is skipped"
fi

echo
ok "kind smoke test passed"
