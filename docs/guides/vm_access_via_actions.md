# VM access via GitHub Actions (Option A)

Cloud Agents and contributors without `gcloud` SSH can run commands on **fxtrade-vm** through **workflow_dispatch** workflows. SSH keys stay in GitHub Secrets; nothing sensitive is copied to the agent environment.

## One-time setup (repo admin)

### 1. GitHub Actions secrets

**Settings → Secrets and variables → Actions** — must already exist for deploy:

| Secret | Purpose |
|--------|---------|
| `GCP_VM_HOST` | VM external IP or hostname |
| `GCP_VM_USER` | SSH username |
| `GCP_SSH_KEY` | Private key (full PEM/OpenSSH text) |

### 2. Workflow permissions

**Settings → Actions → General → Workflow permissions**

- Select **Read and write permissions**
- Save

### 3. Allow Cloud Agents to dispatch workflows

The Cursor Cloud Agent GitHub integration needs permission to **trigger workflows** (`workflow_dispatch`). If `gh workflow run` returns **HTTP 403**, enable one of:

- **Cursor:** Cloud Agent / GitHub app permissions — allow **Actions: Read and write** on this repository
- **GitHub org:** Third-party access → Cursor → grant Actions access
- **Fallback:** Run workflows manually from **Actions → VM exec → Run workflow**

## Workflows

| Workflow | Purpose |
|----------|---------|
| **VM exec** | General presets + optional custom command |
| **VM daily email preview** | Shortcut for `daily-email-print` |
| **VM trade analysis** | Upload fresh `bot-analyze` and run SQLite report |

## Agent / CLI usage

From repo root (requires `gh auth login` with workflow dispatch rights):

```bash
chmod +x scripts/vm-exec.sh

# Preview today's daily email on the VM
./scripts/vm-exec.sh daily-email-print

# Full bot analysis
./scripts/vm-exec.sh bot-analyze

# Account NAV vs baseline
./scripts/vm-exec.sh account-pnl

# Service health
./scripts/vm-exec.sh service-status

# Custom (runs as fxtrade user in /opt/fxtrade)
./scripts/vm-exec.sh custom 'bin/bot-metrics -bot universe_scanner'
```

Or dispatch directly:

```bash
gh workflow run vm-exec.yml -f command=daily-email-print
gh run watch
gh run view --log
```

## Preset commands

| `command` | Runs on VM |
|-----------|------------|
| `daily-email-print` | `bot-daily-email -print` |
| `bot-analyze` | `bot-analyze -root /opt/fxtrade` |
| `account-pnl` | `account-pnl` |
| `reconcile-trades` | `reconcile-trades -root /opt/fxtrade` |
| `service-status` | `systemctl is-active` + health curls (as deploy SSH user) |
| `custom` | Your shell string via `custom_command` input |

Preset commands (except `service-status`) run as:

```bash
sudo -u fxtrade bash -lc 'cd /opt/fxtrade && …'
```

## Security notes

- `custom` is intended for maintainers only — avoid arbitrary shell from untrusted input.
- `.credentials` never leaves the VM; workflows only print command stdout in Action logs.
- Use **VM exec** concurrency group — one SSH session at a time.

## Troubleshooting

| Symptom | Fix |
|---------|-----|
| `HTTP 403` on `gh workflow run` | Enable Actions write for Cursor integration (see §3) |
| `(no data)` in email preview | Command must `cd /opt/fxtrade` — use presets or `scripts/vm-exec.sh` |
| SSH fails in workflow | Verify `GCP_SSH_KEY`, host firewall allows TCP 22 |
| Permission denied on `/opt/fxtrade` | Use presets (sudo -u fxtrade) — do not `cd` as deploy user |
