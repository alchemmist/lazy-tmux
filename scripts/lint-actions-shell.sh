#!/usr/bin/env bash
# Run checksum-pinned actionlint and shellcheck against this repository.
#
# Versions and archive checksums are pinned below so CI and local runs use the
# same binaries. The runner lints every workflow in .github/workflows and every
# shell script in scripts/, docker/, and docs/public/. Generated Quality Graph
# workflows and the tmux version scripts are included in that scan; this runner
# does not modify them.

set -euo pipefail

ACTIONLINT_VERSION=1.7.12
SHELLCHECK_VERSION=0.11.0

ROOT=$(
  unset CDPATH
  cd "$(dirname -- "${BASH_SOURCE[0]}")/.." || exit 1
  pwd
)
TOOL_DIR=${LINT_TOOLS_DIR:-$ROOT/.cache/lint-tools}
TMP_DIRS=()

cleanup() {
  local dir
  if [[ ${#TMP_DIRS[@]} -eq 0 ]]; then
    return 0
  fi
  for dir in "${TMP_DIRS[@]}"; do
    rm -rf "$dir"
  done
}

sha256_file() {
  local file
  file=$1
  if command -v sha256sum >/dev/null 2>&1; then
    sha256sum "$file" | awk '{print $1}'
  elif command -v shasum >/dev/null 2>&1; then
    shasum -a 256 "$file" | awk '{print $1}'
  else
    echo "sha256sum or shasum is required to verify pinned tools" >&2
    return 1
  fi
}

verify_sha256() {
  local file expected actual
  file=$1
  expected=$2
  actual=$(sha256_file "$file")
  if [[ "$actual" != "$expected" ]]; then
    echo "checksum mismatch for $(basename -- "$file")" >&2
    echo "expected $expected" >&2
    echo "actual   $actual" >&2
    return 1
  fi
}

platform_id() {
  local os arch
  os=$(uname -s)
  arch=$(uname -m)
  case "$os" in
    Darwin) os=darwin ;;
    Linux) os=linux ;;
    *)
      echo "unsupported OS for pinned lint tools: $os" >&2
      return 1
      ;;
  esac
  case "$arch" in
    x86_64 | amd64) arch=amd64 ;;
    arm64 | aarch64) arch=arm64 ;;
    *)
      echo "unsupported architecture for pinned lint tools: $arch" >&2
      return 1
      ;;
  esac
  printf '%s-%s\n' "$os" "$arch"
}

actionlint_asset() {
  case "$(platform_id)" in
    darwin-amd64) printf 'actionlint_%s_darwin_amd64.tar.gz\n' "$ACTIONLINT_VERSION" ;;
    darwin-arm64) printf 'actionlint_%s_darwin_arm64.tar.gz\n' "$ACTIONLINT_VERSION" ;;
    linux-amd64) printf 'actionlint_%s_linux_amd64.tar.gz\n' "$ACTIONLINT_VERSION" ;;
    linux-arm64) printf 'actionlint_%s_linux_arm64.tar.gz\n' "$ACTIONLINT_VERSION" ;;
    *)
      echo "no pinned actionlint archive for $(platform_id)" >&2
      return 1
      ;;
  esac
}

shellcheck_asset() {
  case "$(platform_id)" in
    darwin-amd64) printf 'shellcheck-v%s.darwin.x86_64.tar.xz\n' "$SHELLCHECK_VERSION" ;;
    darwin-arm64) printf 'shellcheck-v%s.darwin.aarch64.tar.xz\n' "$SHELLCHECK_VERSION" ;;
    linux-amd64) printf 'shellcheck-v%s.linux.x86_64.tar.xz\n' "$SHELLCHECK_VERSION" ;;
    linux-arm64) printf 'shellcheck-v%s.linux.aarch64.tar.xz\n' "$SHELLCHECK_VERSION" ;;
    *)
      echo "no pinned shellcheck archive for $(platform_id)" >&2
      return 1
      ;;
  esac
}

pinned_sha256() {
  case "$1" in
    actionlint_1.7.12_darwin_amd64.tar.gz) printf '%s\n' '5b44c3bc2255115c9b69e30efc0fecdf498fdb63c5d58e17084fd5f16324c644' ;;
    actionlint_1.7.12_darwin_arm64.tar.gz) printf '%s\n' 'aba9ced2dee8d27fecca3dc7feb1a7f9a52caefa1eb46f3271ea66b6e0e6953f' ;;
    actionlint_1.7.12_linux_amd64.tar.gz) printf '%s\n' '8aca8db96f1b94770f1b0d72b6dddcb1ebb8123cb3712530b08cc387b349a3d8' ;;
    actionlint_1.7.12_linux_arm64.tar.gz) printf '%s\n' '325e971b6ba9bfa504672e29be93c24981eeb1c07576d730e9f7c8805afff0c6' ;;
    shellcheck-v0.11.0.darwin.x86_64.tar.xz) printf '%s\n' '3c89db4edcab7cf1c27bff178882e0f6f27f7afdf54e859fa041fca10febe4c6' ;;
    shellcheck-v0.11.0.darwin.aarch64.tar.xz) printf '%s\n' '56affdd8de5527894dca6dc3d7e0a99a873b0f004d7aabc30ae407d3f48b0a79' ;;
    shellcheck-v0.11.0.linux.x86_64.tar.xz) printf '%s\n' '8c3be12b05d5c177a04c29e3c78ce89ac86f1595681cab149b65b97c4e227198' ;;
    shellcheck-v0.11.0.linux.aarch64.tar.xz) printf '%s\n' '12b331c1d2db6b9eb13cfca64306b1b157a86eb69db83023e261eaa7e7c14588' ;;
    *)
      echo "no pinned checksum for $1" >&2
      return 1
      ;;
  esac
}

