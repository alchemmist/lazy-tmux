#!/bin/sh

IMAGE="lazy-tmux:version-test"

printf "\nBuilding test image (once)...\n"
docker build -t "$IMAGE" -f docker/sandbox.Dockerfile . >/dev/null 2>&1

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
