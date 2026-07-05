#!/bin/bash
# Cron wrapper: resolve health URL from fxtrade env, then run health-watch.
set -euo pipefail

INSTALL_ROOT="${FXTRADE_HOME:-/opt/fxtrade}"
HEALTH_ADDR="${FXTRADE_HEALTH_ADDR:-:8080}"

if [[ -f /etc/fxtrade/fxtrade.env ]]; then
  # shellcheck disable=SC1091
  source /etc/fxtrade/fxtrade.env
  HEALTH_ADDR="${FXTRADE_HEALTH_ADDR:-$HEALTH_ADDR}"
fi

# Per-bot units override the shared env file.
for envfile in /etc/fxtrade/fxtrade@*.env; do
  [[ -f "$envfile" ]] || continue
  unit="${envfile##*/}"
  unit="${unit%.env}"
  if systemctl is-enabled "${unit}.service" &>/dev/null; then
    # shellcheck disable=SC1090
    source "$envfile"
    HEALTH_ADDR="${FXTRADE_HEALTH_ADDR:-$HEALTH_ADDR}"
    break
  fi
done

host="${HEALTH_ADDR#:}"
exec "$INSTALL_ROOT/bin/health-watch" \
  -credentials "$INSTALL_ROOT/.credentials" \
  -health-url "http://127.0.0.1:${host}/health" \
  -state-file "$INSTALL_ROOT/data/health-watch.state.json" \
  "$@"
