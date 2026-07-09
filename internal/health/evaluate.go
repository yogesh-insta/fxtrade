package health

import (
	"fmt"
	"strings"
	"time"

	"github.com/ym/fxtrade/internal/config"
)

const (
	DefaultStaleTickAge  = 10 * time.Minute
	DefaultStaleCycleAge = 15 * time.Minute // btc_cfd runs 24/7
	fxStaleCycleAge      = 30 * time.Minute // FX bots during market hours
)

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
			continue
		}
		if issue := staleCycleIssue(b, now); issue != "" {
			issues = append(issues, issue)
		}
	}
	return len(issues) == 0, issues
}

// staleCycleIssue reports when a running bot has not completed a successful cycle
// recently. btc_cfd is checked 24/7; FX bots only during likely market hours.
func staleCycleIssue(b BotStatus, now time.Time) string {
	if b.LastCycleOKAt.IsZero() {
		return ""
	}
	threshold, check := staleCycleThreshold(b.ID, now)
	if !check {
		return ""
	}
	age := now.Sub(b.LastCycleOKAt)
	if age <= threshold {
		return ""
	}
	return fmt.Sprintf("bot %s stale cycle (last ok %s, %s ago)",
		b.ID, b.LastCycleOKAt.UTC().Format(time.RFC3339), age.Round(time.Minute))
}

func staleCycleThreshold(botID string, now time.Time) (time.Duration, bool) {
	switch botID {
	case config.BotBtcCfd:
		return DefaultStaleCycleAge, true
	case config.BotUniverseScanner, config.BotFxSentiment:
		return fxStaleCycleAge, fxMarketLikelyOpen(now)
	default:
		return DefaultStaleCycleAge, fxMarketLikelyOpen(now)
	}
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
