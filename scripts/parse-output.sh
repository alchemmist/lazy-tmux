#!/bin/sh
# Parses the output of `make test-sup-versions` into JSON.
# Usage: make test-sup-versions | scripts/parse-output.sh > results.json

# Extract the supported versions (rows marked with a checkmark).
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