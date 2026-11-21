#!/bin/bash

set -euo pipefail

CACHE_DIR=${QUALITY_GRAPH_REPORT_DIR:-.cache/quality-graph}
REPORT=$CACHE_DIR/tmux-versions.json
RESULTS=$CACHE_DIR/tmux-versions.txt
BASE_RESULTS=$CACHE_DIR/tmux-versions-base.txt
SUMMARY=$CACHE_DIR/tmux-versions.md

mkdir -p "$(dirname "$REPORT")"

status=skipped

if [[ ${GITHUB_EVENT_NAME:-} == pull_request ]]; then
    TMUX_RELEASES_FILE=$(cd "$CACHE_DIR" && pwd)/versions.txt
    export TMUX_RELEASES_FILE
    inventory_rc=0
    scripts/tmux-release-tags.sh >"$TMUX_RELEASES_FILE" || inventory_rc=$?
    head_rc=0
    TMUX_COMPARE_BASE=1 make test-sup-versions >"$RESULTS" || head_rc=$?

    base_dir=$(mktemp -d)
    trap 'rm -rf "$base_dir"' EXIT
    base_sha=$(jq -r '.pull_request.base.sha' "${GITHUB_EVENT_PATH:?}")
    git archive "$base_sha" | tar -x -C "$base_dir"
    # Exercise the base application with the same inventory, test fixture,
    # and exit-code protocol as head, not the old rate-limited driver.
    cp scripts/test-tmux-versions.sh "$base_dir/scripts/"
    cp docker/test-versions-inner.sh "$base_dir/docker/"

    base_rc=0
    TMUX_COMPARE_BASE=1 make -C "$base_dir" test-sup-versions >"$BASE_RESULTS" || base_rc=$?
    scripts/report-tmux-versions.sh compare "$inventory_rc" "$head_rc" "$base_rc"
    exit $?
elif [[ ${GITHUB_EVENT_NAME:-} == workflow_dispatch ]]; then
    if scripts/repeat-tmux-version-tests.sh --self-test && scripts/repeat-tmux-version-tests.sh; then
        status=passed
        echo 'Two complete tmux matrix runs passed.' >"$SUMMARY"
    else
        status=failed
        echo 'Repeated tmux verification failed; see the job log.' >"$SUMMARY"
    fi
else
    printf 'The tmux version matrix runs on pull requests.\n' >"$SUMMARY"
fi

scripts/report-tmux-versions.sh "$status"
