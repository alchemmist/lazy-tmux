#!/bin/sh
set -eu
if (set -o pipefail) 2>/dev/null; then
  set -o pipefail
fi

# ---- progress UI -------------------------------------------------------------
# A yellow progress bar is shown only on a real terminal with color allowed.
# Piped runs (curl | sh > log), CI, NO_COLOR and TERM=dumb fall back to plain
# "==> step" lines, so no escape codes or carriage-return spam leak into logs.
ui=0
if [ -t 1 ] && [ -z "${NO_COLOR:-}" ] && [ "${TERM:-dumb}" != "dumb" ]; then
  ui=1
fi

YELLOW=''
RESET=''
sleep_ok=0
if [ "$ui" -eq 1 ]; then
  YELLOW=$(printf '\033[33m')
  RESET=$(printf '\033[0m')
  # Only animate the fill if the platform's sleep accepts fractional seconds;
  # otherwise jump straight to each milestone.
  if sleep 0.01 2>/dev/null; then
    sleep_ok=1
  fi
fi

BAR_WIDTH=40
bar_cur=0
bar_active=0

draw_bar() { # <percent> <label>
  _pct=$1
  _label=$2
  _filled=$(( _pct * BAR_WIDTH / 100 ))
  _bar=''
  _i=0
  while [ "$_i" -lt "$_filled" ]; do
    _bar="${_bar}■"
    _i=$(( _i + 1 ))
  done
  while [ "$_i" -lt "$BAR_WIDTH" ]; do
    _bar="${_bar}･"
    _i=$(( _i + 1 ))
  done
  printf '\r%s%s%s %3d%%  %s\033[K' "$YELLOW" "$_bar" "$RESET" "$_pct" "$_label"
}

step() { # <target percent> <label>
  _target=$1
  _label=$2
  if [ "$ui" -eq 1 ]; then
    bar_active=1
    while [ "$bar_cur" -lt "$_target" ]; do
      bar_cur=$(( bar_cur + 3 ))
      if [ "$bar_cur" -gt "$_target" ]; then
        bar_cur=$_target
      fi
      draw_bar "$bar_cur" "$_label"
      if [ "$sleep_ok" -eq 1 ]; then
        sleep 0.012
      fi
    done
    bar_cur=$_target
    draw_bar "$bar_cur" "$_label"
  else
    printf '==> %s\n' "$_label"
  fi
}

bar_done() { # <final label>
  if [ "$ui" -eq 1 ]; then
    step 100 "$1"
    printf '\n'
    bar_active=0
  else
    printf '==> %s\n' "$1"
  fi
}

note() {
  printf '%s\n' "$*"
}

warn() {
  if [ "$bar_active" -eq 1 ]; then
    printf '\n' >&2
    bar_active=0
  fi
  printf 'warning: %s\n' "$*" >&2
}

die() {
  if [ "$bar_active" -eq 1 ]; then
    printf '\n' >&2
    bar_active=0
  fi
  printf 'error: %s\n' "$*" >&2
  exit 1
}

print_usage() {
  cat <<'USAGE'
lazy-tmux install script

Usage:
  install.sh [--fzf-engine]

Options:
  --fzf-engine   Install the lightweight fzf-only binary (requires fzf)
USAGE
}

fzf_only=0
for arg in "$@"; do
  case "$arg" in
    --fzf-engine)
      fzf_only=1
      ;;
    -h|--help)
      print_usage
      exit 0
      ;;
    *)
      print_usage
      die "Unknown argument: $arg"
      ;;
  esac
done

os=$(uname -s | tr '[:upper:]' '[:lower:]')
case "$os" in
  darwin|linux) ;;
  *)
    die "Unsupported OS: $os"
    ;;
 esac

arch=$(uname -m)
case "$arch" in
  x86_64|amd64)
    arch="amd64"
    ;;
  arm64|aarch64)
    arch="arm64"
    ;;
  *)
    die "Unsupported architecture: $arch"
    ;;
 esac

suffix=""
kind="TUI binary"
if [ "$fzf_only" -eq 1 ]; then
  suffix="_fzf"
  kind="fzf-only binary"
fi

repo="alchemmist/lazy-tmux"
asset="lazy-tmux_${os}_${arch}${suffix}.tar.gz"
url="https://github.com/${repo}/releases/latest/download/${asset}"
checksums_url="https://github.com/${repo}/releases/latest/download/checksums.txt"

note "lazy-tmux · ${os}/${arch} · ${kind}"

if ! command -v tmux >/dev/null 2>&1; then
  warn "tmux is not installed; lazy-tmux requires tmux to run."
fi

if [ "$fzf_only" -eq 1 ] && ! command -v fzf >/dev/null 2>&1; then
  warn "fzf is not installed; the fzf-only binary requires fzf in PATH."
fi

tmp_dir=$(mktemp -d)
cleanup() {
  if [ "$bar_active" -eq 1 ]; then
    printf '\n'
    bar_active=0
  fi
  rm -rf "$tmp_dir"
}
trap cleanup EXIT INT TERM

step 30 "Downloading ${asset}"
curl -fsSL "$url" -o "$tmp_dir/$asset"

step 55 "Verifying checksum"
curl -fsSL "$checksums_url" -o "$tmp_dir/checksums.txt"

expected_sum=$(awk -v asset="$asset" '$2 == asset {print $1}' "$tmp_dir/checksums.txt")
if [ -z "$expected_sum" ]; then
  die "Checksum not found for ${asset}"
fi

if command -v sha256sum >/dev/null 2>&1; then
  actual_sum=$(sha256sum "$tmp_dir/$asset" | awk '{print $1}')
elif command -v shasum >/dev/null 2>&1; then
  actual_sum=$(shasum -a 256 "$tmp_dir/$asset" | awk '{print $1}')
else
  die "No sha256 tool found (sha256sum or shasum required)"
fi

if [ "$actual_sum" != "$expected_sum" ]; then
  die "Checksum mismatch for ${asset}"
fi

step 75 "Extracting archive"
tar -xzf "$tmp_dir/$asset" -C "$tmp_dir"

bin_name="lazy-tmux"
if [ ! -f "$tmp_dir/$bin_name" ]; then
  die "Binary not found in archive"
fi

install_dir="${LAZY_TMUX_INSTALL_DIR:-}"
if [ -z "$install_dir" ]; then
  if [ -w "/usr/local/bin" ]; then
    install_dir="/usr/local/bin"
  else
    install_dir="$HOME/.local/bin"
  fi
fi

mkdir -p "$install_dir"

step 92 "Installing to ${install_dir}"
if command -v install >/dev/null 2>&1; then
  install -m 0755 "$tmp_dir/$bin_name" "$install_dir/$bin_name"
else
  cp "$tmp_dir/$bin_name" "$install_dir/$bin_name"
  chmod 0755 "$install_dir/$bin_name"
fi

bar_done "Installed lazy-tmux to ${install_dir}/${bin_name}"

if [ "$fzf_only" -eq 1 ] && ! command -v fzf >/dev/null 2>&1; then
  note "Note: fzf-only build requires fzf in PATH."
fi

if [ "$install_dir" = "$HOME/.local/bin" ]; then
  case ":$PATH:" in
    *":$HOME/.local/bin:"*) ;;
    *)
      warn "$HOME/.local/bin is not in PATH. Add it to your shell profile to use lazy-tmux."
      ;;
  esac
fi
