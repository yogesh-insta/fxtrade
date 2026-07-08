# FX Sentiment Bot (`fx_sentiment`) — Logic Reference

Range/trend FX strategy with a **Finnhub + Groq sentiment gate**. Supports multiple instruments; one strategy engine per pair, cycled sequentially each tick.

| Item | Value |
|------|-------|
| Bot ID | `fx_sentiment` (legacy alias: `range_trend`) |
| Display name | FX Sentiment Strategy |
| Systemd | `fxtrade@fx_sentiment.service` |
| Health port | `:8082` |
| Config blocks | `strategy`, `range_mode`, `trend_mode`, `llm_gate`, `sentiment`, `risk`, `instruments` |
| State | `data/state-fx_sentiment.json` |
| Trades DB | `data/fx_sentiment/trades.db` |
| Journal | `logs/fx_sentiment/` |
| Sentiment audit | `logs/sentiment/` |
| Design spec (historical) | `plan.md` |
| One-shot tests | `go run ./cmd/strategy-test`, `go run ./cmd/sentiment-test` |

**API keys required:** `finnhub.api_key`, `llm.api_key` (Groq).

---

## Source map (read these, not the whole repo)

| Topic | File |
|-------|------|
| Bot wiring (engines, sentiment worker) | `internal/bots/fx_sentiment/bot.go` |
| Strategy cycle loop | `internal/strategy/engine.go` |
| Range detection + mode selection | `internal/strategy/range.go` |
| Market gates, sentiment veto, trend entry | `internal/strategy/gates.go` |
| Modes / types | `internal/strategy/types.go` |
| OANDA candles + indicators | `internal/market/snapshot.go` |
| Finnhub fetch → Groq → cache | `internal/sentiment/worker.go` |
| Sentiment signal shape | `internal/sentiment/types.go` |
| Config defaults | `internal/config/strategy.go` |
| Config example | `.credentials.example` |

---

## Architecture (two parallel loops)

```
┌─────────────────────────┐     ┌──────────────────────────────┐
│ Sentiment worker        │     │ Strategy engines (per pair)    │
│ every sentiment.interval│     │ every strategy.cycle_minutes │
│ Finnhub → normalize     │     │ snapshot → mode → trade      │
│ → Groq → per-instrument │────►│ reads sentiment cache        │
│   cache                 │     │ sequential RunAll()          │
└─────────────────────────┘     └──────────────────────────────┘
```

Default instruments: `AUD_USD`, `EUR_USD` (from `instruments` array). Engines run **sequentially** per tick so account-wide checks (correlation guard, sizing) do not race.

---

## Strategy cycle (`runCycle`)

Every `cycle_minutes` (default 30; example config 15):

```
1. Halted? → STAND_ASIDE, no trade
2. Load market snapshot (W/D/H4 candles, EMAs, ATR, RSI, 52w high/low)
3. Current sentiment from cache (may be stale / missing)
4. CheckMarketConditions — spread, rollover blackout, Friday blackout, event_risk=high
5. DetectRange → DetectMode → RANGE | TREND | STAND_ASIDE
6. If open position on this instrument → manage (TP1 partial, cancel opposite limits)
7. Else switch on mode:
     STAND_ASIDE → cancel pending limits
     RANGE       → place buy/sell limits at range boundaries (if gates pass)
     TREND       → market entry (if gates pass, 1h cooldown between attempts)
8. Email cycle digest (subject/body from report.go)
```

---

## Mode detection (`DetectMode`)

| Mode | When |
|------|------|
| **TREND** | Weekly close broke outside range zones, **or** strong weekly trend (EMA20W vs EMA50W aligned with price) |
| **RANGE** | Valid range band detected (see below) |
| **STAND_ASIDE** | No valid range and no trend regime |

On mode change: logs `mode_change`; cancels pending limits if `cancel_limits_on_mode_change` and leaving RANGE.

---

## Range detection (`DetectRange`)

Uses last `range_lookback_weeks × 5` daily candles (default 13 weeks ≈ 65 days):

1. **High/low** = max high / min low over lookback
2. **Zones** = boundary ± `zone_atr_multiplier × ATR14_daily` (default 0.5×)
3. **Buy limit** at range low; **sell limit** at range high; **TP** at midpoint
4. **Valid** when all hold:
   - `width_pips >= min_range_width_pips` (default 300; example config 120)
   - `touches_high >= min_boundary_touches` and `touches_low >= min_boundary_touches` (default 2)
   - Weekly EMA20/EMA50 separation ≤ `flat_ema_threshold_pct` of mid (default 0.8%; example 2.5%)

---

## RANGE mode behavior

When `pending_limits_enabled` (default true):

| Side | Place limit when |
|------|------------------|
| **Buy** at support | No existing buy limit; price `ask > buy_limit`; not vetoed by sentiment; no same-direction limit on another USD pair |
| **Sell** at resistance | No existing sell limit; price `bid < sell_limit`; not vetoed; correlation guard |

