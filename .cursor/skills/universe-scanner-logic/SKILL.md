---
name: universe-scanner-logic
description: Explains universe_scanner (FXPulse ORB) opening-range breakout logic, config, source files, and scan cycle. Use when the user asks about scanner bot logic, universe_scanner, opening range, ORB, breakout scoring, force flat, or how the scanner ranks and enters trades.
---

# Universe Scanner Logic

**Do not run a broad codebase search** for routine logic questions. Read the canonical doc and source map first.

## Canonical reference

Full logic write-up: **`docs/universe_scanner_bot_spec.md`** — prefer this for explanations.

## Source map (targeted reads only)

| Question | Read |
|----------|------|
| Poll loop, entry, force-flat | `internal/scanner/engine.go` |
| Per-symbol scan steps | `internal/scanner/scan.go` |
| Opening range candles | `internal/scanner/range.go` |
| Score formula, rank, breakout | `internal/scanner/rank.go` |
| Session windows, force-flat UTC | `internal/scanner/session.go` |
| Symbol lists / universe | `internal/scanner/universe.go` |
| Config defaults | `internal/config/scanner.go` |
| Bot wiring (risk=1 pos, DB) | `internal/bots/universe_scanner/bot.go` |

## One-paragraph summary

Scans a preset/watchlist/account universe every `poll_seconds`. For each in-session symbol: build opening range from first N M15 candles after session open, score range quality vs spread and H1 bias, detect breakout (bid above high = LONG, ask below low = SHORT). Rank passing setups; trade top only if no open scanner position and not already traded that symbol this session. Market entry with class-specific stop and `take_profit_rr` TP. Force-flat at `force_flat_utc` per asset class.

## Key defaults

- `min_setup_score` 0.65, `min_range_spread_ratio` 3.0, `opening_range_candles` 2, `take_profit_rr` 2.1
- `max_open_positions` 1 (via scanner risk in `bot.go`)
- `poll_seconds` 10

## Runtime probes

```bash
go run ./cmd/scanner-test          # one cycle, dry-run
go run ./cmd/bot-metrics -bot universe_scanner
go run ./cmd/bot-analyze -bot universe_scanner
```

## Related (not this bot)

- **P/L tuning workflow:** `.cursor/skills/analyze-pl-tweaks/SKILL.md`
- **FX sentiment bot:** `docs/fx_sentiment_bot_spec.md` + `.cursor/skills/fx-sentiment-logic/SKILL.md`
- **BTC bot spec:** `docs/btc_cfd_bot_spec.md`
- **Email-only scanners:** `nifty-pulse`, `afl-pulse` under `cmd/` — no OANDA orders

## Keeping this current

When scanner logic changes, update this skill and `docs/universe_scanner_bot_spec.md` in the same PR. See **`docs/bot_specs_maintenance.md`** and run `./scripts/verify-bot-spec-paths.sh`.
