#!/usr/bin/env bash
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"
CRED="${CREDENTIALS:-.credentials}"
BIN="$ROOT/bin/afl-pulse"
if [[ ! -x "$BIN" ]]; then
  go build -o "$BIN" ./cmd/afl-pulse
fi
exec "$BIN" -credentials "$CRED" "$@"
