# 📊 BTC CFD Automated Trading Bot (OANDA) — Full System Specification (v2)

## 0. 🏗 Platform Integration (fxtrade)

Standalone bot in the fxtrade monorepo; runs as its own systemd unit on the GCP VM alongside other bots.

| Item | Value |
|------|-------|
| Bot ID | `btc_cfd` |
| Systemd unit | `fxtrade@btc_cfd.service` (template `deploy/gcp/fxtrade@.service`) |
| CLI | `fxtrade -credentials /opt/fxtrade/.credentials -health-addr :8083 -bot btc_cfd` |
| Health port | **:8083** (`install.sh --enable-bot btc_cfd` sets `FXTRADE_HEALTH_ADDR=:8083`) |
| State file | `data/state-btc_cfd.json` (via `StateFileForBot`) |
| Halt file | `.halt.btc_cfd` |
| Journal | `logs/btc_cfd/` (`journal.jsonl`, `trades.jsonl`) |
| Trade DB (primary) | `data/btc_cfd/trades.db` (SQLite on VM) |
| Config block | `.credentials` → `btc_cfd` JSON + shared `oanda`, `risk`, `email` |

**Enable in `.credentials`:**
```json
{
  "bots": { "enabled": ["btc_cfd"] },
  "btc_cfd": {
    "instrument": "BTC_USD",
    "granularity": "M5",
    "db_path": "data/btc_cfd/trades.db",
    "journal_dir": "logs/btc_cfd"
  }
}
```

### Demo → live switch

No code changes. Set `oanda.environment` in `.credentials`:

- `"practice"` — OANDA demo (default; use for paper trading)
- `"live"` — production API (`api-fxtrade.oanda.com`)

Restart the unit after editing credentials: `sudo systemctl restart fxtrade@btc_cfd`.

### Storage: SQLite + GCS backup

- **Primary:** SQLite `trades` table on the VM (`internal/store/sqlite`); `InsertTrade` on position close (wired via monitor close hook).
- **Backup (documented stub):** `deploy/gcp/backup-btc-data.sh` copies `trades.db` and `logs/btc_cfd/` to `gs://BUCKET/btc_cfd/YYYYMMDD/` via `gsutil`. Schedule nightly cron after bucket is provisioned (e.g. `0 3 * * * fxtrade /opt/fxtrade/deploy/gcp/backup-btc-data.sh`).

### Reused platform modules

`internal/oanda`, `risk`, `execution`, `monitor`, `journal`, `state`, `notify`, `health`, `bot` platform — same patterns as `universe_scanner` / `range_trend`.

### Implementation status (foundation)

| Component | Status |
|-----------|--------|
| Bot registration + `Start()` skeleton | Done |
| M5 cycle loop (candle fetch, log tick) | Done |
| Risk / state / monitor / health wiring | Done |
| SQLite trade store on close | Done (minimal fields; strategy fields stubbed) |
| Signal engine, indicators, orders | **Not yet** — see §4–§7 |

---

## 1. 🎯 Objective

Build a fully automated trading system that:

- Trades BTC/USD CFD on OANDA
- Focuses on small, frequent profits
- Uses mean reversion + trend filtering
- Executes trades with strict, internally consistent risk management
- Runs continuously with minimal supervision, and fails safe when it can't

---

## 2. 🧠 Strategy Overview

**Core Strategy: Mean Reversion**
- Enter when price is overextended relative to a short-term mean (RSI extremes + EMA50 deviation)
- Exit on small pullbacks toward the mean

**Supporting Filter: Trend Filter**
- Avoid trading against strong trends
- Use EMA200 as directional bias

**Supporting Filter: Volatility Regime**
- ATR is not optional — it gates entries and sizes both SL and TP dynamically (see §5, §6)
- BTC's volatility regime shifts fast; a static % stop will get chopped in high-vol periods and sit too loose in quiet ones

---

