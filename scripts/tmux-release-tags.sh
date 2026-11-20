#!/usr/bin/env bash
# Stable upstream tags, via Git transport rather than runner-IP-limited REST.
set -euo pipefail
git ls-remote --refs --tags https://github.com/tmux/tmux.git |
    awk '{sub(/^refs\/tags\//, "", $2); if ($2 ~ /^v?[0-9]+(\.[0-9]+)+[a-z]*$/ && $2 !~ /^[0-9]+\.[0-9]+[a-z]*$/) {print "Unrecognized stable tag: " $2 > "/dev/stderr"; exit 2} if ($2 ~ /^[0-9]+\.[0-9]+[a-z]*$/) {split($2,v,"."); if (v[1]+0 > 2 || (v[1]+0 == 2 && v[2]+0 >= 9)) print $2}}' |
    LC_ALL=C sort -u
