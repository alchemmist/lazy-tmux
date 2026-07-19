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
if [ -t 1 ] && [ -z "${CI:-}" ] && [ -z "${NO_COLOR:-}" ] && [ "${TERM:-dumb}" != "dumb" ]; then
  ui=1
fi

YELLOW=''
RESET=''
sleep_ok=0
term_cols=80
if [ "$ui" -eq 1 ]; then
  YELLOW=$(printf '\033[33m')
  RESET=$(printf '\033[0m')
  # Only animate the fill if the platform's sleep accepts fractional seconds;
  # otherwise jump straight to each milestone.
  if sleep 0.01 2>/dev/null; then
    sleep_ok=1
  fi
  # Terminal width, so a frame never wraps (a wrapped line defeats \r + \033[K
  # and leaves the previous frame's tail on screen). Fall back to 80 columns.
  _cols=$(tput cols 2>/dev/null) || _cols=''
  case $_cols in
    *[!0-9]* | '') ;;
    *) term_cols=$_cols ;;
  esac
fi

BAR_WIDTH=40
bar_cur=0
bar_active=0
# Columns consumed before the label: the bar (BAR_WIDTH), a space, "%3d%%" (4)
# and two trailing spaces = BAR_WIDTH + 7, plus 1 column of slack so the cursor
# never lands in the last column (where some terminals auto-wrap). Budgets how
# much of the label fits on one row.
BAR_PREFIX_WIDTH=$(( BAR_WIDTH + 8 ))

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
  # Clamp the label to what's left on the row so the line never wraps. Labels
  # are ASCII, so a byte-precision cut ('%.*s') matches the display width.
  _max=$(( term_cols - BAR_PREFIX_WIDTH ))
  if [ "$_max" -lt 0 ]; then
    _max=0
  fi
  printf '\r%s%s%s %3d%%  %.*s\033[K' "$YELLOW" "$_bar" "$RESET" "$_pct" "$_max" "$_label"
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

# Finish the bar's line on stdout (where the bar is drawn), so a following
# message or shell prompt doesn't land on the progress line — even if stderr
# is redirected away from the terminal.
end_bar_line() {
  if [ "$bar_active" -eq 1 ]; then
    printf '\n'
    bar_active=0
  fi
}

warn() {
  end_bar_line
  printf 'warning: %s\n' "$*" >&2
}

die() {
  end_bar_line
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
  end_bar_line
  rm -rf "$tmp_dir"
}
trap cleanup EXIT INT TERM

download() {
  _url=$1
  _out=$2
  _i=1
  while [ "$_i" -le 5 ]; do
    if curl -fsSL --retry 3 --retry-delay 2 --connect-timeout 30 "$_url" -o "$_out"; then
      return 0
    fi
    if command -v wget >/dev/null 2>&1 && wget -q -O "$_out" "$_url"; then
      return 0
    fi
    _i=$((_i + 1))
    sleep 2
  done
  return 1
}

step 30 "Downloading ${asset}"
download "$url" "$tmp_dir/$asset" || die "Failed to download ${asset}"

step 55 "Verifying checksum"
download "$checksums_url" "$tmp_dir/checksums.txt" || die "Failed to download checksums"

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

# Keep the bar's final label short (it would otherwise be the longest line and
# wrap); print the full install path on its own line below the finished bar.
bar_done "Installed lazy-tmux"
note "→ ${install_dir}/${bin_name}"

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
