# Deploy fxtrade on GCP (e2-micro)

Bare-metal deploy on a single **e2-micro** VM (Ubuntu 22.04/24.04). Target: **$0/month** with GCP free tier (`us-east1` recommended). No Cloud Run or container registry required.

## Infrastructure (Terraform)

**Preferred:** provision the VM, service account, Secret Manager secret, IAM, and firewall with Terraform.

```bash
cd deploy/gcp/terraform
cp terraform.tfvars.example terraform.tfvars   # edit admin_cidr, ssh_public_keys
terraform init && terraform plan && terraform apply
```

Full instructions (import existing VM, upload secrets, GitHub Actions wiring): **[terraform/README.md](terraform/README.md)**.

The sections below describe **legacy manual `gcloud` steps** — use them only if you are not using Terraform.

## What you provide from GCP

| Item | Example | Used for |
|------|---------|----------|
| **Project ID** | `my-fxtrade-prod` | `gcloud`, Secret Manager, firewall rules |
| **Region / zone** | `us-east1-b` | VM placement (free-tier eligible) |
| **VM external IP or hostname** | `34.x.x.x` | SSH, GitHub Actions `GCP_VM_HOST` |
| **SSH user** | `your_gcp_username` | GitHub Actions `GCP_VM_USER` |
| **SSH private key** | ed25519 key pair | GitHub secret `GCP_SSH_KEY` |
| **Optional: Secret Manager secret** | `fxtrade-credentials` | VM pulls `.credentials` at boot |
| **Optional: service account** | VM attached SA with `secretmanager.secretAccessor` | `fetch-credentials.sh` without user login |

## 1. Create the VM

```bash
export PROJECT_ID="your-gcp-project"
export ZONE="us-east1-b"

gcloud config set project "$PROJECT_ID"

gcloud compute instances create fxtrade-vm \
  --zone="$ZONE" \
  --machine-type=e2-micro \
  --image-family=ubuntu-2204-lts \
  --image-project=ubuntu-os-cloud \
  --boot-disk-size=10GB \
  --tags=fxtrade
```

Add your SSH public key at create time, or via OS Login / metadata:

```bash
gcloud compute os-login ssh-keys add --key-file=~/.ssh/id_ed25519.pub
```

Connect:

```bash
gcloud compute ssh fxtrade-vm --zone="$ZONE"
```

## 2. Clone repo and run install script

On the VM:

```bash
sudo apt-get update && sudo apt-get install -y git
# Private repo: use SSH deploy key on the VM, or clone from a machine with access:
git clone git@github.com:yogesh-insta/fxtrade.git
# Or: gh repo clone yogesh-insta/fxtrade  (after gh auth login)
cd fxtrade
chmod +x deploy/gcp/install.sh deploy/gcp/fetch-credentials.sh
sudo ./deploy/gcp/install.sh --enable-all
```

This creates:

| Path | Purpose |
|------|---------|
| `/opt/fxtrade/bin/fxtrade` | FX daemon binary (deployed by CI or manual `scp`) |
| `/opt/fxtrade/bin/nifty-pulse` | NiftyPulse daily NSE scanner |
| `/opt/fxtrade/bin/afl-pulse` | AFLPulse weekly AFL round scanner |
| `/opt/fxtrade/bin/afl-pulse-pregame` | AFLPulse T-45 pregame scanner (Gemini) |
| `/opt/fxtrade/watchlist.txt` | NSE symbol watchlist for NiftyPulse |
| `/opt/fxtrade/data/afl/` | AFL seed stats (teams, venues, model coefficients) |
| `/opt/fxtrade/.credentials` | Secrets JSON (600, owner `fxtrade`) |
| `/opt/fxtrade/data/` | Bot state files |
| `/opt/fxtrade/logs/` | Daemon stdout/stderr |
| `/etc/fxtrade/fxtrade.env` | Health addr, dry-run flag |
| `/etc/systemd/system/fxtrade.service` | All enabled bots |
| `/etc/systemd/system/fxtrade@.service` | One bot per unit (`--bot`) |
| `/etc/systemd/system/nifty-pulse.service` | NiftyPulse oneshot scan |
| `/etc/systemd/system/nifty-pulse.timer` | Daily 18:00 Australia/Sydney (Sun–Fri) |
| `/etc/systemd/system/afl-pulse.service` | AFLPulse oneshot round scan |
| `/etc/systemd/system/afl-pulse.timer` | Weekly 18:00 Australia/Melbourne (Thursday) |
| `/etc/systemd/system/afl-pulse-pregame.service` | AFLPulse T-45 pregame oneshot |
| `/etc/systemd/system/afl-pulse-pregame.timer` | Every 5 minutes (pregame poll) |

