#!/bin/bash
# Сравнивает результаты тестов версий и генерирует Markdown для комментария
# Usage: scripts/format-comment.sh pr.json main.json pr_raw.txt > comment.md

PR_JSON=$1
MAIN_JSON=$2
PR_RAW=${3:-}

# Получаем отсортированные списки
PR_LIST=$(jq -r '.versions[]' "$PR_JSON" | sort)
MAIN_LIST=$(jq -r '.versions[]' "$MAIN_JSON" | sort)

# Новые в PR
NEW=$(comm -23 <(echo "$PR_LIST") <(echo "$MAIN_LIST"))
# Пропавшие в PR
MISSING=$(comm -13 <(echo "$PR_LIST") <(echo "$MAIN_LIST"))

echo "<!-- tmux-versions-marker -->"
echo ""
echo "### tmux version support test"
echo ""

if [ -n "$NEW" ]; then
    echo "**⚠️ New versions supported in this PR:**"
    echo ""
    for v in $NEW; do echo "- \`$v\`"; done
    echo ""
fi

if [ -n "$MISSING" ]; then
    echo "**⚠️ Versions no longer supported:**"
    echo ""
    for v in $MISSING; do echo "- \`$v\`"; done
    echo ""
fi

echo "<details>"
echo "<summary>Full test output</summary>"
echo ""
echo "\`\`\`"
if [ -n "$PR_RAW" ] && [ -f "$PR_RAW" ]; then
    # Backstop: even with the build log silenced upstream, never let the embedded
    # output grow past GitHub's comment size limit. Show the tail (the version
    # results live at the end) and flag the truncation.
    MAX_LINES=300
    TOTAL=$(wc -l <"$PR_RAW" | tr -d ' ')
    if [ "$TOTAL" -gt "$MAX_LINES" ]; then
        echo "... (truncated, showing last $MAX_LINES of $TOTAL lines) ..."
        tail -n "$MAX_LINES" "$PR_RAW"
    else
        cat "$PR_RAW"
    fi
else
    echo "Results unavailable"
fi
echo "\`\`\`"
echo "</details>"
echo ""
echo "---"
echo "🤖 Last updated: $(date -u '+%Y-%m-%d %H:%M UTC') | [View run]($GITHUB_SERVER_URL/$GITHUB_REPOSITORY/actions/runs/$GITHUB_RUN_ID)"
