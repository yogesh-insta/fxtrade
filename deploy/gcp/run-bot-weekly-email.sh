#!/bin/bash
# Cron wrapper for combined weekly bot performance email.
set -euo pipefail

INSTALL_ROOT="${FXTRADE_HOME:-/opt/fxtrade}"

exec "$INSTALL_ROOT/bin/bot-weekly-email" \
  -credentials "$INSTALL_ROOT/.credentials"