### Systemd modes

**All enabled bots** (recommended on one VM — single health endpoint):

```bash
sudo systemctl enable --now fxtrade.service
```

Runs `fxtrade` with no `--bot` flag; bots come from `"bots": { "enabled": [...] }` in `.credentials`.

**One bot per unit** (process isolation; assign unique health ports):

```bash
sudo ./deploy/gcp/install.sh --enable-bot universe_scanner
sudo ./deploy/gcp/install.sh --enable-bot fx_sentiment
```

Or manually:

```bash
sudo systemctl enable --now fxtrade@universe_scanner.service
```

Per-bot health ports (set by `install.sh --enable-bot`):

| Bot | Health port |
|-----|-------------|
| `universe_scanner` | `:8081` |
| `fx_sentiment` | `:8082` |
| `fxtrade.service` (all) | `:8080` |

## 2b. Email alerts when bots fail (free)

`install.sh` installs a **watchdog** that emails you via the same Gmail SMTP in `.credentials`. No external monitoring service or open firewall ports.

| Check | How often | What triggers an email |
|-------|-----------|------------------------|
| FX daemon | Every 5 min | Process down, stream disconnected, stale ticks (FX hours only), bot not running, stale strategy cycle (`last_cycle_ok_at`), kill switch on |
| NiftyPulse | Daily 18:30 Sydney (Sun–Fri) | `nifty-pulse.service` failed |
| AFLPulse | Thu 18:30 Melbourne | `afl-pulse.service` failed |
| AFL pregame | Every 20 min | `afl-pulse-pregame.service` failed |

Files:

| Path | Purpose |
|------|---------|
| `/opt/fxtrade/bin/health-watch` | Watchdog binary |
| `/opt/fxtrade/scripts/run-health-watch.sh` | Resolves health port from `/etc/fxtrade/fxtrade.env` |
| `/opt/fxtrade/scripts/check-scheduled-jobs.sh` | Wrapper for timer failure checks |
| `/etc/cron.d/fxtrade-watch` | Cron schedule (runs as `fxtrade` user) |
| `/opt/fxtrade/data/health-watch.state.json` | Dedupes alerts (one email per incident) |
| `/opt/fxtrade/logs/health-watch.log` | Watchdog log |

Manual test on the VM:

```bash
# Should print "health ok" and send no email
sudo -u fxtrade /opt/fxtrade/scripts/run-health-watch.sh

# Simulate failure: stop daemon, wait for cron (or run again within 5 min)
sudo systemctl stop fxtrade@universe_scanner.service
sudo -u fxtrade /opt/fxtrade/scripts/run-health-watch.sh
# Check inbox for "fxtrade: unhealthy"

sudo systemctl start fxtrade@universe_scanner.service
sudo -u fxtrade /opt/fxtrade/scripts/run-health-watch.sh
# Check inbox for "fxtrade: recovered"
```

Build/deploy the watchdog binary:

```bash
GOOS=linux GOARCH=amd64 go build -o health-watch ./cmd/health-watch
scp health-watch user@VM:/tmp/ && sudo install -m 755 /tmp/health-watch /opt/fxtrade/bin/health-watch
```

The `/health` endpoint returns HTTP 503 with `"ok": false` when unhealthy (stream down, stale ticks, etc.).

## 2c. Scheduled performance emails

Cron jobs in `/etc/cron.d/fxtrade-watch` send plain-text summaries via the same SMTP block in `.credentials`.

| Job | Schedule | Binary | Log |
|-----|----------|--------|-----|
| BTC daily summary | 12:00 UTC daily | `btc-daily-email` | `/opt/fxtrade/logs/btc-daily-email.log` |
| Combined weekly bot summary | **Monday 07:00 UTC** | `bot-weekly-email` | `/opt/fxtrade/logs/bot-weekly-email.log` |

The weekly email covers all three platform bots (`universe_scanner`, `fx_sentiment`, `btc_cfd`) for the **previous seven UTC calendar days** ending Sunday (inclusive).

Manual test on the VM:

```bash
# Print without sending
sudo -u fxtrade /opt/fxtrade/bin/bot-weekly-email -credentials /opt/fxtrade/.credentials -print

# Send for real
sudo -u fxtrade /opt/fxtrade/scripts/run-bot-weekly-email.sh
```

Build/deploy the weekly email binary:

