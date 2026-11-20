#!/usr/bin/env bash
# Run the tmux version matrix more than once and require the same full pass.
#
# scripts/test-tmux-versions.sh prints one "  tmux <version>  ✓|✗" line per
# supported upstream release.
# This wrapper treats a non-zero exit, a missing version, a ✗, a duplicate,
# or a difference between repeats as failure. When the default matrix script
# is used, the reported versions must also match a fresh GitHub release list
# filtered the same way (tmux >= 2.9, no pre-releases).
#
# No packages are published. Override TMUX_VERSION_TEST_CMD only for the
# harness self-test.

set -euo pipefail

script=$(cd -- "$(dirname -- "$0")" >/dev/null && pwd)/$(basename -- "$0")
root=$(cd -- "$(dirname -- "$script")/.." >/dev/null && pwd)
cd "$root"

default_cmd=$root/scripts/test-tmux-versions.sh

if [[ ${1:-} == "--self-test" ]]; then
    if [[ $# -ne 1 ]]; then
        printf 'usage: %s [--self-test]\n' "$script" >&2
        exit 2
    fi
else
    if [[ $# -ne 0 ]]; then
        printf 'usage: %s [--self-test]\n' "$script" >&2
        exit 2
    fi
fi

fail() {
    printf '%s\n' "$1" >&2
    exit 1
}

normalize_results() {
    local log=$1 dest=$2
    awk '
        /^  tmux / {
            version = $2
            status = $3
            if (version !~ /^[0-9]/ || (status != "✓" && status != "✗")) {
                printf "unparseable tmux result line: %s\n", $0 > "/dev/stderr"
                exit 2
            }
            printf "%s %s\n", version, status
        }
    ' "$log" | LC_ALL=C sort >"$dest"
}

fetch_supported_tmux_versions() {
    local payload attempt filtered
    attempt=1
    while [[ $attempt -le 3 ]]; do
        if payload=$(curl -sf "https://api.github.com/repos/tmux/tmux/releases?per_page=100"); then
            break
        fi
        payload=
        if [[ $attempt -eq 3 ]]; then
            fail "failed to fetch tmux releases from GitHub"
        fi
        attempt=$((attempt + 1))
        sleep 2
    done
    if [[ -z $payload ]]; then
        fail "failed to fetch tmux releases from GitHub"
    fi

    filtered=$(printf '%s\n' "$payload" \
        | grep -o '"tag_name": *"[^"]*"' \
        | sed 's/"tag_name": *"//;s/"//g' \
        | sort -V \
        | while IFS= read -r version; do
            case $version in
                *-*) continue ;;
            esac
            major=$(printf '%s\n' "$version" | cut -d. -f1)
            minor=$(printf '%s\n' "$version" | cut -d. -f2 | sed 's/[^0-9]//g')
            minor=${minor:-0}
            if [[ $major -ge 2 ]] && { [[ $major -gt 2 ]] || [[ $minor -ge 9 ]]; }; then
                printf '%s\n' "$version"
            fi
        done) || fail "could not parse tmux releases from GitHub"
    if [[ -z $filtered ]]; then
        fail "upstream tmux release list was empty"
    fi
    printf '%s\n' "$filtered"
}

run_repeats() {
    local repeats=${TMUX_TEST_REPEATS:-2}
    local cmd=${TMUX_VERSION_TEST_CMD:-$default_cmd}
    local expect_upstream=${TMUX_EXPECT_UPSTREAM:-auto}
    local work log errlog i rc

    case $repeats in
        2|3|4|5) ;;
        *)
            printf 'TMUX_TEST_REPEATS must be 2, 3, 4, or 5\n' >&2
            exit 2
            ;;
    esac

    if [[ $expect_upstream == auto ]]; then
        if [[ $cmd == "$default_cmd" ]]; then
            expect_upstream=1
        else
            expect_upstream=0
        fi
    fi
    case $expect_upstream in
        0|1) ;;
        *)
            printf 'TMUX_EXPECT_UPSTREAM must be auto, 0, or 1\n' >&2
            exit 2
            ;;
    esac

    work=$(mktemp -d)
    trap 'rm -rf "'"$work"'"' RETURN

    if [[ $expect_upstream == 1 ]]; then
        fetch_supported_tmux_versions | LC_ALL=C sort >"$work/expected-before.txt"
        if [[ ! -s $work/expected-before.txt ]]; then
            fail "upstream tmux release list was empty"
        fi
    fi

    i=1
    while [[ $i -le $repeats ]]; do
        log=$work/run-$i.log
        errlog=$work/run-$i.err
        printf '==> tmux version test run %s/%s\n' "$i" "$repeats"
        set +e
        bash -c "$cmd" >"$log" 2>"$errlog"
        rc=$?
        set -e
        if [[ $rc -ne 0 ]]; then
            printf 'tmux version test run %s exited %s\n' "$i" "$rc" >&2
            cat "$errlog" >&2 || true
            cat "$log" >&2 || true
            exit 1
        fi
        cat "$log"
        if [[ -s $errlog ]]; then
            cat "$errlog" >&2
        fi

        if ! normalize_results "$log" "$work/run-$i.txt"; then
            fail "could not parse tmux version results from run $i"
        fi
        if [[ ! -s $work/run-$i.txt ]]; then
            fail "run $i produced no tmux version results"
        fi
        if awk '{ print $1 }' "$work/run-$i.txt" | LC_ALL=C uniq -d | grep -q .; then
            fail "run $i reported a duplicate tmux version"
        fi
        if grep -q ' ✗$' "$work/run-$i.txt"; then
            printf 'run %s reported an unsupported tmux version:\n' "$i" >&2
            cat "$work/run-$i.txt" >&2
            exit 1
        fi
        if [[ $i -gt 1 ]] && ! cmp -s "$work/run-1.txt" "$work/run-$i.txt"; then
            printf 'tmux version results differ between run 1 and run %s\n' "$i" >&2
            diff -u "$work/run-1.txt" "$work/run-$i.txt" >&2 || true
            exit 1
        fi
        i=$((i + 1))
    done

    if [[ $expect_upstream == 1 ]]; then
        fetch_supported_tmux_versions | LC_ALL=C sort >"$work/expected-after.txt"
        if ! cmp -s "$work/expected-before.txt" "$work/expected-after.txt"; then
            fail "tmux release list changed while the repeated tests ran"
        fi
        awk '{ print $1 }' "$work/run-1.txt" | LC_ALL=C sort >"$work/got.txt"
        if ! cmp -s "$work/expected-before.txt" "$work/got.txt"; then
            printf 'tmux version results do not match the upstream release list\n' >&2
            diff -u "$work/expected-before.txt" "$work/got.txt" >&2 || true
            exit 1
        fi
    fi

    printf 'Repeated tmux version tests passed (%s identical runs)\n' "$repeats"
}

