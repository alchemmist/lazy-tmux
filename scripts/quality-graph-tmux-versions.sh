#!/bin/bash

set -euo pipefail

CACHE_DIR=${QUALITY_GRAPH_REPORT_DIR:-.cache/quality-graph}
REPORT=$CACHE_DIR/tmux-versions.json
RESULTS=$CACHE_DIR/tmux-versions.txt
SUMMARY=$CACHE_DIR/tmux-versions.md

mkdir -p "$(dirname "$REPORT")"

status=skipped
failure_kind=
exit_code=0

if [[ ${GITHUB_EVENT_NAME:-} == pull_request ]]; then
    set +e
    make test-sup-versions | tee "$RESULTS"
    exit_code=${PIPESTATUS[0]}
    set -e

    {
        printf '| tmux version | Status |\n'
        printf '| --- | --- |\n'
        awk '$1 == "tmux" { printf "| `%s` | %s |\n", $2, $3 }' "$RESULTS"
    } >"$SUMMARY"

    if [[ $exit_code -eq 0 ]]; then
        status=passed
    else
        status=failed
        failure_kind=quality
    fi
else
    printf 'The tmux version matrix runs on pull requests.\n' >"$SUMMARY"
fi

pull_request=$(jq -r '.pull_request.number // empty' "${GITHUB_EVENT_PATH:?}")
graph_digest=$(jq -r '.graphDigest' .quality-graph/manifest.json)

jq -n \
    --arg node_id tmux-versions \
    --arg title 'Tmux version test' \
    --arg status "$status" \
    --arg failure_kind "$failure_kind" \
    --rawfile summary "$SUMMARY" \
    --arg repository "${GITHUB_REPOSITORY:?}" \
    --arg pull_request "$pull_request" \
    --arg head_sha "${GITHUB_SHA:?}" \
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