```bash
GOOS=linux GOARCH=amd64 go build -o bot-weekly-email ./cmd/bot-weekly-email
scp bot-weekly-email user@VM:/tmp/ && sudo install -m 755 /tmp/bot-weekly-email /opt/fxtrade/bin/bot-weekly-email
sudo install -m 755 deploy/gcp/run-bot-weekly-email.sh /opt/fxtrade/scripts/run-bot-weekly-email.sh
sudo install -m 644 deploy/gcp/fxtrade-watch.cron /etc/cron.d/fxtrade-watch
```

## 3. Place `.credentials`

**Option A — copy from laptop (simplest):**

```bash
scp -i ~/.ssh/your_key .credentials user@VM_IP:/tmp/.credentials
ssh user@VM_IP 'sudo install -o fxtrade -g fxtrade -m 600 /tmp/.credentials /opt/fxtrade/.credentials && rm /tmp/.credentials'
```

**Option B — GCP Secret Manager:**

```bash
# Once, from your laptop (JSON file must not be committed):
gcloud secrets create fxtrade-credentials --replication-policy=automatic
gcloud secrets versions add fxtrade-credentials --data-file=.credentials

# On VM (service account needs secretmanager.secretAccessor):
sudo /opt/fxtrade/deploy/gcp/fetch-credentials.sh fxtrade-credentials
```

Grant the VM's service account:

```bash
gcloud secrets add-iam-policy-binding fxtrade-credentials \
  --member="serviceAccount:VM_SERVICE_ACCOUNT@PROJECT_ID.iam.gserviceaccount.com" \
  --role="roles/secretmanager.secretAccessor"
```

Restart after credentials are in place:

```bash
sudo systemctl restart fxtrade.service
# or per-bot: sudo systemctl restart fxtrade@universe_scanner.service
```

## 4. Deploy the Linux binaries

**From your laptop:**

```bash
GOOS=linux GOARCH=amd64 go build -o fxtrade ./cmd/fxtrade
GOOS=linux GOARCH=amd64 go build -o nifty-pulse ./cmd/nifty-pulse
GOOS=linux GOARCH=amd64 go build -o afl-pulse ./cmd/afl-pulse
GOOS=linux GOARCH=amd64 go build -o afl-pulse-pregame ./cmd/afl-pulse-pregame
scp fxtrade nifty-pulse afl-pulse afl-pulse-pregame watchlist.txt user@VM_IP:/tmp/
scp -r data/afl user@VM_IP:/tmp/
ssh user@VM_IP 'sudo install -m 755 /tmp/fxtrade /opt/fxtrade/bin/fxtrade && \
  sudo install -m 755 /tmp/nifty-pulse /opt/fxtrade/bin/nifty-pulse && \
  sudo install -m 755 /tmp/afl-pulse /opt/fxtrade/bin/afl-pulse && \
  sudo install -m 755 /tmp/afl-pulse-pregame /opt/fxtrade/bin/afl-pulse-pregame && \
  sudo install -o fxtrade -g fxtrade -m 644 /tmp/watchlist.txt /opt/fxtrade/watchlist.txt && \
  sudo mkdir -p /opt/fxtrade/data/afl && sudo cp -f /tmp/afl/*.json /opt/fxtrade/data/afl/ && \
  sudo chown -R fxtrade:fxtrade /opt/fxtrade/data/afl && \
  sudo systemctl restart fxtrade.service'
```

**Via GitHub Actions:** push to `main` (see `.github/workflows/deploy.yml`) after configuring secrets below. The workflow deploys `fxtrade`, `nifty-pulse`, `afl-pulse`, `watchlist.txt`, and `data/afl/`, then restarts every enabled `fxtrade.service` and `fxtrade@*.service` unit.

## 5. NiftyPulse (NSE daily scanner)

NiftyPulse scans the NSE watchlist after market close and emails a single swing-trade pick (if any symbol passes filters + sentiment gate). It does **not** place orders.

### Schedule

`install.sh` enables `nifty-pulse.timer`, which fires **Sun–Fri at 18:00 Australia/Sydney** (`OnCalendar=Sun..Fri *-*-* 18:00:00` with `Timezone=Australia/Sydney`). Systemd applies **AEST/AEDT automatically** — no manual UTC offset or cron DST hacks. Sunday evening prep covers Monday’s trading week; Saturday is excluded.

That is roughly **13:30 IST** (standard time) / **12:30 IST** (daylight), i.e. shortly after the NSE cash session close.

```bash
sudo systemctl status nifty-pulse.timer
sudo systemctl list-timers nifty-pulse.timer
journalctl -u nifty-pulse.service -n 50
tail -f /opt/fxtrade/logs/nifty-pulse.log
```

