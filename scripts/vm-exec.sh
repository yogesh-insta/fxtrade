#!/usr/bin/env bash
# Trigger VM exec via GitHub Actions (Option A — no direct SSH from agent).
# Usage:
#   ./scripts/vm-exec.sh daily-email-print
#   ./scripts/vm-exec.sh custom 'bin/bot-metrics -bot universe_scanner'
set -euo pipefail

CMD="${1:?usage: $0 <command> [custom_command]}"
CUSTOM="${2:-}"

if ! command -v gh >/dev/null 2>&1; then
  echo "error: gh CLI required" >&2
  exit 1
fi

ARGS=(-f "command=$CMD")
if [[ "$CMD" == "custom" ]]; then
  [[ -n "$CUSTOM" ]] || { echo "error: custom command required" >&2; exit 1; }
  ARGS+=(-f "custom_command=$CUSTOM")
fi

echo "Dispatching VM exec workflow: command=$CMD"
gh workflow run vm-exec.yml "${ARGS[@]}"

# Wait for the new run to appear, then watch it.
for _ in $(seq 1 30); do
  sleep 2
  RUN_ID=$(gh run list --workflow=vm-exec.yml --limit 1 --json databaseId,status -q '.[0] | select(.status != "completed") | .databaseId' 2>/dev/null || true)
  if [[ -n "${RUN_ID:-}" ]]; then
    break
  fi
  RUN_ID=$(gh run list --workflow=vm-exec.yml --limit 1 --json databaseId -q '.[0].databaseId')
  STATUS=$(gh run list --workflow=vm-exec.yml --limit 1 --json status -q '.[0].status')
  [[ "$STATUS" != "completed" ]] && break
done

RUN_ID=$(gh run list --workflow=vm-exec.yml --limit 1 --json databaseId -q '.[0].databaseId')
echo "Run: https://github.com/$(gh repo view --json nameWithOwner -q .nameWithOwner)/actions/runs/$RUN_ID"
gh run watch "$RUN_ID" --exit-status

echo ""
echo "── output ──"
gh run view "$RUN_ID" --log 2>&1 | sed -n '/======== vm-exec:/,/======== end vm-exec ========/p' \
  | sed 's/^[^[:space:]]*[[:space:]][^[:space:]]*[[:space:]][^[:space:]]*[[:space:]]//' \
  | grep -v '^========' || gh run view "$RUN_ID" --log-failed 2>&1 | tail -80
