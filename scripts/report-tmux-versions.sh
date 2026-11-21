#!/bin/bash
set -euo pipefail
CACHE_DIR=${QUALITY_GRAPH_REPORT_DIR:-.cache/quality-graph}
REPORT=$CACHE_DIR/tmux-versions.json
SUMMARY=$CACHE_DIR/tmux-versions.md
status=$1
failure_kind=
exit_code=0
if [[ $status == compare ]]; then
    inventory_rc=$2
    head_rc=$3
    base_rc=$4
    RESULTS=$CACHE_DIR/tmux-versions.txt
    BASE_RESULTS=$CACHE_DIR/tmux-versions-base.txt
    TMUX_RELEASES_FILE=$CACHE_DIR/versions.txt
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
elif [[ $status == failed ]]; then
    failure_kind='command'
    exit_code=1
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
