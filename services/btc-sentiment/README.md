# BTC/USD Sentiment Agent (Cloud Run + Gemini function calling)

Real agent runtime: **skills** (markdown) + **tools** (Gemini function declarations) + a thin **system prompt**. Gemini chooses which tools to call; Cloud Scheduler triggers `POST /run-sentiment-pass`.

```text
Cloud Scheduler → HTTP → Agent loop (Gemini + tools) → emit_sentiment → JSON
```

## Architecture

| Layer | Location | Role |
|-------|----------|------|
| System prompt | [`internal/agent/skills.go`](internal/agent/skills.go) | Thin wrapper + all skill bodies |
| Skills | [`internal/agent/skills/*/SKILL.md`](internal/agent/skills/) (mirrored in [`skills/`](skills/)) | Operating rules the agent must follow |
| Tools | [`internal/tools/`](internal/tools/) | `fetch_news`, `fetch_reddit`, `compute_window_key`, `cache_get`, `cache_set`, `log_run`, `emit_sentiment` |
| Gemini FC client | [`internal/gemini/`](internal/gemini/) | `generateContent` with `functionDeclarations` |
| Agent loop | [`internal/agent/agent.go`](internal/agent/agent.go) | Multi-turn tool loop until `emit_sentiment` |
| HTTP | [`cmd/server`](cmd/server), [`internal/server`](internal/server) | Cloud Run entry |

### Tools

| Tool | Purpose |
|------|---------|
| `fetch_news` | CryptoPanic or RSS headlines for window |
| `fetch_reddit` | Reddit OAuth posts for configured subs |
| `compute_window_key` | SHA256 cache key from texts |
| `cache_get` / `cache_set` | SQLite result cache |
| `log_run` | Audit row for backtests |
| `emit_sentiment` | **Terminal** — final `SentimentResult` |

### Skills

- `core` — mission + required workflow
- `data-sources` — how to use fetch tools
- `sentiment-scoring` — score schema / confidence rules
- `cache-and-audit` — window key + logging

## Reddit setup

Reddit is already wired as the `fetch_reddit` tool. You only need OAuth credentials.

1. Go to https://www.reddit.com/prefs/apps → **create another app…**
2. Choose type **script**, name e.g. `fxtrade-btc-sentiment`, redirect `http://localhost:8080`
3. Copy:
   - **client id** (string under the app name)
   - **secret**
4. Put them in repo-root [`.credentials`](../../.credentials) under `btc_sentiment` (template in [`.credentials.example`](../../.credentials.example)):

```json
"btc_sentiment": {
  "reddit_client_id": "YOUR_ID",
  "reddit_client_secret": "YOUR_SECRET",
  "reddit_user_agent": "fxtrade:btc-sentiment:1.0 (by /u/YOUR_REDDIT_USERNAME)",
  "reddit_subreddits": "Bitcoin,CryptoCurrency"
}
```

5. Smoke-test (from `services/btc-sentiment`):

```bash
go run ./cmd/reddit-smoke
```

You should see post counts from the configured subreddits. Then restart the agent server — it loads `.credentials` automatically (`CREDENTIALS_PATH` override supported). Env vars `REDDIT_CLIENT_ID` / `REDDIT_CLIENT_SECRET` still override the file.

## Environment

| Variable | Required | Description |
|----------|----------|-------------|
| `GEMINI_API_KEY` | yes* | Or `afl.gemini_api_key` / `btc_sentiment.gemini_api_key` in `.credentials` |
| `GEMINI_MODEL` | no | Default `gemini-2.5-flash-lite` |
| `CRYPTOPANIC_API_KEY` | no | Else RSS fallback |
| `REDDIT_CLIENT_ID` / `REDDIT_CLIENT_SECRET` | for Reddit | Or `btc_sentiment.reddit_*` in `.credentials` |
| `REDDIT_USER_AGENT` | no | Must include your Reddit username |
| `REDDIT_SUBREDDITS` | no | Default `Bitcoin,CryptoCurrency` |
| `CREDENTIALS_PATH` | no | Path to `.credentials` |
| `WINDOW_HOURS` | no | Default `4` |
| `CACHE_TTL_HOURS` | no | Default `4` |
| `MIN_ITEMS_THRESHOLD` | no | Default `3` |
| `DB_PATH` | no | Default `/tmp/btc-sentiment.db` |
| `PORT` | Cloud Run | Listen port |

\*Loaded from `.credentials` when env is unset.

## Local run

```bash
cd services/btc-sentiment
# credentials auto-loaded from ../../.credentials (Gemini from afl.*, Reddit from btc_sentiment.*)
go run ./cmd/reddit-smoke   # verify Reddit first
go run ./cmd/server

curl -s localhost:8080/healthz
curl -s -X POST localhost:8080/run-sentiment-pass -H 'Content-Type: application/json' -d '{}'
```

## Tests

```bash
go test ./...
```

## Deploy (Cloud Run + Scheduler)

Same shape as before — image from this directory, secrets via Secret Manager, Scheduler OIDC to `POST /run-sentiment-pass`, `--min-instances=0`, `GEMINI_MODEL=gemini-2.5-flash-lite`.

See previous README deploy block; swap Anthropic secrets for `GEMINI_API_KEY=gemini-api-key:latest`.
