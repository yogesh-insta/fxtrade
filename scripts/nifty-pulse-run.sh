#!/usr/bin/env bash
# Run NiftyPulse on demand (local dev or VM). Builds ./bin/nifty-pulse if missing.
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
HOME="${FXTRADE_HOME:-$ROOT}"
BIN="${NIFTY_PULSE_BIN:-$HOME/bin/nifty-pulse}"
CRED="${NIFTY_PULSE_CREDENTIALS:-$HOME/.credentials}"
WATCH="${NIFTY_PULSE_WATCHLIST:-$HOME/watchlist.txt}"

DRY_RUN=false
if [[ "${1:-}" == "--dry-run" ]]; then
  DRY_RUN=true
  shift
fi

if [[ ! -x "$BIN" ]]; then
  mkdir -p "$(dirname "$BIN")"
  echo "Building $BIN ..."
  (cd "$ROOT" && go build -o "$BIN" ./cmd/nifty-pulse)
fi

ARGS=(-credentials "$CRED" -watchlist "$WATCH")
if $DRY_RUN; then
  ARGS+=(-dry-run)
fi

exec "$BIN" "${ARGS[@]}" "$@"
