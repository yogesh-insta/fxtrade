#!/bin/bash
# Cron wrapper for combined daily bot performance email (all platform bots).
set -euo pipefail

INSTALL_ROOT="${FXTRADE_HOME:-/opt/fxtrade}"

exec "$INSTALL_ROOT/bin/bot-daily-email" \
  -credentials "$INSTALL_ROOT/.credentials" \
  -same-day