## 3. 📥 Data Inputs

**Market Data (OANDA API)**
- Instrument: `BTC_USD`
- Timeframe: M5 (primary), M15 (confirmation, see §4)
- Candles: last 200 (enough for EMA200 warm-up)

**Fields:** Open, High, Low, Close, Volume

**Indicators**
- RSI — period configurable, **default 21, not 14** (see rationale in §4)
- EMA50
- EMA200
- ATR(14) — mandatory, drives SL/TP and the volatility filter

**Optional Inputs (Phase 2+)**
- News sentiment (Gemini)
- Macro data (EUR/USD, NASDAQ, Gold)

---

## 4. ⚙️ Signal Logic

### Overtrading fix
RSI(14) on a 5-minute BTC chart will breach 30/70 constantly — that's noise, not signal. Two changes:
- Use a longer/smoothed RSI (start with RSI(21), tune in backtest)
- Require RSI to **cross back** through the threshold (e.g., was <30, now ≥30) rather than firing on breach — this confirms the reversion has actually started

### EMA50 deviation — must be a defined number, not a qualitative check
Define deviation in ATR multiples so it's testable and consistent with the volatility filter:

```
deviation = (Close - EMA50) / ATR(14)
```

- LONG requires `deviation <= -1.5` (price is at least 1.5 ATR below EMA50)
- SHORT requires `deviation >= +1.5` (price is at least 1.5 ATR above EMA50)

