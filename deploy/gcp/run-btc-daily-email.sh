#!/bin/bash
# Cron wrapper for BTC CFD daily summary email.
set -euo pipefail

INSTALL_ROOT="${FXTRADE_HOME:-/opt/fxtrade}"

exec "$INSTALL_ROOT/bin/btc-daily-email" \
  -credentials "$INSTALL_ROOT/.credentials"
