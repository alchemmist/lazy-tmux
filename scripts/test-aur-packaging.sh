#!/usr/bin/env bash
set -euo pipefail
trap 'printf "AUR packaging check failed at line %s: %s\n" "$LINENO" "$BASH_COMMAND" >&2' ERR
work=$(mktemp -d)
trap 'rm -rf "$work"' EXIT
# AUR remains the single source of truth. An explicit local checkout can be
# supplied when validating an unpublished AUR packaging fix.
recipe_dir=${AUR_RECIPE_DIR:-}
if [[ -z "$recipe_dir" ]]; then
  git clone --depth 1 https://aur.archlinux.org/lazy-tmux.git "$work/aur"
  recipe_dir="$work/aur"
fi
source "$recipe_dir/PKGBUILD"
for CARCH in x86_64 aarch64; do
  srcdir="$work/$CARCH/src"
  mkdir -p "$srcdir" "$work/fixture"
  for suffix in '' _fzf; do
    printf '%s\n' "binary$suffix" > "$work/fixture/lazy-tmux"
    printf 'license\n' > "$work/fixture/LICENSE"
    printf 'readme\n' > "$work/fixture/README.md"
    tar -czf "$srcdir/lazy-tmux_${pkgver}_linux_$(_package_arch_suffix)${suffix}.tar.gz" -C "$work/fixture" .
  done
  for binary in lazy-tmux lazy-tmux-fzf; do
    pkgdir="$work/$CARCH/$binary"
    "package_$binary"
    test -x "$pkgdir/usr/bin/$binary"
    test -f "$pkgdir/usr/share/licenses/$binary/LICENSE"
    test -f "$pkgdir/usr/share/doc/$binary/README.md"
    test "$(find "$pkgdir" -type f | wc -l | tr -d ' ')" = 3
    (cd "$pkgdir" && find . -type f | sort) > "$work/$binary.files"
  done
  test -z "$(comm -12 "$work/lazy-tmux.files" "$work/lazy-tmux-fzf.files")"
  grep -qx binary "$work/$CARCH/lazy-tmux/usr/bin/lazy-tmux"
  grep -qx binary_fzf "$work/$CARCH/lazy-tmux-fzf/usr/bin/lazy-tmux-fzf"
done
printf 'AUR split-package tests passed for x86_64 and aarch64\n'
