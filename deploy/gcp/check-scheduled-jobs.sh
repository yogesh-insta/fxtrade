#!/bin/bash
# Check a scheduled systemd job and email on failure via health-watch.
set -euo pipefail

INSTALL_ROOT="${FXTRADE_HOME:-/opt/fxtrade}"
JOB="${1:-}"

if [[ -z "$JOB" ]]; then
  echo "usage: $0 {nifty|afl|pregame}" >&2
  exit 1
fi

exec "$INSTALL_ROOT/bin/health-watch" \
  -credentials "$INSTALL_ROOT/.credentials" \
  -state-file "$INSTALL_ROOT/data/health-watch.state.json" \
  -check-timers "$JOB"
