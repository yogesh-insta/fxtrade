# AUD/USD Automated Trading System — Master Plan

> **Status:** Planning phase. Practice account only until 4+ weeks of paper trading pass.
> **Stack:** Go 1.22+, OANDA v20 API, Finnhub API (free), central-bank RSS, Groq free LLM (upgrade to GPT-4o-mini when profitable).
> **Instrument:** `AUD_USD` only.

---

## 1. What You Are Building (Four Pillars)

This application has four distinct problems to solve. Each has a single, defined solution — no alternatives.

| Pillar | Problem | Solution |
|--------|---------|----------|
| **1. Data ingestion** | Where does news and macro data come from? | Finnhub API + official central-bank RSS feeds |
| **2. Sentiment intelligence** | How do you turn raw headlines into a trade signal? | Normalize into JSON → send to Groq free LLM → structured signal |
| **3. Trade execution** | Can the program actually go long/short on OANDA? | Yes — OANDA v20 market orders (positive units = long, negative = short) |
| **4. Risk & operations** | How do you avoid blowing up the account? | Hard risk rules + mandatory stop-loss/take-profit on every order + email alerts |

You are **not** buying a third-party "predict the price" product. Those do not work reliably for FX. You build the prediction logic yourself using **price data from OANDA** + **sentiment from the LLM**.

---

## 2. Executive Summary

A Go daemon running 24/7 on your laptop that:

1. **Detects the market mode daily** from weekly/daily candles: a tradeable **range** (support/resistance band, the usual state of AUD/USD), a **trend** (confirmed breakout or strong weekly move), or **neither** (stand aside, protect capital).
2. **RANGE mode (primary):** when a valid range is detected, **places pending LIMIT orders at both boundaries** — **buy at the bottom (support), sell at the top (resistance)** — and waits for price to come to you. Can wait days or weeks. On fill, attaches stop beyond the boundary and take-profit toward the midpoint / far edge.
3. **Every 60 minutes:** fetches forex headlines (Finnhub) + central-bank announcements (RSS) + macro calendar, sends one JSON bundle to the LLM (Groq free tier), and gets a structured sentiment signal.
4. **Uses news as a veto:** strongly bearish news at support blocks the buy (that's how floors break); aligned news allows full position size.
5. **Takes profit at price levels, not day counts:** half the position at the range midpoint, the rest near the opposite boundary. A breakout flips the system to TREND mode to ride the move with a trailing stop instead.
6. **Holds through weekends**; exits are triggered by price structure only. Hard risk limits (0.5–1% per trade, monthly trade cap, loss halts) protect capital at all times.
7. **Emails** you at you@example.com on every order event and mode change.

---

## 3. Pillar 1 — Where Data Comes From (Definitive)

Your program does **not scrape random websites**. It uses two official mechanisms:

### A. Finnhub API (primary news + calendar)

Sign up at [finnhub.io](https://finnhub.io). **Use the free tier ($0/month)** — it is enough for this project while you develop and paper-trade.

| Plan | Cost | What you get | When to use |
|------|------|--------------|-------------|
| **Free** | **$0** | Forex news, economic calendar, 60 API calls/min | **Now — development + paper trading** |
| Starter | $49/month | Higher rate limits, premium datasets | Only when you go live and need more volume |

**Why free tier is enough:** Your daemon polls every 30 minutes = ~48 news calls + ~48 calendar calls per day (~96 total). That is well within free limits. You do **not** need the $49 plan until you are live and profitable.

**Upgrade trigger:** Move to Starter only when you are trading live with real money and consistently hitting rate limits or need premium data feeds.

| Endpoint | URL | What you get | Frequency |
|----------|-----|--------------|-----------|
| Forex news | `GET https://finnhub.io/api/v1/news?category=forex&token=KEY` | Headlines with headline, summary, source, datetime | Every 30 min |
| Economic calendar | `GET https://finnhub.io/api/v1/calendar/economic?from=YYYY-MM-DD&to=YYYY-MM-DD&token=KEY` | Scheduled AU/US macro releases (CPI, NFP, RBA rate decision) with impact level | Every 30 min |

Example Finnhub forex news response (what your Go code receives):

```json
[
  {
    "category": "forex",
    "datetime": 1719900000,
    "headline": "Australian dollar rises after RBA holds rates",
    "id": 123456,
    "source": "Reuters",
    "summary": "The AUD gained after the Reserve Bank of Australia...",
    "url": "https://..."
  }
]
```

### B. Central bank RSS feeds (official, free, no API key)

Central banks do **not** offer trading APIs. They publish **RSS feeds** — XML documents your program polls on a schedule. Go library: `github.com/mmcdole/gofeed`.

| Source | RSS URL | What you get |
|--------|---------|--------------|
| RBA Media Releases | `https://www.rba.gov.au/rss/rss-cb-media-releases.xml` | Rate decisions, monetary policy statements |
| RBA Speeches | `https://www.rba.gov.au/rss/rss-cb-speeches.xml` | Governor speeches affecting AUD |
| Fed Monetary Policy | `https://www.federalreserve.gov/feeds/press_monetary.xml` | FOMC statements, Fed rate decisions |

RSS item structure (what your parser extracts):

```go
type Headline struct {
    Source    string    `json:"source"`     // "RBA", "Fed", "Finnhub"
    Title     string    `json:"title"`
    Summary   string    `json:"summary"`    // RSS description or Finnhub summary
    URL       string    `json:"url"`
    Published time.Time `json:"published"`
}
```

### C. Price data from OANDA (not Finnhub)

All price data for trading decisions comes from OANDA — the same broker you execute on.

| Endpoint | What you get | Frequency |
|----------|--------------|-----------|
| Pricing stream | Live bid/ask ticks | Continuous (WebSocket-like HTTP stream) |
| Candles `GET /v3/instruments/AUD_USD/candles` | OHLCV history for indicators | Every 5 minutes |
| Pricing snapshot `GET /v3/accounts/{id}/pricing` | Current spread | On demand before orders |

### What you do NOT use

- Web scraping of news sites (breaks, violates ToS, noisy)
- Twitter/X feeds (rate-limited, unreliable)
- Third-party "AI trading signal" subscriptions (black boxes, no audit trail)

---

## 4. Pillar 2 — Sentiment Pipeline (Data → LLM → Signal)

This is the core intelligence loop. It runs **every 30 minutes**, not on every price tick.

### Step-by-step flow

```
┌─────────────┐    ┌──────────────┐    ┌─────────────┐    ┌──────────────┐
│  Fetch raw  │───▶│  Normalize   │───▶│  Build LLM  │───▶│  Groq LLM    │
│  headlines  │    │  + dedupe    │    │  prompt     │    │  (JSON mode) │
│  + calendar │    │  + filter    │    │  payload    │    └──────┬───────┘
└─────────────┘    └──────────────┘    └─────────────┘           │
     ▲                                                              ▼
 Finnhub API                                                   ┌──────────────┐
 RBA RSS                                                       │ SentimentSignal│
 Fed RSS                                                       │ cache 30 min  │
 OANDA candles                                                 └──────┬───────┘
                                                                      │
                                                                      ▼
                                                               Strategy Engine
```

### Step 1 — Fetch (Go `internal/sentiment/fetcher.go`)

On a 30-minute ticker:

1. Call Finnhub forex news (last 24h).
2. Poll RBA + Fed RSS feeds; keep items from last 48h.
3. Call Finnhub economic calendar (today + tomorrow).
4. Pull last 60 weekly + 120 daily + 60 H4 candles from OANDA (weekly regime, daily ATR stops, H4 entry timing).

### Step 2 — Normalize (Go `internal/sentiment/normalizer.go`)

Raw data is messy. Your code must:

1. **Deduplicate** — same headline from multiple sources → keep one.
2. **Filter** — keep only AUD/USD-relevant items. Keywords: `AUD`, `USD`, `RBA`, `Fed`, `FOMC`, `Australia`, `US dollar`, `iron ore`, `commodity`, `rate`, `inflation`, `CPI`, `employment`, `NFP`.
3. **Cap size** — max 25 headlines (LLM context cost control).
4. **Tag upcoming events** — calendar items in next 4 hours get `event_risk: "high"`.

### Step 3 — Build LLM payload (Go `internal/sentiment/payload.go`)

Massage everything into one JSON document. This is the **only** thing sent to the LLM:

```json
{
  "task": "usd_aud_sentiment_analysis",
  "as_of": "2026-07-02T10:00:00Z",
  "instrument": "AUD_USD",
  "price_context": {
    "current_bid": 0.6900,
    "current_ask": 0.6902,
    "spread_pips": 2.0,
    "change_1h_pct": -0.08,
    "change_24h_pct": -0.35,
    "ema20_w": 0.6880,
    "ema50_w": 0.6750,
    "week52_high": 0.7250,
    "week52_low": 0.5950,
    "distance_to_52w_high_pips": 350,
    "distance_to_52w_low_pips": 950,
    "range_position_pct": 73,
    "atr14_daily_pips": 85,
    "trend": "bearish",
    "session": "london_ny_overlap"
  },
  "upcoming_events": [
    {
      "time": "2026-07-02T13:30:00Z",
      "country": "US",
      "event": "Non-Farm Payrolls",
      "impact": "high"
    }
  ],
  "headlines": [
    {
      "source": "RBA",
      "time": "2026-07-01T03:30:00Z",
      "title": "Statement by the Monetary Policy Board",
      "summary": "The Board decided to leave the cash rate unchanged at 4.35 per cent..."
    },
    {
      "source": "Finnhub/Reuters",
      "time": "2026-07-02T08:00:00Z",
      "title": "US dollar strengthens ahead of jobs data",
      "summary": "The greenback rose against major currencies..."
    }
  ]
}
```

### Step 4 — LLM call (Go `internal/sentiment/llm.go`)

**Use Groq free tier while developing ($0).** Upgrade to OpenAI GPT-4o-mini when you are live and profitable.

| Phase | Provider | Model | Cost | Sign up |
|-------|----------|-------|------|---------|
| **Now (dev + paper)** | **Groq** | `llama-3.3-70b-versatile` | **$0** | [console.groq.com](https://console.groq.com) |
| Later (live trading) | OpenAI | `gpt-4o-mini` | ~$0.50–1/day | [platform.openai.com](https://platform.openai.com) |

**Why Groq free is enough:** Your daemon calls the LLM every 30 minutes = ~48 requests/day. Groq free tier allows **1,000 requests/day** and **30 requests/minute**. No credit card required.

**API:** Groq uses the same format as OpenAI — just change the base URL:

```
POST https://api.groq.com/openai/v1/chat/completions
Authorization: Bearer GROQ_API_KEY
```

```json
{
  "model": "llama-3.3-70b-versatile",
  "response_format": { "type": "json_object" },
  "messages": [
    { "role": "system", "content": "..." },
    { "role": "user", "content": "{ ... payload JSON ... }" }
  ],
  "temperature": 0
}
```

Your Go `llm.go` client should accept `provider`, `base_url`, `api_key`, and `model` from config so switching Groq → OpenAI later is a config change, not a code rewrite.

**Alternative (fully offline, $0 forever):** [Ollama](https://ollama.com) runs a model locally on your laptop (`ollama pull llama3.1:8b`). No API key, no rate limits, but uses RAM/CPU and is slower. Use only if you have 16GB+ RAM and don't mind the laptop working harder 24/7.

**System prompt** (store in `internal/sentiment/prompt.go`):

```
You are a forex sentiment analyst specializing in AUD/USD.

You receive a JSON payload with current price context, upcoming economic events,
and recent headlines from central banks and news sources.

Your job is to assess market sentiment for AUD_USD and output ONLY valid JSON
matching this schema:

{
  "direction": "LONG" | "SHORT" | "FLAT",
  "confidence": 0.0 to 1.0,
  "aud_bias": "bullish" | "bearish" | "neutral",
  "drivers": ["string"],
  "risks": ["string"],
  "event_risk": "low" | "medium" | "high",
  "hold_reason": "string or null",
  "valid_minutes": 30
}

Rules:
- LONG = expect AUD_USD to rise (buy AUD, sell USD).
- SHORT = expect AUD_USD to fall (sell AUD, buy USD).
- FLAT = no clear edge; say why in hold_reason.
- If a high-impact event is within 2 hours, set event_risk="high" and direction="FLAT"
  unless headline impact is already fully priced in (explain in hold_reason).
- confidence below 0.6 should normally mean direction="FLAT".
- Base your analysis ONLY on the provided data. Do not invent news or prices.
- drivers: max 3 bullet reasons. risks: max 3 bullet risks.
```

**LLM response** (parsed into Go struct):

```go
type SentimentSignal struct {
    Direction    string    `json:"direction"`     // "LONG", "SHORT", "FLAT"
    Confidence   float64   `json:"confidence"`
    AUDBias      string    `json:"aud_bias"`
    Drivers      []string  `json:"drivers"`
    Risks        []string  `json:"risks"`
    EventRisk    string    `json:"event_risk"`
    HoldReason   *string   `json:"hold_reason"`
    ValidMinutes int       `json:"valid_minutes"`
    AnalyzedAt   time.Time // set by your code after parsing
}
```

### Step 5 — Cache and consume

- Store the signal in memory (and optionally a local JSON file for audit).
- Strategy engine reads cached signal; refreshes when `AnalyzedAt + ValidMinutes` expires.
- Log every prompt payload and LLM response to `logs/sentiment/YYYY-MM-DD.jsonl` (redact API keys).

### Estimated LLM cost

| Provider | Cost while developing |
|----------|----------------------|
| **Groq free** | **$0** |
| OpenAI GPT-4o-mini (upgrade later) | ~$0.50–1.00/day |
| Ollama local | $0 (uses your laptop's RAM/CPU) |

---

## 5. Pillar 3 — OANDA API Capabilities (Definitive Answer)

**Yes, you can fully automate long and short trades on AUD_USD via the API.** This is exactly what the OANDA v20 API is built for.

### Environments

| Mode | REST | Stream |
|------|------|--------|
| Practice (start here) | `https://api-fxpractice.oanda.com` | `https://stream-fxpractice.oanda.com` |
| Live (later) | `https://api-fxtrade.oanda.com` | `https://stream-fxtrade.oanda.com` |

### What the API can do

| Action | Endpoint | Supported? |
|--------|----------|------------|
| Stream live bid/ask | `GET /v3/accounts/{id}/pricing/stream?instruments=AUD_USD` | Yes |
| Get candle history | `GET /v3/instruments/AUD_USD/candles` | Yes |
| **Go LONG** (buy AUD) | `POST /v3/accounts/{id}/orders` with `"units": "1000"` (positive) | Yes |
| **Go SHORT** (sell AUD) | `POST /v3/accounts/{id}/orders` with `"units": "-1000"` (negative) | Yes |
| Attach stop-loss on entry | `stopLossOnFill` in order body | Yes |
| Attach take-profit on entry | `takeProfitOnFill` in order body | Yes |
| Close open trade | `PUT /v3/accounts/{id}/trades/{tradeID}/close` | Yes |
| List open trades | `GET /v3/accounts/{id}/openTrades` | Yes |
| Account balance / margin | `GET /v3/accounts/{id}/summary` | Yes |
| **Pending limit at boundary** | BUY LIMIT at support / SELL LIMIT at resistance, GTC, with SL beyond zone + TP at midpoint on fill |
| Cancel opposite limit | When one side fills, cancel the other pending order immediately |
| List/cancel pending orders | `GET /v3/accounts/{id}/pendingOrders`, `PUT .../orders/{id}/cancel` | Yes |

### Long vs short — how it works

In OANDA, there is no separate "short order" type. A **market order** with:

- **Positive units** → opens a LONG position (you bought AUD)
- **Negative units** → opens a SHORT position (you sold AUD)

Example: SHORT 1,000 units of AUD_USD (sell AUD, expect AUD to weaken) with stop-loss and take-profit. AUD/USD trades around 0.65:

```json
{
  "order": {
    "type": "MARKET",
    "instrument": "AUD_USD",
    "units": "-1000",
    "timeInForce": "FOK",
    "positionFill": "DEFAULT",
    "stopLossOnFill": {
      "price": "0.65420",
      "timeInForce": "GTC"
    },
    "takeProfitOnFill": {
      "price": "0.64840",
      "timeInForce": "GTC"
    }
  }
}
```

Example: LONG 1,000 units (buy AUD, expect AUD to strengthen):

```json
{
  "order": {
    "type": "MARKET",
    "instrument": "AUD_USD",
    "units": "1000",
    "timeInForce": "FOK",
    "positionFill": "DEFAULT",
    "stopLossOnFill": {
      "price": "0.64980",
      "timeInForce": "GTC"
    },
    "takeProfitOnFill": {
      "price": "0.65640",
      "timeInForce": "GTC"
    }
  }
}
```

OANDA fills the order, automatically creates the linked stop-loss and take-profit trades, and manages them server-side — your program does not need to watch every tick for SL/TP hits.

### Order rules (mandatory in all code)

- Every market order **must** include `stopLossOnFill`. **No `takeProfitOnFill` at entry** — profit exits via trailing stop in `positionMonitor`.
- Default `timeInForce`: `FOK` for market orders.
- Parse all API price strings to `float64` before calculating SL/TP prices.
- All request/response structs use explicit `json` struct tags in `internal/oanda/types.go`.

---

## 6. Pillar 4 — Strategy, Risk & Execution

### Critical: pips are NOT percentages

The earlier draft used **15 pips** stop-loss and **30 pips** take-profit. These are **price distances**, not account percentages.

| Term | Meaning | Example on AUD_USD @ 0.6520 |
|------|---------|----------------------------|
| **1 pip** | 0.0001 price move | 0.6520 → 0.6521 |
| **15 pip stop** | Price moves 0.0015 against you | Long entry 0.6520, SL at 0.6505 |
| **30 pip target** | Price moves 0.0030 in your favour | Long entry 0.6520, TP at 0.6550 |
| **15% account loss** | You lose 15% of your balance on one trade | On $10,000 account = **$1,500 lost** |

**A 15% loss per trade would destroy your account in a handful of bad trades. Never use percentage-of-price as stop-loss unless you mean risk-per-trade sizing (see below).**

What actually makes algo trading survive is risking a **small % of your account per trade** (typically 1%), with stop distance set by **market volatility**, not fixed pips.

### The decision algorithm (v4 — range-first, trend on breakout)

**Strategy style (user choice):** Support/resistance range trading. Identify the band the price oscillates in, **buy near the floor, sell near the ceiling**, with the LLM news sentiment acting as a veto. Patience over frequency: it is fine to wait 1–2 weeks (or longer) for price to reach a boundary. **No time-based exits anywhere** — trades are judged by price levels, not by number of days held.

The daemon runs in one of three modes, re-evaluated daily from weekly/daily candles:

```
MODE DETECTION (daily, from weekly candles)
├─ Strong weekly trend (EMA20 vs EMA50) or confirmed range breakout
│      → TREND mode (ride it — the v3 logic below)
├─ Valid range detected + flat weekly EMAs
│      → RANGE mode (primary: buy support, sell resistance)
└─ Neither (no clean range, no clean trend)
       → STAND ASIDE (no trades — protecting capital IS a position)
```

#### How the range is identified (deterministic, in code)

- **Range high / low:** rolling **13-week (≈3-month) high and low** from daily candles. Each boundary becomes a **zone**: boundary ± 0.5 × ATR(Daily).
- **Validity checks (all required):**
  - Range width ≥ **300 pips** (room to profit after costs; too-narrow ranges are noise)
  - Each boundary touched **≥ 2 times** in the lookback (proven support/resistance, not a one-off spike)
  - Weekly EMA(20)/EMA(50) flat or intertwined (no strong trend to fight)
- The **52-week high/low** (already computed for Gate 1.5) provides extra confluence: a 13-week floor sitting near the 52-week low is a stronger buy zone.
- **Range invalidation:** a **weekly close beyond a boundary zone** kills the range immediately — the system exits any open range trade and re-runs mode detection (which will usually flip to TREND mode to ride the breakout).

**Expected trade frequency:** AUD/USD touches a 3-month boundary roughly every few weeks → realistically **1–3 range trades per month**, matching your "happy to wait a week or two" preference.

#### RANGE mode — pending orders at top and bottom (primary entry method)

This is the **"place orders at the range edges and wait"** logic. OANDA has no options/puts on spot FX — we use **LIMIT orders** at the support and resistance zones:

| Range location | What you want | OANDA order | Units |
|----------------|---------------|-------------|-------|
| **Bottom (support zone)** | Buy AUD — expect bounce up | **BUY LIMIT** below current price | Positive (`"1000"`) |
| **Top (resistance zone)** | Sell AUD — expect drop down | **SELL LIMIT** above current price | Negative (`"-1000"`) |

> **Terminology note:** If you said "buy at the top / sell at the bottom", that would be *chasing* breakouts (momentum). **Range trading is the opposite:** **buy at the bottom, sell at the top.** That is what this plan implements.

**When RANGE mode activates** (valid band detected, flat weekly EMAs):

1. **Compute zones:** `support_zone` = range_low ± 0.5×ATR(D); `resistance_zone` = range_high ± 0.5×ATR(D).
2. **Place two pending LIMIT orders** (if LLM R3 veto allows — see below):
   - **Buy limit** at the **center of the support zone** (or upper edge of support zone for earlier fill).
   - **Sell limit** at the **center of the resistance zone** (or lower edge of resistance zone).
3. Each pending order includes **`stopLossOnFill`** at 1×ATR(D) **beyond** that boundary and **`takeProfitOnFill`** at the range midpoint (TP1 target; remainder managed by `positionMonitor` per exit table).
4. **`timeInForce`: `GTC`** — good-til-cancelled. Orders can sit for 1–2+ weeks until price reaches the level.
5. **Only one side fills at a time** — max 1 open position. When one limit fills, **cancel the opposite pending order** immediately (you are now in a trade; don't want the other side opening a hedge by accident).
6. **Refresh daily:** after mode re-detection, cancel stale limits and re-place at updated zone prices if the range is still valid.
7. **Cancel all pending range limits** when: range invalidates (weekly close beyond zone), mode flips to TREND or STAND ASIDE, event blackout starts, or LLM veto blocks that side.

**Example — BUY LIMIT at support (~0.6600), range midpoint ~0.6900:**

```json
{
  "order": {
    "type": "LIMIT",
    "instrument": "AUD_USD",
    "units": "1000",
    "price": "0.6600",
    "timeInForce": "GTC",
    "positionFill": "DEFAULT",
    "stopLossOnFill": { "price": "0.6520", "timeInForce": "GTC" },
    "takeProfitOnFill": { "price": "0.6900", "timeInForce": "GTC" }
  }
}
```

**Example — SELL LIMIT at resistance (~0.7200):**

```json
{
  "order": {
    "type": "LIMIT",
    "instrument": "AUD_USD",
    "units": "-1000",
    "price": "0.7200",
    "timeInForce": "GTC",
    "positionFill": "DEFAULT",
    "stopLossOnFill": { "price": "0.7280", "timeInForce": "GTC" },
    "takeProfitOnFill": { "price": "0.6900", "timeInForce": "GTC" }
  }
}
```

**News (LLM) and pending orders:** before *placing* each limit, run R3 veto. If bearish-AUD confidence ≥ 0.65, **do not place** the buy limit at support. If bullish-AUD confidence ≥ 0.65, **do not place** the sell limit at resistance. Re-check every sentiment cycle (60 min); cancel a resting limit if news turns against it.

#### RANGE mode — entry gates (market fallback + fill validation)

Pending limits are the **default**. If price is **already inside a zone** without a resting order having filled, use a **market entry** only after rejection confirmation (R2 below).

| Gate | Rule |
|------|------|
| **R0 Market conditions** | Same as Gate 0 below: event blackout, rollover blackout, spread ≤ 2.5 pips |
| **R1 Pending limits** | When range valid: place BUY LIMIT at support zone + SELL LIMIT at resistance zone (per table above), subject to R3 veto. Cancel opposite limit on fill. |
| **R1b Location (market fallback)** | If already inside support/resistance zone and no pending fill: eligible for market entry after R2. **No market entries in the middle of the range.** |
| **R2 Rejection confirmation** | **Market fallback only:** Daily candle closes back above support zone (LONG) or below resistance zone (SHORT) with RSI turn. Pending limits do not need R2 — the limit price *is* the patience mechanism. |
| **R3 LLM news veto** | Block placing or keep resting **buy limit** if bearish-AUD ≥ 0.65 or `event_risk = high`. Block **sell limit** if bullish-AUD ≥ 0.65. Full 1% size when sentiment aligns ≥ 0.75. |
| **R4 Risk manager** | 0.5% risk default (1% with aligned sentiment), all hard limits below |

#### RANGE mode — exits (price-based, never time-based)

| Exit | Rule |
|------|------|
| **Stop-loss (at entry)** | 1 × ATR(Daily) **beyond the range boundary** — if price goes that far through the floor/ceiling, the range thesis is wrong |
| **Take-profit 1** | Close **50% of the position at the range midpoint** — banks profit, reduces risk |
| **Take-profit 2** | Remaining 50% targets the **opposite boundary zone edge** (~80% of range width) |
| **Breakeven** | After TP1 fills, move SL on the remainder to entry + 2 pips — the trade can no longer lose |
| **Range invalidation** | Weekly close beyond the far boundary zone → exit at market immediately |
| **News emergency** | LLM flips strongly against the position (confidence ≥ 0.75, two consecutive readings) while trade is still below TP1 → exit early |

**Why the math works:** with a 400-pip range, a LONG at the floor risks ~1 × ATR (~50–80 pips) to make ~200 pips (midpoint) + ~320 pips (far edge) — roughly **3:1 reward-to-risk**, and range fades win more often than trend entries (typically 55–70%). Capital protection comes from the hard rule: *wrong by one ATR beyond the boundary = out, no arguing*.

#### TREND mode (secondary — only on confirmed breakout or strong weekly trend)

When a weekly close breaks the range (or weekly EMAs align into a clear trend), the range logic stands down and the v3 trend logic below takes over: ride the move with a trailing stop for as long as it lasts. This is how the system avoids the classic range-trader failure — fading a breakout repeatedly while the market runs away.

The gates below (0 through 4) are the TREND-mode pipeline. Gate 0 also applies to RANGE mode as R0.

**Weekend holds (both modes):** positions stay open Friday→Monday (your choice). Weekend gaps can jump past a stop — mitigated by boundary-based stops sized off Daily ATR and, optionally, OANDA guaranteed stop-loss orders later.

#### Gate 0 — Market conditions (deterministic, code-enforced)

These are enforced **in Go code from the economic calendar and clock — never delegated to the LLM**. The LLM's `event_risk` is a secondary opinion; the calendar blackout is law.

| Check | Rule |
|-------|------|
| **Event blackout** | No **new entries** from **60 min before** to **30 min after** any high-impact AU/US calendar event (CPI, NFP, RBA/FOMC decisions). Existing positions are **not** closed — only new entries blocked. |
| **Rollover blackout** | No **new entries** 16:45–18:15 New York time (spreads spike). Existing positions held. |
| **Weekend** | **Hold positions over the weekend** (user choice). No forced Friday close. No new entries within 2 hours of Friday close (liquidity thinning). |
| **Spread guard** | No trade if spread > 2.5 pips (AUD/USD normally runs 1–2). |
| **ATR sanity** | Skip **new entries** if ATR(14) on **Daily** < 30 pips (dead market) or > 3× its 90-day average (chaos). |

#### Gate 1 — Weekly regime filter (trade the multi-week trend)

Align entries with the **weekly** trend — the timeframe that produced the 0.68→0.73 move on your chart.

| Weekly (W) condition | Allowed directions |
|----------------------|-------------------|
| Price above EMA(20) W and EMA(20) > EMA(50) W | LONG only |
| Price below EMA(20) W and EMA(20) < EMA(50) W | SHORT only |
| EMA(20) and EMA(50) intertwined / flat | No trades (weekly chop) |

Data: OANDA weekly candles, refreshed daily.

#### Gate 1.5 — Range-location filter (don't buy the ceiling)

AUD/USD has traded in a **0.59–0.72 range** since 2021, with a multi-year ceiling at **0.72–0.75** and floor near **0.60**. Use 52-week high/low from weekly candles:

| Condition | Action |
|-----------|--------|
| LONG and price within **100 pips** of 52-week high | **Block entry** unless prior weekly close broke above the high (breakout) |
| SHORT and price within **100 pips** of 52-week low | **Block entry** unless prior weekly close broke below the low |
| Within **250 pips** of either boundary | Half position size (0.5% risk cap) |

#### Gate 2 — H4 entry timing (pullback, not chase)

Enter on **H4 pullbacks** in the weekly trend direction — don't chase extended moves.

| Direction | Conditions (all required) |
|-----------|---------------------------|
| LONG (weekly uptrend) | Price pulled back to EMA(20) H4 or within 1× ATR(D) of it; RSI(14) H4 between 40–65 (not overbought); H4 candle closes bullish |
| SHORT (weekly downtrend) | Price rallied to EMA(20) H4; RSI(14) H4 between 35–60; H4 candle closes bearish |

#### Gate 3 — LLM sentiment with persistence

A single 30-minute LLM reading is noisy. Require **persistence**:

- Direction must match Gates 1–2, with confidence ≥ 0.65.
- The **previous two** sentiment readings (60 min apart) must agree on direction (weekly holds need higher conviction than H1 scalps).
- LLM `event_risk = "high"` blocks entry even if the calendar blackout didn't catch it.

#### Gate 4 — Risk manager (sizing + limits)

Position sizing scales with conviction instead of all-or-nothing:

| Confidence | Risk per trade |
|------------|----------------|
| 0.65 – 0.75 | 0.5% of account |
| > 0.75 | 1.0% of account |

Plus all the hard limits in the risk table below.

### Profit strategy (weekly trend — wide stops, trail winners, no fixed TP)

For multi-week holds, stops must be **wide enough to survive daily noise** and weekend gaps. Profit comes from **trailing the trend**, not a fixed 2:1 target.

#### Step 1 — Calculate stop distance from Daily/Weekly ATR

```
stop_distance = ATR(14) on Daily × 2.5     // e.g. ~80–120 pips in normal AUD/USD conditions
// No fixed take-profit — use trailing stop instead
trailing_distance = ATR(14) on Daily × 2.0  // trail this far behind price once in profit
```

Wider than H1 stops by design: a weekly trend trade must survive pullbacks of 50–100 pips without stopping out.

#### Step 2 — Size position so you risk 0.5–1% of account per trade

Same formula as before, but `stop_distance` is much larger → **fewer units**, same dollar risk.

#### Step 3 — Place order with stop-loss only (no take-profit on fill)

```json
{
  "order": {
    "type": "MARKET",
    "instrument": "AUD_USD",
    "units": "1000",
    "timeInForce": "FOK",
    "stopLossOnFill": { "price": "0.6820", "timeInForce": "GTC" }
  }
}
```

Take-profit is managed by the **trailing stop logic** in `positionMonitor`, not a fixed TP at order time.

#### Why no fixed take-profit?

On your chart, the 0.68→0.73 rally was **~500 pips over ~8 weeks**. A fixed 2:1 target (e.g. 150 pips) would have exited at 0.695 and missed most of the move. Trailing lets you capture the bulk of a multi-week trend.

### Risk rules (non-negotiable — override everything, both modes)

| Rule | Default | Why |
|------|---------|-----|
| Risk per trade | **0.5–1% of account** (confidence-scaled) | Survives losing streaks |
| Stop-loss distance | RANGE: **1 × ATR(D) beyond boundary**; TREND: **2.5 × ATR(14) Daily** | Thesis-based stops |
| Take-profit | RANGE: **50% at midpoint, 50% at far edge**; TREND: trailing stop only | Match exit to strategy |
| Max open positions | 1 | One thesis at a time |
| Max daily loss | **2% of account** → halt new entries for the day | Existing positions still managed |
| Max weekly loss | **5% of account** → halt until Monday | Circuit breaker |
| Max new trades per month | **4** | Patience is the edge; waiting 1–2 weeks for a boundary is normal |
| Cooldown after a losing trade | **3 days** | A stopped-out range trade means the zone failed — wait for structure to re-form |
| Spread guard | No trade if spread > 2.5 pips | Cost control |
| Event blackout | −60/+30 min around high-impact events (new entries only) | News spikes |
| Weekend | **Hold open** — no forced Friday close | User choice; accept gap risk |
| Max hold time | **None** — exits are price-based (boundaries/trailing), never day-count | User requirement |
| Stand-aside mode | No valid range AND no trend → **zero trades** | Protecting capital is a position |
| Kill switch | `.halt` file or `POST /kill` | Emergency stop |
| Min confidence | 0.65 to veto/enter, 0.75 for full size or reversal | Require conviction |

### TREND-mode exit rules (in priority order)

1. **Initial stop-loss:** Server-side SL at entry (OANDA manages).
2. **Breakeven move:** At +1× stop_distance unrealized profit, move SL to entry + 2 pips.
3. **Trailing stop (primary profit exit):** At +1.5× stop_distance, activate trail at **2× ATR(Daily)** behind price. Ratchet up only — never widen. This is how you capture 200–500 pip weekly trends.
4. **Weekly trend break:** For LONG, close if weekly candle **closes below EMA(20) W**. For SHORT, close if weekly candle closes above EMA(20) W. The trend you rode is over.
5. **Signal reversal:** LLM flips with confidence ≥ 0.75 for **two consecutive readings** AND H4 trend breaks → close early.
6. **No time stop.** Positions stay open as long as the weekly trend holds and the trailing stop has not fired.

(RANGE-mode exits are defined in the RANGE mode section above.)

Breakeven, partial take-profit, and trailing logic run in `positionMonitor` off the live price stream and modify the OANDA SL via `PUT /v3/accounts/{id}/trades/{tradeID}/orders`.

### What changed in v4 (range-first, per your request)

| Change | Reason |
|--------|--------|
| **RANGE mode is now primary** | Your read of the chart is correct: AUD/USD spends most of its time oscillating in identifiable bands (0.59–0.72 for ~4 years). Buy support / sell resistance monetizes that. |
| **Explicit range detection** (13-week high/low, ≥2 touches, ≥300 pips wide, flat weekly EMAs) | "The range" must be computed from data, not eyeballed — and must be *proven* by repeated touches |
| **Rejection-confirmation entry (R2)** | Never buy just because price reached a number — wait for the daily candle to confirm the bounce. This is the main capital protection at boundaries |
| **News as veto, not driver (R3)** | Your instinct "adjust buying/selling based on news": sentiment blocks bad boundary trades (bearish news at support = skip) and upsizes aligned ones |
| **Partial profit at midpoint** | Banks profit at the statistically most reachable target; remainder rides to the far edge risk-free |
| **Breakout → TREND mode handoff** | The fatal range-trading mistake is fading a real breakout. A weekly close beyond the zone flips the system to trend-riding automatically |
| **Stand-aside mode** | No valid range + no trend = no trades. You said you're happy to wait — the system now formalizes that |
| **Max 4 trades/month** (was 2/week) | Range boundaries are touched every few weeks; the cap now matches the strategy's natural rhythm |
| **Pending LIMIT orders at top + bottom (v4.1)** | Place BUY LIMIT at support and SELL LIMIT at resistance (GTC); wait weeks for fill; cancel opposite side when one fills — matches "put orders at the range edges" |

### Still true from v2/v3

No time-based exits, hold over weekends, event blackouts code-enforced, confidence-scaled sizing, trade journal with expectancy tracking, ATR-based position sizing, risk manager veto on everything.

**Honest answer: no one has a guaranteed smartest FX algorithm.** Markets adapt; edges decay. What separates surviving algos from blown accounts is not a magic entry signal — it is **risk management**:

| What matters most | Share of success |
|-------------------|------------------|
| Position sizing + stop logic | ~70% |
| Avoiding bad conditions (news, chop, wide spread) | ~20% |
| Entry signal quality (LLM + indicators) | ~10% |

Your LLM sentiment layer is a **reasonable experimental entry filter**, not a crystal ball. The edge — if there is one — comes from disciplined execution, not from outsmarting the market every trade.

What this plan builds is a **professionally structured** system, not a guaranteed money printer. Validate on practice account for 4+ weeks before live capital.

### How you actually increase accuracy and profit over time: measure, don't guess

The one metric that matters is **expectancy**:

```
expectancy = (win_rate × avg_win) − (loss_rate × avg_loss)
```

Positive expectancy after spread = profitable system. Everything else is noise.

The daemon must record for **every trade**: entry/exit prices and times, direction, which gate values were true at entry (H4 regime, RSI, LLM confidence, ATR), R-multiple result (+2R, −1R, +0.3R…), and spread paid. Store as JSONL in `logs/trades/`.

After 30+ paper trades, this record answers the questions that actually improve the algorithm:

- Do high-confidence (>0.75) LLM signals outperform 0.65–0.75 ones? (If not, the LLM adds nothing — drop it and stay quant-only.)
- Do longs and shorts perform symmetrically?
- Which session's entries win: Sydney, London, or NY overlap?
- Are trailing-stop exits beating fixed 2:1 take-profits?

Tune the config values from this evidence, one change at a time. **Never tune on gut feel, and never change two things at once** — you won't know which change helped.

Also required before trusting the system: a simple **backtest harness** (Phase 5) that replays historical OANDA candles through Gates 0–2 (the deterministic gates) to sanity-check the quant core. The LLM gate can only be validated forward — another reason the quant gates must be able to stand alone.

---

## 7. Third-Party Prediction Tools — Do You Need One?

**No. Do not buy a separate "AI price prediction" service.**

| Tool | What it does | Why we don't use it |
|------|--------------|---------------------|
| TradingView alerts | Chart indicators, manual alerts | No OANDA execution; you build your own indicators |
| MetaTrader signals | Copy-trading | Different platform, not OANDA |
| "AI trading bots" (various SaaS) | Black-box signals | No audit trail, can't customize, mostly marketing |
| Finnhub pattern recognition | Technical pattern API | Useful later as an extra indicator, not a replacement for your logic |
| QuantConnect / Zipline | Backtesting frameworks | Useful in Phase 5 for backtesting, not for live sentiment |

**What you are building IS the prediction tool.** It combines:

- OANDA price data (quantitative)
- Finnhub + RSS news (qualitative)
- Groq LLM (interpretation; upgrade to GPT-4o-mini when profitable)
- Your risk rules (safety)

This gives you full control and a complete audit trail — something no SaaS provides.

---

## 8. System Architecture

```
┌──────────────────────────────────────────────────────────────────────┐
│                        fxtrade daemon (Go)                           │
├──────────────────────────────────────────────────────────────────────┤
│                                                                      │
│  ┌─────────────┐  ┌──────────────────┐  ┌───────────────────────┐   │
│  │ OANDA Stream│  │ Sentiment Worker │  │   Strategy Engine     │   │
│  │ (live ticks)│  │ (every 30 min)   │  │ (fuses sentiment +  │   │
│  │             │  │                  │  │  price indicators)    │   │
│  │ candlePoller│  │ Finnhub news     │  │                       │   │
│  │ (H1 candles)│ │ RBA/Fed RSS     │  └──────────┬────────────┘   │
│  └──────┬──────┘  │ Groq LLM        │             │                │
│         │         └────────┬─────────┘             │                │
│         │                  │                        ▼                │
│         │                  ▼              ┌─────────────────┐       │
│         │         SentimentSignal cache   │  Risk Manager   │       │
│         │                                 └────────┬────────┘       │
│         │                                          │                │
│         │                                          ▼                │
│         │                                 ┌─────────────────┐       │
│         └────────────────────────────────▶│ Order Executor  │       │
│                                           │ (OANDA market   │       │
│  ┌─────────────────┐                    │  long/short)    │       │
│  │ Position Monitor │◄───────────────────└────────┬────────┘       │
│  │ (P&L, exits)     │                             │                │
│  └─────────────────┘                             ▼                │
│                                           ┌─────────────────┐       │
│  ┌─────────────────┐                     │ Email Notifier  │       │
│  │ Health HTTP      │                     │ you@example.com │       │
│  │ GET /health      │                     └─────────────────┘       │
│  └─────────────────┘                                                │
├──────────────────────────────────────────────────────────────────────┤
│  Config (.credentials) │ slog logging │ state persistence          │
└──────────────────────────────────────────────────────────────────────┘
         │                    │                    │
         ▼                    ▼                    ▼
   OANDA v20 API       Finnhub API         Groq API (free)
   (practice)          (news + calendar)   (llama-3.3-70b)
                             +
                       RBA/Fed RSS
```

### Goroutines

| Goroutine | Responsibility |
|-----------|----------------|
| `priceStream` | OANDA pricing stream → `chan PriceTick` |
| `candlePoller` | OANDA Weekly + Daily + H4 candle refresh every 5 min → `chan Candle` |
| `sentimentWorker` | Every 30 min: fetch → normalize → LLM → `chan SentimentSignal` |
| `strategyLoop` | Consumes sentiment + candles → emits `TradeIntent` |
| `riskGate` | Validates intent against limits → forward or reject |
| `executor` | Places/closes OANDA orders |
| `positionMonitor` | Tracks open trades, triggers exits |
| `notifier` | Async email dispatch |
| `healthServer` | `GET /health` |

---

## 9. Project Structure

```
fxtrade/
├── cmd/fxtrade/main.go
├── internal/
│   ├── config/config.go
│   ├── oanda/
│   │   ├── client.go            # REST: orders, trades, candles, account
│   │   ├── stream.go            # Pricing stream with auto-reconnect
│   │   └── types.go             # All OANDA JSON structs
│   ├── sentiment/
│   │   ├── fetcher.go           # Finnhub API + RSS polling
│   │   ├── normalizer.go        # Dedupe, filter, cap headlines
│   │   ├── payload.go           # Build LLM input JSON
│   │   ├── llm.go               # LLM client (Groq/OpenAI-compatible)
│   │   ├── prompt.go            # System prompt (constant)
│   │   └── types.go             # Headline, SentimentSignal
│   ├── market/
│   │   ├── indicators.go        # EMA, RSI, ATR
│   │   └── context.go           # Build price_context for LLM payload
│   ├── strategy/
│   │   ├── engine.go            # Fuse sentiment + indicators → TradeIntent
│   │   └── rules.go             # Entry/exit logic
│   ├── risk/manager.go
│   ├── execution/executor.go
│   ├── monitor/position.go
│   └── notify/email.go
├── logs/sentiment/              # Audit trail (gitignored)
├── .credentials                 # ALL secrets (gitignored)
├── .gitignore
├── go.mod
└── plan.md
```

---

## 10. Configuration (`.credentials`)

```json
{
  "oanda": {
    "account_id": "YOUR_ACCOUNT_ID",
    "token": "YOUR_API_TOKEN",
    "environment": "practice"
  },
  "finnhub": {
    "api_key": "YOUR_FINNHUB_KEY"
  },
  "llm": {
    "provider": "groq",
    "base_url": "https://api.groq.com/openai/v1",
    "api_key": "gsk_...",
    "model": "llama-3.3-70b-versatile"
  },
  "email": {
    "smtp_host": "smtp.gmail.com",
    "smtp_port": 587,
    "username": "your@gmail.com",
    "password": "gmail-app-password",
    "alert_to": "you@example.com"
  },
  "risk": {
    "risk_per_trade_pct_base": 0.5,
    "risk_per_trade_pct_high_conf": 1.0,
    "high_conf_threshold": 0.75,
    "max_daily_loss_pct": 2.0,
    "max_weekly_loss_pct": 5.0,
    "max_trades_per_month": 4,
    "max_open_positions": 1,
    "atr_period": 14,
    "atr_timeframe": "D",
    "min_atr_pips_daily": 30,
    "max_atr_multiplier_of_avg": 3.0,
    "min_confidence": 0.65,
    "reverse_confidence": 0.75,
    "cooldown_after_loss_days": 3,
    "max_spread_pips": 2.5,
    "max_hold_hours": 0,
    "event_blackout_before_minutes": 60,
    "event_blackout_after_minutes": 30,
    "hold_over_weekend": true,
    "no_new_entries_before_friday_close_hours": 2
  },
  "range_mode": {
    "range_lookback_weeks": 13,
    "min_range_width_pips": 300,
    "min_boundary_touches": 2,
    "zone_atr_multiplier": 0.5,
    "stop_atr_beyond_boundary": 1.0,
    "tp1_fraction": 0.5,
    "tp1_target": "range_midpoint",
    "tp2_target_pct_of_range": 0.8,
    "rejection_rsi_long_below": 40,
    "rejection_rsi_short_above": 60,
    "invalidation": "weekly_close_beyond_zone",
    "pending_limits_enabled": true,
    "pending_limit_time_in_force": "GTC",
    "refresh_pending_limits_daily": true,
    "cancel_opposite_limit_on_fill": true,
    "cancel_limits_on_mode_change": true
  },
  "trend_mode": {
    "regime_ema_fast_w": 20,
    "regime_ema_slow_w": 50,
    "entry_ema_h4": 20,
    "rsi_period_h4": 14,
    "rsi_long_min": 40,
    "rsi_long_max": 65,
    "rsi_short_min": 35,
    "rsi_short_max": 60,
    "atr_stop_multiplier": 2.5,
    "trailing_atr_multiplier": 2.0,
    "breakeven_trigger_r": 1.0,
    "trailing_trigger_r": 1.5,
    "week52_block_distance_pips": 100,
    "week52_halfsize_distance_pips": 250
  },
  "llm_gate": {
    "sentiment_persistence_readings": 2,
    "sentiment_persistence_interval_minutes": 60,
    "veto_confidence": 0.65,
    "full_size_confidence": 0.75
  },
  "sentiment": {
    "interval_minutes": 60,
    "max_headlines": 25,
    "headline_max_age_hours": 48
  }
}
```

---

## 11. Development Phases

### Phase 1 — OANDA connectivity (Week 1)

- [ ] Go module, config loader, `.credentials` parsing
- [ ] OANDA REST client: account summary, pricing, candles
- [ ] OANDA pricing stream with auto-reconnect
- [ ] Health endpoint, structured logging

**Gate:** Stream AUD_USD prices for 1 hour without crash.

### Phase 2 — Execution + risk (Week 2)

- [ ] Market order builder (long + short) with TP/SL
- [ ] Place and close practice orders via API
- [ ] Position monitor, risk manager, kill switch
- [ ] Email notifications

**Gate:** Place and close 5 test orders (mix of long and short) on practice account.

### Phase 3 — Sentiment pipeline (Week 3)

- [ ] Finnhub news + calendar fetcher
- [ ] RBA + Fed RSS fetcher (gofeed)
- [ ] Normalizer (dedupe, filter, cap)
- [ ] LLM payload builder + Groq client (OpenAI-compatible)
- [ ] Sentiment signal cache + audit logging

**Gate:** Run sentiment pipeline for 24 hours; verify JSON signals in logs.

### Phase 4 — Full trading loop (Week 4)

- [ ] Price indicators (EMA, RSI, ATR) on Weekly + Daily + H4
- [ ] Mode detector: RANGE (13-week band, ≥2 touches, ≥300 pips, flat weekly EMAs) / TREND / STAND ASIDE
- [ ] RANGE mode: range detection, **pending BUY LIMIT at support + SELL LIMIT at resistance**, cancel-opposite-on-fill, daily refresh, LLM veto before place
- [ ] TREND mode: weekly regime + H4 pullback gates, trailing stop, weekly trend-break exit
- [ ] Complete loop: mode → gates → order → monitor → exit
- [ ] Trade journal: JSONL record per trade with mode, gate values, and R-multiple result
- [ ] Email on every decision (trade, no-trade with reason, mode change)

**Gate:** Paper trade full loop for 2 weeks on practice account.

### Phase 5 — Hardening + measurement (Week 5+)

- [ ] macOS `launchd` plist for auto-start on boot
- [ ] State persistence (daily/weekly P&L, trade count, last trade, sentiment cache)
- [ ] Integration tests against OANDA practice API
- [ ] Backtest harness: replay historical OANDA candles through Gates 0–2 (quant core)
- [ ] Expectancy report from trade journal (win rate, avg R, by session / confidence bucket)

**Gate:** 4+ weeks stable paper trading AND positive expectancy over 30+ trades before live consideration.

---

## 12. Go Dependencies

| Package | Purpose |
|---------|---------|
| Standard library | `net/http`, `encoding/json`, `log/slog`, `context` |
| `github.com/mmcdole/gofeed` | Parse RSS feeds (RBA, Fed) |
| Custom HTTP clients | OANDA, Finnhub, LLM (thin wrappers, ~200 lines each) |

No heavy frameworks. Custom OANDA client for full control over struct tags and error handling.

---

## 13. Coding Standards

- Go 1.22+, `go fmt`, `go vet`.
- All external API types use explicit `json` struct tags.
- Convert API price strings to `float64` at the boundary, never inside business logic.
- Goroutines communicate via typed channels; `context.Context` for cancellation.
- No credentials in source code or git.
- Every trade decision logged with correlation ID.
- Every LLM call logged with input payload and output (redact keys).

---

## 14. Email Notifications

Send to `you@example.com` on:

- Order submitted (direction, units, entry, SL, TP)
- Order filled or rejected
- Position closed (P&L)
- Sentiment update (direction, confidence, drivers)
- No-trade decision with reason (e.g. "indicators disagree", "event_risk high")
- Daily loss limit hit
- Daemon start / stop / crash recovery

---

## 15. Immediate Next Steps

1. **Create accounts/API keys (all free to start):**
   - OANDA practice account → API token from "Manage API Access" (free)
   - Finnhub.io → API key on **free tier** ($0) — no credit card needed
   - Groq → API key from [console.groq.com](https://console.groq.com) (free, no credit card)
   - Gmail → app-specific password for SMTP (free)
2. **Populate `.credentials`** with all keys above.
3. **Approve this plan.**
4. **Begin Phase 1** implementation.

---

## Appendix A — Agent Prompt (use in Cursor when implementing)

```
You are an expert Go engineer building fxtrade — a local 24/7 AUD_USD trading daemon.

ARCHITECTURE (do not deviate):
- OANDA v20 API for all price data and trade execution (practice environment)
- Finnhub API for forex news headlines and economic calendar
- RBA RSS (rss-cb-media-releases.xml, rss-cb-speeches.xml) and Fed RSS
  (press_monetary.xml) for central bank announcements
- Groq free LLM (llama-3.3-70b) for sentiment analysis (JSON mode, every 60 minutes)
  Upgrade to OpenAI GPT-4o-mini via config when profitable
- Range-first strategy: detect support/resistance band, buy floor / sell ceiling,
  news sentiment as veto; flip to trend-riding on confirmed breakout; stand aside otherwise
- No time-based exits anywhere; hold through weekends; risk manager vetoes every trade

DATA PIPELINE:
1. Fetch headlines from Finnhub + RSS + calendar + OANDA candles (W, D, H4)
2. Normalize: dedupe, filter for AUD/USD relevance, cap at 25 headlines
3. Build JSON payload with 52-week range context (see plan.md Section 4, Step 3)
4. Send to LLM (Groq free) with system prompt (see plan.md Section 4, Step 4)
5. Parse SentimentSignal, cache for valid_minutes
6. Strategy engine reads cached signal + weekly/H4 indicators

OANDA ORDERS:
- LONG = positive units, SHORT = negative units on AUD_USD
- Every market order MUST include stopLossOnFill; do NOT set takeProfitOnFill at entry
- Use FOK for market orders
- Parse all price strings to float64 before SL calculation
- All OANDA types in internal/oanda/types.go with json struct tags

STRATEGY (dual-mode, range-first — see plan.md Section 6):
- Mode detection daily: RANGE (13-week high/low band, >= 2 touches each boundary,
  >= 300 pips wide, flat weekly EMAs) / TREND (weekly EMA20 vs EMA50 aligned, or
  confirmed weekly-close breakout of the range) / STAND ASIDE (neither -> no trades)
- RANGE mode (primary): when range valid, place GTC BUY LIMIT at support zone and SELL LIMIT
  at resistance zone (buy bottom / sell top); cancel opposite limit when one fills; refresh daily;
  LLM veto before placing each side; market+rejection fallback if price already in zone;
  stop = 1 x ATR(D) beyond boundary; TP1 = 50% at midpoint then breakeven on remainder
- TREND mode: weekly regime gate, H4 pullback entry (RSI bands), stop = 2.5 x ATR(D),
  no fixed TP, trailing 2 x ATR(D) once +1.5R, weekly EMA20 close-through = exit
- Gate 0 blackouts apply to NEW entries in both modes (calendar -60/+30 min, rollover,
  spread > 2.5 pips, daily ATR sanity)

RISK (non-negotiable):
- Risk 0.5-1% per trade (1% only with aligned sentiment >= 0.75); size always calculated
- Max 4 new trades/month; 3-day cooldown after a losing trade
- NO time-based exits anywhere; hold over weekends (no Friday flat)
- Max 1 open position; daily loss 2% halt; weekly loss 5% halt; kill switch via .halt file

CREDENTIALS:
- Load from .credentials (never hardcode or commit)
- Email alerts to you@example.com on every order and sentiment update

CODE STYLE:
- Match package layout in plan.md Section 9
- Minimal diffs, no over-engineering
- Log every trade decision and LLM call with correlation ID
- Follow the phase plan; do not skip phases
```
