# fxtrade

Multi-bot trading platform for **OANDA practice**, email-only scanners, and a standalone **BTC/USD sentiment agent**.

| Layer | What |
|-------|------|
| **Platform bots** | Long-running daemon (`cmd/fxtrade`): opening-range FX scanner, FX sentiment range/trend, BTC/USD CFD mean reversion |
| **Scheduled scanners** | Email NSE swing picks (`nifty-pulse`) and AFL round/pregame reports (`afl-pulse*`) — no OANDA orders |
| **Standalone services** | `services/btc-sentiment` — Gemini tool-calling agent (local HTTP or Cloud Run + Scheduler) |

Deploy platform bots/scanners on a GCP e2-micro VM (systemd) or macOS (`launchd`). The BTC sentiment agent is a separate Go module aimed at Cloud Run.

**Contributing:** all changes go through pull requests — see **[CONTRIBUTING.md](CONTRIBUTING.md)**. Full doc index: **[docs/README.md](docs/README.md)**.

---

## Prerequisites

| Requirement | Notes |
|-------------|--------|
| **Go 1.22+** | `go version` |
| **OANDA practice account** | Required for all platform bots — [oanda.com](https://www.oanda.com) demo + API token |
| **Gmail app password** | Optional; used for alerts across bots |
| **macOS** | Optional; for `launchd` auto-start |

**Per-bot API keys** (only when that bot/scanner is enabled):

| Bot / program | Keys in `.credentials` |
|---------------|------------------------|
| `fx_sentiment` | `finnhub.api_key`, `llm.api_key` (Groq) |
| `universe_scanner`, `btc_cfd` | OANDA only |
| `nifty-pulse` | Email; OANDA keys required for `config.Load`; `llm.api_key` for RSS/LLM sentiment gate |
| `afl-pulse` | `afl.odds_api_key` |
| `afl-pulse-pregame` | `afl.odds_api_key`, `afl.gemini_api_key` |
| `btc-sentiment` (service) | `btc_sentiment.*` (Gemini, Reddit, optional CryptoPanic); Gemini may fall back to `afl.gemini_api_key` |

---

## One-time setup

```bash
cd /path/to/fxtrade

cp .credentials.example .credentials
chmod 600 .credentials
```

Edit `.credentials`:

- **Required:** `oanda.account_id`, `oanda.token`
- **Which bots run:** set `"bots": { "enabled": ["universe_scanner"] }` (see below)
- **Paper-trade vs observe:** run with `--dry-run` locally, or set `FXTRADE_DRY_RUN=--dry-run` on the VM (Terraform default for `universe_scanner`)

Legacy fallback if `"bots.enabled"` is empty: `"strategy": { "mode": "universe_scanner" }` enables the scanner; `"strategy": { "enabled": true }` enables `fx_sentiment`. Prefer `"bots.enabled"` for new configs.

```bash
go mod download
```

---

## Run locally

```bash
# All bots in bots.enabled (default health :8080)
go run ./cmd/fxtrade

# One bot, unique health port (matches GCP per-bot units)
go run ./cmd/fxtrade -bot fx_sentiment -health-addr :8082

# Full daemon, no OANDA orders
go run ./cmd/fxtrade --dry-run
```

**Stop:** `Ctrl+C`.

| Flag | Purpose |
|------|---------|
| `-credentials PATH` | Credentials file (default `.credentials`) |
| `-bot ID` | Run one bot (`universe_scanner`, `fx_sentiment`, `btc_cfd`; `range_trend` is a deprecated alias for `fx_sentiment`) |
| `-health-addr ADDR` | Health HTTP listen address (default `:8080`) |
| `--dry-run` | Stream and strategy cycles run; no place/close orders |

---

## Verify it's working

```bash
curl http://localhost:8080/health | python3 -m json.tool
```

Expect `"ok": true`, `"stream_connected": true`, and `"ticks_received"` increasing. Multi-bot responses include `"active_bots"` and a `"bots"` array with per-bot `"detail"`. When `fx_sentiment` is active, top-level `"sentiment_direction"` and `"trading_mode"` are also populated.

**Logs:**

```bash
tail -f logs/sentiment/$(date +%Y-%m-%d).jsonl   # fx_sentiment LLM audit
tail -f logs/fx_sentiment/journal.jsonl          # fx_sentiment strategy journal + signals
tail -f logs/universe_scanner/trades.jsonl       # universe_scanner closed trades (JSONL)
```

**Performance data (SQLite, for fine-tuning):**

| Bot | Trades DB | Journal / signals |
|-----|-----------|-------------------|
| `btc_cfd` | `data/btc_cfd/trades.db` | `logs/btc_cfd/` |
| `fx_sentiment` | `data/fx_sentiment/trades.db` | `logs/fx_sentiment/` + `signals` table |
| `universe_scanner` | `data/universe_scanner/trades.db` | `logs/universe_scanner/` + `signals` table |

Query closed-trade metrics:

```bash
go run ./cmd/bot-metrics -bot universe_scanner
go run ./cmd/bot-metrics -bot fx_sentiment
go run ./cmd/btc-metrics   # alias for btc_cfd defaults
```

**Email:** alerts go to `email.alert_to` when SMTP is configured.

---

## Test commands

| Command | Purpose |
|---------|---------|
| `go run ./cmd/sentiment-test` | One fx_sentiment cycle (news → Groq → JSON) |
| `go run ./cmd/strategy-test` | One fx_sentiment strategy cycle (mode + gates) |
| `go run ./cmd/scanner-test` | One universe_scanner scan (defaults to `-dry-run`) |
| `go run ./cmd/order-test -count 2 -units 100` | Place and close tiny practice orders |
| `go run ./cmd/backtest -days 200` | Replay history through fx_sentiment quant gates |
| `go run ./cmd/expectancy` | Win rate / expectancy from closed fx_sentiment trades (`logs/fx_sentiment/trades.jsonl`) |
| `go run ./cmd/email-test` | Verify SMTP from `.credentials` |
| `go run ./cmd/afl-train`, `go run ./cmd/afl-backtest` | Train / holdout-test AFL models |
| `go run ./cmd/btc-metrics` | BTC CFD stats from `data/btc_cfd/trades.db` |
| `go run ./cmd/bot-metrics -bot fx_sentiment` | Closed-trade stats from any bot SQLite store |

**Unit tests:**

```bash
go test ./...                                    # main module (platform + scanners)
(cd services/btc-sentiment && go test ./...)     # BTC sentiment agent module
```

**Integration tests** (requires `.credentials`):

```bash
go test -tags=integration ./internal/integration/...
```

---

## Services & programs

Platform bots, scanners, and ops CLIs live under `cmd/` (main Go module). The BTC sentiment agent is a **separate module** under `services/btc-sentiment/`.

On **fxtrade-vm**, `install.sh` installs scanner timers and cron watchdog jobs; enable long-running bots with `--enable-all` or `--enable-bot ID`. Terraform startup uses `install.sh --enable-bot universe_scanner --dry-run`. The BTC sentiment agent is **not** installed on the VM — deploy it to Cloud Run (see below).

**Typical layout on fxtrade-vm:**

| Always on / scheduled | Notes |
|-----------------------|--------|
| `fxtrade@BOT.service` or `fxtrade.service` | Platform bots (OANDA orders when not in dry-run) |
| `nifty-pulse.service` | Daily NSE scan — started by cron (Sun–Fri 18:00 Sydney); timer unit is installed but disabled |
| `afl-pulse.timer` | Weekly AFL round scan (Thu 18:00 Melbourne) |
| `afl-pulse-pregame.timer` | Pregame poll (every 15 min) |
| `/etc/cron.d/fxtrade-watch` | Starts `nifty-pulse.service` at 18:00 Sydney; `health-watch` every 5 min; timer failure checks; `bot-daily-email` at 20:30 UTC; `bot-weekly-email` Mon 07:00 UTC |

### Platform bots (long-running, OANDA orders)

Run locally with `go run ./cmd/fxtrade` (optional `-bot`, `-health-addr`, `--dry-run`). On the VM: `fxtrade.service` (all entries in `"bots.enabled"`) or `fxtrade@ID.service`.

| Bot ID | What | Systemd unit | Health port |
|--------|------|--------------|-------------|
| `universe_scanner` | Opening-range breakout scanner; one FX trade at a time | `fxtrade@universe_scanner.service` | `:8081` |
| `fx_sentiment` | FX range/trend + Finnhub/Groq sentiment gate | `fxtrade@fx_sentiment.service` | `:8082` |
| `btc_cfd` | BTC/USD M5 mean reversion (demo first) | `fxtrade@btc_cfd.service` | `:8083` |
| *(all enabled)* | Every bot in `"bots.enabled"` in one process | `fxtrade.service` | `:8080` |

Legacy id **`range_trend`** is a deprecated alias for **`fx_sentiment`** (still accepted in CLI/systemd for one release).

Enable per-bot units: `sudo ./deploy/gcp/install.sh --enable-bot ID`.

### Scheduled scanners (email only, no OANDA orders)

| Program | What | Schedule |
|---------|------|----------|
| `nifty-pulse` | NSE watchlist swing scan (SMA/RSI filters + RSS/LLM sentiment gate); emails one pick if found | Cron Sun–Fri 18:00 Sydney → `nifty-pulse.service` |
| `afl-pulse` | AFL round odds, projections, value bets | `afl-pulse.timer` |
| `afl-pulse-pregame` | T-45 pregame report (Gemini + Google Search) | `afl-pulse-pregame.timer` |

Local: `go run ./cmd/nifty-pulse`, `go run ./cmd/afl-pulse`, `go run ./cmd/afl-pulse-pregame` (add `-dry-run` to skip email).

### Ops & monitoring

| Program | What | How it runs |
|---------|------|-------------|
| `health-watch` | Polls `/health`; emails on daemon failure, stale ticks, stale bot cycles (`last_cycle_ok_at`), failed timer jobs | Cron via `run-health-watch.sh` |
| `bot-daily-email` | Combined daily P&L + analysis for all platform bots | Cron 20:30 UTC via `run-bot-daily-email.sh` |
| `bot-analyze` | Trade analysis and tweak suggestions from SQLite history | Manual CLI |
| `reconcile-trades` | Backfill $0 P/L and missing instruments from OANDA transactions | Manual CLI; runs automatically before daily email |
| `bot-weekly-email` | Combined weekly P&L for all platform bots | Cron Mon 07:00 UTC via `run-bot-weekly-email.sh` |

### Standalone services (separate Go module)

| Service | What | How it runs |
|---------|------|-------------|
| `btc-sentiment` | Gemini skills+tools agent: news/Reddit → scored BTC/USD sentiment JSON | Local `go run ./cmd/server`, or Cloud Run + Scheduler `POST /run-sentiment-pass` |

Details: [services/btc-sentiment/README.md](services/btc-sentiment/README.md).

---

## Deploy on GCP

**Platform bots + scanners:** bare-metal **e2-micro** VM (systemd, optional GitHub Actions CD). **Full guide:** [deploy/gcp/DEPLOY.md](deploy/gcp/DEPLOY.md). **Terraform (preferred):** [deploy/gcp/terraform/README.md](deploy/gcp/terraform/README.md).

Quick path: provision VM → `sudo ./deploy/gcp/install.sh --enable-bot universe_scanner --dry-run` → place `/opt/fxtrade/.credentials` → deploy Linux binaries (manual `scp` or push to `main` with `GCP_VM_HOST`, `GCP_VM_USER`, `GCP_SSH_KEY` secrets).

**BTC sentiment agent:** build the image from `services/btc-sentiment/Dockerfile`, deploy to Cloud Run (`--min-instances=0`), put secrets in Secret Manager (`GEMINI_API_KEY`, Reddit, optional CryptoPanic), and schedule Cloud Scheduler (OIDC) to `POST /run-sentiment-pass`. See [services/btc-sentiment/README.md](services/btc-sentiment/README.md).

---

## Auto-start on boot (macOS, optional)

```bash
./deploy/install-launchd.sh
```

```bash
launchctl list | grep fxtrade
tail -f logs/daemon.stdout.log
```

**Uninstall:** `launchctl unload ~/Library/LaunchAgents/com.fxtrade.daemon.plist`

---

## Safety controls

| Action | Command |
|--------|---------|
| **Emergency stop** | `curl -X POST http://localhost:8080/kill` or `touch .halt` (per-bot: `.halt.<bot_id>`) |
| **Resume trading** | Remove halt file(s) and restart |
| **Disable orders only** | `--dry-run`, or remove bot from `"bots.enabled"` and restart |

Risk limits live under `"risk"` and per-bot sections in `.credentials` (daily/weekly loss caps, max open positions, etc.).

---

## What to expect (`fx_sentiment`)

- **Most of the time:** `STAND_ASIDE` — no trades (normal).
- **RANGE mode:** resting buy/sell limits at range edges (can sit for days).
- **TREND mode:** occasional market entries on pullbacks.
- **Emails** on decisions, orders, and sentiment updates.

Before live trading: run on practice for **4+ weeks**, then `go run ./cmd/bot-metrics -bot fx_sentiment` (or `go run ./cmd/expectancy -trades logs/fx_sentiment/trades.jsonl`) — aim for **positive expectancy over 30+ trades**.

Other bots persist closed trades and decision signals under `data/<bot_id>/trades.db`; see `docs/specs/btc_cfd_bot_spec.md` for BTC CFD details.

---

## Demo P/L and algorithm tweaks

Practice-account performance, daily analysis email, and config iteration are documented in **[docs/guides/demo_trading_loop.md](docs/guides/demo_trading_loop.md)**. Full doc index: **[docs/README.md](docs/README.md)**. Cursor agents can use **`.cursor/skills/analyze-pl-tweaks/`** when you ask to analyse P/L, review bot performance, or suggest algorithm tweaks (`bot-analyze`, `account-pnl`, daily email workflow).

---

## Project layout

```
fxtrade/
├── .credentials              # secrets (gitignored)
├── cmd/fxtrade/              # multi-bot platform daemon
├── cmd/nifty-pulse/          # NSE swing scanner
├── cmd/afl-pulse/            # AFL weekly round scanner
├── cmd/afl-pulse-pregame/    # AFL T-45 pregame scanner
├── cmd/health-watch/         # VM watchdog
├── cmd/bot-daily-email/      # Combined daily bot performance email
├── cmd/bot-analyze/          # Trade analysis CLI
├── cmd/bot-weekly-email/     # Combined weekly bot performance email
├── cmd/*-test/               # one-shot dev CLIs (sentiment, strategy, scanner, order, …)
├── services/
│   └── btc-sentiment/        # standalone Gemini sentiment agent (own go.mod)
├── data/                     # bot state, AFL stats, per-bot trades.db
├── deploy/gcp/               # systemd units, install.sh, Terraform (VM)
├── logs/                     # sentiment audit, trade journal, daemon logs
├── docs/                     # README index, specs/, guides/, skills/
├── .cursor/skills/           # Cursor agent skills (catalog in docs/skills/)
├── watchlist.txt.example     # NiftyPulse NSE symbol list template
└── plan.md                   # fx_sentiment strategy spec
```

---

## Typical first run

With `"bots": { "enabled": ["fx_sentiment"] }` and Finnhub + Groq keys filled in:

```bash
go run ./cmd/sentiment-test
go run ./cmd/strategy-test
go run ./cmd/fxtrade --dry-run
```

With `"bots": { "enabled": ["universe_scanner"] }` (matches `.credentials.example`):

```bash
go run ./cmd/scanner-test
go run ./cmd/fxtrade --dry-run
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

## BTC sentiment agent (`services/btc-sentiment`)

Standalone **Gemini function-calling** agent (not part of `cmd/fxtrade`). Skills (markdown) + tools drive a multi-turn loop until `emit_sentiment` returns scored BTC/USD sentiment JSON. Intended for Cloud Run; Cloud Scheduler triggers `POST /run-sentiment-pass`.

```text
Cloud Scheduler → HTTP → Agent loop (Gemini + tools) → emit_sentiment → JSON
```

| Piece | Role |
|-------|------|
| Skills | `core`, `data-sources`, `sentiment-scoring`, `cache-and-audit` |
| Tools | `fetch_news`, `fetch_reddit`, `compute_window_key`, `cache_get` / `cache_set`, `log_run`, `emit_sentiment` (terminal) |

**Requires** in `.credentials` under `btc_sentiment` (see `.credentials.example`): Gemini API key (or reuse `afl.gemini_api_key`), Reddit OAuth script app (`reddit_client_id` / `reddit_client_secret` / user-agent with your Reddit username). Optional: `cryptopanic_api_key` (else RSS fallback).

```bash
cd services/btc-sentiment
go run ./cmd/reddit-smoke    # verify Reddit OAuth
go run ./cmd/server          # loads ../../.credentials by default

curl -s localhost:8080/healthz
curl -s -X POST localhost:8080/run-sentiment-pass \
  -H 'Content-Type: application/json' -d '{}'
```

Full architecture, env vars, and Cloud Run notes: **[services/btc-sentiment/README.md](services/btc-sentiment/README.md)**.

---

## NiftyPulse (NSE swing scanner)

Daily NSE watchlist scanner on GCP: `/etc/cron.d/fxtrade-watch` starts `nifty-pulse.service` **Sun–Fri at 18:00 Australia/Sydney** (after NSE close). For each symbol in `watchlist.txt` it fetches Yahoo daily bars, keeps names with **close > SMA(50)** and **RSI(14) in [30, 45]** (`stock_scan` in `.credentials`), ranks by lowest RSI, then runs an **RSS + LLM sentiment gate** on the top candidates (default 3) and skips Negative picks. Emails a single swing suggestion (entry / SL / target) if any survivor remains. Does **not** place orders.

**Requires:** `llm.api_key` for the sentiment gate. Without it (or if RSS/LLM fails), the run degrades to the top ranked filter pass. SMTP for email. OANDA keys still required for `config.Load`. Watchlist: copy `watchlist.txt.example` → `watchlist.txt` (one NSE ticker per line, no `.NS`).

```bash
go run ./cmd/nifty-pulse -dry-run     # scan + log pick, no email
go run ./cmd/nifty-pulse              # email if a pick survives
./scripts/nifty-pulse-run.sh --dry-run
```

---

## AFLPulse (AFL value betting scanner)

Weekly AFL round scanner (Thursday 18:00 Australia/Melbourne on GCP via `afl-pulse.timer`): fetches AU bookmaker h2h + totals odds via [The Odds API](https://the-odds-api.com), builds match context (form, venue, weather, travel, player availability), projects scores, and emails a full round report with value bets highlighted.

**Requires:** `afl.odds_api_key` in `.credentials`. OANDA keys are still required for `config.Load` when using the shared credentials file.

```bash
go run ./cmd/afl-pulse -dry-run                              # log only
go run ./cmd/afl-pulse                                       # email full round report
go run ./cmd/afl-pulse -injuries data/afl/injuries.example.json -dry-run
go run ./cmd/afl-pulse -value-only                           # legacy: email only when value bets exist
```

Seed stats live in `data/afl/` (`venues.json`, `teams.json`, `team_aliases.json`, `model_coefficients.json`). Refresh these periodically; odds are live from the API.
