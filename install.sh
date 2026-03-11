#!/bin/sh
set -eu
if (set -o pipefail) 2>/dev/null; then
  set -o pipefail
fi

info() {
  printf '%s\n' "==> $*"
}

warn() {
  printf '%s\n' "warning: $*" >&2
}

die() {
  printf '%s\n' "error: $*" >&2
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
      die "Unknown argument: $arg"
      print_usage
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
if [ "$fzf_only" -eq 1 ]; then
  suffix="_fzf"
fi

repo="alchemmist/lazy-tmux"
asset="lazy-tmux_${os}_${arch}${suffix}.tar.gz"
url="https://github.com/${repo}/releases/latest/download/${asset}"

info "Detected platform: ${os}/${arch}"
if [ "$fzf_only" -eq 1 ]; then
  info "Selected installer: fzf-only binary"
else
  info "Selected installer: TUI binary"
fi

if ! command -v tmux >/dev/null 2>&1; then
  warn "tmux is not installed; lazy-tmux requires tmux to run."
fi

if [ "$fzf_only" -eq 1 ] && ! command -v fzf >/dev/null 2>&1; then
  warn "fzf is not installed; the fzf-only binary requires fzf in PATH."
fi

tmp_dir=$(mktemp -d)
cleanup() {
  rm -rf "$tmp_dir"
}
trap cleanup EXIT INT TERM

info "Downloading ${asset}"
curl -fsSL "$url" -o "$tmp_dir/$asset"

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

info "Installing to ${install_dir}"
if command -v install >/dev/null 2>&1; then
  install -m 0755 "$tmp_dir/$bin_name" "$install_dir/$bin_name"
else
  cp "$tmp_dir/$bin_name" "$install_dir/$bin_name"
  chmod 0755 "$install_dir/$bin_name"
fi

info "Installed lazy-tmux to $install_dir/$bin_name"
if [ "$fzf_only" -eq 1 ]; then
  info "Note: fzf-only build requires fzf in PATH."
fi

if [ "$install_dir" = "$HOME/.local/bin" ]; then
  case ":$PATH:" in
    *":$HOME/.local/bin:"*) ;;
    *)
      warn "$HOME/.local/bin is not in PATH. Add it to your shell profile to use lazy-tmux."
      ;;
  esac
fi
