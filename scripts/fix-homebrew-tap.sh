#!/usr/bin/env bash
set -euo pipefail

if [ -z "${TAP_REPO:-}" ]; then
  echo "TAP_REPO is not set"
  exit 1
fi

if [ -z "${TAP_TOKEN:-}" ]; then
  echo "TAP_TOKEN is not set"
  exit 1
fi

tmpdir="$(mktemp -d)"
trap 'rm -rf "$tmpdir"' EXIT

git clone "https://x-access-token:${TAP_TOKEN}@github.com/${TAP_REPO}.git" "$tmpdir/tap"

cd "$tmpdir/tap"

mkdir -p Formula

moved=false
for f in *.rb; do
  if [ -f "$f" ]; then
    git mv "$f" Formula/
    moved=true
  fi
done

if [ "$moved" = true ]; then
  git config user.email "actions@github.com"
  git config user.name "github-actions"
  git commit -m "Move Homebrew formulas to Formula directory"
  git push
else
  echo "No formulas to move"
fi
