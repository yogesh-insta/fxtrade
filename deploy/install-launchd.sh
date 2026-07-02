#!/bin/bash
# Install fxtrade as a macOS launchd service (auto-start on boot).
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
PLIST_SRC="$ROOT/deploy/com.fxtrade.daemon.plist"
PLIST_DST="$HOME/Library/LaunchAgents/com.fxtrade.daemon.plist"

if [[ ! -f "$ROOT/.credentials" ]]; then
  echo "error: $ROOT/.credentials not found — copy .credentials.example and fill keys first"
  exit 1
fi

mkdir -p "$ROOT/logs"
sed "s|__REPO_ROOT__|$ROOT|g" "$PLIST_SRC" > "$PLIST_DST"

launchctl unload "$PLIST_DST" 2>/dev/null || true
launchctl load "$PLIST_DST"

echo "Installed: $PLIST_DST"
echo "Status:    launchctl list | grep fxtrade"
echo "Logs:      tail -f $ROOT/logs/daemon.stdout.log"
