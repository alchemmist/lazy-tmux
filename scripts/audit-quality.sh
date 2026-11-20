#!/usr/bin/env bash
# Commands return nonzero both on findings and when a scanner cannot finish.
set -euo pipefail
root=$(git rev-parse --show-toplevel)
cd "$root"
case ${1:-} in
    go)
        go mod verify
        # Keep the scanner outside the project's module graph. Test both sets of
        # reachable symbols: a clean default build says nothing about lazy_fzf.
        tmp=$(mktemp -d); trap 'rm -rf "$tmp"' EXIT
        GOBIN="$tmp" go install golang.org/x/vuln/cmd/govulncheck@v1.10.0
        "$tmp/govulncheck" ./...
        "$tmp/govulncheck" -tags lazy_fzf ./...
        ;;
    npm)
        npm --prefix docs audit --package-lock-only --audit-level=high
        ;;
    secrets)
        tmp=$(mktemp -d); trap 'rm -rf "$tmp"' EXIT
        case $(uname -sm) in
            'Linux x86_64') target=linux_x64; sha=551f6fc83ea457d62a0d98237cbad105af8d557003051f41f3e7ca7b3f2470eb ;;
            'Darwin arm64') target=darwin_arm64; sha=b40ab0ae55c505963e365f271a8d3846efbc170aa17f2607f13df610a9aeb6a5 ;;
            *) echo 'Run the secrets audit on Linux x86_64 or Darwin arm64' >&2; exit 2 ;;
        esac
        curl -fsSL --retry 2 "https://github.com/gitleaks/gitleaks/releases/download/v8.30.1/gitleaks_8.30.1_${target}.tar.gz" -o "$tmp/archive"
        (cd "$tmp" && printf '%s  archive\n' "$sha" | shasum -a 256 -c - && tar -xzf archive gitleaks)
        # Scan the tracked tree, including pre-existing leaks, but not caches,
        # .git or ignored local files. Never emit secret values to logs.
        mkdir "$tmp/src"
        git archive HEAD | tar -x -C "$tmp/src"
        "$tmp/gitleaks" dir --no-banner --redact=100 "$tmp/src"
        ;;
    *) echo 'usage: audit-quality.sh go|npm|secrets' >&2; exit 2 ;;
esac
