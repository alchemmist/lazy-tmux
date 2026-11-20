#!/usr/bin/env bash
# Build a goreleaser snapshot of both binaries and check the archives.
#
# The TUI build is archive id "default" (lazy-tmux_<os>_<arch>.tar.gz). The
# fzf build is archive id "fzf" (the same name with an _fzf suffix). Both are
# produced by one snapshot so checksums.txt matches a real release layout.
#
# Snapshot mode does not publish. Publish, announce, and every external
# packager are skipped, and GitHub/Homebrew/AUR tokens are removed from the
# environment. The script fails if HEAD, tags, or the worktree status change.

set -euo pipefail

root=$(cd -- "$(dirname -- "$0")/.." >/dev/null && pwd)
cd "$root"

if [[ $# -ne 0 ]]; then
    printf 'usage: %s\n' "$0" >&2
    exit 2
fi

fail() {
    printf '%s\n' "$1" >&2
    exit 1
}

sha256_file() {
    if command -v sha256sum >/dev/null 2>&1; then
        sha256sum "$1" | awk '{ print tolower($1) }'
    else
        shasum -a 256 "$1" | awk '{ print tolower($1) }'
    fi
}

goreleaser_target() {
    local os arch
    os=$(uname -s)
    arch=$(uname -m)
    case $os in
        Darwin|Linux) ;;
        *) fail "unsupported OS for pinned goreleaser: $os" ;;
    esac
    case $arch in
        x86_64) arch=x86_64 ;;
        arm64|aarch64) arch=arm64 ;;
        *) fail "unsupported architecture for pinned goreleaser: $arch" ;;
    esac
    printf '%s_%s\n' "$os" "$arch"
}

goreleaser_sha256() {
    case $1 in
        Darwin_arm64) printf '%s\n' 'c1423474ab35ab6d7669d87d403dc9ddebf6013a11d3de7309ba840765a19af1' ;;
        Darwin_x86_64) printf '%s\n' '8c7ac7f542f3048b2bd87bb49e2f189174087296beee99aad132d58e49660973' ;;
        Linux_arm64) printf '%s\n' 'cfce0afc7bcc14447df211cc10c0012cfc395646070bcf23e840b16cbce6d204' ;;
        Linux_x86_64) printf '%s\n' 'a36d5d7d07a5e538e052cef2a9f93f1cd4af493cf5a3712b9177c1ebe360f0e7' ;;
        *) fail "no pinned goreleaser checksum for $1" ;;
    esac
}

install_goreleaser() {
    local version=2.20.3 target url archive got want dest
    target=$(goreleaser_target)
    dest=$root/.cache/goreleaser/v$version/$target
    if [[ -x $dest/goreleaser ]] && "$dest/goreleaser" --version 2>/dev/null | grep -q "GitVersion:[[:space:]]*$version"; then
        printf '%s\n' "$dest/goreleaser"
        return
    fi
    mkdir -p "$dest"
    archive=$(mktemp)
    url=https://github.com/goreleaser/goreleaser/releases/download/v$version/goreleaser_$target.tar.gz
    curl -fsSL --retry 3 --retry-delay 2 "$url" -o "$archive"
    got=$(sha256_file "$archive")
    want=$(goreleaser_sha256 "$target")
    if [[ $got != "$want" ]]; then
        rm -f "$archive"
        fail "goreleaser checksum mismatch for $target"
    fi
    tar -xzf "$archive" -C "$dest" goreleaser
    rm -f "$archive"
    chmod +x "$dest/goreleaser"
    printf '%s\n' "$dest/goreleaser"
}

archive_names() {
    local raw
    raw=$(tar -tzf "$1" | sed -e 's#^\./##' -e 's#/$##' | grep -v '^$' | LC_ALL=C sort)
    if [[ -z $raw ]]; then
        fail "archive is empty: $1"
    fi
    if ! printf '%s\n' "$raw" | grep -q -v '/'; then
        printf '%s\n' "$raw" | awk -F/ 'NR == 1 { prefix = $1 } $1 != prefix { exit 1 }' \
            || fail "archive $1 is not a flat layout or a single directory"
        raw=$(printf '%s\n' "$raw" | cut -d/ -f2- | LC_ALL=C sort)
    fi
    printf '%s\n' "$raw"
}

executable_in_archive() {
    local mode
    mode=$(tar -tvf "$1" | awk '{ print $1, $NF }' | awk '$2 ~ /(^|\/)lazy-tmux$/ { print $1; exit }')
    case $mode in
        *x*) ;;
        *) fail "lazy-tmux is not executable in $1 (mode: ${mode:-missing})" ;;
    esac
}

