#!/usr/bin/env bash
# Shared SSH setup for GitHub Actions VM workflows.
# Requires env: GCP_VM_HOST, GCP_SSH_KEY
set -euo pipefail

mkdir -p ~/.ssh
printf '%s\n' "$GCP_SSH_KEY" > ~/.ssh/deploy_key
chmod 600 ~/.ssh/deploy_key
ssh-keyscan -H "$GCP_VM_HOST" >> ~/.ssh/known_hosts 2>/dev/null
