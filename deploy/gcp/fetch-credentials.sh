#!/bin/bash
# Pull .credentials JSON from GCP Secret Manager (optional).
# Requires: gcloud auth (VM service account or user login) and secretmanager.versions.access.
set -euo pipefail

SECRET_NAME="${1:-fxtrade-credentials}"
OUTPUT="${2:-/opt/fxtrade/.credentials}"
PROJECT="${GCP_PROJECT:-$(gcloud config get-value project 2>/dev/null)}"

if [[ -z "$PROJECT" || "$PROJECT" == "(unset)" ]]; then
  echo "error: set GCP_PROJECT or run: gcloud config set project YOUR_PROJECT_ID"
  exit 1
fi

tmp="$(mktemp)"
gcloud secrets versions access latest \
  --secret="$SECRET_NAME" \
  --project="$PROJECT" >"$tmp"

install -o fxtrade -g fxtrade -m 600 "$tmp" "$OUTPUT"
rm -f "$tmp"
echo "Wrote $OUTPUT from secret $SECRET_NAME (project $PROJECT)"
