#!/bin/sh

IMAGE="lazy-tmux:version-test"

printf "\nBuilding test image (once)...\n"
# Build the matrix image from docker/version-test.Dockerfile, which compiles
# lazy-tmux from this repo's source (not the published binary). BuildKit writes
# its progress to stderr, so redirecting only stdout (>/dev/null) still leaked
# the entire apt-get/BuildKit transcript into this script's output — which the CI
# workflow embeds verbatim into the PR comment, blowing past GitHub's size limit.
# Capture the whole build log to a file and print it only on failure, so a broken
# build still surfaces while the success path stays quiet.
build_log=$(mktemp)
if ! docker build -t "$IMAGE" -f docker/version-test.Dockerfile . >"$build_log" 2>&1; then
    printf "ERROR: failed to build test image:\n" >&2
    cat "$build_log" >&2
    rm -f "$build_log"
    exit 1
fi
rm -f "$build_log"

printf "Fetching tmux releases from GitHub...\n"
versions=$(curl -sf "https://api.github.com/repos/tmux/tmux/releases?per_page=100" \
    | grep -o '"tag_name": *"[^"]*"' \
    | sed 's/"tag_name": *"//;s/"//g' \
    | sort -V)

if [ -z "$versions" ]; then
    printf "ERROR: failed to fetch tmux releases from GitHub API (rate limit or network error)\n" >&2
    exit 1
fi

filtered=""
for v in $versions; do
    # Skip pre-releases / release candidates (e.g. 3.7-rc): their tarballs are
    # not published under this URL scheme, so they only produce noisy ✗ rows.
    case "$v" in
        *-*) continue ;;
    esac
    major=$(echo "$v" | cut -d. -f1)
    minor=$(echo "$v" | cut -d. -f2 | sed 's/[^0-9]//g')
    minor=${minor:-0}
    if [ "$major" -ge 2 ] && { [ "$major" -gt 2 ] || [ "$minor" -ge 9 ]; }; then
        filtered="$filtered $v"
    fi
done

printf "\ntmux version support:\n\n"

docker run --rm --entrypoint "" "$IMAGE" /bin/sh /test-versions-inner.sh $filtered

printf "\n"