**Stops:** `stop_atr_beyond_boundary × ATR14_daily` beyond the breached boundary (default 1.0×).  
**Take profit:** range midpoint (limit order TP).  
**On fill:** optional `cancel_opposite_limit_on_fill`; while open, **TP1** closes 50% when mid crossed (`tp1_partial`).

**Sentiment veto (range):**
- Block buy if `event_risk=high` or bearish `base_bias` with confidence ≥ `veto_confidence`
- Block sell if `event_risk=high` or bullish `base_bias` with confidence ≥ `veto_confidence`

---

## TREND mode behavior

1. **1-hour cooldown** between trend entry attempts per engine
2. **TrendEntry gates:**
   - Weekly regime: long = EMA20W > EMA50W and mid > EMA20W (mirror for short)
   - Block near 52-week high/low within `week52_block_distance_pips`
   - H4 setup: price near EMA20H4 (within daily ATR), RSI in configured band, close on correct side of EMA
3. **Sentiment gates:**
   - Reject if sentiment direction disagrees with trend direction
   - Require `sentiment_persistence_readings` consecutive aligned readings (default 2) with confidence ≥ `veto_confidence`
4. **Entry:** market order; stop = `atr_stop_multiplier × ATR14_daily` (default 2.5×); no fixed TP on trend entries

---

## Sentiment pipeline

Every `sentiment.interval_minutes` (default 60, min enforced; example 15):

1. **Fetch** Finnhub forex news + economic calendar (+ RSS in fetcher)
2. **Normalize** headlines/events
3. Per instrument: build price context → JSON payload → **Groq LLM** → `SentimentSignal`
4. Store in per-instrument **cache**; audit to `logs/sentiment/`

**Signal fields used by strategy:**

| Field | Use |
|-------|-----|
| `direction` | LONG / SHORT / FLAT — trend alignment & persistence |
| `base_bias` | bullish / bearish — range veto |
| `confidence` | veto threshold, full-size sizing |
| `event_risk` | `high` blocks all new entries |

**Sizing confidence:** `SentimentConfidence` returns 0.5 default; aligned high-confidence sentiment can increase position size via `risk_per_trade_pct_high_conf`.

---

## Market condition gates (`CheckMarketConditions`)

Blocks new activity when:

| Gate | Rule |
|------|------|
| Spread | `spread_pips > risk.max_spread_pips` |
| Rollover | NY 16:45–18:15 |
| Friday | After 15:00 NY |
| Event risk | LLM `event_risk=high` |

---

## Risk and position limits

From shared `risk` config (bot uses `HaltFileForBot`):

| Key | Typical role |
|-----|----------------|
| `max_open_positions` | Account-wide cap (example 3) |
| `allocated_capital_usd` | Sizing slice per bot on shared account |
| `risk_per_trade_pct_base` / `_high_conf` | Unit sizing via stop distance |
| `max_daily_loss_pct` / `max_weekly_loss_pct` | Kill switch |
| `max_trades_per_month` | Live only |

**Correlation guard:** won't place a buy limit on one USD pair if another pair already has a pending buy limit (same for sells).

---

## Key config knobs

| Key | Effect |
|-----|--------|
| `strategy.cycle_minutes` | How often each instrument is evaluated |
| `sentiment.interval_minutes` | How often news → LLM runs |
| `range_mode.min_boundary_touches` | Strictness of range validity |
| `range_mode.min_range_width_pips` | Minimum tradeable range width |
| `llm_gate.sentiment_persistence_readings` | Trend entries need N aligned readings |
| `llm_gate.veto_confidence` | Threshold to veto limits / require persistence |
| `risk.max_open_positions` | Cap concurrent positions |
| `instruments` | Pairs to run (one engine each) |

See `docs/demo_trading_loop.md` for tweak workflow.

---

## vs other platform bots

| | `fx_sentiment` | `universe_scanner` | `btc_cfd` |
|--|----------------|-------------------|-----------|
| Signal | Range limits + trend breakout + LLM gate | Opening-range breakout score | M5 mean reversion |
| Cadence | 15–30 min cycle | 5–10 s poll | M5 candle-aligned |
| External APIs | Finnhub + Groq | OANDA only | OANDA only |
| Order types | Limit (range) + market (trend) | Market | Market |

---

## Quick inspection commands

```bash
# One strategy cycle (mode + gates, no orders if dry-run)
go run ./cmd/strategy-test

# One sentiment cycle (news → Groq → JSON)
go run ./cmd/sentiment-test

# Backtest quant gates on history
go run ./cmd/backtest -days 200

# Closed-trade stats
go run ./cmd/bot-metrics -bot fx_sentiment
go run ./cmd/expectancy -trades logs/fx_sentiment/trades.jsonl

# Tail strategy decisions
tail -f logs/fx_sentiment/journal.jsonl
tail -f logs/sentiment/$(date +%Y-%m-%d).jsonl
```

---

## Maintenance

Keep this doc aligned with code: **`docs/bot_specs_maintenance.md`**. CI runs `./scripts/verify-bot-spec-paths.sh` on every PR.
