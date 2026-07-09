#!/usr/bin/env bash
# Run AFLPulse pregame (T-45) on demand. Builds ./bin/afl-pulse-pregame if missing.
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
HOME="${FXTRADE_HOME:-$ROOT}"
BIN="${AFL_PULSE_PREGAME_BIN:-$HOME/bin/afl-pulse-pregame}"
CRED="${AFL_PULSE_CREDENTIALS:-$HOME/.credentials}"

DRY_RUN=false
if [[ "${1:-}" == "--dry-run" ]]; then
  DRY_RUN=true
  shift
fi

if [[ ! -x "$BIN" ]]; then
  mkdir -p "$(dirname "$BIN")"
  echo "Building $BIN ..."
  (cd "$ROOT" && go build -o "$BIN" ./cmd/afl-pulse-pregame)
fi

ARGS=(-credentials "$CRED")
if $DRY_RUN; then
  ARGS+=(-dry-run)
fi

exec "$BIN" "${ARGS[@]}" "$@"
