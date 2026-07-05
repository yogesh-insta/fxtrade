# fxtrade

Multi-instrument forex trading daemon for **OANDA practice** (default: AUD/USD + EUR/USD, set via `"instruments"` in `.credentials`). It streams live prices, runs a per-instrument sentiment pipeline every 30 minutes, and trades when range/trend rules pass. Account-wide 1-position cap and a correlation guard prevent doubled USD exposure across pairs.

---

## Prerequisites

| Requirement | Notes |
|-------------|--------|
| **Go 1.22+** | `go version` |
| **macOS** | For optional `launchd` auto-start |
| **OANDA practice account** | [oanda.com](https://www.oanda.com) demo + API token |
| **Finnhub key** | Free at [finnhub.io](https://finnhub.io) |
| **Groq key** | Free at [console.groq.com](https://console.groq.com) |
| **Gmail app password** | Optional, for email alerts |

---

## One-time setup

```bash
cd /path/to/fxtrade

cp .credentials.example .credentials
chmod 600 .credentials
```

Edit `.credentials`:

- **Required:** `oanda.account_id`, `oanda.token`
- **Sentiment:** `finnhub.api_key`, `llm.api_key`
- **Email (optional):** Gmail SMTP + app password
- **Trading:** set `"strategy": { "enabled": true }` when ready to paper-trade

```bash
go mod download
```

---

## Run the main application

```bash
go run ./cmd/fxtrade
```

Leave this terminal open. The daemon runs:

| Component | What it does |
|-----------|----------------|
| Price stream | Live ticks from OANDA for all configured instruments |
| Sentiment | News + LLM every 30 min |
| Strategy | Mode detection + orders every 30 min (if enabled) |
| Health API | `http://localhost:8080/health` |
| State | Saves to `data/state.json` every 5 min |

**Stop:** `Ctrl+C` in that terminal.

---

## Local testing (dry-run)

Run the full daemon without placing or closing OANDA orders:

```bash
go run ./cmd/fxtrade --dry-run
```

Health still works (`/health` shows `"dry_run": true`). Use this to validate streaming, sentiment, and strategy cycles on practice credentials before enabling live order flow.

---

## Verify it's working

**Health check:**

```bash
curl http://localhost:8080/health | python3 -m json.tool
```

You want:

- `"ok": true`
- `"stream_connected": true`
- `"ticks_received"` increasing
- `"sentiment_direction"` e.g. `"LONG"` / `"FLAT"`
- `"trading_mode"` e.g. `"STAND_ASIDE"` / `"RANGE"` / `"TREND"`

**Logs:**

```bash
tail -f logs/sentiment/$(date +%Y-%m-%d).jsonl   # LLM signals
tail -f logs/trades/journal.jsonl                 # strategy decisions
```

**Email:** alerts go to the address in `email.alert_to`.

---

## Test commands

| Command | Purpose |
|---------|---------|
| `go run ./cmd/sentiment-test` | One sentiment cycle (news → LLM → JSON) |
| `go run ./cmd/strategy-test` | One strategy cycle (mode + gates) |
| `go run ./cmd/order-test -count 2 -units 100` | Place & close 2 tiny practice orders |
| `go run ./cmd/backtest -days 200` | Replay history through quant gates |
| `go run ./cmd/expectancy` | Win rate / expectancy from closed trades |

**Integration tests** (requires `.credentials`):

```bash
go test -tags=integration ./internal/integration/...
```

**Unit tests:**

```bash
go test ./...
```

---

## Deploy on GCP (e2-micro)

Production-style deploy on a single Ubuntu VM (bare binary, systemd, optional GitHub Actions CD).

**Full guide:** [deploy/gcp/DEPLOY.md](deploy/gcp/DEPLOY.md)

Quick outline:

1. Create an **e2-micro** in `us-east1` (free tier).
2. Run `sudo ./deploy/gcp/install.sh --enable-all` on the VM.
3. Place `/opt/fxtrade/.credentials` (SCP or GCP Secret Manager).
4. Deploy the binary manually or via `.github/workflows/deploy.yml` (configure `GCP_VM_HOST`, `GCP_VM_USER`, `GCP_SSH_KEY`).

One systemd unit runs all enabled bots; use `fxtrade@BOT.service` for one bot per process (`--bot` flag).

---

## Services & programs

All binaries live under `cmd/`. On **fxtrade-vm**, `install.sh` enables the three scanner timers and installs cron watchdog jobs; long-running bots are enabled separately (`--enable-all` or `--enable-bot`).

**Typical concurrent layout on fxtrade-vm:**

| Always on | Notes |
|-----------|--------|
| `fxtrade@BOT.service` or `fxtrade.service` | One or more platform bots (Terraform default: `universe_scanner` in dry-run) |
| `nifty-pulse.timer` | Daily NSE scan (Sun–Fri 18:00 Sydney) |
| `afl-pulse.timer` | Weekly AFL round scan (Thu 18:00 Melbourne) |
| `afl-pulse-pregame.timer` | Pregame poll (every 15 min) |
| `/etc/cron.d/fxtrade-watch` | `health-watch` every 5 min; timer failure checks; `btc-daily-email` at 12:00 UTC |

### Platform bots (long-running)

Run locally with `go run ./cmd/fxtrade` (optional `-bot ID`, `-dry-run`). On the VM: `fxtrade.service` (all bots in `.credentials`) or `fxtrade@ID.service`. Legacy id **`range_trend`** is a deprecated alias for **`fx_sentiment`**.

| Bot ID | What | Unit / local | Health | Orders |
|--------|------|--------------|--------|--------|
| `universe_scanner` | OANDA opening-range breakout scanner; one FX trade at a time | `fxtrade@universe_scanner.service` | `:8081` | OANDA |
| `fx_sentiment` | FX range/trend strategy with Finnhub + Groq sentiment gate | `fxtrade@fx_sentiment.service` | `:8082` | OANDA |
| `btc_cfd` | BTC/USD M5 mean reversion on OANDA CFD (demo first) | `fxtrade@btc_cfd.service` | `:8083` | OANDA |
| *(all enabled)* | Every bot listed in `"bots.enabled"` in one process | `fxtrade.service` | `:8080` | OANDA |

Enable per-bot units: `sudo ./deploy/gcp/install.sh --enable-bot ID`. Details: [deploy/gcp/DEPLOY.md](deploy/gcp/DEPLOY.md).

### Scheduled scanners (oneshot)

| Program | What | Schedule | Orders |
|---------|------|----------|--------|
| `nifty-pulse` | NSE watchlist swing scan; emails one pick if found | `nifty-pulse.timer` → `nifty-pulse.service` | Email only |
| `afl-pulse` | AFL round odds, projections, and value bets | `afl-pulse.timer` → `afl-pulse.service` | Email only |
| `afl-pulse-pregame` | T-30 pregame report (Gemini + Google Search) | `afl-pulse-pregame.timer` → `afl-pulse-pregame.service` | Email only |

Local: `go run ./cmd/nifty-pulse`, `go run ./cmd/afl-pulse`, `go run ./cmd/afl-pulse-pregame` (add `-dry-run` to skip email).

### Ops & monitoring

| Program | What | How it runs | Orders |
|---------|------|-------------|--------|
| `health-watch` | Polls `/health`; emails on daemon failure, stale ticks, or failed timer jobs | Cron `/etc/cron.d/fxtrade-watch` via `run-health-watch.sh` | — |
| `btc-daily-email` | BTC CFD daily P&L summary (prior UTC day) | Cron 12:00 UTC via `run-btc-daily-email.sh` | Email only |
| `btc-metrics` | Win rate, drawdown, headroom from `data/btc_cfd/trades.db` | Manual: `go run ./cmd/btc-metrics` | — |

### Test & dev CLIs

One-shot local tools (no systemd). See **Test commands** above for common invocations.

| Command | Purpose |
|---------|---------|
| `sentiment-test`, `strategy-test`, `order-test`, `scanner-test` | Single cycle of sentiment, strategy, OANDA order, or ORB scanner |
| `backtest`, `expectancy` | Historical gate replay; closed-trade performance |
| `afl-train`, `afl-backtest` | Train / holdout-test AFL models |
| `email-test` | Verify SMTP from `.credentials` |

---

## Auto-start on boot (macOS, optional)

```bash
./deploy/install-launchd.sh
```

Check status:

```bash
launchctl list | grep fxtrade
tail -f logs/daemon.stdout.log
```

**Uninstall:**

```bash
launchctl unload ~/Library/LaunchAgents/com.fxtrade.daemon.plist
```

---

## Safety controls

| Action | Command |
|--------|---------|
| **Emergency stop** | `curl -X POST http://localhost:8080/kill` or `touch .halt` |
| **Resume trading** | `rm .halt` and restart daemon |
| **Disable orders only** | Set `"strategy": { "enabled": false }` in `.credentials` |

Risk limits: max 4 trades/month, 1 open position, 2% daily / 5% weekly loss halt.

---

## What to expect

- **Most of the time:** `STAND_ASIDE` — no trades (normal).
- **RANGE mode:** resting buy/sell limits at range edges (can sit for days).
- **TREND mode:** occasional market entries on pullbacks.
- **Emails** on decisions, orders, and sentiment updates.

Before live trading: run on practice for **4+ weeks**, then check:

```bash
go run ./cmd/expectancy
```

Aim for **positive expectancy over 30+ trades**.

---

## Project layout

```
fxtrade/
├── .credentials          # secrets (local only, gitignored)
├── cmd/fxtrade/          # main daemon
├── cmd/sentiment-test/   # test sentiment
├── cmd/strategy-test/    # test strategy
├── cmd/nifty-pulse/      # NSE swing scanner (NiftyPulse)
├── cmd/afl-pulse/        # AFL value betting scanner (AFLPulse)
├── data/afl/             # AFL seed stats, model coefficients, aliases
├── cmd/backtest/         # historical gate replay
├── cmd/expectancy/       # performance report
├── deploy/               # macOS launchd + GCP systemd (deploy/gcp/)
├── .github/workflows/    # CI (test/build) + GCP deploy
├── logs/sentiment/       # LLM audit trail
├── logs/trades/          # journal + trade P&L
├── data/state.json       # persisted daemon state
└── plan.md               # full strategy spec
```

---

## Typical first run

```bash
go run ./cmd/sentiment-test      # confirm LLM works
go run ./cmd/strategy-test       # see current mode
go run ./cmd/fxtrade             # run 24/7
```

In another terminal:

```bash
watch -n 10 'curl -s http://localhost:8080/health | python3 -m json.tool'
```

---

## Security

- Never commit `.credentials` — it is gitignored.
- Use OANDA **practice** until expectancy is proven.
- Revoke API tokens if they are ever exposed.

---

## AFLPulse (AFL value betting scanner)

Weekly AFL round scanner (Thursday 18:00 Australia/Melbourne on GCP via `afl-pulse.timer`): fetches AU bookmaker h2h + totals odds via [The Odds API](https://the-odds-api.com), builds match context (form, venue, weather, travel, player availability), projects scores, and emails a full round report with value bets highlighted.

**Prerequisites:** `afl.odds_api_key` in `.credentials` (free tier at the-odds-api.com). OANDA keys are still required for `config.Load` when using the shared credentials file.

```bash
# Safe test — log predictions, no email
go run ./cmd/afl-pulse -credentials .credentials -dry-run

# Live run — email full round report
go run ./cmd/afl-pulse -credentials .credentials

# Optional match-day injuries override
go run ./cmd/afl-pulse -credentials .credentials -injuries data/afl/injuries.example.json -dry-run

# Legacy: email only when value bets exist
go run ./cmd/afl-pulse -credentials .credentials -value-only
```

Seed stats live in `data/afl/` (`venues.json`, `teams.json`, `team_aliases.json`, `model_coefficients.json`). Refresh these periodically; odds are live from the API.