self_test() {
    local mark mock out err rc

    run_case() {
        local name=$1 expect=$2 needle=$3
        shift 3
        out=$(mktemp)
        err=$(mktemp)
        set +e
        env "$@" "$script" >"$out" 2>"$err"
        rc=$?
        set -e
        if [[ $expect == ok && $rc -ne 0 ]]; then
            printf 'self-test %s: expected success, exited %s\n' "$name" "$rc" >&2
            cat "$err" >&2 || true
            cat "$out" >&2 || true
            exit 1
        fi
        if [[ $expect == fail && $rc -eq 0 ]]; then
            printf 'self-test %s: expected failure\n' "$name" >&2
            cat "$out" >&2 || true
            exit 1
        fi
        if [[ -n $needle ]] && ! grep -q -- "$needle" "$out" && ! grep -q -- "$needle" "$err"; then
            printf 'self-test %s: missing [%s]\n' "$name" "$needle" >&2
            cat "$err" >&2 || true
            cat "$out" >&2 || true
            exit 1
        fi
        rm -f "$out" "$err"
    }

    run_case stable ok "Repeated tmux version tests passed" \
        TMUX_TEST_REPEATS=2 \
        TMUX_EXPECT_UPSTREAM=0 \
        TMUX_VERSION_TEST_CMD='printf "  tmux 3.5a    ✓\n  tmux 3.6a    ✓\n"'

    mark=$(mktemp)
    rm -f "$mark"
    run_case diverge fail "differ" \
        TMUX_TEST_REPEATS=2 \
        TMUX_EXPECT_UPSTREAM=0 \
        TMUX_MARK="$mark" \
        TMUX_VERSION_TEST_CMD="if [ -f \"\$TMUX_MARK\" ]; then printf \"  tmux 3.6a    ✓\n\"; else touch \"\$TMUX_MARK\"; printf \"  tmux 3.5a    ✓\n\"; fi"
    rm -f "$mark"

    run_case nonzero fail "exited 1" \
        TMUX_TEST_REPEATS=2 \
        TMUX_EXPECT_UPSTREAM=0 \
        TMUX_VERSION_TEST_CMD='printf "  tmux 3.6a    ✓\n"; exit 1'

    run_case unsupported fail "unsupported" \
        TMUX_TEST_REPEATS=2 \
        TMUX_EXPECT_UPSTREAM=0 \
        TMUX_VERSION_TEST_CMD='printf "  tmux 3.6a    ✗\n"'

    run_case empty fail "no tmux version results" \
        TMUX_TEST_REPEATS=2 \
        TMUX_EXPECT_UPSTREAM=0 \
        TMUX_VERSION_TEST_CMD='printf "tmux version support:\n"'

    run_case duplicate fail "duplicate" \
        TMUX_TEST_REPEATS=2 \
        TMUX_EXPECT_UPSTREAM=0 \
        TMUX_VERSION_TEST_CMD='printf "  tmux 3.6a    ✓\n  tmux 3.6a    ✓\n"'

    mock=$(mktemp -d)
    cat >"$mock/docker" <<'EOF'
#!/bin/sh
if [ "$1" = build ]; then
    exit 0
fi
if [ "$1" = run ]; then
    for arg in "$@"; do
        case $arg in
            [0-9]*) printf '  tmux %-7s  ✓\n' "$arg" ;;
        esac
    done
    exit 0