main() {
    local goreleaser dist log head_before tags_before status_before module_backup other_dirty
    local names expected listed sum file base actual info version
    local os arch variant archive tmp bin hash host_os host_arch
    local tui_hash fzf_hash version_out help_out rc

    command -v go >/dev/null 2>&1 || fail "go is required to inspect snapshot binaries"
    command -v git >/dev/null 2>&1 || fail "git is required"
    command -v curl >/dev/null 2>&1 || fail "curl is required"

    goreleaser=$(install_goreleaser)
    dist=$root/dist
    log=$(mktemp)
    module_backup=$(mktemp -d)
    cp go.mod go.sum "$module_backup/"
    head_before=$(git rev-parse HEAD)
    tags_before=$(git tag --list | LC_ALL=C sort)
    status_before=$(git status --porcelain)

    rm -rf "$dist"
    printf '==> goreleaser snapshot (no publish)\n'
    set +e
    env \
        -u GITHUB_TOKEN \
        -u GH_TOKEN \
        -u GITHUB_API_TOKEN \
        -u HOMEBREW_TAP_GITHUB_TOKEN \
        -u AUR_SSH_PRIVATE_KEY \
        -u AUR_REPO_URL \
        GIT_TERMINAL_PROMPT=0 \
        "$goreleaser" release --snapshot --clean \
        --skip=announce,publish,validate,sign,notarize,aur,aur-source,homebrew,scoop,chocolatey,docker,snapcraft,winget,nfpm,ko,nix,sbom,flatpak,iru,makeself,mcp,srpm \
        >"$log" 2>&1
    rc=$?
    set -e
    # The release config runs `go mod tidy` before building. That may edit
    # go.mod or go.sum. Any other tracked change is a failure. Restore the
    # module files either way so verification does not keep the side effect.
    other_dirty=0
    if ! git diff --quiet -- . ':(exclude)go.mod' ':(exclude)go.sum' || ! git diff --cached --quiet; then
        other_dirty=1
        git diff --stat -- . ':(exclude)go.mod' ':(exclude)go.sum' >&2 || true
    fi
    cp "$module_backup/go.mod" go.mod
    cp "$module_backup/go.sum" go.sum
    rm -rf "$module_backup"
    if [[ $rc -ne 0 ]]; then
        cat "$log" >&2
        fail "goreleaser snapshot exited $rc"
    fi
    if [[ $other_dirty -ne 0 ]]; then
        fail "snapshot packaging modified tracked files other than go.mod/go.sum"
    fi
    if ! grep -q -i snapshot "$log"; then
        cat "$log" >&2
        fail "goreleaser log does not mention snapshot mode"
    fi
    if grep -E -i 'published successfully|release created|git push|uploading to|brew tap|pushed to' "$log"; then
        fail "snapshot command produced publication output"
    fi

    if [[ $(git rev-parse HEAD) != "$head_before" ]]; then
        fail "snapshot packaging moved HEAD"
    fi
    if [[ $(git tag --list | LC_ALL=C sort) != "$tags_before" ]]; then
        fail "snapshot packaging changed git tags"
    fi
    if [[ $(git status --porcelain) != "$status_before" ]]; then
        git status --porcelain >&2
        fail "snapshot packaging changed the worktree"
    fi

    expected=$(mktemp)
    for os in darwin linux; do
        for arch in amd64 arm64; do
            printf 'lazy-tmux_%s_%s.tar.gz\n' "$os" "$arch"
            printf 'lazy-tmux_%s_%s_fzf.tar.gz\n' "$os" "$arch"
        done
    done | LC_ALL=C sort >"$expected"

    find "$dist" -type f -name 'lazy-tmux_*.tar.gz' -exec basename {} \; | LC_ALL=C sort >"$dist/found-archives.txt"
    if ! cmp -s "$expected" "$dist/found-archives.txt"; then
        diff -u "$expected" "$dist/found-archives.txt" >&2 || true
        fail "snapshot archives do not match the TUI and fzf matrices"
    fi

    if [[ ! -f $dist/checksums.txt ]]; then
        fail "missing $dist/checksums.txt"
    fi
    listed=$(mktemp)
    : >"$listed"
    while read -r sum file; do
        if [[ -z ${sum:-} || $sum == \#* ]]; then
            continue
        fi
        file=${file#\*}
        base=$(basename "$file")
        [[ -f $dist/$base ]] || fail "checksum entry does not exist: $base"
        if [[ ! $sum =~ ^[0-9a-fA-F]{64}$ ]]; then
            fail "checksum for $base is not sha256"
        fi
        actual=$(sha256_file "$dist/$base")
        if [[ $actual != "$(printf '%s' "$sum" | tr '[:upper:]' '[:lower:]')" ]]; then
            fail "checksum mismatch for $base"
        fi
        printf '%s\n' "$base" >>"$listed"
    done <"$dist/checksums.txt"
    LC_ALL=C sort -o "$listed" "$listed"
    if ! cmp -s "$expected" "$listed"; then
        diff -u "$expected" "$listed" >&2 || true
        fail "checksums.txt does not list exactly the TUI and fzf archives"
    fi

    if [[ ! -f $dist/artifacts.json ]]; then
        fail "missing $dist/artifacts.json"
    fi
    grep -E -q '"ID"[[:space:]]*:[[:space:]]*"lazy-tmux"[,}]' "$dist/artifacts.json" \
        || fail "artifacts.json is missing the TUI build id"
    grep -E -q '"ID"[[:space:]]*:[[:space:]]*"lazy-tmux-fzf"[,}]' "$dist/artifacts.json" \
        || fail "artifacts.json is missing the fzf build id"

    host_os=$(uname -s | tr '[:upper:]' '[:lower:]')
    case $(uname -m) in
        x86_64|amd64) host_arch=amd64 ;;
        arm64|aarch64) host_arch=arm64 ;;
        *) fail "unsupported host architecture $(uname -m)" ;;
    esac

    version=
    while IFS= read -r base; do
        case $base in
            *_fzf.tar.gz) variant=fzf ;;
            *) variant=tui ;;
        esac
        os=${base#lazy-tmux_}
        os=${os%.tar.gz}
        os=${os%_fzf}
        arch=${os#*_}
        os=${os%%_*}

        archive=$dist/$base
        names=$(archive_names "$archive")
        if [[ $names != $'LICENSE\nREADME.md\nlazy-tmux' ]]; then
            printf 'archive %s contains:\n%s\n' "$base" "$names" >&2
            fail "archive $base does not contain exactly LICENSE, README.md, and lazy-tmux"
        fi
        executable_in_archive "$archive"

        tmp=$(mktemp -d)
        tar -xzf "$archive" -C "$tmp"
        bin=$(find "$tmp" -type f -name lazy-tmux)
        [[ -n $bin && $(printf '%s\n' "$bin" | wc -l | tr -d ' ') -eq 1 ]] || fail "expected one lazy-tmux binary in $base"
        [[ -x $bin ]] || fail "extracted lazy-tmux is not executable in $base"

        info=$(go version -m "$bin")
        printf '%s\n' "$info" | grep -q 'path[[:space:]]*github.com/alchemmist/lazy-tmux/cmd/lazy-tmux' \
            || fail "$base build info is missing the lazy-tmux module path"
        printf '%s\n' "$info" | grep -q "GOOS=$os" || fail "$base build info GOOS is not $os"
        printf '%s\n' "$info" | grep -q "GOARCH=$arch" || fail "$base build info GOARCH is not $arch"
        if [[ $variant == fzf ]]; then
            printf '%s\n' "$info" | grep -q 'lazy_fzf' || fail "$base is not the fzf build"
        elif printf '%s\n' "$info" | grep -q 'lazy_fzf'; then
            fail "$base TUI build contains the lazy_fzf tag"
        fi
        hash=$(printf '%s\n' "$info" | sed -n 's/.*main.version=\([^" ]*\).*/\1/p' | head -n 1)
        [[ $hash =~ ^[0-9]+\.[0-9]+\.[0-9]+- ]] || fail "$base version is not a snapshot version: ${hash:-missing}"
        if [[ -z $version ]]; then
            version=$hash
        elif [[ $hash != "$version" ]]; then
            fail "$base version $hash does not match $version"
        fi

        actual=$(sha256_file "$bin")
        printf '%s\n' "$actual" >"$dist/$variant-$os-$arch.binary.sha"

        if [[ $os == "$host_os" && $arch == "$host_arch" ]]; then
            version_out=$("$bin" --version)
            printf '%s\n' "$version_out" | grep -q "^lazy-tmux $version ($os/$arch," \
                || fail "$base --version was: $version_out"
            version_out=$("$bin" version)
            printf '%s\n' "$version_out" | grep -q "^lazy-tmux $version ($os/$arch," \
                || fail "$base version command was: $version_out"
            help_out=$("$bin" --help)
            printf '%s\n' "$help_out" | grep -q 'lazy-tmux <command>' \
                || fail "$base --help did not print usage"
        fi
        rm -rf "$tmp"
    done <"$expected"

    for os in darwin linux; do
        for arch in amd64 arm64; do
            tui_hash=$(cat "$dist/tui-$os-$arch.binary.sha")
            fzf_hash=$(cat "$dist/fzf-$os-$arch.binary.sha")
            [[ -n $tui_hash && -n $fzf_hash && $tui_hash != "$fzf_hash" ]] \
                || fail "TUI and fzf binaries for $os/$arch are identical"
        done
    done

    rm -rf "$dist"
    rm -f "$log" "$expected" "$listed"
    printf 'Snapshot packages verified for TUI and fzf (%s)\n' "$version"
}

main