### On-demand runs

**On the VM** (sends a real email if a pick is found — same as the scheduled run):

```bash
sudo systemctl start nifty-pulse.service
```

**Safe test on the VM** (scan only, no email):

```bash
sudo -u fxtrade /opt/fxtrade/bin/nifty-pulse \
  -credentials /opt/fxtrade/.credentials \
  -watchlist /opt/fxtrade/watchlist.txt \
  -dry-run
```

**Locally** (from repo root):

```bash
chmod +x scripts/nifty-pulse-run.sh
./scripts/nifty-pulse-run.sh --dry-run    # safe: logs pick, no email
./scripts/nifty-pulse-run.sh              # sends email if configured in .credentials
```

Or build and run directly:

```bash
go build -o bin/nifty-pulse ./cmd/nifty-pulse
./bin/nifty-pulse -credentials .credentials -watchlist watchlist.txt -dry-run
```

**Email on real runs:** any on-demand run **without** `-dry-run` uses the SMTP settings in `.credentials` (`email.alert_to`) and sends the same Zerodha-style alert as the timer. Use `-dry-run` when testing credentials, watchlist, or scanner changes.

Re-install or update systemd units after pulling deploy changes:

```bash
sudo cp deploy/gcp/nifty-pulse.{service,timer} /etc/systemd/system/
sudo systemctl daemon-reload
sudo systemctl enable --now nifty-pulse.timer
```

## 6. AFLPulse (AFL weekly round scanner)

