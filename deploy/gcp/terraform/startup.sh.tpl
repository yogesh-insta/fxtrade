#!/bin/bash
# GCP metadata startup script — first boot only is idempotent via git clone check.
set -euo pipefail
exec > /var/log/fxtrade-startup.log 2>&1

PROJECT_ID="${project_id}"
SECRET_NAME="${secret_name}"
REPO_URL="${github_repo}"
SRC_DIR="/opt/fxtrade-src"

export DEBIAN_FRONTEND=noninteractive
apt-get update
apt-get install -y git curl gnupg

if ! command -v gcloud >/dev/null 2>&1; then
  install -m 0755 -d /usr/share/keyrings
  curl -fsSL https://packages.cloud.google.com/apt/doc/apt-key.gpg \
    | gpg --dearmor -o /usr/share/keyrings/cloud.google.gpg
  echo "deb [signed-by=/usr/share/keyrings/cloud.google.gpg] https://packages.cloud.google.com/apt cloud-sdk main" \
    > /etc/apt/sources.list.d/google-cloud-sdk.list
  apt-get update
  apt-get install -y google-cloud-cli
fi

if [[ ! -d "$SRC_DIR/.git" ]]; then
  git clone "$REPO_URL" "$SRC_DIR"
fi

cd "$SRC_DIR"
chmod +x deploy/gcp/install.sh deploy/gcp/fetch-credentials.sh
./deploy/gcp/install.sh --enable-bot universe_scanner --dry-run

export GCP_PROJECT="$PROJECT_ID"
if gcloud secrets describe "$SECRET_NAME" --project="$PROJECT_ID" >/dev/null 2>&1; then
  if gcloud secrets versions list "$SECRET_NAME" --project="$PROJECT_ID" \
      --filter="state=ENABLED" --format="value(name)" 2>/dev/null | head -1 | grep -q .; then
    /opt/fxtrade/deploy/gcp/fetch-credentials.sh "$SECRET_NAME" || true
    systemctl restart "fxtrade@universe_scanner.service" || true
  fi
fi
