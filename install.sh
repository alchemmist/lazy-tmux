#!/bin/sh
set -euo pipefail

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
      echo "Unknown argument: $arg" >&2
      print_usage
      exit 1
      ;;
  esac
done

os=$(uname -s | tr '[:upper:]' '[:lower:]')
case "$os" in
  darwin|linux) ;; 
  *)
    echo "Unsupported OS: $os" >&2
    exit 1
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
    echo "Unsupported architecture: $arch" >&2
    exit 1
    ;;
 esac

suffix=""
if [ "$fzf_only" -eq 1 ]; then
  suffix="_fzf"
fi

repo="alchemmist/lazy-tmux"
asset="lazy-tmux_${os}_${arch}${suffix}.tar.gz"
url="https://github.com/${repo}/releases/latest/download/${asset}"

tmp_dir=$(mktemp -d)
cleanup() {
  rm -rf "$tmp_dir"
}
trap cleanup EXIT INT TERM

curl -fsSL "$url" -o "$tmp_dir/$asset"

tar -xzf "$tmp_dir/$asset" -C "$tmp_dir"

bin_name="lazy-tmux"
if [ ! -f "$tmp_dir/$bin_name" ]; then
  echo "Binary not found in archive" >&2
  exit 1
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

if command -v install >/dev/null 2>&1; then
  install -m 0755 "$tmp_dir/$bin_name" "$install_dir/$bin_name"
else
  cp "$tmp_dir/$bin_name" "$install_dir/$bin_name"
  chmod 0755 "$install_dir/$bin_name"
fi

echo "Installed lazy-tmux to $install_dir/$bin_name"
if [ "$fzf_only" -eq 1 ]; then
  echo "Note: fzf-only build requires fzf in PATH."
fi