AFLPulse scans upcoming AFL fixtures every Thursday evening: fetches AU bookmaker h2h and totals odds via [The Odds API](https://the-odds-api.com), builds match context from seed stats (`data/afl/teams.json`, `venues.json`, `players.json`), optional injuries override, and Open-Meteo weather, then emails a **full round report** with win probability, predicted scores, margin, and any value bets highlighted.

### Data sources (free)

| Source | File / API | Used for |
|--------|------------|----------|
| Team form & efficiency | `data/afl/teams.json` | Form, inside-50, clearances, contested possessions, disposals |
| Venues | `data/afl/venues.json` | Home win rates, ground dimensions, lat/lon |
| Players / injuries | `data/afl/players.json` + `-injuries` JSON | Key player availability impact |
| Weather | Open-Meteo (no key) | Rain, wind, scoring/total adjustment |
| Odds | The Odds API (`h2h,totals`) | Head-to-head EV; totals line benchmark |
| Team name aliases | `data/afl/team_aliases.json` | Odds API name → team ID |
| Model coefficients | `data/afl/model_coefficients.json` | Win probability (matrix predictor) |

### Schedule

`install.sh` enables `afl-pulse.timer`, which fires **every Thursday at 18:00 Australia/Melbourne** (`OnCalendar=Thu *-*-* 18:00:00` with `Timezone=Australia/Melbourne`). Systemd applies **AEST/AEDT automatically**.

```bash
sudo systemctl status afl-pulse.timer
sudo systemctl list-timers afl-pulse.timer
journalctl -u afl-pulse.service -n 50
tail -f /opt/fxtrade/logs/afl-pulse.log
```

### On-demand runs

**On the VM** (sends a real email with full round report):

```bash
sudo systemctl start afl-pulse.service
```

**Safe test on the VM** (scan only, no email):

```bash
sudo -u fxtrade /opt/fxtrade/bin/afl-pulse \
  -credentials /opt/fxtrade/.credentials \
  -dry-run
```

**Locally** (from repo root):

```bash
chmod +x scripts/afl-pulse-run.sh
./scripts/afl-pulse-run.sh --dry-run
./scripts/afl-pulse-run.sh
```

Optional match-day injuries override:

```bash
./scripts/afl-pulse-run.sh -injuries data/afl/injuries.example.json --dry-run
```

Legacy value-bet-only email (no full round report):

```bash
./bin/afl-pulse -credentials .credentials -value-only
```

Re-install or update systemd units after pulling deploy changes:

```bash
sudo cp deploy/gcp/afl-pulse.{service,timer} /etc/systemd/system/
sudo systemctl daemon-reload
sudo systemctl enable --now afl-pulse.timer
```

## 6b. AFLPulse pregame (T-45)

`afl-pulse-pregame` polls Squiggle every **5 minutes** for fixtures whose kickoff is **45–50 minutes away**. When a fixture enters the window and has not been emailed yet:

1. Fetches AU bookmaker odds (The Odds API)
2. Builds one AFLPulse model report (`BuildRoundReports`)
3. Calls **Gemini 2.5 Flash-Lite + Google Search** (compact JSON in, structured JSON out)
4. Emails the T-45 pregame report and records the Squiggle game ID in `data/afl/pregame-sent.json`

If Gemini fails after retries, an **alert email** is sent instead (dedup state unchanged so the next poll can retry).

**Requires** `afl.gemini_api_key` in `.credentials`. Weekly round-scan LLM (`llm_analytics_enabled`) is **opt-in** and defaults to `false`.

### Schedule

`install.sh` enables `afl-pulse-pregame.timer` (`OnUnitActiveSec=5min`).

```bash
sudo systemctl status afl-pulse-pregame.timer
sudo systemctl list-timers afl-pulse-pregame.timer
journalctl -u afl-pulse-pregame.service -n 50
tail -f /opt/fxtrade/logs/afl-pulse-pregame.log
```

### On-demand / dry-run

**On the VM** (real email when a fixture is in window):

```bash
sudo systemctl start afl-pulse-pregame.service
```

**Safe test on the VM**:

```bash
sudo -u fxtrade /opt/fxtrade/bin/afl-pulse-pregame \
  -credentials /opt/fxtrade/.credentials \
  -dry-run
```

**Locally**:

```bash
chmod +x scripts/afl-pulse-pregame-run.sh
./scripts/afl-pulse-pregame-run.sh --dry-run
go run ./cmd/afl-pulse-pregame -credentials .credentials -dry-run
```

Re-install systemd units after pulling deploy changes:

```bash
sudo cp deploy/gcp/afl-pulse-pregame.{service,timer} /etc/systemd/system/
sudo systemctl daemon-reload
sudo systemctl enable --now afl-pulse-pregame.timer
```

## 7. Firewall (optional health check from outside)

Health listens on `:8080` by default (localhost-only is fine for SSH tunneling).

Allow inbound 8080 only if you need external monitoring:

```bash
gcloud compute firewall-rules create fxtrade-health \
  --allow=tcp:8080 \
  --target-tags=fxtrade \
  --source-ranges=YOUR_IP/32 \
  --description="fxtrade health endpoint"
```

Verify on VM:

```bash
curl -s http://127.0.0.1:8080/health | python3 -m json.tool
```

SSH tunnel from laptop:

```bash
ssh -L 8080:127.0.0.1:8080 user@VM_IP
curl http://localhost:8080/health
```

## 8. Operations

```bash
sudo systemctl status fxtrade.service
sudo journalctl -u fxtrade.service -f
tail -f /opt/fxtrade/logs/daemon.stdout.log

# Emergency halt (same as POST /kill)
sudo -u fxtrade touch /opt/fxtrade/.halt
sudo systemctl restart fxtrade.service

# Dry-run mode (no OANDA orders)
echo 'FXTRADE_DRY_RUN=--dry-run' | sudo tee -a /etc/fxtrade/fxtrade.env
sudo systemctl restart fxtrade.service
```

## GitHub Actions secrets

Configure in **Settings → Secrets and variables → Actions**:

| Secret | Required | Description |
|--------|----------|-------------|
| `GCP_VM_HOST` | Yes | VM external IP or hostname |
| `GCP_VM_USER` | Yes | SSH username on the VM |
| `GCP_SSH_KEY` | Yes | Private key (full PEM/OpenSSH text) |
| `GCP_VM_PATH` | No | Binary path (default `/opt/fxtrade/bin/fxtrade`) |

Deploy workflow: `.github/workflows/deploy.yml` — runs on push to `main` or manual **workflow_dispatch**.

CI workflow: `.github/workflows/ci.yml` — `go test ./...` and build on every push/PR.

## Cost notes (e2-micro free tier)

- **e2-micro** in `us-east1`, `us-west1`, or `us-central1`: 1 instance free per month (subject to GCP free tier terms).
- 10 GB standard persistent disk included in free tier allowance.
- Egress to OANDA/Finnhub/Groq is minimal; stay on practice account until validated.

## Troubleshooting

| Symptom | Check |
|---------|--------|
| Service exits immediately | `journalctl -u fxtrade -n 50`; missing `.credentials` or invalid JSON |
| `address already in use` | Two bot units on same health port — use `install.sh --enable-bot` or edit drop-in |
| SSH deploy fails | `GCP_SSH_KEY` format, `known_hosts`, firewall allows TCP 22 |
| No trades | `"strategy": { "enabled": true }`, not in dry-run, OANDA practice funded |
