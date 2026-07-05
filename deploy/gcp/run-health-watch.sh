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

run_watch() {
  local addr="$1"
  local state="$2"
  shift 2
  local host="${addr#:}"
  "$INSTALL_ROOT/bin/health-watch" \
    -credentials "$INSTALL_ROOT/.credentials" \
    -health-url "http://127.0.0.1:${host}/health" \
    -state-file "$state" \
    "$@"
}

# Timer checks are global (not per-bot).
if [[ "${1:-}" == -check-timers ]]; then
  exec "$INSTALL_ROOT/bin/health-watch" \
    -credentials "$INSTALL_ROOT/.credentials" \
    -state-file "$INSTALL_ROOT/data/health-watch.state.json" \
    "$@"
fi

enabled_envs=()
for envfile in /etc/fxtrade/fxtrade@*.env; do
  [[ -f "$envfile" ]] || continue
  unit="${envfile##*/}"
  unit="${unit%.env}"
  if systemctl is-enabled "${unit}.service" &>/dev/null; then
    enabled_envs+=("$envfile")
  fi
done

if [[ ${#enabled_envs[@]} -eq 0 ]]; then
  exec run_watch "$HEALTH_ADDR" "$INSTALL_ROOT/data/health-watch.state.json" "$@"
fi

exit_code=0
for envfile in "${enabled_envs[@]}"; do
  unit="${envfile##*/}"
  unit="${unit%.env}"
  bot="${unit#fxtrade@}"
  addr="$HEALTH_ADDR"
  # shellcheck disable=SC1090
  source "$envfile"
  addr="${FXTRADE_HEALTH_ADDR:-$addr}"
  if ! run_watch "$addr" "$INSTALL_ROOT/data/health-watch.${bot}.state.json" "$@"; then
    exit_code=1
  fi
done
exit "$exit_code"
