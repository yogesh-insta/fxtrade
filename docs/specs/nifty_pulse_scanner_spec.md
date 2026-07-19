# NiftyPulse Scanner (`nifty-pulse`) — Logic Reference

Daily NSE swing scanner. Screens the Nifty 200 and Nifty 500 (ex-200) universes for pullbacks inside an uptrend, applies an LLM sentiment gate, and **emails up to two picks**. It does **not** place orders — it is an alert-only scanner, not a platform bot.

| Item | Value |
|------|-------|
| Binary | `cmd/nifty-pulse` |
| Schedule | `nifty-pulse.timer` — 18:00 Australia/Sydney, Sun–Fri |
| On demand | `sudo systemctl start nifty-pulse.service` or `./scripts/nifty-pulse-run.sh` |
| Safe test | `go run ./cmd/nifty-pulse -dry-run` |
| Config block | `.credentials` → `stock_scan` (+ shared `email`, `notifications`, `llm`) |
| Watchlists | `watchlist.txt` (Nifty 200), `watchlist-nifty500-rest.txt` (Nifty 500 ex-200) |
| Data source | Yahoo Finance daily charts + Groq/LLM sentiment + RSS feeds |
| Output | Email alert (no orders, no state DB) |

---

## Source map (read these, not the whole repo)

| Topic | File |
|-------|------|
| Entry point, two-universe run, dry-run | `cmd/nifty-pulse/main.go` |
| Scan pipeline (fetch → filter → rank) | `internal/stockscan/scan.go` |
| SMA / RSI / filter / SL-TP levels | `internal/stockscan/indicators.go` |
| Ranking, contenders, reasons | `internal/stockscan/rank.go` |
| Yahoo Finance client | `internal/stockscan/yahoo.go` |
| LLM sentiment gate | `internal/stockscan/sentiment.go` |
| Email formatting + universe labels | `internal/stockscan/notify.go` |
| Config defaults + keys | `internal/config/config.go` (`StockScanConfig`, `DefaultStockScanConfig`) |

---

## Code organisation & storage (where to find things)

### Code layout

```
cmd/
└── nifty-pulse/main.go      # entry point: two-universe run, dry-run, email dispatch
internal/
├── stockscan/               # all NiftyPulse logic
│   ├── scan.go              # LoadWatchlist, ScanAll, scanOne, RunSymbols
│   ├── indicators.go        # SMA, RSI, PassesFilter, Levels (SL/TP)
│   ├── rank.go              # RankByRSI, BuildReasons, BuildContenders
│   ├── yahoo.go             # Yahoo Finance daily-chart client
│   ├── sentiment.go         # LLM sentiment gate (PickWithSentiment)
│   └── notify.go            # email formatting + universe labels
└── config/config.go         # StockScanConfig + DefaultStockScanConfig
watchlist.txt                # Nifty 200 symbols (input)
watchlist-nifty500-rest.txt  # Nifty 500 ex-200 symbols (input)
scripts/nifty-pulse-run.sh   # manual run helper
```

Uses the shared `internal/notify` (email) and `internal/config` `llm` block (sentiment). It does **not** import the `bot` platform, `oanda`, `risk`, `execution`, or `state`.

### Runtime artifacts

NiftyPulse is **stateless** — a scheduled one-shot that reads watchlists and emails picks. There is no state file, trades DB, journal, or on-disk cache.

| Artifact | Path | Notes |
|----------|------|-------|
| **Inputs** | `watchlist.txt`, `watchlist-nifty500-rest.txt` | NSE symbols, one per line (`#` comments allowed) |
| **Yahoo data** | in-memory per run | Fetched fresh each run; no cache on disk |
| **Sentiment** | in-memory per run | RSS + LLM; not persisted |
| **Output** | email (via shared `email` config) | The only durable output; `-dry-run` logs instead |
| **Process logs** | systemd journal (`journalctl -u nifty-pulse`) | slog → stdout |

---

## Algorithm diagram

```mermaid
flowchart TD
    A[Timer fires 18:00 Sydney] --> B[Load watchlist.txt + watchlist-nifty500-rest.txt]
    B --> C[Drop Nifty-200 symbols from Nifty-500 list]
    C --> D{For each universe:<br/>Nifty 200, then Nifty 500 ex-200}
    D --> E[ScanAll concurrently: Yahoo daily bars per symbol]
    E --> F[Per symbol: SMA(50), RSI(14)]
    F --> G{PassesFilter?<br/>close &gt; SMA AND rsi_min ≤ RSI ≤ rsi_max}
    G -- no --> H[Drop]
    G -- yes --> I[RankByRSI ascending: most oversold first]
    I --> J[Take top sentiment_candidates]
    J --> K[LLM sentiment gate: PickWithSentiment]
    K --> L{Pick survives gate?}
    L -- no --> M[No pick for this universe]
    L -- yes --> N["BuildPick: entry = close,<br/>SL = entry·(1−stop_loss_pct),<br/>target = entry·(1+target_pct)"]
    N --> O[Collect pick + top-5 contenders]
    M --> P
    O --> P{Any picks across universes?}
    P -- no --> Q[No email sent]
    P -- yes --> R[Email one BUY per universe with reasons + contenders]
```

