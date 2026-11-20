#!/bin/sh

IMAGE="lazy-tmux:version-test"

printf "\nBuilding test image (once)...\n"
# Build the matrix image from docker/version-test.Dockerfile, which compiles
# lazy-tmux from this repo's source (not the published binary). BuildKit writes
# its progress to stderr, so redirecting only stdout (>/dev/null) still leaks
# the entire apt-get/BuildKit transcript into the result stream. Capture the
# whole build log and print it only on failure so the success summary stays
# focused on the version table.
build_log=$(mktemp)
if ! docker build -t "$IMAGE" -f docker/version-test.Dockerfile . >"$build_log" 2>&1; then
    printf "ERROR: failed to build test image:\n" >&2
    cat "$build_log" >&2
    rm -f "$build_log"
    exit 1
fi
rm -f "$build_log"

if [ -n "${TMUX_RELEASES_FILE:-}" ]; then
    versions=$(cat "$TMUX_RELEASES_FILE") || exit 1
else
    versions=$("$(dirname "$0")/tmux-release-tags.sh") || exit 1
fi
[ -n "$versions" ] || { echo 'Empty tmux inventory' >&2; exit 1; }
set --
for version in $versions; do set -- "$@" "$version"; done

printf "\ntmux version support:\n\n"

rc=0
docker run --rm --entrypoint "" "$IMAGE" /bin/sh /test-versions-inner.sh "$@" || rc=$?
if [ "$rc" -eq 42 ] && [ "${TMUX_COMPARE_BASE:-0}" = 1 ]; then
    rc=0
fi

printf "\n"
exit "$rc"
