---
name: fx-sentiment-logic
description: Explains fx_sentiment range/trend strategy, Finnhub/Groq sentiment gate, mode detection, limit orders, and trend entries. Use when the user asks about fx_sentiment, range_trend, sentiment strategy, RANGE/TREND mode, LLM gate, boundary limits, or Groq/Finnhub pipeline.
---

# FX Sentiment Logic

**Do not run a broad codebase search** for routine logic questions. Read the canonical doc and source map first.

## Canonical reference

Full logic write-up: **`docs/specs/fx_sentiment_bot_spec.md`** — prefer this for explanations.

Historical design doc: **`plan.md`** (AUD/USD master plan; implementation now supports multiple instruments).

## Source map (targeted reads only)

| Question | Read |
|----------|------|
| Bot wiring, sentiment worker | `internal/bots/fx_sentiment/bot.go` |
| Main cycle, range/trend execution | `internal/strategy/engine.go` |
| Range band + mode detection | `internal/strategy/range.go` |
| Spread/rollover gates, sentiment veto, trend entry | `internal/strategy/gates.go` |
| Market data / indicators | `internal/market/snapshot.go` |
| Finnhub → Groq → cache | `internal/sentiment/worker.go` |
| Config defaults | `internal/config/strategy.go` |

## One-paragraph summary

Two loops: sentiment worker fetches Finnhub news and calls Groq per instrument on an interval; strategy engines run every `cycle_minutes`, load OANDA W/D/H4 snapshot, detect a weekly range band or trend regime (RANGE / TREND / STAND_ASIDE). RANGE places GTC buy/sell limits at support/resistance with ATR stops and mid TP, vetoed by bearish/bullish LLM bias. TREND takes market entries on weekly+H4 setup with sentiment alignment and persistence. Manages open trades with TP1 at midpoint. Gates: spread, NY rollover, Friday afternoon, `event_risk=high`.

## Key defaults

- `cycle_minutes` 30, `sentiment.interval_minutes` 60 (example `.credentials` uses 15 for both)
- Range: 13-week lookback, min width 300 pips, 2 boundary touches, pending limits enabled
- LLM gate: `veto_confidence` 0.65, `sentiment_persistence_readings` 2
- Instruments default `AUD_USD`, `EUR_USD`

## Runtime probes

```bash
go run ./cmd/strategy-test
go run ./cmd/sentiment-test
go run ./cmd/bot-metrics -bot fx_sentiment
go run ./cmd/backtest -days 200
```

## Related (not this bot)

- **P/L tuning:** `.cursor/skills/analyze-pl-tweaks/SKILL.md`
- **ORB scanner:** `docs/specs/universe_scanner_bot_spec.md` + `.cursor/skills/universe-scanner-logic/SKILL.md`
- **BTC bot:** `docs/specs/btc_cfd_bot_spec.md`

## Keeping this current

When strategy or sentiment logic changes, update this skill and `docs/specs/fx_sentiment_bot_spec.md` in the same PR. See **`docs/guides/bot_specs_maintenance.md`** and run `./scripts/code-quality.sh`.
