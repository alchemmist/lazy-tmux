#!/usr/bin/env bash
# Proves the pinned actionlint and shellcheck runner executes, rejects a bad
# checksum, and fails on invalid workflow and shell fixtures.

set -euo pipefail

ROOT=$(
  unset CDPATH
  cd "$(dirname -- "${BASH_SOURCE[0]}")/.." || exit 1
  pwd
)
# ROOT is computed at runtime, so shellcheck cannot follow this source.
# shellcheck disable=SC1091
source "$ROOT/scripts/lint-actions-shell.sh"

assert_hex_checksum() {
  local asset sum
  asset=$1
  sum=$(pinned_sha256 "$asset")
  if [[ ${#sum} -ne 64 ]]; then
    echo "pinned checksum for $asset is not 64 characters" >&2
    return 1
  fi
  case "$sum" in
    *[!0-9a-f]*)
      echo "pinned checksum for $asset is not lowercase hex" >&2
      return 1
      ;;
  esac
}

expect_checksum_mismatch() {
  local sample
  sample=$(mktemp)
  printf 'not the pinned archive\n' >"$sample"
  if verify_sha256 "$sample" "0000000000000000000000000000000000000000000000000000000000000000" 2>/dev/null; then
    rm -f "$sample"
    echo "verify_sha256 accepted a mismatched checksum" >&2
    return 1
  fi
  verify_sha256 "$sample" "$(sha256_file "$sample")"
  rm -f "$sample"
}

expect_actionlint_failure() {
  local fixture
  fixture=$(mktemp -d)
  cat >"$fixture/bad.yml" <<'EOF'
name: bad
on: push
jobs:
  bad:
    runs-on: ubuntu-latest
    steps:
      - name: Unquoted expansion
        run: echo $UNQUOTED
      - name: Broken expression
        run: echo ${{ github.sha }
EOF
  if "$TOOL_DIR/actionlint" -color=false -shellcheck="$TOOL_DIR/shellcheck" "$fixture/bad.yml" >/dev/null 2>&1; then
    rm -rf "$fixture"
    echo "actionlint accepted an invalid workflow" >&2
    return 1
  fi
  rm -rf "$fixture"
}

expect_shellcheck_failure() {
  local fixture
  fixture=$(mktemp)
  cat >"$fixture" <<'EOF'
#!/usr/bin/env bash
echo $UNQUOTED
EOF
  if "$TOOL_DIR/shellcheck" --external-sources --severity=style --format=gcc --norc "$fixture" >/dev/null 2>&1; then
    rm -f "$fixture"
    echo "shellcheck accepted an unquoted expansion" >&2
    return 1
  fi
  rm -f "$fixture"
}

assert_hex_checksum "$(actionlint_asset)"
assert_hex_checksum "$(shellcheck_asset)"
expect_checksum_mismatch
"$ROOT/scripts/lint-actions-shell.sh"
assert_tool_version "$TOOL_DIR/actionlint" -version "$ACTIONLINT_VERSION"
assert_tool_version "$TOOL_DIR/shellcheck" --version "$SHELLCHECK_VERSION"
expect_actionlint_failure
expect_shellcheck_failure
printf 'test-lint-actions-shell: ok\n'
