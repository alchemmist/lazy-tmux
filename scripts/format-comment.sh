#!/bin/sh
# Сравнивает результаты тестов версий и генерирует Markdown для комментария
# Usage: scripts/format-comment.sh pr.json main.json pr_raw.txt > comment.md

PR_JSON=$1
MAIN_JSON=$2
PR_RAW=${3:-}

# Используем jq для сравнения
NEW=$(jq -r '
    [ .versions[] | select(. as $v | input_filename as $f | $v | not) ]' \
    "$PR_JSON" 2>/dev/null || jq -r '.versions[]' "$PR_JSON")

MISSING=$(jq -r '
    [ .versions[] | select(. as $v | input_filename as $f | $v | not) ]' \
    "$MAIN_JSON" 2>/dev/null || jq -r '.versions[]' "$MAIN_JSON")

# Простое сравнение массивов
echo "### tmux version support test"
echo ""

# Получаем списки
PR_LIST=$(jq -r '.versions[]' "$PR_JSON" 2>/dev/null)
MAIN_LIST=$(jq -r '.versions[]' "$MAIN_JSON" 2>/dev/null)

# Новые в PR
NEW=$(comm -23 <(echo "$PR_LIST" | sort) <(echo "$MAIN_LIST" | sort))
if [ -n "$NEW" ]; then
    echo "**⚠️ New versions supported in this PR:**"
    echo ""
    for v in $NEW; do echo "- \`$v\`"; done
    echo ""
fi

# Пропавшие в PR
MISSING=$(comm -13 <(echo "$PR_LIST" | sort) <(echo "$MAIN_LIST" | sort))
if [ -n "$MISSING" ]; then
    echo "**⚠️ Versions no longer supported:**"
    echo ""
    for v in $MISSING; do echo "- \`$v\`"; done
    echo ""
fi

# Полный вывод
echo "<details>"
echo "<summary>Full test output</summary>"
echo ""
echo "\`\`\`"
if [ -n "$PR_RAW" ] && [ -f "$PR_RAW" ]; then
    cat "$PR_RAW"
else
    echo "Results unavailable"
fi
echo "\`\`\`"
echo "</details>"