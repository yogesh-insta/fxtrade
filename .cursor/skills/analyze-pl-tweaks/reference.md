# P/L Analysis Reference

Extended context for the analyze-pl-tweaks skill.

## Data flow

```
OANDA practice account (shared NAV)
        │
        ├─► account-pnl / bot-daily-email ──► NAV vs oanda.initial_capital_aud
        │
        └─► 3 platform bots (fxtrade daemon)
                 │
                 ├─ universe_scanner ──► data/universe_scanner/trades.db
                 ├─ fx_sentiment      ──► data/fx_sentiment/trades.db
                 └─ btc_cfd           ──► data/btc_cfd/trades.db
                          │
                          ▼
                 bot-analyze / FormatDailyAnalysis
                          │
                          ▼
                 tweak suggestions (email + CLI, not auto-applied)
```

**Layers of P/L:**

1. **Account** — OANDA NAV minus `initial_capital_aud` (includes all bots + manual activity)
2. **Per-bot SQLite** — closed trades with `net_pl`, `realized_pl`, instrument, correlation_id
3. **Daily email** — same-day UTC section per bot + combined + analysis block

Each bot sizes from `allocated_capital_usd` (5000) slices, not full NAV — account P/L won't equal sum of bot DB P/L if capital allocation or open positions differ.

## Jul 7 2026 example (first full analysis day)

**Baseline:** $100,000 demo seed + $250 virtual deposit = **$100,250** (`initial_capital_aud`).

**Account snapshot (evening Jul 7):**

| Metric | Value |
|--------|-------|
| NAV | ~$100,076 |
| Total P/L vs baseline | ~−$174 (−0.17%) |
| Realized (balance − baseline) | ~−$224 |
| Unrealized | ~+$51 |

**Findings that drove moderate tweaks:**

| Issue | Signal | Action taken |
|-------|--------|--------------|
| Thin opening ranges | Scanner false breakouts | `opening_range_candles` 1→2 |
| Noisy ranges | Losses on low spread/range ratio | `min_range_spread_ratio` 1.5→2.0 (not 3.0+) |
| Sentiment flip-flop | FX entries on single LLM reading | `sentiment_persistence_readings` 1→2 |
| Daily email timing | 12:00 UTC reported previous day; user in AEST wanted evening | Cron **20:30 UTC** with `-same-day` after force-flat |
| Email noise | Too many trade alerts | `daily_summary_only: true`, trade entry/exit off |

**Scanner day (Jul 7):** multiple trades, net negative; force-flat and late-session closes contributed; analysis flagged 19:00–21:00 UTC loss cluster.

## Known bugs fixed (check if resurfacing)

### metaStore on trade open

**Symptom:** `bot-analyze` reports trades with empty instrument or wrong metadata.

**Cause:** Scanner/engine closed trades without `metaStore.Put` on open — monitor couldn't attach instrument to SQLite row.

**Fix:** `universe_scanner` bot wires `metaStore` through `engine.SetPerformanceStore`; fx_sentiment and btc_cfd have their own meta stores.

**If you see:** `"N trade(s) missing instrument — ensure metaStore on trade open"` — inspect recent commits to bot open paths, not just config.

### P/L lookup ($0 trades)

**Symptom:** `"N trade(s) recorded $0 P/L — OANDA transaction lookup may have failed"`

**Cause:** External closes (SL/TP hit on OANDA side) sometimes failed to fetch realized P/L from transactions API within retry window.

**Fix:** Extended transaction lookup retry in position monitor; re-backfill may need manual DB fix for old rows.

**Trust rule:** Don't tune algorithm on trades with $0 P/L until data is fixed.

### Account P/L format in email

`FormatAccountPNL` in `internal/report/daily.go` — was double-formatting currency; fixed to single `formatMoneySign` pass.

## Improvement loop

```
Run bots on demo (weeks)
        ↓
Daily email 20:30 UTC (auto) or bot-analyze (manual)
        ↓
Review: instrument / exit / hour buckets + suggestions
        ↓
Pick ONE config knob → edit .credentials → restart bot service on VM
        ↓
Wait 3–7 days → bot-analyze again
        ↓
Positive expectancy over 30+ trades per bot before live
```

## When to tighten vs loosen

### Tighten (raise bars, reduce activity)

- Yesterday: all trades lost
- All-time win rate &lt;40% with ≥5 trades
- Instrument: 3+ trades, 0 wins
- Exit `force_flat`: net negative, win rate &lt;35%
- Hour bucket 19:00–21:00 UTC: majority losses
- Realized R:R &lt;1.0 with win rate &lt;55%
- BTC: win rate &lt;40% → enable `m15_confirmation`, raise `deviation_atr_multiple`

### Loosen (more trades, don't over-filter)

- Very low trade count after 1+ week
- `min_setup_score` above median of *winning* setup scores
- `min_range_spread_ratio` ≥3.0 with &lt;1 trade/day
- Sentiment persistence blocking all FX entries in volatile news weeks

### Do not change without user approval

- `oanda.initial_capital_aud` (baseline is historical fact)
- `bots.enabled` (stops/starts entire strategies)
- Risk caps (`max_daily_loss_pct`) downward without explicit request

## VM deploy notes

After code changes affecting analysis or email:

```bash
GOOS=linux GOARCH=amd64 go build -o bot-analyze ./cmd/bot-analyze
GOOS=linux GOARCH=amd64 go build -o bot-daily-email ./cmd/bot-daily-email
# scp to VM /tmp, then:
sudo install -m 755 /tmp/bot-analyze /opt/fxtrade/bin/bot-analyze
sudo install -m 755 /tmp/bot-daily-email /opt/fxtrade/bin/bot-daily-email
```

Or push to `main` — GitHub Actions deploys binaries when configured.

Config-only tweaks: edit `/opt/fxtrade/.credentials`, then `sudo systemctl restart fxtrade@BOT.service` (or `fxtrade.service`).

## Related source files

| File | Purpose |
|------|---------|
| `internal/report/analyze.go` | Analysis + `suggestTweaks` |
| `internal/report/daily.go` | Daily email + account P/L format |
| `internal/report/bots.go` | Bot IDs and DB path resolution |
| `cmd/bot-analyze/main.go` | CLI entry |
| `cmd/bot-daily-email/main.go` | Email sender |
| `cmd/account-pnl/main.go` | Live account P/L |
| `deploy/gcp/run-bot-daily-email.sh` | Cron wrapper |
| `deploy/gcp/fxtrade-watch.cron` | Cron schedule |
