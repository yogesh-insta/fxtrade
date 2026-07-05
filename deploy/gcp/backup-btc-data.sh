#!/usr/bin/env bash
# OPTIONAL manual backup of btc_cfd SQLite + journal to GCS.
# Not installed or scheduled by default — avoids paid bucket/cron setup.
# Primary store is local SQLite on the VM (free). Run manually when a bucket exists.
set -euo pipefail

INSTALL_ROOT="${INSTALL_ROOT:-/opt/fxtrade}"
GCS_BUCKET="${GCS_BUCKET:-gs://YOUR_PROJECT-fxtrade-backups}"
STAMP="$(date -u +%Y%m%d)"

DB_SRC="$INSTALL_ROOT/data/btc_cfd/trades.db"
JOURNAL_SRC="$INSTALL_ROOT/logs/btc_cfd"

if [[ ! -f "$DB_SRC" ]]; then
  echo "skip: no sqlite db at $DB_SRC"
  exit 0
fi

if ! command -v gsutil >/dev/null 2>&1; then
  echo "error: gsutil not found — install Google Cloud SDK on the VM"
  exit 1
fi

DEST="$GCS_BUCKET/btc_cfd/$STAMP"
echo "backing up btc_cfd data to $DEST"
gsutil cp "$DB_SRC" "$DEST/trades.db"
if [[ -d "$JOURNAL_SRC" ]]; then
  gsutil -m cp -r "$JOURNAL_SRC" "$DEST/journal"
fi
echo "backup complete"
