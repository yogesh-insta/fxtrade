---
name: data-sources
description: When and how to use news and Reddit fetch tools.
---

# Data sources skill

## Tools

- `fetch_news`: CryptoPanic when configured, otherwise RSS (CoinDesk / CoinTelegraph BTC-related items).
- `fetch_reddit`: Reddit OAuth listing for configured subreddits.

## Guidance

- Always call both fetch tools for a full pass unless a previous tool error makes a retry useless in-window.
- Pass `start` and `end` as RFC3339 UTC timestamps from the user message.
- Treat tool errors as partial failure: continue with the other source if it has items.
- Empty results are normal; they are not crashes.
- Do not invent headlines or posts. Only use tool payloads.
