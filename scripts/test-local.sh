#!/usr/bin/env bash
#
# test-local.sh — local validation harness for the Tailscale integration stack.
#
# Each PR in the stack documents which mode(s) it should be tested with.
# By default this runs the fast checks (build + unit tests). Pass --kind to
# additionally spin up a throwaway kind cluster and apply the component's
# generated manifests/CRDs against a live API server.
#
# Usage:
#   scripts/test-local.sh                # build + unit tests (default, no cluster)
#   scripts/test-local.sh --kind         # + spin up kind and smoke-test manifests
#   scripts/test-local.sh --kind --keep  # leave the kind cluster running afterwards
#   scripts/test-local.sh --integration  # also run the Docker/testcontainers suite
#   PKG=./internal/component/tailscale/... scripts/test-local.sh   # narrow tests
#
# By default the Docker-backed integration suite under ./test/integration/... is
# EXCLUDED — it needs a running Docker daemon and is the slow tier. Use --kind
# (manifest smoke test) or --integration (full container suite) to opt in.
#
# Requirements: go. For --kind/--integration: kind/kubectl/Docker as noted.
set -euo pipefail

CLUSTER_NAME="${CLUSTER_NAME:-foundry-local-test}"
# Default: everything except the Docker-gated integration suite.
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

# Resolve repo root and the Go module dir (module lives under v1/).
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
MODDIR="$ROOT/v1"

step() { printf '\n\033[1;34m==> %s\033[0m\n' "$1"; }
ok()   { printf '\033[1;32m✓ %s\033[0m\n' "$1"; }

# Resolve the package set to test. When PKG is unset, test everything except the
# Docker-gated integration suite (unless --integration was passed).
if [[ -n "$PKG" ]]; then
  PKGS="$PKG"
elif [[ "$DO_INTEGRATION" -eq 1 ]]; then
  PKGS="./..."
else
  PKGS="$( cd "$MODDIR" && go list ./... | grep -v '/test/integration' )"
fi

step "go build"
( cd "$MODDIR" && go build ./... )
ok "build"

step "go vet"
( cd "$MODDIR" && go vet ./... )
ok "vet"

step "go test"
( cd "$MODDIR" && go test $PKGS )
ok "unit tests"

# Guard against regressions the reviews found: leftover conflict markers or
# debug prints that leak secrets. Cheap to check, worth catching in every PR.
step "hygiene checks (conflict markers / secret-leaking debug prints)"
if git -C "$ROOT" grep -nE '^(<<<<<<<|=======|>>>>>>>)' -- '*.go' '*.md' >/dev/null 2>&1; then
  echo "found merge-conflict markers:" >&2
  git -C "$ROOT" grep -nE '^(<<<<<<<|=======|>>>>>>>)' -- '*.go' '*.md' >&2
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
  ok "all fast checks passed (run with --kind for live-cluster smoke test)"
  exit 0
fi

# ---- kind live smoke test ---------------------------------------------------
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

# Per-PR manifest smoke test hook.
# Later PRs that generate manifests/CRDs (helm values, Connector/DNSConfig CRDs,
# CoreDNS ConfigMap patch) drop rendered YAML into a dir and set MANIFEST_DIR so
# this loop dry-run-applies them against the live API server to catch schema
# errors. If nothing is provided, this is a no-op and the cluster-up is the test.
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
  echo "no MANIFEST_DIR set — skipping manifest apply (cluster-up smoke test only)"
fi

echo
ok "kind smoke test passed"