tool_reports_version() {
  local bin flag expected output
  bin=$1
  flag=$2
  expected=$3
  [[ -x "$bin" ]] || return 1
  output=$("$bin" "$flag" 2>&1) || return 1
  case "$output" in
    *"$expected"*) return 0 ;;
    *) return 1 ;;
  esac
}

assert_tool_version() {
  local bin flag expected output
  bin=$1
  flag=$2
  expected=$3
  output=$("$bin" "$flag" 2>&1) || true
  case "$output" in
    *"$expected"*) ;;
    *)
      echo "expected $(basename -- "$bin") to report $expected" >&2
      printf '%s\n' "$output" >&2
      return 1
      ;;
  esac
}

download_verified() {
  local url asset dest
  url=$1
  asset=$2
  dest=$3
  command -v curl >/dev/null 2>&1 || {
    echo "curl is required to download pinned lint tools" >&2
    return 1
  }
  curl -fsSL --retry 3 --retry-delay 2 -o "$dest" "$url"
  verify_sha256 "$dest" "$(pinned_sha256 "$asset")"
}

extract_archive() {
  local archive dest_dir
  archive=$1
  dest_dir=$2
  case "$archive" in
    *.tar.gz) tar -xzf "$archive" -C "$dest_dir" ;;
    *.tar.xz) tar -xJf "$archive" -C "$dest_dir" ;;
    *)
      echo "unsupported archive type: $archive" >&2
      return 1
      ;;
  esac
}

install_pinned_tool() {
  local url asset member dest bin_flag expected_version tmp
  url=$1
  asset=$2
  member=$3
  dest=$4
  bin_flag=$5
  expected_version=$6

  if tool_reports_version "$dest" "$bin_flag" "$expected_version"; then
    return 0
  fi

  tmp=$(mktemp -d)
  TMP_DIRS+=("$tmp")
  download_verified "$url" "$asset" "$tmp/$asset"
  extract_archive "$tmp/$asset" "$tmp"
  if [[ ! -f "$tmp/$member" ]]; then
    echo "archive $asset did not contain $member" >&2
    return 1
  fi
  mkdir -p "$(dirname -- "$dest")"
  mv "$tmp/$member" "$dest"
  chmod 0755 "$dest"
  assert_tool_version "$dest" "$bin_flag" "$expected_version"
}

install_tools() {
  local actionlint_name shellcheck_name
  actionlint_name=$(actionlint_asset)
  shellcheck_name=$(shellcheck_asset)
  install_pinned_tool \
    "https://github.com/rhysd/actionlint/releases/download/v${ACTIONLINT_VERSION}/${actionlint_name}" \
    "$actionlint_name" \
    "actionlint" \
    "$TOOL_DIR/actionlint" \
    "-version" \
    "$ACTIONLINT_VERSION"
  install_pinned_tool \
    "https://github.com/koalaman/shellcheck/releases/download/v${SHELLCHECK_VERSION}/${shellcheck_name}" \
    "$shellcheck_name" \
    "shellcheck-v${SHELLCHECK_VERSION}/shellcheck" \
    "$TOOL_DIR/shellcheck" \
    "--version" \
    "$SHELLCHECK_VERSION"
}

collect_files() {
  local kind
  kind=$1
  case "$kind" in
    workflows)
      find .github/workflows -type f \( -name '*.yml' -o -name '*.yaml' \) | sort
      ;;
    scripts)
      find scripts docker docs/public -type f -name '*.sh' | sort
      ;;
    *)
      echo "unknown file group: $kind" >&2
      return 1
      ;;
  esac
}

append_lines() {
  local line
  while IFS= read -r line; do
    [[ -n "$line" ]] || continue
    FILES+=("$line")
  done <<EOF
$1
EOF
}

lint_repo() {
  local status workflow_list script_list
  cd "$ROOT"
  workflow_list=$(collect_files workflows) || exit 1
  script_list=$(collect_files scripts) || exit 1

  status=0
  FILES=()
  if [[ -n "$workflow_list" ]]; then
    append_lines "$workflow_list"
    "$TOOL_DIR/actionlint" -color=false -shellcheck="$TOOL_DIR/shellcheck" "${FILES[@]}" || status=1
  fi
  FILES=()
  if [[ -n "$script_list" ]]; then
    append_lines "$script_list"
    "$TOOL_DIR/shellcheck" --external-sources --severity=style --format=gcc --norc "${FILES[@]}" || status=1
  fi
  return "$status"
}

main() {
  trap cleanup EXIT
  printf 'lint-actions-shell: actionlint %s, shellcheck %s\n' "$ACTIONLINT_VERSION" "$SHELLCHECK_VERSION"
  install_tools
  lint_repo
}

if [[ "${BASH_SOURCE[0]}" == "$0" ]]; then
  main "$@"
fi
