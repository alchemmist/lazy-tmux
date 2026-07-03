#!/usr/bin/env bash
# Cut a release: bump the semver tag and push it, letting the release pipeline
# (GoReleaser -> GitHub release, Homebrew tap, AUR) take over from there.
#
# Usage: scripts/release.sh <patch|minor|major>
# Invoked via `make release-patch` / `release-minor` / `release-major`.
set -euo pipefail

bump="${1:-}"
case "$bump" in
patch | minor | major) ;;
*)
  echo "usage: $0 <patch|minor|major>" >&2
  exit 1
  ;;
esac

# Releases are cut from an up-to-date, clean main only: the tag must point at
# a commit that is already on origin/main, or the pipeline builds something
# nobody has reviewed.
branch="$(git rev-parse --abbrev-ref HEAD)"
if [ "$branch" != "main" ]; then
  echo "releases are cut from main (currently on $branch)" >&2
  exit 1
fi

if ! git diff --quiet || ! git diff --cached --quiet; then
  echo "working tree is dirty; commit or stash changes first" >&2
  exit 1
fi

git fetch origin main --tags
if [ "$(git rev-parse HEAD)" != "$(git rev-parse origin/main)" ]; then
  echo "main is not in sync with origin/main; pull or push first" >&2
  exit 1
fi

latest="$(git tag --list 'v*' --sort=-v:refname | head -n1)"
if [ -z "$latest" ]; then
  next="v0.1.0"
else
  ver="${latest#v}"
  IFS=. read -r major minor patch <<<"$ver"
  case "$bump" in
  patch) patch=$((patch + 1)) ;;
  minor)
    minor=$((minor + 1))
    patch=0
    ;;
  major)
    major=$((major + 1))
    minor=0
    patch=0
    ;;
  esac
  next="v${major}.${minor}.${patch}"
fi

if git rev-parse -q --verify "refs/tags/$next" >/dev/null; then
  echo "tag $next already exists" >&2
  exit 1
fi

echo "==> running checks before tagging"
make check

echo
echo "==> $bump release: $latest -> $next"
if [ -n "$latest" ]; then
  echo
  git log --oneline --no-decorate "$latest"..HEAD
  echo
fi

read -r -p "tag and push $next? [y/N] " answer
case "$answer" in
[yY]*) ;;
*)
  echo "aborted; nothing was tagged"
  exit 1
  ;;
esac

git tag -a "$next" -m "release $next"
git push origin "$next"

echo "pushed $next — the release workflow takes it from here:"
echo "  https://github.com/alchemmist/lazy-tmux/actions/workflows/release.yml"
