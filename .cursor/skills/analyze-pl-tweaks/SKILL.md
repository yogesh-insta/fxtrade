---
name: analyze-pl-tweaks
description: Analyzes fxtrade demo P/L from SQLite and OANDA, suggests algorithm config tweaks, and runs bot-analyze and daily email tools. Use when the user asks about profit/loss, performance, tweaks, algorithm improvement, daily summary, or demo account analysis.
---

# Analyze P/L and Suggest Algorithm Tweaks

Operational workflow for the three platform bots on the OANDA practice account.

## Environment

| Context | Path / command |
|---------|----------------|
| **VM** | `fxtrade-vm` in `us-east1-b`; install root `/opt/fxtrade`; run as `fxtrade` user |
| **SSH** | `gcloud compute ssh fxtrade-vm --zone=us-east1-b` |
| **Workdir** | Always `cd /opt/fxtrade` on VM (or repo root locally) |

## Three bots and databases

| Bot ID | Trades DB |
|--------|-----------|
| `universe_scanner` | `data/universe_scanner/trades.db` |
| `fx_sentiment` | `data/fx_sentiment/trades.db` |
| `btc_cfd` | `data/btc_cfd/trades.db` |

## Account baseline

- Config key: `oanda.initial_capital_aud` (demo baseline **100250** AUD — $100k seed + $250 virtual deposit)
- Optional note: `oanda.initial_capital_note`
- **Never** put tokens or secrets in docs or skill output

## Quick commands

### Local (repo root)

```bash
go run ./cmd/account-pnl                    # live NAV vs baseline
go run ./cmd/bot-analyze                    # all 3 bots: metrics + tweak suggestions
go run ./cmd/reconcile-trades               # backfill $0 P/L rows from OANDA
go run ./cmd/bot-analyze -bot universe_scanner
go run ./cmd/bot-metrics -bot fx_sentiment  # single-bot SQLite stats
go run ./cmd/bot-daily-email -print         # preview today's daily email
go run ./cmd/bot-daily-email -date 2026-07-07 -print
```

### VM

```bash
cd /opt/fxtrade
sudo -u fxtrade /opt/fxtrade/bin/account-pnl -credentials /opt/fxtrade/.credentials
sudo -u fxtrade /opt/fxtrade/bin/reconcile-trades -credentials /opt/fxtrade/.credentials -root /opt/fxtrade
sudo -u fxtrade /opt/fxtrade/bin/bot-analyze -credentials /opt/fxtrade/.credentials -root /opt/fxtrade
sudo -u fxtrade /opt/fxtrade/bin/bot-daily-email -credentials /opt/fxtrade/.credentials -print
```

Cron daily email: `20:30 UTC` via `/opt/fxtrade/scripts/run-bot-daily-email.sh` (`-same-day`, after force-flat at 20:00 UTC).

## Analysis workflow

1. **Account level** — `account-pnl`: NAV, realized, unrealized vs `initial_capital_aud`
2. **Reconcile** — `reconcile-trades` backfills $0 P/L rows (also runs before `bot-daily-email`)
3. **Per-bot SQLite** — `bot-analyze` (or `bot-metrics` for one bot)
4. **Read dimensions** in output:
   - By instrument (worst first)
   - By exit reason (`force_flat`, `scanner_breakout`, `tp1_partial`, `max_hold`, etc.)
   - By close hour UTC (loss clusters 19:00–21:00 → entry cutoff)
   - Win rate, all-time vs yesterday
4. **Cross-check** — account P/L should roughly match sum of bot net P/L plus open positions; run `reconcile-trades` if many `$0` trades or missing instruments
5. **Suggest tweaks** — use built-in suggestions + judgment; see philosophy below
6. **Optional** — preview or send daily email with `-print` / without `-print`

Implementation: `internal/report/analyze.go` (`AnalyzeBot`, `suggestTweaks`, `FormatDailyAnalysis`).

## Tweaks philosophy

- **Moderate, not restrictive** — prefer small steps (e.g. `min_range_spread_ratio` 2.0 not 3.0+) so bots stay active on demo
- **One knob at a time** — change one config field, observe 3–7 days, re-run `bot-analyze`
- **Suggestions only** — daily email and agent output recommend changes; **do not** auto-edit `.credentials` or restart services unless the user asks
- **Tighten** when: all-red yesterday, win rate &lt;40% over 5+ trades, instrument 0-for-3+, force-flat losses, late-hour clusters
- **Loosen** when: too few trades, filters blocking winners (compare setup scores of wins vs losses)

## Config knobs (`.credentials`)

### universe_scanner (`scanner` section)

| Knob | Role |
|------|------|
| `opening_range_candles` | M15 candles before range (≥2 reduces false breakouts) |
| `min_range_spread_ratio` | Skip thin ranges |
| `min_setup_score` | Entry quality floor |
| `take_profit_rr` | Reward vs stop |
| `risk_per_trade_pct` | Position size |
| `force_flat_utc` | Session close before rollover (~20:00 UTC FX) |

### fx_sentiment

| Knob | Section |
|------|---------|
| `sentiment_persistence_readings` | `llm_gate` |
| `min_boundary_touches` | `range_mode` |
| `max_open_positions` | `risk` |
| `risk_per_trade_pct_base` | `risk` |

### btc_cfd (`btc_cfd` section)

| Knob | Role |
|------|------|
| `per_trade_risk_pct` | Size per trade |
| `deviation_atr_multiple` | Mean-reversion entry depth |
| `target_rr` | Take-profit vs stop |
| `m15_confirmation` | Trend filter |
| `max_trades_per_day` | Activity cap |

## Email settings

- `notifications.daily_summary_only: true` — suppress per-trade emails; only cron summaries
- `notifications.on_trade_entry` / `on_trade_exit`: `false`
- Daily: `bot-daily-email` at **20:30 UTC** (includes P/L + `FormatDailyAnalysis` tweaks block)
- Weekly: `bot-weekly-email` Mon **07:00 UTC**

## Output format for user

When reporting findings:

```markdown
## Account
NAV vs baseline, total P/L %

## Per bot
- All-time: trades, win rate, net P/L
- Yesterday: trades, net P/L
- Worst instruments / exit reasons / hours

## Suggested tweaks (apply manually)
1. One specific change with rationale
2. ...

## Next step
Re-run bot-analyze after N days or after applying change
```

## Additional resources

- Deep reference (data flow, Jul 7 example, known bugs): [reference.md](reference.md)
- Canonical repo playbook: [docs/demo_trading_loop.md](../../../docs/demo_trading_loop.md)
