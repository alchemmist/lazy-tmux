#!/usr/bin/env bash
set -euo pipefail

if [ -z "${GITHUB_REF_NAME:-}" ]; then
  echo "GITHUB_REF_NAME is required" >&2
  exit 1
fi

if [ -z "${AUR_REPO_URL:-}" ]; then
  echo "AUR_REPO_URL is required" >&2
  exit 1
fi

tag="$GITHUB_REF_NAME"
ver="${tag#v}"

dist_dir="${DIST_DIR:-dist}"
checksums_file="$dist_dir/checksums.txt"

if [ ! -f "$checksums_file" ]; then
  echo "checksums not found: $checksums_file" >&2
  exit 1
fi

sha_amd64=$(awk '/lazy-tmux_linux_amd64.tar.gz$/{print $1}' "$checksums_file")
sha_amd64_fzf=$(awk '/lazy-tmux_linux_amd64_fzf.tar.gz$/{print $1}' "$checksums_file")
sha_arm64=$(awk '/lazy-tmux_linux_arm64.tar.gz$/{print $1}' "$checksums_file")
sha_arm64_fzf=$(awk '/lazy-tmux_linux_arm64_fzf.tar.gz$/{print $1}' "$checksums_file")

if [ -z "$sha_amd64" ] || [ -z "$sha_amd64_fzf" ] || [ -z "$sha_arm64" ] || [ -z "$sha_arm64_fzf" ]; then
  echo "missing checksums for required artifacts" >&2
  exit 1
fi

workdir=$(mktemp -d)
trap 'rm -rf "$workdir"' EXIT

# Full (not shallow) clone so the rebase-on-retry below has the history it needs
# if a concurrent release advanced the AUR repo between clone and push.
git clone --single-branch "$AUR_REPO_URL" "$workdir"

pkgbuild="$workdir/PKGBUILD"
srcinfo="$workdir/.SRCINFO"

sed -i "s/^pkgver=.*/pkgver=${ver}/" "$pkgbuild"
sed -i "s/^sha256sums_x86_64=.*/sha256sums_x86_64=('${sha_amd64}' '${sha_amd64_fzf}')/" "$pkgbuild"
sed -i "s/^sha256sums_aarch64=.*/sha256sums_aarch64=('${sha_arm64}' '${sha_arm64_fzf}')/" "$pkgbuild"

# The release assets are named WITHOUT the version (goreleaser name_template is
# "{{ .ProjectName }}_{{ .Os }}_{{ .Arch }}"), so the download URL filename must
# not carry ${pkgver} — only the local alias (before "::") and the v${pkgver}
# path tag keep it. Strip a stray version from the URL-side filename (issue #141).
# Single-quoted so ${pkgver} stays a literal PKGBUILD variable.
sed -i 's#\(/releases/download/v${pkgver}/\)lazy-tmux_${pkgver}_#\1lazy-tmux_#g' "$pkgbuild"

sed -i "s/^\tpkgver = .*/\tpkgver = ${ver}/" "$srcinfo"
sed -i "s#releases/download/v[0-9.\-]\+/lazy-tmux_#releases/download/v${ver}/lazy-tmux_#g" "$srcinfo"
sed -i "s#lazy-tmux_[0-9.\-]\+_#lazy-tmux_${ver}_#g" "$srcinfo"

# Same as PKGBUILD: the download URL filename must be unversioned. The previous
# sed bumps the version everywhere (correct for the local alias), so strip it
# back off only the URL-side filename, right after the v${ver} path (issue #141).
sed -i -E "s#(/releases/download/v${ver}/)lazy-tmux_[0-9][0-9.\-]*_#\1lazy-tmux_#g" "$srcinfo"

perl -0777 -i -pe "s/sha256sums_x86_64 = .*\n\tsha256sums_x86_64 = .*/sha256sums_x86_64 = ${sha_amd64}\n\tsha256sums_x86_64 = ${sha_amd64_fzf}/" "$srcinfo"
perl -0777 -i -pe "s/sha256sums_aarch64 = .*\n\tsha256sums_aarch64 = .*/sha256sums_aarch64 = ${sha_arm64}\n\tsha256sums_aarch64 = ${sha_arm64_fzf}/" "$srcinfo"

grep -q "pkgver = ${ver}" "$srcinfo" || { echo "AUR update failed: pkgver" >&2; exit 1; }
grep -q "sha256sums_x86_64 = ${sha_amd64}" "$srcinfo" || { echo "AUR update failed: sha256sums_x86_64 (main)" >&2; exit 1; }
grep -q "sha256sums_x86_64 = ${sha_amd64_fzf}" "$srcinfo" || { echo "AUR update failed: sha256sums_x86_64 (fzf)" >&2; exit 1; }
grep -q "sha256sums_aarch64 = ${sha_arm64}" "$srcinfo" || { echo "AUR update failed: sha256sums_aarch64 (main)" >&2; exit 1; }
grep -q "sha256sums_aarch64 = ${sha_arm64_fzf}" "$srcinfo" || { echo "AUR update failed: sha256sums_aarch64 (fzf)" >&2; exit 1; }

# Guard against the issue #141 regression: the download URL must point at the
# real (unversioned) release asset, not lazy-tmux_<ver>_linux_*.tar.gz (404).
grep -q "releases/download/v${ver}/lazy-tmux_linux_amd64.tar.gz" "$srcinfo" || { echo "AUR update failed: versioned asset URL in .SRCINFO (issue #141)" >&2; exit 1; }
grep -qE "releases/download/v${ver}/lazy-tmux_[0-9]" "$srcinfo" && { echo "AUR update failed: .SRCINFO still has a versioned asset URL (issue #141)" >&2; exit 1; }
grep -q 'releases/download/v${pkgver}/lazy-tmux_linux_amd64.tar.gz' "$pkgbuild" || { echo "AUR update failed: versioned asset URL in PKGBUILD (issue #141)" >&2; exit 1; }

git -C "$workdir" add PKGBUILD .SRCINFO
if git -C "$workdir" diff --cached --quiet; then
  echo "AUR: no changes to commit"
  exit 0
fi

git -C "$workdir" commit -m "update to ${tag}"

# Push with retry: a concurrent release may have advanced the AUR repo since we
# cloned, which rejects the push as non-fast-forward. Rebase onto the latest
# remote state and retry a few times before giving up.
for attempt in 1 2 3; do
  if git -C "$workdir" push; then
    exit 0
  fi

  echo "AUR push rejected (attempt ${attempt}); rebasing onto remote and retrying" >&2
  git -C "$workdir" pull --rebase --no-edit
done

echo "AUR push failed after retries" >&2
exit 1