---

## Algorithm

### 1. Universes
Two runs per invocation:
1. **Nifty 200** — from `watchlist.txt`.
2. **Nifty 500 (ex-200)** — from `watchlist-nifty500-rest.txt`, with any symbol already present in the Nifty 200 list removed (`ExcludeSymbols`).

Each watchlist is one NSE symbol per line; blank lines and `#` comments ignored, symbols uppercased, duplicates dropped.

### 2. Per-symbol scan (`scanOne`)
For each symbol (up to `concurrency` in parallel, `rate_limit_ms` between Yahoo calls):
1. Fetch daily bars from Yahoo Finance.
2. Compute `SMA(sma_period)` and `RSI(rsi_period)` over closes. Skip if insufficient history.
3. `PassesFilter(close, sma, rsi, rsi_min, rsi_max)`:
   - `close > SMA` — price above moving average (**uptrend filter**), and
   - `rsi_min ≤ RSI ≤ rsi_max` — RSI in the pullback zone (**oversold bounce within uptrend**).

### 3. Rank
Passing candidates are sorted ascending by RSI (`RankByRSI`) — the most oversold pullback ranks first (ties broken by symbol name).

### 4. Sentiment gate
The top `sentiment_candidates` (default 3) go through `PickWithSentiment`, which pulls news (RSS + LLM) and returns the best candidate that survives the sentiment check for that universe. If none survive, the universe yields no pick.

### 5. Trade levels & alert
For the selected candidate (`BuildPickUniverse` → `Levels`):

| Field | Formula |
|-------|---------|
| Entry | last close (rounded INR) |
| Stop loss | `entry × (1 − stop_loss_pct)` |
| Target | `entry × (1 + target_pct)` |

The email contains up to two BUY picks (one per universe) with human-readable reasons (% above SMA, RSI pullback zone, optional H1 trend, sentiment note) plus the top-5 contenders per universe. With `-dry-run`, picks are logged and no email is sent.

---

## Configuration (`stock_scan` block)

Defaults from `DefaultStockScanConfig` (`internal/config/config.go`).

| Key | Default | Effect |
|-----|---------|--------|
| `concurrency` | 4 | Parallel Yahoo fetches |
| `request_timeout_seconds` | 15 | Per-request Yahoo timeout |
| `rate_limit_ms` | 300 | Delay between Yahoo calls |
| `overall_timeout_minutes` | 15 | Whole-run deadline (~500 chart calls) |
| `sma_period` | 50 | Trend-filter moving average |
| `rsi_period` | 14 | RSI lookback |
| `rsi_min` | 30 | Lower bound of pullback zone |
| `rsi_max` | 45 | Upper bound of pullback zone |
| `stop_loss_pct` | 0.02 | Stop distance below entry (2%) |
| `target_pct` | 0.03 | Target distance above entry (3%) |
| `sentiment_candidates` | 3 | Top-N sent through the LLM sentiment gate |
| `rss_feeds` | — | RSS sources for sentiment context |

Email routing uses shared `email` + `notifications` (`EffectiveNSEPrefix`). Sentiment uses the shared `llm` block.

### Key knobs
- **`rsi_min` / `rsi_max`** — widen for more candidates, tighten for stricter pullbacks.
- **`sma_period`** — the uptrend gate; larger = stronger trend requirement.
- **`stop_loss_pct` / `target_pct`** — fixed R:R on the suggested levels (default ~1.5:1).
- **`sentiment_candidates`** — more candidates = more LLM calls but better odds of a pick surviving the gate.

---

## Quick inspection commands

```bash
# Scan both universes, log picks, no email
go run ./cmd/nifty-pulse -dry-run

# Custom watchlists
go run ./cmd/nifty-pulse -watchlist watchlist.txt -watchlist-extended watchlist-nifty500-rest.txt -dry-run
```

---

## vs other scanners / bots

| | `nifty-pulse` | `afl-pulse` | Platform bots |
|--|---------------|-------------|---------------|
| Market | NSE equities | AFL betting | OANDA FX/CFD |
| Action | Email alert | Email alert | Live orders |
| Signal | SMA uptrend + RSI pullback + sentiment | Positive-EV odds | See per-bot specs |

---

## Maintenance

Keep this doc aligned with code: **`docs/guides/bot_specs_maintenance.md`**. CI runs `./scripts/code-quality.sh` on every PR (verifies referenced paths exist).
