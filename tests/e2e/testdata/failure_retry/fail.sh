#!/usr/bin/env bash
# Stub override that always fails (simulates a bad Claude run)
set -euo pipefail
TIMESTAMP=$(date -u +"%Y-%m-%dT%H:%M:%SZ")
PURPOSE="${SORTIE_PURPOSE:-step}"
if [[ -n "${SORTIE_E2E_LOG:-}" ]]; then
    ARGV_JOINED=$(printf '%s' "$*" | tr ' ' '|')
    printf '%s\t%s\t%s\t%s\t\n' "$TIMESTAMP" "$PURPOSE" "$(pwd)" "$ARGV_JOINED" >> "$SORTIE_E2E_LOG"
fi
echo "simulated claude failure" >&2
exit 1
