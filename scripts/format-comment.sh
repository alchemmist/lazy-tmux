#!/bin/sh
# Сравнивает результаты тестов версий и генерирует Markdown для комментария
# Usage: scripts/format-comment.sh pr.json main.json > comment.md

PR_JSON=$1
MAIN_JSON=$2

# Используем jq для сравнения
DIFF=$(jq -r -n --slurpfile pr "$PR_JSON" --slurpfile main "$MAIN_JSON" '
    ($pr[0].versions | map(select(. as $v | $main[0].versions | index($v) | not))) as $new,
    ($main[0].versions | map(select(. as $v | $pr[0].versions | index($v) | not))) as $missing |
    {
        new: $new,
        missing: $missing
    }
')

NEW=$(echo "$DIFF" | jq -r '.new[]')
MISSING=$(echo "$DIFF" | jq -r '.missing[]')

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
cat pr_results_raw.txt 2>/dev/null || echo "Results unavailable"
echo "\`\`\`"
echo "</details>"