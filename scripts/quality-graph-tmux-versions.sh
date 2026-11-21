#!/bin/bash

set -euo pipefail

CACHE_DIR=${QUALITY_GRAPH_REPORT_DIR:-.cache/quality-graph}
REPORT=$CACHE_DIR/tmux-versions.json
RESULTS=$CACHE_DIR/tmux-versions.txt
BASE_RESULTS=$CACHE_DIR/tmux-versions-base.txt
SUMMARY=$CACHE_DIR/tmux-versions.md

mkdir -p "$(dirname "$REPORT")"

status=skipped
failure_kind=
exit_code=0

if [[ ${GITHUB_EVENT_NAME:-} == pull_request ]]; then
    TMUX_RELEASES_FILE=$(cd "$CACHE_DIR" && pwd)/versions.txt
    export TMUX_RELEASES_FILE
    inventory_rc=0
    scripts/tmux-release-tags.sh >"$TMUX_RELEASES_FILE" || inventory_rc=$?
    head_rc=0
    TMUX_COMPARE_BASE=1 make test-sup-versions >"$RESULTS" || head_rc=$?

    if [[ -n ${QUALITY_GRAPH_BASE_DIR:-} ]]; then
        base_dir=$QUALITY_GRAPH_BASE_DIR
    else
        base_dir=$(mktemp -d)
        trap 'rm -rf "$base_dir"' EXIT
        base_sha=$(jq -r '.pull_request.base.sha' "${GITHUB_EVENT_PATH:?}")
        git archive "$base_sha" | tar -x -C "$base_dir"
        # Exercise the base application with the same inventory, test fixture,
        # and exit-code protocol as head, not the old rate-limited driver.
        cp scripts/test-tmux-versions.sh "$base_dir/scripts/"
        cp docker/test-versions-inner.sh "$base_dir/docker/"
    fi

    base_rc=0
    TMUX_COMPARE_BASE=1 make -C "$base_dir" test-sup-versions >"$BASE_RESULTS" || base_rc=$?
    matrix_rc=0
    {
        printf '| tmux version | Status |\n| --- | --- |\n'
        awk '/^  tmux / && $1 == "tmux" { printf "| `%s` | %s |\n", $2, $3 }' "$RESULTS"
        if (( inventory_rc )) || [[ ! -s $TMUX_RELEASES_FILE ]]; then
            matrix_rc=2
            echo 'Cannot validate expected releases.'
        elif (( head_rc || base_rc )); then
            matrix_rc=2
            echo 'A matrix command did not complete.'
        else
            python3 scripts/tmux_matrix_check.py "$BASE_RESULTS" "$RESULTS" "$TMUX_RELEASES_FILE" || matrix_rc=$?
        fi
    } >"$SUMMARY"

    if (( matrix_rc > 1 )); then
        status=failed
        failure_kind=infrastructure
        exit_code=1
    elif (( matrix_rc == 1 )); then
        status=failed
        failure_kind=quality
        exit_code=1
    else
        status=passed
    fi
elif [[ ${GITHUB_EVENT_NAME:-} == workflow_dispatch ]]; then
    if scripts/repeat-tmux-version-tests.sh --self-test && scripts/repeat-tmux-version-tests.sh; then
        status=passed
        echo 'Two complete tmux matrix runs passed.' >"$SUMMARY"
    else
        status=failed
        failure_kind='command'
        exit_code=1
        echo 'Repeated tmux verification failed; see the job log.' >"$SUMMARY"
    fi
else
    printf 'The tmux version matrix runs on pull requests.\n' >"$SUMMARY"
fi

pull_request=$(jq -r '.pull_request.number // empty' "${GITHUB_EVENT_PATH:?}")
head_sha=${GITHUB_SHA:?}
if [[ -n $pull_request ]]; then
    event_head_sha=$(jq -r '.pull_request.head.sha // empty' "$GITHUB_EVENT_PATH")
    if [[ -n $event_head_sha ]]; then
        head_sha=$event_head_sha
    fi
fi
graph_digest=$(jq -r '.graphDigest' .quality-graph/manifest.json)

jq -n \
    --arg node_id tmux-versions \
    --arg title 'Tmux version test' \
    --arg status "$status" \
    --arg failure_kind "$failure_kind" \
    --rawfile summary "$SUMMARY" \
    --arg repository "${GITHUB_REPOSITORY:?}" \
    --arg pull_request "$pull_request" \
    --arg head_sha "$head_sha" \
    --argjson workflow_run_id "${GITHUB_RUN_ID:?}" \
    --argjson run_attempt "${GITHUB_RUN_ATTEMPT:?}" \
    --arg graph_digest "$graph_digest" \
    '{
        schemaVersion: 0,
        nodeId: $node_id,
        title: $title,
        status: $status,
        summary: $summary,
        metrics: [],
        findings: [],
        annotations: [],
        diagnostics: [],
        controls: [],
        notes: [],
        provenance: {
            repository: $repository,
            headSha: $head_sha,
            workflowRunId: $workflow_run_id,
            runAttempt: $run_attempt,
            graphDigest: $graph_digest
        }
    }
    | if $failure_kind != "" then .failureKind = $failure_kind else . end
    | if $pull_request != "" then .provenance.pullRequest = ($pull_request | tonumber) else . end' \
    >"$REPORT"

exit "$exit_code"
