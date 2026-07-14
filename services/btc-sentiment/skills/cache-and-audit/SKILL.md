---
name: cache-and-audit
description: Cache keying, TTL, and run logging rules for backtesting.
---

# Cache and audit skill

## Window key

- Call `compute_window_key` with the headline strings and Reddit post texts you collected.
- The tool returns a SHA256 hex digest. Use that exact value for `cache_get` / `cache_set`.

## Cache

- On hit: emit the cached result; still call `log_run` with `cache_used` true when possible.
- On miss after scoring: call `cache_set` before `emit_sentiment`.

## Audit

- Call `log_run` once per pass with window bounds, item counts, parsed result, and flags (`cache_used`, `fallback_used`).
- Prefer logging before the final `emit_sentiment`.
