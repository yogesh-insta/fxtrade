# Demo Trading Loop

How the OANDA practice account, P/L tracking, daily analysis, and config tweaks fit together.

## Purpose

Run three platform bots (`universe_scanner`, `fx_sentiment`, `btc_cfd`) on a **shared OANDA demo account** to collect closed-trade data, measure expectancy, and iterate config before any live trading. Virtual money only.

**Account baseline:** `oanda.initial_capital_aud` in `.credentials` (currently **100250** AUD = $100k seed + $250 demo deposit). Used by `account-pnl` and daily email for total P/L vs starting capital.

## P/L tracking layers

| Layer | Source | What it measures |
|-------|--------|------------------|
| Account | OANDA API (`account-pnl`, daily email header) | NAV vs `initial_capital_aud` — whole account |
| Per-bot | `data/<bot_id>/trades.db` (SQLite) | Closed trades: net P/L, instrument, exit reason |
| Signals | `signals` table in same DB | Setup scores linked by `correlation_id` |

Bots size from per-bot `allocated_capital_usd` (5000), not full NAV. Account P/L and per-bot DB totals may diverge when positions are open or sizing differs.

## Tools

| Command | Purpose |
|---------|---------|
| `go run ./cmd/account-pnl` | Live NAV, realized, unrealized vs baseline |
| `go run ./cmd/bot-analyze` | All bots: all-time + yesterday metrics, buckets, tweak suggestions |
| `go run ./cmd/bot-analyze -bot ID` | Single-bot analysis |
| `go run ./cmd/bot-metrics -bot ID` | Detailed SQLite metrics for one bot |
| `go run ./cmd/bot-daily-email -print` | Preview combined daily email (P/L + analysis) |
| `go run ./cmd/bot-weekly-email -print` | Preview weekly rollup |

On VM: binaries under `/opt/fxtrade/bin/`, credentials at `/opt/fxtrade/.credentials`, run as `fxtrade` user from `/opt/fxtrade`.

## Email schedule

| Job | When (UTC) | Script |
|-----|------------|--------|
| Daily summary | **20:30** (same UTC day, after force-flat ~20:00) | `scripts/run-bot-daily-email.sh` |
| Weekly summary | **Mon 07:00** (previous 7 UTC days) | `scripts/run-bot-weekly-email.sh` |

**Notification mode:** `notifications.daily_summary_only: true` suppresses per-trade emails. Only cron-driven summaries (and health-watch alerts) are sent.

Daily email includes: per-bot day + all-time P/L, account NAV vs baseline, and **Analysis & Suggested Tweaks** from `internal/report/analyze.go`.

## Analysis workflow

1. Check account: `account-pnl`
2. Run `bot-analyze` (local or VM with `-root /opt/fxtrade`)
3. Review dimensions:
   - **Instrument** — remove or raise min score for repeat losers
   - **Exit reason** — `force_flat`, `scanner_breakout`, `tp1_partial`, etc.
   - **Hour UTC** — late-session loss clusters (19:00–21:00)
   - **Win rate** — all-time and yesterday
4. Read auto-generated suggestions; apply judgment (see below)
5. Optionally `-print` daily email to see what cron sends

**Agent skill:** `.cursor/skills/analyze-pl-tweaks/` — step-by-step workflow for Cursor agents.

## Config tweak guidelines

- **One knob at a time** — edit `.credentials`, restart affected `fxtrade@BOT.service`, wait several days
- **Moderate changes** — demo is for learning; avoid choking trade flow (e.g. prefer `min_range_spread_ratio` 2.0 over 3.0+)
- **Suggestions are not auto-applied** — daily email and `bot-analyze` recommend; human (or explicit agent request) applies
- **Data quality first** — fix `$0 P/L` or missing-instrument rows before tuning (metaStore / transaction lookup)
- **Target:** positive expectancy over **30+ closed trades** per bot before live

### Key knobs

**universe_scanner:** `opening_range_candles`, `min_range_spread_ratio`, `min_setup_score`, `take_profit_rr`, `force_flat_utc`

**fx_sentiment:** `sentiment_persistence_readings`, `min_boundary_touches`, `max_open_positions`, `risk_per_trade_pct_base`

**btc_cfd:** `per_trade_risk_pct`, `deviation_atr_multiple`, `target_rr`, `m15_confirmation`, `max_trades_per_day`

## VM deploy

| Item | Value |
|------|-------|
| Instance | `fxtrade-vm` |
| Zone | `us-east1-b` |
| Install root | `/opt/fxtrade` |
| User | `fxtrade` |

```bash
gcloud compute ssh fxtrade-vm --zone=us-east1-b
cd /opt/fxtrade
sudo -u fxtrade bin/bot-analyze -credentials .credentials -root /opt/fxtrade
```

After pulling code or CI deploy: ensure `bin/bot-analyze` and `bin/bot-daily-email` exist; cron in `/etc/cron.d/fxtrade-watch`.

Full deploy guide: [deploy/gcp/DEPLOY.md](../deploy/gcp/DEPLOY.md).
