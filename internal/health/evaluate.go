package health

import (
	"fmt"
	"strings"
	"time"
)

const DefaultStaleTickAge = 10 * time.Minute

// Evaluate checks daemon health. Returns ok=false with human-readable issues when alerting is warranted.
func Evaluate(st Status, now time.Time, staleAge time.Duration) (ok bool, issues []string) {
	if staleAge <= 0 {
		staleAge = DefaultStaleTickAge
	}
	if st.Halted {
		issues = append(issues, "kill switch / halt active")
	}
	if !st.StreamConnected {
		issues = append(issues, "OANDA price stream disconnected")
	}
	if fxMarketLikelyOpen(now) {
		if !st.LastTickAt.IsZero() && now.Sub(st.LastTickAt) > staleAge {
			issues = append(issues, fmt.Sprintf("stale ticks (last %s)", st.LastTickAt.UTC().Format(time.RFC3339)))
		} else if st.StreamConnected && st.LastTickAt.IsZero() && !st.StartedAt.IsZero() && now.Sub(st.StartedAt) > staleAge {
			issues = append(issues, "no price ticks received since startup")
		}
	}
	for _, b := range st.Bots {
		if !b.Running {
			issues = append(issues, fmt.Sprintf("bot %s not running", b.ID))
		}
	}
	return len(issues) == 0, issues
}

// IncidentKey returns a stable key for deduplicating alert emails.
func IncidentKey(issues []string) string {
	return strings.Join(issues, "|")
}

// fxMarketLikelyOpen approximates OANDA FX hours to avoid stale-tick alerts when the market is closed.
func fxMarketLikelyOpen(now time.Time) bool {
	utc := now.UTC()
	switch utc.Weekday() {
	case time.Saturday:
		return false
	case time.Sunday:
		return utc.Hour() >= 22
	case time.Friday:
		return utc.Hour() < 22
	default:
		return true
	}
}