(1.5 is a starting point — tune in backtest, don't hardcode without validation.)

### LONG Entry Conditions (all must hold)
1. RSI crossed back above 30 (from below)
2. Close > EMA200 (uptrend bias)
3. `deviation <= -1.5` (oversold vs EMA50)
4. Spread ≤ max_spread_threshold
5. ATR within acceptable band (not in the volatility-spike exclusion zone, §4 HOLD)
6. No open position (see §6)
7. Daily trade count < max_trades AND daily PnL > -max_daily_loss

### SHORT Entry Conditions (all must hold)
1. RSI crossed back below 70 (from above)
2. Close < EMA200 (downtrend bias)
3. `deviation >= +1.5` (stretched above EMA50)
4. Spread ≤ max_spread_threshold
5. ATR within acceptable band
6. No open position
7. Daily trade count < max_trades AND daily PnL > -max_daily_loss

### HOLD Conditions — no trade if:
- RSI between 30–70 (no extreme, or extreme not yet confirmed by cross-back)
- ATR > `atr_spike_multiple × ATR_sma(20)` (volatility spike — regime is trending/breaking out, mean reversion logic doesn't apply)
- Spread > max_spread_threshold
- Current price has moved more than `max_slippage_pct` from the price that generated the signal (staleness check — see §7)
- Outside OANDA's tradable hours/liquidity window for BTC_USD (see §8)

### Optional M15 confirmation (Phase 2)
Require the M15 EMA200 bias to agree with M5 before entry — reduces false signals at the cost of trade frequency. Flag as a backtest variant, not a day-one requirement.

---

## 5. 💰 Risk Management

### Per-Trade Risk
- Risk: **0.5% of account per trade** (fixed, not 0.5–1% — see reconciliation with daily cap below)
- Position size calculated dynamically from SL distance (see formula below)

### Stop Loss (ATR-based, primary method)
```
SL_distance = ATR(14) × sl_atr_multiple      # start: sl_atr_multiple = 1.5
```
A fixed-% SL (0.3–0.5%) can be used as a **fallback only** if ATR is unavailable (e.g., data gap) — never as the primary method, since it doesn't adapt to regime.

### Take Profit — derived from SL, not chosen independently
```
TP_distance = SL_distance × target_RR         # target_RR minimum 1.2, default 1.5
```
This guarantees RR is correct by construction rather than by coincidence between two separately-picked ranges. If the resulting TP_distance falls outside a sanity band (e.g., <0.2% or >1.5% of price), skip the trade rather than force it — that signals the ATR-derived stop is unusually large/small for current conditions.

### Position Sizing Formula
```
account_risk_amount = account_balance × per_trade_risk_pct   # 0.005
position_size = account_risk_amount / SL_distance
```
Round to instrument's minimum tradable unit; if resulting size is below the broker minimum, skip the trade (don't round up and silently increase risk %).

### Risk/Reward
- Minimum: 1.2 (enforced structurally via TP formula above)
- Default target: 1.5

### Daily Limits — reconciled with per-trade risk
With 0.5% risk per trade:
```
max_daily_loss = 2.5%           # 5 losing trades × 0.5% = 2.5%, consistent
max_trades_per_day = 5
```
If per-trade risk is later changed, **recompute max_trades from max_daily_loss / per_trade_risk_pct** — these two numbers must never be set independently again. Stop opening new trades once either cap is hit, whichever comes first.

---

## 6. 🧮 Position Management

- Only one open trade at a time
- No averaging down, no martingale, no pyramiding
- **Startup reconciliation (new):** on every bot start/restart, before entering the normal cycle, query OANDA for open positions and pending orders. If a position exists that the bot's local state doesn't know about, load it into state (with its SL/TP) rather than treating the account as flat. Do not evaluate new entry signals until this reconciliation step completes.

---

## 7. 🔄 Execution Cycle

**Frequency:** every 5 minutes, aligned to candle close (not wall-clock drift)

**Pipeline Flow**
1. Fetch candles (OANDA)
2. Calculate indicators (RSI, EMA50, EMA200, ATR)
3. **Reconcile state** — confirm bot's view of open positions matches OANDA's (see §6)
4. Evaluate signal (§4)
5. Validate conditions (spread, ATR band, daily limits)
6. **Staleness/slippage check (new):** compare current bid/ask to the price the signal was computed on; if drift exceeds `max_slippage_pct`, abort this cycle rather than send a stale-priced order
7. Calculate position size (§5)
8. Place order with idempotency key (§8) including SL + TP
9. Confirm fill and log actual fill price/size (don't assume request success = fill)
10. Log trade

---

## 8. 📡 OANDA Integration

**Endpoints Used**
- `GET` candles
- `GET` account summary
- `GET` open positions / pending orders
- `POST` order

**Order Type**
- MARKET order, including `stopLossOnFill` and `takeProfitOnFill`

**Idempotency (new)**
- Every order request includes a unique client-generated order ID (e.g., UUID tied to signal timestamp + direction)
- Before retrying a failed/timed-out request, first re-query open positions/orders to check whether the original request actually succeeded server-side. Only resubmit if confirmed absent — never retry blindly on a timeout.

**Tradable hours / liquidity (new)**
- Confirm OANDA's actual BTC_USD CFD trading schedule (it is not identical to 24/7 spot crypto) and document it here once confirmed.
- Treat the open/close edges of that window as a heightened-spread period — the existing spread filter should be validated specifically against this window in backtesting/paper trading, not just against intraday spikes.

**Financing/swap costs (new)**
- CFD positions held overnight accrue financing charges. At 0.4–0.8% TP targets, multi-day swap costs on a slow-to-resolve trade can materially erode edge.
- Track swap cost per trade in the logger (§11) and include it in net PnL, not just gross.
- Consider a max-hold-time rule (e.g., force-close or tighten trailing stop if a position is open >N hours without reaching TP) — flag as a Phase 1 backtest variable.

---

## 9. 🧱 System Architecture

### Modules

**1. Data Layer**
- Fetch candles, fetch account info, fetch open positions/orders

**2. Indicator Engine**
- RSI, EMA, ATR, ATR_SMA (for spike detection)

**3. Strategy Engine**
- Signal generation, entry/hold filters, EMA50-deviation calc

**4. Risk Engine**
- Position sizing, SL/TP derivation, daily loss/trade-count tracking (single source of truth — §5)

**5. Execution Engine**
- Order placement with idempotency key, fill confirmation, position tracking, startup reconciliation

**6. Scheduler**
- Runs every 5 minutes, aligned to candle boundaries

**7. Logger**
- Trades (signal price, fill price, slippage, SL, TP, swap cost, net PnL)
- Errors (API failures, retries, aborted cycles and why)
- Performance (rolling win rate, drawdown, daily PnL vs. cap)

**8. Alerting (new)**
- Push/Telegram/email notification on: kill-switch triggered, daily loss limit hit, repeated API failures, position reconciliation mismatch on startup
- Do not rely on manual log-checking to discover these events

---

## 10. 🛑 Safety Controls

- No trade if API fails — abort cycle, log, alert if failures repeat past a threshold
- Retry logic for requests — **gated by idempotency check** (§8), not blind retry
- Skip if spread too high
- Skip during abnormal volatility (ATR spike band, §4)
- Skip on price staleness/slippage beyond threshold (§7)
- Startup reconciliation before any new entries (§6)
- Kill switch (manual stop) — **must trigger an alert**, not just halt silently
- Max-hold-time safeguard for stuck positions (§8, swap cost consideration)

---

## 11. 📊 Metrics to Track

- Win rate
- Avg win / avg loss (gross and net of swap costs)
- Realized risk/reward ratio (actual, vs. target)
- Max drawdown
- Daily PnL vs. daily loss cap (headroom remaining)
- Slippage per trade (signal price vs. fill price)
- Swap cost accrued per trade and cumulatively

---

## 12. 🧪 Testing Plan

**Phase 1 — Backtest**
- Historical data across multiple volatility regimes (trending and ranging periods, not just one favorable window)
- **Explicit walk-forward methodology:** split data into in-sample (parameter tuning: RSI period, deviation multiple, ATR multiples, target RR) and out-of-sample (validation only — no further tuning on this data)
- No parameter reuse across the split; if out-of-sample results diverge significantly from in-sample, treat the strategy as overfit and revisit before proceeding

**Phase 2 — Paper Trading (demo account)**
- Run live pipeline end-to-end, including idempotency, reconciliation, and alerting, against a demo account
- Specifically validate spread/liquidity behavior at session open/close edges (§8)

**Phase 3 — Small capital live trading**
- Start at minimum viable position size
- Confirm regulatory/leverage terms for BTC CFDs in your jurisdiction and OANDA's current offering before committing capital — this is a broker/regulatory question, not a strategy one, and worth resolving before scaling up (this note isn't financial or legal advice — check OANDA's current terms and local regulations directly)

---

## 13. 🚀 Future Enhancements

**Phase 2**
- Gemini sentiment filter
- News-based trade blocking
- M15 confirmation filter (§4)

**Phase 3**
- Multi-timeframe confirmation
- Volatility regime detection (formalize beyond the ATR-spike gate)

**Phase 4**
- Machine learning optimization — **must reuse the Phase 1 walk-forward discipline** (separate train/validation/test splits); ML optimization without this is the fastest path to an overfit, live-losing bot

---

## 14. ⚠️ Constraints & Reality

- No strategy is always profitable
- Sideways market = best performance
- Trending market = biggest risk
- Discipline > prediction
- Automated ≠ unsupervised — alerting and reconciliation exist because "runs with minimal supervision" still requires the system to fail safely and loudly, not silently

---

## 15. 🧠 Design Philosophy

- Rule-based > prediction-based
- Risk control > profit chasing
- Consistency > high returns
- Internal consistency of the risk formulas (§5) is a prerequisite for everything else — a bot that violates its own stated limits on paper will violate them live

---

## ✅ Final Goal

Build a system that:
- Survives long-term
- Produces steady returns
- Avoids large drawdowns
- Fails safely and audibly, not silently

**NOT** a "get rich quick" bot.
