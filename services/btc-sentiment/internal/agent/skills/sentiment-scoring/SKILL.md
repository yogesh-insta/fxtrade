---
name: sentiment-scoring
description: How to produce and validate the BTC/USD SentimentResult JSON schema.
---

# Sentiment scoring skill

When scoring (cache miss with at least some items), reason from headlines and posts only.

## Rules

- Weight recent items more heavily when timestamps are present.
- Distinguish sentiment about price direction (bullish/bearish) from unrelated BTC news (regulation, adoption) that could still move price.
- If input is empty, contradictory, or too sparse: set `low_confidence` true rather than guessing.
- If total item count is below the configured minimum threshold provided in the user message, set `low_confidence` true.

## Output schema (emit via `emit_sentiment`)

```json
{
  "sentiment_score": <float, -1.0 to 1.0>,
  "confidence": <float, 0.0 to 1.0>,
  "low_confidence": <boolean>,
  "key_drivers": [<string>, max 3],
  "divergence_note": <string or null>
}
```

Do not wrap the tool args in markdown fences. Pass fields as structured tool arguments.

## Failure

If you cannot form a valid score after gathering data, emit the neutral fallback:

- `sentiment_score` 0, `confidence` 0, `low_confidence` true
- `divergence_note`: `scoring failed, using neutral default`
