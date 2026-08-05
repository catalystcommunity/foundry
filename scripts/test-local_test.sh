#!/usr/bin/env bash
#
# Test the command selection and the final messages in test-local.sh.
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
TEST_DIR="$(mktemp -d)"
FAKE_BIN="$TEST_DIR/bin"
mkdir -p "$FAKE_BIN"

cleanup() {
  rm -rf "$TEST_DIR"
}
trap cleanup EXIT

cat >"$FAKE_BIN/go" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
printf '%s\n' "$*" >>"$TRACE_FILE"
if [[ "${1:-}" == "list" ]]; then
  printf '%s\n' \
    "github.com/catalystcommunity/foundry/v1/internal/config" \
    "github.com/catalystcommunity/foundry/v1/test/integration"
fi
EOF

cat >"$FAKE_BIN/gofmt" <<'EOF'
#!/usr/bin/env bash
exit 0
EOF

cat >"$FAKE_BIN/git" <<'EOF'
#!/usr/bin/env bash
exit 1
EOF

chmod +x "$FAKE_BIN/go" "$FAKE_BIN/gofmt" "$FAKE_BIN/git"

assert_contains() {
  local content="$1"
  local expected="$2"
  if [[ "$content" != *"$expected"* ]]; then
    printf 'missing expected text: %s\n' "$expected" >&2
    exit 1
  fi
}

assert_trace_line() {
  local trace_file="$1"
  local expected="$2"
  if ! grep -Fxq "$expected" "$trace_file"; then
    printf 'missing expected command: %s\n' "$expected" >&2
    exit 1
  fi
}

assert_no_trace_line() {
  local trace_file="$1"
  local rejected="$2"
  if grep -Fxq "$rejected" "$trace_file"; then
    printf 'found unexpected command: %s\n' "$rejected" >&2
    exit 1
  fi
}

FAST_TRACE="$TEST_DIR/fast.trace"
FAST_OUTPUT="$(
  PATH="$FAKE_BIN:$PATH" \
  TRACE_FILE="$FAST_TRACE" \
  PKG="./internal/component/tailscale/..." \
  "$ROOT/scripts/test-local.sh"
)"
assert_contains "$FAST_OUTPUT" "all fast checks passed"
assert_trace_line "$FAST_TRACE" "test -short ./internal/component/tailscale/..."

DEFAULT_TRACE="$TEST_DIR/default.trace"
DEFAULT_OUTPUT="$(
  PATH="$FAKE_BIN:$PATH" \
  TRACE_FILE="$DEFAULT_TRACE" \
  "$ROOT/scripts/test-local.sh"
)"
assert_contains "$DEFAULT_OUTPUT" "all fast checks passed"
assert_trace_line "$DEFAULT_TRACE" "test -short github.com/catalystcommunity/foundry/v1/internal/config"
assert_no_trace_line "$DEFAULT_TRACE" "test -short github.com/catalystcommunity/foundry/v1/test/integration"

INTEGRATION_TRACE="$TEST_DIR/integration.trace"
INTEGRATION_OUTPUT="$(
  PATH="$FAKE_BIN:$PATH" \
  TRACE_FILE="$INTEGRATION_TRACE" \
  "$ROOT/scripts/test-local.sh" --integration
)"
assert_contains "$INTEGRATION_OUTPUT" "all integration checks passed"
assert_trace_line "$INTEGRATION_TRACE" "test -timeout=60m -tags=integration ./..."
assert_no_trace_line "$INTEGRATION_TRACE" "test -short ./..."

printf 'test-local.sh command tests passed\n'
