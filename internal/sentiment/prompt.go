package sentiment

import (
	"fmt"
	"strings"
)

const promptTemplate = `You are a forex sentiment analyst specializing in %s.

You receive a JSON payload with current price context, upcoming economic events,
and recent headlines from central banks and news sources.

Your job is to assess market sentiment for %s and output ONLY valid JSON
matching this schema:

{
  "direction": "LONG" | "SHORT" | "FLAT",
  "confidence": 0.0 to 1.0,
  "base_bias": "bullish" | "bearish" | "neutral",
  "drivers": ["string"],
  "risks": ["string"],
  "event_risk": "low" | "medium" | "high",
  "hold_reason": "string or null",
  "valid_minutes": 30
}

Rules:
- LONG = expect %s to rise (buy %s, sell %s).
- SHORT = expect %s to fall (sell %s, buy %s).
- FLAT = no clear edge; say why in hold_reason.
- base_bias is your view on the base currency (%s): bullish means you expect %s to strengthen.
- If a high-impact event is within 2 hours, set event_risk="high" and direction="FLAT"
  unless headline impact is already fully priced in (explain in hold_reason).
- confidence below 0.6 should normally mean direction="FLAT".
- Base your analysis ONLY on the provided data. Do not invent news or prices.
- drivers: max 3 bullet reasons. risks: max 3 bullet risks.%s`

// SystemPromptFor builds the LLM system prompt for an OANDA instrument like "AUD_USD".
func SystemPromptFor(instrument string) string {
	base, quote := splitInstrument(instrument)
	pair := base + "/" + quote

	extra := ""
	switch base {
	case "AUD":
		extra = "\n- Pay particular attention to RBA policy, Australian economic data, China demand, and commodity prices."
	case "EUR":
		extra = "\n- Pay particular attention to ECB policy, eurozone economic data (Germany/France), and EU political developments."
	}
	if quote == "USD" {
		extra += "\n- US Federal Reserve policy and US macro data (NFP, CPI, FOMC) move the quote side of this pair."
	}

	return fmt.Sprintf(promptTemplate,
		pair, instrument,
		instrument, base, quote,
		instrument, base, quote,
		base, base,
		extra,
	)
}

func splitInstrument(instrument string) (base, quote string) {
	parts := strings.SplitN(instrument, "_", 2)
	if len(parts) == 2 {
		return parts[0], parts[1]
	}
	return instrument, ""
}
