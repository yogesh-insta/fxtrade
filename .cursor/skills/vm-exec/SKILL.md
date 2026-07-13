---
name: vm-exec
description: Run commands on fxtrade-vm via GitHub Actions (no direct SSH). Use when the user asks to check prod VM, preview daily email, run bot-analyze on VM, VM status, or deploy verification without gcloud.
---

# VM exec (GitHub Actions → fxtrade-vm)

Run commands on production VM **fxtrade-vm** (`us-east1-b`, `/opt/fxtrade`) without `gcloud` SSH. SSH keys stay in GitHub Secrets; use **workflow_dispatch**.

**Full guide:** `docs/guides/vm_access_via_actions.md`

## Quick commands (repo root)

```bash
./scripts/vm-exec.sh daily-email-print
./scripts/vm-exec.sh bot-analyze
./scripts/vm-exec.sh account-pnl
./scripts/vm-exec.sh reconcile-trades
./scripts/vm-exec.sh service-status
./scripts/vm-exec.sh custom 'bin/bot-metrics -bot universe_scanner'
```

Requires `gh auth login` with workflow dispatch permission.

## Direct gcloud SSH (laptop only)

```bash
gcloud compute ssh fxtrade-vm --zone=us-east1-b -- \
  'sudo -u fxtrade bash -lc "cd /opt/fxtrade && /opt/fxtrade/bin/bot-daily-email -credentials /opt/fxtrade/.credentials -print"'
```

## Workflows

| Workflow file | Purpose |
|---------------|---------|
| `vm-exec.yml` | Presets + custom command |
| `vm-daily-email.yml` | Daily email preview |
| `vm-analyze.yml` | Trade analysis on VM |
| `deploy.yml` | Auto-deploy on push to `main` |

Manual UI: **GitHub → Actions → VM exec → Run workflow**

## If `HTTP 403` on dispatch

Repo admin must enable **Settings → Actions → Workflow permissions → Read and write**, and grant Cursor/GitHub app **Actions** access on this repo.

## Agent workflow

1. `git pull origin main` (ensure `scripts/vm-exec.sh` exists)
2. Run `./scripts/vm-exec.sh <command>`
3. If 403, tell user to enable workflow dispatch (see guide) or run from Actions UI
4. Parse output between `======== vm-exec:` and `======== end vm-exec ========`
