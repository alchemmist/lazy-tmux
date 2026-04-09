#!/bin/sh

IMAGE="lazy-tmux:version-test"

printf "\nBuilding test image (once)...\n"
docker build -t "$IMAGE" -f docker/sandbox.Dockerfile . >/dev/null

printf "Fetching tmux releases from GitHub...\n"
versions=$(curl -sf "https://api.github.com/repos/tmux/tmux/releases?per_page=100" \
    | grep -o '"tag_name": *"[^"]*"' \
    | sed 's/"tag_name": *"//;s/"//g' \
    | sort -V)

if [ -z "$versions" ]; then
    printf "ERROR: failed to fetch tmux releases from GitHub API (rate limit or network error)\n" >&2
    exit 1
fi

# Фильтруем версии >= 2.9 и собираем в одну строку (пробел как разделитель)
filtered=""
for v in $versions; do
    major=$(echo "$v" | cut -d. -f1)
    minor=$(echo "$v" | cut -d. -f2 | sed 's/[^0-9]//g')
    minor=${minor:-0}
    if [ "$major" -ge 2 ] && { [ "$major" -gt 2 ] || [ "$minor" -ge 9 ]; }; then
        filtered="$filtered $v"
    fi
done

printf "\ntmux version support:\n\n"

# Запускаем один контейнер, передаём версии как аргументы
# shellcheck disable=SC2086  # intentional word-splitting: each version is a separate arg
docker run --rm --entrypoint "" "$IMAGE" /bin/sh /test-versions-inner.sh $filtered

printf "\n"
