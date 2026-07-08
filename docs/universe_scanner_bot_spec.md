# Universe Scanner Bot (`universe_scanner`) — Logic Reference

Opening-range breakout (ORB) scanner on OANDA. Ranks a multi-instrument universe, trades **one position at a time**.

| Item | Value |
|------|-------|
| Bot ID | `universe_scanner` |
| Display name | FXPulse ORB Scanner |
| Systemd | `fxtrade@universe_scanner.service` |
| Health port | `:8081` |
| Config block | `.credentials` → `scanner` |
| State | `data/state-universe_scanner.json` |
| Trades DB | `data/universe_scanner/trades.db` |
| Journal | `logs/universe_scanner/` |
| One-shot test | `go run ./cmd/scanner-test` (defaults to dry-run) |

---

## Source map (read these, not the whole repo)

| Topic | File |
|-------|------|
| Bot wiring (risk, monitor, DB) | `internal/bots/universe_scanner/bot.go` |
| Main poll loop + entry/exit | `internal/scanner/engine.go` |
| Per-symbol scan | `internal/scanner/scan.go` |
| Opening range build | `internal/scanner/range.go` |
| Score, rank, breakout | `internal/scanner/rank.go` |
| Sessions + force-flat times | `internal/scanner/session.go` |
| Universe presets / filters | `internal/scanner/universe.go` |
| Config defaults + keys | `internal/config/scanner.go` |
| Config example | `.credentials.example` → `"scanner"` |

---

## Runtime loop (every `poll_seconds`, default 10)

```
halted? → skip
balance + weekly-target email + daily/weekly summaries
force-flat any open positions past ForceFlatUTC for asset class
already have an open trade tagged universe_scanner? → skip (max 1)
ScanAll(symbols) → RankSetups(min_setup_score) → top setup
breakout confirmed? → session_already_traded? → enter market + SL/TP
```

**One trade per instrument per session:** `sessionEntered[instrument] == range.SessionOpen` blocks re-entry.

---

## Per-symbol scan (`scanOne`)

1. **In session?** Asset-class UTC window (`scanner.asset_classes.*.session_utc`). Crypto default `24/7`.
2. **Live price** — skip if not tradeable or spread > class max.
3. **Opening range** — first `opening_range_candles` complete M15 bars after session open (`opening_range_granularity`, default M15). High/low → `Range.High`, `Range.Low`, `RangePips`.
4. **H1 trend bias** — +1 / −1 / 0 from first vs last H1 candle (coarse filter for score only).
5. **Score** — reject if `range_pips/spread_pips < min_range_spread_ratio` or spread too wide.

```
score = 0.35×rangeScore + 0.20×spreadScore + 0.30×ratioScore + 0.15×trendScore
```

Defaults: `min_setup_score` 0.65, `min_range_spread_ratio` 3.0.

6. **Breakout** (on bid/ask):
   - LONG if `bid > range.High`
   - SHORT if `ask < range.Low`
   - else `await_breakout` (no order)

---

## Ranking (when multiple setups pass)

Sort descending: **score** → **range/spread ratio** → **tightest spread** → symbol name. Only **#1** is traded; runners-up go to notification only.

---

## Entry sizing and brackets

| Input | Source |
|-------|--------|
| Entry | Ask (LONG) or Bid (SHORT) |
| Stop distance | `stop_loss_pips_fx` / `stop_loss_points_crypto` / `stop_loss_points_index` × pip size |
| Take profit | `take_profit_rr × stop distance` (default RR 2.1) |
| Units | `floor(balance × risk_per_trade_pct/100 / stop_distance)` |
| Balance | `balance_source`: `oanda_nav` (default), `oanda_balance`, or `config` |

Correlation ID: `universe_scanner:scan-{instrument}-{nanos}`.

---

## Exits

| Exit | Mechanism |
|------|-----------|
| Stop / TP | Set on market order at entry |
| `force_flat` | `force_flat_utc` per class (default 20:00 UTC FX/JPY/METAL/INDEX) — before NY rollover |
| Kill switch | Risk manager halt (`daily_loss_cap_pct`, default 1%) |

Signal actions logged to SQLite `signals` table: `no_setup`, `await_breakout`, `session_already_traded`, `entry_taken`.

---

## Universe

| `universe_mode` | Behavior |
|-----------------|----------|
| `preset` (default) | `universe_preset`: `volatile_phase1` (~18 symbols: volatile FX, XAU/XAG, BTC, indices, oil) |
| `watchlist` | `scanner.watchlist` array |
| `account` | All OANDA instruments filtered by `universe_filters` (types, exotics, quote currencies) |

---

## Key config knobs

| Key | Effect |
|-----|--------|
| `opening_range_candles` | Width/stability of OR (default 2 M15) |
| `min_range_spread_ratio` | Filters choppy/thin ranges |
| `min_setup_score` | Minimum quality to rank |
| `take_profit_rr` | Reward vs stop |
| `force_flat_utc` | Session-end flat per asset class |
| `risk_per_trade_pct` | Position size |
| `poll_seconds` | Scan frequency |

See `docs/demo_trading_loop.md` for tweak workflow (one knob at a time, 30+ closed trades before live).

---

## vs other platform bots

| | `universe_scanner` | `fx_sentiment` | `btc_cfd` |
|--|-------------------|----------------|-----------|
| Signal | OR breakout + score | Range/trend + sentiment | M5 mean reversion |
| Universe | Many instruments | FX | BTC_USD only |
| Max open | 1 | Configurable | Own limits |

Shared stack: `oanda`, `risk`, `execution`, `monitor`, `journal`, `state`, `bot`.

**Deep dive:** `docs/fx_sentiment_bot_spec.md` for the sentiment/range bot.

---

## Quick inspection commands

```bash
# One scan cycle, no orders (default dry-run)
go run ./cmd/scanner-test

# Live metrics from SQLite
go run ./cmd/bot-metrics -bot universe_scanner

# P&L + tweak suggestions
go run ./cmd/bot-analyze -bot universe_scanner

# Tail closed trades
tail -f logs/universe_scanner/trades.jsonl
```
