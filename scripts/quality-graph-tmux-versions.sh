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
    make test-sup-versions | tee "$RESULTS" || true

    if [[ -n ${QUALITY_GRAPH_BASE_DIR:-} ]]; then
        base_dir=$QUALITY_GRAPH_BASE_DIR
    else
        base_dir=$(mktemp -d)
        trap 'rm -rf "$base_dir"' EXIT
        base_sha=$(jq -r '.pull_request.base.sha' "${GITHUB_EVENT_PATH:?}")
        git archive "$base_sha" | tar -x -C "$base_dir"
    fi

    make -C "$base_dir" test-sup-versions | tee "$BASE_RESULTS" || true

    awk '$1 == "tmux" && $3 == "✓" { print $2 }' "$RESULTS" | LC_ALL=C sort -u >"$CACHE_DIR/head-supported.txt"
    awk '$1 == "tmux" && $3 == "✓" { print $2 }' "$BASE_RESULTS" | LC_ALL=C sort -u >"$CACHE_DIR/base-supported.txt"
    LC_ALL=C comm -23 "$CACHE_DIR/base-supported.txt" "$CACHE_DIR/head-supported.txt" >"$CACHE_DIR/missing.txt"

    {
        printf '| tmux version | Status |\n'
        printf '| --- | --- |\n'
        awk '$1 == "tmux" { printf "| `%s` | %s |\n", $2, $3 }' "$RESULTS"

        if [[ -s $CACHE_DIR/missing.txt ]]; then
            printf '\nVersions supported on the base branch but missing from this change:\n\n'
            sed 's/^/- `/' "$CACHE_DIR/missing.txt" | sed 's/$/`/'
        fi
    } >"$SUMMARY"

    if ! grep -q '^  tmux ' "$RESULTS" || ! grep -q '^  tmux ' "$BASE_RESULTS"; then
        status=failed
        failure_kind=infrastructure
        exit_code=1
    elif [[ -s $CACHE_DIR/missing.txt ]]; then
        status=failed
        failure_kind=quality
        exit_code=1
    else
        status=passed
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