fi
printf 'unexpected docker invocation: %s\n' "$*" >&2
exit 1
EOF
    cat >"$mock/curl" <<'EOF'
#!/bin/sh
printf '%s\n' '[{"tag_name":"3.7-rc"},{"tag_name":"2.8"},{"tag_name":"2.9"},{"tag_name":"3.6a"}]'
EOF
    chmod +x "$mock/docker" "$mock/curl"

    out=$(mktemp)
    err=$(mktemp)
    set +e
    env PATH="$mock:$PATH" TMUX_TEST_REPEATS=2 TMUX_EXPECT_UPSTREAM=1 "$script" >"$out" 2>"$err"
    rc=$?
    set -e
    if [[ $rc -ne 0 ]]; then
        printf 'self-test upstream wiring: expected success, exited %s\n' "$rc" >&2
        cat "$err" >&2 || true
        cat "$out" >&2 || true
        exit 1
    fi
    if ! grep -q "Repeated tmux version tests passed" "$out" \
        || ! grep -q "tmux 2.9" "$out" \
        || ! grep -q "tmux 3.6a" "$out" \
        || grep -q "2.8" "$out" \
        || grep -q "3.7-rc" "$out"; then
        printf 'self-test upstream wiring: unexpected version set\n' >&2
        cat "$out" >&2 || true
        cat "$err" >&2 || true
        exit 1
    fi
    rm -f "$out" "$err"
    rm -rf "$mock"

    printf 'repeat-tmux-version-tests self-test passed\n'
}

if [[ ${1:-} == "--self-test" ]]; then
    self_test
    exit 0
fi

run_repeats
