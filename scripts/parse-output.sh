#!/bin/sh
# Парсит вывод make test-sup-versions в JSON
# Usage: make test-sup-versions | scripts/parse-output.sh > results.json

# Извлекаем галочки (✓)
SUPPORTED=$(grep -oP "tmux \S+\s+✓" | awk '{print $2}' | sort -V)

echo "{"
echo "  \"versions\": ["
FIRST=true
for v in $SUPPORTED; do
    if [ "$FIRST" = true ]; then
        FIRST=false
    else
        echo ","
    fi
    printf '    "%s"' "$v"
done
echo ""
echo "  ]"
echo "}"