package sentiment

import (
	"fmt"
	"strings"

	"github.com/ym/fxtrade/internal/notify"
)

func FormatEmailBody(signal SentimentSignal, intervalMinutes int) string {
	if intervalMinutes <= 0 {
		intervalMinutes = 30
	}
	instrument := signal.Instrument
	if instrument == "" {
		instrument = "AUD_USD"
	}
	var b strings.Builder
	fmt.Fprintf(&b, "%s sentiment update (every %d min)\n", instrument, intervalMinutes)
	b.WriteString("This is an LLM news opinion only — it does NOT place a trade.\n\n")

	fmt.Fprintf(&b, "Direction: %s\n", signal.Direction)
	fmt.Fprintf(&b, "Confidence: %.0f%%\n", signal.Confidence*100)
	fmt.Fprintf(&b, "Base currency bias: %s\n", signal.BaseBias)
	fmt.Fprintf(&b, "Event risk: %s\n", signal.EventRisk)
	if signal.HoldReason != nil && *signal.HoldReason != "" {
		fmt.Fprintf(&b, "Hold reason: %s\n", *signal.HoldReason)
	}

	b.WriteString("\nWhy (drivers):\n")
	b.WriteString(notify.BulletList(signal.Drivers))

	b.WriteString("\nRisks:\n")
	b.WriteString(notify.BulletList(signal.Risks))

	b.WriteString("\nTrades still require RANGE or TREND mode plus all quant gates.\n")
	return b.String()
}
